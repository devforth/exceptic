package ingest

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"
)

func TestScrubJSONRemovesCredentials(t *testing.T) {
	payload := `{
	  "event_id": "0123456789abcdef0123456789abcdef",
	  "message": "login failed for Bearer eyJhbGciOiJIUzI1NiJ9.payload.signature",
	  "request": {
	    "url": "https://app.example.com/orders?id=7&access_token=query-token-value",
	    "query_string": "id=7&api_key=query-key-value",
	    "headers": {"Authorization": "Bearer header-token-value", "Cookie": "sid=cookie-value", "X-Api-Key": "api-key-value", "User-Agent": "curl/8"},
	    "cookies": {"sid": "cookie-object-value"},
	    "data": {"email": "user@example.com", "password": "body-password-value"}
	  },
	  "exception": {"values": [{"type": "Error", "value": "boom", "stacktrace": {"frames": [
	    {"function": "login", "lineno": 3, "vars": {"sessionToken": "frame-var-value", "count": 12345678901234567890}}
	  ]}}]},
	  "breadcrumbs": {"values": [{"category": "http", "data": {"url": "https://api.example.com/v1?token=breadcrumb-token-value", "method": "GET"}}]},
	  "extra": {"headers": [["authorization", "pair-token-value"], ["accept", "*/*"]]},
	  "contexts": {"trace": {"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736", "span_id": "00f067aa0ba902b7"}}
	}`

	scrubbed := string(scrubJSON([]byte(payload)))

	for _, secret := range []string{
		"query-token-value", "query-key-value", "header-token-value", "cookie-value", "cookie-object-value",
		"api-key-value", "body-password-value", "frame-var-value", "breadcrumb-token-value", "pair-token-value", "eyJhbGci",
	} {
		if strings.Contains(scrubbed, secret) {
			t.Errorf("%q survived:\n%s", secret, scrubbed)
		}
	}

	// What is needed to debug stays, numbers exactly as sent.
	for _, kept := range []string{"curl/8", "user@example.com", "id=7", `"method":"GET"`, "4bf92f3577b34da6a3ce929d0e0e4736", "12345678901234567890", `"function":"login"`, "*/*"} {
		if !strings.Contains(scrubbed, kept) {
			t.Errorf("%q was removed:\n%s", kept, scrubbed)
		}
	}

	if !json.Valid([]byte(scrubbed)) {
		t.Fatal("scrubbed payload is not valid JSON")
	}
}

// URLs with a password and no "=" still go through scrubText.
func TestScrubJSONRemovesURLPasswords(t *testing.T) {
	payload := `{"request":{"url":"https://app.example.com/cb#access_token=fragment-token-value"},"message":"connect postgres://admin:userinfo-password-value@db/app failed","breadcrumbs":[{"category":"navigation","data":{"from":"/","to":"https://admin:breadcrumb-password-value@host.example.com/"}}]}`

	scrubbed := string(scrubJSON([]byte(payload)))

	for _, secret := range []string{"fragment-token-value", "userinfo-password-value", "breadcrumb-password-value"} {
		if strings.Contains(scrubbed, secret) {
			t.Errorf("%q survived:\n%s", secret, scrubbed)
		}
	}

	if !strings.Contains(scrubbed, "postgres://admin:") {
		t.Errorf("username was removed:\n%s", scrubbed)
	}
}

// SDKs send request.query_string raw, so values may hold ':' and '/'.
func TestScrubJSONRemovesRawQueryString(t *testing.T) {
	payload := `{"request":{"url":"https://app.example.com/s?from=2024-01-01T10:00:00Z&token=url-token-value","query_string":"from=2024-01-01T10:00:00Z&token=query-token-value"}}`

	scrubbed := string(scrubJSON([]byte(payload)))

	for _, secret := range []string{"url-token-value", "query-token-value"} {
		if strings.Contains(scrubbed, secret) {
			t.Errorf("%q survived:\n%s", secret, scrubbed)
		}
	}
}

func TestScrubJSONKeepsLogAttributeShape(t *testing.T) {
	payload := `{"items": [{"body": "request", "attributes": {"http.request.header.cookie": {"value": "sid=secret", "type": "string"}, "gen_ai.usage.input_tokens": {"value": 42, "type": "integer"}}}]}`

	scrubbed := string(scrubJSON([]byte(payload)))

	if strings.Contains(scrubbed, "sid=secret") || !strings.Contains(scrubbed, `"http.request.header.cookie":{"type":"string","value":"[Filtered]"}`) {
		t.Errorf("cookie attribute:\n%s", scrubbed)
	}

	// Counts of tokens are not tokens.
	if !strings.Contains(scrubbed, `"value":42`) {
		t.Errorf("token count was removed:\n%s", scrubbed)
	}
}

