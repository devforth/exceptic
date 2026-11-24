package database

import (
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

	database.issuesMutex.RLock()
	table := make([]issueRecord, 0, len(database.table.byID))

	for _, issue := range database.table.byID {
		table = append(table, *issue)
	}

	database.issuesMutex.RUnlock()

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

	err := kv.Merge(database.db, walks, true, func(entry kv.Entry[issueRecord]) (bool, error) {
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
