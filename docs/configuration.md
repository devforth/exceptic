# Configuration

Everything is set with environment variables (in Docker: `-e`, `env_file` or `environment:`). An invalid value stops the server at startup with a message naming the variable. Lists are comma-separated. Durations take days or Go units: `30d`, `12h`, `90m`.

[.env.example](../.env.example) is a working local setup to copy to `.env.local`; the tables below have every variable.

## Server

| Variable | Default | |
|---|---|---|
| `APP_NAME` | `Exceptic` | Shown in the UI, emails and notifications, and to coding agents, who register the MCP server under it in lowercase with dashes (`Acme Errors` → `acme-errors`). Up to 60 characters. |
| `PORT` | `8000` | Port to listen on. |
| `DATA_DIR` | `./data` (`/data` in the image) | Where the database (`leveldb`) lives, and `secret.key` when `SECRET_KEY` is not set. Keep it on a volume. |
| `PUBLIC_URL` | | The address users and SDKs reach the server at, e.g. `https://errors.example.com` (a path prefix is kept; a user, query or fragment is refused at start). Used for links in notifications, the DSNs on the Projects page and the MCP address. Without it, DSNs use the address the UI was opened with and notifications have no links. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. `debug` also logs every request and every rejected envelope. |
| `GOMEMLIMIT` | `200MiB` | A soft limit the server aims to stay under: the Go runtime collects garbage harder as it nears it. Takes `MiB`, `GiB`. |
| `LOG_FORMAT` | `auto` | `auto`: readable colored lines in a terminal, logfmt otherwise (e.g. `docker logs`). Or `pretty`, `text`, `json`. `NO_COLOR` turns colors off. |

## Projects

| Variable | Default | |
|---|---|---|
| `PROJECTS` | | `id:name:public_key,...`, e.g. `1:backend:<key>,2:frontend:<key>`. The id is from 1 to 4294967295, the key is 16–64 letters or digits (`openssl rand -hex 16`). Created at start when missing, with that name; after that the name, color and levels change on the Projects page, and only the key follows `PROJECTS` at every start (a changed key stops the old DSN). A new entry whose name or key another project holds stops the start with a message naming that project. Such a project cannot be deleted on the Projects page; one removed from the list stays, DSN included, and can then be deleted there. Projects can also be added on the Projects page. |

The id is part of the DSN (`https://<public_key>@errors.example.com/<id>`) and of every stored event, so never change or reuse it. Name each service (backend, frontend, …) as its own project.

## Access

