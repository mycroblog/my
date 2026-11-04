// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"net/url"
	"slices"
	"strings"
)

// This file reads robots.txt as RFC 9309 defines it, for the link cards (ADR-0062 §11): the
// groups of user-agent lines, the allow and disallow rules, the special characters, and the
// matching of URIs. Every other record, Crawl-delay included, is ignored.

// robotsFilePath is the path of the file itself, which is always allowed (§2.2.2).
const robotsFilePath = "/robots.txt"

// RobotsRule is one allow or disallow line. Pattern is percent-normalized as URIs are for the
// comparison, with * and a final $ as the special characters.
type RobotsRule struct {
	Allow   bool
	Pattern string
}

// RobotsRules are the rules that apply to one product token; none allows everything.
type RobotsRules []RobotsRule

// ParseRobotsTxt returns the rules that apply to token in a robots.txt: those of the groups
// whose user-agent matches it without regard to case, merged, or else those of the "*" groups.
// truncated says that body was cut at a size limit, so its last line, which may be incomplete
// and then mean something else, is not read. Lines that cannot be parsed are skipped.
func ParseRobotsTxt(body []byte, token string, truncated bool) RobotsRules {
	text := strings.TrimPrefix(string(body), "\uFEFF")
	if truncated {
		text = text[:strings.LastIndexAny(text, "\r\n")+1]
	}
	var named, star RobotsRules
	var namedFound, inNamed, inStar, afterRule bool
	for line := range strings.Lines(strings.ReplaceAll(text, "\r", "\n")) {
		key, value, ok := robotsRecord(line)
		if !ok {
			continue
		}
		switch key {
		case "user-agent":
			if afterRule {
				inNamed, inStar, afterRule = false, false, false
			}
			switch {
			case value == "*":
				inStar = true
			case strings.EqualFold(robotsProductToken(value), token):
				inNamed, namedFound = true, true
			}
		case "allow", "disallow":
			afterRule = true
			// A rule that precedes every user-agent line is in no group, and an empty path
			// matches nothing.
			if (!inNamed && !inStar) || (!strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "*")) {
				continue
			}
			r := RobotsRule{Allow: key == "allow", Pattern: normalizeRobotsPattern(value)}
			if inNamed && !slices.Contains(named, r) {
				named = append(named, r)
			}
			if inStar && !slices.Contains(star, r) {
				star = append(star, r)
			}
		}
	}
	if namedFound {
		return named
	}
	return star
}

// robotsRecord splits a line into its lowercase key and its value, without its comment and the
// white space around them. It reports false for a line without a key.
func robotsRecord(line string) (key, value string, ok bool) {
	line, _, _ = strings.Cut(line, "#")
	key, value, ok = strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value), true
}

// robotsProductToken returns the characters of a product token at the start of a user-agent
// value, so that "my-linkcard/1.0" names my-linkcard.
func robotsProductToken(value string) string {
	end := strings.IndexFunc(value, func(r rune) bool {
		return r != '-' && r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z')
	})
	if end < 0 {
		return value
	}
	return value[:end]
}

// Allowed reports whether the rules allow target, the path and query of a URI: the rule that
// matches with the most octets decides, allow over disallow when they tie, and no match allows.
// The file itself is always allowed.
func (rs RobotsRules) Allowed(target string) bool {
	if path, _, _ := strings.Cut(target, "?"); path == robotsFilePath {
		return true
	}
	target = normalizeRobotsTarget(target)
	allowed, longest := true, -1
	for _, r := range rs {
		if !r.matches(target) {
			continue
		}
		if n := len(r.Pattern); n > longest || (n == longest && r.Allow) {
			allowed, longest = r.Allow, n
		}
	}
	return allowed
}

// matches reports whether the pattern matches target from its first octet. Each * takes the
// fewest octets that let the next part match, which needs no backtracking, so a hostile pattern
// costs no more than the parts and the target together.
func (r RobotsRule) matches(target string) bool {
	pattern, anchored := strings.CutSuffix(r.Pattern, "$")
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(target, parts[0]) {
		return false
	}
	pos := len(parts[0])
	last := len(parts) - 1
	if last == 0 {
		return !anchored || pos == len(target)
	}
	for _, part := range parts[1:last] {
		i := strings.Index(target[pos:], part)
		if i < 0 {
			return false
		}
		pos += i + len(part)
	}
	if anchored {
		return pos <= len(target)-len(parts[last]) && strings.HasSuffix(target, parts[last])
	}
	return strings.Contains(target[pos:], parts[last])
}

// RobotsOrigin returns the origin of an http or https URL (lowercase scheme and host, the port
// unless it is the scheme's default), where its robots.txt lives, and its target: the path and
// query that the rules match. It reports false for any other URL.
func RobotsOrigin(rawURL string) (origin, target string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", "", false
	}
	host := strings.ToLower(u.Host)
	if (scheme == "https" && u.Port() == "443") || (scheme == "http" && u.Port() == "80") {
		host = strings.TrimSuffix(host, ":"+u.Port())
	}
	target = u.EscapedPath()
	if target == "" {
		target = "/"
	}
	if u.RawQuery != "" {
		target += "?" + u.RawQuery
	}
	return scheme + "://" + host, target, true
}

// RobotsFileURL returns the URL of the robots.txt of an origin that RobotsOrigin returned.
func RobotsFileURL(origin string) string { return origin + robotsFilePath }

// normalizeRobotsPattern normalizes the path of a rule: * stays the wildcard, and $ stays the
// end of the pattern only as its last character (§2.2.3).
func normalizeRobotsPattern(p string) string {
	body, anchored := strings.CutSuffix(p, "$")
	out := normalizeRobotsOctets(body, "$")
	if anchored {
		out += "$"
	}
	return out
}

// normalizeRobotsTarget normalizes the path and query of a URI: the * and $ in it are literal,
// so they compare as the %2A and %24 of a pattern (§2.2.3).
func normalizeRobotsTarget(t string) string { return normalizeRobotsOctets(t, "*$") }

// normalizeRobotsOctets puts a path and query in the form that §2.2.2 compares: octets outside
// ASCII, the delimiters inside a query, and the literal characters percent-encoded, the escapes
// of unreserved characters decoded, and the other escapes in uppercase.
func normalizeRobotsOctets(s, literal string) string {
	const hex = "0123456789ABCDEF"
	escape := func(b *strings.Builder, c byte) {
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	var b strings.Builder
	b.Grow(len(s))
	inQuery := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '%' && i+2 < len(s) && isHexDigit(s[i+1]) && isHexDigit(s[i+2]):
			v := unhex(s[i+1])<<4 | unhex(s[i+2])
			i += 2
			if isRobotsUnreserved(v) {
				b.WriteByte(v)
			} else {
				escape(&b, v)
			}
		case c == '?' && !inQuery:
			inQuery = true
			b.WriteByte(c)
		case c <= ' ' || c >= 0x7f,
			inQuery && strings.IndexByte(":/?#[]@", c) >= 0,
			strings.IndexByte(literal, c) >= 0:
			escape(&b, c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func isRobotsUnreserved(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' || c == '~'
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func unhex(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}
