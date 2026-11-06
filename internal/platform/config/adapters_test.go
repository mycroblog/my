// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"slices"
	"testing"
)

func TestLoadRejectsExcludedAdapters(t *testing.T) {
	saved := excludedValues
	t.Cleanup(func() { excludedValues = saved })
	excludedValues = map[string]map[string]string{}
	exclude("no_postgres", "database", "postgres")
	exclude("no_postgres", "stream", "postgres")
	exclude("no_billing_aws", "billing_sources", "aws")
	exclude("no_billing_gcp", "billing_sources", "gcp")

	if _, _, err := Load(source(minimal(), nil)); err != nil {
		t.Fatalf("Load() with database = sqlite: %v", err)
	}

	_, _, err := Load(source(with(minimal(), "MY_DATABASE", "postgres", "MY_STREAM", "postgres", "MY_BILLING_SOURCES", "gcp,aws"), nil))
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Load() error = %v, want *ValidationError", err)
	}
	var got []string
	for _, is := range ve.Issues {
		got = append(got, is.String())
	}
	for _, want := range []string{
		`database (MY_DATABASE): "postgres" is not in this build: the build tag no_postgres left its adapter out (https://mycroblog.dev/next/reference/build-tags/)`,
		`stream (MY_STREAM): "postgres" is not in this build: the build tag no_postgres left its adapter out (https://mycroblog.dev/next/reference/build-tags/)`,
		`billing_sources (MY_BILLING_SOURCES): "gcp" is not in this build: the build tag no_billing_gcp left its adapter out (https://mycroblog.dev/next/reference/build-tags/)`,
	} {
		if !slices.Contains(got, want) {
			t.Errorf("issues = %q, want %q among them", got, want)
		}
	}

	_, _, err = Load(source(with(minimal(), "MY_BILLING_SOURCES", "aws"), nil))
	want := `billing_sources (MY_BILLING_SOURCES): "aws" is not in this build: the build tag no_billing_aws left its adapter out (https://mycroblog.dev/next/reference/build-tags/)`
	if !errors.As(err, &ve) || len(ve.Issues) != 1 || ve.Issues[0].String() != want {
		t.Errorf("Load() with billing_sources = aws: %v, want %q", err, want)
	}
}

func TestExcludedError(t *testing.T) {
	got := ExcludedError("kv", "valkey", "no_valkey").Error()
	want := `kv = "valkey" is not in this build: the build tag no_valkey left its adapter out (https://mycroblog.dev/next/reference/build-tags/)`
	if got != want {
		t.Errorf("ExcludedError() = %q, want %q", got, want)
	}
}
