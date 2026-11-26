package database

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The records and collections of the store: the one place where keys are declared.

type collections struct {
	inbox     *kv.Collection[inboxRecord]
	issues    *kv.Collection[issueRecord]
	status    *kv.Collection[statusRecord]
	events    *kv.Collection[eventRecord]
	logs      *kv.Collection[logRecord]
	timeMarks *kv.Collection[timeMark]
	thin      *kv.Collection[thinProgress]
	cleanups  *kv.Collection[CleanupRun]

	users        *kv.Collection[userRecord]
	sessions     *kv.Collection[sessionRecord]
	signIns      *kv.Collection[signInRecord]
	oauthClients *kv.Collection[store.OAuthClient]
	oauthCodes   *kv.Collection[codeRecord]
	grants       *kv.Collection[grantRecord]
	spentTokens  *kv.Collection[spentToken]

	projects     *kv.Collection[store.Project]
	purges       *kv.Collection[projectPurge]
	contacts     *kv.Collection[projectContact]
	dropNotices  *kv.Collection[dropNotice]
	destinations *kv.Collection[destinationRecord]
	connections  *kv.Collection[connectionRecord]
	outbox       *kv.Collection[store.OutboxRecord]
}

// Kinds of the byte counters.
const (
	kindLog   byte = 'l'
	kindEvent byte = 'e'
	kindIssue byte = 'i'
)

func sizeCounter(kind, rank byte, day int32) kv.Key {
	return kv.K(kv.U8(kind), kv.U8(rank), kv.U32(uint32(day)))
}

// projectCounter is the group counter of a project: the bytes of its issues, events and logs.
func projectCounter(projectID int) kv.Key { return projectKey(projectID) }

