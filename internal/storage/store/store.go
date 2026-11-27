// Package store is the API of the storage: the records, their rules and the
// Store interface, the one list of every operation but the ingest inbox, the
// periodic jobs and backups, which only ingest and app call on database.Store.
package store

import (
	"context"
	"time"
)

// Store reads and writes the records of the application.
type Store interface {
	// Agents: OAuth clients, codes, grants, personal tokens.

	RegisterOAuthClient(ctx context.Context, client OAuthClient) error
	GetOAuthClient(ctx context.Context, clientID string) (OAuthClient, error)
	CreateOAuthCode(ctx context.Context, codeHash string, code OAuthCode, now time.Time) error
	// TakeOAuthCode returns a code and deletes it: a code works once.
	TakeOAuthCode(ctx context.Context, codeHash string, now time.Time) (OAuthCode, error)
	CreateGrant(ctx context.Context, grant Grant, tokens GrantTokens) (string, error)
	// ReplacePersonalToken keeps one personal token per user.
	ReplacePersonalToken(ctx context.Context, userID string, accessHash string, now time.Time) error
	// RotateGrant swaps a grant's tokens; a spent refresh token revokes the grant.
	RotateGrant(ctx context.Context, clientID, refreshHash string, tokens GrantTokens, now time.Time) error
	// UseGrant finds the grant of a valid access token and records the use.
	UseGrant(ctx context.Context, accessHash string, now time.Time) (Grant, error)
	ListGrants(ctx context.Context, userID string, now time.Time) ([]Grant, error)
	GetGrant(ctx context.Context, id string) (Grant, error)
	DeleteGrant(ctx context.Context, id string) error
	HasPersonalToken(ctx context.Context, userID string) (bool, error)

	// Notification destinations and connections.

	CreateDestination(ctx context.Context, destination Destination, now time.Time) (string, error)
	GetDestination(ctx context.Context, id string) (Destination, error)
	// FindEmailDestination prefers a confirmed destination of the address.
	FindEmailDestination(ctx context.Context, address string) (Destination, error)
	UpdateDestinationSecret(ctx context.Context, id string, secret string) error
	SetVerificationCode(ctx context.Context, destinationID string, codeHash string, expiresAt, now time.Time) error
	// CheckVerificationCode counts an attempt; a match confirms the destination.
	CheckVerificationCode(ctx context.Context, destinationID string, codeHash string, now time.Time) (VerifyResult, error)
	ListConnections(ctx context.Context) ([]Connection, error)
	GetConnection(ctx context.Context, id string) (Connection, error)
	CreateConnection(ctx context.Context, connection Connection, now time.Time) (string, error)
	UpdateConnection(ctx context.Context, connection Connection) error
	// DeleteConnection keeps the destination, so an address stays confirmed.
	DeleteConnection(ctx context.Context, id string) error

	// Events and logs.

	GetIssues(ctx context.Context, issueIDs []string) ([]Issue, error)
	// SetIssuesStatus changes all or none (ErrNotFound for a missing ID).
	SetIssuesStatus(ctx context.Context, issueIDs []string, status string, actor Actor) ([]Issue, error)
	// IgnoreIssues sets them ignored until bound (the zero value: for good), all or none.
	IgnoreIssues(ctx context.Context, issueIDs []string, bound IgnoreBound, actor Actor) ([]Issue, error)
	IssueStatusHistory(ctx context.Context, issueID string) ([]StatusChange, error)

	// Notification outbox.

	EnqueueOutbox(ctx context.Context, channels []string, projectIDs []int, kind string, payload []byte, now time.Time) error
	// DueOutbox returns a channel's due messages, oldest first.
	DueOutbox(ctx context.Context, channel string, now time.Time, limit int) ([]OutboxMessage, error)
	MarkOutboxSent(ctx context.Context, id int64, now time.Time) error
	MarkOutboxRetry(ctx context.Context, id int64, lastError string, next, now time.Time) error
	// RetryOutboxNow makes a channel's waiting messages due now.
	RetryOutboxNow(ctx context.Context, channel string, now time.Time) error
	MarkOutboxFailed(ctx context.Context, id int64, lastError string, now time.Time) error
	CleanOutbox(ctx context.Context, sentBefore, failedBefore time.Time) (int64, error)
	// HasPendingOutbox says whether a channel has messages to send, due or not.
	HasPendingOutbox(ctx context.Context, channel string) (bool, error)
	// PendingChannels lists channels with messages to send, for a restart.
	PendingChannels(ctx context.Context) ([]string, error)
	ListOutbox(ctx context.Context, filter OutboxFilter) ([]OutboxRecord, error)
	CountOutbox(ctx context.Context, filter OutboxFilter) (int, error)
	DeliveryHealthByChannel(ctx context.Context) (map[string]DeliveryHealth, error)

	// Projects.

	ListProjects(ctx context.Context) ([]Project, error)
	GetProject(ctx context.Context, id int) (Project, error)
	// CreateProject adds a project; a limited owner (a user id, or "") gets it in the same batch.
	CreateProject(ctx context.Context, project Project, now time.Time, ownerID string) (int, error)
	// UpdateProject saves a project's settings; its key changes only by RotateProjectKey.
	UpdateProject(ctx context.Context, project Project) error
	// RotateProjectKey gives a project a new DSN key at now; ErrProjectConflict when another project has it.
	RotateProjectKey(ctx context.Context, id int, key string, now time.Time) error
	// SetProjectPaused pauses or resumes storing what the project's apps send.
	SetProjectPaused(ctx context.Context, id int, paused bool) error
	// DeleteProject deletes a project at once, also from connections and
	// users' access; PurgeProjects then deletes its data.
	DeleteProject(ctx context.Context, id int) error
	PurgeProjects(ctx context.Context) error

	// Reading issues, events and logs.

	// CountIssues counts up to LogCountLimit; more: there are more.
	CountIssues(ctx context.Context, filter IssueFilter) (count int, more bool, err error)
	ListIssues(ctx context.Context, filter IssueFilter) ([]Issue, error)
	GetIssue(ctx context.Context, issueID string) (Issue, error)
	// DeleteIssues deletes the issues that exist, with their events and history; it says how many.
	DeleteIssues(ctx context.Context, issueIDs []string) (int, error)
	UnresolvedCounts(ctx context.Context) (map[int]int, error)
	// IssueLevels returns the levels at least one issue of the projects has
	// (every project when none is given), in no order.
	IssueLevels(ctx context.Context, projectIDs []int) ([]string, error)
	ListEvents(ctx context.Context, filter EventFilter) ([]EventSummary, error)
	// GetEvent finds an event by its SDK event_id within a project.
	GetEvent(ctx context.Context, projectID int, eventID string) (Event, error)
	LatestEvent(ctx context.Context, issueID string) (Event, error)
	// CountLogs stops at LogCountLimit or, when it reads the logs rather
	// than counting keys, at config.LogSearchTime.
	CountLogs(ctx context.Context, filter LogFilter) (LogCount, error)
	// LogLevels returns the levels at least one log of the projects has
	// (every project when none is given).
	LogLevels(ctx context.Context, projectIDs []int) ([]string, error)
	// ListLogs stops at config.LogSearchTime too.
	ListLogs(ctx context.Context, filter LogFilter) (LogPage, error)
	// IssuesForLogs finds the issue of each log's request, by span.
	IssuesForLogs(ctx context.Context, logs []LogRecord) (map[int64]LogIssue, error)
	// DeleteLogs deletes the logs that exist and access sees; it says how many.
	DeleteLogs(ctx context.Context, access Access, logIDs []int64) (int, error)

	// The database.

	// DatabaseSize is what the database takes on disk.
	DatabaseSize(ctx context.Context) (int64, error)
	ProjectUsage(ctx context.Context) (ProjectUsage, error)
	// CleanupHistory returns the latest runs of the cleanup by size, newest first.
	CleanupHistory(ctx context.Context, limit int) ([]CleanupRun, error)
	Ping(ctx context.Context) error
	Close() error

	// Sessions of the web UI (by the hash of the cookie).

	CreateSession(ctx context.Context, session NewSession) error
	SessionUser(ctx context.Context, tokenHash string, now time.Time) (User, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	// TouchSession records a session's activity at now (the caller writes it at most every SessionActiveEvery).
	TouchSession(ctx context.Context, tokenHash string, now time.Time) error
	// Sessions are a user's sessions that have not expired, newest first.
	Sessions(ctx context.Context, userID string, now time.Time) ([]Session, error)
	// EndUserSession ends a user's session by its ID; ErrNotFound for none of theirs.
	EndUserSession(ctx context.Context, userID, sessionID string) error
	// RecordSignIn adds an attempt to the user's sign-in log, keeping the newest SignInsKept.
	RecordSignIn(ctx context.Context, signIn SignIn) error
	// SignIns are a user's sign-in attempts, newest first.
	SignIns(ctx context.Context, userID string) ([]SignIn, error)
	// GoogleUser finds the user a Google account (its subject) is linked to.
	GoogleUser(ctx context.Context, sub string) (User, error)
	// LinkGoogle links a Google account to the user, checked in its batch: the
	// user's email is still email (else ErrNotFound) and no other account is
	// linked (ErrGoogleLinked) or Google unlinked (ErrGoogleOff);
	// ErrGoogleTaken when another user has it.
	LinkGoogle(ctx context.Context, userID, email, sub string, now time.Time) error
	// UnlinkGoogle drops the user's Google account, keeps it from linking again
	// by email until a password sign-in, and ends the sessions opened through
	// Google but the one of ID keepSession.
	UnlinkGoogle(ctx context.Context, userID, keepSession string) error

	// Users.

	// CreateUser returns ErrEmailTaken for an email taken in any case.
	CreateUser(ctx context.Context, user User, passwordHash string) (string, error)
	// CreateFirstAdmin creates the first admin; ErrAdminExists once there is one.
	CreateFirstAdmin(ctx context.Context, user User, passwordHash string) (string, error)
	GetUser(ctx context.Context, id string) (User, error)
	FindUser(ctx context.Context, email string) (User, error)
	FindLogin(ctx context.Context, email string) (User, string, error)
	UserPassword(ctx context.Context, id string) (string, error)
	// Admins are the users of the role admin; none before the first is set up.
	Admins(ctx context.Context) ([]User, error)
	ListUsers(ctx context.Context) ([]User, error)
	// UpdateUser and DeleteUser return ErrLastAdmin rather than leave no admin.
	UpdateUser(ctx context.Context, id string, change UserChange) error
	UpdateUserName(ctx context.Context, id string, name string) error
	UpdateUserTheme(ctx context.Context, id string, theme string) error
	// SetPassword ends the user's other sessions.
	SetPassword(ctx context.Context, id string, change PasswordChange) error
	DeleteUser(ctx context.Context, id string) error
	RecordLogin(ctx context.Context, id, method string, now time.Time) error
}
