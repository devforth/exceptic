# Configuration

Everything is set with environment variables (in Docker: `-e`, `env_file` or `environment:`). An invalid value stops the server at startup with a message naming the variable. Lists are comma-separated. Durations take days or Go units: `30d`, `12h`, `90m`.

[.env.example](../.env.example) has every variable with a comment; copy it to `.env` and fill in what you need.

## Server

| Variable | Default | |
|---|---|---|
| `APP_NAME` | `Exceptic` | Shown in the UI, emails and notifications, and to coding agents, who register the MCP server under it in lowercase with dashes (`Acme Errors` → `acme-errors`). Up to 60 characters. |
| `PORT` | `8000` | Port to listen on. |
| `DATA_DIR` | `./data` (`/data` in the image) | Where the database (`leveldb`) lives, and `secret.key` when `SECRET_KEY` is not set. Keep it on a volume. |
| `PUBLIC_URL` | | The address users and SDKs reach the server at, e.g. `https://errors.example.com` (a path prefix is kept; a user, query or fragment is refused at start). Used for links in notifications, the DSNs on the Projects page and the MCP address. Without it, DSNs use the address the UI was opened with and notifications have no links. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. `debug` also logs every request and every rejected envelope. |
| `GOMEMLIMIT` | `200MiB` | The memory the server aims to stay under: the Go runtime collects garbage harder as it nears it. A soft limit, not a cap. Takes `MiB`, `GiB`. |
| `LOG_FORMAT` | `auto` | `auto`: readable colored lines in a terminal, logfmt otherwise (e.g. `docker logs`). Or `pretty`, `text`, `json`. `NO_COLOR` turns colors off. |

## Projects

| Variable | Default | |
|---|---|---|
| `PROJECTS` | | Optional: `id:name:public_key,...`, e.g. `1:backend:<key>,2:frontend:<key>`. The id is from 1 to 4294967295, the key is 16–64 letters or digits (`openssl rand -hex 16`). Created at start when missing, with that name; after that the name, color and levels change on the Projects page, and only the key follows `PROJECTS` at every start (a changed key stops the old DSN). A new entry whose name or key another project holds stops the start with a message naming that project. Such a project cannot be deleted on the Projects page; one removed from the list stays, DSN included, and can then be deleted there. Projects can also be added on the Projects page. |

The id is part of the DSN (`https://<public_key>@errors.example.com/<id>`) and of every stored event, so never change or reuse it. Name each service (backend, frontend, …) as its own project.

## Access

