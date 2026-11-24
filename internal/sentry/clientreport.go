package sentry

// Discarded is what an SDK reports it dropped: its reason (ratelimit_backoff,
// queue_overflow, sample_rate, …), the category (error, log_item, …) and how many.
type Discarded struct {
	Reason   string `json:"reason"`
	Category string `json:"category"`
	Quantity int64  `json:"quantity"`
}

// ParseClientReport reads the discarded_events of a client_report item;
// anything malformed gives what could be read.
func ParseClientReport(payload []byte) []Discarded {
	var report struct {
		DiscardedEvents []Discarded `json:"discarded_events"`
	}

	Decode(payload, &report)

	return report.DiscardedEvents
}
