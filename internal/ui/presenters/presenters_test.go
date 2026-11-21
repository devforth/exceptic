package presenters

import (
	"fmt"
	"slices"
	"testing"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/internal/ui/lists"
)

func testConnections() Connections {
	return Connections{
		// SMTP but no SECRET_KEY: only email can be used.
		Notifications: config.Notifications{SMTP: config.SMTP{Host: "smtp.example.com"}},
		Projects:      config.Projects{{ID: 1, Name: "backend"}},
		ProjectName:   func(projectID int) string { return fmt.Sprintf("project %d", projectID) },
	}
}

func TestConnectionStatus(t *testing.T) {
	presenter := testConnections()

	for _, test := range []struct {
		verified, active bool
		status, class    string
	}{
		{false, true, "awaiting confirmation", "awaiting"},
		{true, true, "active", "active"},
		{true, false, "inactive", "inactive"},
	} {
		row := presenter.Row(store.Connection{Type: config.ChannelEmail, Address: "dev@example.com", Verified: test.verified, Active: test.active, ProjectIDs: []int{7}})

		if row.Status != test.status || row.StatusClass != test.class {
			t.Errorf("verified %v, active %v: status %q (%s), want %q (%s)", test.verified, test.active, row.Status, row.StatusClass, test.status, test.class)
		}

		if row.Destination != "dev@example.com" || len(row.Projects) != 1 || row.Projects[0] != "project 7" {
			t.Errorf("row = %+v, want the address and the project name", row)
		}
	}
}

func TestConnectionLevelsHighestFirst(t *testing.T) {
	presenter := testConnections()
	connection := store.Connection{Type: config.ChannelEmail, Levels: []string{"warning", "fatal", "error"}}

	if levels := presenter.Row(connection).Levels; levels != "fatal, error, warning" {
		t.Errorf("row levels = %q, want highest first", levels)
	}

	var names []string

	for _, level := range presenter.Form(connection).Levels {
		names = append(names, level.Value)
	}

	if !slices.Equal(names, lists.EventLevels) {
		t.Errorf("form levels = %v, want %v", names, lists.EventLevels)
	}
}

func TestEmailStepState(t *testing.T) {
	presenter := testConnections()

	for _, test := range []struct {
		name       string
		connection store.Connection
		state      string
		fixed      bool
	}{
		{"new", store.Connection{Type: config.ChannelEmail}, "input", false},
		{"existing, not confirmed", store.Connection{ID: "con00003", Type: config.ChannelEmail}, "unconfirmed", true},
		{"existing, confirmed", store.Connection{ID: "con00003", Type: config.ChannelEmail, Verified: true}, "verified", true},
	} {
		view := presenter.Form(test.connection)

		if view.Email.State != test.state || view.Email.Fixed != test.fixed {
			t.Errorf("%s: state %q, fixed %v; want %q, %v", test.name, view.Email.State, view.Email.Fixed, test.state, test.fixed)
		}
	}

	// A new email form shown again: a confirmed address stays confirmed, a
	// taken one goes back to the address field with a link.
	confirmed := presenter.FormAgain(store.Connection{Type: config.ChannelEmail, DestinationID: "dst00004"}, map[string]string{"levels": "x"}, "", nil, "dev@example.com")

	if confirmed.Email.State != "verified" || confirmed.Email.Address != "dev@example.com" || confirmed.Errors["levels"] == "" {
		t.Errorf("form again = %+v, want the confirmed step and the errors", confirmed.Email)
	}

	existing := &services.Duplicate{ID: "con00009", Name: "dev"}
	taken := presenter.FormAgain(store.Connection{Type: config.ChannelEmail, Address: "dev@example.com", DestinationID: "dst00004"}, nil, "", existing, "dev@example.com")

	if taken.Email.State != "input" || taken.Email.Duplicate != existing || taken.Duplicate != nil {
		t.Errorf("taken address: step %+v, form duplicate %v; want the link in the email step", taken.Email, taken.Duplicate)
	}
}

func TestNewConnectionDefaults(t *testing.T) {
	presenter := testConnections()

	// Without SECRET_KEY only email can be used: it is preselected and gets issue links.
	connection := presenter.NewConnection()

	if connection.Type != config.ChannelEmail || connection.HideIssueLink || !connection.AllProjects || !connection.Active {
		t.Errorf("new connection = %+v, want email, all projects, active, with issue links", connection)
	}

	presenter.Notifications.SMTP = config.SMTP{}
	presenter.Notifications.SecretKey = "test-secret-key-test-secret-key-0123"

	if chat := presenter.NewConnection(); chat.Type != config.ChannelTelegram || !chat.HideIssueLink {
		t.Errorf("new connection without SMTP = %+v, want a chat without issue links", chat)
	}

	// The levels are those recorded by default (RECORD_EVENT_LEVELS); all when all are recorded.
	presenter.Recording = config.Recording{EventLevels: []string{"warning", "error"}}

	if levels := presenter.NewConnection().Levels; !slices.Equal(levels, []string{"warning", "error"}) {
		t.Errorf("levels = %v, want the recorded ones", levels)
	}

	presenter.Recording = config.Recording{}

	if levels := presenter.NewConnection().Levels; !slices.Equal(levels, config.Levels) {
		t.Errorf("levels = %v, want every level", levels)
	}
}
