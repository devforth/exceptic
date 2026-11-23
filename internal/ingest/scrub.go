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
	"unicode"
	"unicode/utf8"

	"github.com/devforth/exceptic/internal/platform/textutil"
)

// filteredText replaces a removed credential: the value of a key that names
// a secret, the credential of an authorization scheme, a sensitive URL
// parameter, a URL password, a card number or the body of a private key.
const filteredText = "[Filtered]"

// isToken reports whether what follows a scheme such as Bearer is a credential: it has
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

// isSensitiveKey reports whether a value stored under key is a credential
// (namesSecret).
func isSensitiveKey(key string) bool {
	if known, ok := knownKeys.Load(key); ok {
		return known.(bool)
	}

	sensitive := namesSecret(key)

	if len(key) <= maxKnownKeyLength && knownKeyCount.Load() < maxKnownKeys {
		if _, loaded := knownKeys.LoadOrStore(key, sensitive); !loaded {
			knownKeyCount.Add(1)
		}
	}

	return sensitive
}

// forEachWord calls each with the words of a key in lowercase. A key splits
// at every character that is not a letter or a digit, before a capital after
// a lowercase letter ("apiKey"), before the last capital of a run followed by
// a lowercase letter ("APIKey") and before a digit after a letter
// ("Signature256"). Letters that Unicode folds to ASCII are folded ("ſecret"
// is "secret").
func forEachWord(key string, each func(word string)) {
	if !isASCII(key) {
		key = strings.Map(foldToASCII, key)
	}

	start, previous := -1, rune(0) // start of the current word

	for index, char := range key {
		isWordChar := unicode.IsLetter(char) || unicode.IsDigit(char)

		if start >= 0 {
			_, size := utf8.DecodeRuneInString(key[index:])
			next, _ := utf8.DecodeRuneInString(key[index+size:])
			split := !isWordChar ||
				unicode.IsUpper(char) && (unicode.IsLower(previous) || unicode.IsUpper(previous) && unicode.IsLower(next)) ||
				unicode.IsDigit(char) && unicode.IsLetter(previous)

			if split {
				each(strings.ToLower(key[start:index]))

				start = -1
			}
		}

		if isWordChar && start < 0 {
			start = index
		}

		previous = char
	}

	if start >= 0 {
		each(strings.ToLower(key[start:]))
	}
}

// foldToASCII returns the ASCII letter Unicode folds char to, in its case, or char.
func foldToASCII(char rune) rune {
	if char < utf8.RuneSelf {
		return char
	}

	for folded := unicode.SimpleFold(char); folded != char; folded = unicode.SimpleFold(folded) {
		if folded < utf8.RuneSelf {
			if unicode.IsUpper(char) {
				return unicode.ToUpper(folded)
			}

			return unicode.ToLower(folded)
		}
	}

	return char
}

// namesSecret decides on the words of a key (forEachWord). A word names a
// secret when it is one (secretWords), ends with one written without
// separators ("csrftoken", secretSuffixes), or is a word that does only after
// certain words ("card_number", "recovery_codes", qualifiedSecrets; "key"
// after any word but those of plainKeys, so "APP_KEY" and "access_key_id" but
// not "public_key", "key_id" or "key"). The key is about the secret rather
// than the secret itself when a later word describes it ("session_count",
// "token_expires_at", describingWords) or it starts with "is" or "has".
func namesSecret(key string) bool {
	secret, isFlag, previous := false, false, ""

	forEachWord(key, func(word string) {
		if previous == "" && (word == "is" || word == "has") {
			isFlag = true
		}

		if describingWords[word] {
			secret = false // every secret's word before it is described
		}

		if !isFlag && wordNamesSecret(word, previous) {
			secret = true
		}

		previous = word
	})

	return secret
}

func wordNamesSecret(word, previous string) bool {
	if secretWords[word] {
		return true
	}

	for _, suffix := range secretSuffixes {
		if len(word) > len(suffix) && strings.HasSuffix(word, suffix) {
			return true
		}
	}

	if word == "key" || word == "keys" {
		return previous != "" && !plainKeys[previous]
	}

	return qualifiedSecrets[word][previous]
}

func wordSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))

	for _, word := range words {
		set[word] = true
	}

	return set
}

var (
	secretWords = wordSet(
		"password", "passwords", "passwd", "pass", "pw", "pwd", "passphrase", "passcode", "secret", "secrets",
		"token", "auth", "authorization", "credential", "credentials", "creds", "cookie", "cookies",
		"session", "sessionid", "sessid", "phpsessid", "jsessionid", "sid", "csrf", "xsrf",
		"apikey", "privatekey", "secretkey", "accesskey", "signature", "sig", "dsn", "jwt",
		"ssn", "cvv", "cvc", "iban", "pin", "otp", "totp", "hotp",
		// "password" in other languages.
		"passwort", "kennwort", "contraseña", "contrasena", "motdepasse", "senha", "wachtwoord", "hasło", "haslo", "heslo",
		"lösenord", "losenord", "passord", "adgangskode", "salasana", "jelszó", "jelszo", "пароль", "密码", "密碼", "パスワード", "비밀번호",
	)
	// secretSuffixes end words that join a secret to the word before it.
	secretSuffixes = []string{"token", "secret", "password", "passwd", "apikey", "privatekey", "secretkey", "accesskey", "sessid", "sessionid"}
	// qualifiedSecrets name a secret only after one of their words.
	qualifiedSecrets = map[string]map[string]bool{
		"tokens": wordSet("access", "refresh", "api", "auth", "id", "bearer", "session", "csrf", "xsrf", "oauth", "personal", "device"),
		"code":   wordSet("pin", "security", "verification", "verify", "mfa", "2fa", "otp", "totp", "recovery", "backup", "oauth", "authorization", "reset", "confirmation", "sms", "login"),
		"codes":  wordSet("pin", "security", "verification", "mfa", "2fa", "otp", "totp", "recovery", "backup"),
		"number": wordSet("card"),
		"card":   wordSet("credit", "debit", "payment"),
		"url":    wordSet("webhook", "hook"),
		"passe":  wordSet("de"), // mot de passe
		"uri":    wordSet("webhook", "hook"),
	}
	// plainKeys are the words before "key" that name no secret key.
	plainKeys = wordSet(
		"public", "primary", "foreign", "cache", "idempotency", "sort", "partition", "hash", "range", "row", "unique", "composite",
		"natural", "surrogate", "candidate", "lookup", "map", "object", "group", "routing", "dedup", "dedupe", "deduplication",
		"event", "translation", "i18n", "message", "error", "query", "redis", "storage", "local", "state", "store", "index",
		"shard", "column", "field", "attribute", "property", "prop", "metric", "setting", "config", "feature", "flag", "lock",
		"job", "task", "queue", "rate", "limit", "throttle", "list", "item", "entry", "cursor", "page", "bucket", "file", "path",
		"template", "hot", "shortcut", "modifier", "press", "music", "table", "name", "id", "react", "issue",
	)
	// describingWords after a secret's word make the key about the secret: its count, size, time or kind.
	describingWords = wordSet(
		"count", "counts", "length", "len", "size", "strength", "score", "policy", "type", "kind", "provider", "method", "mode", "scheme",
		"required", "enabled", "disabled", "expires", "expiry", "expiration", "expired", "ttl", "timeout", "duration", "age", "lifetime",
		"at", "time", "timestamp", "date", "started", "start", "ended", "end", "created", "updated", "changed", "last", "rate", "sample",
		"set", "used", "valid", "version", "name", "status", "state", "error", "errors", "result", "level", "strategy", "flow", "source",
		"url", "uri", "endpoint", "path", "replay", "exception", "manager", "mismatch", "missing", "invalid", "failed", "failure",
		"not", "found", "denied", "rejected", "consent", "domain", "driver",
	)
	// numericSecretWords name secrets that may be a few digits: a short
	// number under any other secret's name is a count or a status.
	numericSecretWords = wordSet(
		"pin", "cvv", "cvc", "otp", "totp", "hotp", "code", "codes", "passcode", "password", "passwords", "passwd", "pwd", "pw", "ssn", "secret",
	)
)

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

