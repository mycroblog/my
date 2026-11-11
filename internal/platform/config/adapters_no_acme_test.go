// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_acme

package config

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestNoACMERejectsTLSACME(t *testing.T) {
	_, _, err := Load(source(with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true"), nil))
	var ve *ValidationError
	if !errors.As(err, &ve) || !slices.ContainsFunc(ve.Issues, func(i Issue) bool { return i.Key == "tls" && strings.Contains(i.Message, "no_acme") }) {
		t.Fatalf("err = %v, want tls rejected by no_acme", err)
	}
}
