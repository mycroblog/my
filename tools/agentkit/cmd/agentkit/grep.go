// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"

	"github.com/mycroblog/my/tools/agentkit/internal/grep"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// runGrep searches the working tree; like grep, it exits 1 when nothing matches.
func runGrep(r *repo.Repo, args []string) error {
	o, err := grep.ParseArgs(args)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	found, err := grep.Run(r, cwd, o, os.Stdout)
	if err != nil {
		return err
	}
	if !found {
		return exitError{code: 1}
	}
	return nil
}
