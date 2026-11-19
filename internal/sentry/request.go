package sentry

import "encoding/json"

// Request is the "request" context of an event.
type Request struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// ParseRequest decodes the "request" context of an event; nil when it is not
// an object. Headers may come as an object or as [name, value] pairs.
func ParseRequest(raw json.RawMessage) *Request {
	var request Request

	if !Decode(raw, &request) {
		return nil
	}

	if request.Headers == nil {
		var pairs struct {
			Headers [][]string `json:"headers"`
		}

		Decode(raw, &pairs)

		for _, pair := range pairs.Headers {
			if len(pair) != 2 {
				continue
			}

			if request.Headers == nil {
				request.Headers = map[string]string{}
			}

			request.Headers[pair[0]] = pair[1]
		}
	}

	return &request
}