// Everything under a key that names a secret goes; a log attribute keeps
// only its {value, type} shape, so the log still parses.
func TestScrubJSONFiltersWholeSecretValue(t *testing.T) {
	testCases := map[string]string{
		`{"extra":{"auth":{"value":{"user":7},"token":"sibling-token-value","password":"sibling-password"}}}`: `{"extra":{"auth":{"type":"string","value":"[Filtered]"}}}`,
		`{"state":{"session":{"value":"x","refresh":"sibling-refresh"}}}`:                                     `{"state":{"session":{"type":"string","value":"[Filtered]"}}}`,
		`{"headers":[["authorization",{"value":"a","raw":"Bearer sibling-bearer-0123456789"}]]}`:              `{"headers":[["authorization",{"type":"string","value":"[Filtered]"}]]}`,
		`{"attributes":{"session.id":{"value":"s","type":"string","unit":"none"}}}`:                           `{"attributes":{"session.id":{"type":"string","value":"[Filtered]"}}}`,
	}

	for payload, want := range testCases {
		if got := normalized(scrubJSON([]byte(payload))); got != want {
			t.Errorf("scrubJSON(%s)\ngot  %s\nwant %s", payload, got, want)
		}
	}
}

func TestScrubJSONLeavesInvalidInput(t *testing.T) {
	if got := string(scrubJSON([]byte("not json"))); got != "not json" {
		t.Fatalf("got %q", got)
	}
}