func defineCollections(schema *kv.Schema) collections {
	return collections{
		// inbox/<seq>: an envelope waiting to be stored.
		inbox: kv.Define(schema, kv.Definition[inboxRecord]{
			Name:       "inbox",
			Key:        func(entry inboxRecord) kv.Key { return sequenceKey(entry.Sequence) },
			Encode:     encodeInbox,
			Decode:     decodeInbox,
			DecodeHead: decodeInboxHead,
		}),
		// issue/<id>, read through its indexes: they list issues by status,
		// project and level, each in the order of one sort, and the tallies
		// count them so (issueTally).
		issues: kv.Define(schema, kv.Definition[issueRecord]{
			Name: "issue",
			Key:  func(issue issueRecord) kv.Key { return kv.K(kv.Text(issue.ID)) },
			Indexes: []kv.Index[issueRecord]{
				// issue.fingerprint/<project>/<fingerprint>: one issue per fingerprint of a project.
				{Name: "fingerprint", Key: func(issue issueRecord) kv.Key {
					return kv.K(kv.U32(uint32(issue.ProjectID)), kv.Text(issue.Fingerprint))
				}, Unique: true},
				{Name: "seen", Key: func(issue issueRecord) kv.Key { return issueSortKey(issue, issue.LastSeen.UnixNano()) }},
				{Name: "first", Key: func(issue issueRecord) kv.Key { return issueSortKey(issue, issue.FirstSeen.UnixNano()) }},
				{Name: "count", Key: func(issue issueRecord) kv.Key { return issueSortKey(issue, int64(issue.EventCount)) }},
				// issue.expiry/<project>/<until>/<id>: the ignores that end at a time.
				{Name: "expiry", Key: func(issue issueRecord) kv.Key {
					return kv.KIf(issue.Status == store.StatusIgnored && issue.IgnoredUntil != nil,
						kv.U32(uint32(issue.ProjectID)), kv.U64(uint64(timeValue(issue.IgnoredUntil))), kv.Text(issue.ID))
				}},
				// issue.trim/<project>/<stored>/<id>: the issues storing more events than the cleanup's step 7 keeps.
				{Name: "trim", Key: func(issue issueRecord) kv.Key {
					return kv.KIf(issue.Stored > config.OldEventsPerIssue, kv.U32(uint32(issue.ProjectID)), kv.U64(uint64(issue.Stored)), kv.Text(issue.ID))
				}},
			},
			Backfill: &kv.Backfill{Name: "trim", Indexes: []string{"trim"}},
			Counter:  func(issueRecord) kv.Key { return sizeCounter(kindIssue, 0, 0) },
			Group:    func(issue issueRecord) kv.Key { return projectCounter(issue.ProjectID) },
			Tally: func(issue issueRecord) []kv.Key {
				return []kv.Key{issueTally(issue.ProjectID, issue.Status, issue.Level)}
			},
		}),
		// status/<issue>/<seq>: the status changes of an issue, oldest first.
		status: kv.Define(schema, kv.Definition[statusRecord]{
			Name: "status",
			Key: func(change statusRecord) kv.Key {
				return kv.K(kv.Text(change.IssueID), kv.U64(change.Sequence))
			},
		}),
		// event/<issue>/<seq>: the events of an issue, oldest first.
		events: kv.Define(schema, kv.Definition[eventRecord]{
			Name: "event",
			Key: func(event eventRecord) kv.Key {
				return kv.K(kv.Text(event.IssueID), kv.U64(event.Sequence))
			},
			Indexes: []kv.Index[eventRecord]{
				// The SDK's event_id per project, so a retry is recognized.
				{Name: "id", Unique: true, Key: func(event eventRecord) kv.Key { return eventIDKey(event.ProjectID, event.EventID) }},
				// By level, then arrival.
				{Name: "age", Key: func(event eventRecord) kv.Key {
					return kv.K(kv.U8(levelRank(event.Level)), kv.U64(event.Sequence))
				}},
				{Name: "trace", Key: func(event eventRecord) kv.Key {
					return kv.KIf(event.TraceID != "", kv.Text(event.TraceID), kv.U64(event.Sequence))
				}},
				{Name: "span", Key: func(event eventRecord) kv.Key {
					return kv.KIf(event.SpanID != "", kv.Text(event.SpanID), kv.U64(event.Sequence))
				}},
				{Name: "parent", Key: func(event eventRecord) kv.Key {
					return kv.KIf(event.ParentSpanID != "", kv.Text(event.ParentSpanID), kv.U64(event.Sequence))
				}},
				// By project, level, then arrival, like the logs' project index.
				{Name: "project", Key: func(event eventRecord) kv.Key {
					return kv.K(kv.U32(uint32(event.ProjectID)), kv.U8(levelRank(event.Level)), kv.U64(event.Sequence))
				}},
			},
			Encode:     encodeEvent,
			Decode:     decodeEvent,
			DecodeHead: decodeEventHead,
			Counter: func(event eventRecord) kv.Key {
				return sizeCounter(kindEvent, levelRank(event.Level), dayOf(event.ReceivedAt))
			},
			Group: func(event eventRecord) kv.Key { return projectCounter(event.ProjectID) },
		}),
		// log/<level>/<seq>: logs by level, then arrival.
		logs: kv.Define(schema, kv.Definition[logRecord]{
			Name: "log",
			Key:  func(record logRecord) kv.Key { return kv.K(kv.U8(levelRank(record.Level)), kv.U64(record.Sequence)) },
			Indexes: []kv.Index[logRecord]{
				{Name: "project", Key: func(record logRecord) kv.Key {
					return kv.K(kv.U32(uint32(record.ProjectID)), kv.U8(levelRank(record.Level)), kv.U64(record.Sequence))
				}},
				{Name: "trace", Key: func(record logRecord) kv.Key {
					return kv.KIf(record.TraceID != "", kv.Text(record.TraceID), kv.U64(record.Sequence))
				}},
				{Name: "span", Key: func(record logRecord) kv.Key {
					return kv.KIf(record.SpanID != "", kv.Text(record.SpanID), kv.U64(record.Sequence))
				}},
				// The copies of a repeated message, oldest first.
				{Name: "message", Key: func(record logRecord) kv.Key {
					return kv.KIf(record.Fingerprint != "", append(messageParts(record.ProjectID, record.Fingerprint), kv.U64(record.Sequence))...)
				}},
			},
			Encode: encodeLog,
			Decode: decodeLog,
			Counter: func(record logRecord) kv.Key {
				return sizeCounter(kindLog, levelRank(record.Level), record.ReceivedDay)
			},
			Group: func(record logRecord) kv.Key { return projectCounter(record.ProjectID) },
		}),
		// mark/<interval>: the first sequence number of an interval.
		timeMarks: kv.Define(schema, kv.Definition[timeMark]{
			Name: "mark",
			Key:  func(mark timeMark) kv.Key { return kv.K(kv.U64(mark.Interval)) },
		}),
		// cleanup/<seq>: the latest runs of the cleanup (cleanup.go).
		cleanups: kv.Define(schema, kv.Definition[CleanupRun]{
			Name: "cleanup",
			Key:  func(run CleanupRun) kv.Key { return kv.K(kv.U64(run.Sequence)) },
		}),
		// thin/<level>: how far old events are thinned.
		thin: kv.Define(schema, kv.Definition[thinProgress]{
			Name: "thin",
			Key:  func(progress thinProgress) kv.Key { return kv.K(kv.U8(progress.Rank)) },
		}),
		// user/<id>: a user of the panel with the password hash.
		users: kv.Define(schema, kv.Definition[userRecord]{
			Name: "user",
			Key:  func(user userRecord) kv.Key { return textKey(user.ID) },
			Indexes: []kv.Index[userRecord]{
				// Emails are unique in any case ("name": the stored keys keep the index's former name).
				{Name: "name", Unique: true, Key: func(user userRecord) kv.Key { return emailKey(user.Email) }},
				// A Google account is linked to one user; users stored before have none, so nothing to backfill.
				{Name: "google", Unique: true, Key: func(user userRecord) kv.Key {
					return kv.KIf(user.GoogleSub != "", kv.Text(user.GoogleSub))
				}},
			},
		}),
		// session/<hash of the cookie>: a login to the UI.
		sessions: kv.Define(schema, kv.Definition[sessionRecord]{
			Name: "session",
			Key:  func(session sessionRecord) kv.Key { return textKey(session.TokenHash) },
			Indexes: []kv.Index[sessionRecord]{
				{Name: "user", Key: func(session sessionRecord) kv.Key {
					return kv.K(kv.Text(session.UserID), kv.Text(session.TokenHash))
				}},
			},
		}),
		// signin/<user>/<sequence>: an attempt to sign in as a user, the newest store.SignInsKept per user.
		signIns: kv.Define(schema, kv.Definition[signInRecord]{
			Name: "signin",
			Key:  func(signIn signInRecord) kv.Key { return kv.K(kv.Text(signIn.UserID), kv.U64(signIn.Sequence)) },
		}),
		// oauth_client/<client id>: an agent registered for MCP.
		oauthClients: kv.Define(schema, kv.Definition[store.OAuthClient]{
			Name: "oauth_client",
			Key:  func(client store.OAuthClient) kv.Key { return textKey(client.ID) },
		}),
		// oauth_code/<hash>: an authorization code, used once.
		oauthCodes: kv.Define(schema, kv.Definition[codeRecord]{
			Name: "oauth_code",
			Key:  func(code codeRecord) kv.Key { return textKey(code.Hash) },
			Indexes: []kv.Index[codeRecord]{
				{Name: "client", Key: func(code codeRecord) kv.Key { return kv.K(kv.Text(code.ClientID), kv.Text(code.Hash)) }},
				{Name: "user", Key: func(code codeRecord) kv.Key { return kv.K(kv.Text(code.UserID), kv.Text(code.Hash)) }},
			},
		}),
		// grant/<id>: a connected agent or a personal token, with its token hashes.
		grants: kv.Define(schema, kv.Definition[grantRecord]{
			Name: "grant",
			Key:  func(grant grantRecord) kv.Key { return textKey(grant.ID) },
			Indexes: []kv.Index[grantRecord]{
				{Name: "access", Unique: true, Key: func(grant grantRecord) kv.Key { return kv.K(kv.Text(grant.AccessHash)) }},
				{Name: "refresh", Unique: true, Key: func(grant grantRecord) kv.Key {
					return kv.KIf(grant.RefreshHash != "", kv.Text(grant.ClientID), kv.Text(grant.RefreshHash))
				}},
				{Name: "user", Key: func(grant grantRecord) kv.Key { return kv.K(kv.Text(grant.UserID), kv.Text(grant.ID)) }},
				{Name: "client", Key: func(grant grantRecord) kv.Key {
					return kv.KIf(grant.ClientID != "", kv.Text(grant.ClientID), kv.Text(grant.ID))
				}},
				// One personal token per user.
				{Name: "token", Unique: true, Key: func(grant grantRecord) kv.Key {
					return kv.KIf(grant.Kind == store.GrantToken, kv.Text(grant.UserID))
				}},
			},
		}),
		// oauth_spent/<hash>: a refresh token already used; presented again, it revokes its grant.
		spentTokens: kv.Define(schema, kv.Definition[spentToken]{
			Name: "oauth_spent",
			Key:  func(spent spentToken) kv.Key { return textKey(spent.Hash) },
		}),
		// project/<id>: a project; its id is part of the DSN.
		projects: kv.Define(schema, kv.Definition[store.Project]{
			Name: "project",
			Key:  func(project store.Project) kv.Key { return projectKey(project.ID) },
			Indexes: []kv.Index[store.Project]{
				{Name: "name", Unique: true, Key: func(project store.Project) kv.Key { return kv.K(kv.Text(project.Name)) }},
				{Name: "key", Unique: true, Key: func(project store.Project) kv.Key { return kv.K(kv.Text(project.PublicKey)) }},
			},
		}),
		// project_purge/<id>: a deleted project whose data is still to delete.
		purges: kv.Define(schema, kv.Definition[projectPurge]{
			Name: "project_purge",
			Key:  func(purge projectPurge) kv.Key { return projectKey(purge.ProjectID) },
		}),
		// project_contact/<id>: when a project first sent a request with its key.
		contacts: kv.Define(schema, kv.Definition[projectContact]{
			Name: "project_contact",
			Key:  func(contact projectContact) kv.Key { return projectKey(contact.ProjectID) },
		}),
		// drop_notice/<id>: when a project's dropped data was last notified.
		dropNotices: kv.Define(schema, kv.Definition[dropNotice]{
			Name: "drop_notice",
			Key:  func(notice dropNotice) kv.Key { return projectKey(notice.ProjectID) },
		}),
		// destination/<id>: where notifications go, an email address or a chat.
		destinations: kv.Define(schema, kv.Definition[destinationRecord]{
			Name: "destination",
			Key:  func(destination destinationRecord) kv.Key { return textKey(destination.ID) },
			Indexes: []kv.Index[destinationRecord]{
				// The destinations of an address, a confirmed one first.
				{Name: "email", Key: func(destination destinationRecord) kv.Key {
					confirmed := byte(1)

					if destination.Verified {
						confirmed = 0
					}

					return kv.KIf(destination.Type == config.ChannelEmail, kv.Text(destination.Address), kv.U8(confirmed), kv.Text(destination.ID))
				}},
			},
		}),
		// connection/<id>: a notification rule.
		connections: kv.Define(schema, kv.Definition[connectionRecord]{
			Name: "connection",
			Key:  func(connection connectionRecord) kv.Key { return textKey(connection.ID) },
		}),
		// outbox/<seq>: a notification message for one connection.
		outbox: kv.Define(schema, kv.Definition[store.OutboxRecord]{
			Name: "outbox",
			Key:  func(message store.OutboxRecord) kv.Key { return sequenceKey(uint64(message.ID)) },
			Indexes: []kv.Index[store.OutboxRecord]{
				{Name: "channel", Key: func(message store.OutboxRecord) kv.Key {
					return kv.K(kv.Text(message.Channel), kv.U64(uint64(message.ID)))
				}},
				// The messages still to send, per channel, oldest first.
				{Name: "pending", Key: func(message store.OutboxRecord) kv.Key {
					return kv.KIf(message.Status == store.OutboxPending, kv.Text(message.Channel), kv.U64(uint64(message.ID)))
				}},
				{Name: "status", Key: func(message store.OutboxRecord) kv.Key {
					return kv.K(kv.Text(message.Status), kv.U64(uint64(message.ID)))
				}},
			},
			Encode:     encodeOutbox,
			Decode:     decodeOutbox,
			DecodeHead: decodeOutboxHead,
		}),
	}
}

