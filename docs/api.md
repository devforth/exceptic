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
- Free text (titles, log lines) is returned as stored; the MCP tools return it redacted, since it goes to a model provider ([What leaves the server](mcp.md#what-leaves-the-server)). `get_issue` and `get_event` return that redacted prompt to every caller; `get_event_payload` has the event as stored.

## Endpoints

<!-- endpoints: generated from internal/api by make api-types -->
| Endpoint | Input (required first) | Output | |
|---|---|---|---|
| `GET list_issues` | `cursor`, `date_by`, `level`, `limit`, `project`, `query`, `since`, `sort`, `status`, `until` | `items` [`id`, `project`, `title`, `location`, `level`, `status`, `event_count`, `not_stored`, `stored_events`, `first_seen`, `last_seen`, `resolved_at`, `ignored_until`, `ignore_events_left`, `mcp_reference`], `next_cursor` | List issues (errors grouped by cause) with event count and first/last seen. Default: unresolved, most recently seen first, every project. The way to find errors without known IDs. |
| `GET get_issue` | `issue_id`, `event_id` | text | Diagnose an issue: its latest event (or event_id) as the agent prompt of notifications, with the exception chain, stack frames with source, message, request, runtime, tags, breadcrumbs, up to 20 logs of the request and the issue history. |
| `GET get_issues` | `issue_ids` | `issues` [`id`, `project`, `title`, `location`, `level`, `status`, `event_count`, `not_stored`, `stored_events`, `first_seen`, `last_seen`, `resolved_at`, `ignored_until`, `ignore_events_left`, `mcp_reference`], `missing_issue_ids`, `forbidden_issue_ids` | Summaries and current statuses of 1 to 100 known issue IDs in one call, without event details. |
| `GET list_issue_events` | `issue_id`, `cursor`, `limit` | `items` [`event_id`, `timestamp`, `level`, `environment`, `release`, `trace_id`, `received_at`], `next_cursor` | List the stored events of an issue, newest first: time, level, environment, release and trace ID. Pass an event_id to get_issue to read one. |
| `GET get_event` | `event_id`, `project` | text | Like get_issue, for an event ID the SDK reported to the application (with its project). |
| `GET list_logs` | `cursor`, `from_id`, `level`, `limit`, `offset`, `project`, `query`, `since`, `trace_id`, `until` | `items` [`id`, `time`, `project`, `level`, `body`, `trace_id`, `issue`], `next_cursor`, `stopped`, `searched_to` | Search logs, newest first. Pass trace_id (from get_issue) for the logs of one request or browser session. A search stops after 20s: then stopped is true, searched_to says how far back it got and next_cursor goes on from there; narrow it (since/until, project, level) rather than paging far. |
| `GET list_projects` | `cursor`, `limit` | `items` [`id`, `name`, `dsn`, `from_env`, `log_levels`, `unresolved_issues`], `next_cursor` | List the monitored projects with their unresolved issue counts and DSNs (what a Sentry SDK is configured with to report to the project). Not needed before list_issues: issue rows name their project. |
| `POST update_issue_status` | `issue_ids`, `status`, `ignore_for`, `ignore_until_events` | `issues` [`id`, `project`, `title`, `location`, `level`, `status`, `event_count`, `not_stored`, `stored_events`, `first_seen`, `last_seen`, `resolved_at`, `ignored_until`, `ignore_events_left`, `mcp_reference`], `missing_issue_ids`, `forbidden_issue_ids` | Set the status of 1 to 100 issues in one atomic call: resolved (fixed, reopens if the error happens again), ignored (no notifications; for good, or until the time or number of events chosen when ignoring) or unresolved (still happening or not looked at yet). Only when the user asks. Returns the updated summaries. |
| `POST delete_issue` | `issue_id` | `deleted` | Delete an issue with its events; if the error happens again it comes back as a new issue. Admins only. |
| `GET get_event_payload` | `event_id`, `project` | `project_id`, `issue_id`, `event_id`, `timestamp`, `received_at`, `level`, `environment`, `release`, `trace_id`, `span_id`, `parent_span_id`, `thinned`, `payload` | An event with its JSON as stored: credentials and parts that do not help a fix removed; thinned marks an old event whose details were removed. |
| `GET list_trace` | `trace_id` | `trace_id`, `events` [`project_id`, `issue_id`, `event_id`, `timestamp`, `received_at`, `level`, `environment`, `release`, `trace_id`, `span_id`, `parent_span_id`, `thinned`], `logs` [`id`, `project_id`, `timestamp`, `level`, `severity_number`, `body`, `trace_id`, `span_id`, `attributes`], `stopped`, `searched_to` | Events (without payloads) and logs of one trace, up to 100 of each. |
<!-- /endpoints -->

### list_issues input

<!-- list-issues-input: generated from internal/api by make api-types -->
| Parameter | |
|---|---|
| `project` | Project names or IDs; every project when empty. |
| `status` | Issue status, unresolved by default; all for every status. One of `unresolved`, `resolved`, `ignored`, `all`. |
| `level` | Issue levels; every level when empty. Any of `debug`, `info`, `warning`, `error`, `fatal`. |
| `query` | Text in the issue title or location. |
| `since` | RFC 3339 time, e.g. 2026-11-11T10:00:00Z: only issues with an event at or after it. |
| `until` | RFC 3339 time: only issues with an event before it. |
| `date_by` | What since/until apply to; by default any event of the issue, its first or last seen time included. One of `event`, `first_seen`, `last_seen`. |
| `sort` | Order, last_seen by default: a field newest or most first, with _asc the reverse; project A to Z, project_desc the reverse. One of `project`, `project_desc`, `last_seen`, `last_seen_asc`, `first_seen`, `first_seen_asc`, `event_count`, `event_count_asc`. |
| `limit` | Issues per page. At most 100, 20 by default. |
| `cursor` | next_cursor of the previous page. |
<!-- /list-issues-input -->

## Example: resolve every unresolved issue of a project

```sh
api="https://errors.example.com/api/v1"
auth="Authorization: Bearer $API_TOKEN"

# Resolved issues leave the list, so the first page is always the next 100.
while ids=$(curl -fsS -H "$auth" "$api/list_issues?project=backend&limit=100" | jq -c '[.items[].id]') && [ "$ids" != "[]" ]; do
  curl -fsS -X POST -H "$auth" -d "{\"issue_ids\":$ids,\"status\":\"resolved\"}" "$api/update_issue_status" > /dev/null || break
done
```
