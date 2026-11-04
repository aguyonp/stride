package analysis

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/sync/errgroup"

	"stride/internal/config"
	"stride/internal/fitdata"
	"stride/internal/garmin"
	"stride/internal/logx"
)

// MaxHistoryDays borne le paramètre history.
const MaxHistoryDays = 90

// Reporter génère les rapports texte (implémenté par le package report).
type Reporter interface {
	Activity(a *ActivityAnalysis, p config.Profile) string
	Day(d *DayAnalysis, p config.Profile) string
}

// Service orchestre récupération des données et calculs.
type Service struct {
	G        *garmin.Client
	FIT      *fitdata.Store
	Settings *config.Store
	Reporter Reporter
	Log      *slog.Logger
	Now      func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Today retourne la date du jour dans le fuseau de l'application.
func (s *Service) Today() string {
	return s.now().In(s.Settings.Location()).Format("2006-01-02")
}

// EstimateTokens ≈ nombre de caractères / 4.
func EstimateTokens(report string) int {
	return (utf8.RuneCountInString(report) + 3) / 4
}

// ListActivities : GET /api/activities.
func (s *Service) ListActivities(ctx context.Context, start, limit int, from, to string) (*ActivityListResponse, error) {
	items, err := s.G.ListActivities(ctx, garmin.ListQuery{Start: start, Limit: limit + 1, StartDate: from, EndDate: to})
	if err != nil {
		return nil, err
	}
	resp := &ActivityListResponse{Items: []ActivitySummary{}}
	if len(items) > limit {
		resp.HasMore = true
		items = items[:limit]
	}
	for i := range items {
		resp.Items = append(resp.Items, SummaryFromList(&items[i]))
	}
	return resp, nil
}

// fetchOpts : sources optionnelles.
type fetchOpts struct {
	listFrom string // début de la liste d'activités ("" = pas de liste)
	zones    bool
}

// collector accumule sources/avertissements de manière thread-safe.
type collector struct {
	mu       sync.Mutex
	sources  []string
	seen     map[string]bool
	warnings []string
	errs     []error
	okCount  int
}

func (c *collector) ok(src string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.okCount++
	if c.seen == nil {
		c.seen = map[string]bool{}
	}
	if !c.seen[src] {
		c.seen[src] = true
		c.sources = append(c.sources, src)
	}
}

func (c *collector) fail(label string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errs = append(c.errs, err)
	c.warnings = append(c.warnings, fmt.Sprintf("%s indisponible (%s)", label, describeErr(err)))
}

func (c *collector) noData() {
	c.mu.Lock()
	c.okCount++
	c.mu.Unlock()
}

func describeErr(err error) string {
	var ae *garmin.APIError
	switch {
	case errors.Is(err, garmin.ErrNoAPIKey):
		return "clé API absente"
	case garmin.IsTimeout(err):
		return "délai dépassé"
	case errors.As(err, &ae) && ae.Status > 0:
		return fmt.Sprintf("HTTP %d", ae.Status)
	case errors.As(err, &ae):
		return "erreur réseau"
	case errors.Is(err, context.Canceled):
		return "requête annulée"
	}
	return "erreur de lecture"
}

// fetchWellness récupère en parallèle les données bien-être autour de la date J.
func (s *Service) fetchWellness(ctx context.Context, date string, o fetchOpts, col *collector) *wellness {
	w := &wellness{date: date}
	d6, d1 := dateAdd(date, -6), dateAdd(date, -1)
	var g errgroup.Group
	g.SetLimit(8)
	run := func(src, label string, f func() error) {
		g.Go(func() error {
			err := f()
			switch {
			case err == nil:
				col.ok(src)
			case errors.Is(err, garmin.ErrNoData):
				col.noData()
			default:
				s.Log.Warn(fmt.Sprintf("⚠️ %s indisponible pour %s", label, date), "err", err.Error())
				col.fail(label, err)
			}
			return nil // jamais d'échec global
		})
	}
	run("sleep", "Sommeil", func() (err error) { w.sleep, err = s.G.DailySleep(ctx, date); return })
	run("hrv", "VFC", func() error {
		h, err := s.G.HRVDay(ctx, date)
		if err == nil && h != nil {
			w.hrvDay = h.HrvSummary
		}
		return err
	})
	run("hrv", "VFC (7 jours)", func() (err error) { w.hrvRange, err = s.G.HRVRange(ctx, d6, date); return })
	run("bodyBattery", "Body Battery", func() (err error) { w.bb, err = s.G.BodyBattery(ctx, d1, date); return })
	run("dailyHeartRate", "FC du jour", func() (err error) { w.dhr, err = s.G.DailyHeartRate(ctx, date); return })
	run("dailySummaries", "Résumés quotidiens", func() (err error) { w.sums, err = s.G.DailySummaries(ctx, d6, date); return })
	run("steps", "Pas", func() (err error) { w.steps, err = s.G.Steps(ctx, d6, date); return })
	run("sleep", "Sommeil (7 jours)", func() (err error) { w.sleeps, err = s.G.SleepRange(ctx, d6, date); return })
	if o.listFrom != "" {
		run("activityList", "Liste d'activités", func() error {
			acts, err := s.G.ListActivitiesRange(ctx, o.listFrom, date)
			if err == nil {
				w.acts, w.actsOK = acts, true
			}
			return err
		})
	}
	if o.zones {
		run("hrZones", "Zones FC", func() (err error) { w.zones, err = s.G.HRZones(ctx); return })
	}
	_ = g.Wait()
	return w
}

// AnalyzeActivity : GET /api/activities/{id}/analysis.
func (s *Service) AnalyzeActivity(ctx context.Context, id int64, mode Mode, historyDays int) (*ActivityAnalysis, error) {
	t0 := time.Now()
	prof := s.Settings.Profile()
	if historyDays > MaxHistoryDays {
		historyDays = MaxHistoryDays
	}

	// Le FIT (le plus lourd) part immédiatement, en parallèle du détail.
	var g errgroup.Group
	var fitAct *fitdata.Activity
	var fitErr error
	g.Go(func() error {
		fitAct, fitErr = s.FIT.Load(ctx, id)
		return nil
	})

	det, err := s.G.Activity(ctx, id)
	if err != nil {
		_ = g.Wait()
		return nil, err
	}
	date := det.LocalDate()
	col := &collector{}
	col.ok("activity")

	listDays := 28
	if historyDays > listDays {
		listDays = historyDays
	}
	var w *wellness
	g.Go(func() error {
		w = s.fetchWellness(ctx, date, fetchOpts{listFrom: dateAdd(date, -(listDays - 1)), zones: mode == ModePerformance}, col)
		return nil
	})
	_ = g.Wait()

	if fitErr == nil && fitAct != nil {
		col.ok("fit")
	} else {
		if fitErr != nil {
			s.Log.Warn(fmt.Sprintf("⚠️ FIT #%d indisponible, repli sur /details", id), "err", fitErr.Error())
			col.fail("Fichier FIT", fitErr)
		}
		start, _ := det.StartGMT()
		if d, err := s.G.ActivityDetails(ctx, id, 0); err == nil {
			fitAct = fitdata.FromDetails(d, start)
			col.ok("details")
		} else {
			col.fail("Séries détaillées", err)
		}
	}

	var it *garmin.ActivityListItem
	for i := range w.acts {
		if w.acts[i].ActivityID == id {
			it = &w.acts[i]
			break
		}
	}

	var sess *session
	if fitAct != nil && len(fitAct.Records) > 0 {
		sess = newSession(fitAct, IsRunningType(det.ActivityTypeDTO.TypeKey))
	}
	a := &ActivityAnalysis{
		Mode:        mode,
		GeneratedAt: s.now().In(s.Settings.Location()).Format(time.RFC3339),
		Activity:    DetailFromAPI(det, it),
		HistoryDays: historyDays,
		Alerts:      []Alert{},
		Warnings:    []string{},
	}
	isRun := a.Activity.IsRunning
	a.Biomechanics = computeBiomech(sess, det, prof, isRun)
	a.Segments = buildSegments(sess)
	a.Series = buildSeries(sess)

	// Zones FC : liste Garmin > FIT > calcul sur records avec les zones du profil Garmin.
	var hrZones, pwZones []float64
	if it != nil {
		hrZones, pwZones = it.HRZones(), it.PowerZones()
	}
	if hrZones == nil && fitAct != nil {
		hrZones = fitAct.HRZonesS
	}
	if pwZones == nil && fitAct != nil {
		pwZones = fitAct.PowerZonesS
	}
	if hrZones == nil {
		hrZones = zonesFromRecords(sess, pickZones(w.zones))
	}
	hrZones, pwZones = roundZones(hrZones), roundZones(pwZones)

	elevLoss := pf(det.Summary.ElevationLoss)
	elevGain := pf(det.Summary.ElevationGain)
	if fitAct != nil {
		elevLoss = first(elevLoss, fitAct.Session.DescentM)
		elevGain = first(elevGain, fitAct.Session.AscentM)
	}
	drift := computeDrift(sess)
	a.Load = Load{
		ElevGainM:    num(elevGain, 0),
		ElevLossM:    num(elevLoss, 0),
		Drift:        drift,
		TrainingLoad: a.Activity.TrainingLoad,
		AerobicTE:    a.Activity.AerobicTE,
		AnaerobicTE:  a.Activity.AnaerobicTE,
		HrZonesS:     hrZones,
		PowerZonesS:  pwZones,
	}
	a.Recovery = buildRecovery(w)
	if mode == ModePerformance {
		a.Performance = buildPerformance(sess, det, it, w, prof, hrZones, a.Activity.StartLocal)
	}
	if historyDays > 0 {
		a.History = buildHistory(w, dateAdd(date, -(historyDays-1)), date, id, a.Activity.StartLocal)
		if !w.actsOK {
			a.History = nil
		}
	}

	in := alertInput{
		isRunning: isRun,
		bio:       &a.Biomechanics,
		elevLossM: elevLoss,
		drift:     drift,
		fadePct:   fadeEnd(a.Segments),
		recovery:  a.Recovery,
	}
	if a.Performance != nil {
		in.acwr = a.Performance.ACWR
	}
	a.Alerts = buildAlerts(in, prof)
	a.DataSources = col.sources
	if col.warnings != nil {
		a.Warnings = col.warnings
	}
	s.Log.Info(fmt.Sprintf("🧮 Analyse #%d (%s) en %s", id, mode, logx.Ms(time.Since(t0))),
		"sources", len(a.DataSources), "avertissements", len(a.Warnings))

	if s.Reporter != nil {
		a.Report = s.Reporter.Activity(a, prof)
		a.ReportTokensEstimate = EstimateTokens(a.Report)
		s.Log.Info(fmt.Sprintf("📝 Rapport %s #%d · ≈ %d tokens", mode, id, a.ReportTokensEstimate))
	}
	return a, nil
}

func roundZones(z []float64) []float64 {
	if z == nil {
		return nil
	}
	out := make([]float64, len(z))
	for i, v := range z {
		out[i] = roundTo(v, 0)
	}
	return out
}

// buildHistory : activités dont la date locale est dans [from, to], hors `excludeID`
// et hors activités démarrées après `untilLocal` (si fourni), ordre chronologique.
func buildHistory(w *wellness, from, to string, excludeID int64, untilLocal string) []HistoryRow {
	var items []*garmin.ActivityListItem
	for i := range w.acts {
		a := &w.acts[i]
		d := a.LocalDate()
		if a.ActivityID == excludeID || d < from || d > to {
			continue
		}
		if untilLocal != "" && garmin.NormalizeLocal(a.StartTimeLocal) > untilLocal {
			continue
		}
		items = append(items, a)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].StartTimeLocal < items[j].StartTimeLocal })
	out := []HistoryRow{}
	for _, it := range items {
		out = append(out, historyRow(it))
	}
	return out
}

