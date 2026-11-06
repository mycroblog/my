// SPDX-License-Identifier: AGPL-3.0-only

package main

import "flag"

// The generator of docs/reference/cli.md (cli_reference_test.go) walks the commands of my by running each one with
// -help, and learns through these hooks the subcommands of a command and the flags of each command. They are nil
// outside it.
var (
	// traceSubcommands is told the subcommands of cmd ("" for my itself, "admin role" for my admin role) when
	// the command picks one.
	traceSubcommands func(cmd string, subs []string)
	// traceFlagSet is told each flag set a command creates, before the command defines its flags.
	traceFlagSet func(fs *flag.FlagSet)
)

// newFlagSet returns the flag set of the command name, such as "my admin role grant", which returns its errors
// from Parse instead of exiting.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	if traceFlagSet != nil {
		traceFlagSet(fs)
	}
	return fs
}
