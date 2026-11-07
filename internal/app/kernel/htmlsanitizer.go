// SPDX-License-Identifier: AGPL-3.0-only

package kernel

// HTMLSanitizer cleans HTML that arrives from other servers (ADR-0019 §2, federation.md §5).
// The adapter lives in internal/adapter/htmlsanitizer and passes contenttest.HTMLSanitizerContract.
//
// Remote HTML is sanitized once, when it is ingested, and stored together with the
// PolicyVersion that produced it, so that stored HTML can be sanitized again after the policy
// changes. The output of SanitizeHTML is the only remote HTML that reaches the web client's
// my-server-sanitized-html Trusted Types policy (web-client/README.md).
type HTMLSanitizer interface {
	// SanitizeHTML returns the HTML fragment s reduced to the allowlist of federation.md §5.
	// HTML that local statuses are rendered as (internal/app/content, text.go) is returned
	// unchanged, and sanitizing an output again returns it unchanged.
	SanitizeHTML(s string) string
	// PlainText returns the text of a field that allows no HTML (display names, usernames,
	// poll options, alternative text): tags are removed, the contents of elements that never
	// carry text (script, style, ...) are dropped, and character references are decoded.
	// The result is text, not HTML; it is escaped wherever it is rendered.
	PlainText(s string) string
	// PolicyVersion is the version of the policy SanitizeHTML applies. It increases whenever
	// the policy changes in a way that changes outputs.
	PolicyVersion() int
}