func TestScrubText(t *testing.T) {
	testCases := map[string]string{
		"plain error text": "plain error text",
		"x = a == b":       "x = a == b",
		"/orders?page=2":   "/orders?page=2",
		"call https://x.example.com/a?token=abc done": "call https://x.example.com/a?token=%5BFiltered%5D done",
		"Authorization: Basic dXNlcjpwYXNzd29yZA==":   "Authorization: Basic [Filtered]",
		"retry with bearer opaqueAccessToken":         "retry with bearer [Filtered]",
		"Bearer abc123def":                            "Bearer [Filtered]",
		"Authorization: Basic dXNlcjpwYXNz":           "Authorization: Basic [Filtered]",
		"Basic dXNlcjpwYXNz":                          "Basic [Filtered]",
		"Bearer abcd-efgh-ijkl":                       "Bearer [Filtered]",
		"Bearer abcdefghIjkl":                         "Bearer [Filtered]",
		// Lowercase base64 that decodes to "j'an:,", a user and a password.
		"Basic aidhbjos": "Basic [Filtered]",
		// Words after Basic or Bearer are not tokens.
		"Invalid basic authentication credentials.": "Invalid basic authentication credentials.",
		"Basic configuration incomplete":            "Basic configuration incomplete",
		"Missing bearer authorization.":             "Missing bearer authorization.",
		"id=7&api_key=k1;token=k2&page=2":           "id=7&api_key=%5BFiltered%5D;token=%5BFiltered%5D&page=2",
		"https://app.example.com/a?b=1&token=x%zz":  "https://app.example.com/a?b=1&token=%5BFiltered%5D",
		"https://app.example.com/#section=2":        "https://app.example.com/#section=2",
		// OAuth implicit flows put the tokens in the fragment.
		"https://app.example.com/callback#access_token=at1&refresh_token=rt1&expires_in=3600": "https://app.example.com/callback#access_token=%5BFiltered%5D&refresh_token=%5BFiltered%5D&expires_in=3600",
		"https://app.example.com/#/reset?token=rt1":                                           "https://app.example.com/#/reset?token=%5BFiltered%5D",
		"navigated to https://app.example.com/#access_token=at1&type=bearer":                  "navigated to https://app.example.com/#access_token=%5BFiltered%5D&type=bearer",
		"/callback#access_token=at1":                                                          "/callback#access_token=%5BFiltered%5D",
		// Access-log lines carry relative paths.
		"GET /reset-password?token=SECRET123 200 5ms":   "GET /reset-password?token=%5BFiltered%5D 200 5ms",
		"GET /api/v1/items?page=2&sort=name 200 3ms":    "GET /api/v1/items?page=2&sort=name 200 3ms",
		"GET /login#session=s1 302":                     "GET /login#session=%5BFiltered%5D 302",
		"https://app.example.com/a?b=1 and /c?token=t1": "https://app.example.com/a?b=1 and /c?token=%5BFiltered%5D",
		// The password of userinfo goes, the username stays.
		"https://admin:s3cretpass@db.example.com/x":                     "https://admin:%5BFiltered%5D@db.example.com/x",
		"dial postgres://admin:pass@host/db failed":                     "dial postgres://admin:%5BFiltered%5D@host/db failed",
		"redis://:pass@cache:6379/0":                                    "redis://:%5BFiltered%5D@cache:6379/0",
		"https://user@host.example.com/x and mail user@example.com":     "https://user@host.example.com/x and mail user@example.com",
		"https://admin:p@ss@db.example.com/x?token=t1#access_token=at1": "https://admin:%5BFiltered%5D@db.example.com/x?token=%5BFiltered%5D#access_token=%5BFiltered%5D",
		"postgres://user:ab/cd+ef@host:5432/db":                         "postgres://user:%5BFiltered%5D@host:5432/db",
		"connect postgres://user:ab/cd+ef@host:5432/db failed":          "connect postgres://user:%5BFiltered%5D@host:5432/db failed",
		"https://admin:p#ss@db.example.com/x":                           "https://admin:%5BFiltered%5D@db.example.com/x",
		"https://admin:p?ss@db.example.com/x":                           "https://admin:%5BFiltered%5D@db.example.com/x",
		"redis://cache:6379,postgres://u:pw@h/db":                       "redis://cache:6379,postgres://u:%5BFiltered%5D@h/db",
		"connect redis://cache:6379,postgres://u:pw@h/db failed":        "connect redis://cache:6379,postgres://u:%5BFiltered%5D@h/db failed",
		"https://h.example.com/x?redirect=https://admin:pw@h2/y":        "https://h.example.com/x?redirect=https://admin:%5BFiltered%5D@h2/y",
		"https://cdn.example.com/npm/@sentry/browser@7.0.0/index.js":    "https://cdn.example.com/npm/@sentry/browser@7.0.0/index.js",
		"https://cdn.example.com:443/npm/@sentry/browser@7.0.0/x.js":    "https://cdn.example.com:443/npm/@sentry/browser@7.0.0/x.js",
		"https://user@host.example.com:8080/a@b?t=10:00@y":              "https://user@host.example.com:8080/a@b?t=10:00@y",
		// Nested and bare queries, values with ':' or '/' included.
		"GET /login?next=/reset?token=abc 302":              "GET /login?next=/reset?token=%5BFiltered%5D 302",
		"from=2024-01-01T10:00:00Z&token=abc":               "from=2024-01-01T10:00:00Z&token=%5BFiltered%5D",
		"next=/dashboard&token=abc":                         "next=/dashboard&token=%5BFiltered%5D",
		"redirect=https://app.example.com&access_token=abc": "redirect=https://app.example.com&access_token=%5BFiltered%5D",
		"redirect=https://admin:pw@app.example.com/&page=2": "redirect=https://admin:%5BFiltered%5D@app.example.com/&page=2",
		"ratio=1:2": "ratio=1:2",
	}

	for input, want := range testCases {
		if got := scrubText(input); got != want {
			t.Errorf("scrubText(%q) = %q, want %q", input, got, want)
		}
	}
}

