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

// trunkFix is what brings a branch back to a state the merge queue takes.
func trunkFix() string {
	return "in the branch's worktree: " + mergeFix(trunkRef()) + ", git push; then run pr land again"
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
		say("PR #%d conflicts with its base: the merge queue does not take it\nfix: %s", n, trunkFix())
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
			say("PR #%d conflicts with its base: the merge queue does not take it\nfix: %s", n, trunkFix())
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
				say("%s", strings.Join(dropReport(n, p.HeadRefOid, o.LogDir), "\n"))
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

// The causes of a merge group's failure, which choose the fix of a drop.
const (
	dropAhead  = iota // no merge group of the head failed: it conflicts with a pull request ahead of it
	dropChange        // a check failed because of the change
	dropRunner        // every failed job failed in the runner or a container, not because of the change
)

// dropReport returns the lines that say why the merge queue dropped pull request n at head and the fix that
// fits the cause.
func dropReport(n int, head, logDir string) []string {
	lines := []string{fmt.Sprintf("PR #%d left the merge queue without merging: its merge group failed, or it conflicts with a pull request ahead of it", n)}
	failure, cause := mergeGroupFailure(n, head, logDir)
	lines = append(lines, failure...)
	switch cause {
	case dropChange:
		return append(lines, "fix: repair it in the branch's worktree, commit, push; then run pr land again, which queues it again")
	case dropRunner:
		return append(lines, "fix: nothing in the change to repair; run pr land again to queue it again")
	}
	return append(lines, "fix: "+trunkFix())
}

// mergeGroupFailure returns the failing lines of the failed jobs of the merge group that held head, the head of
// pull request n, and the cause they show. A merge group runs on the branch gh-readonly-queue/<base>/pr-<n>-<base
// sha>, which an earlier group of the same pull request shares; the group of head is the one whose commit merges
// head. The queue drops a pull request at the first failed required job, so the group's run is often cancelled
// rather than failed. With no failed job of that group it returns nothing and dropAhead.
func mergeGroupFailure(n int, head, logDir string) ([]string, int) {
	var runs []struct {
		ID         int64  `json:"databaseId"`
		HeadBranch string `json:"headBranch"`
		HeadSha    string `json:"headSha"`
		Conclusion string `json:"conclusion"`
	}
	if err := gh.JSON(&runs, "run", "list", "-R", gh.Repo(), "--event", "merge_group", "--limit", "40", "--json", "databaseId,headBranch,headSha,conclusion"); err != nil {
		return nil, dropAhead
	}
	merges := map[string]bool{} // whether the commit of a merge group merges head
	var lines []string
	shown, read := 0, 0
	runner := true
	var reasons []string
	for _, run := range runs {
		if run.Conclusion == "success" || !strings.Contains(run.HeadBranch, fmt.Sprintf("/pr-%d-", n)) {
			continue
		}
		in, seen := merges[run.HeadSha]
		if !seen {
			in = mergesHead(run.HeadSha, head)
			merges[run.HeadSha] = in
		}
		if !in {
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
			continue
		}
		for _, j := range view.Jobs {
			// `gate` only collects the other jobs' results; its log says nothing new.
			if j.Conclusion != "failure" || j.Name == gateName {
				continue
			}
			show := shown < shownFailures
			if !show && (!runner || read == maxFailureLogs) {
				runner = false
				continue
			}
			read++
			if show {
				shown++
			}
			out, err := gh.Run("api", "repos/"+gh.Repo()+"/actions/jobs/"+strconv.FormatInt(j.ID, 10)+"/logs")
			if err != nil {
				runner = false
				if show {
					lines = append(lines, fmt.Sprintf("== %s: could not read the log: %v", j.Name, err))
				}
				continue
			}
			file := filepath.Join(logDir, fmt.Sprintf("pr-%d-%d.log", n, j.ID))
			_ = os.WriteFile(file, out, 0o600)
			if show {
				lines = append(lines, fmt.Sprintf("== %s (merge group of %s; full log: %s)", j.Name, shortSHA(head), file))
				lines = append(lines, JobFailures(string(out), 40)...)
			}
			why := runnerFailure(string(out))
			runner = runner && why != ""
			reasons = append(reasons, why)
		}
	}
	switch {
	case read == 0:
		return []string{fmt.Sprintf("no merge group of the head %s failed", shortSHA(head))}, dropAhead
	case runner:
		return append([]string{"the merge group failed in the runner, not in the change: " + strings.Join(unique(reasons), "; ")}, lines...), dropRunner
	}
	return lines, dropChange
}

// mergesHead reports whether the merge group commit sha merges head: head is one of its parents.
func mergesHead(sha, head string) bool {
	var parents []string
	if err := gh.JSON(&parents, "api", "repos/"+gh.Repo()+"/commits/"+sha, "--jq", "[.parents[].sha]"); err != nil {
		return false
	}
	for _, p := range parents {
		if p == head {
			return true
		}
	}
	return false
}

// runnerFailure says why a failed job's log failed in the runner or a container rather than in the change, or
// "" when it shows no such cause.
func runnerFailure(log string) string {
	if hosts := Unreachable(log); len(hosts) > 0 {
		return "the runner could not reach " + most(hosts)
	}
	if line := ContainerFailure(log); line != "" {
		return "a container failed: " + line
	}
	return ""
}

// unique returns items without the repeats, in their order.
func unique(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, i := range items {
		if !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	return out
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
