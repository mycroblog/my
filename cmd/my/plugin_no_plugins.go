// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_plugins

package main

import (
	"fmt"
	"io"
)

// runPlugin reports that the build tag no_plugins left the plugins out.
func runPlugin(_ []string, _, stderr io.Writer) int {
	_, _ = fmt.Fprintln(stderr, "my plugin is not in this build: the build tag no_plugins left the plugins out (docs/reference/build-tags.md)")
	return 1
}
