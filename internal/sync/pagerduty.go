package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PagerDuty/go-pagerduty"
)

type pagerDutyClient struct {
	client  *pagerduty.Client
	token   string
	baseURL string
}

func newPagerDutyClient(token string) *pagerDutyClient {
	c := pagerduty.NewClient(token)
	c.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	return &pagerDutyClient{client: c, token: token, baseURL: "https://api.pagerduty.com"}
}

// Current uses the final schedule (including overrides), filtering at one instant
// so a handoff inside the requested window cannot add the next responder early.
func (p *pagerDutyClient) Current(ctx context.Context, id string, now time.Time) (string, []string, error) {
	schedule, err := p.getSchedule(ctx, id, now)
	if err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(schedule.Name) == "" {
		return "", nil, fmt.Errorf("schedule %s has no name", id)
	}
	var emails []string
	seen := map[string]bool{}
	for _, entry := range schedule.FinalSchedule.RenderedScheduleEntries {
		start, e1 := time.Parse(time.RFC3339, entry.Start)
		end, e2 := time.Parse(time.RFC3339, entry.End)
		if e1 != nil || e2 != nil {
			return "", nil, fmt.Errorf("invalid schedule entry timestamps")
		}
		if now.Before(start) || !now.Before(end) {
			continue
		}
		if seen[entry.User.ID] {
			continue
		}
		if entry.User.ID == "" {
			return "", nil, fmt.Errorf("schedule entry missing user ID")
		}
		seen[entry.User.ID] = true
		user, err := p.client.GetUserWithContext(ctx, entry.User.ID, pagerduty.GetUserOptions{})
		if err != nil {
			return "", nil, err
		}
		if strings.TrimSpace(user.Email) == "" {
			return "", nil, fmt.Errorf("PagerDuty user %s has no email", entry.User.ID)
		}
		emails = append(emails, user.Email)
	}
	return schedule.Name, emails, nil
}

// The SDK does not yet expose shift-based schedules. Try v3 first, falling
// back to the SDK's v2 endpoint only for the documented 400 response.
func (p *pagerDutyClient) getSchedule(ctx context.Context, id string, now time.Time) (*pagerduty.Schedule, error) {
	query := url.Values{"since": {now.UTC().Format(time.RFC3339)}, "until": {now.Add(time.Minute).UTC().Format(time.RFC3339)}, "include[]": {"final_schedule"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/v3/schedules/"+url.PathEscape(id)+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token token="+p.token)
	req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")
	resp, err := p.client.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadRequest {
		return p.client.GetScheduleWithContext(ctx, id, pagerduty.GetScheduleOptions{Since: query.Get("since"), Until: query.Get("until")})
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PagerDuty v3 schedule request returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		Schedule struct {
			Name  string `json:"name"`
			Final *struct {
				Assignments []struct {
					Start  string `json:"start_time"`
					End    string `json:"end_time"`
					Member struct {
						Type   string `json:"type"`
						UserID string `json:"user_id"`
					} `json:"member"`
				} `json:"computed_shift_assignments"`
			} `json:"final_schedule"`
		} `json:"schedule"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return nil, err
	}
	if body.Schedule.Final == nil {
		return nil, fmt.Errorf("PagerDuty response missing final_schedule")
	}
	result := &pagerduty.Schedule{Name: body.Schedule.Name}
	for _, a := range body.Schedule.Final.Assignments {
		if a.Member.Type == "empty_member" {
			continue
		}
		if a.Member.Type != "user_member" {
			return nil, fmt.Errorf("unsupported PagerDuty schedule member type %q", a.Member.Type)
		}
		result.FinalSchedule.RenderedScheduleEntries = append(result.FinalSchedule.RenderedScheduleEntries, pagerduty.RenderedScheduleEntry{Start: a.Start, End: a.End, User: pagerduty.APIObject{ID: a.Member.UserID}})
	}
	return result, nil
}
