# PagerDuty → Slack current-on-call sync

Cybernetic fork: https://github.com/cybernetic-ai/go-pagerduty-slack-sync

Each PagerDuty schedule maps to one Slack user group. The schedule's name becomes
the group's display name. The explicitly configured Slack handle stays stable.
Only currently on-call users are members; no all-rotation group is maintained.
Supports shift-based v3 schedules and legacy v2 schedules, including overrides.

```dotenv
SCHEDULE_TELEPHONY=P37DOZN,oncall-telephony
RUN_ONCE=true
PAGERDUTY_TOKEN=<read-only REST API key>
SLACK_TOKEN=<bot or user OAuth token>
```

Add more `SCHEDULE_` variables with unique handles for other schedules. Handles
omit `@`. Existing groups are matched by handle; missing groups are created.
If a schedule is renamed, its Slack display name follows on the next sync.

## Behavior

- Run immediately once with `RUN_ONCE=true`; exit nonzero on any sync failure.
- Otherwise repeat with `RUN_INTERVAL_SECONDS` (default 60; range 1–86400).
- Read final schedule assignments at one instant, including concurrent responders.
- Match PagerDuty email to an active Slack member. Resolve every user before writes.
- Replace membership only when it differs. Failures preserve the previous membership
  where possible; a display-name change or group creation may precede a failed write.
- On confirmed coverage gaps, log a warning and leave the group enabled and its
  membership unchanged. This deliberately leaves the previous responder tagged.
- Never enable or disable groups. A manually disabled target produces an error.
- Each run has a two-minute deadline; individual HTTP calls have a 20-second timeout.

## Slack permissions

OAuth scopes: `usergroups:read`, `usergroups:write`, `users:read`, `users:read.email`.
A bot token works only when the workspace permits it to manage user groups.
For admin-only group management, use an authorized user's OAuth token instead.
An app-level `xapp-` token is not used. No Slack events, public endpoint, or message
posting permission is required.

## Development

Requires Go 1.27.2 (matches the container build).

```sh
go test -race ./...
go vet ./...
go build ./cmd/pagerduty-slack-sync
```

Tests use local mock HTTP servers; they do not access real secrets or modify Slack.
CI validates code only; it does not publish to the original author's Docker Hub.

## Cloud deployment

Project: `internal-tools-511717`; region: `us-central1`.
Job: `pd-slack-sync`; Scheduler: `pd-slack-sync` at `0 * * * *` UTC (hourly).
Runtime: 1 vCPU, 512 MiB, one task, one retry, three-minute task timeout.
No VPC connector, public endpoint, or always-running service is required.

Runtime secrets in Secret Manager:
- `pd-slack-sync-pd-key`
- `pd-slack-sync-slack-bot-token`

`pd-slack-sync-slack-app-token` is unused. Secret values are never stored in Git
or container images. The runtime account has accessor permission only on those
two secrets. Scheduler has `roles/run.invoker` only on this job.

One-time prerequisites (provisioned for this project): enable Run, Scheduler,
Artifact Registry, Cloud Build, IAM, and Secret Manager APIs; create the
`internal-tools` Docker repository, the `internal-tools-511717-pdsync-builds`
source bucket, and `pd-slack-sync`, `pd-slack-sync-scheduler`, and
`pd-slack-sync-builder` service accounts. The builder needs Artifact Registry
Writer on this repository, Object Viewer on the source bucket, and Logs Writer
on the project. Deployers need permission to build, deploy jobs, manage Scheduler,
and act as these service accounts.

From a committed checkout, run `./deploy/deploy.sh`. It builds and tests in Cloud
Build, deploys an immutable image digest, executes the job successfully, then
creates or updates its hourly schedule. If initial execution fails, fix the error
before enabling scheduling. Existing schedules are paused during deployment and
resumed only after successful verification. If deployment fails, fix the issue
and rerun the deployment script; the schedule stays paused until verification passes.

```sh
gcloud run jobs executions list --job=pd-slack-sync --region=us-central1 --project=internal-tools-511717
gcloud run jobs execute pd-slack-sync --region=us-central1 --project=internal-tools-511717 --wait
gcloud scheduler jobs pause pd-slack-sync --location=us-central1 --project=internal-tools-511717
```

Check Cloud Run execution failures and `severity=ERROR` logs. A gap emits a warning
but exits successfully, by design. An hourly poll can lag an override by about an
hour plus startup time. Monitoring notification channels must be configured to
receive failure notifications; logging alone does not send alerts.

## Cost

At 720 monthly executions under one minute, 1 vCPU and 512 MiB in us-central1,
compute is approximately $0.82/month before free-tier allowances; Scheduler is
$0.10/month unless covered by its free tier. Builds, image/source storage, secrets,
and logs are additional. Retries and longer runs increase compute costs.

## Upstream

Derived from kevholditch/go-pagerduty-slack-sync under the MIT license. Changes
are maintained in cybernetic-ai's fork; no upstream pull request is required.
