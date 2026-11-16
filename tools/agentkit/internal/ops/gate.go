// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/mycroblog/my/tools/agentkit/internal/gh"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// refusal is why `pr open` does not open the pull request, and the command that fixes it.
func refusal(reason, fix string) error {
	return fmt.Errorf("pr open refused: %s\nfix: %s", reason, fix)
}

// linkedWorktree reports whether root is a worktree other than the main checkout.
func linkedWorktree(r *repo.Repo) bool {
	dir, err := repo.Git(r.Root, "rev-parse", "--path-format=absolute", "--git-dir")
	return err == nil && strings.TrimSpace(dir) != r.Common
}

// foreignMerges returns the merge commits of base..HEAD in dir that merge in something not on base, each as
// "<short sha> <subject>". Bringing a branch up to date is `git merge <base>`, whose parents after the first are
// on base; so is a branch merged in after it landed on base. CI's docs:pr-commits (tasks/docs.yml) is the same rule.
func foreignMerges(dir, base string) ([]string, error) {
	out, err := repo.Git(dir, "log", "--min-parents=2", "--format=%h%x00%P%x00%s", base+"..HEAD")
	if err != nil {
		return nil, err
	}
	var foreign []string
	for _, l := range nonEmpty(out) {
		f := strings.SplitN(l, "\x00", 3)
		if len(f) < 3 {
			continue
		}
		for _, p := range strings.Fields(f[1])[1:] {
			if exec.Command("git", "-C", dir, "merge-base", "--is-ancestor", p, base).Run() != nil {
				foreign = append(foreign, f[0]+" "+f[2])
				break
			}
		}
	}
	return foreign, nil
}

func nonEmpty(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// short joins at most n of items and counts the rest.
func short(items []string, n int) string {
	if len(items) <= n {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s, +%d more", strings.Join(items[:n], ", "), len(items)-n)
}

// mergeFix is how a branch takes in its base: a merge, the regenerated files, and a plain push.
func mergeFix(base string) string {
	return "git fetch origin && git merge " + base + ", settle conflicts in generated files with scripts/agentkit.sh regen, git commit"
}

// history refuses a branch that cannot merge as it is: a merge of something not on its base, or a conflict with
// its base.
func history(r *repo.Repo, branch, base string) error {
	merges, err := foreignMerges(r.Root, base)
	if err != nil {
		return err
	}
	if len(merges) > 0 {
		return refusal(fmt.Sprintf("%s..HEAD has %d merge commit(s) of something not on %s (%s); a branch merges in only its base", base, len(merges), base, short(merges, 3)),
			"land what it merges into "+base+" first, or take the merge out with git rebase "+base)
	}
	// A dry merge with the base: exit 1 and the conflicting files when it conflicts.
	cmd := exec.Command("git", "-C", r.Root, "merge-tree", "--write-tree", "--name-only", "--no-messages", base, "HEAD")
	out, err := cmd.Output()
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
		files := nonEmpty(string(out))
		if len(files) > 0 {
			files = files[1:] // the first line is the tree
		}
		return refusal(fmt.Sprintf("%s conflicts with %s in %s", branch, base, short(files, 5)), mergeFix(base))
	}
	_ = err // a git without merge-tree --write-tree: the pull request page reports the conflict
	return nil
}

// gate is everything `pr open` requires before it pushes: a worktree, a history that merges in nothing but its
// base and merges cleanly, and the quick checks.
func gate(r *repo.Repo, branch, base string) error {
	if !linkedWorktree(r) {
		dir := ".claude/worktrees/" + strings.ReplaceAll(branch, "/", "-")
		return refusal("this is the main checkout; work happens in a worktree of its own",
			fmt.Sprintf("git switch %s && git worktree add %s %s && cd %s", repo.Trunk, dir, branch, dir))
	}
	// The base as GitHub has it; offline, the last fetched one.
	_, _ = repo.Git(r.Root, "fetch", "--quiet", "origin", strings.TrimPrefix(base, "origin/"))
	if err := history(r, branch, base); err != nil {
		return err
	}
	ok, lines, err := CheckFast(r, base)
	if err != nil {
		return err
	}
	if !ok {
		return refusal("the quick checks fail:\n"+strings.Join(lines, "\n"), "repair the lines above, commit, and run pr open again (scripts/agentkit.sh check reruns the checks)")
	}
	return nil
}

// reSubjectIssues matches the issues a commit subject closes: "(#12)" or "(#12, #15)" at its end.
var reSubjectIssues = regexp.MustCompile(`\((#\d+(?:,\s*#\d+)*)\)\s*$`)

// subjectIssues returns the issue numbers the commit subjects name, in order, without duplicates.
func subjectIssues(subjects []string) []int {
	var out []int
	seen := map[int]bool{}
	for _, s := range subjects {
		m := reSubjectIssues.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		for _, f := range strings.Split(m[1], ",") {
			if n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(f), "#")); err == nil && !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	return out
}

// closing returns the issues the pull request closes: the given ones, or else the issue of the branch name and
// those the commit subjects name. A number read from a subject that is a pull request, or that cannot be read,
// is left out; a given one that cannot be read is an error.
func closing(given []int, branch string, subjects []string) ([]*gh.Issue, error) {
	nums := given
	if len(nums) == 0 {
		if n, err := strconv.Atoi(gh.BranchIssue(branch)); err == nil {
			nums = append(nums, n)
		}
		for _, n := range subjectIssues(subjects) {
			if len(nums) == 0 || nums[0] != n {
				nums = append(nums, n)
			}
		}
	}
	var out []*gh.Issue
	for _, n := range nums {
		is, err := gh.GetIssue(n)
		switch {
		case err != nil && len(given) > 0:
			return nil, err
		case err != nil, len(given) == 0 && strings.Contains(is.URL, "/pull/"):
			continue
		}
		out = append(out, is)
	}
	return out, nil
}
