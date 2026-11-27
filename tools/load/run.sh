#!/bin/sh
# Loads a Docker image of the server through the real @sentry/node SDK, starting from a copy of a
# database, and prints what the SDKs got and what the server logged. Nothing is sent elsewhere:
# the container listens on 127.0.0.1 only.
#
#   tools/load/run.sh <cpus> <memory> <seconds> [database dir]
#   tools/load/run.sh 1 512m 120
#   DSN=http://<key>@127.0.0.1:18000/<id> tools/load/run.sh 1 512m 120 data/flood-run4-start
#
# Environment: IMAGE (default exceptic:load, built from this checkout when missing), WORKERS (8 SDK
# processes), MAX_DATABASE_SIZE (1500MB), LIMITS=off (no per-project rate limits), PORT (18000),
# SENTRY_NODE_DIR (a directory whose node_modules has @sentry/node; default ../sentry-playground),
# DSN (a project of the database dir, required with one; without a database dir the script sets up
# an admin and adds a project). Needs curl and jq.
# Results go to data/load-runs/<time>-<cpus>-<memory>/.
set -eu

[ $# -ge 3 ] || { echo "usage: tools/load/run.sh <cpus> <memory> <seconds> [database dir]" >&2; exit 2; }

cpus=$1 memory=$2 seconds=$3 source=${4:-}
dsn=${DSN:-}
[ -z "$source" ] || [ -n "$dsn" ] || {
	echo "DSN is required with a database dir: the DSN of one of its projects, with host 127.0.0.1:${PORT:-18000}" >&2
	exit 2
}
cd "$(dirname "$0")/../.."
image=${IMAGE:-exceptic:load}
port=${PORT:-18000}
workers=${WORKERS:-8}
out=data/load-runs/$(date +%Y%m%d-%H%M%S)-$cpus-$memory
data=$out/data
name=exceptic-load
mkdir -p "$data"

docker image inspect "$image" >/dev/null 2>&1 || docker build -q -t "$image" . >/dev/null
[ -n "$source" ] && cp -a "$source"/. "$data"/

limits=""
[ "${LIMITS:-on}" = off ] && limits="-e RATE_LIMIT_EVENTS_PER_MINUTE=0 -e RATE_LIMIT_LOGS_PER_MINUTE=0"

docker rm -f "$name" >/dev/null 2>&1 || true
# shellcheck disable=SC2086 # limits are several words or none
docker run -d --name "$name" --user "$(id -u):$(id -g)" -v "$PWD/$data:/data" \
	--cpus "$cpus" --memory "$memory" --memory-swap "$memory" -p "127.0.0.1:$port:8000" \
	-e LOG_LEVEL=debug -e LOG_FORMAT=json \
	-e ADMIN_EMAIL=admin -e ADMIN_PASSWORD=load-password-123 -e MAX_DATABASE_SIZE="${MAX_DATABASE_SIZE:-1500MB}" $limits \
	"$image" >/dev/null

until curl -fs "http://127.0.0.1:$port/health" >/dev/null; do sleep 0.5; done

# post <request> <json>: a request of the web app's API, as its page sends it.
post() {
	curl -fsS -b "$out/cookies" -c "$out/cookies" -H 'Content-Type: application/json' \
		-H "Origin: http://127.0.0.1:$port" -d "$2" "http://127.0.0.1:$port/api/ui/$1"
}

# fail <message>: stops the container and the script.
fail() {
	echo "$1" >&2
	docker rm -f "$name" >/dev/null 2>&1 || true
	exit 1
}

if [ -z "$dsn" ]; then
	# The env login opens the first admin's setup, which starts a session.
	post login '{"email":"admin","password":"load-password-123"}' >/dev/null || fail "the env login failed"
	post setup_admin '{"email":"load@example.com","password":"load-admin-password","confirm_password":"load-admin-password"}' >/dev/null ||
		fail "the first admin was not set up"
	dsn=$(post create_project '{"name":"loadtest"}' | jq -r '.dsn // empty') || true
	[ -n "$dsn" ] || fail "the project was not added"
fi

echo "server up; $workers SDK processes for ${seconds}s"

(while docker inspect -f '{{.State.Running}}' "$name" 2>/dev/null | grep -q true; do
	docker stats --no-stream --format '{{.CPUPerc}} {{.MemUsage}}' "$name"
done) >"$out/stats" &
sampler=$!
pids=""

for worker in $(seq 0 $((workers - 1))); do
	node tools/load/sdk-load.mjs "$dsn" "$seconds" "$worker" >"$out/sdk$worker" 2>&1 &
	pids="$pids $!"
done

# shellcheck disable=SC2086
wait $pids
sleep 20
docker logs "$name" >"$out/server.log" 2>&1
docker rm -f "$name" >/dev/null
kill "$sampler" 2>/dev/null || true
rm -rf "$data"

cat "$out"/sdk* | node -e '
let input = ""; process.stdin.on("data", (c) => (input += c)).on("end", () => {
	const status = {}, refused = {};
	let captured = 0;
	for (const line of input.split("\n")) {
		let run; try { run = JSON.parse(line); } catch { continue; }
		captured += run.captured;
		for (const [k, v] of Object.entries(run.status)) status[k] = (status[k] ?? 0) + v;
		for (const [k, v] of Object.entries(run.refused)) refused[k] = (refused[k] ?? 0) + v;
	}
	console.log("captured", captured, "answers", status, "(none: dropped by the SDK itself)");
	for (const [k, v] of Object.entries(refused).sort((a, b) => b[1] - a[1]).slice(0, 8)) console.log("  429", v, k);
});'
grep -o '"msg":"cleanup[^,]*' "$out/server.log" | sort | uniq -c
awk '{ split($2, m, "MiB"); if (m[1] + 0 > max) max = m[1] + 0 } END { print "memory peak", max, "MiB" }' "$out/stats"
echo "files in $out"
