package garmin

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const fitAccept = "application/zip, application/x-zip-compressed, application/octet-stream"

// getJSON récupère (avec cache) et décode une réponse JSON.
// Corps vide / 204 → ErrNoData.
func (c *Client) getJSON(ctx context.Context, pathQ, key string, pol policy, persist func([]byte) bool, out any) error {
	b, err := c.cached(ctx, key, pol, func(ctx context.Context) ([]byte, error) {
		return c.get(ctx, pathQ, "application/json", "data")
	}, persist)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return ErrNoData
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("décodage JSON %s : %w", strings.SplitN(pathQ, "?", 2)[0], err)
	}
	return nil
}

// ListQuery : paramètres de la liste d'activités.
type ListQuery struct {
	Start        int
	Limit        int
	StartDate    string // YYYY-MM-DD (optionnel)
	EndDate      string
	ActivityType string
}

// ListActivities : /activitylist-service/activities/search/activities (mémoire 2 min).
func (c *Client) ListActivities(ctx context.Context, q ListQuery) ([]ActivityListItem, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	v := url.Values{}
	v.Set("start", strconv.Itoa(q.Start))
	v.Set("limit", strconv.Itoa(q.Limit))
	if q.StartDate != "" {
		v.Set("startDate", q.StartDate)
	}
	if q.EndDate != "" {
		v.Set("endDate", q.EndDate)
	}
	if q.ActivityType != "" {
		v.Set("activityType", q.ActivityType)
	}
	key := fmt.Sprintf("activities/s%d_l%d_%s_%s_%s", q.Start, q.Limit, safeKey(q.StartDate), safeKey(q.EndDate), safeKey(q.ActivityType))
	var out []ActivityListItem
	err := c.getJSON(ctx, "/activitylist-service/activities/search/activities?"+v.Encode(), key, policy{ttl: TTLActivityList}, nil, &out)
	if errors.Is(err, ErrNoData) {
		return nil, nil
	}
	return out, err
}

// ListActivitiesRange retourne toutes les activités entre deux dates (pagination par 100).
func (c *Client) ListActivitiesRange(ctx context.Context, from, to string) ([]ActivityListItem, error) {
	const page = 100
	var all []ActivityListItem
	for p := 0; p < 10; p++ {
		items, err := c.ListActivities(ctx, ListQuery{Start: p * page, Limit: page, StartDate: from, EndDate: to})
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) < page {
			break
		}
	}
	return all, nil
}

// Activity : /activity-service/activity/{id}. Disque (immuable) si l'activité est
// terminée depuis plus de 2 h (évite de figer un résumé encore en cours de calcul).
func (c *Client) Activity(ctx context.Context, id int64) (*ActivityDetail, error) {
	var out ActivityDetail
	persist := func(b []byte) bool {
		var d ActivityDetail
		if json.Unmarshal(b, &d) != nil {
			return false
		}
		start, ok := d.StartGMT()
		if !ok {
			return false
		}
		dur := 0.0
		if d.Summary.ElapsedDuration != nil {
			dur = *d.Summary.ElapsedDuration
		} else if d.Summary.Duration != nil {
			dur = *d.Summary.Duration
		}
		end := start.Add(time.Duration(dur * float64(time.Second)))
		return c.now().Sub(end) > 2*time.Hour
	}
	err := c.getJSON(ctx, fmt.Sprintf("/activity-service/activity/%d", id), fmt.Sprintf("activity/%d.json", id), policy{disk: true, ttl: TTLWellness}, persist, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ActivityDetails : /activity-service/activity/{id}/details (disque, immuable).
func (c *Client) ActivityDetails(ctx context.Context, id int64, maxChartSize int) (*ActivityDetails, error) {
	if maxChartSize <= 0 {
		maxChartSize = 100000
	}
	var out ActivityDetails
	err := c.getJSON(ctx, fmt.Sprintf("/activity-service/activity/%d/details?maxChartSize=%d", id, maxChartSize),
		fmt.Sprintf("details/%d_%d.json", id, maxChartSize), policy{disk: true, ttl: TTLWellness}, nil, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ActivityFIT télécharge le FIT (zip) avec l'en-tête Accept exact, le décompresse et le met
// en cache disque (DATA_DIR/cache/fit/{id}.fit, immuable). Retourne les octets du .fit.
func (c *Client) ActivityFIT(ctx context.Context, id int64) ([]byte, error) {
	return c.cached(ctx, fmt.Sprintf("fit/%d.fit", id), policy{disk: true, ttl: TTLWellness}, func(ctx context.Context) ([]byte, error) {
		raw, err := c.get(ctx, fmt.Sprintf("/download-service/files/activity/%d", id), fitAccept, "fit")
		if err != nil {
			return nil, err
		}
		return extractFIT(raw)
	}, nil)
}

// extractFIT retourne le premier .fit d'un zip (ou les octets tels quels si c'est déjà un FIT).
func extractFIT(raw []byte) ([]byte, error) {
	if len(raw) >= 12 && string(raw[8:12]) == ".FIT" {
		return raw, nil
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("archive FIT illisible : %w", err)
	}
	var chosen *zip.File
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".fit") {
			chosen = f
			break
		}
	}
	if chosen == nil && len(zr.File) > 0 {
		chosen = zr.File[0]
	}
	if chosen == nil {
		return nil, errors.New("archive FIT vide")
	}
	rc, err := chosen.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxBodyBytes))
}

