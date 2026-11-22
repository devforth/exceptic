package database

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// issueRecord is a stored issue; the whole table is also kept in memory.
type issueRecord struct {
	store.Issue
	Fingerprint string `json:"fingerprint"`
	// Stored counts the stored events; EventCount counts every event received.
	Stored int `json:"stored"`
	// EventDays counts the stored events per day received, oldest first.
	EventDays []dayCount `json:"event_days"`

	// storedSize is the size of the stored value (kv.Collection.Write).
	storedSize int
}

type dayCount struct {
	Day   int32 `json:"d"` // days since 1970-01-01, UTC
	Count int   `json:"n"`
}

func (issue *issueRecord) addEvent(at time.Time) {
	issue.Stored++
	number := dayOf(at)
	index, found := slices.BinarySearchFunc(issue.EventDays, number, func(entry dayCount, target int32) int { return cmp.Compare(entry.Day, target) })

	if found {
		issue.EventDays[index].Count++

		return
	}

	issue.EventDays = slices.Insert(issue.EventDays, index, dayCount{Day: number, Count: 1})
}

func (issue *issueRecord) removeEvent(at time.Time) {
	issue.Stored = max(issue.Stored-1, 0)
	number := dayOf(at)
	index, found := slices.BinarySearchFunc(issue.EventDays, number, func(entry dayCount, target int32) int { return cmp.Compare(entry.Day, target) })

	if !found {
		return
	}

	if issue.EventDays[index].Count--; issue.EventDays[index].Count <= 0 {
		issue.EventDays = slices.Delete(issue.EventDays, index, index+1)
	}
}

func (issue *issueRecord) copy() *issueRecord {
	duplicate := *issue
	duplicate.EventDays = slices.Clone(issue.EventDays)

	if issue.ResolvedAt != nil {
		resolvedAt := *issue.ResolvedAt
		duplicate.ResolvedAt = &resolvedAt
	}

	return &duplicate
}

type issueTable struct {
	byID          map[string]*issueRecord
	byFingerprint map[string]*issueRecord
}

func fingerprintKey(projectID int, fingerprint string) string {
	return string(kv.K(kv.U32(uint32(projectID)), kv.Text(fingerprint)))
}

func (table *issueTable) put(issue *issueRecord) {
	table.byID[issue.ID] = issue
	table.byFingerprint[fingerprintKey(issue.ProjectID, issue.Fingerprint)] = issue
}

func (table *issueTable) remove(id string) {
	if issue, ok := table.byID[id]; ok {
		delete(table.byID, id)
		delete(table.byFingerprint, fingerprintKey(issue.ProjectID, issue.Fingerprint))
	}
}

func (database *Store) loadIssues() error {
	table := &issueTable{byID: map[string]*issueRecord{}, byFingerprint: map[string]*issueRecord{}}

	err := database.issues.Primary().Each(database.db, func(entry kv.Entry[issueRecord]) (bool, error) {
		issue := entry.Record
		issue.storedSize = entry.Size
		table.put(&issue)

		return true, nil
	})

	database.table = table

	return err
}

type match int

const (
	noMatch match = iota
	isMatch
	// checkEvents: the range covers a day of its events in part, so their times decide.
	checkEvents
)

func inRange(filter store.IssueFilter, at time.Time) bool {
	return (filter.SeenSince.IsZero() || !at.Before(filter.SeenSince)) && (filter.SeenUntil.IsZero() || at.Before(filter.SeenUntil))
}

