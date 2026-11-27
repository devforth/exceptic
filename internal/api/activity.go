package api

import (
	"context"
	"slices"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

type activityInput struct {
	UserID string `json:"user_id,omitempty" jsonschema:"id from list_users; empty: yourself."`
}

type loginSessionOutput struct {
	ID           string    `json:"id"`
	Method       string    `json:"method,omitempty"`
	Address      string    `json:"address,omitempty"`
	Browser      string    `json:"browser,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	// Current: the session this request came with.
	Current bool `json:"current,omitempty"`
}

type signInOutput struct {
	At      time.Time `json:"at"`
	Method  string    `json:"method"`
	Result  string    `json:"result"`
	Address string    `json:"address,omitempty"`
	Browser string    `json:"browser,omitempty"`
}

type activityOutput struct {
	Sessions       []loginSessionOutput `json:"sessions"`
	SignIns        []signInOutput       `json:"sign_ins"`
	GoogleLinkedAt *time.Time           `json:"google_linked_at,omitempty"`
}

func (handler *handlers) getActivity(ctx context.Context, call Call, input activityInput) (activityOutput, error) {
	activity, err := handler.services.Users.Activity(ctx, call.Viewer, input.UserID)

	if err != nil {
		return activityOutput{}, err
	}

	output := activityOutput{Sessions: []loginSessionOutput{}, SignIns: []signInOutput{}, GoogleLinkedAt: activity.GoogleLinkedAt}
	own := input.UserID == "" || input.UserID == call.Viewer.ID

	for _, session := range activity.Sessions {
		output.Sessions = append(output.Sessions, loginSessionOutput{
			ID: session.ID, Method: session.Method, Address: session.Address, Browser: session.Browser,
			CreatedAt: session.CreatedAt, LastActiveAt: session.LastActiveAt, ExpiresAt: session.ExpiresAt,
			Current: own && session.ID == call.Session,
		})
	}

	for _, signIn := range activity.SignIns {
		output.SignIns = append(output.SignIns, signInOutput{At: signIn.At, Method: signIn.Method, Result: signIn.Result, Address: signIn.Address, Browser: signIn.Browser})
	}

	return output, nil
}

type endSessionInput struct {
	UserID    string `json:"user_id,omitempty" jsonschema:"id from list_users; empty: yourself."`
	SessionID string `json:"session_id" jsonschema:"id from get_activity."`
}

// endSession ends a session; the audit log records one of another user.
func (handler *handlers) endSession(ctx context.Context, call Call, input endSessionInput) (NoContent, error) {
	other := input.UserID != "" && input.UserID != call.Viewer.ID

	var (
		target   store.User
		sessions []store.Session
	)

	if other {
		// Read for the audit log; EndSession checks the rest.
		target, _ = handler.services.Users.Get(ctx, input.UserID)

		if activity, err := handler.services.Users.Activity(ctx, call.Viewer, input.UserID); err == nil {
			sessions = activity.Sessions
		}
	}

	if err := handler.services.Users.EndSession(ctx, call.Viewer, input.UserID, input.SessionID, call.Session); err != nil {
		return NoContent{}, err
	}

	if other {
		before := map[string]any{"user": target.Email}

		if index := slices.IndexFunc(sessions, func(session store.Session) bool { return session.ID == input.SessionID }); index >= 0 {
			session := sessions[index]
			before["method"], before["address"], before["browser"], before["created_at"] = session.Method, session.Address, session.Browser, session.CreatedAt
		}

		handler.audit(ctx, call, auditChange{
			Resource: store.AuditSession, Action: store.AuditDelete, RecordID: input.SessionID, RecordName: target.Email,
			ProjectIDs: userProjects(target), Before: before,
		})
	}

	return NoContent{}, nil
}

// unlinkGoogle unlinks a Google account; the audit log records one of another user.
func (handler *handlers) unlinkGoogle(ctx context.Context, call Call, input activityInput) (NoContent, error) {
	target, _ := handler.services.Users.Get(ctx, input.UserID)

	if err := handler.services.Users.UnlinkGoogle(ctx, call.Viewer, input.UserID, call.Session); err != nil {
		return NoContent{}, err
	}

	if input.UserID != "" && input.UserID != call.Viewer.ID {
		handler.audit(ctx, call, userChange(store.AuditUnlinkGoogle, target,
			map[string]bool{"google_linked": target.GoogleSub != ""}, map[string]bool{"google_linked": false}))
	}

	return NoContent{}, nil
}
