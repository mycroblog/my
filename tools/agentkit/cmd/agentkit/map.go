// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mycroblog/my/tools/agentkit/internal/ctxmap"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// contextsScript is the script whose contexts a map's `checks:` may name: it selects them, so a name it does not have
// would select nothing.
const contextsScript = "scripts/changed-contexts.sh"

// contextsLine is the script's list of every context, `ALL=(docs go ...)`.
var contextsLine = regexp.MustCompile(`(?m)^ALL=\(([^)]*)\)$`)

// checkContexts reads the contexts of scripts/changed-contexts.sh from its list of them, so that the two never differ.
func checkContexts(root string) ([]string, error) {
	b, err := os.ReadFile(filepath.Join(root, contextsScript))
	if err != nil {
		return nil, err
	}
	m := contextsLine.FindSubmatch(b)
	if m == nil {
		return nil, fmt.Errorf("%s: no line ALL=(...) lists its contexts", contextsScript)
	}
	return strings.Fields(string(m[1])), nil
}

func runMap(r *repo.Repo, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agentkit map check | map show <path>")
	}
	m, err := ctxmap.Of(r)
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
		contexts, err := checkContexts(r.Root)
		if err != nil {
			return err
		}
		problems := m.Check(r.Root, paths, contexts)
		for _, p := range problems {
			fmt.Printf("error: %s: %s\n", p.Path, p.Msg)
		}
		if len(problems) > 0 {
			return exitError{1, fmt.Sprintf("agentkit map check: %d problems", len(problems))}
		}
		fmt.Printf("the %s files cover all %d files\n", ctxmap.File, len(paths))
		return nil
	case "show":
		for _, p := range args[1:] {
			e, ok := m.For(p)
			if !ok {
				fmt.Printf("%s: no map\n", p)
				continue
			}
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
		}
		return nil
	}
	return fmt.Errorf("unknown map command %q", args[0])
}
