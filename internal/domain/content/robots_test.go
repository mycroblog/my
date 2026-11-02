// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/domain/content"
)

const robotsToken = "my-linkcard"

func robotsAllowed(t *testing.T, file, target string) bool {
	t.Helper()
	return content.ParseRobotsTxt([]byte(file), robotsToken, false).Allowed(target)
}

func TestRobotsGroups(t *testing.T) {
	t.Parallel()
	const merged = "User-agent: my-linkcard\nDisallow: /foo\n\nUser-agent: otherbot\nDisallow: /other\n\nUser-agent: my-linkcard\nDisallow: /baz\n"
	const fallback = "User-agent: *\nDisallow: /foo\n\nUser-agent: BazBot\nDisallow: /baz\n"
	tests := []struct {
		name, file, target string
		want               bool
	}{
		{"an empty file", "", "/a", true},
		{"no group applies", "User-agent: otherbot\nDisallow: /\n", "/a", true},
		{"the token without regard to case", "USER-AGENT: My-LinkCard\nDISALLOW: /\n", "/a", false},
		{"the star group as the fallback", fallback, "/foo", false},
		{"another bot's group is not read", fallback, "/baz", true},
		{"a matching group replaces the star group", "User-agent: *\nDisallow: /\n\nUser-agent: my-linkcard\nDisallow: /private\n", "/a", true},
		{"a matching group replaces the star group, in any order", "User-agent: my-linkcard\nAllow: /\n\nUser-agent: *\nDisallow: /\n", "/a", true},
		{"an empty matching group allows everything", "User-agent: *\nDisallow: /\n\nUser-agent: my-linkcard\n", "/a", true},
		{"matching groups merge: the first", merged, "/foo", false},
		{"matching groups merge: the second", merged, "/baz", false},
		{"matching groups merge: neither", merged, "/other", true},
		{"consecutive user-agent lines share a group", "User-agent: barbot\nUser-agent: my-linkcard\nDisallow: /a\n", "/a", false},
		{"a user-agent line after a rule starts a group", "User-agent: my-linkcard\nDisallow: /a\nUser-agent: barbot\nDisallow: /b\n", "/b", true},
		{"a rule outside every group is ignored", "Disallow: /a\nUser-agent: *\nDisallow: /b\n", "/a", true},
		{"another record does not end a group", "User-agent: my-linkcard\nSitemap: https://a.example/s.xml\nCrawl-delay: 10\nDisallow: /a\n", "/a", false},
		{"another record between user-agent lines", "User-agent: barbot\nSitemap: https://a.example/s.xml\nUser-agent: my-linkcard\nDisallow: /a\n", "/a", false},
		{"a version after the token", "User-agent: my-linkcard/1.0\nDisallow: /\n", "/a", false},
		{"a longer name is another bot", "User-agent: my-linkcard-extra\nDisallow: /\n", "/a", true},
		{"the name my is another bot", "User-agent: my\nDisallow: /\n", "/a", true},
		{"no space after the colon", "User-Agent:my-linkcard\nDisallow:/\n", "/a", false},
		{"space before the colon", "User-Agent : my-linkcard\nDisallow : /\n", "/a", false},
		{"a comment after a value", "User-agent: my-linkcard # us\nDisallow: /a # not /a/b\n", "/a", false},
		{"a comment on its own line", "# Disallow: /\nUser-agent: *\n# Disallow: /\nAllow: /x\n", "/a", true},
		{"CR line ends", "User-agent: my-linkcard\rDisallow: /a\r", "/a", false},
		{"CRLF line ends", "User-agent: my-linkcard\r\nDisallow: /a\r\n", "/a", false},
		{"a byte order mark", "\uFEFFUser-agent: my-linkcard\nDisallow: /a\n", "/a", false},
		{"an empty disallow matches nothing", "User-agent: *\nDisallow:\n", "/a", true},
		{"a line that cannot be parsed is skipped", "User-agent: *\nthis is not a record\nDisallow /a\nDisallow: /b\n", "/a", true},
		{"the rules after a line that cannot be parsed are used", "User-agent: *\nthis is not a record\nDisallow: /b\n", "/b", false},
		{"a path that does not start with a slash is skipped", "User-agent: *\nDisallow: private\n", "/private", true},
		{"the file itself is always allowed", "User-agent: *\nDisallow: /\n", "/robots.txt", true},
		{"the file with a query is always allowed", "User-agent: *\nDisallow: /\n", "/robots.txt?x=1", true},
		{"another file with that prefix is not", "User-agent: *\nDisallow: /\n", "/robots.txt.bak", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := robotsAllowed(t, tt.file, tt.target); got != tt.want {
				t.Errorf("Allowed(%q) = %v, want %v, in:\n%s", tt.target, got, tt.want, tt.file)
			}
		})
	}
}

