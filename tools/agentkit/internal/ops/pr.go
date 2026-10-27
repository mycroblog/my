// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/docsync"
	"github.com/mycroblog/my/tools/agentkit/internal/gh"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// OpenOptions are the parts of a pull request the agent writes; the rest comes from the issue and the commits.
type OpenOptions struct {
	Issue     int
	Base      string
	Title     string
	Summary   string
	Decisions string
	Footer    string // e.g. the attribution line the agent's harness asks for
	// DocReasons says, by document path, why a user or operator document that describes the changed paths needs
	// no change (the Documentation section, docsync).
	DocReasons map[string]string
	Confirm    bool // the agent confirms the docs and no-secrets items of the checklist
	Draft      bool
	NoPush     bool
}

var reCheckbox = regexp.MustCompile(`^\s*- \[([ xX])\]\s+(.*)$`)

// criterion is one item of the issue's acceptance criteria and whether the issue has it checked.
type criterion struct {
	Text    string
	Checked bool
}

// acceptanceCriteria returns the checklist under the issue's "Acceptance criteria" heading, each item with its
// checked state.
func acceptanceCriteria(body string) []criterion {
	var out []criterion
	in := false
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, "#") {
			in = strings.Contains(strings.ToLower(l), "acceptance criteria") || strings.Contains(l, "受け入れ基準")
			continue
		}
		if m := reCheckbox.FindStringSubmatch(l); in && m != nil {
			out = append(out, criterion{Text: m[2], Checked: m[1] != " "})
		}
	}
	return out
}