// Request bodies, breadcrumb messages and log lines carry JSON as text.
func TestScrubTextJSON(t *testing.T) {
	testCases := map[string]string{
		`{"username":"root","password":"pw1","api_key":"k1"}`:       `{"username":"root","password":"[Filtered]","api_key":"[Filtered]"}`,
		`{"cookie":"sid=abc","token":"t1"}`:                         `{"cookie":"[Filtered]","token":"[Filtered]"}`,
		` {"session":{"id":"s1"},"user":"root"} `:                   `{"session":"[Filtered]","user":"root"}`,
		`[["authorization","Basic dXNlcjpwYXNz"],["accept","*/*"]]`: `[["authorization","[Filtered]"],["accept","*/*"]]`,
		`{"body":"{\"password\":\"pw1\",\"page\":2}"}`:              `{"body":"{\"password\":\"[Filtered]\",\"page\":2}"}`,
		// Inside longer text, or cut by the SDK's length limit.
		`POST /login {"username":"root","password":"pw1"} 401`:      `POST /login {"username":"root","password":"[Filtered]"} 401`,
		`{"username":"root","password":"p\"w1","card_number":"4111`: `{"username":"root","password":"[Filtered]","card_number":"[Filtered]"`,
		`charge failed {"cvv": 123, "amount": 5`:                    `charge failed {"cvv": "[Filtered]", "amount": 5`,
		// Form bodies are bare query strings.
		"username=root&password=pw1": "username=root&password=%5BFiltered%5D",
		// Unchanged: no secret, so the text stays byte for byte.
		"{\n  \"user\": \"root\",\n  \"tokens\": 2\n}": "{\n  \"user\": \"root\",\n  \"tokens\": 2\n}",
		`[1, 2] failed`:                       `[1, 2] failed`,
		`{"a":1} and {"token":2} trailing`:    `{"a":1} and {"token":"[Filtered]"} trailing`,
		`Cannot find module "auth": gone`:     `Cannot find module "auth": gone`,
		`"token": else, "session": {"id": 1}`: `"token": else, "session": {"id": 1}`,
		`{"token": "t1"`:                      `{"token": "[Filtered]"`,
		// Python dict reprs (str(dict)) quote with ' and use " for a string holding one.
		`login failed {'username': 'root', 'password': 'pw1', 'token': 'abc'}`: `login failed {'username': 'root', 'password': '[Filtered]', 'token': '[Filtered]'}`,
		`{'cvv': 123, 'amount': 5}`:                         `{'cvv': '[Filtered]', 'amount': 5}`,
		`{'password': "it's", 'user': 'root'}`:              `{'password': '[Filtered]', 'user': 'root'}`,
		`{'password': 'p\'w', 'next': 1}`:                   `{'password': '[Filtered]', 'next': 1}`,
		`{'note': 'say "hi', 'token': 'abc'}`:               `{'note': 'say "hi', 'token': '[Filtered]'}`,
		`don't retry {'session': 'abc'}`:                    `don't retry {'session': '[Filtered]'}`,
		`{"note": "it's", 'api_key': 'k1'}`:                 `{"note": "it's", 'api_key': '[Filtered]'}`,
		`cut {'username': 'root', 'password': 'pw`:          `cut {'username': 'root', 'password': '[Filtered]'`,
		`can't find 'auth': gone, it's 'token' time`:        `can't find 'auth': gone, it's 'token' time`,
		`{'tokens': None, 'session': {'id': 1}, 'auth': 1}`: `{'tokens': None, 'session': {'id': 1}, 'auth': '[Filtered]'}`,
	}

	for input, want := range testCases {
		if got := scrubText(input); got != want {
			t.Errorf("scrubText(%q)\ngot  %q\nwant %q", input, got, want)
		}
	}
}

// isSensitiveKey checks ASCII keys without the regexp: it must agree with it.
func TestIsSensitiveKeyMatchesTheRegexp(t *testing.T) {
	keys := []string{
		"Authorization", "X-Api-Key", "api_key", "api.key", "api-key", "apikey", "API_KEY", "apiKey", "api__key", "privatekey", "private-key",
		"card_number", "card number", "cardNumber", "tokens", "token", "Tokens", "access_token", "tokens_token", "tokenizer", "max_tokens",
		"passwd", "user_password", "pwd", "secret", "session_id", "csrf", "xsrf_token", "signature", "dsn", "cvv", "ssn", "cookie",
		"credentials", "url", "headers", "user_id", "release", "", "a", "authority", "tokenstoken", "toke", "apikeyz", "privatekeyring",
	}
	alphabet := []byte("apikeytoknsrwdcuhvbmPIKETOS_-. ")
	random := rand.New(rand.NewSource(1))

	for range 200000 {
		key := make([]byte, random.Intn(14))

		for index := range key {
			key[index] = alphabet[random.Intn(len(alphabet))]
		}

		keys = append(keys, string(key))
	}

	for _, key := range keys {
		if got, want := sensitiveASCIIKey(strings.ToLower(key)), sensitiveKey.MatchString(key); got != want {
			t.Fatalf("key %q: %v, the regexp says %v", key, got, want)
		}

		if isSensitiveKey(key) != sensitiveKey.MatchString(key) {
			t.Fatalf("isSensitiveKey(%q) disagrees", key)
		}
	}
}