// textKey is the key of a record kept under one text field, such as its id or hash.
func textKey(text string) kv.Key { return kv.K(kv.Text(text)) }

// sequenceKey is the key of a record kept under its sequence number.
func sequenceKey(sequence uint64) kv.Key { return kv.K(kv.U64(sequence)) }

func projectKey(id int) kv.Key { return kv.K(kv.U32(uint32(id))) }

// emailKey is the key of the "name" index of users: emails are unique in any case.
func emailKey(email string) kv.Key { return textKey(strings.ToLower(email)) }

// eventIDKey and messageParts lead the "id" and "message" index keys.
func eventIDKey(projectID int, eventID string) kv.Key {
	return kv.K(kv.U32(uint32(projectID)), kv.Text(eventID))
}

func messageParts(projectID int, fingerprint string) []kv.Part {
	return []kv.Part{kv.U32(uint32(projectID)), kv.Text(fingerprint)}
}

// InboxEntry is an envelope waiting in the inbox, without its body (InboxBody).
type InboxEntry struct {
	Sequence  uint64
	ProjectID int
	Received  time.Time
	Size      int // bytes the entry takes in the inbox (InboxBytes)
}

type inboxRecord struct {
	InboxEntry
	Body []byte
}

// An inbox entry is its sequence, project and receive time, then the body.
const inboxHeader = 8 + 4 + 8

