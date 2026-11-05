// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_plugins

package config

import (
	"errors"
	"strings"
	"testing"
)

func TestNoPluginsRejectsADestination(t *testing.T) {
	_, _, err := Load(source(with(minimal(), "MY_PLUGIN_BILLING_ADDRESS", "unix:///s"), nil))
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Issues) != 1 || !strings.Contains(ve.Issues[0].Message, "no_plugins") {
		t.Fatalf("err = %v", err)
	}
}
