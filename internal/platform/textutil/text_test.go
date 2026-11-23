package textutil

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestShorten(t *testing.T) {
	for _, test := range []struct {
		text  string
		limit int
		want  string
	}{
		{"abc", 3, "abc"},
		{"abcd", 3, "ab…"},
		{"привіт", 4, "при…"},
		{"привіт", 6, "привіт"},
		{"ab", 1, "…"},
	} {
		if got := Shorten(test.text, test.limit); got != test.want {
			t.Errorf("Shorten(%q, %d) = %q, want %q", test.text, test.limit, got, test.want)
		}
	}

	if got := TruncateRunes("привіт", 3); got != "при" {
		t.Errorf("TruncateRunes = %q", got)
	}
}

// A long text is cut without copying it whole (an exception value can be megabytes).
func TestShortenDoesNotCopyTheText(t *testing.T) {
	long := strings.Repeat("x", 1<<20)

	var cut string

	if allocations := testing.AllocsPerRun(10, func() { cut = TruncateRunes(long, 500) }); allocations != 0 || len(cut) != 500 {
		t.Errorf("TruncateRunes allocated %v times", allocations)
	}

	if allocations := testing.AllocsPerRun(10, func() { cut = Shorten(long, 500) }); allocations > 1 {
		t.Errorf("Shorten allocated %v times", allocations)
	}
}

func TestMaskSecret(t *testing.T) {
	for secret, want := range map[string]string{
		"":      "",
		"short": "…",
		// As Slack shows it: the workspace and hook ids, never the last segment.
		"https://hooks.slack.com/services/T0001/B0002/abcdefghijklmnopqrstuvwx": "https://hooks.slack.com/services/T0001/B0002/…",
		"https://chat.example.com/hooks/abcdefghijklmnopqrstuvwx":               "https://chat.example.com/hooks/…",
		// A webhook URL with userinfo or a query shows neither.
		"https://user:pass@hooks.example.com/hooks/abcdefghij?token=abcdefghij": "https://hooks.example.com/hooks/…",
		// Split so secret scanners do not take this made-up token for a real one.
		"xoxb-" + "1234567890-0987654321-AbCdEfGhIjKlMnOp": "xoxb-1234567890-0987654321-…", // the team id is not secret
		"123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw":     "123456789:…",
		// A "-" in the random part reveals nothing before it.
		"123456789:AAH-qTcvCH1vGWJxfSeofSAs0K5PALDsaw":              "123456789:…",
		"emcp_pt_" + "Ab-cdefghijklmnopqrstuvwxyz0123456789ABCDEFG": "emcp…",
	} {
		if got := MaskSecret(secret); got != want {
			t.Errorf("MaskSecret(%q) = %q, want %q", secret, got, want)
		}
	}
}

func TestIndentJSON(t *testing.T) {
	for raw, want := range map[string]string{
		`{"a":[1,2]}`: "{\n  \"a\": [\n    1,\n    2\n  ]\n}",
		`not json`:    "not json",
	} {
		if got := IndentJSON(json.RawMessage(raw)); got != want {
			t.Errorf("IndentJSON(%q) = %q, want %q", raw, got, want)
		}
	}
}
