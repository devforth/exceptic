# REST API

A JSON API under `/api/v1/` for scripts and integrations. It is closed until `API_TOKEN` is set; every request sends the token, and `ALLOWED_IPS` applies:

```sh
curl -H "Authorization: Bearer $API_TOKEN" "https://errors.example.com/api/v1/list_issues?project=backend"
```

The API and the [MCP tools](mcp.md) are the same endpoints with the same names, inputs and outputs. A few endpoints are REST only (marked below); managing projects, notifications and users is for agents and the web interface only, since it acts as a user and the API has only a token. Status changes made through the API are recorded in the issue history as "API token". The token sees every project, whatever projects users are limited to, and runs every endpoint.

## Conventions

- Each endpoint is `/api/v1/<name>`. `/api/ui/` is the web interface's own API (the endpoints below that the web app uses and its own, such as `get_issue_details`, plus its login), bound to its login session and same-origin checks; it is not for scripts. Endpoints that only read are `GET` with the input as query parameters (`?project=backend&limit=50`; a list takes repeated or comma-separated values: `issue_ids=k3x9q2mf,a7c2d4e9`). Endpoints that change data are `POST` with the input as a JSON object.
- Errors: a status code with `{"error": "message"}`:
  - 400: invalid input, unknown parameters too.
  - 401: no valid token.
  - 403: an address outside `ALLOWED_IPS`. (The web interface and MCP also answer 403 for a record of a project outside the user's projects and for what the user's role may not do.)
  - 404: a record that does not exist (`not found: issue k3x9q2mf`, `update_issue_status` with any such ID too), an unknown project, an unknown endpoint.
  - 405: a known endpoint with the other method, with `Allow`.
  - 413: a body over 64 KiB.
  - 500: a server failure (logged; the answer says only `internal error`).
- Lists return `{"items": [...], "next_cursor": "..."}`. Pass `cursor=<next_cursor>` for the next page; there is no `next_cursor` on the last one. `limit` is 1–100, default 20.
- `project` takes project ids (`1`) or names (`backend`), `level` levels; each takes several values (`?project=backend&project=frontend`), and none means every one.
- Issues have IDs of 8 random lowercase letters and digits (`k3x9q2mf`); a project keeps the number in its DSN.
- Times are RFC 3339 in UTC, e.g. `2026-11-11T10:00:00Z`.
- Free text (titles, log lines) is returned as stored; the MCP tools return it with IPs, hosts, emails and credentials replaced, since it goes to a model provider. `get_issue` and `get_event` return that redacted prompt to every caller; `get_event_payload` has the event as stored.

## Endpoints

| Endpoint | Input | |
|---|---|---|
| `GET list_projects` | `limit`, `cursor` | Projects: `id`, `name`, `unresolved_issues`, `dsn`, `from_env` (from `PROJECTS`: its key comes from there and it cannot be deleted), `log_levels` (recorded, highest first; events of every level are stored). |
| `GET list_issues` | see below | Issues: `id`, `project`, `title`, `location`, `level`, `status`, `event_count`, `not_stored` (of `event_count`, over 10 a minute; left out when 0), `stored_events` (stored now, what `list_issue_events` returns), `first_seen`, `last_seen`, `resolved_at`; an ignored one `ignored_until` and `ignore_events_left` when its ignore ends by itself. |
| `GET get_issues` | `issue_ids` (1–100) | `{"issues": [...], "missing_issue_ids": [...]}`; for an agent also `forbidden_issue_ids`, those of projects outside its user's. |
| `GET get_issue` | `issue_id`, `event_id` | The latest (or given) event of an issue as a text prompt for a coding agent: exception, stack, request, breadcrumbs, related logs, status, event count, first and last seen. |
| `GET list_issue_events` | `issue_id`, `limit`, `cursor` | Events of an issue, newest first: `event_id`, `timestamp`, `level`, `environment`, `release`, `trace_id`, `received_at`. |
| `GET get_event` | `project`, `event_id` | Like `get_issue`, for an event ID (32 hex characters, with or without dashes). |
| `GET list_logs` | `project`, `level`, `query` (text in the line), `trace_id`, `since`, `until` (by the SDK's time of the log; with `until`, logs stored more than about an hour after it are left out: sent from an offline cache or delayed by an ingest backlog), `limit`, `cursor` or `offset` (logs to skip, for numbered pages), `from_id` (only logs with this id or a greater one) | Logs, newest first: `id`, `time`, `project`, `level`, `body`, `trace_id`, `issue` (`id` and `title` of the error raised in the same request, when there is one). A search stops after 20 seconds: then `stopped` is true, `searched_to` is the time from which every log received was searched, and `next_cursor` goes on with the older logs (none when it stopped while skipping `offset`); narrowing by date, project or level searches further faster. |
| `POST update_issue_status` | `issue_ids` (1–100), `status`, `ignore_for`, `ignore_until_events` | Set `resolved`, `ignored` or `unresolved` on all of them or none; returns the updated issues. With `ignored` only: `ignore_for` (`1h`, `24h`, `7d`, `30d`) or `ignore_until_events` (1–1000000 events more) end the ignore, then the issue is unresolved again (both: whichever comes first); without them it stays ignored. Any later status change drops them. |
| `GET get_event_payload` | `project`, `event_id` | REST only. The event with its JSON as stored in `payload`: without credentials and the parts that do not help a fix ([What is stored](sdk.md#what-is-stored)); `thinned` marks an old event reduced as [Retention](configuration.md#retention) says. |
| `GET list_trace` | `trace_id` | REST only. Events (without payloads) and logs, with their attributes, of one trace, up to 100 of each; a log search that stops after 20 seconds says so with `stopped` and `searched_to`, as `list_logs` does. |
| `POST delete_issue` | `issue_id` | Delete the issue with its events: `{"deleted": "<id>"}` (agents: admins only). |

### list_issues input

| Parameter | |
|---|---|
| `project`, `level` | Filters. |
| `status` | `unresolved` (default), `resolved`, `ignored` or `all`. |
| `query` | Text in the title or location. |
| `since`, `until` | Time range, `since` inclusive, `until` exclusive. |
| `date_by` | What the range applies to: `event` (default: any event of the issue in the range, its first or last seen time included, so an issue whose events the cleanup deleted stays listed), `first_seen` or `last_seen`. |
| `sort` | `last_seen` (default), `first_seen`, `event_count`, each newest or most first, or with `_asc`; `project` (A–Z) or `project_desc`. |
| `limit`, `cursor` | Paging. |

## Example: resolve every unresolved issue of a project

```sh
api="https://errors.example.com/api/v1"
auth="Authorization: Bearer $API_TOKEN"

# Resolved issues leave the list, so the first page is always the next 100.
while ids=$(curl -fsS -H "$auth" "$api/list_issues?project=backend&limit=100" | jq -c '[.items[].id]') && [ "$ids" != "[]" ]; do
  curl -fsS -X POST -H "$auth" -d "{\"issue_ids\":$ids,\"status\":\"resolved\"}" "$api/update_issue_status" > /dev/null || break
done
```
