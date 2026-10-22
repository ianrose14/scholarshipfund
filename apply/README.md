# apply.allisonrosememorialfund.org

The "manage my application" site. Applicants sign in with an emailed magic link
(no passwords) and see a checklist of required materials:

1. **Application form**: a web form; on save, the server fills in the official
   PDF (`assets/application_form.pdf`) and saves it as `Application Form.pdf`.
2. **Personal statement**, uploaded (PDF/Word).
3. **Resume / CV**, uploaded (PDF/Word).
4. **Financial aid certification**: the applicant downloads the blank form
   (`static/financial_aid_certification.pdf`, a copy of
   `forms/financial_aid_certification_v1.pdf`), has their school's financial aid
   office fill in and sign it, and uploads it.
5. **Current NNP program transcript**, uploaded (PDF/Word). Only required (and
   only shown) if the applicant's program status says they're already enrolled.
6. **Letters of recommendation**: the applicant lists recommenders (name, email,
   title, supervisor?). Each recommender is emailed a personal upload link.
   Applicants see whether each letter has arrived, never its contents. Complete
   once 2 letters are in, at least one from a supervisor. The page also asks for
   at least one letter from a physician or APP, but that isn't tracked or enforced.

When everything is in, the applicant and `NOTIFY_EMAILS` get an email. A daily
cron job sends one reminder to incomplete applicants and outstanding recommenders
in the last week before the deadline. Admins (`ADMIN_EMAILS`) get a table of all
applicants at `/admin` and a CSV export.

## Storage: Dropbox is the database

There is no database. Each applicant gets a folder in Dropbox:

```
/Apps/<your Dropbox app>/2027/jane.doe@example.com/
    applicant.json            <- all form answers, recommenders, statuses
    Application Form.pdf      <- regenerated on every save
    Personal Statement.pdf
    Resume.docx
    Letter - Sam Sullivan.pdf
```

Logins, sessions and recommender links are HMAC-signed tokens (`tokens.go`), so
nothing else needs storing. `applicant.json` is updated with Dropbox's
revision-checked writes, so simultaneous edits (e.g. a recommender uploading
while the applicant edits) can't clobber each other. Replaced files remain in
Dropbox's version history.

## Code map

| File | What |
|---|---|
| `main.go`, `config.go` | startup and env-var configuration |
| `server.go` | routes, sessions, middleware, rendering |
| `handlers_*.go` | sign-in, applicant, recommender, admin and cron endpoints |
| `model.go` | `applicant.json` schema and checklist rules |
| `store.go`, `dropbox.go`, `localstore.go` | storage (Dropbox in prod, a local dir in dev) |
| `mail.go`, `emails.go` | Resend client and email text |
| `pdf.go` | fills the official PDF form |
| `templates/`, `static/` | HTML and CSS (no JavaScript) |

## Local development

No Dropbox or Resend needed: files go to a local directory, and emails (including
sign-in links) are printed to the terminal.

```sh
cd apply
go test ./...

PORT=8080 BASE_URL=http://localhost:8080 \
SECRET_KEY=dev-secret-dev-secret-dev-secret-dev \
CYCLE=2027 DEADLINE=2027-03-01T23:59:59-05:00 \
LOCAL_DATA_DIR=/tmp/apply-data ADMIN_EMAILS=you@example.com \
go run .
```

## Configuration

| Variable | Required | Notes |
|---|---|---|
| `BASE_URL` | yes | `https://apply.allisonrosememorialfund.org` |
| `SECRET_KEY` | yes | 32+ random chars (`openssl rand -base64 48`). Changing it signs everyone out and breaks outstanding recommender links. |
| `CYCLE` | yes | e.g. `2027` |
| `DEADLINE` | yes | RFC 3339, e.g. `2027-03-01T23:59:59-05:00`. Applicants can't make changes or start new applications after this. |
| `LETTER_DEADLINE` | | Defaults to `DEADLINE`. Set it later to give recommenders a grace period. |
| `STORAGE_ROOT` | | Folder for this cycle; defaults to `/$CYCLE`. |
| `ADMIN_EMAILS` | | Comma-separated. These users see `/admin` instead of an application. |
| `NOTIFY_EMAILS` | | Who's told when an application is complete; defaults to `ADMIN_EMAILS`. |
| `DROPBOX_APP_KEY`, `DROPBOX_APP_SECRET`, `DROPBOX_REFRESH_TOKEN` | prod | See below. |
| `LOCAL_DATA_DIR` | dev | Used when there's no Dropbox token. |
| `RESEND_API_KEY` | prod | Without it, emails are printed to stdout. |
| `EMAIL_FROM` | | Default `Dr. Allison Rose Memorial Fund <applications@allisonrosememorialfund.org>` |
| `EMAIL_REPLY_TO` | | Default `applications@allisonrosememorialfund.org` |
| `CRON_SECRET` | prod | Shared secret for `POST /tasks/reminders`. |