func TestRobotsMatching(t *testing.T) {
	t.Parallel()
	const simple = "Disallow: *.gif$\nDisallow: /example/\nAllow: /publications/"
	const foobot = "Disallow:/\nAllow:/example/page.html\nAllow:/example/allowed.gif"
	const longest = "Allow: /example/page/\nDisallow: /example/page/disallowed.gif"
	tests := []struct {
		name, rules, target string
		want                bool
	}{
		// RFC 9309 §5.1.
		{"a suffix", simple, "/a/b.gif", false},
		{"a query is not the end", simple, "/a/b.gif?x=1", true},
		{"a prefix", simple, "/example/a", false},
		{"an allow", simple, "/publications/a", true},
		{"an allow under a full disallow", foobot, "/example/page.html", true},
		{"a full disallow", foobot, "/example/other.html", false},
		// RFC 9309 §5.2.
		{"the longest match: disallow", longest, "/example/page/disallowed.gif", false},
		{"the longest match: allow", longest, "/example/page/other.gif", true},
		{"the longest match, in the other order", "Disallow: /example/page/disallowed.gif\nAllow: /example/page/", "/example/page/disallowed.gif", false},
		{"allow wins a tie", "Disallow: /a\nAllow: /a", "/a", true},
		{"allow wins a tie, in the other order", "Allow: /a\nDisallow: /a", "/a", true},
		{"a tie of different patterns", "Disallow: /a*\nAllow: /ab", "/ab", true},
		{"matching starts at the first octet", "Disallow: /b", "/a/b", true},
		{"matching is case sensitive", "Disallow: /A", "/a", true},
		{"a prefix of a segment matches", "Disallow: /a", "/abc", false},
		{"a pattern longer than the path", "Disallow: /abc", "/a", true},
		{"the wildcard", "Disallow: /this/*/exactly", "/this/one/exactly", false},
		{"the wildcard takes zero characters", "Disallow: /a*b", "/ab", false},
		{"the wildcard crosses slashes", "Disallow: /a*z", "/a/b/c/z", false},
		{"the wildcard needs the rest", "Disallow: /a*z", "/a/b", true},
		{"several wildcards", "Disallow: /a*b*c", "/a-b-b-c", false},
		{"several wildcards out of order", "Disallow: /a*b*c", "/a-c-b", true},
		{"a trailing wildcard", "Disallow: /a*", "/a", false},
		{"the end anchor", "Disallow: /this/path/exactly$", "/this/path/exactly", false},
		{"the end anchor refuses more", "Disallow: /this/path/exactly$", "/this/path/exactly/more", true},
		{"a wildcard and the end anchor cannot overlap", "Disallow: /a*a$", "/a", true},
		{"a wildcard and the end anchor take the last match", "Disallow: /*.gif$", "/a.gif.gif", false},
		{"a dollar inside a pattern is literal", "Disallow: /a$b", "/a$b", false},
		{"a dollar inside a pattern does not anchor", "Disallow: /a$b", "/a", true},
		{"a query is part of the target", "Disallow: /a?b=1", "/a?b=1", false},
		{"another query", "Disallow: /a?b=1", "/a?b=2", true},
		{"a wildcard before the query", "Disallow: /*?session=", "/a?session=1", false},
		// RFC 9309 Figures 4 and 6.
		{"a query with delimiters, encoded in the target", "Disallow: /foo/bar?baz=https://foo.bar", "/foo/bar?baz=https%3A%2F%2Ffoo.bar", false},
		{"a query with delimiters, encoded in the rule", "Disallow: /foo/bar?baz=https%3A%2F%2Ffoo.bar", "/foo/bar?baz=https://foo.bar", false},
		{"non-ASCII in the rule", "Disallow: /foo/bar/ツ", "/foo/bar/%E3%83%84", false},
		{"lowercase escapes", "Disallow: /foo/bar/%e3%83%84", "/foo/bar/%E3%83%84", false},
		{"an escaped unreserved character is decoded", "Disallow: /foo/bar/baz", "/foo/bar/%62%61%7A", false},
		{"an escaped slash stays escaped", "Disallow: /a/b", "/a%2Fb", true},
		{"an encoded star in a pattern is literal", "Disallow: /path/file-with-a-%2A.html", "/path/file-with-a-*.html", false},
		{"an encoded star does not match others", "Disallow: /path/file-with-a-%2A.html", "/path/file-with-a-b.html", true},
		{"an encoded dollar in a pattern is literal", "Disallow: /path/foo-%24", "/path/foo-$", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := robotsAllowed(t, "User-agent: my-linkcard\n"+tt.rules+"\n", tt.target); got != tt.want {
				t.Errorf("Allowed(%q) = %v, want %v, with:\n%s", tt.target, got, tt.want, tt.rules)
			}
		})
	}
}

