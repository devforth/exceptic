package database

import (
	"fmt"
	"maps"

	"github.com/devforth/exceptic/internal/storage/kv"
)

// Verify checks the index keys of every collection, the byte counters and the
// event counts of issues, and finds events and status changes of no issue; it reads everything.
func (database *Store) Verify() ([]string, error) {
	problems, err := database.schema.Check(database.db)

	if err != nil {
		return problems, err
	}

	counted, groups, err := database.schema.Counted(database.db)

	if err != nil {
		return problems, err
	}

	if stored := database.db.Counters(); !maps.Equal(stored, counted) {
		problems = append(problems, fmt.Sprintf("byte counters %v, records take %v", stored, counted))
	}

	if stored := database.db.Groups(); !maps.Equal(stored, groups) {
		problems = append(problems, fmt.Sprintf("project byte counters %v, records take %v", stored, groups))
	}

	database.issuesMutex.RLock()

	issues := make(map[string]issueRecord, len(database.table.byID))

	for id, issue := range database.table.byID {
		issues[id] = *issue
	}

	database.issuesMutex.RUnlock()

	countedEvents := 0

	for _, issue := range issues {
		stored, _, err := database.events.Primary(kv.Text(issue.ID)).Count(database.db, 0)

		if err != nil {
			return problems, err
		}

		days := 0

		for _, entry := range issue.EventDays {
			days += entry.Count
		}

		if stored != issue.Stored || days != issue.Stored {
			problems = append(problems, fmt.Sprintf("issue %s counts %d events (%d by day), %d stored", issue.ID, issue.Stored, days, stored))
		}

		countedEvents += stored
	}

	if events, _, err := database.events.Primary().Count(database.db, 0); err != nil {
		return problems, err
	} else if events != countedEvents {
		problems = append(problems, fmt.Sprintf("%d events stored, %d of them belong to an issue", events, countedEvents))
	}

	err = database.status.Primary().Each(database.db, func(entry kv.Entry[statusRecord]) (bool, error) {
		if _, ok := issues[entry.Record.IssueID]; !ok {
			problems = append(problems, fmt.Sprintf("status change %x of missing issue %s", entry.Key, entry.Record.IssueID))
		}

		return true, nil
	})

	return problems, err
}
