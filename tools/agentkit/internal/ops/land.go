// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/gh"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// The exit statuses of Land other than 0 (merged) and 1 (CI failed, closed, or gh refused).
const (
	LandConflict  = 3   // the pull request conflicts with its base
	LandCancelled = 4   // checks were cancelled and no newer run replaced them
	LandDropped   = 5   // the merge queue put the pull request out without merging it
	LandTimeout   = 124 // still open after the timeout
)

// rebaseFix is what brings a branch back to a state the merge queue takes.
func rebaseFix() string {
	return "in the branch's worktree: git fetch origin && git rebase " + trunkRef() + " && scripts/agentkit.sh regen && git push --force-with-lease; then run pr land again"
}

// LandOptions are the waits of Land.
type LandOptions struct {
	Timeout   time.Duration
	Interval  time.Duration
	Heartbeat time.Duration // how often a line says that the wait goes on; 0 is every 5 minutes
	LogDir    string        // where the logs of failed jobs are saved
}

// settle is how long Land waits for GitHub to close the issues of a merged pull request, in steps.
var settle = 5 * time.Second

// Land takes pull request n to main and leaves nothing behind: it turns on auto-merge (the merge queue, a merge
// commit), waits until the pull request is merged, printing one line per event and one at each heartbeat and
// following the pull request's head when a push moves it, then
// removes the branch and its worktree, checks that the issues it closes are closed, and closes each parent issue
// whose sub-issues are all closed. It ends with three lines — merged, cleaned up, remains — and returns the exit
// status. A failure prints what failed and the fix.
func Land(w io.Writer, r *repo.Repo, n int, o LandOptions) int {
	say := sayTo(w)
	if o.Heartbeat <= 0 {
		o.Heartbeat = defaultHeartbeat
	}
	p, err := viewPR(n)
	if err != nil {
		say("pr land: %v", err)
		return 1
	}
	switch {
	case p.State == "MERGED":
		return landed(w, r, p)
	case p.State == "CLOSED":
		say("PR #%d is closed without merging", n)
		return 1
	case p.Mergeable == "CONFLICTING":
		say("PR #%d conflicts with its base: the merge queue does not take it\nfix: %s", n, rebaseFix())
		return LandConflict
	}
	autoMerge := func() bool {
		if _, err := gh.Run("pr", "merge", strconv.Itoa(n), "-R", gh.Repo(), "--auto", "--merge"); err != nil {
			say("PR #%d: could not turn on auto-merge: %v", n, err)
			return false
		}
		return true
	}
	if !autoMerge() {
		return 1
	}
	say("PR #%d: auto-merge on; waiting for its CI and the merge queue", n)
	say("%s", waitingLine(n, p))
	main := mainCheckout(r)
	deadline := time.Now().Add(o.Timeout)
	beat := time.Now().Add(o.Heartbeat)
	last := p // the newest state of the pull request's own head
	var old prSnapshot
	had := false
	var follow headFollow
	for {
		p, err := viewPR(n)
		queue, qerr := queuedPRs()
		switch {
		case err != nil || qerr != nil:
			// gh and the API fail now and then; give up only at the deadline.
		case p.State == "MERGED":
			return landed(w, r, p)
		case p.State == "CLOSED":
			say("PR #%d is closed without merging", n)
			return 1
		case p.Mergeable == "CONFLICTING":
			say("PR #%d conflicts with its base: the merge queue does not take it\nfix: %s", n, rebaseFix())
			return LandConflict
		default:
			behind, from := follow.observe(p.HeadRefOid, pushedHead(main, p.HeadRefName))
			if behind {
				// GitHub has not seen the last push yet: its checks are the previous commit's.
				break
			}
			ci, failed, _ := summarize(p.StatusCheckRollup)
			cur := prSnapshot{ci, p.Mergeable, failed, p.AutoMergeRequest != nil || queue[n], p.HeadRefOid}
			if from != "" {
				// A push by someone who may write keeps auto-merge on; a push to a pull request that is in the
				// merge queue takes it out, and then nothing would merge it.
				again := ""
				if !cur.queued {
					if !autoMerge() {
						return 1
					}
					cur.queued, again = true, " (the push turned auto-merge off: turned on again)"
				}
				say("PR #%d: the head moved to %s; following it%s", n, shortSHA(p.HeadRefOid), again)
				old, had = prSnapshot{}, false
			}
			switch {
			case ci == "failed":
				lines, rerun := failureReport(n, p, failed, o.LogDir)
				say("%s", strings.Join(lines, "\n"))
				if rerun != "" {
					say("fix: %s; then run pr land again", rerun)
				} else {
					say("fix: repair it in the branch's worktree, commit, push; then run pr land again")
				}
				return 1
			case ci == "cancelled":
				say("PR #%d: CI cancelled, not failed: %s; no newer run replaced them\nfix: gh run rerun <run> --failed, or push; then run pr land again", n, strings.Join(failed, ", "))
				return LandCancelled
			case had && old.queued && !cur.queued:
				// A merge between the two reads above also empties the queue.
				again, err := viewPR(n)
				if err == nil && again.State == "MERGED" {
					return landed(w, r, again)
				}
				if err == nil && again.HeadRefOid != p.HeadRefOid {
					// A push between the two reads took it out of the queue: the next poll follows the new head.
					break
				}
				say("PR #%d left the merge queue without merging: its merge group failed, or it conflicts with a pull request ahead of it", n)
				for _, l := range mergeGroupFailure(n, o.LogDir) {
					say("%s", l)
				}
				say("fix: %s", rebaseFix())
				return LandDropped
			}
			for _, e := range transitions(old, had, cur) {
				say("%s PR #%d %s", time.Now().Format("15:04"), n, e)
			}
			old, had, last = cur, true, p
		}
		now := time.Now()
		where := ""
		if had && old.queued && old.ci == "passed" {
			where = ", in the merge queue"
		}
		if now.After(beat) {
			say("%s%s", stillWaiting(last), where)
			beat = now.Add(o.Heartbeat)
		}
		if now.After(deadline) {
			say("PR #%d: timed out after %s, not merged: %s%s; run pr land again to keep waiting", n, o.Timeout, stillPending(last), where)
			return LandTimeout
		}
		time.Sleep(o.Interval)
	}
}

