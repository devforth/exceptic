package fixtures

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// DocValue is a value of the code that a document states: Phrase is the
// text around it, with {} where the value stands.
type DocValue struct {
	Phrase string
	Value  any
}

// DocValues checks that a document (a path from the repository's root)
// states each value of the code wherever its phrase occurs, so the code is
// the one source of the number: a stale value fails, UPDATE_API_TYPES=1
// (make api-types) writes the current one in.
func DocValues(t testing.TB, document string, values ...DocValue) {
	t.Helper()

	path := filepath.Join(Root(), document)
	content, err := os.ReadFile(path)

	if err != nil {
		t.Fatal(err)
	}

	text := string(content)

	var stale []string

	for _, value := range values {
		before, after, ok := strings.Cut(value.Phrase, "{}")

		if !ok || before == "" || after == "" {
			t.Fatalf("phrase %q: needs text on both sides of {}", value.Phrase)
		}

		slot := `(.+?)`

		switch value.Value.(type) {
		case int, int64, uint32:
			slot = `(\d[\d,]*)`
		}

		want := fmt.Sprint(value.Value)
		matches := regexp.MustCompile(regexp.QuoteMeta(before)+slot+regexp.QuoteMeta(after)).FindAllStringSubmatchIndex(text, -1)

		if len(matches) == 0 {
			t.Errorf("%s no longer says %q: change the phrase in the test", document, value.Phrase)

			continue
		}

		// From the end, so the earlier indexes stay valid.
		for index := len(matches) - 1; index >= 0; index-- {
			start, end := matches[index][2], matches[index][3]

			if text[start:end] != want {
				stale = append(stale, fmt.Sprintf("%q says %s, the code %s", value.Phrase, text[start:end], want))
				text = text[:start] + want + text[end:]
			}
		}
	}

	if len(stale) == 0 {
		return
	}

	if os.Getenv("UPDATE_API_TYPES") == "" {
		t.Errorf("%s is stale (run make api-types):\n%s", document, strings.Join(stale, "\n"))

		return
	}

	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Thousands writes a number as the documents do, 10000 as 10,000.
func Thousands(number int) string {
	text := strconv.Itoa(number)

	for index := len(text) - 3; index > 0; index -= 3 {
		text = text[:index] + "," + text[index:]
	}

	return text
}

// Root is the repository's root, found relative to this file.
func Root() string {
	_, file, _, _ := runtime.Caller(0)

	return filepath.Join(filepath.Dir(file), "..", "..")
}
