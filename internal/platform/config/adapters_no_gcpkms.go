// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_gcpkms

package config

func init() { exclude("no_gcpkms", "key_manager", "gcpkms") }

// checkGCPKMSKeyName rejects every Cloud KMS key: this build has no adapter that could decrypt
// with it.
func checkGCPKMSKeyName(string) error { return excludedError("gcpkms", "no_gcpkms") }