## One-time setup

### 1. Dropbox app

1. At <https://www.dropbox.com/developers/apps>, choose **Create app** → *Scoped access* → **App folder** (the app can only see `/Apps/<name>/` in your Dropbox).
2. On the **Permissions** tab, enable `files.metadata.read`, `files.content.read` and `files.content.write`, then **Submit**.
3. Note the **App key** and **App secret** on the Settings tab.
4. Get a long-lived refresh token. Open this URL while signed in to Dropbox, approve, and copy the code:
   `https://www.dropbox.com/oauth2/authorize?client_id=APP_KEY&response_type=code&token_access_type=offline`
5. Exchange the code:
   ```sh
   curl https://api.dropboxapi.com/oauth2/token -u APP_KEY:APP_SECRET \
     -d grant_type=authorization_code -d code=THE_CODE
   ```
   Keep the `refresh_token` from the response (the `access_token` expires in 4 hours and isn't needed).

### 2. Resend

The domain is already verified for sending (see `../dns.md`). Create an API key
with "sending access" at <https://resend.com/api-keys>.

### 3. Google Cloud

```sh
PROJECT=ian-rose
gcloud config set project $PROJECT
gcloud services enable run.googleapis.com cloudbuild.googleapis.com \
  artifactregistry.googleapis.com secretmanager.googleapis.com cloudscheduler.googleapis.com

# secrets
openssl rand -base64 48 | tr -d '\n' | gcloud secrets create apply-secret-key --data-file=-
openssl rand -hex 24    | tr -d '\n' | gcloud secrets create apply-cron-secret --data-file=-
printf %s "$DROPBOX_APP_SECRET"    | gcloud secrets create apply-dropbox-app-secret --data-file=-
printf %s "$DROPBOX_REFRESH_TOKEN" | gcloud secrets create apply-dropbox-refresh-token --data-file=-
printf %s "$RESEND_API_KEY"        | gcloud secrets create apply-resend-api-key --data-file=-

# let Cloud Run's runtime service account read them
SA=$(gcloud projects describe $PROJECT --format='value(projectNumber)')-compute@developer.gserviceaccount.com
for s in apply-secret-key apply-cron-secret apply-dropbox-app-secret apply-dropbox-refresh-token apply-resend-api-key; do
  gcloud secrets add-iam-policy-binding $s --member=serviceAccount:$SA --role=roles/secretmanager.secretAccessor
done
```

### 4. Deploy

Create `apply/.env` (gitignored) with the per-cycle settings:

```sh
CYCLE=2027
DEADLINE=2027-04-19T00:00:00-04:00
ADMIN_EMAILS=ianrose@allisonrosememorialfund.org
DROPBOX_APP_KEY=...
```

Then run `./deploy.sh`. It loads `.env` and runs `gcloud run deploy` with those
values plus the secrets from step 3. Re-run it to deploy changes; for a new
cycle, change `CYCLE` and `DEADLINE` in `.env`. `ADMIN_EMAILS` may contain
commas (multiple admins).

### 5. Custom domain

```sh
gcloud domains verify allisonrosememorialfund.org   # one-time, via Search Console
gcloud beta run domain-mappings create --service apply --region us-east1 \
  --domain apply.allisonrosememorialfund.org
```

Then add the DNS record it prints: `CNAME apply → ghs.googlehosted.com.`
(documented in `../dns.md`). The TLS certificate is issued automatically, which can take
anywhere from a few minutes to about an hour.

### 6. Daily reminders

```sh
gcloud scheduler jobs create http apply-reminders --location us-east1 \
  --schedule "0 10 * * *" --time-zone America/New_York \
  --uri https://apply.allisonrosememorialfund.org/tasks/reminders --http-method POST \
  --headers "X-Cron-Secret=$(gcloud secrets versions access latest --secret apply-cron-secret)"
```

It's safe to run daily all year: outside the last week before the deadline it does nothing,
and each person is reminded at most once.

## Each new cycle

1. Update `CYCLE` and `DEADLINE` in `.env` and run `./deploy.sh`. Files go to a fresh `/<CYCLE>` folder, so last year's applicants start over.
2. If the PDF form changes, regenerate it (`python scripts/gen_application.py`), copy `forms/application_form_v1.pdf` to `apply/assets/application_form.pdf`, and keep the field names in `pdf.go` in sync.