func encodeInbox(entry inboxRecord) ([]byte, error) {
	value := make([]byte, inboxHeader, inboxHeader+len(entry.Body))
	binary.BigEndian.PutUint64(value, entry.Sequence)
	binary.BigEndian.PutUint32(value[8:], uint32(entry.ProjectID))
	binary.BigEndian.PutUint64(value[12:], uint64(entry.Received.UnixNano()))

	return append(value, entry.Body...), nil
}

var errShortRecord = errors.New("database: record too short")

func decodeInbox(value []byte) (inboxRecord, error) {
	entry, err := decodeInboxHead(value)

	if err == nil {
		entry.Body = bytes.Clone(value[inboxHeader:])
	}

	return entry, err
}

// inboxBody slices the body out of a stored entry without a copy.
func inboxBody(value []byte) ([]byte, error) {
	if len(value) < inboxHeader {
		return nil, errShortRecord
	}

	return value[inboxHeader:], nil
}

func decodeInboxHead(value []byte) (inboxRecord, error) {
	if len(value) < inboxHeader {
		return inboxRecord{}, errShortRecord
	}

	return inboxRecord{InboxEntry: InboxEntry{
		Sequence:  binary.BigEndian.Uint64(value),
		ProjectID: int(binary.BigEndian.Uint32(value[8:])),
		Received:  time.Unix(0, int64(binary.BigEndian.Uint64(value[12:]))).UTC(),
		Size:      len(value),
	}}, nil
}

