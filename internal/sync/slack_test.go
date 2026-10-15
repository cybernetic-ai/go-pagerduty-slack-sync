package sync

import (
	"context"
	"fmt"
	"github.com/slack-go/slack"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSlackRenameAndHandoff(t *testing.T) {
	var actions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		actions = append(actions, r.URL.Path)
		switch r.URL.Path {
		case "/users.lookupByEmail":
			fmt.Fprint(w, `{"ok":true,"user":{"id":"U_NEW"}}`)
		case "/usergroups.list":
			fmt.Fprint(w, `{"ok":true,"usergroups":[{"id":"S1","name":"Old Name","handle":"oncall-telephony"}]}`)
		case "/usergroups.update":
			if r.Form.Get("usergroup") != "S1" || r.Form.Get("name") != "Primary Telephony Oncall" || r.Form.Get("handle") != "" {
				t.Errorf("unexpected rename %v", r.Form)
			}
			fmt.Fprint(w, `{"ok":true,"usergroup":{"id":"S1"}}`)
		case "/usergroups.users.list":
			fmt.Fprint(w, `{"ok":true,"users":["U_OLD"]}`)
		case "/usergroups.users.update":
			if r.Form.Get("usergroup") != "S1" || r.Form.Get("users") != "U_NEW" {
				t.Errorf("unexpected membership %v", r.Form)
			}
			fmt.Fprint(w, `{"ok":true,"usergroup":{"id":"S1"}}`)
		default:
			t.Errorf("unexpected write %s", r.URL.Path)
			fmt.Fprint(w, `{"ok":false,"error":"unexpected"}`)
		}
	}))
	defer server.Close()
	client := &slackClient{client: slack.New("test", slack.OptionAPIURL(server.URL+"/"))}
	if err := client.Sync(context.Background(), "oncall-telephony", "Primary Telephony Oncall", []string{"a@example.com"}); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 5 {
		t.Fatalf("unexpected actions: %v", actions)
	}
}
func TestMissingSlackUserDoesNotWrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "lookupByEmail") {
			t.Errorf("unexpected mutation or lookup: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"ok":false,"error":"users_not_found"}`)
	}))
	defer server.Close()
	client := &slackClient{client: slack.New("test", slack.OptionAPIURL(server.URL+"/"))}
	if err := client.Sync(context.Background(), "oncall-telephony", "Primary", []string{"missing@example.com"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestSlackGroupLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name             string
		groups, user     string
		creates, updates int
		wantErr          bool
	}{
		{"create", `[]`, `{"id":"U_NEW"}`, 1, 1, false},
		{"unchanged", `[{"id":"S1","name":"Primary","handle":"oncall-telephony"}]`, `{"id":"U_NEW"}`, 0, 0, false},
		{"disabled", `[{"id":"S1","name":"Primary","handle":"oncall-telephony","date_delete":1}]`, `{"id":"U_NEW"}`, 0, 0, true},
		{"inactive", `[]`, `{"id":"U_NEW","deleted":true}`, 0, 0, true},
		{"guest", `[]`, `{"id":"U_NEW","is_restricted":true}`, 0, 0, true},
		{"bot", `[]`, `{"id":"U_NEW","is_bot":true}`, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			creates, updates := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/users.lookupByEmail":
					fmt.Fprintf(w, `{"ok":true,"user":%s}`, tc.user)
				case "/usergroups.list":
					fmt.Fprintf(w, `{"ok":true,"usergroups":%s}`, tc.groups)
				case "/usergroups.create":
					creates++
					r.ParseForm()
					if r.Form.Get("name") != "Primary" || r.Form.Get("handle") != "oncall-telephony" {
						t.Errorf("wrong group: %v", r.Form)
					}
					fmt.Fprint(w, `{"ok":true,"usergroup":{"id":"S1","name":"Primary"}}`)
				case "/usergroups.users.list":
					if creates > 0 {
						fmt.Fprint(w, `{"ok":true,"users":[]}`)
					} else {
						fmt.Fprint(w, `{"ok":true,"users":["U_NEW"]}`)
					}
				case "/usergroups.users.update":
					updates++
					fmt.Fprint(w, `{"ok":true,"usergroup":{"id":"S1"}}`)
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					fmt.Fprint(w, `{"ok":false}`)
				}
			}))
			defer server.Close()
			client := &slackClient{client: slack.New("test", slack.OptionAPIURL(server.URL+"/"))}
			err := client.Sync(context.Background(), "oncall-telephony", "Primary", []string{"a@example.com"})
			if (err != nil) != tc.wantErr || creates != tc.creates || updates != tc.updates {
				t.Fatalf("err=%v creates=%d updates=%d", err, creates, updates)
			}
		})
	}
}
