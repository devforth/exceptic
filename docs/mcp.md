# Coding agents (MCP)

Coding agents such as Claude Code and Codex can read your errors and logs through the MCP server at `/mcp`. Ask an agent, for example: "explain issue k3x9q2mf and find the cause in this repository".

The **MCP** page of the UI has the exact commands for your server. The agent acts as the user who connected it and sees only their projects.

## Connect with OAuth (recommended)

Claude Code (then run `/mcp` in it and sign in):

```sh
claude mcp add --transport http exceptic https://errors.example.com/mcp
```

Codex (then open the sign-in address it prints):

```sh
codex mcp add exceptic --url https://errors.example.com/mcp
```

Sign in in the browser and press **Allow**. Access lasts an hour and is renewed by the agent for up to 30 days of inactivity. Connected agents are listed on the MCP page, where you can disconnect them.

## Connect with a personal token

For clients without a browser sign-in: on the MCP page, tab **Other**, press **Generate token**. Regenerating it stops the old one.

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

Keep the token out of repositories.

## Tools

The tools are the endpoints marked MCP in the [table of endpoints](api.md#endpoints). An agent gets only the tools its user's role allows. Tools that delete or change settings ask the agent to get your confirmation in chat first.

Notifications and the issue page have a line to paste into the agent, e.g. `Exceptic MCP: get_issue {"issue_id":"k3x9q2mf"}`.

`fetch_skill` loads one of these guides:

<!-- skills: generated from internal/api by make api-types -->
- `analyze-errors`: Investigating an issue from its stack trace, events and logs, and matching it to the repository.
- `fetch-data`: Filters, paging and level names for list_issues, list_issue_events and list_logs.
- `update-issue-status`: What the statuses mean and when an agent may change them.
<!-- /skills -->

## What leaves the server

Everything an agent reads goes to its model provider. Before that, IP addresses, hosts, emails, tokens and home folders in your apps' data are replaced with placeholders, and raw event data is never returned.
