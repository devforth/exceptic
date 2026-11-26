# Coding agents (MCP)

The server is an [MCP](https://modelcontextprotocol.io) server at `/mcp` (Streamable HTTP), so coding agents such as Claude Code and Codex can list issues, read an error with its stack, request and logs, and change issue statuses. A typical request to an agent: "explain issue k3x9q2mf and find the cause in this repository".

The **MCP** page of the UI has the exact commands for your server. The agent acts as the user who connected it, with that user's role, and every status change it makes is recorded under that user's name.

## Connect with OAuth (recommended)

Add the server, sign in in the browser and press **Allow**.

Claude Code (then run `/mcp` in it and choose the server to sign in):

```sh
claude mcp add --transport http exceptic https://errors.example.com/mcp
```

Codex (then open the sign-in address it prints):

```sh
codex mcp add exceptic --url https://errors.example.com/mcp
```

The name is `APP_NAME` in lowercase with dashes (`exceptic` by default); with a custom name the MCP page shows it. Connected agents are listed on the MCP page, where they can be disconnected. Access lasts an hour and is renewed by the agent for up to 30 days of inactivity.

Agents identify themselves either with a Client ID Metadata Document (their `client_id` is an https address of a document they publish; the consent page shows that site) or by registering with the server; both work without configuration.

## Connect with a personal token

For clients without OAuth, or servers without a browser: on the MCP page, tab **Other**, press **Generate token** and send it as `Authorization: Bearer <token>`. Each user has one personal token; regenerating it stops the old one. **Copy setup prompt** gives a prompt that has an agent add the server with the token itself.

Claude Code:

```sh
claude mcp add --transport http --scope user exceptic https://errors.example.com/mcp \
  --header "Authorization: Bearer <token>"
```

Codex, in `~/.codex/config.toml`:

```toml
[mcp_servers.exceptic]
url = "https://errors.example.com/mcp"
http_headers = { Authorization = "Bearer <token>" }
```

Keep the token out of repositories (`--scope user` in Claude Code).

## Agent skill

The [exceptic skill](../skills/exceptic/SKILL.md) tells an agent how to do the rest on its own: deploy the server, connect itself, add the Sentry SDK to an app with the DSN from `list_projects` and check that a test error arrives. How to install it: [README](../README.md#agent-skill).

## Tools

An agent is offered only the tools of its user's role and sees only that user's projects. The role and the projects are read at every request, so a change applies at once. The data tools are the endpoints of the [REST API](api.md) with the same names and inputs; the management tools are for agents and the web interface only, since the REST API has a token but no user.

<!-- tools: generated from internal/api by make api-types -->
| Tool | Who | Input (required first) | Output | |
|---|---|---|---|---|
| `list_issues` | all | `cursor`, `date_by`, `level`, `limit`, `project`, `query`, `since`, `sort`, `status`, `until` | `items` [`id`, `project`, `title`, `location`, `level`, `status`, `event_count`, `not_stored`, `stored_events`, `first_seen`, `last_seen`, `resolved_at`, `ignored_until`, `ignore_events_left`, `mcp_reference`], `next_cursor` | List issues (errors grouped by cause) with event count and first/last seen. Default: unresolved, most recently seen first, every project. The way to find errors without known IDs. |
| `get_issue` | all | `issue_id`, `event_id` | text | Diagnose an issue: its latest event (or event_id) as the agent prompt of notifications, with the exception chain, stack frames with source, message, request, runtime, tags, breadcrumbs, up to 20 logs of the request and the issue history. |
| `get_issues` | all | `issue_ids` | `issues` [`id`, `project`, `title`, `location`, `level`, `status`, `event_count`, `not_stored`, `stored_events`, `first_seen`, `last_seen`, `resolved_at`, `ignored_until`, `ignore_events_left`, `mcp_reference`], `missing_issue_ids`, `forbidden_issue_ids` | Summaries and current statuses of 1 to 100 known issue IDs in one call, without event details. |
| `list_issue_events` | all | `issue_id`, `cursor`, `limit` | `items` [`event_id`, `timestamp`, `level`, `environment`, `release`, `trace_id`, `received_at`], `next_cursor` | List the stored events of an issue, newest first: time, level, environment, release and trace ID. Pass an event_id to get_issue to read one. |
| `get_event` | all | `event_id`, `project` | text | Like get_issue, for an event ID the SDK reported to the application (with its project). |
| `list_logs` | all | `cursor`, `from_id`, `level`, `limit`, `offset`, `project`, `query`, `since`, `trace_id`, `until` | `items` [`id`, `time`, `project`, `level`, `body`, `trace_id`, `issue`], `next_cursor`, `stopped`, `searched_to` | Search logs, newest first. Pass trace_id (from get_issue) for the logs of one request or browser session. A search stops after 20s: then stopped is true, searched_to says how far back it got and next_cursor goes on from there; narrow it (since/until, project, level) rather than paging far. |
| `list_projects` | all | `cursor`, `limit` | `items` [`id`, `name`, `dsn`, `from_env`, `log_levels`, `unresolved_issues`], `next_cursor` | List the monitored projects with their unresolved issue counts and DSNs (what a Sentry SDK is configured with to report to the project). Not needed before list_issues: issue rows name their project. |
| `update_issue_status` | all | `issue_ids`, `status`, `ignore_for`, `ignore_until_events` | `issues` [`id`, `project`, `title`, `location`, `level`, `status`, `event_count`, `not_stored`, `stored_events`, `first_seen`, `last_seen`, `resolved_at`, `ignored_until`, `ignore_events_left`, `mcp_reference`], `missing_issue_ids`, `forbidden_issue_ids` | Set the status of 1 to 100 issues in one atomic call: resolved (fixed, reopens if the error happens again), ignored (no notifications; for good, or until the time or number of events chosen when ignoring) or unresolved (still happening or not looked at yet). Only when the user asks. Returns the updated summaries. |
| `delete_issue` | admin | `issue_id` | `deleted` | Delete an issue with its events; if the error happens again it comes back as a new issue. Admins only. |
| `get_account` | all | — | `all_projects`, `project_ids`, `projects`, `id`, `email`, `name`, `display_name`, `role`, `must_change_password`, `created_at`, `last_login_at`, `last_active_at`, `google_linked` | Who the agent acts for: email, name, role (superadmin, admin or user), which decides the tools offered, and the projects they see. |
| `update_account_name` | all | `name` | `all_projects`, `project_ids`, `projects`, `id`, `email`, `name`, `display_name`, `role`, `must_change_password`, `created_at`, `last_login_at`, `last_active_at`, `google_linked` | Change the display name of the user the agent acts for. The password can only be changed in the UI. |
| `list_agents` | all | `user_id` | `user`, `user_id`, `owners` [`id`, `name`], `agents` [`grant_id`, `name`, `kind`, `created_at`, `last_used_at`, `connected`] | Coding agents and personal tokens connected to MCP for the user (admins: user_id of a user whose projects they all have, from owners), with when each was last used; connected is false for a personal token never used. |
| `disconnect_agent` | all | `grant_id` | `done` | Revoke a connected agent or personal token (grant_id from list_agents); it stops working at once. |
| `get_project` | admin | `project` | `id`, `name`, `dsn`, `from_env`, `log_levels`, `color`, `shown_color`, `disk_share` | Settings of a project: DSN, color (missing when it follows the palette), the color it shows and recorded levels. |
| `create_project` | admin | `name`, `color`, `disk_share`, `log_levels` | `id`, `name`, `dsn`, `from_env`, `log_levels`, `color`, `shown_color`, `disk_share` | Add a project (one per service sending errors); returns its DSN for the app's Sentry SDK. |
| `update_project` | admin | `project`, `color`, `disk_share`, `log_levels`, `name` | `id`, `name`, `dsn`, `from_env`, `log_levels`, `color`, `shown_color`, `disk_share` | Change a project's settings; fields not given keep their values, an empty string returns a setting to the default. |
| `delete_project` | admin | `project` | `done` | Delete a project added in the UI with all its issues, events and logs; apps using its DSN are rejected afterwards. |
| `list_users` | admin | — | `items` [`user`, `can_manage`, `can_take_over`, `take_over_reason`, `access_editable`, `access_reason`], `next_cursor` | Users of the panel with their role, last login, projects and whether you may change their projects (can_manage, access_editable), and their name, role and password or delete them (can_take_over: only when you have all their projects). With only some projects yourself, you see only those of each user's projects. |
| `create_user` | admin | `email`, `role`, `all_projects`, `name`, `projects` | `user` [`all_projects`, `project_ids`, `projects`, `id`, `email`, `name`, `display_name`, `role`, `must_change_password`, `created_at`, `last_login_at`, `last_active_at`, `google_linked`], `temporary_password`, `note` | Add a user with a role (admin or user) and the projects they see (default: yours). Returns a temporary password, shown only this once, that the user must change at the first login: give it to the user privately. |
| `update_user` | admin | `user_id`, `all_projects`, `email`, `name`, `projects`, `role` | `all_projects`, `project_ids`, `projects`, `id`, `email`, `name`, `display_name`, `role`, `must_change_password`, `created_at`, `last_login_at`, `last_active_at`, `google_linked` | Change another user's email, name, role (admin or user) or projects; fields not given keep their values. projects replaces only the user's projects you see; the others stay. Email, name and role change only when you have all the user's projects (can_take_over). Not the superadmin, not yourself (update_account_name). |
| `reset_user_password` | admin | `user_id` | `user` [`all_projects`, `project_ids`, `projects`, `id`, `email`, `name`, `display_name`, `role`, `must_change_password`, `created_at`, `last_login_at`, `last_active_at`, `google_linked`], `temporary_password`, `note` | Give another user a new temporary password (returned once): their sessions end and their connected agents are disconnected. Only for a user whose projects you all have (can_take_over). |
| `delete_user` | admin | `user_id` | `done` | Delete another user; their sessions and connected agents end. Their past status changes keep their name. Only for a user whose projects you all have (can_take_over). |
| `list_connections` | admin | — | `items` [`id`, `name`, `type`, `email`, `destination`, `all_projects`, `projects`, `levels`, `active`, `confirmed`, `include_issue_link`, `delivery`], `next_cursor` | Notification connections (email, Telegram, Slack): where each sends, projects, levels, whether active and confirmed, and delivery health. |
| `start_email_verification` | admin | `email` | `email`, `confirmed`, `state`, `verified_at`, `notice`, `message` | First step of an email connection: mails a 6-digit code to the address, or says it is confirmed already. Ask the user for the code, then call confirm_email. |
| `confirm_email` | admin | `code`, `email` | `email`, `confirmed`, `state`, `verified_at`, `notice`, `message` | Confirm an email address with the code the user received (start_email_verification); then create_connection can use it. |
| `create_connection` | admin | `type`, `active`, `all_projects`, `bot_token`, `chat_id`, `email`, `include_issue_link`, `levels`, `name`, `projects`, `slack_bot_token`, `slack_channel`, `webhook_url` | `id`, `name`, `type`, `email`, `destination`, `all_projects`, `projects`, `levels`, `active`, `confirmed`, `include_issue_link`, `delivery` [`pending`, `retrying`, `failed`, `last_sent_at`, `last_error`, `failing`] | Add a notification connection for new and regressed issues: email (a confirmed address), telegram (bot_token, chat_id) or slack (webhook_url, or slack_bot_token and slack_channel). Test it with send_test_message. |
| `update_connection` | admin | `connection_id`, `active`, `all_projects`, `bot_token`, `chat_id`, `include_issue_link`, `levels`, `name`, `projects`, `slack_bot_token`, `slack_channel`, `webhook_url` | `id`, `name`, `type`, `email`, `destination`, `all_projects`, `projects`, `levels`, `active`, `confirmed`, `include_issue_link`, `delivery` [`pending`, `retrying`, `failed`, `last_sent_at`, `last_error`, `failing`] | Change a connection; fields not given keep their values (tokens too). Saving an active connection retries its waiting messages. |
| `set_connection_active` | admin | `active`, `connection_id` | `id`, `name`, `type`, `email`, `destination`, `all_projects`, `projects`, `levels`, `active`, `confirmed`, `include_issue_link`, `delivery` [`pending`, `retrying`, `failed`, `last_sent_at`, `last_error`, `failing`] | Turn a connection on or off without changing anything else; an unconfirmed address cannot be turned on. |
| `send_test_message` | admin | `connection_id` | `done` | Send a test message through a saved connection and say whether it was delivered. |
| `retry_connection` | admin | `connection_id` | `done` | Send the messages of a connection that wait for a retry now, after the cause was fixed outside this server. |
| `delete_connection` | admin | `connection_id` | `done` | Delete a notification connection; its waiting messages are not sent. A confirmed address stays confirmed. |
| `list_deliveries` | admin | `connection_id`, `cursor`, `limit`, `status` | `items` [`id`, `connection_id`, `message`, `project`, `title`, `issue_id`, `items`, `more`, `status`, `retrying`, `attempts`, `last_error`, `created_at`, `updated_at`, `next_attempt_at`], `next_cursor` | The delivery log, newest first: each message with its connection, issue, status, attempts and last error. |
| `fetch_skill` | all | `name` | text | Detailed guidance by name: analyze-errors, fetch-data, update-issue-status. Only when the server instructions do not settle the task; reuse a skill already loaded. |
<!-- /tools -->

### Skills

`fetch_skill` loads one of these guides; the server's instructions, sent with every connection, list them too.

<!-- skills: generated from internal/api by make api-types -->
- `analyze-errors`: Investigating an issue from its stack trace, events and logs, and matching it to the repository.
- `fetch-data`: Filters, paging and level names for list_issues, list_issue_events and list_logs.
- `update-issue-status`: What the statuses mean and when an agent may change them.
<!-- /skills -->

A refused call is a tool error that starts with its kind:

| Kind | Meaning |
|---|---|
| `invalid: …` | The input is wrong; the message names the field. |
| `not found: …` | It does not exist: deleted, or never existed (`not found: issue k3x9q2mf`). |
| `forbidden: …` | It exists in a project outside the user's, which the message lists (`forbidden: issue k3x9q2mf is not in your projects (frontend, test2)`), or the user's role may not do it (`forbidden: your role may not change this user`, or for a user with projects the admin lacks `forbidden: Only an admin with all of this user's projects can change it.`). |
| `conflict: …` | A stored record is in the way: a name taken, a connection that already sends there, an address not confirmed yet. |
| `unavailable: …` | A destination refused a test message, or the mail server a confirmation code. |
| `internal error` | A server failure, logged on the server. |

`get_issues` lists the IDs of other projects' issues in `forbidden_issue_ids`, apart from `missing_issue_ids`.

Not available to agents: changing your own password (the web interface asks for the current one) and generating personal tokens. Tools that delete data or reset passwords, and those that change users, project settings or notification connections, tell the agent to show what will change and wait for your confirmation in chat.

Notifications and the issue page carry the request for an agent, e.g. `Exceptic MCP: get_issue {"issue_id":"k3x9q2mf"}`, to paste into the agent.

## What leaves the server

Everything an agent reads is sent to its model provider. The server never returns raw event payloads or log attributes, and in the text your apps send replaces IP addresses, hosts, email addresses, credentials (tokens in URL paths too, and a Slack or Discord webhook path or a Telegram bot token wherever it appears) and home directories with placeholders. A trace ID is shown only in its protocol form (32 hex characters). Source lines are included only for your own code, or for every frame of a stack where no frame is marked as your code. The management tools return user emails and notification addresses as stored.

## Security

- Serve the server over HTTPS (the MCP page warns when agents would connect over plain HTTP from another machine).
- `ALLOWED_IPS` applies to MCP as well.
- Deleting a user, or resetting their password from the Users page, disconnects their agents.
