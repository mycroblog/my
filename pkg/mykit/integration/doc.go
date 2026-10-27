// SPDX-License-Identifier: MIT

// Package integration holds the integration tests of pkg/mykit (ADR-0093 §8): the Go client drives a server that
// internal/platform/platformtest runs inside the test process, through the main flows of the client API (posting,
// paging the home timeline, the stream, media uploads, the retry after a 429, and the retry of a post whose answer
// was lost), so that the spec, the generated client, and the hand-written layer are checked against each other. It
// is a module of its own that is never published, and has no code outside its tests.
package integration
