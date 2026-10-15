package sync

import (
	"context"
	"fmt"
	"github.com/PagerDuty/go-pagerduty"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFinalScheduleAtHandoff(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v3/schedules/PD1":
			http.Error(w, "legacy schedule", 400)
		case "/schedules/PD1":
			fmt.Fprint(w, `{"schedule":{"name":"Primary Telephony Oncall","schedule_layers":[{"users":[{"user":{"id":"REGULAR"}}]}],"final_schedule":{"rendered_schedule_entries":[{"start":"2026-10-15T11:00:00Z","end":"2026-10-15T12:00:00Z","user":{"id":"OLD"}},{"start":"2026-10-15T12:00:00Z","end":"2026-10-15T12:00:30Z","user":{"id":"OVERRIDE"}},{"start":"2026-10-15T12:00:30Z","end":"2026-10-15T13:00:00Z","user":{"id":"NEXT"}}]}}}`)
		case "/users/OVERRIDE":
			calls++
			fmt.Fprint(w, `{"user":{"id":"OVERRIDE","email":"override@example.com"}}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.Error(w, "unexpected", 500)
		}
	}))
	defer server.Close()
	client := &pagerDutyClient{baseURL: server.URL, token: "test", client: pagerduty.NewClient("test", pagerduty.WithAPIEndpoint(server.URL))}
	name, emails, err := client.Current(context.Background(), "PD1", now)
	if err != nil || name != "Primary Telephony Oncall" || len(emails) != 1 || emails[0] != "override@example.com" || calls != 1 {
		t.Fatalf("name=%s emails=%v err=%v", name, emails, err)
	}
}

func TestShiftBasedSchedule(t *testing.T) {
	for _, tc := range []struct {
		name, member string
		count        int
		wantErr      bool
	}{
		{"override", `{"type":"user_member","user_id":"OVERRIDE"}`, 1, false},
		{"gap", `{"type":"empty_member"}`, 0, false},
		{"unknown member", `{"type":"new_member_type"}`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v3/schedules/PD1":
					if r.URL.Query().Get("include[]") != "final_schedule" {
						t.Error("missing final schedule include")
					}
					fmt.Fprintf(w, `{"schedule":{"name":"Primary Telephony Oncall","final_schedule":{"computed_shift_assignments":[{"start_time":"2026-10-15T12:00:00Z","end_time":"2026-10-15T13:00:00Z","member":%s,"source":{"type":"schedule_rotation_override"}}]}}}`, tc.member)
				case "/users/OVERRIDE":
					fmt.Fprint(w, `{"user":{"email":"override@example.com"}}`)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					http.Error(w, "unexpected", 500)
				}
			}))
			defer server.Close()
			client := &pagerDutyClient{baseURL: server.URL, token: "test", client: pagerduty.NewClient("test", pagerduty.WithAPIEndpoint(server.URL))}
			_, emails, err := client.Current(context.Background(), "PD1", time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC))
			if (err != nil) != tc.wantErr || len(emails) != tc.count {
				t.Fatalf("emails=%v err=%v", emails, err)
			}
		})
	}
}

func TestInvalidV3ResponseFails(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"unavailable", `{}`, 503}, {"unauthorized", `{}`, 401},
		{"malformed", `not json`, 200}, {"missing final schedule", `{"schedule":{"name":"Primary"}}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			client := &pagerDutyClient{baseURL: server.URL, token: "test", client: pagerduty.NewClient("test", pagerduty.WithAPIEndpoint(server.URL))}
			if _, _, err := client.Current(context.Background(), "PD1", time.Now()); err == nil {
				t.Fatal("expected API error, not a coverage gap")
			}
		})
	}
}
