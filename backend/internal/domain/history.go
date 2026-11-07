package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"stride/internal/analysis"
	"stride/internal/apimeter"
	"stride/internal/queue"
)

const (
	historyWindowDays = 180
	historyFloor      = "2008-01-01" // Garmin Connect opened in 2008
	historyEmptyStop  = 3            // consecutive empty windows ending a full-history listing
	stravaHistoryMax  = 50           // pages of 100 summaries
)

type historyPayload struct {
	From string `json:"from"`
}

type HistorySyncInput struct {
	From *string `json:"from"`
}

// SyncHistory enqueues the backwards listing of the provider history (summaries only) down to
// from, or everything the providers return when from is null. Progress is reported on the job.
func (s *Services) SyncHistory(ctx context.Context, uid int64, in HistorySyncInput) (*queue.JobStatus, error) {
	from := ""
	if in.From != nil {
		if !isValidDate(*in.From) {
			return nil, badRequest("from attendu au format AAAA-MM-JJ (ou null pour tout l'historique).")
		}
		from = *in.From
	}
	g, gerr := s.getConn(ctx, uid, pGarmin)
	st, serr := s.getConn(ctx, uid, pStrava)
	if (gerr != nil || !g.connected()) && (serr != nil || !st.connected()) {
		return nil, ErrNotConnected
	}
	return s.queue.Enqueue(ctx, queue.EnqueueRequest{UserID: uid, Kind: queue.KindSyncHistory, Queue: queue.QueueLow,
		Label: "Import de l'historique complet", MaxAttempts: 5, Payload: historyPayload{From: from}})
}

// historyWindows splits [from, end] into windows of historyWindowDays, newest first.
func historyWindows(from, end string) [][2]string {
	var out [][2]string
	for to := end; to >= from; to = analysis.DateAdd(to, -historyWindowDays) {
		out = append(out, [2]string{max(from, analysis.DateAdd(to, -historyWindowDays+1)), to})
	}
	return out
}

func (s *Services) runHistory(ctx context.Context, jc *queue.JobContext) (any, error) {
	var p historyPayload
	if err := json.Unmarshal(jc.Payload, &p); err != nil {
		return nil, jobErr(err, false)
	}
	uid := jc.UserID
	if !s.setRunning(uid, true, jc.ID) {
		return nil, queue.Transient("Une synchronisation est en cours, nouvelle tentative plus tard.", errSyncRunning)
	}
	defer s.setRunning(uid, false, "")
	ctx = apimeter.WithUser(ctx, uid)
	prof, err := s.profile(ctx, uid)
	if err != nil {
		return nil, jobErr(err, true)
	}
	today := s.today(prof)
	from := cmpOrStr(p.From, historyFloor)
	res := map[string]int{"garmin": 0, "strava": 0}
	if cn, err := s.getConn(ctx, uid, pGarmin); err == nil && cn.connected() {
		end := today
		if cn.HistoryFrom.Valid {
			end = analysis.DateAdd(cn.HistoryFrom.String, -1)
		}
		windows := historyWindows(from, end)
		empty := 0
		for i, w := range windows {
			done, total := i, len(windows)
			jc.UpdateProgress(fmt.Sprintf("Historique Garmin : période %d/%d", i+1, total), &done, &total, nil)
			listed, created, err := s.historyGarminWindow(ctx, uid, w[0], w[1])
			if err != nil {
				return nil, historyErr(err)
			}
			res["garmin"] += created
			if listed == 0 {
				empty++
			} else {
				empty = 0
			}
			if p.From == "" && empty >= historyEmptyStop {
				break
			}
		}
	}
	if cn, err := s.getConn(ctx, uid, pStrava); err == nil && cn.connected() && s.stravaAvailable() == nil &&
		(!cn.Scope.Valid || hasScope(cn.Scope.String, "activity:read")) {
		jc.UpdateProgress("Historique Strava : import des résumés…", nil, nil, nil)
		n, err := s.historyStrava(ctx, uid, cn, from)
		if err != nil {
			return nil, historyErr(err)
		}
		res["strava"] = n
	}
	if _, err := s.records(ctx, uid); err != nil {
		s.log.Warn("⚠️ Calcul des records personnels en échec", "user", uid, "err", errText(err))
	}
	s.log.Info("📚 Historique importé", "user", uid, "garmin", res["garmin"], "strava", res["strava"])
	return res, nil
}

// historyErr retries rate limits (429 / meter cooldown) and outages: the listing resumes from history_from.
func historyErr(err error) error {
	var pe *ProviderError
	return jobErr(err, errors.As(err, &pe) && !pe.Unauthorized && (pe.RateLimited || pe.Timeout || pe.Status == 0 || pe.Status >= 500))
}

// historyGarminWindow stores the summaries of one window and records it as covered.
func (s *Services) historyGarminWindow(ctx context.Context, uid int64, from, to string) (listed, created int, err error) {
	g, err := s.garminClient(ctx, uid)
	if err != nil {
		return 0, 0, err
	}
	items, err := g.ListActivitiesRange(ctx, from, to)
	if err != nil {
		return 0, 0, providerErr(pGarmin, err)
	}
	for _, raw := range items {
		_, isNew, err := s.upsertGarmin(ctx, uid, raw)
		if err != nil {
			return len(items), created, err
		}
		if isNew {
			created++
		}
	}
	_, err = s.db().ExecContext(ctx, `UPDATE provider_connections SET history_from = ? WHERE user_id = ? AND provider = 'garmin'
		AND (history_from IS NULL OR history_from > ?)`, from, uid, from)
	return len(items), created, err
}

