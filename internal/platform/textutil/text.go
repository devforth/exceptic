// Package textutil holds small string helpers.
package textutil

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

// AuthCredential matches "Bearer <token>" and "Basic <token>" in text; group
// 1 is the scheme.
var AuthCredential = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`)

// TruncateRunes cuts text to at most limit characters (not bytes).
func TruncateRunes(text string, limit int) string {
	count := 0

	for index := range text {
		if count == limit {
			return text[:index]
		}

		count++
	}

	return text
}

// Shorten cuts text to at most limit characters (not bytes), ending with
// "…" when it was cut.
func Shorten(text string, limit int) string {
	if cut := TruncateRunes(text, limit); len(cut) < len(text) {
		return TruncateRunes(cut, limit-1) + "…"
	}

	return text
}

// Slug lowercases text and keeps Latin letters, digits and "_", turning every
// other run of characters into one "-" ("Web App / SSR" → "web-app-ssr");
// empty when nothing is left.
func Slug(text string) string {
	var slug strings.Builder

	for _, char := range strings.ToLower(text) {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9', char == '_':
			slug.WriteRune(char)
		case slug.Len() > 0 && !strings.HasSuffix(slug.String(), "-"):
			slug.WriteRune('-')
		}
	}

	return strings.Trim(slug.String(), "-")
}

// ContainsFold reports whether text (a string or bytes) contains part,
// ignoring the case of ASCII letters only.
func ContainsFold[Text string | []byte](text Text, part string) bool {
	if part == "" {
		return true
	}

	first := lowerASCII(part[0])

	for index := 0; index+len(part) <= len(text); index++ {
		if lowerASCII(text[index]) == first && equalFoldASCII(text[index:index+len(part)], part) {
			return true
		}
	}

	return false
}

func lowerASCII(char byte) byte {
	if 'A' <= char && char <= 'Z' {
		return char + 'a' - 'A'
	}

	return char
}

func equalFoldASCII[Text string | []byte](text Text, part string) bool {
	for index := range len(text) {
		if lowerASCII(text[index]) != lowerASCII(part[index]) {
			return false
		}
	}

	return true
}

// EmailLocalPart is the part of an address before "@": the default name of
// an email connection.
func EmailLocalPart(address string) string {
	local, _, _ := strings.Cut(address, "@")

	return local
}

// IndentJSON indents JSON for display; anything else is returned as is.
func IndentJSON(raw json.RawMessage) string {
	var buffer bytes.Buffer

	if err := json.Indent(&buffer, raw, "", "  "); err != nil {
		return string(raw)
	}

	return buffer.String()
}

// MaskSecret shows the start of a stored secret, enough to recognize it as
// Slack's own settings do, never the part that grants access: a webhook URL
// up to its last path segment (the host and the workspace and hook ids), a
// token up to the last "/" or ":" of its first half (the bot id), or the type
// and ids of a Slack token; otherwise its first 4 characters. Empty stays empty.
func MaskSecret(secret string) string {
	runes := []rune(secret)

	if len(runes) == 0 {
		return ""
	}

	if len(runes) <= 8 {
		return "…"
	}

	if link, err := url.Parse(secret); err == nil && link.Scheme != "" && link.Host != "" && strings.Contains(link.Path, "/") {
		path := link.Path[:strings.LastIndex(link.Path, "/")+1]

		return link.Scheme + "://" + link.Host + path + "…"
	}

	// The type and ids of a Slack token are not secret: Slack shows them too.
	if slackIDs := len(slackTokenIDs.FindString(secret)); slackIDs > 0 && slackIDs < len(secret) {
		return secret[:slackIDs] + "…"
	}

	prefix := 0

	// Up to the last separator in the first half, so the secret part stays hidden.
	for index, char := range runes[:len(runes)/2] {
		if char == '/' || char == ':' {
			prefix = index + 1
		}
	}

	if prefix == 0 {
		prefix = 4
	}

	return string(runes[:prefix]) + "…"
}

var slackTokenIDs = regexp.MustCompile(`^xox[a-z]-(?:[0-9]+-)*`)
