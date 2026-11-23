package ingest

import (
	"encoding/json"
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
		`{"cookie":"sid=abc","token":"t1"}`:                         `{"cookie":"sid=[Filtered]","token":"[Filtered]"}`,
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
		`"token": else, "session": {"id": 1}`: `"token": else, "session": "[Filtered]"`,
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
		`{'tokens': None, 'session': {'id': 1}, 'auth': 1}`: `{'tokens': None, 'session': '[Filtered]', 'auth': '[Filtered]'}`,
	}

	for input, want := range testCases {
		if got := scrubText(input); got != want {
			t.Errorf("scrubText(%q)\ngot  %q\nwant %q", input, got, want)
		}
	}
}

// Keys are read as words, so a secret's word inside another word is no secret.
func TestIsSensitiveKey(t *testing.T) {
	secret := []string{
		// Words and their separators: snake, kebab, dots, spaces, camel case, digits.
		"password", "PASSWORD", "user_password", "newPassword", "password_confirmation", "passwords", "passwd", "pass", "pw", "pwd", "DB_PWD",
		"passphrase", "passcode", "secret", "client_secret", "clientSecret", "aws.secret_access_key", "\u017fecret", "\xffpassword", "secrets",
		"token", "access_token", "AccessToken", "accesstoken", "refresh_token", "id_token", "auth_token", "authToken", "X-Auth-Token", "X-CSRF-Token",
		"csrftoken", "xsrf-token", "csrf", "xsrf", "pin_token", "api_tokens",
		"auth", "authorization", "Authorization", "Proxy-Authorization", "HTTP_AUTHORIZATION", "http.request.header.authorization",
		"cookie", "Cookie", "cookies", "set-cookie", "Set-Cookie", "http.request.header.cookie",
		"session", "session_id", "sessionId", "sessionid", "session.id", "sid", "PHPSESSID", "JSESSIONID", "connect.sid", "laravel_session", "ASP.NET_SessionId",
		"api_key", "apiKey", "apikey", "Api.Key", "api-key", "API_KEY", "X-Api-Key", "x-goog-api-key", "X-RapidAPI-Key", "api_keys",
		"private_key", "private-key", "privatekey", "secret_key", "access_key", "AWS_ACCESS_KEY_ID", "encryption_key", "signing_key", "ssh_key",
		"APP_KEY", "STRIPE_KEY", "SENDGRID_KEY", "Ocp-Apim-Subscription-Key", "x-functions-key", "license_key",
		"credential", "credentials", "signature", "X-Hub-Signature-256", "sig", "dsn", "SENTRY_DSN", "ssn", "SSN",
		"card_number", "cardNumber", "card number", "credit_card", "creditCard", "cvv", "cvc", "cvv2", "security_code", "iban",
		"pin", "pin_code", "otp", "totp", "mfa_code", "2fa_code", "verification_code", "two_factor_recovery_codes", "recovery_codes", "jwt",
		"webhook_url", "slackWebhookUrl",
		"пароль", "Пароль", "Passwort", "contraseña", "mot_de_passe", "motDePasse", "senha", "wachtwoord",
	}
	kept := []string{
		"author", "authority", ":authority", "authorName", "oauth_provider", "is_authenticated", "authentication_method",
		"session_count", "session_started_at", "sessionDuration", "password_strength", "password_length", "token_count", "token_expires_at",
		"className", "classname", "adsNetwork", "tokenizer", "tokens", "tokens_used", "tokens_count", "max_tokens", "input_tokens", "gen_ai.usage.input_tokens",
		"key", "keys", "key_id", "public_key", "primary_key", "foreign_key", "cache_key", "idempotency_key", "sort_key", "monkey", "keyboard", "keyCode", "event.key",
		"passenger", "compass", "bypass", "spinner", "pinned", "opinion", "shipping", "design", "signal", "dsnake", "lesson", "tokenize",
		"code", "status_code", "error_code", "country_code", "number", "phone_number", "order_id", "user_id", "url", "headers", "release", "", "a",
		"secretary", "cookbook", "description", "pass_count",
		"SessionNotFound", "PasswordMismatch", "SecretsManagerException", "cookie_consent", "AUTH_DOMAIN", "SESSION_DRIVER", "passe",
	}

	for _, key := range secret {
		if !isSensitiveKey(key) {
			t.Errorf("%q is kept, it names a secret", key)
		}
	}

	for _, key := range kept {
		if isSensitiveKey(key) {
			t.Errorf("%q is filtered, it names no secret", key)
		}
	}
}

