package database

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The issue indexes and tallies follow every change of an issue: new events
// (last seen and count move, the level may change), status changes, ignores
// with a time, deletes. Walking an index gives the order the in-memory
// table gives, and the tallies count what it holds.
func TestIssueIndexesFollowTheIssues(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	random := rand.New(rand.NewPCG(1, 2))
	start := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	levels := []string{"fatal", "error", "warning", "info", "debug"}
	ids := map[string]bool{}

	for index := range 400 {
		project := 1 + random.IntN(3)
		received := start.Add(time.Duration(index) * time.Minute)
		result, err := database.SaveEvent(ctx, store.NewEvent{
			ProjectID: project, EventID: fmt.Sprintf("%032x", index), Fingerprint: fmt.Sprintf("fp-%d", random.IntN(60)),
			Title: "error", Level: levels[random.IntN(len(levels))], Timestamp: received, ReceivedAt: received, Payload: []byte(`{}`),
		})

		if err != nil {
			t.Fatal(err)
		}

		ids[result.IssueID] = true
	}

	all := slices.Sorted(func(yield func(string) bool) {
		for id := range ids {
			if !yield(id) {
				return
			}
		}
	})

	if _, err := database.SetIssuesStatus(ctx, all[:20], store.StatusResolved, store.Actor{Source: store.SourceAPI}); err != nil {
		t.Fatal(err)
	}

	if _, err := database.IgnoreIssues(ctx, all[20:30], store.IgnoreBound{For: time.Hour}, store.Actor{Source: store.SourceAPI}); err != nil {
		t.Fatal(err)
	}

	if _, err := database.DeleteIssues(ctx, all[30:35]); err != nil {
		t.Fatal(err)
	}

	if problems, err := database.Verify(); err != nil || len(problems) > 0 {
		t.Fatalf("verify: %v %v", problems, err)
	}

	table, err := database.issues.Primary().All(database.db)

	if err != nil {
		t.Fatal(err)
	}

	// Tallies: per project, status and level, the issues of the table.
	counted := map[string]int64{}

	for _, issue := range table {
		counted[string(issueTally(issue.ProjectID, issue.Status, issue.Level))]++
	}

	stored := database.db.Tallies()

	for tally, count := range counted {
		if stored[tally] != count {
			t.Errorf("tally %x: %d stored, %d issues", tally, stored[tally], count)
		}
	}

	// Unresolved issues of every project and level, newest seen first,
	// merged from the index: the same order the table sorts by.
	var walks []kv.Query[issueRecord]

	for project := 1; project <= 3; project++ {
		for rank := range len(levels) + 1 {
			walks = append(walks, database.issues.By("seen", kv.U8(statusRank(store.StatusUnresolved)), kv.U32(uint32(project)), kv.U8(byte(rank))))
		}
	}

	var walked []string

	err = kv.Merge(database.db, walks, true, func(entry kv.Entry[issueRecord]) (bool, error) {
		walked = append(walked, entry.Record.ID)

		return true, nil
	})

	if err != nil {
		t.Fatal(err)
	}

	unresolved := slices.DeleteFunc(slices.Clone(table), func(issue issueRecord) bool { return issue.Status != store.StatusUnresolved })
	slices.SortFunc(unresolved, func(first, second issueRecord) int {
		if order := second.LastSeen.Compare(first.LastSeen); order != 0 {
			return order
		}

		// Ties by level, then ID, as the key orders them; the times here are all apart.
		return 0
	})

	want := make([]string, 0, len(unresolved))

	for _, issue := range unresolved {
		want = append(want, issue.ID)
	}

	if !slices.Equal(walked, want) {
		t.Errorf("unresolved newest seen first: walked %d issues, the table has %d\nwalked %v\nwant   %v", len(walked), len(want), walked, want)
	}

	// The ignores with a time are listed by when they end.
	var expiring []string

	_ = database.issues.By("expiry").Each(database.db, func(entry kv.Entry[issueRecord]) (bool, error) {
		expiring = append(expiring, entry.Record.ID)

		return true, nil
	})

	if len(expiring) != 10 {
		t.Errorf("ignores with a time: %d listed, want 10", len(expiring))
	}
}

