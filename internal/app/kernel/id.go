// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import "regexp"

// idPattern matches the canonical form of a UUIDv7: 36 lowercase characters.
var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// ValidID reports whether s is the canonical form of an id this server generates: a UUIDv7 in
// 36 lowercase characters (ADR-0003 §9). PostgreSQL's uuid type also accepts uppercase,
// hyphen-less and braced forms as the same value, so every id that comes from outside (a request
// path, query or body, a cursor, a URI in a received document) goes through this check before it
// reaches an adapter. It is the only place that decides the form.
func ValidID(s string) bool { return idPattern.MatchString(s) }