func matches(issue *issueRecord, filter store.IssueFilter) match {
	if len(filter.ProjectIDs) > 0 && !slices.Contains(filter.ProjectIDs, issue.ProjectID) ||
		filter.Status != "" && issue.Status != filter.Status ||
		len(filter.Levels) > 0 && !slices.Contains(filter.Levels, issue.Level) ||
		filter.Query != "" && !textutil.ContainsFold(issue.Title, filter.Query) && !textutil.ContainsFold(issue.Culprit, filter.Query) {
		return noMatch
	}

	if filter.SeenSince.IsZero() && filter.SeenUntil.IsZero() {
		return isMatch
	}

	yes := func(ok bool) match {
		if ok {
			return isMatch
		}

		return noMatch
	}

	switch filter.DateField {
	case store.DateFirstSeen:
		return yes(inRange(filter, issue.FirstSeen))
	case store.DateLastSeen:
		return yes(inRange(filter, issue.LastSeen))
	}

	// Both are event times, kept when the cleanup deletes the events.
	if inRange(filter, issue.FirstSeen) || inRange(filter, issue.LastSeen) {
		return isMatch
	}

	result := noMatch

	for _, entry := range issue.EventDays {
		start := dayStart(entry.Day)

		switch whole, touched := inRange(filter, start) && inRange(filter, start.Add(day-time.Nanosecond)),
			(filter.SeenUntil.IsZero() || start.Before(filter.SeenUntil)) && (filter.SeenSince.IsZero() || start.Add(day).After(filter.SeenSince)); {
		case whole:
			return isMatch
		case touched:
			result = checkEvents
		}
	}

	return result
}

// anyEventInRange walks the issue's events newest first and stops once they
// are older than the range (by more than markSlack: arrival order follows the
// sequence only roughly).
func (database *Store) anyEventInRange(issueID string, filter store.IssueFilter) (bool, error) {
	found := false

	err := database.events.Primary(kv.Text(issueID)).Newest().Head().Each(database.db, func(entry kv.Entry[eventRecord]) (bool, error) {
		received := entry.Record.ReceivedAt
		found = inRange(filter, received)

		return !found && (filter.SeenSince.IsZero() || !received.Before(filter.SeenSince.Add(-markSlack))), nil
	})

	return found, err
}

// filterIssues visits the issues that match; records in the table are never
// changed in place (a batch replaces them), so they are read after the lock.
// Events are read from disk only after releasing the table.
func (database *Store) filterIssues(filter store.IssueFilter, visit func(issue *issueRecord)) error {
	var check []*issueRecord

	database.issuesMutex.RLock()

	for _, issue := range database.table.byID {
		switch matches(issue, filter) {
		case isMatch:
			visit(issue)
		case checkEvents:
			check = append(check, issue)
		case noMatch:
		}
	}

	database.issuesMutex.RUnlock()

	for _, issue := range check {
		found, err := database.anyEventInRange(issue.ID, filter)

		if err != nil {
			return err
		}

		if found {
			visit(issue)
		}
	}

	return nil
}

type issueComparison func(first, second *issueRecord) int

// reversed orders the other way, ties included.
func (compare issueComparison) reversed() issueComparison {
	return func(first, second *issueRecord) int { return compare(second, first) }
}

// issueOrder breaks ties by id, so pages stay stable.
func issueOrder(sort string, projectNames map[int]string) issueComparison {
	lastSeen := issueComparison(func(first, second *issueRecord) int {
		return cmp.Or(second.LastSeen.Compare(first.LastSeen), cmp.Compare(second.ID, first.ID))
	})
	firstSeen := issueComparison(func(first, second *issueRecord) int {
		return cmp.Or(second.FirstSeen.Compare(first.FirstSeen), cmp.Compare(second.ID, first.ID))
	})
	eventCount := issueComparison(func(first, second *issueRecord) int {
		return cmp.Or(cmp.Compare(second.EventCount, first.EventCount), cmp.Compare(second.ID, first.ID))
	})
	byProject := issueComparison(func(first, second *issueRecord) int {
		return cmp.Compare(projectNames[first.ProjectID], projectNames[second.ProjectID])
	})
	// The issues of one project: newest first in both directions.
	thenLastSeen := func(compare issueComparison) issueComparison {
		return func(first, second *issueRecord) int { return cmp.Or(compare(first, second), lastSeen(first, second)) }
	}

	switch sort {
	case store.SortProject:
		return thenLastSeen(byProject)
	case store.SortProjectDesc:
		return thenLastSeen(byProject.reversed())
	case store.SortLastSeenAsc:
		return lastSeen.reversed()
	case store.SortFirstSeen:
		return firstSeen
	case store.SortFirstSeenAsc:
		return firstSeen.reversed()
	case store.SortEventCount:
		return eventCount
	case store.SortEventCountAsc:
		return eventCount.reversed()
	default: // "": store.SortLastSeen
		return lastSeen
	}
}

