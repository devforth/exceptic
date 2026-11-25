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

// A log's ids (32 and 16 hex characters as SDKs send them), its attribute
// names and types are cut to these.
const (
	maxLogIDBytes            = 64
	maxLogAttributeNameBytes = 256
	maxLogAttributeTypeBytes = 32
)

// limitLog cuts a log's body, ids and attributes to their limits.
func limitLog(record *sentry.Log) {
	record.Body = textutil.ShortenBytes(record.Body, config.MaxLogBodyBytes)
	record.TraceID = textutil.ShortenBytes(record.TraceID, maxLogIDBytes)
	record.SpanID = textutil.ShortenBytes(record.SpanID, maxLogIDBytes)

	// Of names cut alike, the first by name stays.
	for _, name := range slices.SortedFunc(maps.Keys(record.Attributes), compareAttributes) {
		attribute := record.Attributes[name]
		attribute.Type = textutil.ShortenBytes(attribute.Type, maxLogAttributeTypeBytes)

		if len(name) > maxLogAttributeNameBytes {
			delete(record.Attributes, name)

			if name = textutil.ShortenBytes(name, maxLogAttributeNameBytes); hasAttribute(record, name) {
				continue
			}
		}

		record.Attributes[name] = attribute
	}

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

func hasAttribute(record *sentry.Log, name string) bool {
	_, ok := record.Attributes[name]

	return ok
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
// least useful first. cutOther keeps only store.ErrorFields and contexts;
// cutSize, the last resort, leaves out of those what does not fit (fitted).
const (
	cutStrings = iota
	cutBreadcrumbs
	cutExtra
	cutRequestData
	cutContexts
	cutFrameVars
	cutFrameContext
	cutOther
	cutSize
	cutCount
)

// cutNames are what sentry.TruncatedField lists (docs/configuration.md).
var cutNames = [cutCount]string{"strings", "breadcrumbs", "extra", "request.data", "contexts", "frame_vars", "frame_context", "other", "size"}

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

		cut := bytes.TrimRight(output.Bytes(), "\n")
		last := level == cutCount-1

		if budget := config.MaxEventBytes - truncatedRoom; last && len(cut) > budget {
			if fit, ok := fitted(cut, budget, maxFitDepth); ok {
				cut = fit
				cutter.done[cutSize] = true
			}
		}

		var names []string

		for step, done := range cutter.done {
			if done {
				names = append(names, cutNames[step])
			}
		}

		if len(names) == 0 {
			if len(event) <= config.MaxEventBytes || last {
				return unchanged()
			}

			continue
		}

		if result := withTruncated(cut, slices.Concat(earlier, names)); len(result) <= config.MaxEventBytes || last {
			return result
		}
	}

	return unchanged()
}

// truncatedRoom is more than sentry.TruncatedField with every cut takes.
const truncatedRoom = 1 << 10

// maxFitDepth: how deep fitted cuts into a value that does not fit whole;
// deeper ones that do not fit are left out.
const maxFitDepth = 8

// fitted is a JSON value in at most budget bytes, false when nothing of it
// fits: an object keeps whole the members that fit, smallest first, then a
// cut of the others; a list its newest elements that fit whole, or a cut of
// the newest when none does.
func fitted(value jsontext.Value, budget, depth int) (jsontext.Value, bool) {
	if len(value) <= budget {
		return value, true
	}

	if depth == 0 || budget < len("{}") {
		return nil, false
	}

	switch value.Kind() {
	case '{':
		return fittedObject(value, budget, depth)
	case '[':
		return fittedList(value, budget, depth)
	}

	return nil, false
}

func fittedObject(value jsontext.Value, budget, depth int) (jsontext.Value, bool) {
	decoder := newTokenDecoder(value)

	if _, err := decoder.ReadToken(); err != nil {
		return nil, false
	}

	var members [][2]jsontext.Value

	for decoder.PeekKind() == '"' {
		name, err := decoder.ReadValue()

		if err != nil {
			return nil, false
		}

		name = name.Clone() // a read value is valid only until the next read
		member, err := decoder.ReadValue()

		if err != nil {
			return nil, false
		}

		members = append(members, [2]jsontext.Value{name, member.Clone()})
	}

	slices.SortStableFunc(members, func(first, second [2]jsontext.Value) int {
		return cmp.Compare(len(first[0])+len(first[1]), len(second[0])+len(second[1]))
	})

	output := jsontext.Value{'{'}

	for _, whole := range []bool{true, false} {
		for index, member := range members {
			// Room for a comma, the colon and the closing brace.
			room := budget - len(output) - len(member[0]) - 3

			if member[0] == nil || whole && len(member[1]) > room {
				continue
			}

			if part, ok := fitted(member[1], room, depth-1); ok {
				if len(output) > 1 {
					output = append(output, ',')
				}

				output = append(append(append(output, member[0]...), ':'), part...)
			}

			members[index][0] = nil
		}
	}

	return append(output, '}'), true
}

func fittedList(value jsontext.Value, budget, depth int) (jsontext.Value, bool) {
	items, err := elements(newTokenDecoder(value))

	if err != nil {
		return nil, false
	}

	var kept [][]byte

	used := len("[]")

	for index := len(items) - 1; index >= 0; index-- {
		room := budget - used - 1

		if len(kept) > 0 && len(items[index]) > room {
			break
		}

		part, ok := fitted(items[index], room, depth-1)

		if !ok {
			break
		}

		kept = append(kept, part)
		used += len(part) + 1
	}

	slices.Reverse(kept)

	return slices.Concat([]byte{'['}, bytes.Join(kept, []byte{','}), []byte{']'}), true
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

// topLevel cuts the event's fields, their names read in any case as the
// event parser reads them; contexts are a map, named exactly.
func (cutter *eventCutter) topLevel(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder) (bool, error) {
	field := sentry.FoldedName(name)

	if cutter.level >= cutOther && !store.ErrorFields[field] && field != "contexts" {
		return cutter.skip(cutOther, decoder)
	}

	switch {
	case field == "breadcrumbs" && cutter.level >= cutBreadcrumbs:
		return true, member(name, decoder, encoder, cutter.breadcrumbs)
	case field == "extra" && cutter.level >= cutExtra:
		return cutter.skip(cutExtra, decoder)
	case field == "request" && cutter.level >= cutRequestData && decoder.PeekKind() == '{':
		return true, member(name, decoder, encoder, func(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
			return cutter.object(decoder, encoder, func(name string, decoder *jsontext.Decoder, _ *jsontext.Encoder) (bool, error) {
				if sentry.FoldedName(name) == "data" {
					return cutter.skip(cutRequestData, decoder)
				}

				return false, nil
			})
		})
	case field == "contexts" && cutter.level >= cutContexts && decoder.PeekKind() == '{':
		return true, member(name, decoder, encoder, func(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
			return cutter.object(decoder, encoder, func(name string, decoder *jsontext.Decoder, _ *jsontext.Encoder) (bool, error) {
				if !keptContexts[name] {
					return cutter.skip(cutContexts, decoder)
				}

				return false, nil
			})
		})
	case (field == "exception" || field == "threads" || field == "stacktrace") && cutter.level >= cutFrameVars:
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
			if sentry.FoldedName(name) != "values" || decoder.PeekKind() != '[' {
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
			if cut, ok := frameCuts[sentry.FoldedName(name)]; ok && cutter.level >= cut {
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
