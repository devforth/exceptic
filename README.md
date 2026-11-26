# Exceptic

A lightweight, self-hosted service for monitoring application errors and logs.

It collects exceptions and logs from your backend and frontend apps through the official Sentry SDKs: you point an SDK at it with a DSN, no other client library is needed. It runs in one container on your own server, so your data never leaves your infrastructure.

## Features

- **Errors and logs** from any app with a Sentry SDK (Node.js, browsers, Python, …), several projects per instance.
- **Issues:** repeated errors are grouped, with resolve/ignore, regression detection and a history of who changed each status.
- **Logs** with search, linked to the error of the same request.
- **Notifications** about new and regressed issues by email, Telegram and Slack, with retries that survive restarts.
- **Web UI** with users and roles (superadmin, admin, user).
- **MCP server** for coding agents (Claude Code, Codex, …) with OAuth sign-in or a personal token.
- **REST API** for scripts and integrations.
- **Size-based cleanup**: under `MAX_DATABASE_SIZE`, logs and old repeated events go first, errors last; credentials are scrubbed from incoming data.
- **Small:** one static binary with an embedded database, an image of about 20 MB.

Tracing spans, session replays and other Sentry item types are accepted and dropped.

## Quick start

<!-- docker-run -->
```sh
docker run -d --name exceptic -p 8000:8000 -v exceptic-data:/data \
  -e ADMIN_PASSWORD='<at least 12 characters>' \
  devforth/exceptic:latest
```

Open http://localhost:8000 and log in as `admin` with that password. On the **Projects** page add a project and copy its DSN.
<!-- /docker-run -->

For a real deployment use [compose.yml](compose.yml) with an `.env` file and put the server behind HTTPS: see [Deployment](docs/deployment.md).

## Connect an app

Install the Sentry SDK of your platform and give it the project's DSN, e.g. for Node.js:

```js
import * as Sentry from "@sentry/node";

Sentry.init({
  dsn: "https://<public_key>@errors.example.com/1",
  enableLogs: true,
});
```

More platforms, browser apps and what is stored: [Connecting apps](docs/sdk.md).

## Agent skill

[skills/exceptic](skills/exceptic/SKILL.md) teaches a coding agent to deploy the server, connect itself over MCP, add the Sentry SDK to your apps and investigate errors. Install it for your user:

```sh
npx skills add devforth/exceptic -g
```

Or copy the folder to `~/.claude/skills/` (Claude Code) or `~/.agents/skills/` (Codex). Then ask, e.g., "connect this app to our error monitor".

## Documentation

- [Deployment](docs/deployment.md): Docker, reverse proxy and HTTPS, upgrades.
- [Configuration](docs/configuration.md): every environment variable.
- [Settings in the web UI](docs/ui.md): the fields of the Projects, Notifications and Users pages.
- [Connecting apps](docs/sdk.md): SDK setup, levels, logs, grouping.
- [Coding agents (MCP)](docs/mcp.md): connecting Claude Code, Codex and other clients.
- [REST API](docs/api.md).
- [Sentry protocol notes](docs/sentry-protocol.md): the envelope format as SDKs send it.

## Development

Needs Go (see `go.mod`), [golangci-lint](https://golangci-lint.run) v2, and for the web app (`web/`, SvelteKit) Node 24 with pnpm (`corepack enable`).

```sh
cp .env.example .env.local   # works as is: log in as admin / admin-password
make run                     # http://localhost:8000
make dev                     # server and web app with hot reload, both at http://localhost:8000 (Ctrl+C stops both)
make test lint web-check     # what CI runs
make api-types               # after changing the catalog or a value the docs state: the generated types and doc parts
make docker image-size       # build the image and check its size
```

`make release VERSION=X.Y.Z` publishes the image `devforth/exceptic` for amd64 and arm64 from `main` once its CI passed, with this machine's `docker login`, and tags the commit `vX.Y.Z` ([tools/release.sh](tools/release.sh)).

## License

[MIT](LICENSE) © Devforth.io
