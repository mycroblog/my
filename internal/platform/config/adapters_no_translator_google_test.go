// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_translator_google

package config

import (
	"errors"
	"strings"
	"testing"
)

func TestNoTranslatorGoogleRejectsTheProject(t *testing.T) {
	_, _, err := Load(source(with(minimal(), "MY_TRANSLATOR_GOOGLE_PROJECT", "my-project"), nil))
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Issues) != 1 || ve.Issues[0].Key != "translator_google_project" || !strings.Contains(ve.Issues[0].Message, "no_translator_google") {
		t.Fatalf("err = %v", err)
	}
}