// statusRecord is a status change of an issue.
type statusRecord struct {
	IssueID  string `json:"issue_id"`
	Sequence uint64 `json:"seq"`
	store.StatusChange
}

// eventRecord is the event metadata, then its payload as sent.
type eventRecord struct {
	eventMeta
	Payload []byte
}

type eventMeta struct {
	Sequence     uint64    `json:"seq"`
	ProjectID    int       `json:"project_id"`
	IssueID      string    `json:"issue_id"`
	EventID      string    `json:"event_id"`
	Level        string    `json:"level"`
	Environment  string    `json:"environment,omitempty"`
	Release      string    `json:"release,omitempty"`
	TraceID      string    `json:"trace_id,omitempty"`
	SpanID       string    `json:"span_id,omitempty"`
	ParentSpanID string    `json:"parent_span_id,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	ReceivedAt   time.Time `json:"received_at"`
	Thinned      bool      `json:"thinned,omitempty"`
}

func encodeEvent(event eventRecord) ([]byte, error) { return withTail(event.eventMeta, event.Payload) }

func decodeEvent(value []byte) (eventRecord, error) {
	var event eventRecord

	tail, err := splitTail(value, &event.eventMeta)
	event.Payload = tail

	return event, err
}

// decodeEventHead decodes the metadata of an event without its payload.
func decodeEventHead(value []byte) (eventRecord, error) {
	var event eventRecord

	_, err := decodeHead(value, &event.eventMeta)

	return event, err
}

// logRecord is the log fields, then its attributes as JSON.
type logRecord struct {
	logMeta
	Attributes []byte
}

type logMeta struct {
	Sequence       uint64    `json:"seq"`
	ReceivedDay    int32     `json:"day"` // for the byte counters
	ProjectID      int       `json:"project_id"`
	Timestamp      time.Time `json:"timestamp"`
	Level          string    `json:"level"`
	SeverityNumber int       `json:"severity_number,omitempty"`
	Body           string    `json:"body"`
	TraceID        string    `json:"trace_id,omitempty"`
	SpanID         string    `json:"span_id,omitempty"`
	Fingerprint    string    `json:"fingerprint,omitempty"`
}

func encodeLog(record logRecord) ([]byte, error) { return withTail(record.logMeta, record.Attributes) }

func decodeLog(value []byte) (logRecord, error) {
	var record logRecord

	tail, err := splitTail(value, &record.logMeta)
	record.Attributes = tail

	return record, err
}

// thinProgress is the last event of a level the cleanup has thinned.
type thinProgress struct {
	Rank     byte   `json:"rank"`
	Sequence uint64 `json:"seq"`
}

// timeMark ties an interval to the first sequence number written in it.
type timeMark struct {
	Interval uint64 `json:"interval"`
	First    uint64 `json:"first"`
}

// withTail encodes the JSON head's length, the head, then raw tail bytes.
func withTail(head any, tail []byte) ([]byte, error) {
	encoded, err := json.Marshal(head)

	if err != nil {
		return nil, err
	}

	value := binary.BigEndian.AppendUint32(make([]byte, 0, 4+len(encoded)+len(tail)), uint32(len(encoded)))

	return append(append(value, encoded...), tail...), nil
}

// decodeHead decodes the head of a value written by withTail into head and returns where the tail starts.
func decodeHead(value []byte, head any) (int, error) {
	if len(value) < 4 || int(binary.BigEndian.Uint32(value)) > len(value)-4 {
		return 0, errShortRecord
	}

	end := 4 + int(binary.BigEndian.Uint32(value))

	return end, json.Unmarshal(value[4:end], head)
}

// splitTail decodes the head into head and returns a copy of the tail.
func splitTail(value []byte, head any) ([]byte, error) {
	end, err := decodeHead(value, head)

	if err != nil {
		return nil, err
	}

	return bytes.Clone(value[end:]), nil
}

// issueSortKey is an issue's key in a sort index: its stored status,
// project and level, the value it sorts by, then its ID for ties.
func issueSortKey(issue issueRecord, value int64) kv.Key {
	return kv.K(kv.U8(statusRank(issue.Status)), kv.U32(uint32(issue.ProjectID)), kv.U8(levelRank(issue.Level)),
		kv.U64(uint64(max(value, 0))), kv.Text(issue.ID))
}

// issueTally counts the issues of a project with a status and level.
func issueTally(projectID int, status, level string) kv.Key {
	return kv.K(kv.Text("issue"), kv.U32(uint32(projectID)), kv.U8(statusRank(status)), kv.U8(levelRank(level)))
}

// statusRank is a status's place in store.IssueStatuses.
func statusRank(status string) byte {
	return byte(max(slices.Index(store.IssueStatuses, status), 0))
}

func timeValue(at *time.Time) int64 {
	if at == nil {
		return 0
	}

	return at.UnixNano()
}
