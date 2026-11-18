// SPDX-License-Identifier: AGPL-3.0-only

//go:build !e2e_fakes

package config

// Only a test build with the build tag e2e_fakes has the fake payment provider and the fake mailer
// (ADR-0015 §6), the fake search index (ADR-0149 §3), and the fake translation provider
// (ADR-0148).
func init() {
	testOnly("e2e_fakes", "mail", "fake")
	testOnly("e2e_fakes", "payments", "fake")
	testOnly("e2e_fakes", "search", "fake")
	testOnly("e2e_fakes", "translator_fake", "true")
}
