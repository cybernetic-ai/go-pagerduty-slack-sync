package sync

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	Schedules            []Schedule
	PagerDutyToken       string
	SlackToken           string
	RunIntervalInSeconds int
	RunOnce              bool
}

type Schedule struct {
	ID     string
	Handle string
}

func NewConfigFromEnv() (*Config, error) {
	c := &Config{PagerDutyToken: os.Getenv("PAGERDUTY_TOKEN"), SlackToken: os.Getenv("SLACK_TOKEN"), RunIntervalInSeconds: 60}
	if c.PagerDutyToken == "" || c.SlackToken == "" {
		return nil, fmt.Errorf("PAGERDUTY_TOKEN and SLACK_TOKEN are required")
	}
	if strings.HasPrefix(c.SlackToken, "xapp-") {
		return nil, fmt.Errorf("SLACK_TOKEN must be a bot or user OAuth token, not an app-level token")
	}
	if v, ok := os.LookupEnv("RUN_ONCE"); ok {
		var err error
		c.RunOnce, err = strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("RUN_ONCE must be true or false")
		}
	}
	if v, ok := os.LookupEnv("RUN_INTERVAL_SECONDS"); ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 86400 {
			return nil, fmt.Errorf("RUN_INTERVAL_SECONDS must be between 1 and 86400")
		}
		c.RunIntervalInSeconds = n
	}
	seen := map[string]bool{}
	env := os.Environ()
	sort.Strings(env)
	validHandle := regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "SCHEDULE_") {
			continue
		}
		fields := strings.Split(value, ",")
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s must contain schedule-id,slack-handle", key)
		}
		id, handle := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1])
		if id == "" || len(handle) > 80 || !validHandle.MatchString(handle) {
			return nil, fmt.Errorf("invalid schedule ID or Slack handle in %s", key)
		}
		if seen[handle] {
			return nil, fmt.Errorf("duplicate Slack handle %s", handle)
		}
		seen[handle] = true
		c.Schedules = append(c.Schedules, Schedule{ID: id, Handle: handle})
	}
	if len(c.Schedules) == 0 {
		return nil, fmt.Errorf("at least one SCHEDULE_ mapping is required")
	}
	return c, nil
}
