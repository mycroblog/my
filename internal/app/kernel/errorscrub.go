// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"net"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The placeholders of ScrubErrorMessage, in the order they apply: a URL holds the other forms, and
// @user@host holds an email address.
var messagePlaceholders = []struct {
	re          *regexp.Regexp
	placeholder string
	valid       func(string) bool
}{
	// A URL does not end with the punctuation of the sentence around it.
	{re: regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s"'<>()\[\]{}]*[^\s"'<>()\[\]{}.,;:!?]`), placeholder: "<url>"},
	{re: regexp.MustCompile(`@[A-Za-z0-9_.+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*`), placeholder: "<acct>"},
	{re: regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+`), placeholder: "<email>"},
	{re: regexp.MustCompile(`\b[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\b`), placeholder: "<id>"},
	{re: regexp.MustCompile(`\b[0-7][0-9A-HJKMNP-TV-Z]{25}\b`), placeholder: "<id>"},
	{re: regexp.MustCompile(`(?:[0-9A-Fa-f]{0,4}:){2,7}[0-9A-Fa-f]{0,4}(?:%[0-9A-Za-z]+)?|\b(?:\d{1,3}\.){3}\d{1,3}\b`), placeholder: "<ip>", valid: isIP},
	{re: regexp.MustCompile(`\d{10,}`), placeholder: "<n>"},
}

// isIP reports whether s, found by the pattern of the IP addresses, is one: the pattern of IPv6
// also finds times such as 12:30:45.
func isIP(s string) bool {
	if i := strings.IndexByte(s, '%'); i >= 0 {
		s = s[:i]
	}
	return net.ParseIP(s) != nil
}

// MaxErrorMessageBytes is the length to which ScrubErrorMessage cuts a message.
const MaxErrorMessageBytes = 1 << 10

// ScrubErrorMessage replaces the personal and unbounded parts of an error's text with
// placeholders (URLs, email addresses, @user@host, IP addresses, UUIDs and ULIDs, numbers of 10
// digits or more) and cuts it at 1 KiB without splitting a character (ADR-0103 §3). The
// ErrorReporter applies it to every ErrorEvent; the result is also the normalized message that a
// fingerprint is made of, such as the one of a web client error.
func ScrubErrorMessage(s string) string {
	for _, p := range messagePlaceholders {
		if p.valid == nil {
			s = p.re.ReplaceAllLiteralString(s, p.placeholder)
			continue
		}
		s = p.re.ReplaceAllStringFunc(s, func(m string) string {
			if p.valid(m) {
				return p.placeholder
			}
			return m
		})
	}
	if len(s) <= MaxErrorMessageBytes {
		return s
	}
	n := MaxErrorMessageBytes
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
