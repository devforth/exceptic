package textutil

import (
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
