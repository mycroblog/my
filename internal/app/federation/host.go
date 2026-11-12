// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/idna"

	"github.com/mycroblog/my/internal/domain/federation"
)

// ErrInvalidHost reports a host or domain that is not a valid internationalized domain name.
var ErrInvalidHost = errors.New("federation: invalid host")

// hostProfile maps a host to IDNA 2008 A-labels with the UTS #46 non-transitional processing that
// browsers use for URL hosts (WHATWG URL "domain to ASCII"): full-width forms and upper case are
// mapped, the Bidi and ContextJ rules are checked, and label and name lengths are verified.
// Hyphens in the third and fourth positions are allowed (real hosts such as "r3---sn-x.example"
// have them), and the ASCII characters are checked by CanonicalHost instead of by STD3 rules so
// that "_" stays allowed.
var hostProfile = idna.New(
	idna.MapForLookup(),
	idna.Transitional(false),
	idna.BidiRule(),
	idna.CheckJoiners(true),
	idna.CheckHyphens(false),
	idna.StrictDomainName(false),
	idna.VerifyDNSLength(true),
)

// CanonicalHost returns the canonical form of a remote host (without port) or of the domain of a
// handle or a domain-level moderation action: IDNA A-labels in lower case, without the trailing
// dot of a fully qualified name (ADR-0006 §29). A U-label and an A-label spelling of the
// same host give the same value; an IP address gives its canonical text. It returns
// ErrInvalidHost for anything else.
func CanonicalHost(host string) (string, error) {
	h := strings.TrimSuffix(strings.TrimSpace(host), ".")
	if a, err := netip.ParseAddr(h); err == nil && a.Zone() == "" {
		return a.String(), nil
	}
	a, err := hostProfile.ToASCII(h)
	if err != nil {
		return "", fmt.Errorf("%w: %q: %w", ErrInvalidHost, host, err)
	}
	for i := 0; i < len(a); i++ {
		if !hostByte(a[i]) {
			return "", fmt.Errorf("%w: %q contains %q", ErrInvalidHost, host, a[i])
		}
	}
	return a, nil
}

// hostByte reports whether c may appear in an A-label host: a lower-case letter, a digit, "-",
// "_", or the label separator.
func hostByte(c byte) bool {
	return 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-' || c == '_' || c == '.'
}

// CanonicalIRI normalizes an IRI that comes from outside a received document (the URL to fetch,
// a keyId, a link given by a user) the same way Normalize does inside one.
func CanonicalIRI(s string) (federation.IRI, error) {
	return federation.NormalizeIRI(s, CanonicalHost)
}

// Normalize reads a received AS2 document with every IRI host in canonical form, so that
// origin checks and domain lookups compare U-label and A-label spellings as the same host.
func Normalize(raw []byte, now time.Time) (*federation.Document, error) {
	return federation.Normalize(raw, now, CanonicalHost)
}

// DisplayHost returns the form of a canonical host to show to people: its U-labels when every
// label passes the UTS #39 "highly restrictive" profile, otherwise the A-labels, so that a host
// mixing look-alike scripts (Latin "a" with Cyrillic "а") cannot pass for another one
// (ADR-0006 §29).
func DisplayHost(host string) string {
	u, err := hostProfile.ToUnicode(host)
	if err != nil {
		return host
	}
	for label := range strings.SplitSeq(u, ".") {
		if !restrictiveScripts(label) {
			return host
		}
	}
	return u
}

// restrictiveScripts reports whether the letters of label are in one script, or in one of the
// combinations UTS #39 §5.2 "highly restrictive" allows for East Asian names: Latin with Han
// and Hiragana / Katakana, Latin with Han and Bopomofo, or Latin with Han and Hangul. Common
// and Inherited characters (digits, "-", combining marks, "ー") belong to every script.
func restrictiveScripts(label string) bool {
	seen := map[string]bool{}
	for _, r := range label {
		if s := script(r); s != "" {
			seen[s] = true
		}
	}
	if len(seen) <= 1 {
		return true
	}
	for _, allowed := range allowedScriptSets {
		if subset(seen, allowed) {
			return true
		}
	}
	return false
}

var allowedScriptSets = [][]string{
	{"Latin", "Han", "Hiragana", "Katakana"},
	{"Latin", "Han", "Bopomofo"},
	{"Latin", "Han", "Hangul"},
}

func subset(seen map[string]bool, allowed []string) bool {
	n := 0
	for _, s := range allowed {
		if seen[s] {
			n++
		}
	}
	return n == len(seen)
}

// script returns the Unicode script of r, or "" for Common, Inherited, and unassigned runes.
func script(r rune) string {
	if r < 0x80 {
		if 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' {
			return "Latin"
		}
		return ""
	}
	for name, table := range unicode.Scripts {
		if name != "Common" && name != "Inherited" && unicode.Is(table, r) {
			return name
		}
	}
	return ""
}
