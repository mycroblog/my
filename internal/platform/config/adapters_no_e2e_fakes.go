// SPDX-License-Identifier: AGPL-3.0-only

//go:build !e2e_fakes

package config

// Only a test build with the build tag e2e_fakes has the adapters of e2eFakes.
func init() {
	for key, value := range e2eFakes {
		testOnly("e2e_fakes", key, value)
	}
}
