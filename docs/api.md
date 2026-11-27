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

One table for the REST API and the MCP tools ([Coding agents](mcp.md)). REST runs every endpoint marked REST with the API token; MCP offers an agent the tools marked MCP that its user's role may use (Who) and shows it only that user's projects.

<!-- endpoints: generated from internal/api by make api-types -->
| Endpoint | Who | REST | MCP | Input (required first) | Output | |
|---|---|---|---|---|---|---|
| `GET list_issues` | all | ✓ | ✓ | `cursor`, `date_by`, `level`, `limit`, `project`, `query`, `since`, `sort`, `status`, `until` | `items` [`id`, `project`, `title`, `location`, `level`, `status`, `event_count`, `not_stored`, `stored_events`, `first_seen`, `last_seen`, `resolved_at`, `ignored_until`, `ignore_events_left`, `mcp_reference`], `next_cursor` | List issues (errors grouped by cause) with event count and first/last seen. Default: unresolved, most recently seen first, every project. The way to find errors without known IDs. |
| `GET get_issue` | all | ✓ | ✓ | `issue_id`, `event_id` | text | Diagnose an issue: its latest event (or event_id) as the agent prompt of notifications, with the exception chain, stack frames with source, message, request, runtime, tags, breadcrumbs, up to 20 logs of the request and the issue history. |
| `GET get_issues` | all | ✓ | ✓ | `issue_ids` | `issues` [`id`, `project`, `title`, `location`, `level`, `status`, `event_count`, `not_stored`, `stored_events`, `first_seen`, `last_seen`, `resolved_at`, `ignored_until`, `ignore_events_left`, `mcp_reference`], `missing_issue_ids`, `forbidden_issue_ids` | Summaries and current statuses of 1 to 100 known issue IDs in one call, without event details. |
| `GET list_issue_events` | all | ✓ | ✓ | `issue_id`, `cursor`, `limit` | `items` [`event_id`, `timestamp`, `level`, `environment`, `release`, `trace_id`, `received_at`], `next_cursor` | List the stored events of an issue, newest first: time, level, environment, release and trace ID. Pass an event_id to get_issue to read one. |
| `GET get_event` | all | ✓ | ✓ | `event_id`, `project` | text | Like get_issue, for an event ID the SDK reported to the application (with its project). |
| `GET list_logs` | all | ✓ | ✓ | `cursor`, `from_id`, `level`, `limit`, `offset`, `project`, `query`, `since`, `trace_id`, `until` | `items` [`id`, `time`, `project`, `level`, `body`, `trace_id`, `issue`], `next_cursor`, `stopped`, `searched_to` | Search logs, newest first. Pass trace_id (from get_issue) for the logs of one request or browser session. A search stops after 20s: then stopped is true, searched_to says how far back it got and next_cursor goes on from there; narrow it (since/until, project, level) rather than paging far. |
| `GET list_projects` | all | ✓ | ✓ | `cursor`, `limit` | `items` [`id`, `name`, `dsn`, `from_env`, `log_levels`, `paused`, `unresolved_issues`], `next_cursor` | List the monitored projects with their unresolved issue counts and DSNs (what a Sentry SDK is configured with to report to the project). Not needed before list_issues: issue rows name their project. |
| `POST update_issue_status` | all | ✓ | ✓ | `issue_ids`, `status`, `ignore_for`, `ignore_until_events` | `issues` [`id`, `project`, `title`, `location`, `level`, `status`, `event_count`, `not_stored`, `stored_events`, `first_seen`, `last_seen`, `resolved_at`, `ignored_until`, `ignore_events_left`, `mcp_reference`], `missing_issue_ids`, `forbidden_issue_ids` | Set the status of 1 to 100 issues in one atomic call: resolved (fixed, reopens if the error happens again), ignored (no notifications; for good, or until the time or number of events chosen when ignoring) or unresolved (still happening or not looked at yet). Returns the updated summaries. |
| `POST delete_issue` | admin | ✓ | ✓ | `issue_id` | `deleted` | Delete an issue with its events; if the error happens again it comes back as a new issue. Admins only. |
| `set_project_active` | all |  | ✓ | `active`, `project` | `id`, `name`, `dsn`, `from_env`, `log_levels`, `paused` | Pause or resume recording a project: while paused, what its apps send is answered but not stored (errors and logs), and nothing is notified. |
| `GET get_event_payload` | all | ✓ |  | `event_id`, `project` | `project_id`, `issue_id`, `event_id`, `timestamp`, `received_at`, `level`, `environment`, `release`, `trace_id`, `span_id`, `parent_span_id`, `thinned`, `payload` | An event with its JSON as stored: credentials and parts that do not help a fix removed; thinned marks an old event whose details were removed. |
| `GET list_trace` | all | ✓ |  | `trace_id` | `trace_id`, `events` [`project_id`, `issue_id`, `event_id`, `timestamp`, `received_at`, `level`, `environment`, `release`, `trace_id`, `span_id`, `parent_span_id`, `thinned`], `logs` [`id`, `project_id`, `timestamp`, `level`, `severity_number`, `body`, `trace_id`, `span_id`, `attributes`], `stopped`, `searched_to` | Events (without payloads) and logs of one trace, up to 100 of each. |
| `get_account` | all |  | ✓ | — | `all_projects`, `project_ids`, `projects`, `id`, `email`, `name`, `display_name`, `role`, `must_change_password`, `created_at`, `last_login_at`, `last_active_at`, `google_linked` | Who the agent acts for: email, name, role (admin or user), which decides the tools offered, and the projects they see. |
| `update_account_name` | all |  | ✓ | `name` | `all_projects`, `project_ids`, `projects`, `id`, `email`, `name`, `display_name`, `role`, `must_change_password`, `created_at`, `last_login_at`, `last_active_at`, `google_linked` | Change the display name of the user the agent acts for. The password can only be changed in the UI. |
| `list_agents` | all |  | ✓ | `user_id` | `user`, `user_id`, `owners` [`id`, `name`], `agents` [`grant_id`, `name`, `kind`, `created_at`, `last_used_at`, `connected`] | Coding agents and personal tokens connected to MCP for the user (admins: user_id of a user whose projects they all have, from owners), with when each was last used; connected is false for a personal token never used. |
| `disconnect_agent` | all |  | ✓ | `grant_id` | `done` | Revoke a connected agent or personal token (grant_id from list_agents); it stops working at once. |
| `get_project` | all |  | ✓ | `project` | `id`, `name`, `dsn`, `from_env`, `log_levels`, `paused`, `color`, `shown_color`, `disk_share`, `allowed_origins` | Settings of a project: DSN, color (missing when it follows the palette), the color it shows and recorded levels. |
| `create_project` | all |  | ✓ | `name`, `allowed_origins`, `color`, `disk_share`, `log_levels` | `id`, `name`, `dsn`, `from_env`, `log_levels`, `paused`, `color`, `shown_color`, `disk_share`, `allowed_origins` | Add a project (one per service sending errors); returns its DSN for the app's Sentry SDK. |
| `update_project` | all |  | ✓ | `project`, `allowed_origins`, `color`, `disk_share`, `log_levels`, `name` | `id`, `name`, `dsn`, `from_env`, `log_levels`, `paused`, `color`, `shown_color`, `disk_share`, `allowed_origins` | Change a project's settings; fields not given keep their values, an empty string returns a setting to the default. |
| `delete_project` | admin |  | ✓ | `project` | `done` | Delete a project added in the UI with all its issues, events and logs; apps using its DSN are rejected afterwards. |
| `list_users` | all |  | ✓ | — | `items` [`user`, `can_manage`, `can_take_over`, `take_over_reason`, `access_editable`, `access_reason`], `next_cursor` | Users of the panel with their role, last login, projects and whether you may change their projects (can_manage, access_editable), and their name, role and password or delete them (can_take_over: only when you have all their projects). With only some projects yourself, you see only those of each user's projects. |
| `create_user` | all |  | ✓ | `email`, `role`, `all_projects`, `name`, `projects` | `user` [`all_projects`, `project_ids`, `projects`, `id`, `email`, `name`, `display_name`, `role`, `must_change_password`, `created_at`, `last_login_at`, `last_active_at`, `google_linked`], `temporary_password`, `note` | Add a user with a role (admin or user) and the projects they see (default: yours). Returns a temporary password, shown only this once, that the user must change at the first login: give it to the user privately. |
| `update_user` | all |  | ✓ | `user_id`, `all_projects`, `name`, `projects`, `role` | `all_projects`, `project_ids`, `projects`, `id`, `email`, `name`, `display_name`, `role`, `must_change_password`, `created_at`, `last_login_at`, `last_active_at`, `google_linked` | Change another user's name, role (admin or user) or projects; fields not given keep their values. The email is set when the user is added and never changes: for another one, delete the user and add a new one. projects replaces only the user's projects you see; the others stay. Name and role change only when you have all the user's projects (can_take_over). Not yourself (update_account_name). |
| `reset_user_password` | all |  | ✓ | `user_id` | `user` [`all_projects`, `project_ids`, `projects`, `id`, `email`, `name`, `display_name`, `role`, `must_change_password`, `created_at`, `last_login_at`, `last_active_at`, `google_linked`], `temporary_password`, `note` | Give another user a new temporary password (returned once): their sessions end and their connected agents are disconnected. Only for a user whose projects you all have (can_take_over). |
| `delete_user` | all |  | ✓ | `user_id` | `done` | Delete another user; their sessions and connected agents end. Their past status changes keep their name. Only for a user whose projects you all have (can_take_over). |
| `list_connections` | all |  | ✓ | — | `items` [`id`, `name`, `type`, `email`, `destination`, `all_projects`, `projects`, `project_ids`, `levels`, `active`, `confirmed`, `include_issue_link`, `delivery`], `next_cursor` | Notification connections (email, Telegram, Slack): where each sends, projects, levels, whether active and confirmed, and delivery health. |
| `start_email_verification` | all |  | ✓ | `email` | `email`, `confirmed`, `state`, `verified_at`, `notice`, `message` | First step of an email connection: mails a 6-digit code to the address, or says it is confirmed already. Ask the user for the code, then call confirm_email. |
| `confirm_email` | all |  | ✓ | `code`, `email` | `email`, `confirmed`, `state`, `verified_at`, `notice`, `message` | Confirm an email address with the code the user received (start_email_verification); then create_connection can use it. |
| `create_connection` | all |  | ✓ | `type`, `active`, `all_projects`, `bot_token`, `chat_id`, `email`, `include_issue_link`, `levels`, `name`, `projects`, `slack_bot_token`, `slack_channel`, `webhook_url` | `id`, `name`, `type`, `email`, `destination`, `all_projects`, `projects`, `project_ids`, `levels`, `active`, `confirmed`, `include_issue_link`, `delivery` [`pending`, `retrying`, `failed`, `last_sent_at`, `last_error`, `failing`] | Add a notification connection for new and regressed issues: email (a confirmed address), telegram (bot_token, chat_id) or slack (webhook_url, or slack_bot_token and slack_channel). Test it with send_test_message. |
| `update_connection` | all |  | ✓ | `connection_id`, `active`, `all_projects`, `bot_token`, `chat_id`, `include_issue_link`, `levels`, `name`, `projects`, `slack_bot_token`, `slack_channel`, `webhook_url` | `id`, `name`, `type`, `email`, `destination`, `all_projects`, `projects`, `project_ids`, `levels`, `active`, `confirmed`, `include_issue_link`, `delivery` [`pending`, `retrying`, `failed`, `last_sent_at`, `last_error`, `failing`] | Change a connection; fields not given keep their values (tokens too). Saving an active connection retries its waiting messages. |
| `set_connection_active` | all |  | ✓ | `active`, `connection_id` | `id`, `name`, `type`, `email`, `destination`, `all_projects`, `projects`, `project_ids`, `levels`, `active`, `confirmed`, `include_issue_link`, `delivery` [`pending`, `retrying`, `failed`, `last_sent_at`, `last_error`, `failing`] | Turn a connection on or off without changing anything else; an unconfirmed address cannot be turned on. |
| `send_test_message` | all |  | ✓ | `connection_id` | `done` | Send a test message through a saved connection and say whether it was delivered. |
| `retry_connection` | all |  | ✓ | `connection_id` | `done` | Send the messages of a connection that wait for a retry now, after the cause was fixed outside this server. |
| `delete_connection` | all |  | ✓ | `connection_id` | `done` | Delete a notification connection; its waiting messages are not sent. A confirmed address stays confirmed. |
| `list_deliveries` | all |  | ✓ | `connection_id`, `cursor`, `limit`, `status` | `items` [`id`, `connection_id`, `message`, `project`, `title`, `issue_id`, `items`, `more`, `status`, `retrying`, `attempts`, `last_error`, `created_at`, `updated_at`, `next_attempt_at`], `next_cursor` | The delivery log, newest first: each message with its connection, issue, status, attempts and last error. |
| `list_audit` | all |  | ✓ | `action`, `cursor`, `limit`, `project`, `query`, `resource`, `since`, `until`, `user_id` | `items` [`id`, `created_at`, `user_id`, `user_name`, `user_email`, `user_role`, `source`, `agent`, `ip`, `resource`, `action`, `record_id`, `record_name`, `project_ids`, `projects`, `before`, `after`, `exists`, `item_count`], `next_cursor` | The audit log, newest first: each change admins and users made (agents through MCP for them) with who, how, from which address and the fields before and after. Admins see every entry, users those of their projects. |
| `get_audit_entry` | all |  | ✓ | `id` | `id`, `created_at`, `user_id`, `user_name`, `user_email`, `user_role`, `source`, `agent`, `ip`, `resource`, `action`, `record_id`, `record_name`, `project_ids`, `projects`, `before`, `after`, `exists`, `item_count`, `items` [`record_id`, `record_name`, `project`, `before`, `after`, `exists`] | One entry of the audit log with every record it changed (a request that changed several, such as a bulk resolve, is one entry: list_audit gives their count). |
| `fetch_skill` | all |  | ✓ | `name` | text | Detailed guidance by name: analyze-errors, fetch-data, update-issue-status. Only when the server instructions do not settle the task; reuse a skill already loaded. |
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
