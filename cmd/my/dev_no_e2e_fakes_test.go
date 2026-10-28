// SPDX-License-Identifier: AGPL-3.0-only

//go:build !e2e_fakes

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestDevIsOnlyInTestBuilds checks that a build without the tag e2e_fakes has no my dev: the fake
// push service never reaches a released binary.
func TestDevIsOnlyInTestBuilds(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"dev", "push-service"}, &stdout, &stderr); code != 2 {
		t.Errorf("my dev push-service = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), `unknown command "dev"`) {
		t.Errorf("stderr = %q, want the unknown command", stderr.String())
	}
	if strings.Contains(stderr.String(), "push-service") || strings.Contains(stderr.String(), "e2e_fakes") {
		t.Errorf("the usage names the commands of the test builds:\n%s", stderr.String())
	}
	if len(testBuildCommands) != 0 || testBuildUsage != "" {
		t.Errorf("testBuildCommands = %v, testBuildUsage = %q; want none", testBuildCommands, testBuildUsage)
	}
}