// A second issue cannot take the fingerprint another holds in its project.
func TestIssueFingerprintIsUnique(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	received := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)

	first, err := database.SaveEvent(ctx, store.NewEvent{ProjectID: 1, EventID: fmt.Sprintf("%032x", 1), Fingerprint: "same", Title: "a", Level: "error",
		Timestamp: received, ReceivedAt: received, Payload: []byte(`{}`)})

	if err != nil {
		t.Fatal(err)
	}

	err = database.update(func(writer *batchWriter) error {
		copied := issueRecord{Issue: store.Issue{ID: "otherid1", ProjectID: 1, Level: "error", Status: store.StatusUnresolved}, Fingerprint: "same"}
		_, err := database.issues.Write(writer.batch, copied, nil)

		return err
	})

	var conflict kv.ErrConflict

	if !errors.As(err, &conflict) {
		t.Errorf("a second issue of fingerprint %q in project 1 (first %s): %v", "same", first.IssueID, err)
	}
}

// Lists and counts read through the indexes give what filtering and sorting
// every issue gives (matches, issueOrder), for every sort, the status,
// level, project, text and date filters, ignores whose time has passed, and
// pages.
func TestIssueListsMatchEveryIssue(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	random := rand.New(rand.NewPCG(3, 4))
	start := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	now := start

	database.SetClock(func() time.Time { return now })

	levels := []string{"fatal", "error", "warning", "info", "debug"}
	titles := []string{"TypeError: x", "RangeError: y", "fetch failed", "timeout"}

	var ids []string

	for index := range 300 {
		received := start.Add(time.Duration(random.IntN(5000)) * time.Minute)
		result, err := database.SaveEvent(ctx, store.NewEvent{
			ProjectID: 1 + random.IntN(3), EventID: fmt.Sprintf("%032x", index), Fingerprint: fmt.Sprintf("fp-%d", random.IntN(80)),
			Title: titles[random.IntN(len(titles))], Level: levels[random.IntN(len(levels))], Timestamp: received, ReceivedAt: received, Payload: []byte(`{}`),
		})

		if err != nil {
			t.Fatal(err)
		}

		if !slices.Contains(ids, result.IssueID) {
			ids = append(ids, result.IssueID)
		}
	}

	now = start.Add(4 * 24 * time.Hour)

	if _, err := database.SetIssuesStatus(ctx, ids[:10], store.StatusResolved, store.Actor{Source: store.SourceAPI}); err != nil {
		t.Fatal(err)
	}

	if _, err := database.IgnoreIssues(ctx, ids[10:15], store.IgnoreBound{For: time.Hour}, store.Actor{Source: store.SourceAPI}); err != nil {
		t.Fatal(err)
	}

	if _, err := database.IgnoreIssues(ctx, ids[15:18], store.IgnoreBound{}, store.Actor{Source: store.SourceAPI}); err != nil {
		t.Fatal(err)
	}

	// Two hours on: the five ignores for an hour read as unresolved now.
	now = now.Add(2 * time.Hour)

	all, err := database.issues.Primary().All(database.db)

	if err != nil {
		t.Fatal(err)
	}

	projects := map[int]string{1: "backend", 2: "frontend", 3: "api"}

	for id, name := range projects {
		if err := database.AddProjects(ctx, now, store.Project{ID: id, Name: name, PublicKey: fmt.Sprintf("%032d", id)}); err != nil {
			t.Fatal(err)
		}
	}

	want := func(filter store.IssueFilter) []string {
		var kept []*issueRecord

		for index := range all {
			issue := &all[index]

			switch matches(issue, filter, now) {
			case isMatch:
				kept = append(kept, issue)
			case checkEvents:
				if found, _ := database.anyEventInRange(issue.ID, filter); found {
					kept = append(kept, issue)
				}
			}
		}

		slices.SortFunc(kept, issueOrder(filter.Sort, projects))

		result := make([]string, 0, len(kept))

		for _, issue := range kept {
			result = append(result, issue.ID)
		}

		return result
	}

	var filters []store.IssueFilter

	for _, status := range []string{"", store.StatusUnresolved, store.StatusResolved, store.StatusIgnored} {
		filters = append(filters,
			store.IssueFilter{Status: status},
			store.IssueFilter{Status: status, Levels: []string{"error", "fatal"}},
			store.IssueFilter{Status: status, ProjectIDs: []int{2}},
			store.IssueFilter{Status: status, Query: "error"},
			store.IssueFilter{Status: status, SeenSince: start.Add(24 * time.Hour), SeenUntil: start.Add(48 * time.Hour)},
			store.IssueFilter{Status: status, SeenSince: start.Add(24 * time.Hour), SeenUntil: start.Add(48 * time.Hour), DateField: store.DateFirstSeen},
			store.IssueFilter{Status: status, SeenSince: start.Add(30 * time.Hour), SeenUntil: start.Add(31 * time.Hour)},
		)
	}

	for _, sort := range store.IssueSorts {
		for _, base := range filters {
			filter := base
			filter.Sort = sort
			expected := want(filter)

			var got []string

			for offset := 0; ; offset += 7 {
				filter.Offset, filter.Limit = offset, 7
				page, err := database.ListIssues(ctx, filter)

				if err != nil {
					t.Fatal(err)
				}

				for _, issue := range page {
					got = append(got, issue.ID)
				}

				if len(page) < 7 {
					break
				}
			}

			if !slices.Equal(got, expected) {
				t.Errorf("sort %q filter %+v:\ngot  %v\nwant %v", sort, base, got, expected)
			}

			if count, more, err := database.CountIssues(ctx, filter); err != nil || more || count != len(expected) {
				t.Errorf("count sort %q filter %+v: %d %v %v, want %d", sort, base, count, more, err, len(expected))
			}
		}
	}

	counts, err := database.UnresolvedCounts(ctx)

	if err != nil {
		t.Fatal(err)
	}

	for project := 1; project <= 3; project++ {
		if want := len(want(store.IssueFilter{Status: store.StatusUnresolved, ProjectIDs: []int{project}})); counts[project] != want {
			t.Errorf("unresolved of project %d: %d, want %d", project, counts[project], want)
		}
	}
}