// pushedHead is the commit of the local branch when it is pushed, the commit GitHub's pull request is at once it
// has seen the push, or "" when the branch is not here or holds commits that are not pushed.
func pushedHead(main, branch string) string {
	local, err := repo.Git(main, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if err != nil {
		return ""
	}
	remote, err := repo.Git(main, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+branch)
	if err != nil || strings.TrimSpace(local) != strings.TrimSpace(remote) {
		return ""
	}
	return strings.TrimSpace(local)
}

// mergeGroupFailure returns the failing lines of the newest failed run of the merge group of pull request n
// (the branch gh-readonly-queue/<base>/pr-<n>-<sha>), or nothing when there is none.
func mergeGroupFailure(n int, logDir string) []string {
	var runs []struct {
		ID         int64  `json:"databaseId"`
		HeadBranch string `json:"headBranch"`
		Conclusion string `json:"conclusion"`
	}
	if err := gh.JSON(&runs, "run", "list", "-R", gh.Repo(), "--event", "merge_group", "--limit", "40", "--json", "databaseId,headBranch,conclusion"); err != nil {
		return nil
	}
	for _, run := range runs {
		if run.Conclusion != "failure" || !strings.Contains(run.HeadBranch, fmt.Sprintf("/pr-%d-", n)) {
			continue
		}
		var view struct {
			Jobs []struct {
				ID         int64  `json:"databaseId"`
				Name       string `json:"name"`
				Conclusion string `json:"conclusion"`
			} `json:"jobs"`
		}
		if err := gh.JSON(&view, "run", "view", strconv.FormatInt(run.ID, 10), "-R", gh.Repo(), "--json", "jobs"); err != nil {
			return nil
		}
		var lines []string
		shown := 0
		for _, j := range view.Jobs {
			if j.Conclusion != "failure" || j.Name == gateName || shown == 3 {
				continue
			}
			shown++
			out, err := gh.Run("api", "repos/"+gh.Repo()+"/actions/jobs/"+strconv.FormatInt(j.ID, 10)+"/logs")
			if err != nil {
				lines = append(lines, fmt.Sprintf("== %s: could not read the log: %v", j.Name, err))
				continue
			}
			file := filepath.Join(logDir, fmt.Sprintf("pr-%d-%d.log", n, j.ID))
			_ = os.WriteFile(file, out, 0o600)
			lines = append(lines, fmt.Sprintf("== %s (merge group; full log: %s)", j.Name, file))
			lines = append(lines, JobFailures(string(out), 40)...)
		}
		return lines
	}
	return nil
}

// reCloses matches the closing keywords of a pull request description.
var reCloses = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?) #(\d+)`)

