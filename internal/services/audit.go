package services

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

// Audit is the log of the changes admins and users made, kept for good.
type Audit struct {
	logger *slog.Logger
	store  store.Store
	now    func() time.Time
}

func newAudit(logger *slog.Logger, database store.Store) *Audit {
	return &Audit{logger: logger, store: database, now: time.Now}
}

// Record stores entries of a change that was made: a failure is logged,
// never the change's, which stands.
func (audit *Audit) Record(ctx context.Context, entries ...store.AuditEntry) {
	if len(entries) == 0 {
		return
	}

	now := audit.now()

	for index := range entries {
		entries[index].CreatedAt = now
	}

	if err := audit.store.AppendAudit(context.WithoutCancel(ctx), entries); err != nil {
		audit.logger.Error("audit entries not stored", "resource", entries[0].Resource, "action", entries[0].Action, "entries", len(entries), "err", err)
	}
}

// List is the entries of filter the viewer sees, newest first: an entry
// whose projects are not all within the viewer's, or with none, is left out
// unless they have every project; a user they do not see on the Users page
// is shown only by role (store.AuditFilter.Actors).
func (audit *Audit) List(ctx context.Context, viewer store.User, filter store.AuditFilter) ([]store.AuditEntry, store.AuditFilter, error) {
	filter, err := audit.within(ctx, viewer, filter)

	if err != nil {
		return nil, filter, err
	}

	entries, err := audit.store.ListAudit(ctx, filter)

	for index := range entries {
		entries[index] = entries[index].ShownTo(filter.Access)
	}

	return entries, filter, err
}

// Get is one entry as List shows it to the viewer; ErrHidden for one they
// may not see.
func (audit *Audit) Get(ctx context.Context, viewer store.User, id int64) (store.AuditEntry, store.AuditFilter, error) {
	filter, err := audit.within(ctx, viewer, store.AuditFilter{})

	if err != nil {
		return store.AuditEntry{}, filter, err
	}

	entry, err := audit.store.GetAudit(ctx, id)

	switch {
	case err != nil:
		return entry, filter, err
	case !filter.Matches(entry):
		return store.AuditEntry{}, filter, Hidden("audit entry", strconv.FormatInt(id, 10))
	}

	return entry.ShownTo(filter.Access), filter, nil
}

// Count counts what List would page through, up to store.LogCountLimit (more).
func (audit *Audit) Count(ctx context.Context, viewer store.User, filter store.AuditFilter) (int, bool, error) {
	filter, err := audit.within(ctx, viewer, filter)

	if err != nil {
		return 0, false, err
	}

	return audit.store.CountAudit(ctx, filter, store.LogCountLimit)
}

// Choices are the values of the entries of the projects the viewer sees
// (of none only with every project) and the users they see: the choices of
// the audit log's filters, from the counts. They are counted per project,
// so for a limited viewer an entry that also spans a project they lack (and
// is therefore hidden) can offer a value that filters to nothing; rare, and
// accepted.
func (audit *Audit) Choices(ctx context.Context, viewer store.User) (store.AuditChoices, error) {
	var projectIDs []int

	if !viewer.ProjectAccess().Restrict(&projectIDs) {
		return store.AuditChoices{}, nil
	}

	filter, err := audit.within(ctx, viewer, store.AuditFilter{})

	if err != nil {
		return store.AuditChoices{}, err
	}

	choices, err := audit.store.AuditChoices(ctx, projectIDs)

	choices.UserIDs = slices.DeleteFunc(choices.UserIDs, func(id string) bool {
		return !filter.ShowsActor(store.AuditActor{UserID: id})
	})

	return choices, err
}

// within limits filter to the viewer: their projects, and for one who does
// not administer the users they see.
func (audit *Audit) within(ctx context.Context, viewer store.User, filter store.AuditFilter) (store.AuditFilter, error) {
	filter.Access = viewer.ProjectAccess()
	filter.Actors = nil

	if viewer.CanAdminister() {
		return filter, nil
	}

	users, err := audit.store.ListUsers(ctx)

	if err != nil {
		return filter, err
	}

	filter.Actors = map[string]bool{viewer.ID: true}

	for _, user := range users {
		if viewer.SeesUser(user) {
			filter.Actors[user.ID] = true
		}
	}

	return filter, nil
}

// secretWords name the fields AuditFields drops, whatever a caller passes:
// secrets never enter the audit log (store.AuditReplaced says one changed).
var secretWords = []string{"password", "token", "secret", "webhook", "code", "key"}

// AuditFields is value's top-level JSON fields without those named like a
// secret; nil for nil.
func AuditFields(value any) (map[string]json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}

	encoded, err := json.Marshal(value)

	if err != nil {
		return nil, err
	}

	fields := map[string]json.RawMessage{}

	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}

	maps.DeleteFunc(fields, func(name string, _ json.RawMessage) bool {
		return slices.ContainsFunc(secretWords, func(word string) bool { return strings.Contains(strings.ToLower(name), word) })
	})

	return fields, nil
}

// AuditDiff is the fields of before and after that differ: only after's
// when before is nil (a create), only before's when after is nil (a delete).
func AuditDiff(before, after any) (map[string]json.RawMessage, map[string]json.RawMessage, error) {
	old, err := AuditFields(before)

	if err != nil {
		return nil, nil, err
	}

	changed, err := AuditFields(after)

	if err != nil || old == nil || changed == nil {
		return old, changed, err
	}

	for name, value := range old {
		if bytes.Equal(value, changed[name]) {
			delete(old, name)
			delete(changed, name)
		}
	}

	return old, changed, nil
}
