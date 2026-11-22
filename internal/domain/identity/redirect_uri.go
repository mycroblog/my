// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// ErrInvalidRedirectURI is returned by ValidateRedirectURI for a redirect URI that a client may
// not register. ErrRedirectURIForm and ErrRedirectURIFragment wrap it for the rule that the URI
// breaks; a URI that is empty, padded, unparsable, or with userinfo or a backslash is
// ErrInvalidRedirectURI alone.
var ErrInvalidRedirectURI = errors.New("identity: invalid redirect URI")

var (
	// ErrRedirectURIForm reports a redirect URI that is not one of the three forms: an https
	// URL, a private-use URI scheme in reverse domain notation, or an http loopback URL.
	ErrRedirectURIForm = fmt.Errorf("%w: not https, a private-use scheme, or http on a loopback address", ErrInvalidRedirectURI)
	// ErrRedirectURIFragment reports a redirect URI with a fragment.
	ErrRedirectURIFragment = fmt.Errorf("%w: with a fragment", ErrInvalidRedirectURI)
)

// deniedRedirectSchemes are schemes that never make a redirect URI, even though they look like
// a private-use scheme (ADR-0090 §3).
var deniedRedirectSchemes = []string{"http", "javascript", "data", "file", "vbscript", "blob", "about", "ftp", "ws", "wss", "urn"}

// ValidateRedirectURI checks a redirect URI of a client by the rules of RFC 8252 (ADR-0090 §3):
// an https URL (which includes a claimed https URL), a private-use URI scheme in reverse
// domain notation, or an http loopback URL on 127.0.0.1 or [::1] ("localhost" is refused).
// Fragments, userinfo, and the out-of-band URN are refused for every form.
func ValidateRedirectURI(raw string) error {
	bad := func(rule error, why string) error { return fmt.Errorf("%w: %q: %s", rule, raw, why) }
	if strings.ContainsRune(raw, '#') {
		return bad(ErrRedirectURIFragment, "a fragment is not allowed")
	}
	if raw == "" || strings.TrimSpace(raw) != raw || strings.ContainsRune(raw, '\\') {
		return bad(ErrInvalidRedirectURI, "empty, padded, or with a backslash")
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return bad(ErrInvalidRedirectURI, "not a valid URL without userinfo")
	}
	scheme := strings.ToLower(u.Scheme)
	switch {
	case scheme == "https":
		if u.Hostname() == "" {
			return bad(ErrRedirectURIForm, "no host")
		}
	case scheme == "http":
		if !isLoopbackIP(u.Hostname()) {
			return bad(ErrRedirectURIForm, "http is for 127.0.0.1 and [::1] only")
		}
	case slices.Contains(deniedRedirectSchemes, scheme):
		return bad(ErrRedirectURIForm, "scheme is not allowed")
	case !validPrivateUseScheme(scheme):
		return bad(ErrRedirectURIForm, "a private-use scheme must be in reverse domain notation")
	}
	return nil
}

// validPrivateUseScheme reports whether s is a reverse domain name such as "com.example.app":
// at least two dot-separated labels of lowercase letters, digits, and hyphens.
func validPrivateUseScheme(s string) bool {
	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if l == "" || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
		for _, c := range l {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return true
}

func isLoopbackIP(host string) bool { return host == "127.0.0.1" || host == "::1" }

// MatchRedirectURI reports whether the requested redirect URI matches the registered one.
// The match is exact, except that a registered loopback URI (http://127.0.0.1 or http://[::1])
// without a port matches a request with any port (RFC 8252 §7.3); scheme, host, path, and
// query must still agree.
func MatchRedirectURI(registered, requested string) bool {
	if registered == requested {
		return registered != ""
	}
	r, err := url.Parse(registered)
	if err != nil || r.Scheme != "http" || !isLoopbackIP(r.Hostname()) || r.Port() != "" {
		return false
	}
	q, err := url.Parse(requested)
	if err != nil || q.Scheme != "http" || q.Hostname() != r.Hostname() || q.User != nil || q.Fragment != "" {
		return false
	}
	if p := q.Port(); p != "" {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return q.EscapedPath() == r.EscapedPath() && q.RawQuery == r.RawQuery
}
