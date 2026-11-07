// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_translator_aws

package config

// checkTranslatorAWSRegion rejects every value: the build tag no_translator_aws left Amazon
// Translate out.
func checkTranslatorAWSRegion(v string) error { return excludedError(v, "no_translator_aws") }
