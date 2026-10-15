package sync

import (
	"context"
	"errors"
	"testing"
	"time"
)

type sourceResult struct {
	name   string
	emails []string
	err    error
}
type fakeSource map[string]sourceResult

func (s fakeSource) Current(_ context.Context, id string, _ time.Time) (string, []string, error) {
	r := s[id]
	return r.name, r.emails, r.err
}

type targetCall struct {
	handle, name string
	emails       []string
}
type fakeTarget struct {
	calls []targetCall
	err   error
}

func (s *fakeTarget) Sync(_ context.Context, handle, name string, emails []string) error {
	s.calls = append(s.calls, targetCall{handle, name, emails})
	return s.err
}
func TestSyncSchedules(t *testing.T) {
	for _, tc := range []struct {
		name      string
		source    sourceResult
		targetErr error
		calls     int
		wantErr   bool
	}{
		{"current", sourceResult{name: "Primary Telephony Oncall", emails: []string{"oncall@example.com"}}, nil, 1, false},
		{"gap", sourceResult{name: "Primary Telephony Oncall"}, nil, 0, false},
		{"source failure", sourceResult{err: errors.New("source unavailable")}, nil, 0, true},
		{"target failure", sourceResult{name: "Primary", emails: []string{"a@example.com"}}, errors.New("permission_denied"), 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := &fakeTarget{err: tc.targetErr}
			err := syncSchedules(context.Background(), []Schedule{{"PD1", "oncall-telephony"}}, fakeSource{"PD1": tc.source}, target, time.Now())
			if (err != nil) != tc.wantErr || len(target.calls) != tc.calls {
				t.Fatalf("err=%v calls=%v", err, target.calls)
			}
			if tc.calls > 0 && (target.calls[0].handle != "oncall-telephony" || target.calls[0].name != tc.source.name) {
				t.Fatal("mapping was not preserved")
			}
		})
	}
}
func TestSyncContinuesAfterFailure(t *testing.T) {
	target := &fakeTarget{}
	err := syncSchedules(context.Background(), []Schedule{{"bad", "bad"}, {"good", "good"}}, fakeSource{"bad": {err: errors.New("failed")}, "good": {name: "Good", emails: []string{"a@example.com"}}}, target, time.Now())
	if err == nil || len(target.calls) != 1 || target.calls[0].handle != "good" {
		t.Fatalf("err=%v calls=%v", err, target.calls)
	}
}
