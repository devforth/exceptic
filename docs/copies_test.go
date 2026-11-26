package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// copyMarker opens a copy of a block another document holds:
// <!-- name: copied from <file> by make api-types -->.
var copyMarker = regexp.MustCompile(`(?m)^([ \t]*)<!-- ([a-z-]+): copied from (\S+) by make api-types -->\n`)

// TestDocsCopies: a document that cannot link (docs/dockerhub.md, which
// Docker Hub shows alone) holds copies of blocks of the others, marked
// <!-- name --> … <!-- /name --> where they are written; a stale copy fails,
// UPDATE_API_TYPES=1 (make api-types) rewrites it.
func TestDocsCopies(t *testing.T) {
	documents, err := filepath.Glob("*.md")

	if err != nil {
		t.Fatal(err)
	}

	copies := 0

	for _, document := range append(documents, "../README.md") {
		content, err := os.ReadFile(document)

		if err != nil {
			t.Fatal(err)
		}

		text := string(content)
		updated, count, err := fillCopies(text)

		if err != nil {
			t.Fatalf("%s: %v", document, err)
		}

		copies += count

		if updated == text {
			continue
		}

		if os.Getenv("UPDATE_API_TYPES") == "" {
			t.Errorf("%s: a copied block is stale; run make api-types", document)

			continue
		}

		if err := os.WriteFile(document, []byte(updated), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if copies == 0 {
		t.Error("no copied blocks found")
	}
}

// fillCopies puts the current source of every copied block into text.
func fillCopies(text string) (string, int, error) {
	count := 0

	for offset := 0; ; {
		match := copyMarker.FindStringSubmatchIndex(text[offset:])

		if match == nil {
			return text, count, nil
		}

		indent := text[offset+match[2] : offset+match[3]]
		name := text[offset+match[4] : offset+match[5]]
		source := text[offset+match[6] : offset+match[7]]
		start := offset + match[1]
		end := strings.Index(text[start:], indent+"<!-- /"+name+" -->")

		if end < 0 {
			return "", 0, fmt.Errorf("no end of the copy of %s", name)
		}

		content, err := os.ReadFile(filepath.Join("..", source)) // named from the repository's root

		if err != nil {
			return "", 0, err
		}

		block, err := sourceBlock(string(content), name)

		if err != nil {
			return "", 0, fmt.Errorf("%s: %w", source, err)
		}

		block = indented(block, indent)
		text = text[:start] + block + text[start+end:]
		offset = start + len(block)
		count++
	}
}

// sourceBlock is the text between <!-- name --> and <!-- /name -->, its
// lines without the indent of the opening marker.
func sourceBlock(text, name string) (string, error) {
	opening := regexp.MustCompile(`(?m)^([ \t]*)<!-- ` + regexp.QuoteMeta(name) + ` -->\n`)
	match := opening.FindStringSubmatchIndex(text)

	if match == nil {
		return "", fmt.Errorf("no <!-- %s --> block", name)
	}

	indent := text[match[2]:match[3]]
	rest := text[match[1]:]
	end := strings.Index(rest, indent+"<!-- /"+name+" -->")

	if end < 0 {
		return "", fmt.Errorf("no end of the %s block", name)
	}

	var block strings.Builder

	for line := range strings.Lines(rest[:end]) {
		block.WriteString(strings.TrimPrefix(line, indent))
	}

	return block.String(), nil
}

// indented puts indent before every line that is not empty.
func indented(block, indent string) string {
	var result strings.Builder

	for line := range strings.Lines(block) {
		if strings.TrimSpace(line) != "" {
			result.WriteString(indent)
		}

		result.WriteString(line)
	}

	return result.String()
}
