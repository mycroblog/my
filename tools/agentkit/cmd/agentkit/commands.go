// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mycroblog/my/tools/agentkit/internal/kb"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
	"github.com/mycroblog/my/tools/agentkit/internal/store"
)

const usage = `agentkit — context and workflow tools for coding agents in mycroblog/my

Read (instead of grep / cat over the repository):
  ctx <issue> [--layer server|web] [--budget 30000]   everything an issue needs, in one pack (.tmp/agentkit/ctx/<n>.md)
  grep <pattern> [path|glob ...] [-i] [-w] [-F] [-l] [-o] [-C N] [-n 40] [--max 6000] [--kind go,ts,md,sql,yaml,json]
       [--tests|--only-tests] [--gen]                  exact / regexp search of the tracked files: bounded, grouped by
                                                       file, with the declaration or section of each match; skips
                                                       generated files, lock files, snapshots and tests
  find <words> [--kind k,k] [--under path] [-n 15]     ranked locations; kinds: section adr adr-section requirement
                                                       go go-test ts ts-test op schema table query msg file
  show <ref> [--max 12000]                             the text of a ref: a declaration (<GoName>, Recv.Name,
                                                       pkg.Name, <path>:<Name>), lines (<path>:10-40), ADR-0070#decision,
                                                       FR-915, data-model§4.4, web-client#5-アクセシビリティ, op:<id>,
                                                       table:<name>, query:<Name>
  outline <path>                                       headings / declarations with line ranges and sizes
  refs <ref> [-n 40]                                   what points at ref, and what ref points at
  trace <op:id|GoName> [--depth 6]                     handler -> app -> adapter -> query -> table
  adr list [--status Proposed]                         number, status, and title of every ADR

Workflow (instead of gh / scripts/task.sh loops):
  issue claim <n> [--plan text]                        check agent:ready, no assignee, Accepted ADRs; assign; comment
  issue block <n> --reason text|--reason-file f        comment and add agent:blocked
  pr open [--issue n]... [--base b] [--summary-file f] [--decisions text] [--draft]
                                                       the gate: refuses outside a worktree, on a merge or
                                                       fixup! commit, a conflict with the base, a failing
                                                       check, or UI screens not looked at; then pushes, opens
                                                       the PR from the template, links it on each issue;
                                                       prints the unchanged documents of docs impact
  pr land <pr> [--timeout 3h]                          the only way a PR finishes (run it in the background, in
                                                       the main agent): auto-merge through the merge queue, one
                                                       line per event, then remove the branch and worktree and
                                                       close finished parent issues; exit 1 CI failed, 3
                                                       conflict, 5 dropped by the queue (each prints the fix)
  tidy [--dry-run]                                     remove the worktrees and branches of merged or closed PRs
  pr wait <pr> [--timeout 50m] [--all]                 block until CI ends; print one line, or the failing lines
                                                       (run it in the background; exit 3 on a conflict,
                                                       4 when cancelled with no newer run)
  pr watch [<pr>...] [--interval 60s]                  one line per event: merged, closed, conflicting, ci-failed,
                                                       ci-cancelled, ci-green, dequeued (for the Monitor tool)
  check [--base origin/main] [--full]                  the quick checks of the changed files, one line each
                                                       (pr open runs them); --full: scripts/task.sh check:changed
  regen [--base origin/main] [--dry-run]               run the generators the changed sources need
  gh sync                                              refresh the local cache of issues and pull requests

Maintenance:
  docs lint                                            links and anchors resolve, English-only documents have no
                                                       Japanese, operator documents cite no design records,
                                                       docs/README.md links them all (docs/lint.yaml)
  docs adr-shape                                       the size, the implementation names, and the longest table
                                                       of each ADR, which docs lint limits
  docs inventory                                       every command of my, metric, webhook event type, build tag,
                                                       and listed task is mentioned in its document
  docs impact [--base origin/main]                     the user and operator documents (userdocs) of the changed
                                                       paths
  map check                                            every path is covered by a context-map.yaml, every strict
                                                       directory holds one, every user document is some entry's userdocs
  map show <path>                                      the context-map entries, docs and checks of a path
  stats [--since 2026-10-01] [--top 20]                token use of past Claude Code sessions in this repository
  hook pre|post                                        Claude Code hooks (reads the hook JSON on stdin)
  index                                                build the index now and print its size
`

func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(usage)
		return nil
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "hook":
		return runHook(rest)
	case "stats":
		return runStats(rest)
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	switch cmd {
	case "issue":
		return runIssue(r, rest)
	case "pr":
		return runPR(r, rest)
	case "check":
		return runCheck(r, rest)
	case "regen":
		return runRegen(r, rest)
	case "tidy":
		return runTidy(r, rest)
	case "map":
		return runMap(r, rest)
	case "adr":
		return runADR(r, rest)
	case "docs":
		return runDocs(r, rest)
	case "grep":
		return runGrep(r, rest)
	}
	s, err := store.Open(r)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	k := &kb.KB{S: s}
	switch cmd {
	case "index":
		var files, nodes, edges int
		_ = s.DB.QueryRow(`SELECT count(*) FROM view`).Scan(&files)
		_ = s.DB.QueryRow(`SELECT count(*) FROM nodes WHERE file_id IN (SELECT file_id FROM view)`).Scan(&nodes)
		_ = s.DB.QueryRow(`SELECT count(*) FROM edges WHERE file_id IN (SELECT file_id FROM view)`).Scan(&edges)
		fmt.Printf("%s: %d files, %d nodes, %d edges\n", store.Path(r), files, nodes, edges)
		return nil
	case "find":
		fs := flag.NewFlagSet("find", flag.ContinueOnError)
		kinds := fs.String("kind", "", "comma-separated node kinds")
		under := fs.String("under", "", "path prefix")
		n := fs.Int("n", 15, "results")
		words, err := parse(fs, rest)
		if err != nil {
			return err
		}
		return k.Find(os.Stdout, strings.Join(words, " "), split(*kinds), *under, *n)
	case "show":
		fs := flag.NewFlagSet("show", flag.ContinueOnError)
		maxTok := fs.Int("max", 12000, "largest text to print, in tokens")
		refs, err := parse(fs, rest)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			if err := k.Show(os.Stdout, ref, *maxTok); err != nil {
				return err
			}
		}
		return nil
	case "outline":
		for _, p := range rest {
			if err := k.Outline(os.Stdout, p); err != nil {
				return err
			}
		}
		return nil
	case "refs":
		fs := flag.NewFlagSet("refs", flag.ContinueOnError)
		n := fs.Int("n", 40, "lines per direction")
		refs, err := parse(fs, rest)
		if err != nil || len(refs) != 1 {
			return fmt.Errorf("usage: agentkit refs <ref> [-n 40]")
		}
		return k.Refs(os.Stdout, refs[0], *n)
	case "trace":
		fs := flag.NewFlagSet("trace", flag.ContinueOnError)
		depth := fs.Int("depth", 6, "levels")
		refs, err := parse(fs, rest)
		if err != nil || len(refs) != 1 {
			return fmt.Errorf("usage: agentkit trace <op:id|GoName> [--depth 6]")
		}
		return k.Trace(os.Stdout, refs[0], *depth)
	case "ctx":
		return runCtx(r, k, rest)
	case "gh":
		return runGH(r, s, rest)
	}
	return fmt.Errorf("unknown command %q (agentkit help)", cmd)
}

// parse parses flags that may come after the positional arguments too.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
