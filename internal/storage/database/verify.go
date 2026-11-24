package database

import (
	"errors"
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

	tallies, err := database.db.CheckTallies()

	if err != nil {
		return problems, err
	}

	problems = append(problems, tallies...)

	countedEvents := 0

	// Issue by issue as stored, none held at once.
	err = database.issues.Primary().Each(database.db, func(entry kv.Entry[issueRecord]) (bool, error) {
		issue := entry.Record
		stored, _, err := database.events.Primary(kv.Text(issue.ID)).Count(database.db, 0)

		if err != nil {
			return false, err
		}

		days := 0

		for _, entry := range issue.EventDays {
			days += entry.Count
		}

		if stored != issue.Stored || days != issue.Stored {
			problems = append(problems, fmt.Sprintf("issue %s counts %d events (%d by day), %d stored", issue.ID, issue.Stored, days, stored))
		}

		countedEvents += stored

		return true, nil
	})

	if err != nil {
		return problems, err
	}

	if events, _, err := database.events.Primary().Count(database.db, 0); err != nil {
		return problems, err
	} else if events != countedEvents {
		problems = append(problems, fmt.Sprintf("%d events stored, %d of them belong to an issue", events, countedEvents))
	}

	err = database.status.Primary().Each(database.db, func(entry kv.Entry[statusRecord]) (bool, error) {
		if _, err := database.issues.GetValue(database.db, kv.K(kv.Text(entry.Record.IssueID))); errors.Is(err, kv.ErrNotFound) {
			problems = append(problems, fmt.Sprintf("status change %x of missing issue %s", entry.Key, entry.Record.IssueID))
		}

		return true, nil
	})

	return problems, err
}
