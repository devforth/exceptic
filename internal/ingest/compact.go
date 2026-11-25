package ingest

import (
	"encoding/json"
	"encoding/json/jsontext"
	"slices"

	"github.com/devforth/exceptic/internal/sentry"
)

// maxBreadcrumbs: the last steps before the error tell the story; SDKs keep
// up to 100.
const maxBreadcrumbs = 50

// A stack keeps its oldestFrames and newestFrames (a deep recursion repeats
// itself in between); over detailedFrames frames with source, those in the
// middle lose their variables and the lines around the source line.
const (
	oldestFrames   = 50
	newestFrames   = 200
	detailedFrames = 50
)

// framesCut is what sentry.TruncatedField names when a stack lost frames.
const framesCut = "frames"

// compactEvent writes the next value of decoder, an event, with what does not
// help a fix dropped and credentials removed (scrub.go), in one token pass,
// and reports whether a stack lost frames. Anything but an object is only
// scrubbed.
func compactEvent(decoder *jsontext.Decoder, encoder *jsontext.Encoder) (framesDropped bool, err error) {
	if decoder.PeekKind() != '{' {
		return false, scrubValue(decoder, encoder)
	}

	stacks := &stackCompactor{}
	err = stacks.event(decoder, encoder)

	return stacks.framesDropped, err
}

// stackCompactor compacts an event's stacks and remembers whether one lost frames.
type stackCompactor struct {
	framesDropped bool
}

func (stacks *stackCompactor) event(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	return scrubObject(decoder, encoder, func(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder) (bool, error) {
		// Names in any case, as the event parser reads them.
		switch sentry.FoldedName(name) {
		case "modules", "debug_meta", sentry.TruncatedField: // the last is the server's own
			return true, decoder.SkipValue()
		case "sdk":
			return true, member(name, decoder, encoder, func(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
				if decoder.PeekKind() != '{' {
					return scrubValue(decoder, encoder)
				}

				return scrubObject(decoder, encoder, func(name string, decoder *jsontext.Decoder, _ *jsontext.Encoder) (bool, error) {
					if field := sentry.FoldedName(name); field == "integrations" || field == "packages" {
						return true, decoder.SkipValue()
					}

					return false, nil
				})
			})
		case "breadcrumbs":
			if decoder.PeekKind() == 'n' { // nothing to keep: the field goes
				return true, decoder.SkipValue()
			}

			return true, member(name, decoder, encoder, breadcrumbs)
		case "exception", "threads":
			return true, member(name, decoder, encoder, stacks.withStacktraces)
		case "stacktrace":
			return true, member(name, decoder, encoder, stacks.stacktrace)
		}

		return false, nil
	})
}

// member writes the key of an object member and its value through write.
func member(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder, write func(*jsontext.Decoder, *jsontext.Encoder) error) error {
	if err := encoder.WriteToken(jsontext.String(name)); err != nil {
		return err
	}

	return write(decoder, encoder)
}

// breadcrumbs keeps the newest maxBreadcrumbs, as a list or {"values": [...]}.
func breadcrumbs(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	switch decoder.PeekKind() {
	case '[':
		return newest(decoder, encoder)
	case '{':
		return scrubObject(decoder, encoder, func(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder) (bool, error) {
			if sentry.FoldedName(name) != "values" || decoder.PeekKind() != '[' {
				return false, nil
			}

			return true, member(name, decoder, encoder, newest)
		})
	}

	return scrubValue(decoder, encoder)
}

// newest writes the last maxBreadcrumbs elements of the next list.
func newest(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	items, err := elements(decoder)

	if err != nil {
		return err
	}

	return writeList(encoder, items[max(0, len(items)-maxBreadcrumbs):], func(_ int, decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
		return scrubElement(decoder, encoder)
	})
}

// withStacktraces writes exception or threads: a list of items, or
// {"values": [...]}, each item's stacktrace compacted.
func (stacks *stackCompactor) withStacktraces(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	items := func(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
		return scrubValues(decoder, encoder, func(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
			if decoder.PeekKind() != '{' {
				return scrubElement(decoder, encoder)
			}

			return scrubObject(decoder, encoder, func(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder) (bool, error) {
				if sentry.FoldedName(name) != "stacktrace" {
					return false, nil
				}

				return true, member(name, decoder, encoder, stacks.stacktrace)
			})
		})
	}

	switch decoder.PeekKind() {
	case '[':
		return items(decoder, encoder)
	case '{':
		return scrubObject(decoder, encoder, func(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder) (bool, error) {
			if sentry.FoldedName(name) != "values" || decoder.PeekKind() != '[' {
				return false, nil
			}

			return true, member(name, decoder, encoder, items)
		})
	}

	return scrubValue(decoder, encoder)
}

