// SPDX-License-Identifier: MIT

// Package integration holds the integration tests of pkg/mybot (ADR-0094 §10): an echo bot runs against a server
// that internal/platform/platformtest runs inside the test process and answers each mention once, also when its
// event stream is cut and it catches up, and when it is stopped in the middle of a handler and started again; and
// the answers of the fake server of mybottest are checked against api/openapi.yaml. It is a module of its own that
// is never published, and has no code outside its tests.
package integration
