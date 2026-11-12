// SPDX-License-Identifier: AGPL-3.0-only

// Package instance holds the use cases and port definitions of the instance module: instance settings, terms, rules, and NodeInfo.
// It is the module's public API; other modules reach the instance module only through this package.
//
// The terms of service and the privacy policy are Markdown documents, kept as written and
// rendered and sanitized on every read (DocumentRenderer), as an announcement's body is; no HTML
// is stored. Without a document, the document API answers 404, and neither the registration
// screen, the footers, nor the instance's terms_url and privacy_policy_url point at it.
package instance
