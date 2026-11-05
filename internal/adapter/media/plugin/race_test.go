// SPDX-License-Identifier: AGPL-3.0-only

//go:build race

package plugin_test

// raceEnabled says that the race detector runs, which allocates for itself.
const raceEnabled = true
