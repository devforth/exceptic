package store

// Dropped is what ingest did not store of a project, or its SDK reported
// not sending, in the last 24 hours, by reason and kind; counted in memory
// (a restart starts again from zero), never stored.
type Dropped struct {
	Reason string // one of DropReasons
	Kind   string // events, logs, or envelopes (refused before they were read)
	Count  int64
}

// The reasons of Dropped.
const (
	DropRateLimit    = "rate_limit"     // over RATE_LIMIT_EVENTS_PER_MINUTE or RATE_LIMIT_LOGS_PER_MINUTE
	DropBusy         = "busy"           // the inbox full, the reading budget used up, a compaction part running, or the store failing
	DropTooLarge     = "too_large"      // over 20 MB (413)
	DropInvalid      = "invalid"        // not an envelope, or a body that cannot be decoded (400, 415)
	DropIncomplete   = "incomplete"     // not within 10 s (408), or the client hung up mid-body
	DropSDKRateLimit = "sdk_rate_limit" // the SDK held back while a rate limit paused it
	DropSDKOverflow  = "sdk_overflow"   // the SDK's queue, buffer or offline cache was full
)

// DropReasons say each reason for people.
var DropReasons = map[string]string{
	DropRateLimit:    "rate limit",
	DropBusy:         "server busy",
	DropTooLarge:     "too large",
	DropInvalid:      "unreadable",
	DropIncomplete:   "not received whole",
	DropSDKRateLimit: "held back by the SDK during a rate limit",
	DropSDKOverflow:  "the SDK's queue was full",
}
