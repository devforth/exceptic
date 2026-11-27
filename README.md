# Exceptic

A lightweight, self-hosted service for monitoring application errors and logs.

Your apps send errors and logs with the official Sentry SDKs: just give the SDK a DSN. It runs in one container on your own server, so your data stays with you.

## Features

- **Errors and logs** from any app with a Sentry SDK (Node.js, browsers, Python, …), many projects in one server.
- **Issues:** repeated errors are grouped; resolve, ignore, and see when an error comes back.
- **Logs** with search, linked to the error of the same request.
- **Notifications** by email, Telegram and Slack.
- **Users and roles**, each user limited to their projects.
- **MCP server** for coding agents (Claude Code, Codex, …) and a **REST API**.
- **Keeps its size:** under `MAX_DATABASE_SIZE` the least useful data is deleted first; passwords and tokens are removed from incoming data.
- **Small:** one container of about 20 MB.

## Quick start

<!-- docker-run -->
```sh
docker run -d --name exceptic -p 8000:8000 -v exceptic-data:/data \
  -e ADMIN_PASSWORD='<at least 12 characters>' \
  devforth/exceptic:latest
```

Open http://localhost:8000 and log in as `admin` with that password, then set up the first admin with your email and a password of your own. On the **Projects** page add a project and copy its DSN.
<!-- /docker-run -->

For a server use [compose.yml](compose.yml) behind HTTPS: [Deployment](docs/deployment.md).

## Connect an app

Install the Sentry SDK of your platform and give it the project's DSN, e.g. for Node.js:

```js
import * as Sentry from "@sentry/node";

Sentry.init({
  dsn: "https://<public_key>@errors.example.com/1",
  enableLogs: true,
});
```

Other platforms: [Connecting apps](docs/sdk.md).

## Agent skill

[skills/exceptic](skills/exceptic/SKILL.md) lets a coding agent deploy the server, connect to it, add the Sentry SDK to your apps and investigate errors:

```sh
npx skills add devforth/exceptic -g
```

Then ask, e.g., "connect this app to our error monitor".

## Documentation

- [Deployment](docs/deployment.md): Docker, HTTPS, backups, upgrades.
- [Configuration](docs/configuration.md): every setting.
- [Settings in the web UI](docs/ui.md).
- [Connecting apps](docs/sdk.md).
- [Coding agents (MCP)](docs/mcp.md).
- [REST API](docs/api.md).

## Development

Needs Go (see `go.mod`), [golangci-lint](https://golangci-lint.run) v2, and for the web app (`web/`, SvelteKit) Node 24 with pnpm (`corepack enable`).

```sh
cp .env.example .env.local   # works as is: log in as admin / admin-password to set up the first admin
make run                     # http://localhost:8000
make dev                     # server and web app with hot reload, both at http://localhost:8000 (Ctrl+C stops both)
make test lint web-check     # what CI runs
make api-types               # after changing the catalog or a value the docs state: the generated types and doc parts
make docker image-size       # build the image and check its size
```

Notes on the Sentry envelope format: [docs/sentry-protocol.md](docs/sentry-protocol.md).

`make release` publishes the next version (from the `feat:`/`fix:` commits since the last tag, or `VERSION=X.Y.Z`) of the image `devforth/exceptic` for amd64 and arm64 from `main` once its CI passed, with this machine's `docker login`, and tags the commit `vX.Y.Z` ([tools/release.sh](tools/release.sh)).

## License

[MIT](LICENSE) © Devforth.io
