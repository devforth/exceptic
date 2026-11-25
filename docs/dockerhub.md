# Exceptic

A lightweight, self-hosted service for monitoring application errors and logs. Your apps send them with the official Sentry SDKs: point an SDK at a project's DSN, no other client library is needed.

Source and full documentation: https://github.com/devforth/exceptic

## Run it

```sh
docker run -d --name exceptic -p 8000:8000 -v exceptic-data:/data \
  -e ADMIN_PASSWORD='<at least 12 characters>' \
  devforth/exceptic:latest
```

Open http://localhost:8000, log in as `admin` with that password, add a project on the **Projects** page and give its DSN to your app.

For a server use Docker Compose behind HTTPS: [compose.yml](https://github.com/devforth/exceptic/blob/main/compose.yml) with an `.env` next to it:

```sh
PUBLIC_URL=https://errors.example.com
ADMIN_PASSWORD=<at least 12 characters>
SECRET_KEY=<openssl rand -hex 32>
```

## Tags

`latest` (newest release), `X.Y` (newest patch of a minor version), `X.Y.Z`, for amd64 and arm64. Pin `X.Y` or `X.Y.Z` in production.

## Documentation

- [Deployment](https://github.com/devforth/exceptic/blob/main/docs/deployment.md): Docker Compose, reverse proxy and HTTPS, backups, upgrades.
- [Configuration](https://github.com/devforth/exceptic/blob/main/docs/configuration.md): every environment variable.
- [Connecting apps](https://github.com/devforth/exceptic/blob/main/docs/sdk.md): Sentry SDK setup per platform.
- [MCP for coding agents](https://github.com/devforth/exceptic/blob/main/docs/mcp.md) and the [REST API](https://github.com/devforth/exceptic/blob/main/docs/api.md).
