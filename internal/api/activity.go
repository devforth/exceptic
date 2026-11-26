package api

import (
	"context"
	"time"
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

func (handler *handlers) endSession(ctx context.Context, call Call, input endSessionInput) (NoContent, error) {
	return NoContent{}, handler.services.Users.EndSession(ctx, call.Viewer, input.UserID, input.SessionID, call.Session)
}

func (handler *handlers) unlinkGoogle(ctx context.Context, call Call, input activityInput) (NoContent, error) {
	return NoContent{}, handler.services.Users.UnlinkGoogle(ctx, call.Viewer, input.UserID, call.Session)
}
