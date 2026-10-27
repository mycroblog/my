// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ResponseTTL returns how long a response with header h may be reused from now (RFC 9111
// §4.2.1): zero for no-store, no-cache, or a malformed directive; s-maxage, max-age, or Expires
// (against Date) when present; def otherwise. It is at most maxTTL.
func ResponseTTL(h http.Header, now time.Time, def, maxTTL time.Duration) time.Duration {
	maxAge, sharedMaxAge := -1, -1
	for _, line := range h.Values("Cache-Control") {
		for directive := range strings.SplitSeq(line, ",") {
			name, value, _ := strings.Cut(strings.TrimSpace(directive), "=")
			value = strings.Trim(strings.TrimSpace(value), `"`)
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "no-store", "no-cache":
				return 0
			case "max-age":
				if maxAge = deltaSeconds(value, maxTTL); maxAge < 0 {
					return 0
				}
			case "s-maxage":
				if sharedMaxAge = deltaSeconds(value, maxTTL); sharedMaxAge < 0 {
					return 0
				}
			}
		}
	}
	var ttl time.Duration
	switch {
	case sharedMaxAge >= 0:
		ttl = time.Duration(sharedMaxAge) * time.Second
	case maxAge >= 0:
		ttl = time.Duration(maxAge) * time.Second
	case h.Get("Expires") != "":
		expires, err := http.ParseTime(h.Get("Expires"))
		if err != nil {
			return 0
		}
		base := now
		if date, err := http.ParseTime(h.Get("Date")); err == nil {
			base = date
		}
		ttl = expires.Sub(base)
	default:
		ttl = def
	}
	return min(max(ttl, 0), maxTTL)
}

// deltaSeconds parses a delta-seconds value; -1 when it is not one. A value too large to hold
// is capped at maxTTL, as RFC 9111 §1.2.2 allows.
func deltaSeconds(v string, maxTTL time.Duration) int {
	if v == "" || strings.Trim(v, "0123456789") != "" {
		return -1
	}
	n, err := strconv.Atoi(v)
	if err != nil || n > int(maxTTL/time.Second) {
		return int(maxTTL / time.Second)
	}
	return n
}