// Payloads as SDKs send them: every "leak-" value goes, every "keep-" value stays.
func TestScrubKeyNamesInPayloads(t *testing.T) {
	for _, payload := range []string{
		`{"request":{"url":"https://blog.example.com/posts?author=keep-1&page=2","headers":{":authority":"keep-2.example.com","Authorization":"Bearer leak-1"}},"extra":{"author":"keep-3","authority":"keep-4","authorName":"keep-5","oauth_provider":"keep-6","is_authenticated":"keep-7","session_count":"keep-8","password_strength":"keep-9","className":"keep-10","adsNetwork":"keep-11","tokenizer":"keep-12","tokens_used":"keep-13","max_tokens":"keep-14","public_key":"keep-15","key_id":"keep-16","cache_key":"keep-17","passenger":"keep-18","compass":"keep-19","password":"leak-2","ssn":"leak-3"},"breadcrumbs":[{"category":"ui.click","data":{"classname":"keep-20"}}]}`,
		`{"extra":{"pass":"leak-1","pw":"leak-2","passphrase":"leak-3","pin":"leak-4","otp":"leak-5","jwt":"leak-6","access_key":"leak-7","encryption_key":"leak-8","ssh_key":"leak-9","webhook_url":"https://hooks.slack.com/services/T0000/B0000/leak-10","credit_card":"leak-11","iban":"leak-12","cvc":"leak-13","security_code":"leak-14","order_id":"keep-1"}}`,
		`{"extra":{"PASSWORD":"leak-1","AccessToken":"leak-2","X-Auth-Token":"leak-3","aws.secret_access_key":"leak-4","Api.Key":"leak-5","private-key":"leak-6","cardNumber":"leak-7","X-CSRF-Token":"leak-8","SENTRY_DSN":"https://leak-9@o1.ingest.sentry.io/1","DB_PWD":"leak-10","X-Hub-Signature-256":"leak-11","SSN":"leak-12","ſecret":"leak-13","note":"keep-1"}}`,
		`{"extra":{"env":{"APP_ENV":"keep-1","APP_KEY":"base64:leak-1","STRIPE_KEY":"leak-2","AWS_ACCESS_KEY_ID":"leak-3","SENDGRID_KEY":"leak-4"}}}`,
		`{"request":{"url":"https://shop.example.com/cart.php?PHPSESSID=leak-1&page=keep-1","headers":{"Ocp-Apim-Subscription-Key":"leak-2","x-functions-key":"leak-3","X-RapidAPI-Key":"leak-4","X-Request-Id":"keep-2"},"data":{"passphrase":"leak-5","mfa_code":"leak-6","username":"keep-3"}},"extra":{"sid":"leak-7","connect.sid":"leak-8","cart":"keep-4"}}`,
		`{"user":{"id":"42","username":"keep-1","data":{"two_factor_recovery_codes":"leak-1","otp":"leak-2","pin":"leak-3","verification_code":"leak-4"}}}`,
		`{"message":"login {'author': 'keep-1', 'passphrase': 'leak-1'}","logentry":{"message":"POST /login {\"className\":\"keep-2\",\"pin\":\"leak-2\"}"}}`,
	} {
		assertScrubbed(t, payload, string(prepareEvent([]byte(payload))))
	}
}

func assertScrubbed(t *testing.T, payload, scrubbed string) {
	t.Helper()

	if strings.Contains(scrubbed, "leak-") {
		t.Errorf("a secret survived:\n%s", scrubbed)
	}

	if want, got := strings.Count(payload, "keep-"), strings.Count(scrubbed, "keep-"); got != want {
		t.Errorf("%d of %d kept values were removed:\n%s", want-got, want, scrubbed)
	}
}

// A [name, value] pair is an element of a list of pairs (headers, query
// strings, tags); a list of two field names under a key is no pair.
func TestScrubPairsOnlyInLists(t *testing.T) {
	for _, payload := range []string{
		`{"extra":{"validation_errors":{"missing":["password","keep-1"]},"required":["token","keep-2"],"fields":["api_key","keep-3","pin"],"api_key":"leak-1"}}`,
		`{"tags":[["auth_token","leak-1"],["release_channel","keep-1"]],"request":{"query_string":[["access_token","leak-2"],["page","keep-2"]],"headers":[["Authorization","Bearer leak-3"],["Accept","keep-3"]]}}`,
		`{"breadcrumbs":[["token","leak-1"]],"exception":{"values":[["password","leak-2"]]},"stacktrace":{"frames":[{"in_app":true},["secret","leak-3"]]}}`,
	} {
		assertScrubbed(t, payload, string(prepareEvent([]byte(payload))))
		assertScrubbed(t, payload, string(scrubJSON([]byte(payload))))
	}
}

// Cookies are filtered one by one: a cookie's name rarely says whether it
// holds a session, so only a few harmless ones keep their values.
func TestScrubCookies(t *testing.T) {
	for _, payload := range []string{
		`{"request":{"cookies":{"connect.sid":"s%3Aleak-1.sig","PHPSESSID":"leak-2","remember_web_59ba36addc2b2f9401580f014c7f58ea":"leak-3",".AspNetCore.Identity.Application":"leak-4","sid":"leak-5","theme":"keep-1","Locale":"keep-2","tz":null}}}`,
		`{"request":{"headers":{"Cookie":"sid=leak-1; theme=keep-1;lang=keep-2; leak-3; tracking=leak-4"},"cookies":"_ga=leak-5; timezone=keep-3"}}`,
		`{"request":{"cookies":[["sessionid","leak-1"],["theme","keep-1"]]}}`,
		`{"items":[{"body":"done","attributes":{"http.request.header.cookie":{"value":"sid=leak-1","type":"string"}}}]}`,
	} {
		assertScrubbed(t, payload, string(prepareEvent([]byte(payload))))
		assertScrubbed(t, payload, string(scrubJSON([]byte(payload))))
	}

	if got, want := scrubText(`{"cookies":{"theme":"dark","sid":"s1"},"Cookie":"theme=dark; sid=s1"}`), `{"cookies":{"theme":"dark","sid":"[Filtered]"},"Cookie":"theme=dark; sid=[Filtered]"}`; got != want {
		t.Errorf("cookies in a JSON text:\ngot  %s\nwant %s", got, want)
	}
}

