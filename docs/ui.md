# Settings in the web UI

The fields of the settings pages. The UI shows these rows as the hints (?) next to the fields: change a hint here. Settings from the environment are in [Configuration](configuration.md).

## Projects

A project is one service (backend, frontend, worker) with its own DSN. Projects from `PROJECTS` follow it as [Configuration](configuration.md#projects) says; the rest of their settings change here. Deleting a project stops its DSN at once, then deletes its issues, events and logs; a deletion stopped by a restart goes on at the next start. A project an admin with only some projects adds joins his projects at once; a deleted one leaves every user's. The storage card shows the parts of your projects; Other includes the projects you do not see. A name another project has is refused, without naming that project when it is not one of yours.

| Field | |
|---|---|
| Color | Marks the project's badge in lists and the header of its emails. |
| DSN | The address the app's Sentry SDK sends errors and logs to. The key in it only allows sending to this project, so it can be in browser code. Reset DSN on the Installation page gives a new one: the old one is refused from then on. A project from PROJECTS keeps it too, and its key there is no longer read. |
| Paused | Pause recording in a project's menu: what its apps send is answered as usual but nothing is stored or notified until Resume recording. Admins and users with the project. |
| Logs | Log levels to store; others are dropped when they arrive. Unchanged, the project follows RECORD_LOG_LEVELS. Errors and messages of every level are stored. |
| Disk share | The part of MAX_DATABASE_SIZE the project may take. When it reaches 99% of it, even with the database under its limit, the cleanup deletes the project's own data in the usual order, down to 80% of its share, so a flood in one project does not push out the others. A ceiling, not a reservation: shares may add up to more than 100%. At 100% the project has no limit of its own. |
| Allowed domains | The sites whose browsers may send to the project, e.g. `https://app.example.com` or `https://*.example.com` for its subdomains; a browser on another site is refused. Empty: any site. It stops other sites from using the DSN in their pages; a script outside a browser can still send with any Origin, which only the rate limits hold back. Backend SDKs send no origin and are not checked. |
| History kept | The oldest record of each kind still stored; Errors counts events of every level. When the database nears its limit, the cleanup deletes the least useful data first, in the order of the [cleanup by size](configuration.md#retention). |
| Dropped | What the project sent in the last 24 hours that was not stored: over its rate limit, refused while the server was busy, too large, unreadable or not received whole, and what its SDK reports it held back. Resets on restart. |

## Notifications

A connection sends new issues and issues that happen again after being resolved or once their ignore has ended to one place: an email address, a Telegram chat or a Slack channel. It sends at most 5 a minute for each project; the rest of that project's come once the minute ends as one summary ("backend: 120 more issues in the last 1 minute"), so a flood in one project neither holds back nor mixes with another's.

It also sends notices about the storage of its projects, whatever its levels: when data a project sent is dropped for the first time in 24 hours (for the reasons of Dropped under Projects; the next notice after a day without one, also across restarts; the Projects page counts every drop), and when a cleanup run that deleted data of the project is done, with the data held before and after and what each step deleted in its projects, other projects' as one Other row, as the Cleanup history shows it ([Retention](configuration.md#retention)). A connection for all projects also gets the notices of cleanups that deleted no project's data.

With `PUBLIC_URL` set, every email has a link that stops it, and the headers mail providers show their own Unsubscribe button for (one click, RFC 8058): it pauses every connection that sends to that address; an admin can resume them here. The link in the email first asks to confirm, since mail scanners open links.

| Field | |
|---|---|
| Type | Email needs a mail server (SMTP_HOST); Telegram and Slack need only the tokens entered below. |
| Email address | Gets a 6-digit code first; the connection sends nothing until the code is entered. An address confirmed before is accepted at once. |
| Telegram bot token | The token @BotFather gives for your bot. The bot must be a member of the chat. Stored encrypted. |
| Chat ID | The chat the bot writes to: a number (negative for groups; supergroups and channels start with -100) or @channel_name of a public channel. To find the number, write a message in the chat and press Find chat ID: it is chat.id in the answer. |
| Incoming webhook URL | A Slack incoming webhook; it posts to the channel chosen when it was created. Mattermost and Rocket.Chat webhooks work too. Stored encrypted. |
| Slack bot token | Optional, instead of the webhook: a bot token (xoxb-…) with the chat:write scope. The bot posts each error and puts the prompt for a coding agent in a thread under it. A new token replaces a saved webhook, and a new webhook without a token replaces a saved bot and its channel. Stored encrypted. |
| Channel ID | The channel the Slack bot posts to (C0123456789, at the bottom of the channel details). Invite the bot to it first. |
| Projects | Projects whose issues are sent. All projects includes the ones added later. An admin with only some projects chooses among them, and sees only the connections that send for nothing else. |
| Levels | Levels that are sent; a new connection sends fatal and error. Events of other levels are still stored, they only do not notify. |
| Active | Off: nothing is sent, and messages waiting for a retry fail. |
| Include Open issue link | Adds a link to the issue page; offered only when PUBLIC_URL is set. Off by default for chats, which people outside the team may read. |

## Users

Who may change whom:

- An admin changes every other user, other admins too.
- A user sees the users of the role user they share a project with (not the admins), and changes them only when they have all of their projects: they could otherwise reset their password, log in as them and see projects they do not have.
- Everyone changes their own name and password only on the Account page; the password asks for the current one. The email is set once, when the user is added: for another one, delete the user and add a new one.

Changing means the name and role, resetting the password, deleting the user and disconnecting their coding agents. The Projects field is apart: a user edits it for any user they see (see the table).

Sessions and sign-ins (the Account page for oneself, a user's page for those who manage them): the browsers signed in now, with the address, how they signed in and when they were last active; the newest 100 sign-ins and, apart, the newest 100 refused attempts (a wrong password, the login limit (once per network and window), another Google account, Google unlinked); and the linked Google account, if any. Disconnecting a session or unlinking Google is changing the user; nobody disconnects the session they use (they log out).

For a user with the projects backend and frontend, another user with:

- backend, or backend and frontend: he changes everything.
- backend and frontend 2: read-only; in the projects field he adds or removes backend and frontend, frontend 2 stays.
- All projects: read-only, the projects field too.

| Field | |
|---|---|
| Email | The login, unique; set when the user is added and never changed: for another email, delete the user and add a new one. No confirmation is sent. |
| Role | What the user may do. An admin gives admin or user; a user gives user only. |
| Projects | Projects whose issues, logs and settings the user sees, here and through their coding agents; asked only for the role user, since an admin has every project. All projects includes the ones added later. A user changes only their own projects in this field; the other user's other projects stay. Only an admin gives all projects or changes a user who has them. A user without projects sees none. |

## Audit log

Every change made in the UI, by a coding agent or through the REST API: when, who, from which IP address, what record and which fields changed (before and after). Entries are never deleted.

Recorded: issue statuses and deletes, deleted logs, projects, notification connections, users (added, changed, password reset, deleted, sessions ended, agents disconnected). Not recorded: changes to your own account, test messages and failed attempts. Passwords and tokens are never stored; a changed one shows as `[replaced]`. A DSN reset keeps the old and the new DSN. A request that changed several records (a bulk resolve) is one entry with the list of its records.

An admin sees every entry. A user sees the entries of their projects; changes made by people they do not see on the Users page (admins, users of other projects) show only as "Admin" or "User", without email or IP.