| Variable | Default | |
|---|---|---|
| `ADMIN_USERNAME` | `admin` | The superadmin, created at the first start only: up to 64 letters, digits and `. _ @ -`. |
| `ADMIN_PASSWORD` | | 12 to 256 characters. Needed for the first start; later changes are ignored (change the password in the UI, or see [forgotten password](deployment.md#forgotten-password)). Until it is set, nobody can log in. |
| `SESSION_LIFETIME` | `1d` | How long a login to the UI lasts (`12h`, `7d`); it is not extended by use, so then you log in again. |
| `API_TOKEN` | | Token of the [REST API](api.md), at least 32 characters (`openssl rand -hex 32`). Without it the REST API is closed. MCP does not use it. |
| `ALLOWED_IPS` | any | Addresses or networks (`203.0.113.7,10.0.0.0/8`) allowed to open the UI, REST API and MCP; others get 403, and `LOG_LEVEL=debug` logs the address the server saw. Ingest and `/health` stay open. In Docker the host itself, and with `"8000:8000"` every IPv6 client, reach the container from the compose network's gateway, so never let the list cover it (no `172.16.0.0/12` or `192.168.0.0/16`); behind a proxy the client address comes from `TRUSTED_PROXIES`. |
| `TRUSTED_PROXIES` | | Reverse proxies (addresses or networks) whose `X-Forwarded-For` and `X-Forwarded-Proto` are believed. Name only the proxy's own address (in Docker, the gateway of the compose network for a proxy on the host; see [Deployment](deployment.md#https-and-a-reverse-proxy)). Needed behind a proxy: for the client address (`ALLOWED_IPS`, login limits) and to know the page is HTTPS (secure cookies, DSNs). |

## Ingest

| Variable | Default | |
|---|---|---|
| `ALLOWED_ORIGINS` | `*` | Browser origins allowed to send events (CORS), e.g. `https://app.example.com,https://admin.example.com`. Backend SDKs send no origin and are always allowed. |
| `RATE_LIMIT_PER_MINUTE` | `1000` | Envelopes with events or logs per project per minute (spans and other dropped items do not count); over it SDKs get 429 and back off. `0` = no limit. |
| `RECORD_LOG_LEVELS` | `all` | Log levels to store: `fatal`, `error`, `warn`, `info`, `debug`, `trace`, or `all`/`none`. Others are dropped on arrival. A project can choose its own on the Projects page. Events (errors and messages) of every level are stored. |

## Notifications

Where messages go (email addresses, Telegram chats, Slack channels) is set up on the **Notifications** page of the UI. Telegram and Slack need nothing here: their tokens are entered in the form. Email needs a mail server.

| Variable | Default | |
|---|---|---|
| `SECRET_KEY` | generated | At least 32 characters (`openssl rand -hex 32`). Encrypts Telegram tokens and Slack webhooks and tokens in the database. Without it a key is generated at the first start into `DATA_DIR/secret.key`; that protects a leaked copy of the database (`DATA_DIR/leveldb`) but not a copy of the whole data directory, so on a server set it and keep it outside. Do not lose or change it: stored chats stop working (enter their tokens again). To move a generated key out, set `SECRET_KEY` to the contents of `secret.key`, then delete the file (spaces and line breaks around the value are ignored, as in the file). |
| `SMTP_HOST` | | Mail server, without the port (that is `SMTP_PORT`); required for email. |
| `SMTP_PORT` | `587` | Port 465 uses TLS from the start, others STARTTLS when the server offers it; without it mail goes unencrypted, and a login (`SMTP_USERNAME`) is refused unless the server is `localhost`. |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | | SMTP login. |
| `EMAIL_FROM` | | Sender address; required with `SMTP_HOST`. |
| `NOTIFY_RETRY_FOR` | `1d` | A message that cannot be delivered is retried (2s, 10s, 30s, … then hourly) this long, then marked failed. A chat's rate limit (`Retry-After`) pauses its connection for the time the chat asks, never past this time (a message it would delay longer is marked failed); saving the connection or Retry now ends the pause. |
| `NOTIFY_KEEP_SENT` | `1d` | Delivered messages are deleted from the delivery log after this. |
| `NOTIFY_KEEP_FAILED` | `30d` | Failed messages are deleted after this. |

Email addresses are confirmed with a code before they receive anything. Telegram and Slack connections can be tested from the form before saving.

## Retention

While the limit is on, events lose their details as they turn a month old, every 10 minutes: the exception, message and stack trace stay without local variables; breadcrumbs, request, user, tags, contexts and extra go, and the event's page says so. Nothing is deleted by age alone: data stays until the database nears its size limit.

| Variable | Default | |
|---|---|---|
| `MAX_DATABASE_SIZE` | `2GB` | Once the database reaches it, the least useful data is deleted until the database is at 80%. Takes `KB`, `MB`, `GB`, at least `500MB`; `0` turns the limit off, and nothing is deleted or thinned. Envelopes waiting to be stored may take 20% of it (of `2GB` when it is off); beyond that new ones get `429` until the queue drains. |

The size is checked at startup, every 10 minutes and after every 50 MB written. Every 10 minutes, halfway between the checks and limit or not, the ingest queue is compacted, so envelopes that waited there as the SDKs sent them, credentials included, leave the files; the pass is skipped while envelopes arrive (one came in the last 2 seconds, or the server has just started). Only the database files in use count (`DATA_DIR/leveldb`). Deleted data keeps its space until its files are compacted, so at the limit the logs and events are compacted first if anything in them was deleted or replaced since they were last compacted, and the parts that collect deleted data as the server works (the ingest queue, issues) when ingest is quiet; if the database is still at the limit, the cleanup runs and always deletes:

1. Logs older than a month.
2. Events older than a month beyond the newest 20 of their issue.
3. The events of resolved and ignored issues but their latest.
4. Logs below error (trace, debug, info, warn), whatever their age.

Then, only while the database is still above 80%, each step after the one before:

5. Events below error (debug, info, warning: messages, PHP notices, console warnings), oldest first, but the latest of each issue.
6. Error and fatal logs together, oldest first.
7. Every issue down to its newest 20 events (new events are not capped as they arrive).
8. Events older than a week, oldest first whatever their level, but the latest of each issue.
9. Events of the last week, oldest first whatever their level, but the latest of each issue.
10. The latest event of issues not seen for a week, longest unseen first: the issue stays listed with its title, count and dates.
11. Whole issues, longest unseen first.

It deletes in batches of up to 2000 records and stops as soon as enough is freed. Each batch looks at the data as it is then, so an issue that gets a new event or is reopened meanwhile is judged by that.

A project may have a disk share below 100% (its settings page): a part of `MAX_DATABASE_SIZE` it may take, a ceiling rather than a reservation. Each size check first cleans every project that reached its share, the smallest share first, one after another: the same steps in the same order, each deleting only that project's data, down to 80% of its share. Only then, if the database is still at its limit, the cleanup over all projects runs. So a project that floods loses its own data before it can push out the others.

A project's size is its part of the database as the Projects page shows it: the files without the ingest queue's, in the proportion of its data to all the data stored. It is an estimate, since one proportion serves every project: logs and events take different space for the same data, so the sizes move a little as the mix changes, a run that deletes one project's logs included. Deleted and replaced logs and events (by a cleanup, deleted issues, a deleted project, thinning, a repeated log message) keep their space until their files are compacted; the database counts them, across restarts too, and they count as no project's, so one project's deletes never raise another's size. Compacting the logs and events rewrites every project's records, so it waits until those deleted records are more than 10% of the data: then the size check compacts them, before it judges the shares and at the end of a project's run. The database also compacts some of them on its own without saying which, so until then the sizes may read up to that much low. Until a database from an earlier version is counted ("counting…"), only the cleanup over all projects runs.

A cleanup stopped by a restart goes on where it stopped, for the same project, unless `MAX_DATABASE_SIZE` or that project's share changed meanwhile: then it ends as failed ("stopped: MAX_DATABASE_SIZE changed to …", "… turned off", "stopped: the disk share of … changed to …", "stopped: the project was deleted") and the next one starts from the new limit. Each run is logged with what every step deleted per project, and the last 100 runs with their steps and project are on the Projects page (Cleanup history).

The Projects page shows each project's part of the database. A database written by an earlier version is counted once in the background after the upgrade; until then the page says "counting…".

A message logged over and over keeps its newest 10 copies as it is written (numbers and IDs in it do not count as a difference); the issue's event count always includes every event.
