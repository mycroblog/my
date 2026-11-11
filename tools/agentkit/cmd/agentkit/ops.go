// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/ops"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

func runIssue(r *repo.Repo, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: agentkit issue claim <n> [--plan text] | issue block <n> --reason text|--reason-file f")
	}
	fs := flag.NewFlagSet("issue", flag.ContinueOnError)
	plan := fs.String("plan", "", "one-line plan to comment")
	reason := fs.String("reason", "", "why the work stops")
	reasonFile := fs.String("reason-file", "", "file with the reason")
	pos, err := parse(fs, args[1:])
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("one issue number expected")
	}
	n, err := strconv.Atoi(pos[0])
	if err != nil {
		return err
	}
	var line string
	switch args[0] {
	case "claim":
		line, err = ops.Claim(r.Root, n, *plan)
	case "block":
		text := *reason
		if *reasonFile != "" {
			b, err := os.ReadFile(*reasonFile)
			if err != nil {
				return err
			}
			text = string(b)
		}
		line, err = ops.Block(n, text)
	default:
		return fmt.Errorf("unknown issue command %q", args[0])
	}
	if err != nil {
		return err
	}
	fmt.Println(line)
	return nil
}

func runPR(r *repo.Repo, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agentkit pr open|wait|watch")
	}
	switch args[0] {
	case "open":
		fs := flag.NewFlagSet("pr open", flag.ContinueOnError)
		var o ops.OpenOptions
		fs.IntVar(&o.Issue, "issue", 0, "issue number (default: from the branch name)")
		fs.StringVar(&o.Base, "base", "main", "base branch (a stacked PR's base)")
		fs.StringVar(&o.Title, "title", "", "PR title (default: the first commit's subject)")
		fs.StringVar(&o.Summary, "summary", "", "summary: what and why")
		summaryFile := fs.String("summary-file", "", "file with the summary")
		fs.StringVar(&o.Decisions, "decisions", "", "decisions and assumptions (default: None)")
		fs.StringVar(&o.Footer, "footer", "", "last line of the description, such as an attribution line")
		fs.BoolVar(&o.Confirm, "confirm", false, "check the docs and no-secrets items of the checklist")
		fs.BoolVar(&o.Draft, "draft", false, "open as a draft")
		fs.BoolVar(&o.NoPush, "no-push", false, "do not push first")
		if _, err := parse(fs, args[1:]); err != nil {
			return err
		}
		if *summaryFile != "" {
			b, err := os.ReadFile(*summaryFile)
			if err != nil {
				return err
			}
			o.Summary = string(b)
		}
		url, err := ops.Open(r, o)
		if err != nil {
			return err
		}
		fmt.Println(url)
		return nil
	case "wait":
		fs := flag.NewFlagSet("pr wait", flag.ContinueOnError)
		timeout := fs.Duration("timeout", 50*time.Minute, "give up after")
		interval := fs.Duration("interval", 30*time.Second, "poll interval")
		all := fs.Bool("all", false, "wait for every check instead of returning at the first failure")
		pos, err := parse(fs, args[1:])
		if err != nil || len(pos) != 1 {
			return fmt.Errorf("usage: agentkit pr wait <pr> [--timeout 50m] [--all]")
		}
		n, err := strconv.Atoi(strings.TrimPrefix(pos[0], "#"))
		if err != nil {
			return err
		}
		dir, err := r.TmpDir("ci")
		if err != nil {
			return err
		}
		branch, _ := r.Branch()
		head, _ := repo.Git(r.Root, "rev-parse", "HEAD")
		res := ops.Wait(n, branch, strings.TrimSpace(head), *timeout, *interval, *all, dir)
		fmt.Println(strings.Join(res.Lines, "\n"))
		if res.Code != 0 {
			return exitError{res.Code, ""}
		}
		return nil
	case "watch":
		fs := flag.NewFlagSet("pr watch", flag.ContinueOnError)
		interval := fs.Duration("interval", 60*time.Second, "poll interval")
		pos, err := parse(fs, args[1:])
		if err != nil {
			return err
		}
		var nums []int
		for _, p := range pos {
			n, err := strconv.Atoi(strings.TrimPrefix(p, "#"))
			if err != nil {
				return err
			}
			nums = append(nums, n)
		}
		return ops.Watch(os.Stdout, nums, *interval)
	}
	return fmt.Errorf("unknown pr command %q", args[0])
}

func runCheck(r *repo.Repo, args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	base := fs.String("base", "origin/main", "compare with the merge base of this revision")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	ok, lines, err := ops.Check(r, *base)
	if err != nil {
		return err
	}
	fmt.Println(strings.Join(lines, "\n"))
	if !ok {
		return exitError{1, ""}
	}
	return nil
}

func runRegen(r *repo.Repo, args []string) error {
	fs := flag.NewFlagSet("regen", flag.ContinueOnError)
	base := fs.String("base", "origin/main", "compare with the merge base of this revision")
	dry := fs.Bool("dry-run", false, "print the generators without running them")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	lines, err := ops.Regen(r, *base, *dry)
	fmt.Println(strings.Join(lines, "\n"))
	if err != nil {
		return exitError{1, ""}
	}
	return nil
}
