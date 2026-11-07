// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_translator_google

package config

import translatorgoogle "github.com/mycroblog/my/internal/adapter/translator/google"

func checkTranslatorGoogleProject(v string) error { return translatorgoogle.CheckProject(v) }

func checkTranslatorGoogleLocation(v string) error { return translatorgoogle.CheckLocation(v) }