// scrubElement is scrubber.element in a payload.
func scrubElement(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	return scrubber{scrubText}.element(decoder, encoder)
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
		return scrub.array(decoder, encoder, false)
	case '"':
		return scrub.stringValue(decoder, encoder)
	}

	value, err := decoder.ReadValue()

	if err != nil {
		return err
	}

	if value.Kind() == '0' && isCardNumber(value) {
		return encoder.WriteToken(jsontext.String(filteredText))
	}

	return encoder.WriteValue(value)
}

// object copies the next object member by member. takeOver, when set, may
// take one over (write it, or drop it by writing nothing) and says whether it
// did; a member whose key names a secret is filtered before takeOver sees it.
func (scrub scrubber) object(decoder *jsontext.Decoder, encoder *jsontext.Encoder,
	takeOver func(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder) (bool, error),
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

			write := writeFiltered

			if isCookieKey(name) {
				write = scrub.cookies
			}

			if err := write(decoder, encoder); err != nil {
				return err
			}

			continue
		}

		if takeOver != nil {
			if done, err := takeOver(name, decoder, encoder); err != nil || done {
				if err != nil {
					return err
				}

				continue
			}
		}

		write := scrub.value

		switch {
		case sourceFields[name]:
			write = scrubber{scrubSourceText}.value
		case isCardKey(name) && decoder.PeekKind() == '{':
			write = scrub.card
		}

		if err := member(name, decoder, encoder, write); err != nil {
			return err
		}
	}

	return copyToken(decoder, encoder)
}

// sourceFields hold a frame's source lines: code, scrubbed by scrubSourceText.
var sourceFields = map[string]bool{"pre_context": true, "context_line": true, "post_context": true}

// isCardKey reports whether key holds a payment card ("card",
// "billing_card"), whose number member is a secret (cardNumberFields).
func isCardKey(key string) bool {
	last := ""

	forEachWord(key, func(word string) {
		last = word
	})

	return last == "card"
}

// cardNumberFields name a card's number inside its card.
var cardNumberFields = wordSet("number", "num", "pan")

// card copies the next object, a card, with its number filtered.
func (scrub scrubber) card(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	return scrub.object(decoder, encoder, func(name string, decoder *jsontext.Decoder, encoder *jsontext.Encoder) (bool, error) {
		if !cardNumberFields[strings.ToLower(name)] {
			return false, nil
		}

		return true, member(name, decoder, encoder, writeFiltered)
	})
}

