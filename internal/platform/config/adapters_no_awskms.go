// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_awskms

package config

func init() { exclude("no_awskms", "key_manager", "awskms") }

// checkAWSKMSKeyName rejects every AWS KMS key: this build has no adapter that could decrypt
// with it.
func checkAWSKMSKeyName(string) error { return excludedError("awskms", "no_awskms") }