// Open pushes the branch and opens its pull request from .github/pull_request_template.md's sections: the
// summary, Closes #N (or Stacked on #M first), the commit subjects as the changes, the issue's acceptance
// criteria with the checked state they have in the issue, the decisions, the documentation (docsync.Section: it refuses to open the PR while a document
// that describes the changed paths is neither changed nor given a reason), and the checklist (`scripts/task.sh check` checked when `agentkit check` passed on
// this content of the tree). It comments the PR on the issue and returns the PR URL.
func Open(r *repo.Repo, o OpenOptions) (string, error) {
	branch, err := r.Branch()
	if err != nil {
		return "", err
	}
	if branch == "main" || branch == "HEAD" {
		return "", fmt.Errorf("on %s: work on a <type>/<issue>-<slug> branch (workflow.md §3)", branch)
	}
	if o.Issue == 0 {
		o.Issue, _ = strconv.Atoi(gh.BranchIssue(branch))
	}
	if o.Base == "" {
		o.Base = "main"
	}
	ims, err := docsync.Of(r, "origin/"+o.Base)
	if err != nil {
		return "", err
	}
	docSection, problems := docsync.Section(ims, o.DocReasons)
	if len(problems) > 0 {
		return "", fmt.Errorf("the Documentation section is incomplete; update each document, or pass --doc-reason '<path>=<reason>' (AGENTS.md#documentation):\n%s", strings.Join(problems, "\n"))
	}
	var is *gh.Issue
	if o.Issue > 0 {
		if is, err = gh.GetIssue(o.Issue); err != nil {
			return "", err
		}
	}
	if !o.NoPush {
		if out, err := exec.Command("git", "-C", r.Root, "push", "-u", "origin", "HEAD").CombinedOutput(); err != nil {
			return "", fmt.Errorf("git push: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	logOut, err := repo.Git(r.Root, "log", "--reverse", "--no-merges", "--format=%s", "origin/"+o.Base+"..HEAD")
	if err != nil {
		return "", err
	}
	subjects := strings.Split(strings.TrimSpace(logOut), "\n")
	if len(subjects) == 0 || subjects[0] == "" {
		return "", fmt.Errorf("no commits on %s since origin/%s", branch, o.Base)
	}
	if o.Title == "" {
		o.Title = subjects[0]
	}

	var b strings.Builder
	if o.Base != "main" {
		var prs []struct {
			Number int `json:"number"`
		}
		if err := gh.JSON(&prs, "pr", "list", "-R", gh.Repo(), "--head", o.Base, "--json", "number"); err == nil && len(prs) > 0 {
			fmt.Fprintf(&b, "Stacked on #%d\n\n", prs[0].Number)
		}
	}
	summary := strings.TrimSpace(o.Summary)
	if summary == "" && is != nil {
		summary = is.Title
	}
	fmt.Fprintf(&b, "## Summary\n\n%s\n\n", summary)
	if o.Issue > 0 {
		fmt.Fprintf(&b, "Closes #%d\n\n", o.Issue)
	}
	b.WriteString("## Changes\n\n")
	for _, s := range subjects {
		fmt.Fprintf(&b, "- %s\n", s)
	}
	b.WriteString("\n## Acceptance criteria\n\n")
	var ac []criterion
	if is != nil {
		ac = acceptanceCriteria(is.Body)
	}
	switch {
	case is == nil:
		b.WriteString("No issue: nothing to check.\n")
	case len(ac) == 0:
		b.WriteString("- [ ] (the issue has no checklist)\n")
	}
	for _, a := range ac {
		fmt.Fprintf(&b, "- [%s] %s\n", mark(a.Checked), a.Text)
	}
	decisions := strings.TrimSpace(o.Decisions)
	if decisions == "" {
		decisions = "None"
	}
	fmt.Fprintf(&b, "\n## Decisions and assumptions\n\n%s\n\n%s\n## Checklist\n\n", decisions, docSection)
	fmt.Fprintf(&b, "- [%s] `scripts/task.sh check` passes locally\n", mark(CheckPassed(r, "origin/"+o.Base)))
	fmt.Fprintf(&b, "- [%s] Docs / i18n catalogs updated where needed\n", mark(o.Confirm))
	fmt.Fprintf(&b, "- [%s] No secrets, no unrelated changes\n", mark(o.Confirm))
	if o.Footer != "" {
		fmt.Fprintf(&b, "\n%s\n", o.Footer)
	}

	args := []string{"pr", "create", "-R", gh.Repo(), "--base", o.Base, "--head", branch, "--title", o.Title, "--body-file", "-"}
	if o.Draft {
		args = append(args, "--draft")
	}
	cmd := exec.Command("gh", args...)
	cmd.Dir = r.Root
	cmd.Stdin = strings.NewReader(b.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr create: %w: %s", err, strings.TrimSpace(string(out)))
	}
	url := strings.TrimSpace(string(out))
	if i := strings.LastIndex(url, "\n"); i >= 0 {
		url = url[i+1:]
	}
	if o.Issue > 0 {
		if _, err := gh.Run("issue", "comment", strconv.Itoa(o.Issue), "-R", gh.Repo(), "--body", "PR: "+url); err != nil {
			return url, err
		}
	}
	note := ""
	if !CheckPassed(r, "origin/"+o.Base) {
		note = " (the check item is unchecked: run `agentkit check` and tick it)"
	}
	return url + note, nil
}

func mark(b bool) string {
	if b {
		return "x"
	}
	return " "
}

// check is one entry of a pull request's statusCheckRollup.
type check struct {
	Typename     string `json:"__typename"`
	Name         string `json:"name"`
	Context      string `json:"context"`
	WorkflowName string `json:"workflowName"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	State        string `json:"state"`
	StartedAt    string `json:"startedAt"`
	DetailsURL   string `json:"detailsUrl"`
	TargetURL    string `json:"targetUrl"`
}

func (c check) name() string {
	if c.Name != "" {
		return c.Name
	}
	return c.Context
}

// jobID is the Actions job of the check, or 0 for a check that is not an Actions job. Job IDs grow with time,
// so a larger one belongs to a newer run.
func (c check) jobID() int64 {
	m := reJob.FindStringSubmatch(c.DetailsURL + c.TargetURL)
	if m == nil {
		return 0
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	return id
}

// result is pending, failed, cancelled, or passed.
func (c check) result() string {
	if c.Typename == "StatusContext" {
		switch c.State {
		case "SUCCESS":
			return "passed"
		case "FAILURE", "ERROR":
			return "failed"
		}
		return "pending"
	}
	if c.Status != "COMPLETED" {
		return "pending"
	}
	switch c.Conclusion {
	case "FAILURE", "TIMED_OUT", "STARTUP_FAILURE", "ACTION_REQUIRED":
		return "failed"
	case "CANCELLED":
		return "cancelled"
	}
	return "passed"
}

// latest keeps, for each workflow and check name, only the check of the newest run. A push or a label starts
// the workflow again on the same head, and its concurrency group cancels the older run, whose checks stay in
// the rollup next to the newer ones.
func latest(cs []check) []check {
	idx := map[string]int{}
	var out []check
	for _, c := range cs {
		key := c.WorkflowName + "\x00" + c.name()
		i, ok := idx[key]
		if !ok {
			idx[key] = len(out)
			out = append(out, c)
			continue
		}
		if o := out[i]; c.jobID() > o.jobID() || (c.jobID() == o.jobID() && c.StartedAt >= o.StartedAt) {
			out[i] = c
		}
	}
	return out
}

type prState struct {
	Number            int     `json:"number"`
	Title             string  `json:"title"`
	State             string  `json:"state"`
	Mergeable         string  `json:"mergeable"`
	URL               string  `json:"url"`
	HeadRefOid        string  `json:"headRefOid"`
	HeadRefName       string  `json:"headRefName"`
	StatusCheckRollup []check `json:"statusCheckRollup"`
	AutoMergeRequest  *struct {
		EnabledAt string `json:"enabledAt"`
	} `json:"autoMergeRequest"`
}

const prFields = "number,title,state,mergeable,url,headRefOid,headRefName,statusCheckRollup,autoMergeRequest"

func viewPR(n int) (prState, error) {
	var p prState
	err := gh.JSON(&p, "pr", "view", strconv.Itoa(n), "-R", gh.Repo(), "--json", prFields)
	return p, err
}

// summarize returns the CI result of a pull request (none, failed, pending, cancelled, passed) from the latest
// run of each check, the names of the failed checks (of the cancelled ones for cancelled), and whether any
// check is still running. Cancelled is a run that someone stopped, or that no newer run replaced. It is
// passed only when the required gate (the CI workflow's `check`) is among the checks and passed: other checks passing
// while the gate has not reported yet is pending.
func summarize(cs []check) (string, []string, bool) {
	if len(cs) == 0 {
		return "none", nil, false
	}
	var failed, cancelled []string
	pending := false
	for _, c := range latest(cs) {
		switch c.result() {
		case "failed":
			failed = append(failed, c.name())
		case "cancelled":
			cancelled = append(cancelled, c.name())
		case "pending":
			pending = true
		}
	}
	sort.Strings(failed)
	sort.Strings(cancelled)
	switch {
	case len(failed) > 0:
		return "failed", failed, pending
	case pending:
		return "pending", nil, true
	case len(cancelled) > 0:
		return "cancelled", cancelled, false
	case !gatePassed(cs):
		// The other checks passed, but the required gate has not reported (or is not a success yet).
		return "pending", nil, true
	}
	return "passed", nil, false
}

// gate is the required status check of the protect-main ruleset: the `check` job of the CI workflow, which
// passes only when every job it needs passed or was skipped.
const (
	gateName     = "check"
	gateWorkflow = "CI"
)

// gatePassed reports whether the latest run of the required gate is in cs and passed.
func gatePassed(cs []check) bool {
	for _, c := range latest(cs) {
		if c.name() == gateName && (c.WorkflowName == gateWorkflow || c.WorkflowName == "") && c.result() == "passed" {
			return true
		}
	}
	return false
}

// WaitResult is how Wait ended: the exit status and the lines to print.
type WaitResult struct {
	Code  int
	Lines []string
}

var reJob = regexp.MustCompile(`/actions/runs/\d+/job/(\d+)`)

// Wait polls a pull request until its CI ends, it conflicts, it merges, or the timeout passes. With all false
// it returns at the first failed check instead of waiting for the others. A failure comes with the failing
// lines of the failed jobs' logs, which are saved under logDir. When the pull request's branch is branch, the
// checks count only once its head is head, so that a wait right after a push does not read the previous
// commit's results.
func Wait(n int, branch, head string, timeout, interval time.Duration, all bool, logDir string) WaitResult {
	deadline := time.Now().Add(timeout)
	started := time.Now()
	for {
		p, err := viewPR(n)
		switch {
		case err != nil:
			// gh and the API fail now and then; give up only at the deadline.
		case p.State == "MERGED":
			return WaitResult{0, []string{fmt.Sprintf("PR #%d is merged", n)}}
		case p.State == "CLOSED":
			return WaitResult{1, []string{fmt.Sprintf("PR #%d is closed", n)}}
		case p.HeadRefName == branch && head != "" && p.HeadRefOid != head:
			// GitHub has not seen the push yet.
		case p.Mergeable == "CONFLICTING":
			return WaitResult{3, []string{fmt.Sprintf("PR #%d conflicts with its base: CI does not run. Merge origin/main, regenerate generated files (agentkit regen), agentkit check, push (workflow.md §7)", n)}}
		default:
			res, failed, pending := summarize(p.StatusCheckRollup)
			if res == "failed" && (!all || !pending) {
				return WaitResult{1, failureReport(n, p, failed, logDir)}
			}
			if res == "cancelled" {
				return WaitResult{4, []string{fmt.Sprintf("PR #%d: CI cancelled, not failed: %s; no newer run replaced them. Rerun the workflow (gh run rerun) or push", n, strings.Join(failed, ", "))}}
			}
			if res == "passed" {
				return WaitResult{0, []string{fmt.Sprintf("PR #%d: CI passed (%d checks, mergeable %s)", n, len(latest(p.StatusCheckRollup)), strings.ToLower(p.Mergeable))}}
			}
			if res == "none" && time.Since(started) > 10*time.Minute {
				return WaitResult{2, []string{fmt.Sprintf("PR #%d: no check started in 10 minutes (mergeable %s); check the PR page %s", n, strings.ToLower(p.Mergeable), p.URL)}}
			}
		}
		if time.Now().After(deadline) {
			return WaitResult{124, []string{fmt.Sprintf("PR #%d: CI still running after %s", n, timeout)}}
		}
		time.Sleep(interval)
	}
}

func failureReport(n int, p prState, failed []string, logDir string) []string {
	lines := []string{fmt.Sprintf("PR #%d: CI failed: %s", n, strings.Join(failed, ", "))}
	shown := 0
	for _, c := range latest(p.StatusCheckRollup) {
		// `check` only collects the other jobs' results; its log says nothing new.
		if c.result() != "failed" || c.name() == "check" || shown == 3 {
			continue
		}
		m := reJob.FindStringSubmatch(c.DetailsURL + c.TargetURL)
		if m == nil {
			lines = append(lines, fmt.Sprintf("== %s: %s", c.name(), c.DetailsURL+c.TargetURL))
			continue
		}
		shown++
		// The job's own log is readable as soon as the job ends; `gh run view --log` waits for the whole run.
		out, err := gh.Run("api", "repos/"+gh.Repo()+"/actions/jobs/"+m[1]+"/logs")
		if err != nil {
			lines = append(lines, fmt.Sprintf("== %s: could not read the log: %v", c.name(), err))
			continue
		}
		file := filepath.Join(logDir, fmt.Sprintf("pr-%d-%s.log", n, m[1]))
		_ = os.WriteFile(file, out, 0o600)
		lines = append(lines, fmt.Sprintf("== %s (full log: %s)", c.name(), file))
		lines = append(lines, Failures(string(out), 40)...)
	}
	return lines
}

// prSnapshot is what Watch compares between polls.
type prSnapshot struct {
	ci, mergeable string
	failed        []string
	queued        bool // auto-merge on, or in the merge queue
}

// transitions returns the events between two snapshots of an open pull request; had is false on its first poll.
func transitions(old prSnapshot, had bool, cur prSnapshot) []string {
	var out []string
	switch {
	case cur.mergeable == "CONFLICTING" && (!had || old.mergeable != "CONFLICTING"):
		out = append(out, "conflicting")
	case cur.ci == "failed" && (!had || old.ci != "failed"):
		out = append(out, "ci-failed "+strings.Join(cur.failed, ","))
	case cur.ci == "cancelled" && (!had || old.ci != "cancelled"):
		out = append(out, "ci-cancelled "+strings.Join(cur.failed, ","))
	case cur.ci == "passed" && had && old.ci != "passed":
		out = append(out, "ci-green")
	}
	switch {
	case had && !old.queued && cur.queued:
		out = append(out, "queued")
	case had && old.queued && !cur.queued:
		out = append(out, "dequeued (out of the merge queue without merging: a conflict or a failed merge group)")
	}
	return out
}

const queueQuery = `query($owner: String!, $name: String!, $cursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequests(states: OPEN, first: 100, after: $cursor) {
      pageInfo { hasNextPage endCursor }
      nodes { number mergeQueueEntry { state } }
    }
  }
}`

// queuePage is one page of the open pull requests with their merge queue entry.
type queuePage struct {
	Nodes []struct {
		Number          int `json:"number"`
		MergeQueueEntry *struct {
			State string `json:"state"`
		} `json:"mergeQueueEntry"`
	} `json:"nodes"`
	PageInfo struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
}

// collectQueued reads every page that fetch returns, so that a repository with more than one page of open pull
// requests does not lose the queue entries of the later ones.
func collectQueued(fetch func(cursor string) (queuePage, error)) (map[int]bool, error) {
	out := map[int]bool{}
	cursor := ""
	for {
		page, err := fetch(cursor)
		if err != nil {
			return nil, err
		}
		for _, n := range page.Nodes {
			if n.MergeQueueEntry != nil {
				out[n.Number] = true
			}
		}
		if !page.PageInfo.HasNextPage {
			return out, nil
		}
		cursor = page.PageInfo.EndCursor
	}
}

// queuedPRs returns the open pull requests that are in the merge queue. A pull request in the queue has no
// auto-merge request, and gh pr list cannot report the queue, so it is read through GraphQL.
func queuedPRs() (map[int]bool, error) {
	owner, name, _ := strings.Cut(gh.Repo(), "/")
	return collectQueued(func(cursor string) (queuePage, error) {
		args := []string{"api", "graphql", "-f", "query=" + queueQuery, "-F", "owner=" + owner, "-F", "name=" + name}
		if cursor != "" {
			args = append(args, "-F", "cursor="+cursor)
		}
		var resp struct {
			Data struct {
				Repository struct {
					PullRequests queuePage `json:"pullRequests"`
				} `json:"repository"`
			} `json:"data"`
		}
		err := gh.JSON(&resp, args...)
		return resp.Data.Repository.PullRequests, err
	})
}

// openLimit is how many open pull requests Watch lists; a repository with more is reported, not silently cut.
const openLimit = 1000

// Watch prints one line per event of the watched pull requests — opened, merged, closed, conflicting,
// ci-failed, ci-cancelled, ci-green, queued, dequeued (out of the merge queue, or auto-merge off, while open) — until every
// watched pull request is merged or closed. With no numbers it watches every open pull request, including
// new ones, forever.
func Watch(w io.Writer, nums []int, interval time.Duration) error {
	all := len(nums) == 0
	watched := map[int]bool{}
	for _, n := range nums {
		watched[n] = true
	}
	prev := map[int]prSnapshot{}
	first, warned := true, false
	emit := func(n int, title, event string) {
		fmt.Fprintf(w, "%s #%d %s — %s\n", time.Now().Format("15:04"), n, event, title)
		if f, ok := w.(*os.File); ok {
			_ = f.Sync()
		}
	}
	for {
		var open []prState
		if err := gh.JSON(&open, "pr", "list", "-R", gh.Repo(), "--state", "open", "--limit", strconv.Itoa(openLimit), "--json", prFields); err != nil {
			time.Sleep(interval)
			continue
		}
		if len(open) >= openLimit && !warned {
			warned = true
			fmt.Fprintf(w, "%s warning — %d open pull requests: the list is cut at %d, so the later ones are not watched\n", time.Now().Format("15:04"), len(open), openLimit)
		}
		queue, err := queuedPRs()
		if err != nil {
			time.Sleep(interval)
			continue
		}
		seen := map[int]bool{}
		for _, p := range open {
			seen[p.Number] = true
			if all && !watched[p.Number] {
				watched[p.Number] = true
				if !first {
					emit(p.Number, p.Title, "opened")
				}
			}
			if !watched[p.Number] {
				continue
			}
			ci, failed, _ := summarize(p.StatusCheckRollup)
			cur := prSnapshot{ci, p.Mergeable, failed, p.AutoMergeRequest != nil || queue[p.Number]}
			old, had := prev[p.Number]
			for _, e := range transitions(old, had, cur) {
				emit(p.Number, p.Title, e)
			}
			prev[p.Number] = cur
		}
		for n := range watched {
			if seen[n] {
				continue
			}
			p, err := viewPR(n)
			if err != nil {
				continue
			}
			switch p.State {
			case "MERGED":
				emit(n, p.Title, "merged")
				delete(watched, n)
			case "CLOSED":
				emit(n, p.Title, "closed")
				delete(watched, n)
			}
		}
		first = false
		if !all && len(watched) == 0 {
			return nil
		}
		time.Sleep(interval)
	}
}
