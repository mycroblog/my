// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_translator_aws

package config

func checkTranslatorAWSRegion(v string) error { return checkAWSRegion(v) }
