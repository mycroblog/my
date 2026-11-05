// SPDX-License-Identifier: AGPL-3.0-only

//go:build !e2e_fakes

package config

// Only a test build with the build tag e2e_fakes has the fake payment provider (ADR-0015 §6) and
// the fake search index (ADR-0149 §3).
func init() {
	testOnly("e2e_fakes", "payments", "fake")
	testOnly("e2e_fakes", "search", "fake")
}
