package sentry

import (
	"encoding/json"
	"errors"
	"time"
)

// ParentSpanAttribute is the attribute where SDK 11 puts the span a log was
// written in.
const ParentSpanAttribute = "sentry.trace.parent_span_id"

// Log is one structured log record from a "log" item.
// Format: https://develop.sentry.dev/sdk/telemetry/logs/
type Log struct {
	Timestamp Timestamp `json:"timestamp"`
	// Level is as sent, maybe empty: config.LogLevelOf decides with SeverityNumber.
	Level          string               `json:"level"`
	Body           string               `json:"body"`
	TraceID        string               `json:"trace_id"`
	SpanID         string               `json:"span_id"`
	SeverityNumber int                  `json:"severity_number"`
	Attributes     map[string]Attribute `json:"attributes"`
}

// Attribute is a typed log attribute: Type is "string", "integer", "double" or "boolean".
type Attribute struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// String returns the attribute value as text.
func (attribute Attribute) String() string {
	return rawToString(attribute.Value)
}

// ParseLogs decodes a log item payload: {"items": [...]}.
func ParseLogs(payload []byte, received time.Time) ([]Log, error) {
	var batch struct {
		Items []Log `json:"items"`
	}

	if !Decode(payload, &batch) {
		return nil, errors.New("logs: not a JSON object")
	}

	for index := range batch.Items {
		record := &batch.Items[index]

		if record.Timestamp.IsZero() {
			record.Timestamp = Timestamp{received}
		}

		// SDK 11 puts the span the log was written in into an attribute.
		if record.SpanID == "" {
			record.SpanID = record.Attributes[ParentSpanAttribute].String()
		}
	}

	return batch.Items, nil
}
