package ingest

import (
	"bytes"
)

// prepareEvent removes credentials (scrub.go) and what does not help a
// fix (compact.go) in one pass over the JSON tokens, with no tree built.
// Invalid input is left for the event parser to reject, as in scrubJSON.
func prepareEvent(payload []byte) []byte {
	var output bytes.Buffer

	if err := compactEvent(newTokenDecoder(payload), newTokenEncoder(&output)); err != nil {
		return payload
	}

	return bytes.TrimRight(output.Bytes(), "\n")
}
