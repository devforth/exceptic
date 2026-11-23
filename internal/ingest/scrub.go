package ingest

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/json/jsontext"
	"io"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/devforth/exceptic/internal/platform/textutil"
)

// filteredText replaces a removed credential: the value of a key that names
// a secret, a Bearer/Basic token, a sensitive URL parameter or a URL password.
const filteredText = "[Filtered]"

// sensitiveKey matches keys whose value is a credential. Matched anywhere in
// the key, so "Authorization", "X-Api-Key", "http.request.header.cookie" and
// "user_password" are all caught. It runs on every key of every payload, so
// ASCII keys (all of them in practice) are checked by sensitiveASCIIKey and
// remembered; scrub_test.go checks that both agree.
var sensitiveKey = regexp.MustCompile(`(?i)passw|passwd|pwd|secret|token([^s]|$)|api[-_.]?key|apikey|auth|credential|cookie|session|private[-_.]?key|csrf|xsrf|signature|dsn|card[-_.]?number|cvv|ssn`)

// isToken reports whether what follows Bearer or Basic is a credential: it has
// a digit or a base64 sign, a capital or a '-' or '.' inside, is longer than
// words are (JWTs always are), or, after Basic, decodes to "user:password".
func isToken(scheme, word string) bool {
	if len(word) >= 16 || strings.ContainsAny(word, "0123456789=+/_~") {
		return true
	}

	if strings.ContainsAny(word[:len(word)-1], "-.") || strings.ToLower(word[1:]) != word[1:] {
		return true
	}

	if !strings.EqualFold(scheme, "basic") {
		return false
	}

	decoded, err := base64.RawStdEncoding.DecodeString(word)

	return err == nil && utf8.Valid(decoded) && bytes.ContainsRune(decoded, ':')
}

// knownKeys remembers keys already checked: payloads repeat the same few
// hundred. Bounded, so random keys cannot grow it without end.
var (
	knownKeys     sync.Map
	knownKeyCount atomic.Int64
)

// Only keys this short are remembered: real keys are short, and long ones
// sent on purpose must not fill memory (10000 keys of 64 bytes at most).
const (
	maxKnownKeys      = 10000
	maxKnownKeyLength = 64
)

// isSensitiveKey reports whether a value stored under key is a credential.
func isSensitiveKey(key string) bool {
	if known, ok := knownKeys.Load(key); ok {
		return known.(bool)
	}

	var sensitive bool

	if isASCII(key) {
		sensitive = sensitiveASCIIKey(strings.ToLower(key))
	} else {
		sensitive = sensitiveKey.MatchString(key) // Unicode case folding, as before
	}

	if len(key) <= maxKnownKeyLength && knownKeyCount.Load() < maxKnownKeys {
		if _, loaded := knownKeys.LoadOrStore(key, sensitive); !loaded {
			knownKeyCount.Add(1)
		}
	}

	return sensitive
}

// sensitiveWords are the parts of sensitiveKey without separators or
// conditions ("passwd" is covered by "passw").
var sensitiveWords = []string{"passw", "pwd", "secret", "apikey", "auth", "credential", "cookie", "session", "csrf", "xsrf", "signature", "dsn", "cvv", "ssn"}

// sensitiveASCIIKey is sensitiveKey for a lowercase ASCII key.
func sensitiveASCIIKey(key string) bool {
	for _, word := range sensitiveWords {
		if strings.Contains(key, word) {
			return true
		}
	}

	// "token" not followed by "s": "tokens" alone is a count, "tokenizer" is not.
	for rest := key; ; {
		index := strings.Index(rest, "token")

		if index < 0 {
			break
		}

		after := index + len("token")

		if after == len(rest) || rest[after] != 's' {
			return true
		}

		rest = rest[after:]
	}

	// api[-_.]?key, private[-_.]?key, card[-_.]?number
	for _, pair := range [...][2]string{{"api", "key"}, {"private", "key"}, {"card", "number"}} {
		for rest := key; ; {
			index := strings.Index(rest, pair[0])

			if index < 0 {
				break
			}

			after := rest[index+len(pair[0]):]

			if strings.HasPrefix(after, pair[1]) || len(after) > 0 && strings.IndexByte("-_.", after[0]) >= 0 && strings.HasPrefix(after[1:], pair[1]) {
				return true
			}

			rest = rest[index+1:]
		}
	}

	return false
}

