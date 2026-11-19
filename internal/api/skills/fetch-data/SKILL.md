---
name: fetch-data
description: Filters, paging and level names for list_issues, list_issue_events and list_logs.
---

- Narrow on the server: project (name or ID), status (`unresolved` by default, `all` for every status), level, query (text in the title or location), since/until (RFC 3339, e.g. 2026-11-11T10:00:00Z) and sort; each tool's schema lists the values it takes.
- Level names differ on purpose: issues use `warning`, logs `warn`, and only logs have `trace`.
- list_logs with trace_id returns the logs of one request or browser session; get_issue prints the trace ID of its event.
- With next_cursor, more rows exist: say so instead of presenting the page as complete; the next page takes the same filters.
- There is no aggregation tool. list_projects gives the unresolved issue count of each project; there are no other totals, so do not count or chart from a partial page.
- Read details (get_issue) only for the rows that matter, not for every row of a list.
- Present issues as a table: events is event_count, last seen is last_seen (a date and time or "N min ago"). Add first seen when sorting or filtering by it, and the status when the list mixes statuses. A ranking ("most events", "oldest") shows the value it ranks by in every row, equal values too.
- Logs: time, project, level and the line; events: time, level, environment and release.