// issueIDs lists, sorted, the stored issues wants accepts (tests).
func (database *Store) issueIDs(wants func(*issueRecord) bool) []string {
	ids := []string{}

	_ = database.issues.Primary().Each(database.db, func(entry kv.Entry[issueRecord]) (bool, error) {
		if wants(&entry.Record) {
			ids = append(ids, entry.Record.ID)
		}

		return true, nil
	})

	return ids
}

// A list's count stops at the depth a list pages to, as a list of logs does.
func TestIssueCountStopsAtTheLimit(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	limit := store.LogCountLimit
	store.LogCountLimit = 5

	t.Cleanup(func() { store.LogCountLimit = limit })

	for index := range 7 {
		received := time.Date(2026, 11, 10, 8, index, 0, 0, time.UTC)

		if _, err := database.SaveEvent(ctx, store.NewEvent{ProjectID: 1, EventID: fmt.Sprintf("%032x", index), Fingerprint: fmt.Sprint(index),
			Title: "error", Level: "error", Timestamp: received, ReceivedAt: received, Payload: []byte(`{}`)}); err != nil {
			t.Fatal(err)
		}
	}

	if count, more, err := database.CountIssues(ctx, store.IssueFilter{}); err != nil || count != 5 || !more {
		t.Errorf("count %d, more %v, %v: want 5 and more", count, more, err)
	}

	if counts, err := database.UnresolvedCounts(ctx); err != nil || counts[1] != 7 {
		t.Errorf("unresolved %v %v: the menu's count stays exact", counts, err)
	}
}

// issueComparison is the order of a sort over issues held in memory, the
// reference the index walks are checked against.
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