// Free text carries key=value, key: value and key => value pairs: logfmt,
// connection strings, env dumps, header lines and the reprs of many languages.
func TestScrubKeyValuePairsInText(t *testing.T) {
	testCases := map[string]string{
		// logfmt, libpq, env dumps and spaced pairs.
		"level=warn user=root password=pw1 api_key=k1 session=s1":                      "level=warn user=root password=[Filtered] api_key=[Filtered] session=[Filtered]",
		"host=db port=5432 user=app password=pw1 sslmode=require":                      "host=db port=5432 user=app password=[Filtered] sslmode=require",
		"NODE_ENV=production\nDB_PASSWORD=pw1\nAWS_SECRET_ACCESS_KEY=k1\nAPP_NAME=app": "NODE_ENV=production\nDB_PASSWORD=[Filtered]\nAWS_SECRET_ACCESS_KEY=[Filtered]\nAPP_NAME=app",
		"config loaded, password = pw1, secret: s1, user = root":                       "config loaded, password = [Filtered], secret: [Filtered], user = root",
		// Connection strings: the key is the last word before '='.
		"open Server=db;Database=app;User Id=sa;Password=pw1;Encrypt=True":                      "open Server=db;Database=app;User Id=sa;Password=[Filtered];Encrypt=True",
		"open Server=tcp:db.example.com,1433;User ID=app;Password=pw1":                          "open Server=tcp:db.example.com,1433;User ID=app;Password=[Filtered]",
		"connect to jdbc:sqlserver://db.example.com:1433;user=sa;password=pw1;databaseName=app": "connect to jdbc:sqlserver://db.example.com:1433;user=sa;password=[Filtered];databaseName=app",
		// Reprs: Python, Ruby, PHP, Java, Go, Node.
		"User(username='root', password='pw1') <LoginForm token='t1'>":                                 "User(username='root', password='[Filtered]') <LoginForm token='[Filtered]'>",
		`{'user': b'root', 'password': b'pw1'} [(b'host', b'h'), (b'cookie', b'sid=s1')]`:              `{'user': b'root', 'password': b'[Filtered]'} [(b'host', b'h'), (b'cookie', b'sid=[Filtered]')]`,
		`params {"user"=>"root", "password"=>"pw1", :api_token=>"t1"} {:secret=>"s1"}`:                 `params {"user"=>"root", "password"=>"[Filtered]", :api_token=>"[Filtered]"} {:secret=>"[Filtered]"}`,
		`kwargs: {token: "t1", "secret" => "s1"}`:                                                      `kwargs: {token: "[Filtered]", "secret" => "[Filtered]"}`,
		"Array\n(\n    [username] => root\n    [password] => pw1\n)\narray (\n  'api_key' => 'k1',\n)": "Array\n(\n    [username] => root\n    [password] => [Filtered]\n)\narray (\n  'api_key' => '[Filtered]',\n)",
		"{password=pw1, username=root} User(username=root, token=t1) Credentials{apiKey='k1'}":         "{password=[Filtered], username=root} User(username=root, token=[Filtered]) Credentials{apiKey='[Filtered]'}",
		"&{Username:root Password:pw1 APIKey:k1} map[token:t1]":                                        "&{Username:root Password:[Filtered] APIKey:[Filtered]} map[token:[Filtered]]",
		"payload { user: 'root', password: 'pw1', headers: { authorization: 'Token t1' } }":            "payload { user: 'root', password: '[Filtered]', headers: { authorization: '[Filtered]' } }",
		// JSON escaped inside text.
		`request body="{\"username\":\"root\",\"password\":\"pw1\"}"`: `request body="{\"username\":\"root\",\"password\":\"[Filtered]\"}"`,
		// Header lines: Authorization keeps its scheme, cookies go one by one.
		"POST /login HTTP/1.1\r\nHost: api.example.com\r\nAuthorization: Token t1\r\nX-Api-Key: k1\r\nCookie: sid=s1; theme=dark\r\nUser-Agent: curl/8": "POST /login HTTP/1.1\r\nHost: api.example.com\r\nAuthorization: Token [Filtered]\r\nX-Api-Key: [Filtered]\r\nCookie: sid=[Filtered]; theme=dark\r\nUser-Agent: curl/8",
		"request headers: Authorization: token t1\nX-Api-Key: k1\nAccept: */*":                                                                          "request headers: Authorization: token [Filtered]\nX-Api-Key: [Filtered]\nAccept: */*",
		"Authorization: t1": "Authorization: [Filtered]",
	}

	for input, want := range testCases {
		if got := scrubText(input); got != want {
			t.Errorf("scrubText(%q)\ngot  %q\nwant %q", input, got, want)
		}
	}
}

// Prose, times, ports and pairs whose key names no secret stay byte for byte.
func TestScrubKeyValuePairsKeepsProse(t *testing.T) {
	for _, text := range []string{
		"password reset requested for john; token expired; session started; api key rotated",
		"Bearer is required; basic authentication disabled; Basic Auth failed for john; see https://example.com:8080/a?page=2 at 12:30 by user@example.com",
		"Invalid token: expired",
		"login failed: password too short",
		"Error: the session ended at 10:15:30",
		"key: value, name: root, user=root",
		"x = a == b; y != token; z <= secret",
		"std::token::parse failed",
		"password= next",
		"token: {",
		`Cannot find module "auth": gone`,
		`"token": else`,
		"set password: ",
		// Go errors lead with their package: what follows is prose.
		"jwt: token is expired",
		"deploy-credentials: script failed",
		"credentials: missing",
		"authorization: header missing",
		"session: not found",
		"Authorization: Basic ",
		"Authorization: \n",
	} {
		if got := scrubText(text); got != text {
			t.Errorf("scrubText(%q) = %q, want it unchanged", text, got)
		}
	}
}

