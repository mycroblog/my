// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_translator_google

package config

// checkTranslatorGoogleProject refuses every project: the build tag no_translator_google left Google Cloud Translation
// out of this build.
func checkTranslatorGoogleProject(v string) error { return excludedError(v, "no_translator_google") }

// checkTranslatorGoogleLocation accepts every value: no adapter of this build reads translator_google_location.
func checkTranslatorGoogleLocation(string) error { return nil }
