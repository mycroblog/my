// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"strings"

	"github.com/mycroblog/my/tools/agentkit/internal/ctxmap"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// checkContexts are the contexts of scripts/changed-contexts.sh.
var checkContexts = []string{"docs", "go", "i18n", "mail", "api", "ui", "web", "docker", "shell", "schema", "agentkit", "mykit"}

func runMap(r *repo.Repo, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agentkit map check | map show <path>")
	}
	m, err := ctxmap.Load(r.Root)
	if err != nil {
		return err
	}
	switch args[0] {
	case "check":
		files, err := r.Files()
		if err != nil {
			return err
		}
		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = f.Path
		}
		problems := m.Check(r.Root, paths, checkContexts)
		for _, p := range problems {
			fmt.Printf("error: %s: %s\n", p.Path, p.Msg)
		}
		if len(problems) > 0 {
			return exitError{1, fmt.Sprintf("%s: %d problems; add or fix entries or their userdocs (see the comment at its top)", ctxmap.File, len(problems))}
		}
		fmt.Printf("%s covers all %d files\n", ctxmap.File, len(paths))
		return nil
	case "show":
		for _, p := range args[1:] {
			es := m.For(p)
			if len(es) == 0 {
				fmt.Printf("%s: no entry\n", p)
			}
			for _, e := range es {
				fmt.Printf("%s: %s", p, e.Name)
				if e.Shared {
					fmt.Print(" [shared]")
				}
				if len(e.Checks) > 0 {
					fmt.Printf(" checks=%s", strings.Join(e.Checks, ","))
				}
				fmt.Println()
				for _, d := range e.Docs {
					fmt.Println("  doc  " + d)
				}
				for _, d := range e.Userdocs {
					fmt.Println("  userdoc " + d)
				}
				for _, n := range e.Notes {
					fmt.Println("  note " + n)
				}
			}
		}
		return nil
	}
	return fmt.Errorf("unknown map command %q", args[0])
}