func isASCII(text string) bool {
	for index := 0; index < len(text); index++ {
		if text[index] >= 0x80 {
			return false
		}
	}

	return true
}

// newTokenDecoder reads a payload as a token stream. SDKs may repeat a key or send
// invalid UTF-8; both are accepted, as the event parser accepts them.
func newTokenDecoder(payload []byte) *jsontext.Decoder {
	return jsontext.NewDecoder(bytes.NewReader(payload), jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true))
}

// newTokenEncoder writes a token stream compactly, without HTML escaping.
func newTokenEncoder(output io.Writer) *jsontext.Encoder {
	return jsontext.NewEncoder(output, jsontext.EscapeForHTML(false), jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true))
}

// scrubJSON returns payload with credentials replaced, read and written as one
// token stream (no tree is built). Input that is not valid JSON is returned
// unchanged: the parser rejects it later anyway.
func scrubJSON(payload []byte) []byte {
	var output bytes.Buffer

	if err := scrubValue(newTokenDecoder(payload), newTokenEncoder(&output)); err != nil {
		return payload
	}

	return bytes.TrimRight(output.Bytes(), "\n")
}

// scrubber removes credentials from JSON, its strings through text: scrubText
// in a payload, scrubFreeText in a JSON document carried in a string.
type scrubber struct {
	text func(string) string
}

// scrubValue copies the next value of a payload from decoder to encoder with
// credentials removed: values of keys that name a secret, the second of a
// [name, value] pair whose name does, and credentials in strings (scrubText).
func scrubValue(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	return scrubber{scrubText}.value(decoder, encoder)
}

// scrubObject is scrubber.object in a payload.
func scrubObject(decoder *jsontext.Decoder, encoder *jsontext.Encoder,
	member func(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder) (bool, error),
) error {
	return scrubber{scrubText}.object(decoder, encoder, member)
}

func (scrub scrubber) value(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	switch decoder.PeekKind() {
	case '{':
		return scrub.object(decoder, encoder, nil)
	case '[':
		return scrub.array(decoder, encoder)
	case '"':
		return scrub.stringValue(decoder, encoder)
	}

	value, err := decoder.ReadValue()

	if err != nil {
		return err
	}

	return encoder.WriteValue(value)
}

// object copies the next object member by member. member, when set, may take
// one over (write it, or drop it by writing nothing) and says whether it did;
// a member whose key names a secret is filtered before member sees it.
func (scrub scrubber) object(decoder *jsontext.Decoder, encoder *jsontext.Encoder,
	member func(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder) (bool, error),
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

		if isSensitiveKey(name) {
			if err := encoder.WriteToken(jsontext.String(name)); err != nil {
				return err
			}

			if err := writeFiltered(decoder, encoder); err != nil {
				return err
			}

			continue
		}

		if member != nil {
			if done, err := member(name, decoder, encoder); err != nil || done {
				if err != nil {
					return err
				}

				continue
			}
		}

		if err := encoder.WriteToken(jsontext.String(name)); err != nil {
			return err
		}

		if err := scrub.value(decoder, encoder); err != nil {
			return err
		}
	}

	return copyToken(decoder, encoder)
}

