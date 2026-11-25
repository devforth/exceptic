package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// A connection gets a cleanup run's notice with the rows of its projects by
// name and the rest as Other, as the Cleanup history shows it; a project's
// run none outside it.
func TestCleanupNoticeWithinAccess(t *testing.T) {
	projectList := config.Projects{{ID: 1, Name: "backend"}, {ID: 2, Name: "frontend"}}
	run := store.CleanupRun{Steps: []store.CleanupStep{
		{Number: 5, Projects: map[int]*store.CleanupCounts{2: {Logs: 25458}}},
		{Number: 6, Projects: map[int]*store.CleanupCounts{1: {Events: 1302}, 2: {Events: 231}}},
	}}
	build := cleanupNotice(run, projectList, nil, "")

	backend, ok := build(store.OnlyProjects(1))
	text, _ := json.Marshal(backend)

	// Step 5 deleted only frontend's logs, step 6 both projects' events.
	if !ok || len(backend.Table.Rows) != 3 || strings.Contains(string(text), "frontend") ||
		backend.Table.Rows[0][1].Text != "Other" || backend.Table.Rows[1][1].Text != "backend" || backend.Table.Rows[2][1].Text != "Other" {
		t.Errorf("backend and Other: %s", text)
	}

	if every, ok := build(store.Access{}); !ok || len(every.Table.Rows) != 3 {
		t.Errorf("every project: %+v", every.Table)
	}

	run.Project, run.Share = 2, 30

	if _, ok := cleanupNotice(run, projectList, nil, "")(store.OnlyProjects(1)); ok {
		t.Error("another project's run reached a connection of backend")
	}
}
