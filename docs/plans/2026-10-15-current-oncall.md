# Current On-call Cloud Run Implementation Plan

**Goal:** Sync each configured PagerDuty schedule to one Slack group, hourly in internal-tools-511717.
**Architecture:** Keep the Go SDK integrations. Map schedule IDs to explicit Slack handles; fetch display names from PagerDuty. Run immediately once for Cloud Run Jobs. Preserve membership on coverage gaps and API failures; report failures with nonzero exit status.
**Tech Stack:** Go, PagerDuty and Slack SDKs, Cloud Run Jobs, Scheduler, Secret Manager.

Approved design: P37DOZN -> oncall-telephony; display name from PagerDuty. No all-rotation group. Gaps log warnings, never disable groups. Match by handle and use returned immutable group ID for updates. Only cybernetic-ai fork is a publication destination.

1. Replace generated-name configuration in internal/sync/config.go; replace config_test.go with validation cases (missing credentials, malformed mappings, duplicate handles, run-once).
2. Update pagerduty.go to fetch schedule metadata and final rendered schedule at one captured instant, including overrides. Update slack.go to locate by handle, rename display names, and reject missing/inactive accounts before writes.
3. Update schedule_sync.go to sync only current users, preserve gaps, aggregate failures; test with fake clients for handoffs, renames, gaps, errors and multiple mappings.
4. Update cmd/pagerduty-slack-sync/main.go for immediate RUN_ONCE execution and bounded HTTP calls. Run go test ./..., go test -race ./..., go vet ./... and build.
5. Add deploy script and documentation for us-central1, 1 CPU/512 MiB, hourly UTC scheduler, secret-scoped runtime account, scheduler account with invoker access only, limited retries, and no public endpoint.
6. Review diff, build image, deploy, execute once and verify Cloud Run logs and Slack membership. Push branch only to cybernetic-ai fork. Record any blocked validation honestly.

Credentials stay in Secret Manager. App-level Slack token is unused. Bot user-group permission must be verified; if denied request a user OAuth token rather than broadening workspace permissions.
