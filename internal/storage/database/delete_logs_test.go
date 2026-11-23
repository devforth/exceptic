package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

func logIDs(t *testing.T, database *Store, filter store.LogFilter) map[string]int64 {
	t.Helper()

	logs, err := database.ListLogs(context.Background(), filter)

	if err != nil {
		t.Fatal(err)
	}

	ids := map[string]int64{}

	for _, record := range logs {
		ids[fmt.Sprintf("%d %s", record.ProjectID, record.Body)] = record.ID
	}

	return ids
}

// Chosen logs go by id, whatever their level; missing and repeated ids are skipped.
func TestDeleteLogs(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	at := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)

	if err := database.SaveLogs(ctx, []store.NewLog{
		{ProjectID: 1, Timestamp: at, Level: "info", Body: "kept", Fingerprint: "kept"},
		{ProjectID: 1, Timestamp: at, Level: "error", Body: "gone error", Fingerprint: "error"},
		{ProjectID: 2, Timestamp: at, Level: "debug", Body: "gone debug", Fingerprint: "debug"},
	}, 0); err != nil {
		t.Fatal(err)
	}

	ids := logIDs(t, database, store.LogFilter{})
	deleted, err := database.DeleteLogs(ctx, []int64{ids["1 gone error"], ids["2 gone debug"], ids["2 gone debug"], 999_999, 0, -1})

	if err != nil || deleted != 2 {
		t.Fatalf("deleted %d, %v", deleted, err)
	}

	if left := logIDs(t, database, store.LogFilter{}); len(left) != 1 || left["1 kept"] == 0 {
		t.Errorf("left %v", left)
	}

	if usage, err := database.ProjectUsage(ctx); err != nil || usage.Bytes[2] != 0 {
		t.Errorf("project 2 still counts %d bytes (%v)", usage.Bytes[2], err)
	}
}