// scrubValues copies the next array element by element through each.
func scrubValues(decoder *jsontext.Decoder, encoder *jsontext.Encoder, each func(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error) error {
	if err := copyToken(decoder, encoder); err != nil {
		return err
	}

	for decoder.PeekKind() != ']' {
		if err := each(decoder, encoder); err != nil {
			return err
		}
	}

	return copyToken(decoder, encoder)
}

// array copies an array. One led by a secret's name may be a [name, value]
// pair (headers and query strings may come this way), whose value is
// filtered; that takes its length, so such an array (a small one in
// practice) is read whole and scrubbed as a tree: rereading each element
// as a stream would take time growing with the square of its nesting.
func (scrub scrubber) array(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	if err := copyToken(decoder, encoder); err != nil {
		return err
	}

	if decoder.PeekKind() == '"' {
		first, err := decoder.ReadValue()

		if err != nil {
			return err
		}

		first = first.Clone() // a read value is valid only until the next read

		var name string

		if err := json.Unmarshal(first, &name); err != nil {
			return err
		}

		if !isSensitiveKey(name) {
			if err := scrub.writeText(encoder, first, name); err != nil {
				return err
			}
		} else {
			return scrub.ledBySecret(decoder, encoder, first, name)
		}
	}

	for decoder.PeekKind() != ']' {
		if err := scrub.value(decoder, encoder); err != nil {
			return err
		}
	}

	return copyToken(decoder, encoder)
}

// ledBySecret writes the rest of an array whose first element, a secret's
// name, is read: a pair's value is filtered, the elements of a longer array
// are scrubbed as trees.
func (scrub scrubber) ledBySecret(decoder *jsontext.Decoder, encoder *jsontext.Encoder, first jsontext.Value, name string) error {
	var rest []jsontext.Value

	for decoder.PeekKind() != ']' {
		element, err := decoder.ReadValue()

		if err != nil {
			return err
		}

		rest = append(rest, element.Clone())
	}

	if _, err := decoder.ReadToken(); err != nil {
		return err
	}

	if len(rest) == 1 {
		if err := encoder.WriteValue(first); err != nil {
			return err
		}

		if err := writeFiltered(newTokenDecoder(rest[0]), encoder); err != nil {
			return err
		}
	} else {
		if err := scrub.writeText(encoder, first, name); err != nil {
			return err
		}

		for _, element := range rest {
			reader := json.NewDecoder(bytes.NewReader(element))
			reader.UseNumber()

			var node any

			if err := reader.Decode(&node); err != nil {
				return err
			}

			if err := writeJSON(encoder, scrubTree(node, scrub.text)); err != nil {
				return err
			}
		}
	}

	return encoder.WriteToken(jsontext.EndArray)
}

// scrubTree removes credentials from a decoded value in place, by the rules
// of scrubber.value, its strings through text.
func scrubTree(node any, text func(string) string) any {
	switch typed := node.(type) {
	case map[string]any:
		for key, child := range typed {
			if isSensitiveKey(key) {
				typed[key] = filteredNode(child)
			} else {
				typed[key] = scrubTree(child, text)
			}
		}

		return typed
	case []any:
		if len(typed) == 2 {
			if name, ok := typed[0].(string); ok && isSensitiveKey(name) {
				typed[1] = filteredNode(typed[1])

				return typed
			}
		}

		for index, child := range typed {
			typed[index] = scrubTree(child, text)
		}

		return typed
	case string:
		return text(typed)
	default:
		return node
	}
}

// filteredNode is what replaces a secret's value in a tree (see writeFiltered).
func filteredNode(node any) any {
	if attribute, ok := node.(map[string]any); ok {
		if _, hasValue := attribute["value"]; hasValue {
			return map[string]any{"value": filteredText, "type": "string"}
		}
	}

	if node == nil {
		return nil
	}

	return filteredText
}

func copyToken(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	token, err := decoder.ReadToken()

	if err != nil {
		return err
	}

	return encoder.WriteToken(token)
}

// stringValue copies a string, through text only when it may hold a
// credential; most strings are copied as they came.
func (scrub scrubber) stringValue(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	raw, err := decoder.ReadValue()

	if err != nil {
		return err
	}

	if !mayHoldCredential(raw) {
		return encoder.WriteValue(raw)
	}

	var text string

	if err := json.Unmarshal(raw, &text); err != nil {
		return err
	}

	return scrub.writeText(encoder, raw, text)
}

// writeText writes text scrubbed; unchanged, the raw value as it came.
func (scrub scrubber) writeText(encoder *jsontext.Encoder, raw jsontext.Value, text string) error {
	if scrubbed := scrub.text(text); scrubbed != text {
		return encoder.WriteToken(jsontext.String(scrubbed))
	}

	return encoder.WriteValue(raw)
}

// mayHoldCredential reports whether scrubText could change a raw JSON string: it
// has "=" (a query), "@" (a URL's userinfo), "bearer" or "basic" in any
// case, or an escape or a byte beyond ASCII, which are checked the slow way.
func mayHoldCredential(raw []byte) bool {
	for index, char := range raw {
		switch {
		case char == '=' || char == '@' || char == '\\' || char >= 0x80:
			return true
		case char|0x20 == 'b':
			rest := raw[index:]

			if len(rest) >= 5 && bytes.EqualFold(rest[:5], []byte("basic")) || len(rest) >= 6 && bytes.EqualFold(rest[:6], []byte("bearer")) {
				return true
			}
		}
	}

	return false
}

// filteredAttribute replaces an object with a "value" member: a log
// attribute keeps its {"value", "type"} shape, so the log still parses, and
// nothing else of the object stays.
var filteredAttribute = jsontext.Value(`{"type":"string","value":"` + filteredText + `"}`)

// writeFiltered replaces the next value: null stays null, an object with a
// "value" member becomes filteredAttribute, anything else filteredText.
func writeFiltered(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	raw, err := decoder.ReadValue()

	if err != nil {
		return err
	}

	switch raw.Kind() {
	case 'n':
		return encoder.WriteValue(raw)
	case '{':
		if hasValueMember(raw) {
			return encoder.WriteValue(filteredAttribute)
		}
	}

	return encoder.WriteToken(jsontext.String(filteredText))
}

// hasValueMember reports whether a raw object has a "value" member.
func hasValueMember(object jsontext.Value) bool {
	decoder := newTokenDecoder(object)

	if _, err := decoder.ReadToken(); err != nil {
		return false
	}

	for decoder.PeekKind() == '"' {
		name, err := decoder.ReadToken()

		if err != nil {
			return false
		}

		if name.String() == "value" {
			return true
		}

		if decoder.SkipValue() != nil {
			return false
		}
	}

	return false
}

func writeJSON(encoder *jsontext.Encoder, value any) error {
	var output bytes.Buffer

	writer := json.NewEncoder(&output)
	writer.SetEscapeHTML(false)

	if err := writer.Encode(value); err != nil {
		return err
	}

	return encoder.WriteValue(bytes.TrimRight(output.Bytes(), "\n"))
}

// scrubText removes credentials from a string of a payload: a JSON document
// (a request body sent as text) by the rules of the payload, anything else
// as free text.
func scrubText(text string) string {
	if scrubbed, isJSON := scrubJSONText(text); isJSON {
		return scrubbed
	}

	return scrubFreeText(text)
}

// scrubFreeText removes credentials from free text: "key": value pairs of
// JSON inside it or cut short, Bearer/Basic tokens, and in URLs (absolute or
// relative, whole or inside text) and bare query strings the sensitive
// parameters of the query and fragment and the userinfo password.
func scrubFreeText(text string) string {
	if text == "" {
		return text
	}

	if strings.Contains(text, `"`) {
		text = scrubQuotedPairs(text)
	}

	// The regexps run only when their words are there: most strings have none
	// (non-ASCII text always: the regexp also folds letters such as ſ to s).
	if !isASCII(text) || textutil.ContainsFold(text, "bearer") || textutil.ContainsFold(text, "basic") {
		text = textutil.AuthCredential.ReplaceAllStringFunc(text, func(match string) string {
			words := strings.Fields(match) // the scheme and the token

			if !isToken(words[0], words[1]) {
				return match
			}

			return words[0] + " " + filteredText
		})
	}

	if !strings.Contains(text, "=") && (!strings.Contains(text, "@") || !strings.Contains(text, "://")) {
		return text
	}

	if !strings.ContainsAny(text, " \t\n") {
		return scrubLink(text)
	}

	return linkInText.ReplaceAllStringFunc(text, scrubLink)
}

// scrubJSONText scrubs text that is one JSON object or array, such as a
// request body. Its strings are free text: a document in one of them is not
// read as a document again (only its "key": value pairs are scrubbed), so
// every byte is read a fixed number of times however deep documents nest.
// Text with nothing to remove is returned as it came.
func scrubJSONText(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)

	if trimmed == "" || trimmed[0] != '{' && trimmed[0] != '[' {
		return "", false
	}

	decoder := newTokenDecoder([]byte(trimmed))

	var scrubbed bytes.Buffer

	if (scrubber{scrubFreeText}).value(decoder, newTokenEncoder(&scrubbed)) != nil {
		return "", false
	}

	if _, err := decoder.ReadToken(); err != io.EOF {
		return "", false // more than one value: text with JSON in it
	}

	// Both are written the same way, so they differ only where a value was replaced.
	var compact bytes.Buffer

	if value, err := newTokenDecoder([]byte(trimmed)).ReadValue(); err != nil || newTokenEncoder(&compact).WriteValue(value) != nil {
		return "", false
	}

	if bytes.Equal(scrubbed.Bytes(), compact.Bytes()) {
		return text, true
	}

	return string(bytes.TrimRight(scrubbed.Bytes(), "\n")), true
}