// The audit's payloads as SDKs send them: every "leak-" value goes, every "keep-" value stays.
func TestScrubKeyValuePairsInPayloads(t *testing.T) {
	for _, payload := range []string{
		`{"exception":{"values":[{"type":"ValidationError","value":"keep-1 bad input User(username='root', password='leak-1')","stacktrace":{"frames":[{"filename":"app/UserService.java","function":"save","in_app":true,"vars":{"user":"User{username=keep-2, password=leak-2}","config":"{Host:keep-3 Password:leak-3 Port:5432}"}}]}}]}}`,
		`{"message":"keep-1 params {\"user\"=>\"root\", \"password\"=>\"leak-1\"}","extra":{"php_dump":"array ( 'user' => 'keep-2', 'api_token' => 'leak-2' )","ruby_symbols":"{:secret=>\"leak-3\"}"}}`,
		`{"exception":{"values":[{"type":"KeyError","value":"keep-1","stacktrace":{"frames":[{"filename":"app/auth.py","function":"login","in_app":true,"vars":{"payload":"{'user': b'keep-2', 'password': b'leak-1'}","scope_headers":"[(b'host', b'keep-3'), (b'cookie', b'sid=leak-2')]"}}]}}]}}`,
		`{"message":"keep-1 upstream request failed","extra":{"request_dump":"POST /login HTTP/1.1\r\nHost: api.example.com\r\nAuthorization: Token leak-1\r\nX-Api-Key: leak-2\r\nCookie: sid=leak-3; theme=dark\r\nUser-Agent: keep-2"}}`,
		`{"exception":{"values":[{"type":"SqlException","value":"Login failed using Server=db.example.com;Database=app;User Id=sa;Password=leak-1;Encrypt=True"}]},"contexts":{"db":{"connectionString":"Data Source=db;Initial Catalog=app;User ID=keep-1;Password=leak-2"}}}`,
		`{"platform":"go","exception":{"values":[{"type":"*errors.errorString","value":"keep-1 login failed: &{Username:root Password:leak-1 APIKey:leak-2} map[token:leak-3]"}]}}`,
		`{"breadcrumbs":[{"category":"console","message":"request headers: Authorization: token leak-1\nX-Api-Key: leak-2\nAccept: keep-1"}]}`,
		`{"message":"keep-1: config loaded, password = leak-1, secret: leak-2"}`,
		`{"message":"keep-1: startup failed","extra":{"env":"NODE_ENV=production\nDB_PASSWORD=leak-1\nAWS_SECRET_ACCESS_KEY=leak-2\nSTRIPE_API_KEY=leak-3\nAPP_NAME=keep-2"}}`,
		`{"message":"keep-1: Bearer is required; basic authentication disabled; Basic Auth failed for john; see https://example.com:8080/a?page=2 at 12:30 by user@example.com; password reset requested for john; token expired; session started; api key rotated"}`,
	} {
		assertScrubbed(t, payload, string(prepareEvent([]byte(payload))))
		assertScrubbed(t, payload, string(scrubJSON([]byte(payload))))
	}

	for _, item := range []string{
		`{"timestamp":1763900000,"level":"warn","body":"level=warn msg=login_failed user=keep-1 password=leak-1 api_key=leak-2 session=leak-3","attributes":{}}`,
		`{"timestamp":1763900000,"level":"info","body":"keep-1 payload { user: 'root', password: 'leak-1', headers: { authorization: 'Token leak-2' } }","attributes":{}}`,
		`{"timestamp":1795430400,"level":"error","body":"keep-1 Array\n(\n    [username] => root\n    [password] => leak-1\n)\n array (\n  'api_key' => 'leak-2',\n)","attributes":{}}`,
		`{"timestamp":1795430400,"level":"warn","body":"keep-1 request body=\"{\\\"username\\\":\\\"root\\\",\\\"password\\\":\\\"leak-1\\\"}\"","attributes":{}}`,
	} {
		payload := `{"items":[` + item + `]}`

		assertScrubbed(t, payload, string(scrubJSON([]byte(payload))))
	}
}

// Authorization schemes other than Bearer and Basic, quoted tokens and a
// Basic user:password sent as it is.
func TestScrubAuthSchemes(t *testing.T) {
	testCases := map[string]string{
		`proxy rejected Basic admin:leak-1 and Bearer "leak-2"`: `proxy rejected Basic admin:[Filtered] and Bearer "[Filtered]"`,
		"retry with Authorization: ApiKey bGVhay0xOmxlYWstMg==": "retry with Authorization: ApiKey [Filtered]",
		"retry with Token 0123456789abcdef":                     "retry with Token [Filtered]",
		"sent Api-Key leak-1-0123 upstream":                     "sent Api-Key [Filtered] upstream",
		`sent Digest username="root", realm="api", nonce="dcd98b7102dd2f0e", uri="/x", response="6629fae49393a05397450978507c4ef1" to /x`:                                   "sent Digest [Filtered] to /x",
		"PUT /bucket/key Authorization: AWS4-HMAC-SHA256 Credential=AKIAEXAMPLE/20261123/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-date, Signature=leak-1; retry": "PUT /bucket/key Authorization: AWS4-HMAC-SHA256 [Filtered]; retry",
		"got Negotiate YIIGhgYGKwYBBQUCoIIGejCCBnag back":                        "got Negotiate [Filtered] back",
		"got NTLM TlRMTVNTUAABAAAAB4IIogAAAAAAAAAAAAAAAAAAAAAGAbEdAAAADw== back": "got NTLM [Filtered] back",
	}

	for input, want := range testCases {
		if got := scrubText(input); got != want {
			t.Errorf("scrubText(%q)\ngot  %q\nwant %q", input, got, want)
		}
	}

	for _, text := range []string{
		"Token expired; token validation failed; ApiKey missing",
		"Digest authentication failed; NTLM authentication failed; Negotiate failed",
		"basic authentication: disabled",
		`Bearer "token" is required`,
	} {
		if got := scrubText(text); got != text {
			t.Errorf("scrubText(%q) = %q, want it unchanged", text, got)
		}
	}
}

