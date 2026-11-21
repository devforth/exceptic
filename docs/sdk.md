# Connecting apps

The server speaks the protocol of the official [Sentry SDKs](https://docs.sentry.io/platforms/), so an app is connected by installing the SDK of its platform and giving it a DSN. Nothing else changes in the app.

Each service (backend, frontend, worker, …) should be its own project: the UI shows which project an error came from, and notifications and levels can be set per project. Copy the DSN from the **Projects** page (a coding agent connected over [MCP](mcp.md) gets it from `list_projects`):

```
https://<public_key>@errors.example.com/<project id>
```

The project's **Show installation code** (⋯ on the Projects page) shows the install command, the setup code with the DSN filled in and a test line for Node.js, browser, Python, Go, PHP and Ruby, and copies the same as instructions for a coding agent.

Tested with the Node.js and browser SDKs, version 11. Other SDKs send the same envelope format and should work; please report one that does not.

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

The SDK's `Dedupe` integration drops an error identical to the previous one, however much later it happens, so repeats are not counted. The server groups repeats into one issue and has its own rate limit, so you may turn it off with this `integrations` in place of the list above:

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

- **Events** (errors and messages) of the levels the project records, by default `error` and `fatal`; set `RECORD_EVENT_LEVELS` or the project's levels to keep warnings or messages too. Events are grouped into **issues**.
- **Logs** of every level by default (`RECORD_LOG_LEVELS`).
- Tracing spans, transactions, session replays, profiles, attachments and client reports are accepted and dropped.

An event or log keeps the time it happened by the device's clock. When that clock is off by a minute or more (the SDK's `sent_at` against the time the server got the envelope), the times of that envelope are corrected and the event's page says by how much. Log batches without `sent_at` (the JavaScript SDK's) keep the device's time. A time more than 30 days before the server got it, or more than a minute after (after that correction), is taken as wrong (milliseconds instead of seconds, a broken clock) and replaced by the time the server got it. Issues list when the server received their events.

Before an event or log is stored, values under names like password, token, secret, cookie, session or authorization are removed, as are credentials in headers and URLs (such parameters in a query or fragment, `#access_token=…` of OAuth sign-ins included, and the password in `https://user:password@host`), also in URLs inside messages and log lines. Large parts that do not help a fix (library source code, module lists, more than 50 breadcrumbs) are dropped too. Until then a received envelope waits in the ingest queue as it was sent, usually for milliseconds; the queue's part of the database files is compacted every 10 minutes when ingest is quiet (and merged anyway under steady traffic), so the files do not keep it for long.

## Grouping

Events with the same fingerprint in a project are one issue. The default fingerprint is built from the exception types and the in-app stack frames (all frames when none is in-app): file, function and source line, without line numbers, so a deploy that moves code does not open new issues; only an anonymous function without source code counts by its line number. Files under `node_modules/` never count as in-app, even when a browser SDK marks them so (a dev server such as Vite serves libraries from the site's own origin), and only the frames after the last async boundary count (Firefox appends the path that scheduled a callback, e.g. `promise callback*queueFlush`), so the same error reached by different clicks stays one issue. File paths drop the origin, the query string and bundle hashes (webpack and Next.js hex hashes, Vite's `index-BbtQ7Ui0.js`, SvelteKit's and Astro's `Counter.BkGx5dAS.js`), so a new deploy does not open new issues; such a hash without a digit or an uppercase letter after its first character (about one chunk in 400) is kept, because it cannot be told from a word such as `use-settings.js` or `user.reducers.js`. Messages and errors without a usable stack are grouped by their text, with numbers and IDs ignored ("order 890" and "order 891" match). When no frame has source code (browser SDKs send none), the error's text counts as well, so one place that throws different messages (`reading 'id'`, `reading 'name'`) makes separate issues.

To group differently, set the fingerprint in the SDK:

```js
Sentry.withScope((scope) => {
  scope.setFingerprint(["payment-gateway-timeout"]);          // all such errors in one issue
  // scope.setFingerprint(["{{ default }}", tenantId]);       // the default, split per tenant
  Sentry.captureException(error);
});
```

A resolved issue that happens again is reopened (a regression) and notifies again; an ignored one stays ignored.

## Logs and errors

On an issue page, the logs written while handling the failed request are shown with the error. They are matched by the trace and span IDs the SDK attaches when tracing is on; without them the page falls back to logs of the same trace from the minute around the error. The trace ID on the issue page opens every log of that trace on the **Logs** page.