func (database *Store) CountIssues(_ context.Context, filter store.IssueFilter) (int, error) {
	count := 0
	err := database.filterIssues(filter, func(*issueRecord) { count++ })

	return count, err
}

// maxKeptIssues: up to this offset+limit, ListIssues keeps the first issues
// in order as it goes; past it, it sorts every match.
const maxKeptIssues = 1000

// ListIssues sorts the matching issues and copies only the page.
func (database *Store) ListIssues(ctx context.Context, filter store.IssueFilter) ([]store.Issue, error) {
	names := map[int]string{}

	if filter.Sort == store.SortProject || filter.Sort == store.SortProjectDesc {
		projects, err := database.ListProjects(ctx)

		if err != nil {
			return nil, err
		}

		for _, project := range projects {
			names[project.ID] = strings.ToLower(project.Name)
		}
	}

	order := issueOrder(filter.Sort, names)
	offset := min(max(filter.Offset, 0), store.MaxOffset)
	keep := offset + filter.Limit

	var matched []*issueRecord

	collect := func(issue *issueRecord) { matched = append(matched, issue) }

	// A page near the start keeps only the first offset+limit, in order, instead of sorting them all.
	if filter.Limit > 0 && keep <= maxKeptIssues {
		collect = func(issue *issueRecord) {
			if len(matched) == keep && order(issue, matched[keep-1]) >= 0 {
				return
			}

			at, _ := slices.BinarySearchFunc(matched, issue, order)
			matched = slices.Insert(matched, at, issue)

			if len(matched) > keep {
				matched = matched[:keep]
			}
		}
	}

	if err := database.filterIssues(filter, collect); err != nil {
		return nil, err
	}

	slices.SortFunc(matched, order)

	start := min(offset, len(matched))
	end := len(matched)

	if filter.Limit > 0 {
		end = min(start+filter.Limit, len(matched))
	}

	issues := make([]store.Issue, 0, end-start)

	for _, issue := range matched[start:end] {
		issues = append(issues, issue.Issue)
	}

	return issues, nil
}

func (database *Store) GetIssue(_ context.Context, issueID string) (store.Issue, error) {
	database.issuesMutex.RLock()

	defer database.issuesMutex.RUnlock()

	issue, ok := database.table.byID[issueID]

	if !ok {
		return store.Issue{}, store.ErrNotFound
	}

	return issue.Issue, nil
}

func (database *Store) GetIssues(_ context.Context, issueIDs []string) ([]store.Issue, error) {
	database.issuesMutex.RLock()

	defer database.issuesMutex.RUnlock()

	issues := []store.Issue{}

	for _, id := range issueIDs {
		if issue, ok := database.table.byID[id]; ok {
			issues = append(issues, issue.Issue)
		}
	}

	return issues, nil
}

func (database *Store) UnresolvedCounts(context.Context) (map[int]int, error) {
	database.issuesMutex.RLock()

	defer database.issuesMutex.RUnlock()

	counts := map[int]int{}

	for _, issue := range database.table.byID {
		if issue.Status == store.StatusUnresolved {
			counts[issue.ProjectID]++
		}
	}

	return counts, nil
}

func (database *Store) IssueLevels(context.Context) ([]string, error) {
	database.issuesMutex.RLock()

	defer database.issuesMutex.RUnlock()

	seen := map[string]bool{}

	for _, issue := range database.table.byID {
		seen[issue.Level] = true
	}

	return slices.Collect(maps.Keys(seen)), nil
}
