---
name: update-issue-status
description: What the statuses mean and when an agent may change them.
---

- resolved: the error is fixed; a new event reopens the issue and notifies again. ignored: no notifications, stays ignored when the error recurs, for good or until `ignore_for` (a time) or `ignore_until_events` (that many more events) ends it, when the issue is unresolved again and notifies; it is not a substitute for resolved. unresolved: reopen.
- Before update_issue_status, list the issues (ID, title, current status) and the new status and wait for the user's yes, as for every change (the server instructions). One yes covers the list it was given for; if the list changes, ask again.
- One update_issue_status call takes 1 to {{max_issue_batch}} IDs and is atomic: if an ID is "not found" or "forbidden", nothing changes. It returns the updated summaries, so no read is needed before or after when the IDs are known.
- If MCP is unavailable, ask the user to change the status in the panel.
- For more than {{max_issue_batch}} issues, agree on the batches with the user instead of finding and changing all matches yourself.
