package ingest

import (
	"bytes"
	"cmp"
	"encoding/json"
	"encoding/json/jsontext"
	"maps"
	"slices"
	"strings"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Size limits (config.MaxLog*, config.MaxEvent*): a log or an event is cut,
// never dropped, after credentials are removed, so no secret is stored cut
// in half.

// limitLog cuts a log's body and attributes to their limits.
func limitLog(record *sentry.Log) {
	record.Body = textutil.ShortenBytes(record.Body, config.MaxLogBodyBytes)

	if len(record.Attributes) > config.MaxLogAttributes {
		for _, name := range slices.SortedFunc(maps.Keys(record.Attributes), compareAttributes)[config.MaxLogAttributes:] {
			delete(record.Attributes, name)
		}
	}

	for name, attribute := range record.Attributes {
		// Decoded text may be up to 3 times its JSON: invalid bytes become U+FFFD.
		if len(attribute.Value) > config.MaxLogAttributeBytes/3 {
			record.Attributes[name] = limitAttribute(attribute)
		}
	}
}

// compareAttributes orders what a capped log keeps: the parent span (logs tie
// to errors by span), the SDK's other "sentry." attributes, then the rest, each
// by name, so a repeated log keeps the same ones.
func compareAttributes(first, second string) int {
	if rank := cmp.Compare(attributeRank(first), attributeRank(second)); rank != 0 {
		return rank
	}

	return strings.Compare(first, second)
}

func attributeRank(name string) int {
	switch {
	case name == sentry.ParentSpanAttribute:
		return 0
	case strings.HasPrefix(name, "sentry."):
		return 1
	default:
		return 2
	}
}

// limitAttribute cuts a long value; one of another type than string becomes
// its JSON text, cut.
func limitAttribute(attribute sentry.Attribute) sentry.Attribute {
	var text string

	isText := json.Unmarshal(attribute.Value, &text) == nil

	if !isText {
		text = string(attribute.Value)
	}

	if len(text) <= config.MaxLogAttributeBytes {
		return attribute
	}

	value, err := json.Marshal(textutil.ShortenBytes(text, config.MaxLogAttributeBytes))

	if err != nil {
		return attribute
	}

	attribute.Value = value

	if !isText {
		attribute.Type = "string"
	}

	return attribute
}

// The cuts of an event, in the order they are tried: cutStrings always,
// then one more of the others while the event is over config.MaxEventBytes,
// least useful first. cutOther keeps only store.ErrorFields and contexts.
const (
	cutStrings = iota
	cutBreadcrumbs
	cutExtra
	cutRequestData
	cutContexts
	cutFrameVars
	cutFrameContext
	cutOther
	cutCount
)

// cutNames are what sentry.TruncatedField lists (docs/configuration.md).
var cutNames = [cutCount]string{"strings", "breadcrumbs", "extra", "request.data", "contexts", "frame_vars", "frame_context", "other"}

// keptContexts are the contexts cutContexts keeps: the runtime and where the
// event happened in a trace.
var keptContexts = map[string]bool{"runtime": true, "os": true, "browser": true, "trace": true}

// frameCuts are the fields of frames (any object inside a stack) and the cut that drops them.
var frameCuts = map[string]int{"vars": cutFrameVars, "pre_context": cutFrameContext, "post_context": cutFrameContext}

// truncateEvent cuts a prepared event to the size limits and lists the cuts
// in sentry.TruncatedField after earlier, those of the compaction; one that
// needs none, with no earlier, is returned as it is. Each try is one pass
// over the tokens of the event, at most cutCount of them.
func truncateEvent(event []byte, earlier []string) []byte {
	unchanged := func() []byte {
		if len(earlier) == 0 {
			return event
		}

		return withTruncated(event, earlier)
	}

	// A string over the limit needs at least a third of its bytes in JSON.
	if len(event) <= config.MaxEventStringBytes/3 || event[0] != '{' {
		return unchanged()
	}

	for level := cutStrings; level < cutCount; level++ {
		cutter := &eventCutter{level: level}

		var output bytes.Buffer

		if err := cutter.object(newTokenDecoder(event), newTokenEncoder(&output), cutter.topLevel); err != nil {
			return event
		}

		var names []string

		for step, done := range cutter.done {
			if done {
				names = append(names, cutNames[step])
			}
		}

		last := level == cutCount-1

		if len(names) == 0 {
			if len(event) <= config.MaxEventBytes || last {
				return unchanged()
			}

			continue
		}

		if cut := withTruncated(bytes.TrimRight(output.Bytes(), "\n"), slices.Concat(earlier, names)); len(cut) <= config.MaxEventBytes || last {
			return cut
		}
	}

	return unchanged()
}

// withTruncated adds sentry.TruncatedField with names to an event the encoder wrote.
func withTruncated(event []byte, names []string) []byte {
	list, _ := json.Marshal(names)
	field := append([]byte(`"`+sentry.TruncatedField+`":`), list...)

	if !bytes.Equal(event, []byte("{}")) {
		field = append([]byte{','}, field...)
	}

	return slices.Concat(event[:len(event)-1], field, []byte{'}'})
}

// eventCutter copies an event with the cuts up to level; done says which removed something.
type eventCutter struct {
	level int
	done  [cutCount]bool
}

// skip drops the next value for cut.
func (cutter *eventCutter) skip(cut int, decoder *jsontext.Decoder) (bool, error) {
	cutter.done[cut] = true

	return true, decoder.SkipValue()
}

func (cutter *eventCutter) topLevel(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder) (bool, error) {
	if cutter.level >= cutOther && !store.ErrorFields[name] && name != "contexts" {
		return cutter.skip(cutOther, decoder)
	}

	switch {
	case name == "breadcrumbs" && cutter.level >= cutBreadcrumbs:
		return true, member(name, decoder, encoder, cutter.breadcrumbs)
	case name == "extra" && cutter.level >= cutExtra:
		return cutter.skip(cutExtra, decoder)
	case name == "request" && cutter.level >= cutRequestData && decoder.PeekKind() == '{':
		return true, member(name, decoder, encoder, func(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
			return cutter.object(decoder, encoder, func(name string, decoder *jsontext.Decoder, _ *jsontext.Encoder) (bool, error) {
				if name == "data" {
					return cutter.skip(cutRequestData, decoder)
				}

				return false, nil
			})
		})
	case name == "contexts" && cutter.level >= cutContexts && decoder.PeekKind() == '{':
		return true, member(name, decoder, encoder, func(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
			return cutter.object(decoder, encoder, func(name string, decoder *jsontext.Decoder, _ *jsontext.Encoder) (bool, error) {
				if !keptContexts[name] {
					return cutter.skip(cutContexts, decoder)
				}

				return false, nil
			})
		})
	case (name == "exception" || name == "threads" || name == "stacktrace") && cutter.level >= cutFrameVars:
		return true, member(name, decoder, encoder, cutter.stack)
	}

	return false, nil
}

// breadcrumbs keeps the newest config.TruncatedBreadcrumbs, as a list or {"values": [...]}.
func (cutter *eventCutter) breadcrumbs(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	newest := func(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
		items, err := elements(decoder)

		if err != nil {
			return err
		}

		if len(items) > config.TruncatedBreadcrumbs {
			cutter.done[cutBreadcrumbs] = true
			items = items[len(items)-config.TruncatedBreadcrumbs:]
		}

		return writeList(encoder, items, func(_ int, decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
			return cutter.value(decoder, encoder)
		})
	}

	switch decoder.PeekKind() {
	case '[':
		return newest(decoder, encoder)
	case '{':
		return cutter.object(decoder, encoder, func(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder) (bool, error) {
			if name != "values" || decoder.PeekKind() != '[' {
				return false, nil
			}

			return true, member(name, decoder, encoder, newest)
		})
	}

	return cutter.value(decoder, encoder)
}

// stack copies exception, threads or stacktrace without the frame fields
// its level cuts, from every object inside.
func (cutter *eventCutter) stack(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	switch decoder.PeekKind() {
	case '{':
		return cutter.object(decoder, encoder, func(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder) (bool, error) {
			if cut, ok := frameCuts[name]; ok && cutter.level >= cut {
				return cutter.skip(cut, decoder)
			}

			return true, member(name, decoder, encoder, cutter.stack)
		})
	case '[':
		return scrubValues(decoder, encoder, cutter.stack)
	}

	return cutter.value(decoder, encoder)
}

// value copies the next value with its long strings cut.
func (cutter *eventCutter) value(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	switch decoder.PeekKind() {
	case '{':
		return cutter.object(decoder, encoder, nil)
	case '[':
		return scrubValues(decoder, encoder, cutter.value)
	case '"':
		return cutter.text(decoder, encoder)
	}

	raw, err := decoder.ReadValue()

	if err != nil {
		return err
	}

	return encoder.WriteValue(raw)
}

// object copies the next object; take, when set, may take a member over as in scrubObject.
func (cutter *eventCutter) object(decoder *jsontext.Decoder, encoder *jsontext.Encoder,
	take func(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder) (bool, error),
) error {
	if err := copyToken(decoder, encoder); err != nil {
		return err
	}

	for decoder.PeekKind() != '}' {
		key, err := decoder.ReadToken()

		if err != nil {
			return err
		}

		name := key.String()

		if take != nil {
			if done, err := take(name, decoder, encoder); err != nil || done {
				if err != nil {
					return err
				}

				continue
			}
		}

		if err := member(name, decoder, encoder, cutter.value); err != nil {
			return err
		}
	}

	return copyToken(decoder, encoder)
}

// text copies a string, cut when its text is longer than the limit.
func (cutter *eventCutter) text(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	raw, err := decoder.ReadValue()

	if err != nil {
		return err
	}

	// Decoded text may be up to 3 times its JSON: invalid bytes become U+FFFD.
	if len(raw) <= config.MaxEventStringBytes/3 {
		return encoder.WriteValue(raw)
	}

	var text string

	if err := json.Unmarshal(raw, &text); err != nil {
		return err
	}

	if len(text) <= config.MaxEventStringBytes {
		return encoder.WriteValue(raw)
	}

	cutter.done[cutStrings] = true

	return encoder.WriteToken(jsontext.String(textutil.ShortenBytes(text, config.MaxEventStringBytes)))
}
