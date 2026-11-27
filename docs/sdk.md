# Connecting apps

Apps send errors and logs with the official [Sentry SDKs](https://docs.sentry.io/platforms/). Install the SDK of your platform and give it the project's DSN.

Make one project per service (backend, frontend, worker, …). Copy the DSN from the **Projects** page:

```
https://<key>@errors.example.com/<project id>
```

**Show installation code** in the project's menu (⋯) gives ready code with the DSN for Node.js, browser, Python, Go, PHP and Ruby, and shows when the app has connected.

## Node.js

```sh
npm install @sentry/node
```

Load it before anything else (e.g. `node --import ./instrument.mjs`):

```js
import * as Sentry from "@sentry/node";

Sentry.init({
  dsn: process.env.SENTRY_DSN,
  environment: process.env.NODE_ENV,
  enableLogs: true,          // send logs: Sentry.logger.info(...)
  tracesSampleRate: 0.1,     // ties logs to the request that failed
  integrations: [
    Sentry.consoleLoggingIntegration({ levels: ["log", "warn", "error"] }),
  ],
});
```

Unhandled errors are sent on their own. Others: `Sentry.captureException(error)`, messages: `Sentry.captureMessage("text", "warning")`.

## Browser

```sh
npm install @sentry/browser
```

```js
import * as Sentry from "@sentry/browser";

Sentry.init({
  dsn: "https://<key>@errors.example.com/2",
  enableLogs: true,
  integrations: [Sentry.browserTracingIntegration()],
  tracesSampleRate: 0.1,
});
```

- Add your site to the project's **Allowed domains** (`https://app.example.com`), or leave it empty for any site.
- The server must be reachable from your users' browsers.
- The DSN key is safe to put in the page: it only allows sending errors to its project.
- Ad blockers may block the requests. The SDK's `tunnel` option sends them through your own backend; forward them unchanged to `https://errors.example.com/api/<project id>/envelope/`.

Source maps are not supported yet, so minified code shows minified names.

## Other platforms

Python:

```python
import sentry_sdk

sentry_sdk.init(dsn="https://<key>@errors.example.com/3", enable_logs=True)
```

Go, PHP and Ruby: the project's installation code. Others: the SDK's own guide with your DSN.

## What is stored

- **Errors and messages** of every level, grouped into issues. Notifications send only the levels each connection chooses (`fatal` and `error` for a new one).
- **Logs** of the levels the project records (all by default).
- Tracing spans, replays, profiles and attachments are dropped.

Passwords, tokens, cookies, card numbers and similar values are removed before anything is stored. Library source code and long lists of breadcrumbs (more than 50 breadcrumbs) are dropped; very long values are cut ([Size limits](configuration.md#size-limits)).

## Rate limits

Each project may send up to `RATE_LIMIT_EVENTS_PER_MINUTE` errors and `RATE_LIMIT_LOGS_PER_MINUTE` logs a minute ([Configuration](configuration.md#ingest)). Over the limit the app pauses sending that kind until the minute ends.

An error repeating in a loop does not need the limit: its issue stores 10 events a minute and only counts the rest.

The **Projects** page shows what each project sent in the last 24 hours that was not stored, and why.

## Grouping

Errors with the same type and the same place in your code are one issue. Line numbers do not count, so a deploy that moves code opens no new issues. Messages group by their text with numbers ignored ("order 890" and "order 891" are one issue).

To group differently, set a fingerprint:

```js
Sentry.withScope((scope) => {
  scope.setFingerprint(["payment-gateway-timeout"]);          // all such errors in one issue
  // scope.setFingerprint(["{{ default }}", tenantId]);       // the usual grouping, split per tenant
  Sentry.captureException(error);
});
```

A value may be one of these variables (as in Sentry):

| Variable | Value |
|---|---|
| `{{ default }}` | the usual grouping |
| `{{ error.type }}`, `{{ type }}` | the error's type |
| `{{ error.value }}`, `{{ value }}` | the error's text, numbers ignored |
| `{{ message }}` | the message, numbers ignored |
| `{{ transaction }}`, `{{ level }}`, `{{ logger }}` | the event's field |
| `{{ stack.function }}`, `{{ function }}` | the function where it failed |
| `{{ stack.abs_path }}`, `{{ path }}` | its file path |
| `{{ stack.filename }}` | its file name |
| `{{ stack.module }}`, `{{ module }}` | its module |
| `{{ stack.package }}`, `{{ package }}` | its package |
| `{{ tags.NAME }}` | a tag, e.g. `{{ tags.tenant }}` |

A resolved issue that happens again is reopened and notifies again. An ignored issue stays ignored until the time or number of events you chose.

## Logs and errors

The issue page shows the logs written during the failed request, when tracing is on (`tracesSampleRate`). The trace ID there opens all logs of that request on the **Logs** page.
