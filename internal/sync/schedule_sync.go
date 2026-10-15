package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

type scheduleSource interface {
	Current(context.Context, string, time.Time) (string, []string, error)
}
type groupTarget interface {
	Sync(context.Context, string, string, []string) error
}

func Schedules(ctx context.Context, config *Config) error {
	return syncSchedules(ctx, config.Schedules, newPagerDutyClient(config.PagerDutyToken), newSlackClient(config.SlackToken), time.Now().UTC())
}
func syncSchedules(ctx context.Context, schedules []Schedule, source scheduleSource, target groupTarget, now time.Time) error {
	var failures []error
	for _, mapping := range schedules {
		name, emails, err := source.Current(ctx, mapping.ID, now)
		if err != nil {
			failures = append(failures, fmt.Errorf("schedule %s: %w", mapping.ID, err))
			continue
		}
		if len(emails) == 0 {
			logrus.WithFields(logrus.Fields{"schedule": mapping.ID, "handle": mapping.Handle}).Warn("coverage gap: leaving existing group and membership unchanged")
			continue
		}
		if err := target.Sync(ctx, mapping.Handle, name, emails); err != nil {
			failures = append(failures, fmt.Errorf("group @%s: %w", mapping.Handle, err))
			continue
		}
		logrus.WithFields(logrus.Fields{"schedule": mapping.ID, "handle": mapping.Handle, "oncall_count": len(emails)}).Info("sync completed")
	}
	return errors.Join(failures...)
}
