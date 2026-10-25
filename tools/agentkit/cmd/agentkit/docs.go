// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"

	"github.com/mycroblog/my/tools/agentkit/internal/docslint"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

func runDocs(r *repo.Repo, args []string) error {
	if len(args) != 1 || args[0] != "lint" {
		return fmt.Errorf("usage: agentkit docs lint")
	}
	c, err := docslint.Load(r.Root)
	if err != nil {
		return err
	}
	files, err := r.Files()
	if err != nil {
		return err
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	findings, err := docslint.Lint(r.Root, paths, c)
	if err != nil {
		return err
	}
	for _, f := range findings {
		fmt.Println("error: " + f.String())
	}
	if len(findings) > 0 {
		return exitError{1, fmt.Sprintf("agentkit docs lint: %d problems (the rules are in AGENTS.md#documentation, the files each covers in %s)", len(findings), docslint.File)}
	}
	fmt.Println("agentkit docs lint: no problems")
	return nil
}
