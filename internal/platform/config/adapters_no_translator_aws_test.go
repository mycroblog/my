// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_translator_aws

package config

import (
	"errors"
	"strings"
	"testing"
)

func TestNoTranslatorAWSRejectsTheRegion(t *testing.T) {
	_, _, err := Load(source(with(minimal(), "MY_TRANSLATOR_AWS_REGION", "us-east-1"), nil))
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Issues) != 1 || ve.Issues[0].Key != "translator_aws_region" || !strings.Contains(ve.Issues[0].Message, "no_translator_aws") {
		t.Fatalf("err = %v", err)
	}
}