// Command lines put credentials into exception values: curl's -u and -H.
func TestScrubCommandLineCredentials(t *testing.T) {
	testCases := map[string]string{
		"curl -u admin:leak-1 -H 'X-Api-Key: leak-2' https://api.example.com/items?q=keep": "curl -u admin:[Filtered] -H 'X-Api-Key: [Filtered]' https://api.example.com/items?q=keep",
		`curl --user admin:pw1 -H "Authorization: Token t0ken1" https://api.example.com/`:  `curl --user admin:[Filtered] -H "Authorization: Token [Filtered]" https://api.example.com/`,
		"curl --user=admin:pw1 https://api.example.com/":                                   "curl --user=admin:[Filtered] https://api.example.com/",
		"curl -u 'admin:pw1' https://api.example.com/":                                     "curl -u 'admin:[Filtered]' https://api.example.com/",
		"curl -U proxy:pw1 --proxy-user proxy:pw2 https://api.example.com/":                "curl -U proxy:[Filtered] --proxy-user proxy:[Filtered] https://api.example.com/",
	}

	for input, want := range testCases {
		if got := scrubText(input); got != want {
			t.Errorf("scrubText(%q)\ngot  %q\nwant %q", input, got, want)
		}
	}

	for _, text := range []string{"curl -u admin https://api.example.com/", "sort -u names.txt"} {
		if got := scrubText(text); got != text {
			t.Errorf("scrubText(%q) = %q, want it unchanged", text, got)
		}
	}
}

// Card numbers are found by their value: 13 to 19 digits, grouped or not,
// that pass the Luhn check.
func TestScrubCardNumbers(t *testing.T) {
	testCases := map[string]string{
		"charge 4242 4242 4242 4242 declined": "charge [Filtered] declined",
		"card 4111-1111-1111-1111":            "card [Filtered]",
		"amex 378282246310005 ok":             "amex [Filtered] ok",
		"amex 3782 822463 10005, retry":       "amex [Filtered], retry",
		"order 12 4242 4242 4242 4242":        "order 12 [Filtered]",
		"cc=4242424242424242&page=2":          "cc=[Filtered]&page=2",
		`{"pan":"5555555555554444"}`:          `{"pan":"[Filtered]"}`,
	}

	for input, want := range testCases {
		if got := scrubText(input); got != want {
			t.Errorf("scrubText(%q)\ngot  %q\nwant %q", input, got, want)
		}
	}

	for _, text := range []string{
		"at 1700000000000 ms", "id 1234567890123", "card 4242 4242 4242 4241", "random 0.4242424242424242",
		"trace 4bf92f3577b34da6a3ce929d0e0e4736", "n 12345678901234567890", "at 2026-11-23 10:00:00", "call +1 555 123 4567",
		"zeros 0000 0000 0000 0000", "ID 4242424242424242424242", "1 2 3 4 5 6 7 8 9 0 1 2 3",
	} {
		if got := scrubText(text); got != text {
			t.Errorf("scrubText(%q) = %q, want it unchanged", text, got)
		}
	}

	payload := `{"extra":{"amount":4242424242424242,"count":12345678901234567890,"ts":1700000000000,"price":4242424242424242.5,"note":"keep-1"}}`
	want := `{"extra":{"amount":"[Filtered]","count":12345678901234567890,"ts":1700000000000,"price":4242424242424242.5,"note":"keep-1"}}`

	for name, scrub := range map[string]func([]byte) []byte{"prepareEvent": prepareEvent, "scrubJSON": scrubJSON} {
		if got := string(scrub([]byte(payload))); got != want {
			t.Errorf("%s:\ngot  %s\nwant %s", name, got, want)
		}
	}
}

// URL parameters that carry credentials by their name: signatures, API keys,
// OAuth codes and session ids, in the query, the fragment or the path.
func TestScrubURLParameters(t *testing.T) {
	testCases := map[string]string{
		"https://maps.googleapis.com/maps/api/geocode/json?address=x&key=k1": "https://maps.googleapis.com/maps/api/geocode/json?address=x&key=%5BFiltered%5D",
		"/callback?code=c1&state=s1":                                         "/callback?code=%5BFiltered%5D&state=s1",
		"https://app.example.com/cb#code=c1&state=s1":                        "https://app.example.com/cb#code=%5BFiltered%5D&state=s1",
		"code=c1&state=s1": "code=%5BFiltered%5D&state=s1",
		"https://shop.example.com/cart;jsessionid=ABC123?page=2":                  "https://shop.example.com/cart;jsessionid=%5BFiltered%5D?page=2",
		"GET /cart;jsessionid=ABC123/items 200":                                   "GET /cart;jsessionid=%5BFiltered%5D/items 200",
		"https://x.example.com/a?apikey=1&api-key=2&client_secret=3&sid=4":        "https://x.example.com/a?apikey=%5BFiltered%5D&api-key=%5BFiltered%5D&client_secret=%5BFiltered%5D&sid=%5BFiltered%5D",
		"https://x.example.com/a?password_reset_token=5&signature=6&access_key=7": "https://x.example.com/a?password_reset_token=%5BFiltered%5D&signature=%5BFiltered%5D&access_key=%5BFiltered%5D",
		"https://x.example.com/a?sig=8&Key=9":                                     "https://x.example.com/a?sig=%5BFiltered%5D&Key=%5BFiltered%5D",
	}

	for input, want := range testCases {
		if got := scrubText(input); got != want {
			t.Errorf("scrubText(%q)\ngot  %q\nwant %q", input, got, want)
		}
	}

	for _, text := range []string{
		"https://x.example.com/a?monkey=1&keyboard=2&key_id=3&zipcode=4&country_code=UA&state=5",
		"https://x.example.com/a;v=1/b?page=2",
		"process exited with code=1",
	} {
		if got := scrubText(text); got != text {
			t.Errorf("scrubText(%q) = %q, want it unchanged", text, got)
		}
	}
}

