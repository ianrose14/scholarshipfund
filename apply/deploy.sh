#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"

set -a
source .env   # export every variable in .env
set +a

gcloud run deploy apply --source . --region us-east1 --allow-unauthenticated  --max-instances 2 --memory 512Mi \
       --set-env-vars "BASE_URL=https://apply.allisonrosememorialfund.org" \
       --set-env-vars "CYCLE=$CYCLE" \
       --set-env-vars "DEADLINE=$DEADLINE" \
       --set-env-vars "^|^ADMIN_EMAILS=$ADMIN_EMAILS" \
       --set-env-vars "DROPBOX_APP_KEY=$DROPBOX_APP_KEY" \
       --set-secrets "SECRET_KEY=apply-secret-key:latest" \
       --set-secrets "CRON_SECRET=apply-cron-secret:latest" \
       --set-secrets "DROPBOX_APP_SECRET=apply-dropbox-app-secret:latest" \
       --set-secrets "DROPBOX_REFRESH_TOKEN=apply-dropbox-refresh-token:latest" \
       --set-secrets "RESEND_API_KEY=apply-resend-api-key:latest"