// scrubQuotedPairs replaces the value of every "key": value pair of JSON in
// text that is not a JSON document (a log line with a body, a message cut by
// the SDK) whose key names a secret: a string, ended or cut at the end of the
// text, or a number. One pass: each string is skipped whole.
func scrubQuotedPairs(text string) string {
	var output strings.Builder

	copied := 0

	for position := 0; ; {
		open := strings.IndexByte(text[position:], '"')

		if open < 0 {
			break
		}

		open += position
		keyEnd := quotedEnd(text, open)

		if keyEnd < 0 {
			break
		}

		position = keyEnd
		colon := skipJSONSpace(text, keyEnd)

		if colon == len(text) || text[colon] != ':' || !isSensitiveKey(quotedKey(text[open:keyEnd])) {
			continue
		}

		valueStart := skipJSONSpace(text, colon+1)
		valueEnd := valueStart

		switch {
		case valueStart == len(text):
		case text[valueStart] == '"':
			if valueEnd = quotedEnd(text, valueStart); valueEnd < 0 {
				valueEnd = len(text) // cut short
			}
		case text[valueStart] == '-' || text[valueStart] >= '0' && text[valueStart] <= '9':
			valueEnd += len(text[valueStart:]) - len(strings.TrimLeft(text[valueStart:], "-0123456789.eE+"))
		}

		if valueEnd == valueStart {
			continue // an object, an array or a literal
		}

		output.WriteString(text[copied:valueStart])
		output.WriteString(`"` + filteredText + `"`)

		copied, position = valueEnd, valueEnd
	}

	if copied == 0 {
		return text
	}

	return output.String() + text[copied:]
}

