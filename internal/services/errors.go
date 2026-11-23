package services

import (
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/devforth/exceptic/internal/storage/store"
)

// The refusals of the services, each its own meaning for the transports
// (api.Outcome): the role (ErrForbidden), a record of a project outside the
// viewer's (ErrHidden), a record in the way (ErrConflict). A missing record
// is store.ErrNotFound.
var (
	// ErrForbidden: the viewer's role may not do this; Forbidden gives the reason.
	ErrForbidden = errors.New("your role may not do this")
	// ErrHidden: the record exists, in a project outside the viewer's access.
	ErrHidden = errors.New("not in your projects")
	// ErrConflict: a stored record stands in the way of the change.
	ErrConflict = errors.New("conflict")
)

// Reason is a refusal of Kind (ErrForbidden, ErrHidden, ErrConflict or
// store.ErrNotFound) with the text the client is told.
type Reason struct {
	Kind error
	Text string
}

func (err Reason) Error() string        { return err.Text }
func (err Reason) Is(target error) bool { return target == err.Kind }

// Forbidden is ErrForbidden with its reason: "your role may not delete issues".
func Forbidden(reason string) error {
	return Reason{Kind: ErrForbidden, Text: reason}
}

// Hidden refuses records of projects outside the viewer's access: what
// was asked for ("issue", "project") and the ids or names the caller gave,
// never anything of the records themselves.
func Hidden(what string, ids ...string) error {
	subject, verb := named(what, ids)

	return Reason{Kind: ErrHidden, Text: subject + " " + verb + " not in your projects"}
}

// Missing is store.ErrNotFound naming what was asked for.
func Missing(what string, ids ...string) error {
	subject, _ := named(what, ids)

	return Reason{Kind: store.ErrNotFound, Text: "not found: " + subject}
}

func named(what string, ids []string) (subject, verb string) {
	if len(ids) > 1 {
		return what + "s " + strings.Join(ids, ", "), "are"
	}

	return strings.TrimSpace(what + " " + strings.Join(ids, ", ")), "is"
}

// Refusal is a form refused for a reason its fields alone cannot fix:
// Kind is ErrForbidden (what the viewer may not give) or ErrConflict (a
// record in the way); Problems say it at each field, the others included.
type Refusal struct {
	Kind     error
	Problems Problems
}

func (refusal Refusal) Error() string {
	var lines []string

	for _, field := range slices.Sorted(maps.Keys(refusal.Problems)) {
		lines = append(lines, field+": "+refusal.Problems[field])
	}

	return strings.Join(lines, "; ")
}

func (refusal Refusal) Is(target error) bool { return target == refusal.Kind }
