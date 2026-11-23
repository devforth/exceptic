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

// ReplaceServiceTokens replaces each token chat services put into URL paths,
// with the ids before it, by replace(path), path being what leads to it: a
// Telegram bot token ("bot" + id:token, or "" for id:token alone, the id of
// 6 digits or more, the token of 30 characters or more), a Slack webhook
// ("/services/" + T…/B…/token) or a Discord one ("/api/webhooks/" or
// "/api/v10/webhooks/" + id/token). HTTP clients print paths without a
// scheme, so tokens are looked for anywhere in text, in one pass.
func ReplaceServiceTokens(text string, replace func(path string) string) string {
	var output strings.Builder

	copied := 0

	for index := 0; index < len(text); {
		path, end := serviceTokenAt(text, index)

		if end < 0 {
			index++

			continue
		}

		output.WriteString(text[copied:index])
		output.WriteString(replace(path))

		copied, index = end, end
	}

	if copied == 0 {
		return text
	}

	return output.String() + text[copied:]
}

// serviceTokenAt reads the token that starts at index: the path before it
// and where it ends, or -1 when none does.
func serviceTokenAt(text string, index int) (path string, end int) {
	rest := text[index:]

	switch {
	case strings.HasPrefix(rest, "bot"):
		if end := botToken(rest[len("bot"):], 1, 1); end >= 0 {
			return "bot", index + len("bot") + end
		}
	case isDigit(rest[0]) && (index == 0 || !isDigit(text[index-1])):
		if end := botToken(rest, 6, 30); end >= 0 {
			return "", index + end
		}
	case strings.HasPrefix(rest, "/services/T"):
		after := rest[len("/services/"):]

		if team := runOf(after[1:], isUpperOrDigit); team > 0 && strings.HasPrefix(after[1+team:], "/B") {
			after = after[1+team+len("/B"):]

			if channel := runOf(after, isUpperOrDigit); channel > 0 && strings.HasPrefix(after[channel:], "/") {
				if token := runOf(after[channel+1:], isTokenByte); token > 0 {
					return "/services/", len(text) - len(after) + channel + 1 + token
				}
			}
		}
	case strings.HasPrefix(rest, "/api/"):
		path := "/api/"

		if version := rest[len(path):]; strings.HasPrefix(version, "v") {
			if digits := runOf(version[1:], isDigit); digits > 0 && strings.HasPrefix(version[1+digits:], "/") {
				path = rest[:len("/api/v")+digits+1]
			}
		}

		if !strings.HasPrefix(rest[len(path):], "webhooks/") {
			return "", -1
		}

		path += "webhooks/"
		after := rest[len(path):]

		if id := runOf(after, isDigit); id > 0 && strings.HasPrefix(after[id:], "/") {
			if token := runOf(after[id+1:], isTokenByte); token > 0 {
				return path, len(text) - len(after) + id + 1 + token
			}
		}
	}

	return "", -1
}

// botToken returns the length of id:token at the start of text (the ':'
// may come escaped as %3A), with at least minID digits and minToken
// characters, or -1.
func botToken(text string, minID, minToken int) int {
	id := runOf(text, isDigit)

	if id < minID {
		return -1
	}

	separator := 0

	switch {
	case strings.HasPrefix(text[id:], ":"):
		separator = 1
	case len(text) >= id+3 && strings.EqualFold(text[id:id+3], "%3a"):
		separator = 3
	default:
		return -1
	}

	if token := runOf(text[id+separator:], isTokenByte); token >= minToken {
		return id + separator + token
	}

	return -1
}

// runOf returns how many bytes at the start of text pass is.
func runOf(text string, is func(byte) bool) int {
	length := 0

	for length < len(text) && is(text[length]) {
		length++
	}

	return length
}

func isDigit(char byte) bool {
	return char >= '0' && char <= '9'
}

func isUpperOrDigit(char byte) bool {
	return char >= 'A' && char <= 'Z' || isDigit(char)
}

func isTokenByte(char byte) bool {
	return char >= 'a' && char <= 'z' || isUpperOrDigit(char) || char == '_' || char == '-'
}

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
