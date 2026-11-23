#!/usr/bin/env bash
# `make dev`: everything at http://localhost:$PORT (8000 by default) in one terminal. Vite serves
# the web app there with hot reload and sends the Go server's own addresses (API, ingest, MCP,
# OAuth, health) to the server, which listens on the next port. Ctrl+C stops both; when one of
# them exits, the other is stopped too. After a change in Go, run it again.
set -u

set -a
[ -f .env ] && . ./.env
[ -f .env.local ] && . ./.env.local
set +a

port=${PORT:-8000}
server_port=$((port + 1))

go build -o bin/exceptic-dev ./cmd/exceptic || exit 1

PORT=$server_port bin/exceptic-dev &
server=$!
(cd web && EXCEPTIC_URL="http://localhost:$server_port" exec pnpm dev --port "$port" --strictPort) &
app=$!

trap 'kill "$server" "$app" 2>/dev/null; wait 2>/dev/null' EXIT INT TERM

wait -n "$server" "$app"