// stacktrace drops the source and variables of library frames when the
// stack has frames of the application, keeps the oldest and newest frames
// and the detail of those at the ends; the frames are read first to know.
func (stacks *stackCompactor) stacktrace(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	if decoder.PeekKind() != '{' {
		return scrubValue(decoder, encoder)
	}

	return scrubObject(decoder, encoder, func(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder) (bool, error) {
		if sentry.FoldedName(name) != "frames" || decoder.PeekKind() != '[' {
			return false, nil
		}

		frames, err := elements(decoder)

		if err != nil {
			return true, err
		}

		if len(frames) > oldestFrames+newestFrames {
			stacks.framesDropped = true
			frames = slices.Concat(frames[:oldestFrames], frames[len(frames)-newestFrames:])
		}

		app := make([]bool, len(frames))
		hasAppFrames := false

		for index, frame := range frames {
			app[index] = isAppFrame(frame)
			hasAppFrames = hasAppFrames || app[index]
		}

		// The frames that keep their source, counted from each end.
		withSource := make([]int, len(frames))
		sourceCount := 0

		for index := range frames {
			withSource[index] = sourceCount

			if !hasAppFrames || app[index] {
				sourceCount++
			}
		}

		return true, member(name, decoder, encoder, func(_ *jsontext.Decoder, encoder *jsontext.Encoder) error {
			return writeList(encoder, frames, func(index int, decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
				if decoder.PeekKind() != '{' {
					return scrubElement(decoder, encoder)
				}

				dropped := librarySource

				if !hasAppFrames || app[index] {
					fromNewest := sourceCount - 1 - withSource[index]

					if sourceCount <= detailedFrames || withSource[index] < detailedFrames/2 || fromNewest < detailedFrames/2 {
						return scrubElement(decoder, encoder)
					}

					dropped = middleDetail
				}

				return scrubObject(decoder, encoder, func(name string, decoder *jsontext.Decoder, _ *jsontext.Encoder) (bool, error) {
					if dropped[sentry.FoldedName(name)] {
						return true, decoder.SkipValue()
					}

					return false, nil
				})
			})
		})
	})
}

// librarySource are the fields of a library frame dropped when the stack has frames of the application.
var librarySource = map[string]bool{"pre_context": true, "context_line": true, "post_context": true, "vars": true}

// middleDetail are the fields a frame in the middle of a long stack loses;
// its source line stays, grouping uses it.
var middleDetail = map[string]bool{"pre_context": true, "post_context": true, "vars": true}

// elements reads the next list as its raw elements.
func elements(decoder *jsontext.Decoder) ([]jsontext.Value, error) {
	if _, err := decoder.ReadToken(); err != nil {
		return nil, err
	}

	var items []jsontext.Value

	for decoder.PeekKind() != ']' {
		item, err := decoder.ReadValue()

		if err != nil {
			return nil, err
		}

		items = append(items, item.Clone()) // a read value is valid only until the next read
	}

	_, err := decoder.ReadToken()

	return items, err
}

// writeList writes raw elements as a list, each through write.
func writeList(encoder *jsontext.Encoder, items []jsontext.Value, write func(index int, decoder *jsontext.Decoder, encoder *jsontext.Encoder) error) error {
	if err := encoder.WriteToken(jsontext.BeginArray); err != nil {
		return err
	}

	for index, item := range items {
		if err := write(index, newTokenDecoder(item), encoder); err != nil {
			return err
		}
	}

	return encoder.WriteToken(jsontext.EndArray)
}

// isAppFrame is sentry.Frame.IsInApp on a raw frame, its fields read leniently.
func isAppFrame(raw jsontext.Value) bool {
	var fields struct {
		InApp    any `json:"in_app"`
		Filename any `json:"filename"`
		AbsPath  any `json:"abs_path"`
	}

	if raw.Kind() != '{' || json.Unmarshal(raw, &fields) != nil {
		return false
	}

	inApp := fields.InApp == true
	frame := sentry.Frame{InApp: &inApp}
	frame.Filename, _ = fields.Filename.(string)
	frame.AbsPath, _ = fields.AbsPath.(string)

	return frame.IsInApp()
}
