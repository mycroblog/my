// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_translator_aws

package config

import (
	"errors"
	"testing"
)

func TestTranslatorAWSRegion(t *testing.T) {
	cfg, _, err := Load(source(with(minimal(), "MY_TRANSLATOR_AWS_REGION", "us-east-1"), nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TranslatorAWSRegion != "us-east-1" {
		t.Errorf("TranslatorAWSRegion = %q", cfg.TranslatorAWSRegion)
	}
	_, _, err = Load(source(with(minimal(), "MY_TRANSLATOR_AWS_REGION", "not a region"), nil))
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Issues) != 1 || ve.Issues[0].Key != "translator_aws_region" {
		t.Fatalf("err = %v, want an issue of translator_aws_region", err)
	}
}