// DailySleep : /sleep-service/sleep/dailySleep?date=.
func (c *Client) DailySleep(ctx context.Context, date string) (*DailySleep, error) {
	var out DailySleep
	err := c.getJSON(ctx, "/sleep-service/sleep/dailySleep?date="+url.QueryEscape(date), "sleep/"+safeKey(date)+".json", c.wellnessPolicy(date), nil, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SleepRange : /sleep-service/sleep/dailySleepSummariesByDate.
func (c *Client) SleepRange(ctx context.Context, start, end string) ([]DailySleep, error) {
	var out []DailySleep
	err := c.getJSON(ctx, fmt.Sprintf("/sleep-service/sleep/dailySleepSummariesByDate?startDate=%s&endDate=%s", url.QueryEscape(start), url.QueryEscape(end)),
		fmt.Sprintf("sleeprange/%s_%s.json", safeKey(start), safeKey(end)), c.wellnessPolicy(end), nil, &out)
	return out, err
}

// HRVDay : /hrv-service/hrv/{date} (204 → ErrNoData).
func (c *Client) HRVDay(ctx context.Context, date string) (*HRVDay, error) {
	var out HRVDay
	err := c.getJSON(ctx, "/hrv-service/hrv/"+url.PathEscape(date), "hrv/"+safeKey(date)+".json", c.wellnessPolicy(date), nil, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// HRVRange : /hrv-service/hrv/daily/{start}/{end}.
func (c *Client) HRVRange(ctx context.Context, start, end string) ([]HRVSummary, error) {
	var out HRVRange
	err := c.getJSON(ctx, fmt.Sprintf("/hrv-service/hrv/daily/%s/%s", url.PathEscape(start), url.PathEscape(end)),
		fmt.Sprintf("hrvrange/%s_%s.json", safeKey(start), safeKey(end)), c.wellnessPolicy(end), nil, &out)
	return out.HrvSummaries, err
}

// BodyBattery : /wellness-service/wellness/bodyBattery/reports/daily.
func (c *Client) BodyBattery(ctx context.Context, start, end string) ([]BodyBatteryDay, error) {
	var out []BodyBatteryDay
	err := c.getJSON(ctx, fmt.Sprintf("/wellness-service/wellness/bodyBattery/reports/daily?startDate=%s&endDate=%s", url.QueryEscape(start), url.QueryEscape(end)),
		fmt.Sprintf("bodybattery/%s_%s.json", safeKey(start), safeKey(end)), c.wellnessPolicy(end), nil, &out)
	return out, err
}

// DailySummaries : /usersummary-service/v2/dailysummaries.
func (c *Client) DailySummaries(ctx context.Context, start, end string) ([]DailySummary, error) {
	var out DailySummaries
	err := c.getJSON(ctx, fmt.Sprintf("/usersummary-service/v2/dailysummaries?startDate=%s&endDate=%s", url.QueryEscape(start), url.QueryEscape(end)),
		fmt.Sprintf("dailysummaries/%s_%s.json", safeKey(start), safeKey(end)), c.wellnessPolicy(end), nil, &out)
	return out.Data, err
}

// Steps : /usersummary-service/stats/steps/daily/{start}/{end}.
func (c *Client) Steps(ctx context.Context, start, end string) ([]StepsDay, error) {
	var out []StepsDay
	err := c.getJSON(ctx, fmt.Sprintf("/usersummary-service/stats/steps/daily/%s/%s", url.PathEscape(start), url.PathEscape(end)),
		fmt.Sprintf("steps/%s_%s.json", safeKey(start), safeKey(end)), c.wellnessPolicy(end), nil, &out)
	return out, err
}

// DailyHeartRate : /wellness-service/wellness/dailyHeartRate?date=.
func (c *Client) DailyHeartRate(ctx context.Context, date string) (*DailyHeartRate, error) {
	var out DailyHeartRate
	err := c.getJSON(ctx, "/wellness-service/wellness/dailyHeartRate?date="+url.QueryEscape(date), "dailyhr/"+safeKey(date)+".json", c.wellnessPolicy(date), nil, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// HRZones : /biometric-service/heartRateZones/v2 (mémoire 1 h).
func (c *Client) HRZones(ctx context.Context) ([]HRZoneConfig, error) {
	var out []HRZoneConfig
	err := c.getJSON(ctx, "/biometric-service/heartRateZones/v2", "hrzones", policy{ttl: TTLProfile}, nil, &out)
	return out, err
}