// element copies the next element of an array: one that is an array may be
// a [name, value] pair.
func (scrub scrubber) element(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	if decoder.PeekKind() == '[' {
		return scrub.array(decoder, encoder, true)
	}

	return scrub.value(decoder, encoder)
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

// array copies an array. In a list (inList), one led by a secret's name may
// be a [name, value] pair (headers, query strings and tags may come as lists
// of pairs), whose value is filtered; outside a list two strings are a list
// of names, such as the fields a validation error names. Knowing a pair takes
// its length, so such an array (a small one in practice) is read whole and
// scrubbed as a tree: rereading each element as a stream would take time
// growing with the square of its nesting.
func (scrub scrubber) array(decoder *jsontext.Decoder, encoder *jsontext.Encoder, inList bool) error {
	if err := copyToken(decoder, encoder); err != nil {
		return err
	}

	if inList && decoder.PeekKind() == '"' {
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
		if err := scrub.element(decoder, encoder); err != nil {
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

			if err := writeJSON(encoder, scrubTree(node, scrub.text, true)); err != nil {
				return err
			}
		}
	}

	return encoder.WriteToken(jsontext.EndArray)
}

// scrubTree removes credentials from a decoded value in place, by the rules
// of scrubber.value (scrubber.element when inList), its strings through text.
func scrubTree(node any, text func(string) string, inList bool) any {
	switch typed := node.(type) {
	case map[string]any:
		for key, child := range typed {
			switch {
			case isCookieKey(key):
				typed[key] = filteredCookies(child, text)
			case isSensitiveKey(key):
				typed[key] = filteredNode(child)
			case sourceFields[key]:
				typed[key] = scrubTree(child, scrubSourceText, false)
			default:
				typed[key] = scrubTree(filteredCardNumber(key, child), text, false)
			}
		}

		return typed
	case []any:
		if inList && len(typed) == 2 {
			if name, ok := typed[0].(string); ok && isSensitiveKey(name) {
				typed[1] = filteredNode(typed[1])

				return typed
			}
		}

		for index, child := range typed {
			typed[index] = scrubTree(child, text, true)
		}

		return typed
	case string:
		return text(typed)
	case json.Number:
		if isCardNumber(typed) {
			return filteredText
		}

		return node
	default:
		return node
	}
}

// filteredCardNumber filters the number of a card (isCardKey) in a tree.
func filteredCardNumber(key string, node any) any {
	card, ok := node.(map[string]any)

	if !ok || !isCardKey(key) {
		return node
	}

	for name, value := range card {
		if cardNumberFields[strings.ToLower(name)] {
			card[name] = filteredNode(value)
		}
	}

	return card
}

// isCookieKey reports whether key holds the cookies of a request ("cookies",
// a "Cookie" header); "Set-Cookie" is filtered whole.
func isCookieKey(key string) bool {
	last, beforeLast := "", ""

	forEachWord(key, func(word string) {
		last, beforeLast = word, last
	})

	return (last == "cookie" || last == "cookies") && beforeLast != "set"
}

// harmlessCookies keep their plain values; any other cookie may hold a session.
var harmlessCookies = wordSet("theme", "lang", "language", "locale", "timezone", "tz")

// cookies copies the next value, a request's cookies, with every cookie's
// value filtered but those of harmlessCookies (filteredCookies). An object
// is copied member by member, in its order; a list is read as a tree.
func (scrub scrubber) cookies(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	raw, err := decoder.ReadValue()

	if err != nil {
		return err
	}

	if raw.Kind() == '{' && !hasValueMember(raw) {
		return scrub.cookieObject(newTokenDecoder(raw), encoder)
	}

	reader := json.NewDecoder(bytes.NewReader(raw))
	reader.UseNumber()

	var node any

	if err := reader.Decode(&node); err != nil {
		return err
	}

	return writeJSON(encoder, filteredCookies(node, scrub.text))
}

func (scrub scrubber) cookieObject(decoder *jsontext.Decoder, encoder *jsontext.Encoder) error {
	if err := copyToken(decoder, encoder); err != nil {
		return err
	}

	for decoder.PeekKind() != '}' {
		key, err := decoder.ReadToken()

		if err != nil {
			return err
		}

		name := key.String()
		write := writeFiltered

		if harmlessCookies[strings.ToLower(strings.TrimSpace(name))] && decoder.PeekKind() != '{' && decoder.PeekKind() != '[' {
			write = scrub.value
		}

		if err := member(name, decoder, encoder, write); err != nil {
			return err
		}
	}

	return copyToken(decoder, encoder)
}

// filteredCookies filters the values of cookies sent as an object, a
// "name=value; ..." string or a list of [name, value] pairs, keeping those
// of harmlessCookies; a log attribute's {"value", "type"} and anything else
// is filtered whole (filteredNode).
func filteredCookies(node any, text func(string) string) any {
	switch typed := node.(type) {
	case string:
		return filteredCookieText(typed)
	case map[string]any:
		if _, hasValue := typed["value"]; hasValue {
			return filteredNode(typed)
		}

		for name, value := range typed {
			typed[name] = filteredCookie(name, value, text)
		}

		return typed
	case []any:
		for index, element := range typed {
			pair, isList := element.([]any)

			if name, isName := firstString(pair); isList && len(pair) == 2 && isName {
				pair[1] = filteredCookie(name, pair[1], text)
			} else if cookie, isText := element.(string); isText {
				typed[index] = filteredCookieText(cookie)
			} else {
				typed[index] = filteredNode(element)
			}
		}

		return typed
	}

	return filteredNode(node)
}

func firstString(list []any) (string, bool) {
	if len(list) == 0 {
		return "", false
	}

	text, ok := list[0].(string)

	return text, ok
}

// filteredCookie keeps the value of a harmless cookie, a plain one: an
// object or list there is no cookie (and scrubbing it would read every
// level of cookies nested in cookies again).
func filteredCookie(name string, value any, text func(string) string) any {
	switch value.(type) {
	case map[string]any, []any:
	default:
		if harmlessCookies[strings.ToLower(strings.TrimSpace(name))] {
			return scrubTree(value, text, false)
		}
	}

	return filteredNode(value)
}

// filteredCookieText filters a Cookie header's values; the rest stays as it came.
func filteredCookieText(header string) string {
	parts := strings.Split(header, ";")

	for index, part := range parts {
		name, _, hasValue := strings.Cut(part, "=")
		trimmed := strings.TrimSpace(part)

		switch {
		case trimmed == "" || harmlessCookies[strings.ToLower(strings.TrimSpace(name))]:
		case hasValue:
			parts[index] = name + "=" + filteredText
		default:
			parts[index] = strings.Replace(part, trimmed, filteredText, 1)
		}
	}

	return strings.Join(parts, ";")
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
// has "=" (a query), ":" (a key: value pair), "@" (a URL's userinfo), "'" (a Python dict repr),
// an authorization scheme (hasAuthScheme), a card number's digits (hasCardDigits),
// a chat service's webhook path or "%3A" before a bot token (mayHoldServiceToken),
// "-----BEGIN" (a private key), or an escape (a quote among them) or a
// byte beyond ASCII, which are checked the slow way.
func mayHoldCredential(raw []byte) bool {
	if hasAuthScheme(raw) || hasCardDigits(raw) || bytes.Contains(raw, []byte("/services/")) || bytes.Contains(raw, []byte("webhooks/")) ||
		bytes.Contains(raw, []byte("%3")) {
		return true
	}

	for index, char := range raw {
		switch {
		case char == '=' || char == ':' || char == '@' || char == '\'' || char == '\\' || char >= 0x80:
			return true
		case char == '-' && bytes.HasPrefix(raw[index:], []byte(pemBegin)):
			return true
		}
	}

	return false
}

// mayHoldServiceToken reports whether text may hold a token that
// textutil.ReplaceServiceTokens replaces: a webhook's path, or digits (a
// bot's id) before ':' or "%3A".
func mayHoldServiceToken(text string) bool {
	if strings.Contains(text, "/services/") || strings.Contains(text, "webhooks/") {
		return true
	}

	for index := 1; index < len(text); index++ {
		if (text[index] == ':' || text[index] == '%') && isDigit(text[index-1]) {
			return true
		}
	}

	return false
}

// filteredServiceToken replaces a chat service's token, keeping the path before it.
func filteredServiceToken(path string) string {
	if path == "" {
		return filteredText
	}

	return path + filteredParameter
}

// authSchemes are the words authCredential starts at, in lowercase.
var authSchemes = []string{"bearer", "basic", "token", "apikey", "api-key", "digest", "aws4-hmac-sha256", "negotiate", "ntlm"}

// hasAuthScheme reports whether text has one of authSchemes in any case.
func hasAuthScheme[Text ~string | ~[]byte](text Text) bool {
	for index := 0; index < len(text); index++ {
		switch text[index] | 0x20 {
		case 'a', 'b', 'd', 'n', 't':
			for _, scheme := range authSchemes {
				if hasPrefixFold(text[index:], scheme) {
					return true
				}
			}
		}
	}

	return false
}

// hasPrefixFold reports whether text starts with prefix, written in
// lowercase, in any case of its ASCII letters.
func hasPrefixFold[Text ~string | ~[]byte](text Text, prefix string) bool {
	if len(text) < len(prefix) {
		return false
	}

	for index := 0; index < len(prefix); index++ {
		if text[index]|0x20 != prefix[index] {
			return false
		}
	}

	return true
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

// scrubFreeText removes credentials from free text: the bodies of private
// keys (scrubPrivateKeys), the tokens of chat services in URL paths, the
// secret fields of multipart form data (scrubMultipart), "key": value pairs
// of JSON inside it or cut short,
// the credentials of authorization schemes (authCredential) and of command
// lines (commandCredential), key=value pairs (scrubKeyValues), in URLs
// (absolute or relative, whole or inside text) and bare query strings the
// sensitive parameters of the path, query and fragment and the userinfo
// password, and card numbers (scrubCardNumbers).
func scrubFreeText(text string) string {
	return scrubTextAs(text, false)
}

// scrubSourceText is scrubFreeText for source code: a pair's value is an
// expression unless it is a string literal, so only literals are filtered
// ("const token = sign(key)" stays, "API_KEY = 'k1'" goes).
func scrubSourceText(text string) string {
	return scrubTextAs(text, true)
}

func scrubTextAs(text string, code bool) string {
	if text == "" {
		return text
	}

	if strings.Contains(text, pemBegin) {
		text = scrubPrivateKeys(text)
	}

	if mayHoldServiceToken(text) {
		text = textutil.ReplaceServiceTokens(text, filteredServiceToken)
	}

	if strings.Contains(text, "form-data") {
		text = scrubMultipart(text)
	}

	if strings.ContainsAny(text, `"'`) {
		text = scrubQuotedPairs(text)
	}

	// The regexps run only when their words are there: most strings have none
	// (non-ASCII text always: the regexp also folds letters such as ſ to s).
	if !isASCII(text) || hasAuthScheme(text) {
		text = authCredential.ReplaceAllStringFunc(text, filterAuthCredential)
	}

	if strings.Contains(text, "-u") || strings.Contains(text, "-U") {
		text = scrubCommandCredentials(text)
	}

	switch {
	case !strings.Contains(text, "=") && (!strings.Contains(text, "@") || !strings.Contains(text, "://")):
	case !strings.ContainsAny(text, " \t\n"):
		text = scrubLink(text)
	default:
		text = linkInText.ReplaceAllStringFunc(text, scrubLink)
	}

	if strings.ContainsAny(text, "=:") {
		text = scrubKeyValues(text, code)
	}

	if hasCardDigits(text) {
		text = scrubCardNumbers(text)
	}

	return text
}

// authParameter is one name=value of a Digest or AWS Signature V4 credential.
const authParameter = `[A-Za-z0-9_-]+=(?:"[^"]*"|[^\s,";]*(?:;[^\s,";]+)*)`

// authCredential finds the credential after an authorization scheme: the
// parameter list after Digest or AWS4-HMAC-SHA256, user:password after Basic,
// a token, quoted or not, after the other schemes.
var authCredential = regexp.MustCompile(`(?i)\b(?:(?:digest|aws4-hmac-sha256)\s+` + authParameter + `(?:\s*,\s*` + authParameter + `)*` +
	`|basic\s+[A-Za-z0-9._~+@-]+:[^\s"'<>]+` +
	`|(?:bearer|basic|token|api-?key|negotiate|ntlm)\s+(?:"[^"\s]+"|[A-Za-z0-9._~+/=-]{8,}))`)

// filterAuthCredential replaces a match of authCredential that is a
// credential (isToken): a parameter list whole, a quoted token inside its
// quotes, the password of user:password; the scheme and the user stay.
func filterAuthCredential(match string) string {
	schemeEnd := strings.IndexFunc(match, unicode.IsSpace)
	credential := strings.TrimLeftFunc(match[schemeEnd:], unicode.IsSpace)
	scheme, prefix := match[:schemeEnd], match[:len(match)-len(credential)]

	switch {
	case strings.EqualFold(scheme, "digest") || strings.EqualFold(scheme, "aws4-hmac-sha256"):
		return prefix + filteredText
	case strings.HasPrefix(credential, `"`):
		if !isSchemeToken(scheme, credential[1:len(credential)-1]) {
			return match
		}

		return prefix + `"` + filteredText + `"`
	case strings.EqualFold(scheme, "basic") && strings.Contains(credential, ":"):
		user, _, _ := strings.Cut(credential, ":")

		return prefix + user + ":" + filteredText
	case !isSchemeToken(scheme, credential):
		return match
	}

	return prefix + filteredText
}

// isSchemeToken is isToken for the credential of a scheme in text. A pair
// is no credential ("token method=POST": '=' only pads base64 at the end),
// and after "token", a word of parsers and prose too ("Unexpected token
// NumericLiteral"), a token there is long and has a digit.
func isSchemeToken(scheme, word string) bool {
	if strings.Contains(strings.TrimRight(word, "="), "=") {
		return false
	}

	if strings.EqualFold(scheme, "token") && (len(word) < 16 || !strings.ContainsAny(word, "0123456789")) {
		return false
	}

	return isToken(scheme, word)
}

// commandCredential finds the user:password of curl's -u, --user, -U and
// --proxy-user; group 1 is the password.
var commandCredential = regexp.MustCompile(`(?:^|[\s'"])(?:-u|-U|--user|--proxy-user)(?:\s+|=)['"]?[^\s:'"]+:([^\s'"]+)`)

// scrubCommandCredentials replaces the passwords commandCredential finds.
func scrubCommandCredentials(text string) string {
	matches := commandCredential.FindAllStringSubmatchIndex(text, -1)

	if matches == nil {
		return text
	}

	var output strings.Builder

	copied := 0

	for _, match := range matches {
		output.WriteString(text[copied:match[2]])
		output.WriteString(filteredText)

		copied = match[3]
	}

	return output.String() + text[copied:]
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

// scrubQuotedPairs replaces the value of every "key": value pair of JSON,
// and 'key': value of a Python dict repr, in text that is not a JSON
// document (a log line with a body, a message cut by the SDK) whose key names
// a secret: a quoted string or an array or object (collectionEnd), ended or
// cut at the end of the text, or a number. One pass: a "string" is skipped
// whole; a 'string' only when it is a key, since an apostrophe opens none, so
// a key is looked for in at most maxQuotedKey bytes; the scan goes on after
// each value it replaced.
func scrubQuotedPairs(text string) string {
	var output strings.Builder

	copied, quotes := 0, `"'`

	for position := 0; ; {
		open := strings.IndexAny(text[position:], quotes)

		if open < 0 {
			break
		}

		open += position
		quote := text[open]
		searched := len(text)

		if quote == '\'' {
			searched = min(searched, open+maxQuotedKey)
		}

		keyEnd := quotedEnd(text[:searched], open)

		colon := len(text)

		if keyEnd > 0 {
			colon = skipJSONSpace(text, keyEnd)
		}

		isPair := colon < len(text) && text[colon] == ':'

		switch {
		case keyEnd < 0 && quote == '"':
			quotes = "'" // no "string" ends any more, but a 'key' may
			position = open + 1

			continue
		case quote == '\'' && !isPair:
			position = open + 1 // an apostrophe, or a 'string' that is no key

			continue
		}

		position = keyEnd

		if !isPair || !isSensitiveKey(quotedKey(text[open:keyEnd])) {
			continue
		}

		valueStart := skipJSONSpace(text, colon+1)
		valueEnd := valueStart

		switch {
		case valueStart == len(text):
		case text[valueStart] == '"' || text[valueStart] == '\'':
			if valueEnd = quotedEnd(text, valueStart); valueEnd < 0 {
				valueEnd = len(text) // cut short
			}
		case text[valueStart] == '-' || text[valueStart] >= '0' && text[valueStart] <= '9':
			valueEnd += len(text[valueStart:]) - len(strings.TrimLeft(text[valueStart:], "-0123456789.eE+"))
		case text[valueStart] == '[' || text[valueStart] == '{':
			valueEnd = collectionEnd(text, valueStart)
		}

		if valueEnd == valueStart {
			continue // a literal
		}

		output.WriteString(text[copied:valueStart])
		output.WriteByte(quote)
		output.WriteString(filteredText)
		output.WriteByte(quote)

		copied, position = valueEnd, valueEnd
	}

	if copied == 0 {
		return text
	}

	return output.String() + text[copied:]
}

// scrubMultipart replaces the values of multipart/form-data fields whose
// name names a secret: a value follows its part's header lines and a blank
// line and ends before the line of the next boundary ("--…") or with the
// text. One pass over the lines.
func scrubMultipart(text string) string {
	var output strings.Builder

	copied, valueStart, inHeaders := 0, -1, false

	for lineStart := 0; lineStart < len(text); {
		lineEnd := len(text)

		if newline := strings.IndexByte(text[lineStart:], '\n'); newline >= 0 {
			lineEnd = lineStart + newline
		}

		line := strings.TrimSuffix(text[lineStart:lineEnd], "\r")

		switch {
		case valueStart >= 0 && strings.HasPrefix(line, "--"):
			valueEnd := max(valueStart, lineStart-1)

			if valueEnd > valueStart && text[valueEnd-1] == '\r' {
				valueEnd--
			}

			if valueEnd > valueStart {
				output.WriteString(text[copied:valueStart])
				output.WriteString(filteredText)

				copied = valueEnd
			}

			valueStart = -1
		case inHeaders && line == "":
			valueStart, inHeaders = lineEnd+1, false
		}

		if valueStart < 0 && strings.Contains(line, "form-data") {
			inHeaders = namesSecretField(line)
		}

		lineStart = lineEnd + 1
	}

	if valueStart >= 0 && valueStart < len(text) {
		output.WriteString(text[copied:valueStart])
		output.WriteString(filteredText)

		copied = len(text) // cut short
	}

	if copied == 0 {
		return text
	}

	return output.String() + text[copied:]
}

// namesSecretField reports whether a Content-Disposition line names a field
// that holds a secret: name="password" (not filename=).
func namesSecretField(line string) bool {
	for position := 0; ; {
		found := strings.Index(line[position:], "name=")

		if found < 0 {
			return false
		}

		found += position
		position = found + len("name=")

		if found == 0 || strings.IndexByte("; \t", line[found-1]) < 0 {
			continue
		}

		name := line[position:]

		if strings.HasPrefix(name, `"`) {
			name, _, _ = strings.Cut(name[1:], `"`)
		} else if end := strings.IndexAny(name, "; \t"); end >= 0 {
			name = name[:end]
		}

		return isSensitiveKey(name)
	}
}

// maxQuotedKey bounds the search for the end of a 'key'.
const maxQuotedKey = 64

// quotedEnd returns the index after the string that starts at open, quoted
// by the character there, or -1 when the text ends inside it.
func quotedEnd(text string, open int) int {
	for index := open + 1; index < len(text); index++ {
		switch text[index] {
		case '\\':
			index++
		case text[open]:
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

// collectionEnd returns the index after the array or object that starts at
// start, or the end of the text for one cut short: one pass with a depth
// counter that skips strings, quoted, or escaped at any depth ("\\\"a]\\\"").
func collectionEnd(text string, start int) int {
	depth := 0

	for index := start; index < len(text); {
		switch text[index] {
		case '[', '{':
			depth++
		case ']', '}':
			if depth--; depth == 0 {
				return index + 1
			}
		case '"', '\'':
			end := quotedEnd(text, index)

			if end < 0 {
				return len(text)
			}

			index = end

			continue
		case '\\':
			if escape := escapedQuote(text, index); escape != "" {
				index = escapedEnd(text, index+len(escape), escape)

				continue
			}

			for index < len(text) && text[index] == '\\' {
				index++
			}

			continue
		}

		index++
	}

	return len(text)
}

// maxEscapes bounds the backslashes of an escaped quote: JSON in JSON five
// levels deep has 31.
const maxEscapes = 64

// escapedQuote returns the backslashes and the quote at index (\" in JSON
// escaped once, \\\" twice), or "" when there is none.
func escapedQuote(text string, index int) string {
	end := index

	for end < len(text) && end-index <= maxEscapes && text[end] == '\\' {
		end++
	}

	if end == index || end == len(text) || text[end] != '"' || index > 0 && text[index-1] == '\\' {
		return ""
	}

	return text[index : end+1]
}

// escapedEnd returns the index after the closing escape of a string escaped
// as escape whose text starts at from, or the end of the text for one cut
// short; a quote escaped deeper inside it ends nothing.
func escapedEnd(text string, from int, escape string) int {
	for index := from; ; {
		found := strings.Index(text[index:], escape)

		if found < 0 {
			return len(text)
		}

		found += index

		if text[found-1] != '\\' {
			return found + len(escape)
		}

		index = found + len(escape)
	}
}

func skipJSONSpace(text string, index int) int {
	for index < len(text) && strings.IndexByte(" \t\r\n", text[index]) >= 0 {
		index++
	}

	return index
}

// scrubKeyValues replaces the value of every key=value, key: value and
// key => value pair in free text whose key names a secret: logfmt, env dumps,
// connection strings, header lines, escaped JSON and the reprs of Python,
// Ruby, PHP, Java, Go and Node. A key is a word (the last one before the
// separator: "User Id=sa"), 'quoted', "quoted", \"escaped\" or [bracketed].
// After "key: " with a space the key must open a line or follow one of
// "{([,;:" and an unquoted value must look like a token (looksLikeToken), so
// prose such as "Invalid token: expired" or "jwt: token is expired" stays;
// in code (scrubSourceText) only a quoted value goes. One pass: each
// separator looks back at most maxQuotedKey bytes, the ends of values and
// lines are searched from positions that only grow (nextIndex), and the
// scan goes on after every value it replaced.
func scrubKeyValues(text string, code bool) string {
	var output strings.Builder

	scan := pairScan{
		text: text, code: code,
		lineEnds: nextIndex{chars: "\r\n", at: -1}, valueEnds: nextIndex{chars: valueDelimiters, at: -1},
		apostrophes: nextIndex{chars: "'", at: -1}, quotes: nextIndex{chars: `"`, at: -1}, equalSigns: nextIndex{chars: "=", at: -1},
	}
	copied := 0

	for position := 0; position < len(text); {
		offset := strings.IndexAny(text[position:], "=:")

		if offset < 0 {
			break
		}

		separator := position + offset
		next, valueStart, valueEnd, replacement := scan.secretPairAt(separator)
		position = next

		if valueEnd <= valueStart || replacement == text[valueStart:valueEnd] {
			continue
		}

		output.WriteString(text[copied:valueStart])
		output.WriteString(replacement)

		copied = valueEnd
	}

	if copied == 0 {
		return text
	}

	return output.String() + text[copied:]
}

// valueDelimiters end an unquoted value.
const valueDelimiters = " \t\r\n,;&)}]>'\"`"

// pairScan is one scan of scrubKeyValues.
type pairScan struct {
	text                                                 string
	code                                                 bool
	lineEnds, valueEnds, apostrophes, quotes, equalSigns nextIndex
}

// nextIndex finds the next of its chars at or after a position that only
// grows between calls, so the text is searched once in all.
type nextIndex struct {
	chars string
	at    int // the last found; -1 before the first search
}

func (next *nextIndex) from(text string, start int) int {
	if next.at < start {
		next.at = len(text)

		if found := strings.IndexAny(text[start:], next.chars); found >= 0 {
			next.at = start + found
		}
	}

	return next.at
}

// maxPairSpaces bounds the spaces around a pair's separator.
const maxPairSpaces = 8

// pair is what secretPairAt learned before the value: the key, its quote,
// the separator and the field quote a quoted header line ends at.
type pair struct {
	key, keyQuote string
	colon, spaced bool
	// quotedOnly leaves a literal after a JSON or Python "key": to
	// scrubQuotedPairs; tokenOnly keeps an unquoted word after "key: ".
	quotedOnly, tokenOnly bool
	fieldQuote            byte
}

// secretPairAt reads the pair whose separator ('=', "=>" or ':') is at
// separator. It returns where the scan goes on and, when the key names a
// secret, the value's bounds and what replaces it (valueEnd <= valueStart
// for no pair).
func (scan *pairScan) secretPairAt(separator int) (next, valueStart, valueEnd int, replacement string) {
	text := scan.text
	next = separator + 1
	colon := text[separator] == ':'
	separatorEnd := separator + 1

	switch {
	case colon && strings.HasPrefix(text[separatorEnd:], ":"), !colon && strings.HasPrefix(text[separatorEnd:], "="):
		return separator + 2, 0, 0, "" // "::" or "=="
	case colon && strings.HasPrefix(text[separatorEnd:], "//"):
		return separator + 3, 0, 0, "" // a URL's scheme
	case !colon && strings.HasPrefix(text[separatorEnd:], ">"):
		separatorEnd++
	}

	arrow := separatorEnd > separator+1
	keyEnd := skipSpacesBack(text, separator)
	valueStart = skipSpaces(text, separatorEnd)
	spacedBefore, spacedAfter := keyEnd < separator, valueStart > separatorEnd

	if keyEnd == 0 || !colon && !arrow && !spacedBefore && spacedAfter {
		return next, 0, 0, "" // "password= next"
	}

	key, keyStart, quote := keyBefore(text, keyEnd)

	if key == "" {
		return next, 0, 0, ""
	}

	quotedValue := valueStart < len(text) && strings.IndexByte("'\"`", text[valueStart]) >= 0

	if colon && quote == "" && (spacedBefore || spacedAfter) && !quotedValue && !opensField(text, keyStart) {
		return next, 0, 0, "" // prose: "Invalid token: expired"
	}

	if colon && quote == "" && !spacedBefore && !spacedAfter && isFileName(key) && lineNumberAt(text, valueStart) {
		return next, 0, 0, "" // a stack frame: "token.ts:42:13"
	}

	if !isSensitiveKey(key) {
		return next, 0, 0, ""
	}

	current := pair{
		key: key, keyQuote: quote, colon: colon, spaced: quote == "" && (spacedBefore || spacedAfter),
		quotedOnly: colon && (strings.HasSuffix(quote, `"`) || quote == "'"),
		tokenOnly:  colon && quote == "" && (spacedBefore || spacedAfter),
	}

	// A header in quotes ends at its quote: curl -H 'Authorization: Token t'.
	if quote == "" && keyStart > 0 && (text[keyStart-1] == '\'' || text[keyStart-1] == '"') {
		current.fieldQuote = text[keyStart-1]
	}

	valueEnd, replacement = scan.secretValue(valueStart, current)

	if valueEnd > valueStart {
		next = valueEnd
	}

	return next, valueStart, valueEnd, replacement
}

// secretValue finds the end of the value at start and what replaces it: a
// quoted value keeps its quotes, escaped at any depth (and a Python b
// prefix), an array or object after a quoted key becomes a string in the
// key's quotes, an Authorization header line keeps its scheme, a Cookie
// header line every harmless cookie; a header line ends at its field quote
// when the header is quoted. An unquoted value stays when it is a literal
// (null, true), a short number under a name that holds no numbers
// (numericSecretWords: "session: 401"), a word after "key: " (tokenOnly), a
// word followed by prose after a spaced separator ("token: 3 retries left"),
// or anything in code. A value the scrubbing of URLs filtered stays as it is.
func (scan *pairScan) secretValue(start int, current pair) (end int, replacement string) {
	text := scan.text

	if start >= len(text) {
		return start, ""
	}

	for _, marker := range []string{filteredText, filteredParameter} {
		if markerEnd := start + len(marker); strings.HasPrefix(text[start:], marker) {
			// Filtered already; in a URL, the rest of the URL follows.
			if end = scan.valueEnds.from(text, markerEnd); end == markerEnd || marker == filteredParameter && strings.IndexByte("/?#", text[markerEnd]) >= 0 {
				return start, ""
			}

			return end, filteredText
		}
	}

	prefix, quoteStart := "", start

	if (text[start] == 'b' || text[start] == 'B') && start+1 < len(text) && (text[start+1] == '\'' || text[start+1] == '"') {
		prefix, quoteStart = text[start:start+1], start+1
	}

	switch quote := text[quoteStart]; {
	case quote == '\\' && escapedQuote(text, quoteStart) != "":
		escape := escapedQuote(text, quoteStart)

		return escapedEnd(text, quoteStart+len(escape), escape), prefix + escape + filteredText + escape
	case quote == '"' || quote == '\'' || quote == '`' && !scan.code:
		if end = quotedEnd(text, quoteStart); end < 0 {
			end = len(text) // cut short
		}

		return end, prefix + string(quote) + filteredText + string(quote)
	case current.keyQuote != "" && (quote == '[' || quote == '{'):
		return collectionEnd(text, start), current.keyQuote + filteredText + current.keyQuote
	case scan.code || current.quotedOnly || strings.IndexByte("{[(\r\n", quote) >= 0:
		return start, ""
	}

	if current.colon && (isCookieKey(current.key) || isAuthorizationKey(current.key)) {
		return scan.headerValue(start, current)
	}

	end = scan.valueEnds.from(text, start)
	value := text[start:end]

	switch {
	case value == "" || literalWords[strings.ToLower(value)]:
		return start, ""
	case len(value) < minNumericSecret && strings.Trim(value, "0123456789") == "" && !namesNumericSecret(current.key):
		return start, ""
	case current.tokenOnly && !looksLikeToken(value):
		return start, ""
	case current.spaced && proseFollows(text, end):
		return start, ""
	}

	return end, filteredText
}

// headerValue is secretValue for an Authorization or Cookie header line,
// which ends with its line or at its field quote.
func (scan *pairScan) headerValue(start int, current pair) (end int, replacement string) {
	text := scan.text
	end = scan.lineEnds.from(text, start)

	switch current.fieldQuote {
	case '\'':
		end = min(end, scan.apostrophes.from(text, start))
	case '"':
		end = min(end, scan.quotes.from(text, start))
	}

	value := text[start:end]

	if isCookieKey(current.key) {
		if current.tokenOnly && scan.equalSigns.from(text, start) >= end {
			return start, "" // "cookies: 0" is no Cookie header
		}

		return end, filteredCookieText(value)
	}

	scheme, credential, hasScheme := strings.Cut(value, " ")

	if !hasScheme || !isSchemeName(scheme) {
		scheme, credential = "", value
	}

	if credential = strings.TrimSpace(credential); credential == "" || current.tokenOnly && !isToken("", credential) {
		return start, ""
	}

	if scheme != "" {
		return end, scheme + " " + filteredText
	}

	return end, filteredText
}

// literalWords are values that hold no secret.
var literalWords = wordSet("null", "none", "nil", "undefined", "true", "false")

// minNumericSecret: a number shorter than this is a count or a status code
// unless its name holds numbers (namesNumericSecret).
const minNumericSecret = 6

// namesNumericSecret reports whether key names a secret that may be a short
// number: a PIN, a one-time code, a password.
func namesNumericSecret(key string) bool {
	numeric := false

	forEachWord(key, func(word string) {
		numeric = numeric || numericSecretWords[word]
	})

	return numeric
}

// looksLikeToken reports whether a value after "key: " is a credential
// rather than a word: it has a digit or a base64 sign, a capital inside, or
// is longer than words are. Unlike isToken, lowercase words joined by '-' or
// '.' are names of any length ("env-key", "user.updated").
func looksLikeToken(value string) bool {
	if strings.ContainsAny(value, "-.") && strings.Trim(value, "abcdefghijklmnopqrstuvwxyz-.") == "" {
		return false
	}

	return len(value) >= 16 || strings.ContainsAny(value, "0123456789=+/_~") || strings.ToLower(value[1:]) != value[1:]
}

// proseFollows reports whether a word that is not the key of the next pair
// follows the value that ends at end: "token: 3 retries left".
func proseFollows(text string, end int) bool {
	wordStart := skipSpaces(text, end)

	if wordStart == end || wordStart == len(text) || !unicode.IsLetter(rune(text[wordStart])) && text[wordStart] < utf8.RuneSelf {
		return false
	}

	wordEnd := wordStart

	for wordEnd < len(text) && wordEnd-wordStart <= maxQuotedKey && isKeyByte(text[wordEnd]) {
		wordEnd++
	}

	after := skipSpaces(text, wordEnd)

	if after < len(text) && (text[after] == '=' || text[after] == ':' && !strings.HasPrefix(text[after:], "::")) {
		return false // the next pair: "token: a1 user: root"
	}

	return true
}

// isFileName reports whether key is a file's name: a name and an extension
// of up to 5 letters or digits, at least one a letter ("token.ts").
func isFileName(key string) bool {
	dot := strings.LastIndexByte(key, '.')

	if dot <= 0 || len(key)-dot-1 < 1 || len(key)-dot-1 > 5 {
		return false
	}

	extension := key[dot+1:]

	return strings.IndexFunc(extension, unicode.IsLetter) >= 0 && strings.Trim(strings.ToLower(extension), "abcdefghijklmnopqrstuvwxyz0123456789") == ""
}

// lineNumberAt reports whether a line number, up to 6 digits not followed
// by a letter, starts at index: the "42" of "token.ts:42:13".
func lineNumberAt(text string, index int) bool {
	end := index

	for end < len(text) && end-index <= 6 && isDigit(text[end]) {
		end++
	}

	return end > index && end-index <= 6 && (end == len(text) || !isKeyByte(text[end]))
}

// keyBefore reads the key that ends at end: 'quoted', "quoted", \"escaped\"
// at any depth (quote is its quote with the backslashes before it),
// [bracketed] or a word. It looks back at most maxQuotedKey bytes; key is
// empty when there is none.
func keyBefore(text string, end int) (key string, start int, quote string) {
	last := text[end-1]
	from := max(0, end-1-maxQuotedKey)

	switch last {
	case '\'', '"', '`', ']':
		opening, closing := string(last), end-1

		switch last {
		case ']':
			opening = "["
		case '"':
			for closing > 0 && end-closing <= maxEscapes && text[closing-1] == '\\' {
				closing-- // \"key\" of JSON escaped in text
			}

			opening = text[closing:end]
			from = max(0, closing-maxQuotedKey-len(opening))
		}

		open := strings.LastIndex(text[from:closing], opening)

		if open < 0 {
			return "", 0, ""
		}

		open += from

		if last == ']' {
			return text[open+1 : closing], open, ""
		}

		return text[open+len(opening) : closing], open, opening
	}

	start = end

	for start > from && isKeyByte(text[start-1]) {
		start--
	}

	if start > 0 && isKeyByte(text[start-1]) {
		return "", 0, "" // longer than any key
	}

	return text[start:end], start, ""
}

func isKeyByte(char byte) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' ||
		char == '_' || char == '-' || char == '.' || char >= 0x80
}

// opensField reports whether the key at start opens a line, a quoted string
// or follows one of "{([,;:", as the fields of a header block, a struct, a
// dict or a command line's header do.
func opensField(text string, start int) bool {
	before := skipSpacesBack(text, start)

	return before == 0 || strings.IndexByte("\r\n{([,;:'\"", text[before-1]) >= 0
}

func skipSpaces(text string, index int) int {
	for limit := index + maxPairSpaces; index < len(text) && index < limit && (text[index] == ' ' || text[index] == '\t'); {
		index++
	}

	return index
}

func skipSpacesBack(text string, index int) int {
	for limit := index - maxPairSpaces; index > 0 && index > limit && (text[index-1] == ' ' || text[index-1] == '\t'); {
		index--
	}

	return index
}

// isAuthorizationKey reports whether key is an Authorization header's name.
func isAuthorizationKey(key string) bool {
	last := ""

	forEachWord(key, func(word string) {
		last = word
	})

	return last == "authorization"
}

// isSchemeName reports whether word can be an authorization scheme ("Token", "Bearer").
func isSchemeName(word string) bool {
	if word == "" || len(word) > 20 {
		return false
	}

	for index := 0; index < len(word); index++ {
		if char := word[index] | 0x20; char < 'a' || char > 'z' {
			return false
		}
	}

	return true
}

// Card numbers have 13 to 19 digits; written in groups, each has 3 to 6.
const (
	minCardDigits = 13
	maxCardDigits = 19
	minCardGroup  = 3
	maxCardGroup  = 6
)

// hasCardDigits reports whether text has minCardDigits digits in a row,
// single spaces or dashes between them allowed.
func hasCardDigits[Text ~string | ~[]byte](text Text) bool {
	digits := 0

	for index := 0; index < len(text); index++ {
		switch char := text[index]; {
		case isDigit(char):
			if digits++; digits >= minCardDigits {
				return true
			}
		case (char == ' ' || char == '-') && index > 0 && isDigit(text[index-1]):
		default:
			digits = 0
		}
	}

	return false
}

func isDigit(char byte) bool {
	return char >= '0' && char <= '9'
}

// isCardNumber reports whether digits can be a card number: a length its
// first digit's networks use (Visa 4, Mastercard 2 and 5, Amex, Diners and
// JCB 3, Discover, UnionPay and Maestro 6 and 5) and a valid Luhn check digit.
func isCardNumber[Text ~string | ~[]byte](digits Text) bool {
	length := len(digits)

	if length < minCardDigits || length > maxCardDigits {
		return false
	}

	switch digits[0] {
	case '4':
		if length != 13 && length != 16 && length != 19 {
			return false
		}
	case '2':
		if length != 16 {
			return false
		}
	case '3':
		if length < 14 {
			return false
		}
	case '5', '6':
		if length < 16 {
			return false
		}
	default:
		return false
	}

	sum := 0

	for index := range length {
		if !isDigit(digits[index]) {
			return false
		}

		digit := int(digits[index] - '0')

		if (length-index)%2 == 0 {
			if digit *= 2; digit > 9 {
				digit -= 9
			}
		}

		sum += digit
	}

	return sum%10 == 0
}

// scrubCardNumbers replaces card numbers (isCardNumber) in text: a run of
// digits, or of groups of minCardGroup to maxCardGroup digits split by single
// spaces or dashes. Digits that touch a letter, or a decimal's point, belong
// to something else.
func scrubCardNumbers(text string) string {
	var (
		output strings.Builder
		groups [][2]int // the digit groups of a run: start and end
	)

	copied := 0

	for index := 0; index < len(text); {
		if !isDigit(text[index]) {
			index++

			continue
		}

		runStart := index
		groups = groups[:0]

		for {
			groupStart := index

			for index < len(text) && isDigit(text[index]) {
				index++
			}

			groups = append(groups, [2]int{groupStart, index})

			if index+1 >= len(text) || text[index] != ' ' && text[index] != '-' || !isDigit(text[index+1]) {
				break
			}

			index++
		}

		first, last := 0, len(groups)

		if !numberBoundary(text, runStart-1, runStart-2) {
			first++
		}

		if !numberBoundary(text, index, index+1) {
			last--
		}

		for group := first; group < last; {
			count := cardGroups(text, groups[group:last])

			if count == 0 {
				group++

				continue
			}

			output.WriteString(text[copied:groups[group][0]])
			output.WriteString(filteredText)

			copied = groups[group+count-1][1]
			group += count
		}
	}

	if copied == 0 {
		return text
	}

	return output.String() + text[copied:]
}

// numberBoundary reports whether the byte at next, beside a run of digits,
// ends the number: it is no letter, digit or '_', nor a decimal point (a '.'
// with a digit at beyond).
func numberBoundary(text string, next, beyond int) bool {
	if next < 0 || next >= len(text) {
		return true
	}

	char := text[next]

	if char == '.' {
		return beyond < 0 || beyond >= len(text) || !isDigit(text[beyond])
	}

	return char == '-' || !isKeyByte(char)
}

// cardGroups returns how many of groups, from the first, are a card number:
// a group of its own, or the most groups of minCardGroup to maxCardGroup
// digits that are; 0 when none are.
func cardGroups(text string, groups [][2]int) int {
	if size := groups[0][1] - groups[0][0]; size >= minCardDigits {
		if isCardNumber(text[groups[0][0]:groups[0][1]]) {
			return 1
		}

		return 0
	}

	var digits [maxCardDigits]byte

	found, length := 0, 0

	for count, group := range groups {
		size := group[1] - group[0]

		if size < minCardGroup || size > maxCardGroup || length+size > maxCardDigits {
			break
		}

		length += copy(digits[length:], text[group[0]:group[1]])

		if isCardNumber(digits[:length]) {
			found = count + 1
		}
	}

	return found
}

// pemBegin opens a PEM block; one whose label names a private key holds one.
const pemBegin = "-----BEGIN "

// maxPEMLabel bounds the search for the end of a BEGIN line's label.
const maxPEMLabel = 64

// scrubPrivateKeys replaces the body of every PEM private key in text,
// keeping its BEGIN and END lines and the line breaks around it, written
// or escaped; a key without END is filtered to the end of the text.
func scrubPrivateKeys(text string) string {
	var output strings.Builder

	copied := 0

	for position := 0; position < len(text); {
		begin := strings.Index(text[position:], pemBegin)

		if begin < 0 {
			break
		}

		labelStart := position + begin + len(pemBegin)
		labelEnd := strings.Index(text[labelStart:min(len(text), labelStart+maxPEMLabel)], "-----")

		if labelEnd < 0 {
			position = labelStart

			continue
		}

		bodyStart := labelStart + labelEnd + len("-----")
		position = bodyStart

		if !strings.Contains(text[labelStart:labelStart+labelEnd], "PRIVATE KEY") {
			continue
		}

		bodyEnd := len(text)

		if end := strings.Index(text[bodyStart:], "-----END "); end >= 0 {
			bodyEnd = bodyStart + end
		}

		position = bodyEnd

		for bodyStart < bodyEnd {
			width := lineBreak(text[bodyStart:bodyEnd], strings.HasPrefix, 0)

			if width == 0 {
				break
			}

			bodyStart += width
		}

		for bodyStart < bodyEnd {
			width := lineBreak(text[bodyStart:bodyEnd], strings.HasSuffix, bodyEnd-bodyStart-1)

			if width == 0 {
				break
			}

			bodyEnd -= width
		}

		if bodyStart == bodyEnd {
			continue
		}

		output.WriteString(text[copied:bodyStart])
		output.WriteString(filteredText)

		copied = bodyEnd
	}

	if copied == 0 {
		return text
	}

	return output.String() + text[copied:]
}

// lineBreak returns the length of the space or line break, written or
// escaped ("\\n"), at one edge of text: the byte at edge, or an escape that
// ends (strings.HasPrefix or HasSuffix) finds; 0 for none.
func lineBreak(text string, ends func(text, part string) bool, edge int) int {
	switch {
	case strings.IndexByte(" \t\r\n", text[edge]) >= 0:
		return 1
	case ends(text, `\n`) || ends(text, `\r`):
		return 2
	}

	return 0
}

// linkInText finds absolute URLs, and relative ones with a query, a fragment
// or a path parameter ("GET /reset?token=… 200", "/cart;jsessionid=…"),
// inside longer text.
var linkInText = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+|/[^\s"'<>?#;]*[?#;][^\s"'<>]+`)

// filteredParameter replaces a value in a query or userinfo.
var filteredParameter = url.QueryEscape(filteredText)

// scrubLink scrubs one URL or bare query string; the rest of it stays byte for byte.
func scrubLink(text string) string {
	text = withoutPasswords(text)

	// A URL, absolute or relative, has no "=" before its first '/', ':', '?' or '#'.
	equals := strings.IndexByte(text, '=')

	if delimiter := strings.IndexAny(text, "/:?#"); equals >= 0 && (delimiter < 0 || equals < delimiter) {
		// Such as request.query_string, sent raw; one pair alone is no query
		// for the names of credentialParameters ("code=ERR_NETWORK").
		return scrubQuery(text, strings.ContainsAny(text, "&;"))
	}

	start := strings.IndexAny(text, "?#")

	if start < 0 {
		return scrubPathParameters(text)
	}

	return scrubPathParameters(text[:start]) + text[start:start+1] + scrubQuery(text[start+1:], true)
}

// scrubPathParameters replaces the values of sensitive parameters in the
// path of a URL: "/cart;jsessionid=…". The authority keeps its own pairs
// ("jdbc:sqlserver://db:1433;password=…" is left to scrubKeyValues).
func scrubPathParameters(link string) string {
	path := 0

	if scheme := strings.Index(link, "://"); scheme >= 0 {
		if path = strings.IndexByte(link[scheme+len("://"):], '/'); path < 0 {
			return link
		}

		path += scheme + len("://")
	}

	if !strings.Contains(link[path:], ";") {
		return link
	}

	segments := strings.Split(link[path:], "/")

	for index, segment := range segments {
		if strings.Contains(segment, ";") {
			name, parameters, _ := strings.Cut(segment, ";")
			segments[index] = name + ";" + scrubQuery(parameters, true)
		}
	}

	return link[:path] + strings.Join(segments, "/")
}

// withoutPasswords replaces the password in every "scheme://user:password@host", keeping the user.
func withoutPasswords(text string) string {
	var output strings.Builder

	copied := 0

	for from := 0; ; {
		scheme := strings.Index(text[from:], "://")

		if scheme < 0 {
			break
		}

		from += scheme + len("://")

		if colon, at, found := userinfoPassword(text[from:]); found {
			output.WriteString(text[copied : from+colon+1])
			output.WriteString(filteredParameter)

			copied, from = from+at, from+at
		}
	}

	if copied == 0 {
		return text
	}

	return output.String() + text[copied:]
}

// userinfoPassword finds the ':' and the '@' around the password of the userinfo
// that starts authority. The password itself may hold '/', '?', '#' or '@', so
// it ends at the last '@' a host follows (hostFollows) before the next URL,
// or, with none, at the last '@'.
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

	if next := strings.Index(rest, "://"); next >= 0 {
		rest = rest[:next]
	}

	last, beforeHost := -1, -1

	for index := strings.IndexByte(rest, '@'); index >= 0; {
		if last = index; hostFollows(rest[index+1:]) {
			beforeHost = index
		}

		next := strings.IndexByte(rest[index+1:], '@')

		if next < 0 {
			break
		}

		index += 1 + next
	}

	if beforeHost < 0 {
		beforeHost = last
	}

	if beforeHost < 0 {
		return 0, 0, false
	}

	return colon, colon + 1 + beforeHost, true
}

// maxIPv6Host bounds the search for the end of an [IPv6] host.
const maxIPv6Host = 64

// hostFollows reports whether text starts with a host: a name or an
// address, an optional port, then the end, '/', '?', '#' or ',' (a list of
// hosts); or is empty, a
// URL cut where its host starts. It reads the host only, so it stops at the
// next '@'.
func hostFollows(text string) bool {
	index := 0

	if text == "" {
		return true
	}

	if strings.HasPrefix(text, "[") {
		end := strings.IndexByte(text[:min(len(text), maxIPv6Host)], ']')

		if end < 0 {
			return false
		}

		index = end + 1
	} else {
		for index < len(text) && isKeyByte(text[index]) {
			index++
		}

		if index == 0 {
			return false
		}
	}

	if index < len(text) && text[index] == ':' {
		port := index + 1

		for index = port; index < len(text) && isDigit(text[index]); {
			index++
		}

		if index == port {
			return false
		}
	}

	return index == len(text) || strings.IndexByte("/?#,", text[index]) >= 0
}

// scrubQuery replaces the values of sensitive parameters, of
// credentialParameters too when withCredentialNames; everything else stays
// as it came. A '?' or '#' starts more pairs: "next=/reset?token=…".
func scrubQuery(raw string, withCredentialNames bool) string {
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

		if key, _, hasValue := strings.Cut(pair, "="); hasValue && sensitiveParameter(key, withCredentialNames) {
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

func sensitiveParameter(key string, withCredentialNames bool) bool {
	if unescaped, err := url.QueryUnescape(key); err == nil {
		key = unescaped
	}

	return isSensitiveKey(key) || withCredentialNames && credentialParameters[strings.ToLower(key)]
}

// credentialParameters name a credential only as a URL parameter: an OAuth
// authorization code, an API key ("?key=", as Google's APIs take it).
var credentialParameters = wordSet("code", "key")
