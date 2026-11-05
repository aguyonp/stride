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
)

// Endpoint templates (no ids, no dates) used for metering and logs.
const (
	EPActivityList   = "/activitylist-service/activities/search/activities"
	EPActivity       = "/activity-service/activity/{id}"
	EPDetails        = "/activity-service/activity/{id}/details"
	EPFIT            = "/download-service/files/activity/{id}"
	EPSleepRange     = "/sleep-service/sleep/dailySleepSummariesByDate"
	EPHRVDay         = "/hrv-service/hrv/{date}"
	EPHRVRange       = "/hrv-service/hrv/daily/{start}/{end}"
	EPDailySummaries = "/usersummary-service/v2/dailysummaries"
	EPUserSummary    = "/usersummary-service/usersummary/daily"
	EPHRZones        = "/biometric-service/heartRateZones/v2"
)

const jsonAccept = "application/json"

type ListQuery struct {
	Start     int
	Limit     int
	StartDate string
	EndDate   string
}

// ListActivities returns the raw JSON array of the activity search.
func (c *Client) ListActivities(ctx context.Context, q ListQuery) ([]byte, error) {
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
	b, err := c.get(ctx, "list", EPActivityList, EPActivityList+"?"+v.Encode(), jsonAccept)
	if errors.Is(err, ErrNoData) {
		return []byte("[]"), nil
	}
	return b, err
}

// ListActivitiesRange pages through the search (100 per page) between two local dates.
func (c *Client) ListActivitiesRange(ctx context.Context, from, to string) ([]json.RawMessage, error) {
	const page = 100
	var all []json.RawMessage
	for p := 0; p < 20; p++ {
		b, err := c.ListActivities(ctx, ListQuery{Start: p * page, Limit: page, StartDate: from, EndDate: to})
		if err != nil {
			return nil, err
		}
		var items []json.RawMessage
		if err := json.Unmarshal(b, &items); err != nil {
			return nil, fmt.Errorf("garmin: decode activity list: %w", err)
		}
		all = append(all, items...)
		if len(items) < page {
			break
		}
	}
	return all, nil
}

func (c *Client) Activity(ctx context.Context, id int64) ([]byte, error) {
	return c.get(ctx, "activity", EPActivity, fmt.Sprintf("/activity-service/activity/%d", id), jsonAccept)
}

func (c *Client) ActivityDetails(ctx context.Context, id int64) ([]byte, error) {
	return c.get(ctx, "details", EPDetails, fmt.Sprintf("/activity-service/activity/%d/details?maxChartSize=100000", id), jsonAccept)
}

// ActivityFIT downloads the original file (zip) with the exact Accept header Garmin requires
// and returns the extracted .fit bytes.
func (c *Client) ActivityFIT(ctx context.Context, id int64) ([]byte, error) {
	raw, err := c.get(ctx, "fit", EPFIT, fmt.Sprintf("/download-service/files/activity/%d", id), fitAccept)
	if err != nil {
		return nil, err
	}
	return extractFIT(raw)
}

func extractFIT(raw []byte) ([]byte, error) {
	if len(raw) >= 12 && string(raw[8:12]) == ".FIT" {
		return raw, nil
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("garmin: unreadable FIT archive: %w", err)
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
		return nil, errors.New("garmin: empty FIT archive")
	}
	rc, err := chosen.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxBodyBytes))
}

func (c *Client) SleepRange(ctx context.Context, start, end string) ([]byte, error) {
	return c.get(ctx, "sleep", EPSleepRange, fmt.Sprintf("%s?startDate=%s&endDate=%s", EPSleepRange, url.QueryEscape(start), url.QueryEscape(end)), jsonAccept)
}

func (c *Client) HRVDay(ctx context.Context, date string) ([]byte, error) {
	return c.get(ctx, "hrv", EPHRVDay, "/hrv-service/hrv/"+url.PathEscape(date), jsonAccept)
}

func (c *Client) HRVRange(ctx context.Context, start, end string) ([]byte, error) {
	return c.get(ctx, "hrv", EPHRVRange, fmt.Sprintf("/hrv-service/hrv/daily/%s/%s", url.PathEscape(start), url.PathEscape(end)), jsonAccept)
}

func (c *Client) DailySummaries(ctx context.Context, start, end string) ([]byte, error) {
	return c.get(ctx, "summaries", EPDailySummaries, fmt.Sprintf("%s?startDate=%s&endDate=%s", EPDailySummaries, url.QueryEscape(start), url.QueryEscape(end)), jsonAccept)
}

func (c *Client) UserSummaryDaily(ctx context.Context, date string) ([]byte, error) {
	return c.get(ctx, "usersummary", EPUserSummary, EPUserSummary+"?calendarDate="+url.QueryEscape(date), jsonAccept)
}

func (c *Client) HRZones(ctx context.Context) ([]byte, error) {
	return c.get(ctx, "hrzones", EPHRZones, EPHRZones, jsonAccept)
}

// CreateWorkout creates a Garmin Connect workout (JSON payload) and returns the created workout.
func (c *Client) CreateWorkout(ctx context.Context, payload []byte) ([]byte, error) {
	return c.post(ctx, "create workout", "/workout-service/workout", "/workout-service/workout", payload)
}

// ScheduleWorkout puts a workout on the Garmin Connect calendar at date (YYYY-MM-DD).
func (c *Client) ScheduleWorkout(ctx context.Context, workoutID int64, date string) ([]byte, error) {
	body, _ := json.Marshal(map[string]string{"date": date})
	return c.post(ctx, "schedule workout", "/workout-service/schedule/{id}", fmt.Sprintf("/workout-service/schedule/%d", workoutID), body)
}