// A file cut at the limit is read up to its last whole line: the cut line could mean something
// broader than what the site wrote.
func TestRobotsTruncated(t *testing.T) {
	t.Parallel()
	cut := []byte("User-agent: *\nDisallow: /a\nDisallow: /secret")
	rules := content.ParseRobotsTxt(cut, robotsToken, true)
	if rules.Allowed("/a") {
		t.Error("a whole line before the cut is not read")
	}
	if !rules.Allowed("/secret") {
		t.Error("the line that was cut is read")
	}
	if content.ParseRobotsTxt(cut, robotsToken, false).Allowed("/secret") {
		t.Error("the last line of a whole file is not read")
	}
	if got := content.ParseRobotsTxt([]byte("User-agent: *"), robotsToken, true); len(got) != 0 {
		t.Errorf("a file cut in its first line = %+v", got)
	}
}

// A hostile pattern costs no more than its parts and the target.
func TestRobotsHostilePattern(t *testing.T) {
	t.Parallel()
	pattern := "/" + strings.Repeat("a*", 5000) + "b"
	if !robotsAllowed(t, "User-agent: *\nDisallow: "+pattern+"\n", "/"+strings.Repeat("a", 2000)) {
		t.Error("the pattern matched without its last character")
	}
}

func TestRobotsOrigin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, origin, target string
		ok                 bool
	}{
		{"https://Example.COM/a?b=1#c", "https://example.com", "/a?b=1", true},
		{"https://example.com", "https://example.com", "/", true},
		{"https://example.com:443/a", "https://example.com", "/a", true},
		{"http://example.com:80/a", "http://example.com", "/a", true},
		{"https://example.com:8443/a", "https://example.com:8443", "/a", true},
		{"http://example.com:443/a", "http://example.com:443", "/a", true},
		{"https://[2001:db8::1]:443/a", "https://[2001:db8::1]", "/a", true},
		{"https://a.example/%E3%83%84", "https://a.example", "/%E3%83%84", true},
		{"ftp://example.com/a", "", "", false},
		{"/a", "", "", false},
		{"https://bad host/", "", "", false},
	}
	for _, tt := range tests {
		origin, target, ok := content.RobotsOrigin(tt.in)
		if origin != tt.origin || target != tt.target || ok != tt.ok {
			t.Errorf("RobotsOrigin(%q) = %q, %q, %v", tt.in, origin, target, ok)
		}
	}
	if got := content.RobotsFileURL("https://example.com:8443"); got != "https://example.com:8443/robots.txt" {
		t.Errorf("RobotsFileURL = %q", got)
	}
}