| Variable | Default | |
|---|---|---|
| `ADMIN_EMAIL` | `admin` | The superadmin's login, created at the first start only: an email, or up to 254 letters, digits and `. _ @ -` (changed later on the Account page, then to an email only). `ADMIN_USERNAME`, its former name, is read while it is not set, with a warning. |
| `ADMIN_PASSWORD` | | 12 to 256 characters. Needed for the first start; later changes are ignored (change the password in the UI, or see [forgotten password](deployment.md#forgotten-password)). Until it is set, nobody can log in. |
| `SESSION_LIFETIME` | `1d` | How long a login to the UI lasts (`12h`, `7d`); not extended by use. |
| `GOOGLE_CLIENT_ID` | | With `GOOGLE_CLIENT_SECRET`, users may also sign in with Google: an OAuth client of Google Cloud ([Sign in with Google](deployment.md#sign-in-with-google)). Needs `PUBLIC_URL`. |
| `GOOGLE_CLIENT_SECRET` | | The secret of that OAuth client. |
| `API_TOKEN` | | Token of the [REST API](api.md), at least 32 characters (`openssl rand -hex 32`). Without it the REST API is closed. MCP does not use it. |
| `ALLOWED_IPS` | any | Addresses or networks (`203.0.113.7,10.0.0.0/8`) allowed to open the UI, REST API and MCP; others get 403, and `LOG_LEVEL=debug` logs the address the server saw. Ingest, `/health` and the links that stop emails (`/unsubscribe/…`, signed) stay open. In Docker the host itself, and with `"8000:8000"` every IPv6 client, reach the container from the compose network's gateway, so never let the list cover it (no `172.16.0.0/12` or `192.168.0.0/16`); behind a proxy the client address comes from `TRUSTED_PROXIES`. |
| `TRUSTED_PROXIES` | | Reverse proxies (addresses or networks) whose `X-Forwarded-For` and `X-Forwarded-Proto` are believed. Name only the proxy's own address (in Docker, the gateway of the compose network for a proxy on the host; see [Deployment](deployment.md#https-and-a-reverse-proxy)). Needed behind a proxy: for the client address (`ALLOWED_IPS`, login limits) and to know the page is HTTPS (secure cookies, DSNs). |

## Ingest

| Variable | Default | |
|---|---|---|
| `RATE_LIMIT_EVENTS_PER_MINUTE` | `10000` | Events (errors and messages) a project may send per minute, a guard against abuse: an error in a loop below it is already capped per issue ([Retention](#retention)). Over it the SDK is told to pause sending events until the minute ends; its logs keep coming ([what an SDK sees](sdk.md#rate-limits)). `0` = no limit. |
| `RATE_LIMIT_LOGS_PER_MINUTE` | `10000` | Logs a project may send per minute, counted one by one as received, before the levels it does not record are left out; over it only its logs pause, errors keep coming. Keep it at least `100`, the most logs the JavaScript SDK sends in one batch: a batch over the limit is dropped whole (the server warns at start). `0` = no limit. `RATE_LIMIT_PER_MINUTE` (it counted envelopes) is ignored with a warning at start. |
| `RECORD_LOG_LEVELS` | `all` | Log levels to store: `fatal`, `error`, `warn`, `info`, `debug`, `trace`, or `all`/`none`. Others are dropped on arrival. A project can choose its own on the Projects page. Events (errors and messages) of every level are stored. |

### Size limits

Fixed, not settings. An envelope may have 20 MB, before and after decompression; a larger one gets 413. What is stored from it is cut, never dropped, after credentials are removed, so a secret is never stored cut in half. A cut text keeps its first bytes up to the limit, where a character starts, and ends with `…`.

- **Logs:** the body 16 KB; at most 64 attributes: the SDK's `sentry.*` ones first (the parent span, which ties a log to its error, before all), then the others by name (the rest are dropped); an attribute's value 4 KB (a longer value of another type than string is kept as its JSON text, cut), its name 256 bytes and type 32; the trace and span IDs 64 bytes.
- **Stack traces:** at most 250 frames each, the 50 oldest and the 200 newest (a recursion 10,000 frames deep keeps its start and the frames where it failed; grouping then reads the frames kept, so recursions of any depth over 250 frames are one issue). A stack with more than 50 frames that keep their source (the in-app ones, or all when none is in-app) keeps the variables and the lines around the source line (`vars`, `pre_context`, `post_context`) only for the 25 oldest and 25 newest of them; the source line itself stays, as grouping uses it. A stack that lost frames is named `frames` in `_exceptic_truncated`.
- **Events:** every string value 16 KB. An event still over 1 MB then loses, least useful first and only as much as it takes: breadcrumbs beyond the newest 10, `extra`, the request body (`request.data`), contexts other than `runtime`, `os`, `browser` and `trace`, local variables of frames, source lines around them (`pre_context`, `post_context`), and at last every top-level field other than the event's ID, time, level, platform, logger, transaction, release, environment, message, exception, stack traces, fingerprint and contexts. The exception types, frames (file, function, line, the source line) and the fingerprint stay, so grouping is the same. A cut event names its cuts in `_exceptic_truncated`, in that order: `frames` (above), `strings`, `breadcrumbs`, `extra`, `request.data`, `contexts`, `frame_vars`, `frame_context`, `other` (the fields it drops, the breadcrumbs and request included), and `size`: an event still over 1 MB after them keeps what fits, the smallest fields whole first and a list's newest elements; a field of that name sent by an SDK is dropped.

## Notifications

Where messages go (email addresses, Telegram chats, Slack channels) is set up on the **Notifications** page of the UI. Telegram and Slack need nothing here: their tokens are entered in the form. Email needs a mail server.

| Variable | Default | |
|---|---|---|
| `SECRET_KEY` | generated | At least 32 characters (`openssl rand -hex 32`). Encrypts Telegram tokens and Slack webhooks and tokens in the database, and signs the links that stop emails and the cookies of the login's known devices. Without it a key is generated at the first start into `DATA_DIR/secret.key`; that protects a leaked copy of the database (`DATA_DIR/leveldb`) but not a copy of the whole data directory, so on a server set it and keep it outside. Do not lose or change it: stored chats stop working (enter their tokens again), the stop links of emails already sent no longer work, and every browser logs in as a new device. To move a generated key out, set `SECRET_KEY` to the contents of `secret.key`, then delete the file (spaces and line breaks around the value are ignored, as in the file). |
| `SMTP_HOST` | | Mail server, without the port (that is `SMTP_PORT`); required for email. |
| `SMTP_PORT` | `587` | Port 465 uses TLS from the start, others STARTTLS when the server offers it; without it mail goes unencrypted, and a login (`SMTP_USERNAME`) is refused unless the server is `localhost`. |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | | SMTP login. |
| `EMAIL_FROM` | | Sender address; required with `SMTP_HOST`. |
| `NOTIFY_RETRY_FOR` | `1d` | A message that cannot be delivered is retried (2s, 10s, 30s, … then hourly) this long, then marked failed. A chat's rate limit (`Retry-After`) pauses its connection for the time the chat asks, never past this time (a message it would delay longer is marked failed); saving the connection or Retry now ends the pause. |
| `NOTIFY_KEEP_SENT` | `1d` | Delivered messages are deleted from the delivery log after this. |
| `NOTIFY_KEEP_FAILED` | `30d` | Failed messages are deleted after this. |

Telegram and Slack connections can be tested from the form before saving; email addresses are confirmed with a code first ([Notifications](ui.md#notifications)).

## Retention

While the limit is on, events lose their details as they turn a month old, every 10 minutes: the exception, message and stack trace stay without local variables; breadcrumbs, request, user, tags, contexts and extra go, and the event's page says so. Nothing is deleted by age alone: data stays until the database nears its size limit.

| Variable | Default | |
|---|---|---|
| `MAX_DATABASE_SIZE` | `2GB` | Once the data the database holds reaches 99% of it, the least useful data is deleted until it holds 80%. The files may take more (see below); give the disk at least 1.3 times this. Takes `KB`, `MB`, `GB`, at least `500MB`; `0` turns the limit off, and nothing is deleted or thinned. Envelopes waiting to be stored may take 20% of it (of `2GB` when it is off); beyond that new ones get `429` until the queue drains. |

The size is checked at startup, every 10 minutes and after every 50 MB written. At startup and every 10 minutes, halfway between the checks, limit or not, the ingest queue is compacted, so envelopes that waited there as the SDKs sent them, credentials included, leave the files; the pass is skipped while envelopes arrive (one came in the last 2 seconds).

What counts is the data the database holds, as the Projects page shows it (each project's part and the rest, and below what the files take): the errors, issues and logs of every project plus the rest (users, sessions, settings, status history, notifications waiting to be sent). It goes down as soon as something is deleted. The files of the database (`DATA_DIR/leveldb`) also hold what this leaves out:

- deleted and replaced records, until their files are compacted: the compaction after a cleanup frees what it deleted, and the size check compacts the logs and events once they are more than 10% of the data, so about that much between checks, more for a moment after a large delete;
- envelopes waiting in the ingest queue: up to 20% of the limit under a peak, usually next to nothing;
- the write journal: up to 16 MB of the newest writes, before they are compressed into the files.

The cleanup compacts nothing first: deleted records are not counted as data, and compacting would lower the data by a percent or two (old versions of issues, which the database frees on its own), less than a run deletes. As a safety net, when the files take more than 125% of the limit, the size check first compacts the logs and events, the ingest queue, the issues and the status history, even while envelopes arrive (each part of a few seconds then refuses them, see below), and if the files still do, the cleanup runs even with the data under 99%, until the data is at 80% and the files are back under the limit; the Cleanup history shows what the files took.

A run ends once it has deleted enough. The files shrink right after it, in a compaction of their own on the same lane: over all projects it compacts the logs and events the run deleted from, and the issues and status history when it deleted issues; after a project's run, only once deleted records are more than 10% of the data (below that they wait for later deletes, so the compaction never rewrites every project's records for a small one). It goes in parts of a few seconds each, so it never holds back the size check: between parts it lets a size check that is due run first, then goes on where it stopped, after a restart too. Nothing is written while a part runs: the database would go on compacting for as long as new files come, and under ingest every file it writes holds keys of the whole database (measured under a flood: parts of minutes, while the files grew past three times the limit). So each part starts only once no envelope came for 2 seconds, and envelopes arriving during it get `429` with `Retry-After: 5`; under a steady flood the compaction waits ("compacting…" stays) while the cleanup goes on deleting, and the database's own compactions free the deleted records more slowly; the safety net above compacts even then. When runs over all projects and over a project wait together, the one over all projects goes first and frees both; the size check leaves a run's deleted records to its compaction, so it is always recorded. The Cleanup history shows per run how long the compaction took and what it freed (the deleted records it compacted away, so envelopes arriving meanwhile, which can leave the files larger than before, do not hide it), "not needed" when nothing was left to compact, or "compacting…" until it is done ("retrying…" after a failed attempt); the time shown is the time spent compacting. A compaction that fails, or never finishes (a crash, a killed process), is tried again on the next passes of its job, 3 attempts in all, then shows as failed with its last error. No cleanup and no compaction runs while a backup is being written: its snapshot keeps deleted records in the files until it ends, and the compaction comes after it. The cleanup always deletes:

1. Events and status changes of issues that are no longer stored: only a bug leaves them, so the log warns when any go.
2. Logs older than a month.
3. Events older than a month beyond the newest 20 of their issue.
4. The events of resolved and ignored issues but their latest.

These four run in full even when they free more than needed. Then, only while the data is still above 80% (or the files above the limit, for a run the files started), each step after the one before:

5. Logs below error (trace, debug, info, warn), oldest first.
6. Events below error (debug, info, warning: messages, PHP notices, console warnings), oldest first, but the latest of each issue.
7. Every issue down to its newest 20 events (new events are capped at 10 a minute per issue as they arrive, see below; this step trims what the cap let through), so an error in a loop gives up its copies before the error and fatal logs of every project.
8. Error and fatal logs together, oldest first.
9. Events older than a week, oldest first whatever their level, but the latest of each issue.
10. Events of the last week, oldest first whatever their level, but the latest of each issue.
11. The latest event of issues not seen for a week, longest unseen first: the issue stays listed with its title, count and dates.
12. Whole issues, longest unseen first.

It deletes in batches of up to 2000 records and stops as soon as enough is freed. Each batch looks at the data as it is then, so an issue that gets a new event or is reopened meanwhile is judged by that.

A project may have a disk share below 100% (its settings page): a part of `MAX_DATABASE_SIZE` it may take, a ceiling rather than a reservation. Each size check first cleans every project that reached its share, the smallest share first, one after another: the same steps in the same order, each deleting only that project's data, down to 80% of its share. A project's run starts at 99% of its share, like the one over all projects. Only then, if the database is still at 99% of its limit, the cleanup over all projects runs. So a project that floods loses its own data before it can push out the others.

A project's size is its part of the database as the Projects page shows it: the files without the ingest queue's and the rest's (users, sessions, settings, history), in the proportion of its data to all the errors, issues and logs stored. It is an estimate, since one proportion serves every project: logs and events take different space for the same data, so the sizes move a little as the mix changes, a run that deletes one project's logs included. Deleted and replaced logs and events (by a cleanup, deleted issues or logs, a deleted project, thinning, a repeated log message) keep their space until their files are compacted; the database counts them, across restarts too, and they count as no project's, so one project's deletes never raise another's size. Compacting the logs and events rewrites every project's records, so it waits until those deleted records are more than 10% of the data: then the size check compacts them before it judges the shares (unless a run's compaction is waiting, which then does it), and the compaction after a project's run does too. The database also compacts some of them on its own without saying which, so until then the sizes may read up to that much low.

A cleanup stopped by a restart goes on where it stopped, for the same project, unless `MAX_DATABASE_SIZE` or that project's share changed meanwhile, or an upgrade changed the cleanup's steps: then it ends as failed ("stopped: MAX_DATABASE_SIZE changed to …", "… turned off", "stopped: the disk share of … changed to …", "stopped: the project was deleted", "stopped: the cleanup's steps changed") and the next one starts from the new limit. Each run is logged with what every step deleted per project, its compaction with the files' size before and after, and the last 100 runs with their steps and project are on the Projects page (Cleanup history). The history counts each run from the data it started with: after is that data less what the run freed, so errors and logs arriving during a run never make it look as if it freed less; when they come to 1% of the limit or more, the run's details show them apart with the data held at its end, and the result names what the run set out to free, not a percentage of the data held at its end; when other deletes meanwhile (a deleted project, issues or logs) brought the data down before the run freed it, the result says so. Runs recorded before the history counted from the data show the files' size before and no after.

A message logged over and over keeps its newest 10 copies as it is written (numbers and IDs in it do not count as a difference); the issue's event count always includes every event.
An issue stores at most 10 events a minute (by the time they arrive); the others only count: the event count and last seen include them, and the issue page shows how many were not stored. The event that opens an issue or reopens it is always stored. The cap is per issue, so a loop does not hold back other errors, unless it sends more than the project's `RATE_LIMIT_EVENTS_PER_MINUTE`: ingest counts events before grouping, so the SDKs of that project then pause all its events until the minute ends.
