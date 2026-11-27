# Settings in the web UI

The fields of the settings pages. The UI shows these descriptions in the hints (?) next to the fields. Environment variables are in [Configuration](configuration.md).

## Projects

A project is one service (backend, frontend, worker) with its own DSN. Deleting a project stops its DSN at once and deletes its data. Only admins delete projects.

| Field | |
|---|---|
| Color | The color of the project's badge in lists and emails. |
| DSN | The address the app's Sentry SDK sends to. Safe to put in browser code: it only allows sending to this project. Reset DSN on the Installation page gives a new one and stops the old one; the project shows as connected again once an app sends with the new one. |
| Paused | While paused, nothing the project's apps send is stored or notified. |
| Logs | Log levels to store; others are dropped. Errors of every level are always stored. |
| Disk share | The part of MAX_DATABASE_SIZE the project may take. When it reaches 99% of it, the project's own oldest data is deleted down to 80% of its share, so a flood in one project does not push out the others. 100% means no limit of its own. |
| Allowed domains | Sites whose browsers may send to the project, e.g. `https://app.example.com` or `https://*.example.com`. Empty: any site. Backend apps are not checked. |
| History kept | The oldest record of each kind still stored. When the database is full, the least useful data goes first ([Retention](configuration.md#retention)). |
| Dropped | What the project sent in the last 24 hours that was not stored: over its rate limit, over its part of the queue, the server busy, too large or unreadable. Resets on restart. |

## Notifications

A connection sends new issues, and issues that came back, to an email address, a Telegram chat or a Slack channel. It sends at most 5 a minute for each project; the rest come as one summary ("backend: 120 more issues in the last 1 minute").

It also tells when a project's data was dropped (first time in 24 hours) and when the cleanup deleted data. Each email has a link to unsubscribe.

| Field | |
|---|---|
| Type | Email needs a mail server (SMTP_HOST). Telegram and Slack need only their tokens. |
| Email address | Gets a 6-digit code first; nothing is sent until the code is entered. |
| Telegram bot token | The token @BotFather gives for your bot. Add the bot to the chat. |
| Chat ID | The chat the bot writes to: a number (negative for groups) or @channel_name. To find it, write a message in the chat and press Find chat ID. |
| Incoming webhook URL | A Slack incoming webhook. Mattermost and Rocket.Chat webhooks work too. |
| Slack bot token | Instead of a webhook: a bot token (xoxb-…) with the chat:write scope. The bot also puts a prompt for a coding agent in a thread under each error. |
| Channel ID | The channel the Slack bot posts to (C0123456789, at the bottom of the channel details). Invite the bot first. |
| Projects | Projects whose issues are sent. All projects includes ones added later. |
| Levels | Levels that are sent; a new connection sends fatal and error. Events of other levels are still stored. |
| Active | Off: nothing is sent. |
| Include Open issue link | Adds a link to the issue page (needs PUBLIC_URL). Off by default for chats, which people outside the team may read. |

## Users

- An admin changes every other user.
- A user sees only the users of the role user they share a project with, and changes them only when they have all of that user's projects.
- Everyone changes their own name and password on the **Account** page. The email never changes: for another one, delete the user and add a new one.

The user's page also shows their sessions, the newest 100 sign-ins and the newest 100 refused attempts.

| Field | |
|---|---|
| Email | The login. Set once, when the user is added. |
| Role | What the user may do. An admin gives admin or user; a user gives user only. |
| Projects | Projects the user sees, in the UI and through their coding agents. Only for the role user: an admin sees every project. All projects includes ones added later. |

## Audit log

Every change made in the UI, by a coding agent or through the REST API: when, who, from which IP address, what record and which fields changed (before and after). Entries are never deleted.

Recorded: issue statuses and deletes, deleted logs, projects, notification connections, users (added, changed, password reset, deleted, sessions ended, agents disconnected). Not recorded: changes to your own account, test messages and failed attempts. Passwords and tokens are never stored; a changed one shows as `[replaced]`. A DSN reset keeps the old and the new DSN. A request that changed several records (a bulk resolve) is one entry with the list of its records.

An admin sees every entry. A user sees the entries of their projects; changes made by people they do not see on the Users page (admins, users of other projects) show only as "Admin" or "User", without email or IP.