// quotedEnd returns the index after the JSON string that starts at open, or
// -1 when the text ends inside it.
func quotedEnd(text string, open int) int {
	for index := open + 1; index < len(text); index++ {
		switch text[index] {
		case '\\':
			index++
		case '"':
			return index + 1
		}
	}

	return -1
}

// quotedKey is a quoted JSON string's text; one that does not decode, as it is.
func quotedKey(quoted string) string {
	if !strings.Contains(quoted, `\`) {
		return quoted[1 : len(quoted)-1]
	}

	var key string

	if json.Unmarshal([]byte(quoted), &key) != nil {
		return quoted[1 : len(quoted)-1]
	}

	return key
}

func skipJSONSpace(text string, index int) int {
	for index < len(text) && strings.IndexByte(" \t\r\n", text[index]) >= 0 {
		index++
	}

	return index
}

// linkInText finds absolute URLs, and relative ones with a query or fragment
// ("GET /reset?token=… 200"), inside longer text.
var linkInText = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+|/[^\s"'<>?#]*[?#][^\s"'<>]+`)

// filteredParameter replaces a value in a query or userinfo.
var filteredParameter = url.QueryEscape(filteredText)

// scrubLink scrubs one URL or bare query string; the rest of it stays byte for byte.
func scrubLink(text string) string {
	text = withoutPasswords(text)

	// A URL, absolute or relative, has no "=" before its first '/', ':', '?' or '#'.
	equals := strings.IndexByte(text, '=')

	if delimiter := strings.IndexAny(text, "/:?#"); equals >= 0 && (delimiter < 0 || equals < delimiter) {
		return scrubQuery(text) // such as request.query_string, sent raw
	}

	start := strings.IndexAny(text, "?#")

	if start < 0 {
		return text
	}

	return text[:start+1] + scrubQuery(text[start+1:])
}

// withoutPasswords replaces the password in every "scheme://user:password@host", keeping the user.
func withoutPasswords(text string) string {
	for from := 0; ; {
		scheme := strings.Index(text[from:], "://")

		if scheme < 0 {
			return text
		}

		from += scheme + len("://")

		if colon, at, found := userinfoPassword(text[from:]); found {
			text = text[:from+colon+1] + filteredParameter + text[from+at:]
			from += colon + 1 + len(filteredParameter)
		}
	}
}

// userinfoPassword finds the ':' and the '@' around the password of the userinfo
// that starts authority; the password itself may hold '/', '?', '#' or '@'.
func userinfoPassword(authority string) (colon, at int, found bool) {
	colon = strings.IndexAny(authority, ":/?#@")

	if colon < 0 || authority[colon] != ':' {
		return 0, 0, false
	}

	rest := authority[colon+1:]
	digits := len(rest) - len(strings.TrimLeft(rest, "0123456789"))

	if digits > 0 && (digits == len(rest) || strings.IndexByte("/?#,;", rest[digits]) >= 0) {
		return 0, 0, false // a port
	}

	offset := strings.IndexByte(rest, '@')

	if offset < 0 {
		return 0, 0, false
	}

	at = colon + 1 + offset

	// In "p@ss@host" the last '@' before the host's end ends the password.
	host := authority[at+1:]

	if hostEnd := strings.IndexAny(host, "/?#"); hostEnd >= 0 {
		host = host[:hostEnd]
	}

	if last := strings.LastIndexByte(host, '@'); last >= 0 {
		at += 1 + last
	}

	return colon, at, true
}

// scrubQuery replaces the values of sensitive parameters; everything else stays
// as it came. A '?' or '#' starts more pairs: "next=/reset?token=…".
func scrubQuery(raw string) string {
	var output strings.Builder

	changed := false

	for start := 0; start <= len(raw); {
		end := strings.IndexAny(raw[start:], "&;?#")

		if end < 0 {
			end = len(raw)
		} else {
			end += start
		}

		pair := raw[start:end]

		if key, _, hasValue := strings.Cut(pair, "="); hasValue && sensitiveParameter(key) {
			pair = key + "=" + filteredParameter
			changed = true
		}

		output.WriteString(pair)

		if end < len(raw) {
			output.WriteByte(raw[end])
		}

		start = end + 1
	}

	if !changed {
		return raw
	}

	return output.String()
}

func sensitiveParameter(key string) bool {
	if unescaped, err := url.QueryUnescape(key); err == nil {
		key = unescaped
	}

	return isSensitiveKey(key)
}
