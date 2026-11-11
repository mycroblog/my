// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/ctxmap"
	"github.com/mycroblog/my/tools/agentkit/internal/gh"
	"github.com/mycroblog/my/tools/agentkit/internal/kb"
	"github.com/mycroblog/my/tools/agentkit/internal/pack"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
	"github.com/mycroblog/my/tools/agentkit/internal/store"
	"github.com/mycroblog/my/tools/agentkit/internal/text"
)

func runCtx(r *repo.Repo, k *kb.KB, args []string) error {
	fs := flag.NewFlagSet("ctx", flag.ContinueOnError)
	budget := fs.Int("budget", 30000, "tokens for the whole pack")
	layer := fs.String("layer", "", "server, web, or empty for both")
	quiet := fs.Bool("quiet", false, "print only where the pack was written")
	pos, err := parse(fs, args)
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("usage: agentkit ctx <issue> [--layer server|web] [--budget 30000] [--quiet]")
	}
	n, err := strconv.Atoi(pos[0])
	if err != nil {
		return fmt.Errorf("issue number expected, got %q", pos[0])
	}
	cache := &gh.Cache{DB: k.S.DB}
	if time.Since(cache.Synced()) > 15*time.Minute {
		if _, err := cache.Sync(); err != nil {
			fmt.Fprintln(os.Stderr, "agentkit: the GitHub cache is stale and could not be refreshed:", err)
		}
	}
	is, err := gh.GetIssue(n)
	if err != nil {
		return err
	}
	m, err := ctxmap.Of(r)
	if err != nil {
		return err
	}
	out, err := pack.Build(k, cache, m, is, pack.Options{Budget: *budget, Layer: *layer})
	if err != nil {
		return err
	}
	dir, err := r.TmpDir("ctx")
	if err != nil {
		return err
	}
	file := filepath.Join(dir, strconv.Itoa(n)+".md")
	if err := os.WriteFile(file, []byte(out), 0o600); err != nil {
		return err
	}
	if *quiet {
		rel, _ := filepath.Rel(r.Root, file)
		fmt.Printf("%s (~%s tokens)\n", rel, text.Ktok(text.Tokens(out)))
		return nil
	}
	fmt.Print(out)
	return nil
}

func runGH(_ *repo.Repo, s *store.Store, args []string) error {
	if len(args) != 1 || args[0] != "sync" {
		return fmt.Errorf("usage: agentkit gh sync")
	}
	c := &gh.Cache{DB: s.DB}
	n, err := c.Sync()
	if err != nil {
		return err
	}
	fmt.Printf("synced %d issues and pull requests\n", n)
	return nil
}
