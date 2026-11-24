# Connecting apps

The server speaks the protocol of the official [Sentry SDKs](https://docs.sentry.io/platforms/), so an app is connected by installing the SDK of its platform and giving it a DSN. Nothing else changes in the app.

Each service (backend, frontend, worker, …) should be its own project: the UI shows which project an error came from, and notifications and levels can be set per project. Copy the DSN from the **Projects** page (a coding agent connected over [MCP](mcp.md) gets it from `list_projects`):

```
https://<public_key>@errors.example.com/<project id>
```

The project's **Show installation code** (⋯ on the Projects page) shows the install command, the setup code with the DSN filled in and a test line for Node.js, browser, Python, Go, PHP and Ruby, and copies the same as instructions for a coding agent. It shows the project as connected at the first request with its key of any kind: Node.js and browser SDKs send a session when the app starts, Python at its first request; other SDKs may connect only with the test error.

Tested with `@sentry/node` and `@sentry/browser` 11.11. Grouping and rate limits follow Sentry and Relay as checked in November 2026. Other SDKs send the same envelope format and should work; please report one that does not.

## Node.js

```sh
npm install @sentry/node
```

Initialize it before anything else is imported (e.g. a file loaded with `node --import ./instrument.mjs`):

```js
import * as Sentry from "@sentry/node";

Sentry.init({
  dsn: process.env.SENTRY_DSN,
  environment: process.env.NODE_ENV,
  // Send logs: Sentry.logger.info(...), and console/pino output with the integrations below.
  enableLogs: true,
  // Tracing ties logs to the request that failed. Spans are not stored, yet every
  // sampled request sends an envelope of them, so keep the rate low.
  tracesSampleRate: 0.1,
  integrations: [
    Sentry.consoleLoggingIntegration({ levels: ["log", "warn", "error"] }),
    // Sentry.pinoIntegration(),
  ],
});
```

Unhandled errors are sent automatically; handled ones with `Sentry.captureException(error)`, messages with `Sentry.captureMessage("text", "warning")`.

The SDK's `Dedupe` integration drops an error identical to the previous one, however much later it happens, so repeats are not counted. The server groups repeats into one issue and has its own [rate limits](#rate-limits), so you may turn it off with this `integrations` in place of the list above:

```js
integrations: (defaults) => [
  ...defaults.filter((integration) => integration.name !== "Dedupe"),
  Sentry.consoleLoggingIntegration({ levels: ["log", "warn", "error"] }),
],
```

## Browser

```sh
npm install @sentry/browser
```

```js
import * as Sentry from "@sentry/browser";

Sentry.init({
  dsn: "https://<public_key>@errors.example.com/2",
  enableLogs: true,
  integrations: [Sentry.browserTracingIntegration()],
  tracesSampleRate: 0.1,
});
```

Browsers send errors straight to the server, so:

- set `ALLOWED_ORIGINS` to the sites that send them (`https://app.example.com`), or leave `*`;
- the ingest endpoint must be reachable from your users' browsers, even if the UI is limited with `ALLOWED_IPS`.

A DSN key is public by design (it is in the page); it only allows sending events to its project.

Ad blockers often block requests to error trackers. The SDK's `tunnel` option sends them through your own backend instead; forward the request body unchanged to `https://errors.example.com/api/<project id>/envelope/`, and the server reads the key from the envelope.

Production bundles are minified; source maps are not supported yet, so frames show the bundle's names.

## Other platforms

Python:

```python
import sentry_sdk

sentry_sdk.init(dsn="https://<public_key>@errors.example.com/3", enable_logs=True)
```

Go, PHP and Ruby: see the project's installation code. Java, .NET and the rest: follow the SDK's own guide with the DSN from the Projects page.

## What is stored

- **Events** (errors and messages) of every level, grouped into **issues**. Notifications send only the levels each connection chooses (`fatal` and `error` for a new one). To keep noise out of the SDK itself: `beforeSend`/`ignoreErrors` in JavaScript, `before_send` in Python, `error_types` in PHP (whose SDK reports warnings, notices and deprecations as events by default).
- **Logs** of every level by default (`RECORD_LOG_LEVELS`). A log's `level` decides (`warning` is `warn`, `critical` `fatal`); one without a known level takes it from its `severity_number` by the OpenTelemetry ranges (1–4 trace, 5–8 debug, 9–12 info, 13–16 warn, 17–20 error, 21–24 fatal), and is `info` without either.
- Tracing spans, transactions, session replays, profiles and attachments are accepted and dropped. Client reports are only counted (below).

An event or log keeps the time it happened by the device's clock. When that clock is off by a minute or more (the SDK's `sent_at` against the time the server got the envelope), the times of that envelope are corrected and the event's page says by how much. Log batches without `sent_at` (the JavaScript SDK's) keep the device's time. A time more than 30 days before the server got it, or more than a minute after (after that correction), is taken as wrong (milliseconds instead of seconds, a broken clock) and replaced by the time the server got it. Issues list when the server received their events.