// closedBy returns the issues pull request n closes: those GitHub links to it, and the closing keywords of its
// description.
func closedBy(n int) []int {
	var v struct {
		Body    string `json:"body"`
		Closing []struct {
			Number int `json:"number"`
		} `json:"closingIssuesReferences"`
	}
	_ = gh.JSON(&v, "pr", "view", strconv.Itoa(n), "-R", gh.Repo(), "--json", "body,closingIssuesReferences")
	seen := map[int]bool{}
	var out []int
	add := func(i int) {
		if i > 0 && !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	for _, c := range v.Closing {
		add(c.Number)
	}
	for _, m := range reCloses.FindAllStringSubmatch(v.Body, -1) {
		i, _ := strconv.Atoi(m[1])
		add(i)
	}
	sort.Ints(out)
	return out
}

const issueQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    issue(number: $number) {
      number title state
      milestone { number title }
      parent {
        number title state
        subIssuesSummary { total completed }
        subIssues(first: 100) { nodes { number state } }
      }
    }
  }
}`

// issueNode is an issue with its parent issue and the parent's sub-issues.
type issueNode struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Milestone *struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	} `json:"milestone"`
	Parent *struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		State   string `json:"state"`
		Summary struct {
			Total     int `json:"total"`
			Completed int `json:"completed"`
		} `json:"subIssuesSummary"`
		SubIssues struct {
			Nodes []struct {
				Number int    `json:"number"`
				State  string `json:"state"`
			} `json:"nodes"`
		} `json:"subIssues"`
	} `json:"parent"`
}

// issueWithParent reads issue n, its milestone, and its parent issue with the state of the parent's sub-issues.
func issueWithParent(n int) (issueNode, error) {
	owner, name, _ := strings.Cut(gh.Repo(), "/")
	var resp struct {
		Data struct {
			Repository struct {
				Issue issueNode `json:"issue"`
			} `json:"repository"`
		} `json:"data"`
	}
	err := gh.JSON(&resp, "api", "graphql", "-f", "query="+issueQuery, "-F", "owner="+owner, "-F", "name="+name, "-F", "number="+strconv.Itoa(n))
	return resp.Data.Repository.Issue, err
}

// followUp checks that the issues pull request n closes are closed, closes each parent issue whose sub-issues
// are all closed (and that parent's parent, in turn), and returns what is still open: issues the merge did not
// close, parents with open sub-issues, the closed issues with neither a parent nor a milestone, and the
// milestones with their counts. It closes a milestone whose issues are all closed.
func followUp(n int, issues []int) []string {
	var remains, loose []string
	milestones := map[int]string{}
	parents := map[int]bool{}
	queue := append([]int(nil), issues...)
	for depth := 0; len(queue) > 0 && depth < 50; depth++ {
		num := queue[0]
		queue = queue[1:]
		is, err := issueWithParent(num)
		// GitHub closes the issues a moment after the merge.
		for try := 0; err == nil && is.State == "OPEN" && try < 3; try++ {
			time.Sleep(settle)
			is, err = issueWithParent(num)
		}
		if err != nil {
			remains = append(remains, fmt.Sprintf("#%d unread (%v)", num, err))
			continue
		}
		if is.Milestone != nil {
			milestones[is.Milestone.Number] = is.Milestone.Title
		}
		if is.State == "OPEN" {
			remains = append(remains, fmt.Sprintf("#%d is still open (gh issue close %d -R %s)", num, num, gh.Repo()))
			continue
		}
		pa := is.Parent
		if pa == nil && is.Milestone == nil {
			loose = append(loose, "#"+strconv.Itoa(num))
		}
		if pa == nil || parents[pa.Number] {
			continue
		}
		parents[pa.Number] = true
		var open []string
		for _, s := range pa.SubIssues.Nodes {
			if s.State == "OPEN" {
				open = append(open, "#"+strconv.Itoa(s.Number))
			}
		}
		left := pa.Summary.Total - pa.Summary.Completed
		if left < len(open) {
			left = len(open)
		}
		switch {
		case left > 0:
			remains = append(remains, fmt.Sprintf("#%d %s: %d of %d sub-issues open (%s)", pa.Number, pa.Title, left, pa.Summary.Total, short(open, 8)))
		case pa.State != "OPEN":
			queue = append(queue, pa.Number)
		case repo.Trunk != repo.Default:
			remains = append(remains, fmt.Sprintf("#%d %s: every sub-issue is closed; it closes with the merge of %s into %s", pa.Number, pa.Title, repo.Trunk, repo.Default))
		default:
			if _, err := gh.Run("issue", "close", strconv.Itoa(pa.Number), "-R", gh.Repo(), "--comment",
				fmt.Sprintf("Every sub-issue is closed; #%d merged the last one.", n)); err != nil {
				remains = append(remains, fmt.Sprintf("#%d %s: every sub-issue is closed, but closing it failed (%v)", pa.Number, pa.Title, err))
				continue
			}
			remains = append(remains, fmt.Sprintf("closed #%d %s (every sub-issue is closed)", pa.Number, pa.Title))
			queue = append(queue, pa.Number)
		}
	}
	if len(loose) > 0 {
		remains = append(remains, strings.Join(loose, ", ")+": no parent issue and no milestone, so nothing tracks what the work was part of")
	}
	nums := make([]int, 0, len(milestones))
	for m := range milestones {
		nums = append(nums, m)
	}
	sort.Ints(nums)
	for _, m := range nums {
		var ms struct {
			Open   int    `json:"open_issues"`
			Closed int    `json:"closed_issues"`
			State  string `json:"state"`
		}
		if err := gh.JSON(&ms, "api", fmt.Sprintf("repos/%s/milestones/%d", gh.Repo(), m)); err != nil {
			continue
		}
		line := fmt.Sprintf("milestone %s: %d open, %d closed", milestones[m], ms.Open, ms.Closed)
		if ms.Open == 0 && ms.State == "open" {
			path := fmt.Sprintf("repos/%s/milestones/%d", gh.Repo(), m)
			if _, err := gh.Run("api", "-X", "PATCH", path, "-f", "state=closed"); err != nil {
				line += fmt.Sprintf(" (closing it failed: %v; gh api -X PATCH %s -f state=closed)", err, path)
			} else {
				line = fmt.Sprintf("closed milestone %s (each of its %d issues is closed)", milestones[m], ms.Closed)
			}
		}
		remains = append(remains, line)
	}
	return remains
}

// closeIntoTrunk closes the issues pull request n closes, which GitHub leaves open when it merges into a branch
// other than the default one. An issue already closed is left as it is.
func closeIntoTrunk(n int, issues []int) {
	for _, num := range issues {
		var is struct {
			State string `json:"state"`
		}
		if err := gh.JSON(&is, "issue", "view", strconv.Itoa(num), "-R", gh.Repo(), "--json", "state"); err != nil || is.State != "OPEN" {
			continue
		}
		_, _ = gh.Run("issue", "close", strconv.Itoa(num), "-R", gh.Repo(), "--comment",
			fmt.Sprintf("Merged into %s by #%d.", repo.Trunk, n))
	}
}

// landed finishes a merged pull request and prints the three closing lines.
func landed(w io.Writer, r *repo.Repo, p prState) int {
	issues := closedBy(p.Number)
	closes := ""
	if len(issues) > 0 {
		refs := make([]string, len(issues))
		for i, n := range issues {
			refs[i] = "#" + strconv.Itoa(n)
		}
		closes = " (closes " + strings.Join(refs, ", ") + ")"
	}
	fmt.Fprintf(w, "merged: PR #%d %s%s\n", p.Number, p.Title, closes)
	fmt.Fprintf(w, "cleaned up: %s\n", cleanUp(r, p))
	if repo.Trunk != repo.Default {
		closeIntoTrunk(p.Number, issues)
	}
	remains := followUp(p.Number, issues)
	if len(remains) == 0 {
		remains = []string{"nothing (the pull request closes no issue)"}
	}
	fmt.Fprintf(w, "remains: %s\n", strings.Join(remains, "; "))
	return 0
}

// cleanUp removes the local branch of the merged pull request p and its worktree, and says what it did.
func cleanUp(r *repo.Repo, p prState) string {
	main := mainCheckout(r)
	_, _ = repo.Git(main, "fetch", "--prune", "--quiet", "origin")
	defer func() { _, _ = repo.Git(main, "worktree", "prune") }()
	branch := p.HeadRefName
	head, err := repo.Git(main, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if err != nil {
		return "no local branch " + branch
	}
	if h := strings.TrimSpace(head); h != p.HeadRefOid && !ancestor(main, h, trunkRef()) {
		return fmt.Sprintf("kept %s: it has commits PR #%d did not merge (git log %s..%s)", branch, p.Number, trunkRef(), branch)
	}
	wts, err := worktrees(main)
	if err != nil {
		return "kept " + branch + ": " + err.Error()
	}
	line, _ := removeBranch(main, r.Root, wts, branch, false)
	return line
}