// Private keys in PEM keep their BEGIN and END lines; the body goes.
func TestScrubPrivateKeys(t *testing.T) {
	testCases := map[string]string{
		"-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBg\n-----END PRIVATE KEY-----\n":                                                          "-----BEGIN PRIVATE KEY-----\n[Filtered]\n-----END PRIVATE KEY-----\n",
		"bad key -----BEGIN RSA PRIVATE KEY-----\nProc-Type: 4,ENCRYPTED\nMIIE\n-----END RSA PRIVATE KEY----- in env":                       "bad key -----BEGIN RSA PRIVATE KEY-----\n[Filtered]\n-----END RSA PRIVATE KEY----- in env",
		"-----BEGIN OPENSSH PRIVATE KEY-----\\nb3BlbnNzaC1rZXk\\n-----END OPENSSH PRIVATE KEY-----":                                         "-----BEGIN OPENSSH PRIVATE KEY-----\\n[Filtered]\\n-----END OPENSSH PRIVATE KEY-----",
		"a -----BEGIN EC PRIVATE KEY-----\nMHcCAQEE and b -----BEGIN ENCRYPTED PRIVATE KEY-----\nMIIF\n-----END ENCRYPTED PRIVATE KEY-----": "a -----BEGIN EC PRIVATE KEY-----\n[Filtered]\n-----END ENCRYPTED PRIVATE KEY-----",
		"cut -----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBg":                                                                                   "cut -----BEGIN PRIVATE KEY-----\n[Filtered]",
	}

	for input, want := range testCases {
		if got := scrubText(input); got != want {
			t.Errorf("scrubText(%q)\ngot  %q\nwant %q", input, got, want)
		}
	}

	for _, text := range []string{
		"-----BEGIN CERTIFICATE-----\nMIIBszCCAVmgAwIBAgIU\n-----END CERTIFICATE-----",
		"-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYI\n-----END PUBLIC KEY-----",
	} {
		if got := scrubText(text); got != text {
			t.Errorf("scrubText(%q) = %q, want it unchanged", text, got)
		}
	}

	payload := `{"extra":{"config":"-----BEGIN PRIVATE KEY-----\nleak-1MIIEvQIBADANBgkqhkiG9w0BAQEFAASC\n-----END PRIVATE KEY-----\n","name":"keep-1"}}`

	assertScrubbed(t, payload, string(prepareEvent([]byte(payload))))
}

// A password may hold '@' and '/': it ends at the last '@' a host follows.
func TestScrubURLPasswordWithAtAndSlash(t *testing.T) {
	testCases := map[string]string{
		"cannot connect amqp://guest:p@ss/x@mq.example.com:5672/vhost": "cannot connect amqp://guest:%5BFiltered%5D@mq.example.com:5672/vhost",
		"amqp://guest:p@ss/x@mq/vhost":                                 "amqp://guest:%5BFiltered%5D@mq/vhost",
		"https://user:p@ss@db.example.com/x and https://u:pw@cache/0":  "https://user:%5BFiltered%5D@db.example.com/x and https://u:%5BFiltered%5D@cache/0",
		"postgres://user:p@ss/x@":                                      "postgres://user:%5BFiltered%5D@",
		"mongodb://app:pw@h1:27017,h2:27017/app?replicaSet=rs0":        "mongodb://app:%5BFiltered%5D@h1:27017,h2:27017/app?replicaSet=rs0",
		"(postgres://user:pw@db)":                                      "(postgres://user:%5BFiltered%5D@db)",
		"https://admin:pw@api.example.com/users/a@b=c":                 "https://admin:%5BFiltered%5D@api.example.com/users/a@b=c",
		"https://user:a/b@c@10.0.0.5:8080/x":                           "https://user:%5BFiltered%5D@10.0.0.5:8080/x",
		"https://user:a@b/c@[::1]:8080/x":                              "https://user:%5BFiltered%5D@[::1]:8080/x",
	}

	for input, want := range testCases {
		if got := scrubText(input); got != want {
			t.Errorf("scrubText(%q)\ngot  %q\nwant %q", input, got, want)
		}
	}
}