// ErrAllSourcesFailed est retourné quand aucune donnée n'a pu être lue pour un jour.
var ErrAllSourcesFailed = errors.New("aucune donnée Garmin n'a pu être récupérée")

// AnalyzeDay : GET /api/days/{date}/analysis (Mode 2 « Santé & Récupération »).
func (s *Service) AnalyzeDay(ctx context.Context, date string, historyDays int) (*DayAnalysis, error) {
	t0 := time.Now()
	prof := s.Settings.Profile()
	if historyDays > MaxHistoryDays {
		historyDays = MaxHistoryDays
	}
	back := 6
	if historyDays > back {
		back = historyDays
	}
	col := &collector{}
	w := s.fetchWellness(ctx, date, fetchOpts{listFrom: dateAdd(date, -back)}, col)
	if col.okCount == 0 && len(col.errs) > 0 {
		for _, e := range col.errs {
			if errors.Is(e, garmin.ErrNoAPIKey) || garmin.IsUnauthorized(e) {
				return nil, e
			}
		}
		return nil, errors.Join(ErrAllSourcesFailed, col.errs[0])
	}

	d := &DayAnalysis{
		Mode:        ModeRecovery,
		Date:        date,
		GeneratedAt: s.now().In(s.Settings.Location()).Format(time.RFC3339),
		Day:         buildDayInfo(w),
		Activities:  []ActivitySummary{},
		Alerts:      []Alert{},
		Warnings:    []string{},
		HistoryDays: historyDays,
	}
	if rc := buildRecovery(w); rc != nil {
		d.Recovery = *rc
	} else {
		d.Recovery = RecoveryContext{Date: date, Trend: buildTrend(w)}
	}
	var today []*garmin.ActivityListItem
	for i := range w.acts {
		if w.acts[i].LocalDate() == date {
			today = append(today, &w.acts[i])
		}
	}
	sort.SliceStable(today, func(i, j int) bool { return today[i].StartTimeLocal < today[j].StartTimeLocal })
	for _, it := range today {
		d.Activities = append(d.Activities, SummaryFromList(it))
	}
	if historyDays > 0 && w.actsOK {
		d.History = buildHistory(w, dateAdd(date, -historyDays), dateAdd(date, -1), 0, "")
	}
	rc := d.Recovery
	d.Alerts = buildAlerts(alertInput{recovery: &rc, floorsUp: pf(d.Day.FloorsUp), dayMode: true, elevLossM: nan, fadePct: nan}, prof)
	d.DataSources = col.sources
	if d.DataSources == nil {
		d.DataSources = []string{}
	}
	if col.warnings != nil {
		d.Warnings = col.warnings
	}
	s.Log.Info(fmt.Sprintf("🧮 Analyse jour %s en %s", date, logx.Ms(time.Since(t0))), "sources", len(d.DataSources), "avertissements", len(d.Warnings))
	if s.Reporter != nil {
		d.Report = s.Reporter.Day(d, prof)
		d.ReportTokensEstimate = EstimateTokens(d.Report)
		s.Log.Info(fmt.Sprintf("📝 Rapport jour %s · ≈ %d tokens", date, d.ReportTokensEstimate))
	}
	return d, nil
}
