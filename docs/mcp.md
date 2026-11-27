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

The tools are the endpoints marked MCP in the [table of endpoints](api.md#endpoints), with the role each needs.

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

Not available to agents: changing your own password (the web interface asks for the current one) and generating personal tokens. Every tool that changes something (anything but reading) tells the agent to show what will change and wait for your confirmation in chat.

Notifications and the issue page carry the request for an agent, e.g. `Exceptic MCP: get_issue {"issue_id":"k3x9q2mf"}`, to paste into the agent.

## What leaves the server

Everything an agent reads is sent to its model provider. The server never returns raw event payloads or log attributes, and in the text your apps send replaces IP addresses, hosts, email addresses, credentials (tokens in URL paths too, and a Slack or Discord webhook path or a Telegram bot token wherever it appears) and home directories with placeholders. A trace ID is shown only in its protocol form (32 hex characters). Source lines are included only for your own code, or for every frame of a stack where no frame is marked as your code. The management tools return user emails and notification addresses as stored.

## Security

- Serve the server over HTTPS (the MCP page warns when agents would connect over plain HTTP from another machine).
- `ALLOWED_IPS` applies to MCP as well.
- Deleting a user, or resetting their password from the Users page, disconnects their agents.
