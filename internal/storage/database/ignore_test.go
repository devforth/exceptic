package database

import (
	"context"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

func ignore(t *testing.T, database *Store, issueID string, bound store.IgnoreBound) store.Issue {
	t.Helper()

	issues, err := database.IgnoreIssues(context.Background(), []string{issueID}, bound, store.Actor{Name: "admin", Source: store.SourceUI})

	if err != nil {
		t.Fatal(err)
	}

	return issues[0]
}

func unresolvedNow(t *testing.T, database *Store) (count int, perProject int) {
	t.Helper()

	ctx := context.Background()
	count, err := database.CountIssues(ctx, store.IssueFilter{Status: store.StatusUnresolved})

	if err != nil {
		t.Fatal(err)
	}

	counts, err := database.UnresolvedCounts(ctx)

	if err != nil {
		t.Fatal(err)
	}

	return count, counts[1]
}

// An issue ignored for a time reads as unresolved once it has passed, in
// lists, counts and its history, without a write; the next event stores the
// change at the time it happened and reports a regression.
func TestIgnoreForATime(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	start := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	now := start

	database.SetClock(func() time.Time { return now })
	result := saveEvent(t, database, "timed", start)

	issue := ignore(t, database, result.IssueID, store.IgnoreBound{For: time.Hour})

	if issue.Status != store.StatusIgnored || issue.IgnoredUntil == nil || !issue.IgnoredUntil.Equal(start.Add(time.Hour)) {
		t.Fatalf("ignored: %+v", issue)
	}

	if saveEvent(t, database, "timed", start.Add(30*time.Minute)).Regression {
		t.Fatal("an event within the time reopened the issue")
	}

	if count, perProject := unresolvedNow(t, database); count != 0 || perProject != 0 {
		t.Fatalf("unresolved within the time: %d, %d", count, perProject)
	}

	now = start.Add(2 * time.Hour)

	if count, perProject := unresolvedNow(t, database); count != 1 || perProject != 1 {
		t.Fatalf("unresolved after the time: %d, %d", count, perProject)
	}

	listed, err := database.ListIssues(ctx, store.IssueFilter{Status: store.StatusUnresolved})

	if err != nil || len(listed) != 1 || listed[0].IgnoredUntil != nil {
		t.Fatalf("list after the time: %+v %v", listed, err)
	}

	if read, err := database.GetIssue(ctx, result.IssueID); err != nil || read.Status != store.StatusUnresolved {
		t.Fatalf("get after the time: %+v %v", read, err)
	}

	if ignored, _ := database.CountIssues(ctx, store.IssueFilter{Status: store.StatusIgnored}); ignored != 0 {
		t.Fatalf("still listed as ignored: %d", ignored)
	}

	history, err := database.IssueStatusHistory(ctx, result.IssueID)

	if err != nil || len(history) != 2 || !history[0].ByExpiry() || history[0].To != store.StatusUnresolved || !history[0].CreatedAt.Equal(start.Add(time.Hour)) {
		t.Fatalf("history before a write: %+v %v", history, err)
	}

	if !saveEvent(t, database, "timed", now).Regression {
		t.Fatal("the first event after the time was no regression")
	}

	if stored := storedIssue(database, "timed"); stored.Status != store.StatusUnresolved || stored.IgnoredUntil != nil {
		t.Fatalf("stored after the event: %+v", stored.Issue)
	}

	history, err = database.IssueStatusHistory(ctx, result.IssueID)

	if err != nil || len(history) != 2 || !history[0].ByExpiry() || !history[0].CreatedAt.Equal(start.Add(time.Hour)) {
		t.Fatalf("history after the event: %+v %v", history, err)
	}

	if saveEvent(t, database, "timed", now.Add(time.Minute)).Regression {
		t.Fatal("an unresolved issue regressed again")
	}
}

// An issue ignored until N more events comes back with the Nth, events
// counted over the cap of a minute included, stored and reported as a
// regression with the event in its history.
func TestIgnoreUntilEvents(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	start := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	send := sendCapped(t, database, 2)

	database.SetClock(func() time.Time { return start })

	first := send("counted", start, 1)[0]

	issue := ignore(t, database, first.IssueID, store.IgnoreBound{Events: 5})

	if issue.IgnoredUntilCount != 6 || issue.IgnoreEventsLeft() != 5 {
		t.Fatalf("ignored: %+v, left %d", issue, issue.IgnoreEventsLeft())
	}

	results := send("counted", start.Add(10*time.Second), 4)

	for _, result := range results {
		if result.Regression {
			t.Fatalf("came back too early: %+v", results)
		}
	}

	if !results[3].NotStored {
		t.Fatalf("the cap did not apply: %+v", results)
	}

	if left := storedIssue(database, "counted").IgnoreEventsLeft(); left != 1 {
		t.Fatalf("events left %d", left)
	}

	last := send("counted", start.Add(20*time.Second), 1)[0]

	if !last.Regression || last.NotStored {
		t.Fatalf("the 5th event: %+v", last)
	}

	stored := storedIssue(database, "counted")

	if stored.Status != store.StatusUnresolved || stored.IgnoredUntilCount != 0 {
		t.Fatalf("stored: %+v", stored.Issue)
	}

	history, err := database.IssueStatusHistory(ctx, first.IssueID)

	if err != nil || len(history) != 2 || !history[0].ByEvent() || history[0].EventID == "" || history[0].From != store.StatusIgnored {
		t.Fatalf("history: %+v %v", history, err)
	}
}

// Ignoring without a bound stays ignored, and any other status, or ignoring
// again, replaces the bound.
func TestIgnoreBoundsEnd(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	start := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	now := start

	database.SetClock(func() time.Time { return now })
	result := saveEvent(t, database, "plain", start)
	other := saveEvent(t, database, "other", start.Add(time.Second))

	ignore(t, database, result.IssueID, store.IgnoreBound{})

	now = start.Add(400 * 24 * time.Hour)

	for index := range 3 {
		if saveEvent(t, database, "plain", now.Add(time.Duration(index)*time.Second)).Regression {
			t.Fatal("a plain ignore ended")
		}
	}

	if read, _ := database.GetIssue(ctx, result.IssueID); read.Status != store.StatusIgnored || read.IgnoredUntil != nil || read.IgnoredUntilCount != 0 {
		t.Fatalf("plain ignore: %+v", read)
	}

	ignore(t, database, result.IssueID, store.IgnoreBound{For: time.Hour, Events: 10})
	plain := ignore(t, database, result.IssueID, store.IgnoreBound{})

	if plain.IgnoredUntil != nil || plain.IgnoredUntilCount != 0 {
		t.Fatalf("ignoring again kept the bound: %+v", plain)
	}

	ignore(t, database, other.IssueID, store.IgnoreBound{For: time.Hour, Events: 10})

	for _, status := range []string{store.StatusResolved, store.StatusUnresolved} {
		ignore(t, database, other.IssueID, store.IgnoreBound{For: time.Hour, Events: 10})

		issues, err := database.SetIssuesStatus(ctx, []string{other.IssueID}, status, store.Actor{Source: store.SourceAPI})

		if err != nil || issues[0].IgnoredUntil != nil || issues[0].IgnoredUntilCount != 0 {
			t.Fatalf("%s kept the bound: %+v %v", status, issues, err)
		}
	}

	if _, err := database.IgnoreIssues(ctx, []string{other.IssueID}, store.IgnoreBound{Events: store.MaxIgnoreEvents + 1}, store.Actor{}); err == nil {
		t.Fatal("a bound over the most events was taken")
	}
}

// A status change on an issue whose ignore time has passed stores the end
// of the ignore first, at its time.
func TestStatusChangeAfterIgnoreTime(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	start := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	now := start

	database.SetClock(func() time.Time { return now })
	result := saveEvent(t, database, "later", start)

	ignore(t, database, result.IssueID, store.IgnoreBound{For: time.Hour})
	now = start.Add(3 * time.Hour)

	if _, err := database.SetIssuesStatus(ctx, []string{result.IssueID}, store.StatusResolved, store.Actor{Source: store.SourceAPI}); err != nil {
		t.Fatal(err)
	}

	history, err := database.IssueStatusHistory(ctx, result.IssueID)

	if err != nil || len(history) != 3 || history[0].To != store.StatusResolved || history[0].From != store.StatusUnresolved || !history[1].ByExpiry() {
		t.Fatalf("history: %+v %v", history, err)
	}
}

// Events received before an ignore until N more events (an inbox backlog)
// count only: the Nth event received after it ends the ignore.
func TestIgnoreUntilEventsAfterTheIgnore(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	start := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	ignoredAt := start.Add(time.Minute)

	database.SetClock(func() time.Time { return ignoredAt })
	first := saveEvent(t, database, "backlog", start)
	ignore(t, database, first.IssueID, store.IgnoreBound{Events: 3})

	for index := range 5 {
		if saveEvent(t, database, "backlog", start.Add(time.Duration(index+1)*time.Second)).Regression {
			t.Fatalf("backlog event %d ended the ignore", index+1)
		}
	}

	if left := storedIssue(database, "backlog").IgnoreEventsLeft(); left != 3 {
		t.Fatalf("events left after the backlog: %d", left)
	}

	for index := range 3 {
		result := saveEvent(t, database, "backlog", ignoredAt.Add(time.Duration(index+1)*time.Second))

		if result.Regression != (index == 2) {
			t.Fatalf("event %d after the ignore: regression %v", index+1, result.Regression)
		}
	}

	history, err := database.IssueStatusHistory(ctx, first.IssueID)

	if err != nil || len(history) != 2 || !history[0].ByEvent() || !history[0].CreatedAt.After(history[1].CreatedAt) {
		t.Fatalf("history: %+v %v", history, err)
	}
}

// The cleanup judges an issue whose ignore time has passed as unresolved,
// as every read shows it, and keeps its events.
func TestCleanupAfterIgnoreTime(t *testing.T) {
	database := openTest(t)
	start := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	now := start

	database.SetClock(func() time.Time { return now })

	for index := range 4 {
		saveEvent(t, database, "expired", start.Add(time.Duration(index)*time.Second))
	}

	ignore(t, database, storedIssue(database, "expired").ID, store.IgnoreBound{For: time.Hour})
	now = start.Add(48 * time.Hour)

	if _, finished := runBatch(t, database, 4, nil, 0); !finished || storedEvents(database, "expired") != 4 {
		t.Fatalf("expired ignore keeps %d events (finished %v)", storedEvents(database, "expired"), finished)
	}
}
