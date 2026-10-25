// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/mycroblog/my/tools/agentkit/internal/docslint"
	"github.com/mycroblog/my/tools/agentkit/internal/docsync"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

const docsUsage = "usage: agentkit docs lint | docs inventory | docs impact [--base origin/main] [--pr-body file]"

func runDocs(r *repo.Repo, args []string) error {
	if len(args) == 0 {
		return errors.New(docsUsage)
	}
	switch args[0] {
	case "lint", "inventory":
		if len(args) != 1 {
			return errors.New(docsUsage)
		}
		return runDocsRules(r, args[0])
	case "impact":
		return runDocsImpact(r, args[1:])
	}
	return errors.New(docsUsage)
}

func runDocsRules(r *repo.Repo, cmd string) error {
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
	run, what := docslint.Lint, "the rules are in AGENTS.md#documentation, the files each covers in "+docslint.File
	if cmd == "inventory" {
		run, what = docslint.Inventory, "describe each name in its document, or exclude it with the reason in "+docslint.File
	}
	findings, err := run(r.Root, paths, c)
	if err != nil {
		return err
	}
	for _, f := range findings {
		fmt.Println("error: " + f.String())
	}
	if len(findings) > 0 {
		return exitError{1, fmt.Sprintf("agentkit docs %s: %d problems (%s)", cmd, len(findings), what)}
	}
	fmt.Printf("agentkit docs %s: no problems\n", cmd)
	return nil
}

// runDocsImpact prints the user and operator documents that describe the paths changed since the base; with
// --pr-body it fails when the pull request's description neither changes nor explains one of them (the preflight
// of CI).
func runDocsImpact(r *repo.Repo, args []string) error {
	fs := flag.NewFlagSet("docs impact", flag.ContinueOnError)
	base := fs.String("base", "origin/main", "compare with the merge base of this revision")
	body := fs.String("pr-body", "", "the pull request's description, to check its "+docsync.Heading+" section")
	if pos, err := parse(fs, args); err != nil || len(pos) > 0 {
		return errors.New(docsUsage)
	}
	ims, err := docsync.Of(r, *base)
	if err != nil {
		return err
	}
	if len(ims) == 0 {
		fmt.Printf("no user or operator document describes the paths changed since %s\n", *base)
		return nil
	}
	fmt.Printf("user and operator documents that describe the paths changed since %s:\n", *base)
	for _, l := range docsync.Lines(ims) {
		fmt.Println(l)
	}
	if *body == "" {
		return nil
	}
	b, err := os.ReadFile(*body)
	if err != nil {
		return err
	}
	problems := docsync.Verify(ims, docsync.Reasons(string(b)))
	for _, p := range problems {
		fmt.Println("error: " + p)
	}
	if len(problems) > 0 {
		return exitError{1, fmt.Sprintf("agentkit docs impact: %d documents neither changed nor explained (AGENTS.md#documentation); after editing the description, rerun the failed job", len(problems))}
	}
	fmt.Println("each is changed or explained in the description")
	return nil
}
