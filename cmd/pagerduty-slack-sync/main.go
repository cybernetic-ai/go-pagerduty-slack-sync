package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kevholditch/go-pagerduty-slack-sync/internal/sync"
	"github.com/sirupsen/logrus"
)

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{FieldMap: logrus.FieldMap{logrus.FieldKeyLevel: "severity", logrus.FieldKeyMsg: "message"}})
	config, err := sync.NewConfigFromEnv()
	if err != nil {
		logrus.Error(err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	for {
		runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err := sync.Schedules(runCtx, config)
		cancel()
		if err != nil {
			logrus.Error(err)
		}
		if config.RunOnce {
			if err != nil {
				os.Exit(1)
			}
			return
		}
		timer := time.NewTimer(time.Duration(config.RunIntervalInSeconds) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
