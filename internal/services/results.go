package services

import (
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

// The results the services give and the inputs they take, shared by the UI
// (its templates show them) and the agents' tools.

// Problems are invalid fields with their messages, keyed by field name.
type Problems map[string]string

// SavedConnection is the outcome of Connections.Save: the connection as read
// from the form, the field errors, or the connection that already sends to
// the same place. Nothing is saved when Problems is not empty.
type SavedConnection struct {
	Connection store.Connection
	Problems   Problems
	Duplicate  *Duplicate
}

// AgentsPage is what the MCP page lists.
type AgentsPage struct {
	Owner  store.User   // whose agents are listed
	Owners []store.User // whom the viewer may choose (only themselves for most)
	Grants []store.Grant
}

// The states of EmailStep.
const (
	EmailInput       = "input"       // enter an address
	EmailUnconfirmed = "unconfirmed" // a saved connection's address waits for a code
	EmailCode        = "code"        // enter the mailed code
	EmailVerified    = "verified"    // confirmed: the rest of the form opens
)

// EmailStep is the address part of the form, swapped by htmx as it moves on
// through its State. Fixed: a saved connection, whose address cannot change.
type EmailStep struct {
	State         string
	Address       string
	DestinationID string
	VerifiedAt    time.Time
	Fixed         bool
	Duplicate     *Duplicate // the connection that already uses the address
	Error         string
	Notice        string
}

func (step EmailStep) Input() bool { return step.State == EmailInput }

func (step EmailStep) Unconfirmed() bool { return step.State == EmailUnconfirmed }

func (step EmailStep) AwaitingCode() bool { return step.State == EmailCode }

func (step EmailStep) Verified() bool { return step.State == EmailVerified }

// Duplicate names the connection that already sends to an address or chat,
// so the error can link to it.
type Duplicate struct {
	ID   string
	Name string
}

// ChatTest is the result of a test message sent from the form.
type ChatTest struct {
	Sent    bool
	Message string
}

// UserInput is what the user form sends.
type UserInput struct {
	Username string
	Name     string
	Role     string
}
