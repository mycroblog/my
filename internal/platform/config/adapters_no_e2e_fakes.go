// SPDX-License-Identifier: AGPL-3.0-only

//go:build !e2e_fakes

package config

// Only a test build with the build tag e2e_fakes has the fake payment provider (ADR-0117 §2).
func init() { testOnly("e2e_fakes", "payments_provider", "fake") }
