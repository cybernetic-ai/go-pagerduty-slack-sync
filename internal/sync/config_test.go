package sync

import (
	"os"
	"strings"
	"testing"
)

func cleanConfigEnv(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "SCHEDULE_") || key == "RUN_ONCE" || key == "RUN_INTERVAL_SECONDS" {
			old := os.Getenv(key)
			os.Unsetenv(key)
			t.Cleanup(func() { os.Setenv(key, old) })
		}
	}
	t.Setenv("PAGERDUTY_TOKEN", "test-pd")
	t.Setenv("SLACK_TOKEN", "xoxb-test")
	t.Setenv("SCHEDULE_TELEPHONY", "P37DOZN,oncall-telephony")
}

func TestConfig(t *testing.T) {
	cleanConfigEnv(t)
	t.Setenv("RUN_ONCE", "true")
	c, err := NewConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !c.RunOnce || c.Schedules[0] != (Schedule{ID: "P37DOZN", Handle: "oncall-telephony"}) {
		t.Fatalf("unexpected config: %+v", c.Schedules)
	}
}
func TestInvalidConfig(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"SCHEDULE_TELEPHONY", "P37DOZN"}, {"SCHEDULE_TELEPHONY", ",oncall"}, {"SCHEDULE_TELEPHONY", "P37DOZN,@oncall"},
		{"SCHEDULE_DUPLICATE", "P123,oncall-telephony"}, {"RUN_ONCE", "maybe"}, {"RUN_INTERVAL_SECONDS", "0"},
		{"PAGERDUTY_TOKEN", ""}, {"SLACK_TOKEN", "xapp-test"},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			cleanConfigEnv(t)
			t.Setenv(tc.key, tc.value)
			if _, err := NewConfigFromEnv(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