// Bodies that hold other encodings: JSON in JSON on several levels, a body
// encoded twice, collections under a secret's key in text, multipart form
// data and GraphQL arguments.
func TestScrubNestedEncodings(t *testing.T) {
	for name, payload := range map[string]string{
		"json two levels":  `{"message":"keep-1","request":{"method":"POST","url":"https://app.example.com/webhook","data":"{\"event\":\"keep-2\",\"body\":\"{\\\"inner\\\":\\\"{\\\\\\\"password\\\\\\\":\\\\\\\"leak-1\\\\\\\"}\\\"}\"}"}}`,
		"double encoded":   `{"message":"keep-1","request":{"method":"POST","url":"https://app.example.com/login","data":"\"{\\\"user\\\":\\\"keep-2\\\",\\\"password\\\":\\\"leak-1\\\"}\""}}`,
		"multipart":        `{"message":"keep-1","request":{"method":"POST","url":"https://app.example.com/signup","data":"--X\r\nContent-Disposition: form-data; name=\"username\"\r\n\r\nkeep-2\r\n--X\r\nContent-Disposition: form-data; name=\"password\"\r\n\r\nleak-1\r\n--X\r\nContent-Disposition: form-data; name=\"api_key\"\r\nContent-Type: text/plain\r\n\r\nleak-2\r\n--X--\r\n"}}`,
		"graphql":          `{"message":"keep-1","request":{"method":"POST","url":"https://api.example.com/graphql","data":{"operationName":"Login","query":"mutation Login { login(username: \"keep-2\", password: \"leak-1\") { token } }","variables":{"apiKey":"leak-2"}}}}`,
		"graphql as text":  `{"message":"keep-1","request":{"data":"{\"query\":\"mutation { login(username: \\\"keep-2\\\", password: \\\"leak-1\\\") { token } }\"}"}}`,
		"collections text": `{"message":"keep-1 payload {\"api_keys\":[\"leak-1\"],\"credentials\":{\"user\":\"root\",\"value\":\"leak-2\"},\"note\":\"keep-2\"}"}`,
	} {
		t.Run(name, func(t *testing.T) {
			assertScrubbed(t, payload, string(prepareEvent([]byte(payload))))
			assertScrubbed(t, payload, string(scrubJSON([]byte(payload))))
		})
	}

	item := `{"timestamp":1763900000,"level":"warn","body":"keep-1 payload {\"api_keys\":[\"leak-1\"],\"credentials\":{\"user\":\"root\",\"value\":\"leak-2\"}} keep-2","attributes":{}}`
	payload := `{"items":[` + item + `]}`

	assertScrubbed(t, payload, string(scrubJSON([]byte(payload))))

	// Escaped pairs are found at any depth: documents in documents, and a
	// body encoded as a JSON string once more.
	nested, encodedTwice := `{"user":"keep-1","password":"leak-1","pin":1234,"api_keys":["leak-2"],"note":"keep-2"}`, `{"user":"keep-1","password":"leak-1"}`

	for depth := 1; depth <= 5; depth++ {
		encoded, _ := json.Marshal(nested)
		nested = `{"body":` + string(encoded) + `}`
		encoded, _ = json.Marshal(encodedTwice)
		encodedTwice = string(encoded)

		for _, text := range []string{nested, encodedTwice} {
			payload, _ := json.Marshal(text)
			event := `{"message":` + string(payload) + `}`
			scrubbed := prepareEvent([]byte(event))

			assertScrubbed(t, event, string(scrubbed))

			if !json.Valid(scrubbed) {
				t.Errorf("depth %d: not JSON: %s", depth, scrubbed)
			}
		}
	}
}

func TestScrubEscapedAndCollectionValues(t *testing.T) {
	testCases := map[string]string{
		`body {\"user\":\"root\",\"password\":\"a\\\"b\"}`:                     `body {\"user\":\"root\",\"password\":\"[Filtered]\"}`,
		`{\\\"password\\\":\\\"pw1\\\",\\\"n\\\":2}`:                           `{\\\"password\\\":\\\"[Filtered]\\\",\\\"n\\\":2}`,
		`got {"secrets":["s1",{"a":"]"}],"user":"root"}`:                       `got {"secrets":"[Filtered]","user":"root"}`,
		`got {\"api_keys\":[\"k]1\"],\"user\":\"root\"}`:                       `got {\"api_keys\":\"[Filtered]\",\"user\":\"root\"}`,
		`params {"credentials"=>{"user"=>"root"}, "page"=>2}`:                  `params {"credentials"=>"[Filtered]", "page"=>2}`,
		`{'secrets': ['s1', 's2'], 'user': 'root'}`:                            `{'secrets': '[Filtered]', 'user': 'root'}`,
		`cut {"credentials":{"user":"root","password":"pw`:                     `cut {"credentials":"[Filtered]"`,
		"--X\r\nContent-Disposition: form-data; name=\"pin\"\r\n\r\n\r\n--X--": "--X\r\nContent-Disposition: form-data; name=\"pin\"\r\n\r\n\r\n--X--",
		"--X\nContent-Disposition: form-data; name=token\n\nt1\nt2\n--X\nContent-Disposition: form-data; name=\"file\"; filename=\"password.txt\"\n\nkept\n--X--": "--X\nContent-Disposition: form-data; name=token\n\n[Filtered]\n--X\nContent-Disposition: form-data; name=\"file\"; filename=\"password.txt\"\n\nkept\n--X--",
		"--X\r\nContent-Disposition: form-data; name=\"password\"\r\n\r\npw1":                                                                                     "--X\r\nContent-Disposition: form-data; name=\"password\"\r\n\r\n[Filtered]",
	}

	for input, want := range testCases {
		if got := scrubText(input); got != want {
			t.Errorf("scrubText(%q)\ngot  %q\nwant %q", input, got, want)
		}
	}
}

// Chat services put their tokens into URL paths: Slack and Discord
// webhooks, Telegram bot tokens.
func TestScrubServiceTokensInPaths(t *testing.T) {
	payload := `{"message":"keep-1","breadcrumbs":{"values":[` +
		`{"category":"http","data":{"method":"POST","url":"https://hooks.slack.com/services/T0000/B0000/leak-1","status_code":200}},` +
		`{"category":"http","data":{"method":"POST","url":"https://api.telegram.org/bot123456:leak-2/sendMessage","status_code":200}},` +
		`{"category":"http","data":{"method":"POST","url":"https://discord.com/api/webhooks/123/leak-3","status_code":204}},` +
		`{"category":"http","data":{"method":"POST","url":"https://discordapp.com/api/v10/webhooks/123/leak-4?wait=true","status_code":204}},` +
		`{"category":"console","message":"Max retries exceeded with url: /bot123456789:AAEhBOweik6ad9r_QXMENQjcrGbqCr4K-leak-5/getUpdates"},` +
		`{"category":"navigation","data":{"to":"/keep-2/services/T1/settings"}}]}}`

	assertScrubbed(t, payload, string(prepareEvent([]byte(payload))))
	assertScrubbed(t, payload, string(scrubJSON([]byte(payload))))
}

