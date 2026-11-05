// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// minOriginSecretBytes bounds origin_secret, which anyone on the internet may guess at.
const minOriginSecretBytes = 32

// checkOriginSecret validates origin_secret and origin_secret_previous. The messages never hold
// the value. Spaces are refused since the HTTP server trims them from a header's value.
func checkOriginSecret(v string) error {
	if len(v) < minOriginSecretBytes {
		return fmt.Errorf("must be at least %d bytes; generate one with openssl rand -hex 32", minOriginSecretBytes)
	}
	for i := range len(v) {
		if v[i] <= ' ' || v[i] >= 0x7f {
			return errors.New("must be visible ASCII without spaces, to fit in a request header; generate one with openssl rand -hex 32")
		}
	}
	return nil
}

// The names that origin_secret_header must not take: a CDN refuses to set them (Cloudflare the
// cf- and x-cf- ones, the protocol's own, and those that tell the visitor's IP address or
// protocol; CloudFront the x-amz- and x-edge- ones and much the same others), or the server, a
// proxy, or a platform reads them, while the server removes the header that this setting names.
var (
	forbiddenOriginHeaders = []string{
		"accept", "accept-charset", "accept-encoding", "accept-language", "authorization", "cache-control",
		"connection", "content-digest", "content-encoding", "content-length", "content-type", "cookie", "cookie2",
		"date", "digest", "dnt", "expect", "forwarded", "host", "keep-alive", "max-forwards", "origin", "pragma",
		"range", "referer", "request-range", "set-cookie", "signature", "signature-input", "te", "traceparent",
		"tracestate", "trailer", "transfer-encoding", "true-client-ip", "upgrade", "user-agent", "via",
		"x-cloud-trace-context", "x-real-ip", "x-request-id",
	}
	forbiddenOriginHeaderPrefixes = []string{
		"access-control-", "cf-", "if-", "proxy-", "sec-", "x-amz-", "x-amzn-", "x-cf-", "x-edge-", "x-forwarded-", "x-goog-",
	}
)

// checkOriginSecretHeader validates origin_secret_header.
func checkOriginSecretHeader(v string) error {
	for i := range len(v) {
		c := v[i]
		letterOrDigit := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if !letterOrDigit && (c != '-' || i == 0 || i == len(v)-1) {
			return errors.New("must be letters, digits, and inner hyphens, which every proxy passes")
		}
	}
	name := strings.ToLower(v)
	if slices.Contains(forbiddenOriginHeaders, name) ||
		slices.ContainsFunc(forbiddenOriginHeaderPrefixes, func(p string) bool { return strings.HasPrefix(name, p) }) {
		return fmt.Errorf("%s is a header that a CDN refuses to set or that the server or a proxy reads for itself; choose your own name", v)
	}
	return nil
}

// originChecks rejects the combinations of the origin_secret settings that would check nothing
// or refuse everyone. The maintenance mode reads these settings too, and runs no crossChecks.
func originChecks(vals map[string]string, present map[string]bool) []Issue {
	var issues []Issue
	if present["origin_secret"] && !present["origin_secret_header"] {
		issues = append(issues, Issue{Key: "origin_secret_header", Missing: true, Message: "required when origin_secret is set"})
	}
	if !present["origin_secret"] {
		for _, key := range []string{"origin_secret_previous", "origin_secret_header"} {
			if present[key] {
				issues = append(issues, Issue{Key: key, Message: "has no effect without origin_secret, so the server would check nothing; set origin_secret too or remove this"})
			}
		}
	}
	if s := vals["origin_secret"]; s != "" && s == vals["origin_secret_previous"] {
		issues = append(issues, Issue{Key: "origin_secret_previous", Message: "must differ from origin_secret"})
	}
	return issues
}
