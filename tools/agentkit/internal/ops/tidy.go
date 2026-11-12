// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mycroblog/my/tools/agentkit/internal/gh"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// worktree is one entry of `git worktree list`.
type worktree struct {
	path   string
	branch string // "" when detached
	locked bool
}

// worktrees lists the working trees of the repository of dir, the main checkout first.
func worktrees(dir string) ([]worktree, error) {
	out, err := repo.Git(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var wts []worktree
	for _, l := range strings.Split(out, "\n") {
		key, val, _ := strings.Cut(l, " ")
		switch key {
		case "worktree":
			wts = append(wts, worktree{path: val})
		case "branch":
			wts[len(wts)-1].branch = strings.TrimPrefix(val, "refs/heads/")
		case "locked":
			wts[len(wts)-1].locked = true
		}
	}
	return wts, nil
}

// mainCheckout returns the main working tree of the repository of r, which the other worktrees hang off.
func mainCheckout(r *repo.Repo) string {
	if wts, err := worktrees(r.Root); err == nil && len(wts) > 0 {
		return wts[0].path
	}
	return filepath.Dir(r.Common)
}

// ancestor reports whether commit a is b or an ancestor of b.
func ancestor(dir, a, b string) bool {
	_, err := repo.Git(dir, "merge-base", "--is-ancestor", a, b)
	return err == nil
}

// inside reports whether path is dir or below it.
func inside(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// removeBranch removes the worktree of branch, if it has one, and the branch, and returns the line to print and
// whether it removed them (or would, with dry). It keeps a worktree that is the main checkout, the caller's own
// (cwd), locked, or has uncommitted changes. The caller has checked that the branch's commits are not lost.
func removeBranch(main, cwd string, wts []worktree, branch string, dry bool) (string, bool) {
	what := "branch " + branch
	var wt *worktree
	for i := range wts {
		if wts[i].branch == branch {
			wt = &wts[i]
		}
	}
	if wt != nil {
		what = fmt.Sprintf("worktree %s and branch %s", wt.path, branch)
		switch {
		case wt.path == main:
			return fmt.Sprintf("kept %s: the main checkout is on it (git -C %s switch main && scripts/agentkit.sh tidy)", branch, main), false
		case inside(cwd, wt.path):
			return fmt.Sprintf("kept %s: you are in its worktree (cd %s && scripts/agentkit.sh tidy)", branch, main), false
		case wt.locked:
			return fmt.Sprintf("kept %s: its worktree %s is locked, a session is using it", branch, wt.path), false
		}
		status, err := repo.Git(wt.path, "status", "--porcelain")
		if err != nil {
			return fmt.Sprintf("kept %s: %v", branch, err), false
		}
		if n := len(nonEmpty(status)); n > 0 {
			return fmt.Sprintf("kept %s: %d uncommitted files in %s", branch, n, wt.path), false
		}
	}
	if dry {
		return "would remove " + what, true
	}
	if wt != nil {
		if _, err := repo.Git(main, "worktree", "remove", wt.path); err != nil {
			return fmt.Sprintf("kept %s: %v", branch, err), false
		}
	}
	if _, err := repo.Git(main, "branch", "-D", branch); err != nil {
		return fmt.Sprintf("kept %s: %v", branch, err), false
	}
	return "removed " + what, true
}

// trunkRef is the remote-tracking ref of the trunk (repo.Trunk).
func trunkRef() string { return "origin/" + repo.Trunk }

// isTrunk reports whether branch is the trunk or the default branch, which are never tidied or worked on.
func isTrunk(branch string) bool {
	return branch == repo.Trunk || branch == repo.Default
}

// merged returns the local branches other than the trunk whose commits the merge queue merged into the trunk: the
// head is an ancestor of the trunk and not on its first-parent line, where a branch without commits of its
// own points. It reads no remote.
func merged(main string) ([]string, error) {
	refs, err := repo.Git(main, "for-each-ref", "--format=%(refname:short) %(objectname)", "--merged", trunkRef(), "refs/heads")
	if err != nil {
		return nil, err
	}
	line, err := repo.Git(main, "rev-list", "--first-parent", trunkRef())
	if err != nil {
		return nil, err
	}
	first := map[string]bool{}
	for _, c := range strings.Split(line, "\n") {
		first[c] = true
	}
	var out []string
	for _, l := range nonEmpty(refs) {
		branch, head, _ := strings.Cut(l, " ")
		if !isTrunk(branch) && !first[head] {
			out = append(out, branch)
		}
	}
	return out, nil
}

// branchPR is the newest pull request of a branch.
type branchPR struct {
	Number     int    `json:"number"`
	State      string `json:"state"`
	HeadRefOid string `json:"headRefOid"`
}

// prOf returns the pull request of branch that decides its fate: an open one if any, else the newest.
func prOf(branch string) (*branchPR, error) {
	var prs []branchPR
	if err := gh.JSON(&prs, "pr", "list", "-R", gh.Repo(), "--state", "all", "--head", branch, "--limit", "20", "--json", "number,state,headRefOid"); err != nil {
		return nil, err
	}
	var newest *branchPR
	for i := range prs {
		if prs[i].State == "OPEN" {
			return &prs[i], nil
		}
		if newest == nil || prs[i].Number > newest.Number {
			newest = &prs[i]
		}
	}
	return newest, nil
}

// Tidy removes the local branches whose pull request is merged or closed, with their worktrees, and returns one
// line for each it removed (would remove, with dry) or kept: it keeps a branch with commits its pull request does
// not hold, and a worktree with uncommitted changes, in use, or the caller's own. Branches with an open pull
// request or none are not listed.
func Tidy(r *repo.Repo, dry bool) ([]string, error) {
	main := mainCheckout(r)
	_, _ = repo.Git(main, "fetch", "--prune", "--quiet", "origin")
	done, err := merged(main)
	if err != nil {
		return nil, err
	}
	why := map[string]string{}
	for _, b := range done {
		why[b] = "merged into " + trunkRef()
	}
	refs, err := repo.Git(main, "for-each-ref", "--format=%(refname:short) %(objectname)", "refs/heads")
	if err != nil {
		return nil, err
	}
	wts, err := worktrees(main)
	if err != nil {
		return nil, err
	}
	var lines []string
	removed, kept := 0, 0
	for _, l := range nonEmpty(refs) {
		branch, head, _ := strings.Cut(l, " ")
		if isTrunk(branch) {
			continue
		}
		reason, ok := why[branch]
		if !ok {
			pr, err := prOf(branch)
			if err != nil || pr == nil || pr.State == "OPEN" {
				continue
			}
			reason = fmt.Sprintf("PR #%d %s", pr.Number, strings.ToLower(pr.State))
			if head != pr.HeadRefOid && !ancestor(main, head, trunkRef()) {
				kept++
				lines = append(lines, fmt.Sprintf("kept %s: it has commits that %s does not hold (git log %s..%s)", branch, reason, short7(pr.HeadRefOid), branch))
				continue
			}
		}
		line, ok := removeBranch(main, r.Root, wts, branch, dry)
		if ok {
			removed++
			line += " (" + reason + ")"
		} else {
			kept++
		}
		lines = append(lines, line)
	}
	if !dry {
		_, _ = repo.Git(main, "worktree", "prune")
	}
	if removed+kept == 0 {
		return []string{"nothing to tidy: no local branch has a merged or closed pull request"}, nil
	}
	return lines, nil
}

func short7(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// Leftovers counts the merged branches Tidy would look at, without reading a remote: those not checked out in
// the main checkout, the caller's worktree, or a locked one.
func Leftovers(r *repo.Repo) int {
	main := mainCheckout(r)
	done, err := merged(main)
	if err != nil {
		return 0
	}
	wts, err := worktrees(main)
	if err != nil {
		return 0
	}
	busy := map[string]bool{}
	for _, wt := range wts {
		if wt.path == main || wt.locked || inside(r.Root, wt.path) {
			busy[wt.branch] = true
		}
	}
	n := 0
	for _, b := range done {
		if !busy[b] {
			n++
		}
	}
	return n
}