Before an event or log is stored, values under names like password, token, secret, session, authorization, api key, PIN, one-time code or card number (also the `number` of a `card` object, and password in other languages: `пароль`, `Passwort`, `contraseña`, `mot_de_passe`, …) are removed whole (names are read as words, so `author`, `session_count` or `className` keep their values; `APP_KEY`-style names count, `public_key` or `cache_key` do not), cookies one by one (all but `theme`, `lang`, `language`, `locale`, `timezone` and `tz`), also inside JSON sent as text (a request body, a message, a log line) and in Python dict reprs (`{'password': '…'}`), as are credentials in headers and URLs (such parameters in a query or fragment, `#access_token=…` of OAuth sign-ins included, and the password in `https://user:password@host`), also in URLs inside messages and log lines. In source lines of stack frames only a string written after such a name goes (`API_KEY = "…"`); `token = sign(key)` stays. Large parts that do not help a fix (library source code, module lists, more than 50 breadcrumbs) are dropped too, and very long log lines and events are cut ([Size limits](configuration.md#size-limits)). Until then a received envelope waits in the ingest queue as it was sent, usually for milliseconds; the queue's part of the database files is compacted every 10 minutes when ingest is quiet (and merged anyway under steady traffic), so the files do not keep it for long.

## Rate limits

Each project may send up to `RATE_LIMIT_EVENTS_PER_MINUTE` events (10000) and `RATE_LIMIT_LOGS_PER_MINUTE` logs (10000) a minute, counted apart, so a loop that writes logs does not stop its errors. An error in a loop does not need the limit: its issue stores 10 events a minute and only counts the rest, so other errors are still stored ([Retention](configuration.md#retention)), as long as the project sends fewer events than its limit. Over it, the SDKs of the project pause all its events until the minute ends, other errors included. Logs are counted one by one, not by batch, as they arrive: logs of levels the project does not record count too, so a noisy debug log uses up the limit of the levels it does record; filter it in the SDK (`beforeSendLog` in JavaScript, `before_send_log` in Python). Envelopes of spans, sessions and client reports count for nothing. A batch that would go over the limit is dropped whole, and the kind stays limited until the minute ends, so a log limit below the SDK's batch (up to 100 logs in JavaScript) drops every full batch. The server answers as Sentry does:

- What is still within its limit is stored, and the answer is `200` with `X-Sentry-Rate-Limits: 46:log_item:project` (or `46:error;default;security:project` for events): the SDK stops sending only that kind for the seconds given, then sends again. What it drops meanwhile it never sends.
- An envelope with nothing left to store is answered `429` with the same header and `Retry-After`; the SDK does not retry it.
- SDKs that read only `Retry-After` (on a `429`) pause everything for that time.

The server logs a warning once a minute for each project and kind that is limited. When the database is busy (its ingest queue full, too many bodies being read, or a compaction part running for a few seconds, see [Retention](configuration.md#retention)) every SDK gets `429` with `X-Sentry-Rate-Limits: 10::project` (empty categories: everything) and retries later.

The **Projects** page shows under a project what it sent in the last 24 hours (by the hour) that was not stored, by kind and reason: events and logs over the rate limit or refused while the server was busy, envelopes too large (`413`), unreadable (not an envelope, or a body that cannot be decoded) or not received whole (not within 10 s, or the client hung up); and from the SDK's client reports, events and logs it held back while a rate limit paused it or dropped because its queue was full. What the app drops on purpose (sampling, `beforeSend`) is not counted, nor what the SDK reports as failed sends: SDKs report the server's refusals above that way too (sentry-python every answer but `2xx` and `429`), so each would count twice, and network failures the server never saw are not counted. Only requests with the project's key count. The counts live in memory, so a restart starts them from zero, and the server logs a warning at a project's first drop in each hour.

## Grouping

Events with the same fingerprint in a project are one issue. The default fingerprint is built from the exception types and the in-app stack frames (all frames when none is in-app): file, function and source line, without line numbers, so a deploy that moves code does not open new issues; only an anonymous function without source code counts by its line number. Files under `node_modules/` never count as in-app, even when a browser SDK marks them so (a dev server such as Vite serves libraries from the site's own origin), and only the frames after the last async boundary count (Firefox appends the path that scheduled a callback, e.g. `promise callback*queueFlush`), so the same error reached by different clicks stays one issue. File paths drop the origin, the query string and bundle hashes (webpack and Next.js hex hashes, Vite's `index-BbtQ7Ui0.js`, SvelteKit's and Astro's `Counter.BkGx5dAS.js`), so a new deploy does not open new issues; such a hash without a digit or an uppercase letter after its first character (about one chunk in 400) is kept, because it cannot be told from a word such as `use-settings.js` or `user.reducers.js`. Messages and errors without a usable stack are grouped by their text, with numbers and IDs ignored ("order 890" and "order 891" match); a message with a template (Python's `logger.error("Login failed for %s", name)`) groups by the template, so any value put into it matches. A source line over 120 characters (a minified bundle, or a line Node's SDK cut short) does not count as source code. When the frame that threw has no source line that counts (browser SDKs send none), the error's text counts as well, so one place that throws different messages (`reading 'id'`, `reading 'name'`) makes separate issues. In JavaScript (platform `javascript` or `node`, or a `.js`, `.ts`, `.vue`, … file) a function counts by its last name: `Object.onClick` and `Module.onClick` are `onClick`, `new Cart` is `Cart`. An exception group (Python's `ExceptionGroup`, `AggregateError`) counts each kind of error in it once, in any order, so a group of three `ValueError`s is the issue of one of four; a top-level group of one kind is the issue of that error (a group nested in another keeps its own type).

To group differently, set the fingerprint in the SDK:

```js
Sentry.withScope((scope) => {
  scope.setFingerprint(["payment-gateway-timeout"]);          // all such errors in one issue
  // scope.setFingerprint(["{{ default }}", tenantId]);       // the default, split per tenant
  Sentry.captureException(error);
});
```

A fingerprint value may be one of the variables of Sentry's [Fingerprint Rules](https://docs.sentry.io/concepts/data-management/event-grouping/fingerprint-rules/#variables), filled in as Sentry fills them. Only a whole value is a variable: `"tenant-{{ tags.tenant }}"` stays as written.

| Variable | Value |
|---|---|
| `{{ default }}` | the default fingerprint |
| `{{ error.type }}`, `{{ type }}` | the type of the outermost error (the last one thrown) |
| `{{ error.value }}`, `{{ value }}` | its text, numbers and IDs ignored |
| `{{ message }}` | the message, or the error's text when there is none, numbers and IDs ignored |
| `{{ transaction }}`, `{{ level }}`, `{{ logger }}` | the event's field (`level` as stored: `warn` is `warning`, none is `error`) |
| `{{ stack.function }}`, `{{ function }}` | of the crashing frame: the newest in-app frame of the outermost error, else its newest frame |
| `{{ stack.abs_path }}`, `{{ path }}` | its `abs_path`, else `filename` |
| `{{ stack.filename }}` | its `filename`, else `abs_path` |
| `{{ stack.module }}`, `{{ module }}` | its `module` |
| `{{ stack.package }}`, `{{ package }}` | the last part of its `package` path (`libssl.so`) |
| `{{ tags.NAME }}` | the tag, with what Sentry sets before grouping: `environment`, `logger`, `transaction` and `server_name` from the event's fields when it has them, `level` always from the field, `sentry:release`, `sentry:dist` and `sentry:user` (`id:…`, `username:…`, `email:…` or `ip:…`); tags named `release`, `dist`, `user`, `filename` or `function` are not kept, so use `{{ tags.sentry:release }}` |

A variable with no value becomes `<no-type>`, `<no-function>` and so on (`<no-value-for-tag-NAME>` for a tag), so events without it group together. A plain value that is the event's message or error text is grouped with numbers and IDs ignored, as `{{ message }}` is. Any other `{{ … }}` stays as written.

A resolved issue that happens again is reopened (a regression) and notifies again. An ignored one stays ignored, for good or until the time or the number of events more chosen when ignoring: then it is unresolved again, and the event that ends it (the first after the time, or the last one counted) notifies as a regression. Events counted over 10 a minute without being stored count too.

## Logs and errors

On an issue page, the logs written while handling the failed request are shown with the error. They are matched by the trace and span IDs the SDK attaches when tracing is on; without them the page falls back to logs of the same trace from the minute around the error. The trace ID on the issue page opens every log of that trace on the **Logs** page.
