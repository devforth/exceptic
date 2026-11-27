# Configuration

All settings are environment variables. A wrong value stops the server at start with a message naming it. Lists are comma-separated; durations look like `30d`, `12h`, `90m`.

For local development copy [.env.example](../.env.example) to `.env.local`.

## Server

| Variable | Default | |
|---|---|---|
| `APP_NAME` | `Exceptic` | The name shown in the UI, emails and to coding agents. Up to 60 characters. |
| `PORT` | `8000` | Port to listen on. |
| `DATA_DIR` | `./data` (`/data` in the image) | Where the data is kept. Put it on a volume. |
| `PUBLIC_URL` | | The address users and apps reach the server at, e.g. `https://errors.example.com`. Used in DSNs, links in notifications and the MCP address. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. `debug` logs every request and why an envelope was rejected. |
| `GOMEMLIMIT` | `200MiB` | Memory the server tries to stay under. |
| `LOG_FORMAT` | `auto` | `auto` (colored in a terminal, logfmt elsewhere), `pretty`, `text` or `json`. |

## Projects

| Variable | Default | |
|---|---|---|
| `PROJECTS` | | Projects created at start: `id:name:key,...`, e.g. `1:backend:<key>,2:frontend:<key>`. The id is from 1 to 4294967295, the key is 16–64 letters or digits (`openssl rand -hex 16`). After the first start the name and settings change on the Projects page. Such a project cannot be deleted in the UI. Projects can also be added in the UI. |

The id is part of the DSN (`https://<key>@errors.example.com/<id>`): never change or reuse it. Use one project per service (backend, frontend, …).

## Access

| Variable | Default | |
|---|---|---|
| `ADMIN_EMAIL` | `admin` | Login for setting up the first admin: an email, up to 254 characters. |
| `ADMIN_PASSWORD` | | 12 to 256 characters. While there is no admin, log in with `ADMIN_EMAIL` and this password to create the first admin with your own email and password. After that both are ignored. |
| `SESSION_LIFETIME` | `1d` | How long a login lasts. |
| `GOOGLE_CLIENT_ID` | | With `GOOGLE_CLIENT_SECRET` and `PUBLIC_URL`: [Sign in with Google](deployment.md#sign-in-with-google). |
| `GOOGLE_CLIENT_SECRET` | | The secret of that Google client. |
| `API_TOKEN` | | Token for the [REST API](api.md), at least 32 characters (`openssl rand -hex 32`). Without it the REST API is off. |
| `ALLOWED_IPS` | any | Addresses or networks (`203.0.113.7,10.0.0.0/8`) that may open the UI, REST API and MCP. Apps can always send errors. |
| `TRUSTED_PROXIES` | | The address of your reverse proxy, e.g. `172.30.0.1`: [HTTPS and a reverse proxy](deployment.md#https-and-a-reverse-proxy). |

## Ingest

| Variable | Default | |
|---|---|---|
| `RATE_LIMIT_EVENTS_PER_MINUTE` | `10000` | Errors a project may send per minute. Over it the app pauses sending errors until the minute ends. `0` = no limit. |
| `RATE_LIMIT_LOGS_PER_MINUTE` | `10000` | Logs a project may send per minute. Keep it at least `100`, the most logs the JavaScript SDK sends at once. `0` = no limit. |
| `RECORD_LOG_LEVELS` | `all` | Log levels to store: `fatal`, `error`, `warn`, `info`, `debug`, `trace`, or `all`/`none`. A project can choose its own. Errors of every level are always stored. |

### Size limits

These are fixed.

- A request from an app: up to 20 MB.
- **Logs:** the body 16 KB; at most 64 attributes; an attribute's value 4 KB.
- **Events:** every text 16 KB. An event still over 1 MB loses its least useful parts first (breadcrumbs beyond the newest 10, extra data, the request body, local variables); the error, its stack trace and grouping stay.

Longer values are cut and end with `…`.

## Notifications

Where notifications go is set on the **Notifications** page. Telegram and Slack need nothing here. Email needs a mail server:

| Variable | Default | |
|---|---|---|
| `SECRET_KEY` | generated | At least 32 characters (`openssl rand -hex 32`). Encrypts chat tokens. Without it a key is created in `DATA_DIR/secret.key`. Do not lose or change it: saved chats would stop working. |
| `SMTP_HOST` | | Mail server, without the port. |
| `SMTP_PORT` | `587` | Port 465 uses TLS, others STARTTLS. |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | | SMTP login. |
| `EMAIL_FROM` | | Sender address. Required with `SMTP_HOST`. |
| `NOTIFY_RETRY_FOR` | `1d` | How long an undelivered message is retried (2s, 10s, 30s, … then hourly). |
| `NOTIFY_KEEP_SENT` | `1d` | How long sent messages stay in the delivery log. |
| `NOTIFY_KEEP_FAILED` | `30d` | How long failed messages stay there. |

## Retention

Nothing is deleted by age. Data stays until the database nears its size limit.

| Variable | Default | |
|---|---|---|
| `MAX_DATABASE_SIZE` | `2GB` | When the data reaches 99% of it, the least useful data is deleted until it is at 80%. Give the disk 1.3 times this. At least `500MB`; `0` turns the limit off. |

The cleanup deletes in this order:

1. Leftovers of deleted issues.
2. Logs older than a month.
3. Events older than a month, keeping the newest 20 of each issue.
4. Events of resolved and ignored issues, keeping the latest.
5. Logs below error, oldest first.
6. Events below error, oldest first, keeping the latest of each issue.
7. Every issue down to its newest 20 events.
8. Error and fatal logs, oldest first.
9. Events older than a week, keeping the latest of each issue.
10. Events of the last week, keeping the latest of each issue.
11. The latest event of issues not seen for a week (the issue stays in the list).
12. Whole issues, longest unseen first.

The first four always run; the rest only until the data is back at 80%. The **Cleanup history** on the Projects page shows every run.

- Events older than a month keep the error and stack trace and lose the rest (breadcrumbs, request, tags).
- An issue stores at most 10 events a minute; the rest are only counted.
- A log message repeated many times keeps its newest 10 copies.
- The [audit log](ui.md#audit-log) is never deleted.
- A project can have its own **Disk share** (its settings page): it then loses its own data first when it reaches it.