// Key names in other languages and a card's number under its card.
func TestScrubLocalizedKeysAndCards(t *testing.T) {
	for _, payload := range []string{
		`{"extra":{"пароль":"leak-1","Passwort":"leak-2","contraseña":"leak-3","mot_de_passe":"leak-4","senha":"leak-5","wachtwoord":"leak-6","order":"keep-1"}}`,
		`{"request":{"data":{"card":{"number":"leak-1","cvv":"leak-2","brand":"keep-1"},"billing_card":{"number":"leak-3","exp_month":"keep-2"},"order":{"number":"keep-3"}}}}`,
		`{"extra":{"card":{"number":{"value":"leak-1","type":"string"}},"cards":"keep-1"}}`,
	} {
		assertScrubbed(t, payload, string(prepareEvent([]byte(payload))))
		assertScrubbed(t, payload, string(scrubJSON([]byte(payload))))
	}

	if got, want := scrubText(`{"card":{"number":"leak-1","brand":"visa"}}`), `{"card":{"number":"[Filtered]","brand":"visa"}}`; got != want {
		t.Errorf("a card in a JSON text:\ngot  %s\nwant %s", got, want)
	}
}

// Messages, stack traces and log lines whose words only look like pairs.
func TestScrubKeepsMessagesThatLookLikePairs(t *testing.T) {
	for _, text := range []string{
		// Stack frames of files named after a secret's word.
		"at Object.<anonymous> (/app/src/auth/token.ts:42:13)",
		"at login (/app/src/session.js:10:5)",
		"at verify (webpack://app/src/password.ts:3:1)",
		"/app/internal/session/session.go:42 +0x1d",
		"\t/app/token.go:12 +0x20",
		`at com.example.Token.verify(Token.java:42)`,
		"Error: x\n    at f (C:\\app\\src\\token.ts:42:13)",
		// Prose after a key, counts and codes.
		"private-key: error:1E08010C:DECODER routines::unsupported",
		"token: 3 retries left",
		"Session: 2 of 3 failed",
		"session: 401",
		"Session: 12",
		"secrets: 3 loaded",
		"session: 3 of 5 completed",
		"cookies: 0",
		"cookies: cookies",
		"my-cookies: cookies",
		"api-key: env-key",
		"session: abc-def",
		"session: session-id-names",
		"set session = active for root",
		"ValueError: password = None",
		"PasswordMismatch: 2 attempts left",
		"SecretsManagerException: ResourceNotFoundException",
		"tests: pass=118 fail=2 skip=3",
		"pass=true checks=12",
		"session=3 of 5",
		"code=ERR_NETWORK",
		"code=1",
		"key=Enter",
		// Parser messages and prose after the word token.
		"Unexpected token NumericLiteral in JSON",
		"parse error: unexpected token RIGHT_PAREN",
		"lexer emitted token IDENTIFIER(foo)",
		"refresh token 2026-11-23 failed",
		"got token user.updated from queue",
		"path=/api/token method=POST status=200",
	} {
		if got := scrubText(text); got != text {
			t.Errorf("scrubText(%q) = %q, want it unchanged", text, got)
		}
	}

	testCases := map[string]string{
		// A pair inside the message still goes.
		"SessionNotFound: session=abc not in store": "SessionNotFound: session=[Filtered] not in store",
		"redis GET session:abc123 -> nil":           "redis GET session:[Filtered] -> nil",
		"pin=1 otp=sent":                            "pin=[Filtered] otp=[Filtered]",
		"password=1234, pin: 1234":                  "password=[Filtered], pin: [Filtered]",
		// A quoted value is no prose; a marker leaves nothing after it.
		`word password: "leak-1" end`:                 `word password: "[Filtered]" end`,
		"x [Filtered] password=[Filtered]leak-2 y":    "x [Filtered] password=[Filtered] y",
		"token=%5BFiltered%5D password=[Filtered]x":   "token=%5BFiltered%5D password=[Filtered]",
		"password=%5BFiltered%5Dleak-3 next":          "password=[Filtered] next",
		"call Token 0123456789abcdef0123 and retry":   "call Token [Filtered] and retry",
		"login failed: token: eyJhbGciOiJIUzI1NiJ9.x": "login failed: token: [Filtered]",
	}

	for input, want := range testCases {
		if got := scrubText(input); got != want {
			t.Errorf("scrubText(%q)\ngot  %q\nwant %q", input, got, want)
		}
	}
}

// Source lines are code: an expression after a secret's name stays, a
// string literal written there goes.
func TestScrubSourceLines(t *testing.T) {
	lines := []string{
		"  const token = jwt.sign(payload, key);",
		"    password: hashedPassword,",
		"\ttoken := r.Header.Get(\"Authorization\")",
		"    session = Session.objects.get(pk=session_id)",
		"  pin: Boolean(row.pinned),",
		"  cookie: req.headers.cookie,",
		"  otp: z.string().length(6),",
		"  this.session = session;",
		"  headers: { Authorization: `Bearer ${token}` },",
	}
	source, _ := json.Marshal(lines)
	payload := `{"exception":{"values":[{"type":"Error","value":"keep-1","stacktrace":{"frames":[{"filename":"/app/a.js","in_app":true,` +
		`"pre_context":` + string(source) + `,"context_line":"  const token = jwt.sign(payload, key);","post_context":["  const API_KEY = \"leak-1\";","  password: 'leak-2',"]}]}}]}}`

	for name, scrub := range map[string]func([]byte) []byte{"prepareEvent": prepareEvent, "scrubJSON": scrubJSON} {
		scrubbed := string(scrub([]byte(payload)))

		if strings.Contains(scrubbed, "leak-") {
			t.Errorf("%s: a secret survived:\n%s", name, scrubbed)
		}

		for _, line := range lines {
			encoded, _ := json.Marshal(line)

			if !strings.Contains(scrubbed, string(encoded)) {
				t.Errorf("%s: %s was changed:\n%s", name, encoded, scrubbed)
			}
		}
	}
}
