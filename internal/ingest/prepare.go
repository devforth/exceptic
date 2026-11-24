package ingest

import (
	"bytes"
)

// prepareEvent removes credentials (scrub.go) and what does not help a
// fix (compact.go) in one pass over the JSON tokens, with no tree built,
// then cuts the event to the size limits (truncate.go).
// Invalid input is left for the event parser to reject, as in scrubJSON.
func prepareEvent(payload []byte) []byte {
	var output bytes.Buffer

	framesDropped, err := compactEvent(newTokenDecoder(payload), newTokenEncoder(&output))

	if err != nil {
		return payload
	}

	var cuts []string

	if framesDropped {
		cuts = []string{framesCut}
	}

	return truncateEvent(bytes.TrimRight(output.Bytes(), "\n"), cuts)
}
