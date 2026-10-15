#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
PROJECT=internal-tools-511717
REGION=us-central1
JOB=pd-slack-sync
RUNTIME="pd-slack-sync@${PROJECT}.iam.gserviceaccount.com"
SCHEDULER="pd-slack-sync-scheduler@${PROJECT}.iam.gserviceaccount.com"
BUILDER="pd-slack-sync-builder@${PROJECT}.iam.gserviceaccount.com"
REVISION=$(git rev-parse --short=12 HEAD)
if [[ -n $(git status --porcelain) ]]; then
  echo 'Commit changes before deploying so the image tag identifies its source.' >&2
  exit 1
fi
IMAGE="${REGION}-docker.pkg.dev/${PROJECT}/internal-tools/pd-slack-sync:${REVISION}"
gcloud builds submit . --project="$PROJECT" --region="$REGION" \
  --config=deploy/cloudbuild.yaml --substitutions="_IMAGE=${IMAGE}" \
  --service-account="projects/${PROJECT}/serviceAccounts/${BUILDER}" \
  --gcs-source-staging-dir="gs://${PROJECT}-pdsync-builds/source"
# Resolve the tag to an immutable image digest.
DIGEST=$(gcloud artifacts docker images describe "$IMAGE" --project="$PROJECT" --format='value(image_summary.digest)')
IMAGE="${REGION}-docker.pkg.dev/${PROJECT}/internal-tools/pd-slack-sync@${DIGEST}"
# Stop recurring execution while replacing and verifying an existing job.
SCHEDULE_ACTION=create
if STATE=$(gcloud scheduler jobs describe "$JOB" --project="$PROJECT" --location="$REGION" --format='value(state)' 2>/dev/null); then
  SCHEDULE_ACTION=update
  case "$STATE" in
    ENABLED) gcloud scheduler jobs pause "$JOB" --project="$PROJECT" --location="$REGION" ;;
    PAUSED) ;; # A previous failed deployment may already have paused it.
    *) echo "Unexpected Scheduler state: $STATE" >&2; exit 1 ;;
  esac
fi
gcloud run jobs deploy "$JOB" --project="$PROJECT" --region="$REGION" \
  --image="$IMAGE" --service-account="$RUNTIME" \
  --cpu=1 --memory=512Mi --tasks=1 --parallelism=1 --max-retries=1 --task-timeout=180s \
  --set-env-vars='^|^RUN_ONCE=true|SCHEDULE_TELEPHONY=P37DOZN,oncall-telephony' \
  --set-secrets='PAGERDUTY_TOKEN=pd-slack-sync-pd-key:latest,SLACK_TOKEN=pd-slack-sync-slack-bot-token:latest'
gcloud run jobs add-iam-policy-binding "$JOB" --project="$PROJECT" --region="$REGION" \
  --member="serviceAccount:${SCHEDULER}" --role=roles/run.invoker
# Prove the runtime can read its secrets and sync before enabling scheduled work.
gcloud run jobs execute "$JOB" --project="$PROJECT" --region="$REGION" --wait
gcloud scheduler jobs "$SCHEDULE_ACTION" http "$JOB" --project="$PROJECT" --location="$REGION" \
  --schedule='0 * * * *' --time-zone=Etc/UTC \
  --uri="https://run.googleapis.com/v2/projects/${PROJECT}/locations/${REGION}/jobs/${JOB}:run" \
  --http-method=POST --headers='Content-Type=application/json' --message-body='{}' \
  --oauth-service-account-email="$SCHEDULER" \
  --oauth-token-scope=https://www.googleapis.com/auth/cloud-platform --max-retry-attempts=0

if [[ "$SCHEDULE_ACTION" == update ]]; then
  gcloud scheduler jobs resume "$JOB" --project="$PROJECT" --location="$REGION"
fi