// historyStrava lists the Strava summaries older than the covered history, down to from.
func (s *Services) historyStrava(ctx context.Context, uid int64, cn *conn, from string) (int, error) {
	after, _ := time.Parse("2006-01-02", from)
	before := s.now()
	if cn.HistoryFrom.Valid {
		if t, err := time.Parse("2006-01-02", cn.HistoryFrom.String); err == nil {
			before = t
		}
	}
	n := 0
	err := s.withStrava(ctx, uid, func(tok string) error {
		for page := 1; page <= stravaHistoryMax; page++ {
			acts, err := s.deps.Strava.ListActivities(ctx, tok, after, before, page, 100)
			if err != nil {
				return err
			}
			for i := range acts {
				if acts[i].Manual {
					continue
				}
				if _, created, err := s.upsertStrava(ctx, uid, &acts[i], nil, true); err != nil {
					return err
				} else if created {
					n++
				}
			}
			if len(acts) < 100 {
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return n, providerErr(pStrava, err)
	}
	_, err = s.db().ExecContext(ctx, `UPDATE provider_connections SET history_from = ? WHERE user_id = ? AND provider = 'strava'
		AND (history_from IS NULL OR history_from > ?)`, from, uid, from)
	return n, err
}

func cmpOrStr(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// ---------- search ----------

type ActivitySearchHit struct {
	ID         int64    `json:"id"`
	Date       string   `json:"date"`
	StartLocal string   `json:"startLocal"`
	Name       string   `json:"name"`
	Sport      string   `json:"sport"`
	TypeLabel  string   `json:"typeLabel"`
	DistanceM  *float64 `json:"distanceM"`
	DurationS  *float64 `json:"durationS"`
}

type SearchResult struct {
	Query string              `json:"query"`
	Items []ActivitySearchHit `json:"items"`
}

var frMonths = map[string]string{"janvier": "01", "fevrier": "02", "février": "02", "mars": "03", "avril": "04", "mai": "05", "juin": "06",
	"juillet": "07", "aout": "08", "août": "08", "septembre": "09", "octobre": "10", "novembre": "11", "decembre": "12", "décembre": "12"}

var (
	isoDateRe = regexp.MustCompile(`^\d{4}(-\d{2}(-\d{2})?)?$`)
	numDateRe = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})(?:/(\d{4}|\d{2}))?$`)
	frDateRe  = regexp.MustCompile(`^(?:(\d{1,2})(?:er)?\s+)?([a-zéû]+)(?:\s+(\d{4}))?$`)
)

// dateFilter turns « 2025-04-14 », « 14/04 », « 14 avril », « avril 2025 »… into a local_date condition.
func dateFilter(q string) (string, []any) {
	part := func(day, month, year string) (string, []any) {
		var conds []string
		var args []any
		if month != "" {
			conds, args = append(conds, `substr(local_date, 6, 2) = ?`), append(args, month)
		}
		if day != "" {
			d, _ := strconv.Atoi(day)
			conds, args = append(conds, `substr(local_date, 9, 2) = ?`), append(args, fmt.Sprintf("%02d", d))
		}
		if year != "" {
			conds, args = append(conds, `substr(local_date, 1, 4) = ?`), append(args, year)
		}
		return strings.Join(conds, " AND "), args
	}
	switch {
	case isoDateRe.MatchString(q):
		return `local_date LIKE ?`, []any{q + "%"}
	case numDateRe.MatchString(q):
		m := numDateRe.FindStringSubmatch(q)
		mo, _ := strconv.Atoi(m[2])
		if mo < 1 || mo > 12 {
			return "", nil
		}
		year := m[3]
		if len(year) == 2 {
			year = "20" + year
		}
		return part(m[1], fmt.Sprintf("%02d", mo), year)
	case frDateRe.MatchString(q):
		m := frDateRe.FindStringSubmatch(q)
		if mo, ok := frMonths[m[2]]; ok {
			return part(m[1], mo, m[3])
		}
	}
	return "", nil
}

// Search matches activity names and dates over the whole stored history, newest first.
func (s *Services) Search(ctx context.Context, uid int64, q string, limit int) (*SearchResult, error) {
	q = strings.Join(strings.Fields(strings.ToLower(q)), " ")
	if q == "" || len([]rune(q)) > 100 {
		return nil, badRequest("q requis (100 caractères max).")
	}
	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 50)
	like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
	where, args := `name LIKE ? ESCAPE '\'`, []any{uid, like}
	if cond, dargs := dateFilter(q); cond != "" {
		where = "(" + where + " OR (" + cond + "))"
		args = append(args, dargs...)
	}
	rows, err := s.queryActivities(ctx, `WHERE user_id = ? AND `+where+` ORDER BY start_local DESC, id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	out := &SearchResult{Query: q, Items: make([]ActivitySearchHit, 0, len(rows))}
	for _, a := range rows {
		out.Items = append(out.Items, ActivitySearchHit{ID: a.ID, Date: a.LocalDate, StartLocal: a.StartLocal, Name: a.Name, Sport: a.Sport,
			TypeLabel: analysis.TypeLabel(a.Type), DistanceM: a.DistanceM, DurationS: a.DurationS})
	}
	return out, nil
}
