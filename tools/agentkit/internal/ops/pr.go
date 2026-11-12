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

	"github.com/mycroblog/my/tools/agentkit/internal/gh"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// OpenOptions are the parts of a pull request the agent writes; the rest comes from the issue and the commits.
type OpenOptions struct {
	// Issues are the issues the pull request closes; empty takes the issue of the branch name and those the
	// commit subjects name as "(#N)".
	Issues    []int
	Base      string
	Title     string
	Summary   string
	Decisions string
	Footer    string // e.g. the attribution line the agent's harness asks for
	Confirm   bool   // the agent confirms the docs and no-secrets items of the checklist
	Draft     bool
	NoPush    bool
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

// Open passes the branch through the gate (gate.go: a worktree, a linear history, the quick checks), pushes it,
// and opens its pull request from .github/pull_request_template.md's sections: the summary, Closes #N for each
// issue (or Stacked on #M first), the commit subjects as the changes, the issues' acceptance criteria with the checked state they have in the issue, the decisions, and the checklist. It prints the documents that describe the changed paths and are not
// changed (docImpact), and opens the PR either way. It comments the PR on each issue and returns the PR URL.
func Open(r *repo.Repo, o OpenOptions) (string, error) {
	branch, err := r.Branch()
	if err != nil {
		return "", err
	}
	if isTrunk(branch) || branch == "HEAD" {
		return "", fmt.Errorf("on %s: work on a <type>/<issue>-<slug> branch (/work-on-issue)", branch)
	}
	if o.Base == "" {
		o.Base = repo.Trunk
	}
	if err := gate(r, branch, "origin/"+o.Base); err != nil {
		return "", err
	}
	for _, l := range docImpact(r, "origin/"+o.Base) {
		fmt.Fprintln(os.Stderr, l)
	}
	logOut, err := repo.Git(r.Root, "log", "--reverse", "--no-merges", "--format=%s", "origin/"+o.Base+"..HEAD")
	if err != nil {
		return "", err
	}
	subjects := strings.Split(strings.TrimSpace(logOut), "\n")
	if len(subjects) == 0 || subjects[0] == "" {
		return "", fmt.Errorf("no commits on %s since origin/%s", branch, o.Base)
	}
	issues, err := closing(o.Issues, branch, subjects)
	if err != nil {
		return "", err
	}
	if !o.NoPush {
		// A rebased branch replaces what was pushed; the lease refuses when someone else pushed meanwhile.
		if out, err := exec.Command("git", "-C", r.Root, "push", "--force-with-lease", "-u", "origin", "HEAD").CombinedOutput(); err != nil {
			return "", fmt.Errorf("git push: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	if o.Title == "" {
		o.Title = subjects[0]
	}

	var b strings.Builder
	if o.Base != repo.Trunk {
		var prs []struct {
			Number int `json:"number"`
		}
		if err := gh.JSON(&prs, "pr", "list", "-R", gh.Repo(), "--head", o.Base, "--json", "number"); err == nil && len(prs) > 0 {
			fmt.Fprintf(&b, "Stacked on #%d\n\n", prs[0].Number)
		}
	}
	summary := strings.TrimSpace(o.Summary)
	if summary == "" && len(issues) > 0 {
		summary = issues[0].Title
	}
	fmt.Fprintf(&b, "## Summary\n\n%s\n\n", summary)
	for _, is := range issues {
		fmt.Fprintf(&b, "Closes #%d\n", is.Number)
	}
	if len(issues) > 0 {
		b.WriteString("\n")
	}
	b.WriteString("## Changes\n\n")
	for _, s := range subjects {
		fmt.Fprintf(&b, "- %s\n", s)
	}
	b.WriteString("\n## Acceptance criteria\n\n")
	if len(issues) == 0 {
		b.WriteString("No issue: nothing to check.\n")
	}
	for i, is := range issues {
		if len(issues) > 1 {
			if i > 0 {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "#%d %s\n\n", is.Number, is.Title)
		}
		ac := acceptanceCriteria(is.Body)
		if len(ac) == 0 {
			b.WriteString("- [ ] (the issue has no checklist)\n")
		}
		for _, a := range ac {
			fmt.Fprintf(&b, "- [%s] %s\n", mark(a.Checked), a.Text)
		}
	}
	decisions := strings.TrimSpace(o.Decisions)
	if decisions == "" {
		decisions = "None"
	}
	fmt.Fprintf(&b, "\n## Decisions and assumptions\n\n%s\n\n## Checklist\n\n", decisions)
	fmt.Fprintf(&b, "- [%s] Docs / i18n catalogs updated where needed\n", mark(o.Confirm))
	fmt.Fprintf(&b, "- [%s] No secrets, no unrelated changes\n", mark(o.Confirm))
	if o.Footer != "" {
		fmt.Fprintf(&b, "\n%s\n", o.Footer)
	}

	// A branch that already has an open pull request (an epic's draft, which each issue adds a commit to) gets its
	// description rewritten, and only the issues it did not close yet are told.
	var existing []struct {
		Number int    `json:"number"`
		URL    string `json:"url"`
		Body   string `json:"body"`
	}
	if err := gh.JSON(&existing, "pr", "list", "-R", gh.Repo(), "--head", branch, "--state", "open", "--json", "number,url,body"); err == nil && len(existing) > 0 {
		pr := existing[0]
		edit := exec.Command("gh", "pr", "edit", strconv.Itoa(pr.Number), "-R", gh.Repo(), "--title", o.Title, "--body-file", "-")
		edit.Dir = r.Root
		edit.Stdin = strings.NewReader(b.String())
		if out, err := edit.CombinedOutput(); err != nil {
			return "", fmt.Errorf("gh pr edit %d: %w: %s", pr.Number, err, strings.TrimSpace(string(out)))
		}
		for _, is := range issues {
			if strings.Contains(pr.Body, fmt.Sprintf("Closes #%d\n", is.Number)) {
				continue
			}
			if _, err := gh.Run("issue", "comment", strconv.Itoa(is.Number), "-R", gh.Repo(), "--body", "PR: "+pr.URL); err != nil {
				return pr.URL, err
			}
		}
		return pr.URL, nil
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
	for _, is := range issues {
		if _, err := gh.Run("issue", "comment", strconv.Itoa(is.Number), "-R", gh.Repo(), "--body", "PR: "+url); err != nil {
			return url, err
		}
	}
	return url, nil
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

// runID is the Actions run of the check, or 0 for a check that is not an Actions job. Run IDs grow with time,
// so a larger one is a newer run.
func (c check) runID() int64 {
	m := reRun.FindStringSubmatch(c.DetailsURL + c.TargetURL)
	if m == nil {
		return 0
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	return id
}

// latest keeps only the checks of the newest run of each workflow and, of those, the newest check of each name.
// A push or a label starts the workflow again on the same head, and its concurrency group cancels the older run,
// whose checks stay in the rollup next to the newer ones. The cancelled run's gate fails, and the newer run has
// no check of that name until its gate starts, so the older run's checks are dropped as a whole, not name by
// name. A rerun keeps its run ID and gives each job a new job ID.
func latest(cs []check) []check {
	newest := map[string]int64{}
	for _, c := range cs {
		if id := c.runID(); id > newest[c.WorkflowName] {
			newest[c.WorkflowName] = id
		}
	}
	idx := map[string]int{}
	var out []check
	for _, c := range cs {
		if id := c.runID(); id != 0 && id < newest[c.WorkflowName] {
			continue
		}
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

// The exit statuses of Wait when the pull request's head moves while it waits, and when the timeout passes
// before CI ends.
const (
	WaitMoved   = 6
	WaitTimeout = 124
)

// headFollow tells a pull request whose head GitHub has not caught up with from one whose head moved away.
type headFollow struct {
	at    string          // the head that is followed; "" until the pull request is read at one
	been  map[string]bool // the heads the pull request was read at, and the pushes it went past
	stale string          // the pull request's head while it is behind a push
}

// observe takes the pull request's head of one poll and the commit the local branch pushed ("" when unknown).
// behind: GitHub has not seen that push yet, so the checks are an older commit's. from is the head that was
// followed when the pull request has moved to another one, which is followed from now on.
func (h *headFollow) observe(cur, pushed string) (behind bool, from string) {
	if h.been == nil {
		h.been = map[string]bool{}
	}
	if pushed != "" && cur != pushed && !h.been[pushed] {
		if h.stale == "" || h.stale == cur {
			h.stale = cur
			return true, ""
		}
		// The head changed and is still not the push: a later push went past it.
		h.been[pushed] = true
		if h.at == "" {
			h.at = pushed
		}
	}
	h.stale = ""
	h.been[cur] = true
	if h.at == "" || h.at == cur {
		h.at = cur
		return false, ""
	}
	from, h.at = h.at, cur
	return false, from
}

// WaitOptions are the waits of Wait and what it compares the pull request with.
type WaitOptions struct {
	// Branch and Head are the local branch and its commit: when the pull request's branch is Branch, the checks
	// count only once its head is Head, so that a wait right after a push does not read the previous commit's
	// results. Once the pull request was read at the head the wait is for, another head ends the wait.
	Branch, Head string
	Timeout      time.Duration
	Interval     time.Duration
	Heartbeat    time.Duration // how often a line says that the wait goes on; 0 is every 5 minutes
	All          bool          // wait for every check instead of returning at the first failure
	LogDir       string        // where the logs of failed jobs are saved
}

var (
	reJob = regexp.MustCompile(`/actions/runs/\d+/job/(\d+)`)
	reRun = regexp.MustCompile(`/actions/runs/(\d+)`)
)

// sayTo returns the function that prints one line to w at once, so that a wait that is killed leaves what it
// printed.
func sayTo(w io.Writer) func(format string, args ...any) {
	return func(format string, args ...any) {
		fmt.Fprintf(w, format+"\n", args...)
		if f, ok := w.(*os.File); ok {
			_ = f.Sync()
		}
	}
}

// tally is the names of the pending and the failed checks of the latest runs, sorted, and the number that passed.
func tally(cs []check) (pending, failed []string, passed int) {
	for _, c := range latest(cs) {
		switch c.result() {
		case "pending":
			pending = append(pending, c.name())
		case "failed":
			failed = append(failed, c.name())
		case "passed":
			passed++
		}
	}
	sort.Strings(pending)
	sort.Strings(failed)
	return pending, failed, passed
}

// shortSHA is the first characters of a commit.
func shortSHA(sha string) string {
	if len(sha) > 9 {
		return sha[:9]
	}
	return sha
}

// firstLine is the first line of an error's text.
func firstLine(err error) string {
	s, _, _ := strings.Cut(strings.TrimSpace(err.Error()), "\n")
	return s
}

// waitingLine is the line a wait prints when it starts.
func waitingLine(n int, p prState) string {
	if len(p.StatusCheckRollup) == 0 {
		return fmt.Sprintf("waiting for CI of #%d at %s: no check has started yet", n, shortSHA(p.HeadRefOid))
	}
	pending, _, _ := tally(p.StatusCheckRollup)
	return fmt.Sprintf("waiting for CI of #%d at %s: %d checks pending", n, shortSHA(p.HeadRefOid), len(pending))
}

// stillWaiting is the line a wait prints at each heartbeat.
func stillWaiting(p prState) string {
	pending, failed, passed := tally(p.StatusCheckRollup)
	line := fmt.Sprintf("still waiting: %d pending, %d passed", len(pending), passed)
	if len(failed) > 0 {
		line += fmt.Sprintf(", %d failed", len(failed))
	}
	return line
}

// stillPending names what a wait that reached its timeout was waiting for.
func stillPending(p prState) string {
	pending, failed, _ := tally(p.StatusCheckRollup)
	var out string
	switch {
	case len(p.StatusCheckRollup) == 0:
		out = "no check has started"
	case len(pending) > 0:
		out = "still pending: " + short(pending, 8)
	case !gatePassed(p.StatusCheckRollup):
		out = "the required check `" + gateName + "` has not reported"
	default:
		out = "no check is pending"
	}
	if len(failed) > 0 {
		out += "; failed: " + short(failed, 8)
	}
	return out
}

// defaultHeartbeat is how often a wait says that it goes on.
const defaultHeartbeat = 5 * time.Minute

// Wait polls a pull request until its CI ends, it conflicts, it merges, its head moves, or the timeout passes,
// and returns the exit status. It prints to w a line when it starts, a line at each heartbeat, and how it ended, each at once, so
// that no output means it never started. Without All it returns at the first failed check instead of waiting for
// the others. A failure comes with the failing lines of the failed jobs' logs, which are saved under LogDir.
func Wait(w io.Writer, n int, o WaitOptions) int {
	say := sayTo(w)
	if o.Heartbeat <= 0 {
		o.Heartbeat = defaultHeartbeat
	}
	started := time.Now()
	deadline := started.Add(o.Timeout)
	beat := started.Add(o.Heartbeat)
	var last *prState // the newest state of the pull request's own head
	var follow headFollow
	first := true
	for {
		p, err := viewPR(n)
		behind, from := false, ""
		if err == nil && p.State != "MERGED" && p.State != "CLOSED" {
			pushed := ""
			if p.HeadRefName == o.Branch {
				pushed = o.Head
			}
			behind, from = follow.observe(p.HeadRefOid, pushed)
		}
		switch {
		case err != nil:
			// gh and the API fail now and then; give up only at the deadline.
		case p.State == "MERGED":
			say("PR #%d is merged", n)
			return 0
		case p.State == "CLOSED":
			say("PR #%d is closed", n)
			return 1
		case from != "":
			// The checks of the head the wait is for are cancelled or replaced: its CI can no longer end.
			say("PR #%d: the head moved from %s to %s while waiting; run pr wait again for the new head", n, shortSHA(from), shortSHA(p.HeadRefOid))
			return WaitMoved
		case behind:
			// GitHub has not seen the push yet.
		case p.Mergeable == "CONFLICTING":
			say("PR #%d conflicts with its base: CI does not run. Rebase onto "+trunkRef()+", regenerate generated files (agentkit regen), push with --force-with-lease", n)
			return 3
		default:
			last = &p
			res, failed, pending := summarize(p.StatusCheckRollup)
			if res == "failed" && (!o.All || !pending) {
				lines, _ := failureReport(n, p, failed, o.LogDir)
				say("%s", strings.Join(lines, "\n"))
				return 1
			}
			if res == "cancelled" {
				say("PR #%d: CI cancelled, not failed: %s; no newer run replaced them. Rerun the workflow (gh run rerun) or push", n, strings.Join(failed, ", "))
				return 4
			}
			if res == "passed" {
				say("PR #%d: CI passed (%d checks, mergeable %s)", n, len(latest(p.StatusCheckRollup)), strings.ToLower(p.Mergeable))
				return 0
			}
			if res == "none" && time.Since(started) > 10*time.Minute {
				say("PR #%d: no check started in 10 minutes (mergeable %s); check the PR page %s", n, strings.ToLower(p.Mergeable), p.URL)
				return 2
			}
		}
		now := time.Now()
		switch {
		case first && err != nil:
			say("waiting for CI of #%d: gh could not read it yet (%s)", n, firstLine(err))
		case first && behind:
			say("waiting for CI of #%d at %s: GitHub has not seen the push yet", n, shortSHA(o.Head))
		case first:
			say("%s", waitingLine(n, p))
		case now.After(beat) && last != nil:
			say("%s", stillWaiting(*last))
		case now.After(beat):
			say("still waiting: the checks of the pull request's head could not be read yet")
		}
		if first || now.After(beat) {
			first, beat = false, now.Add(o.Heartbeat)
		}
		if now.After(deadline) {
			what := "the pull request was never read at its head"
			if last != nil {
				what = stillPending(*last)
			}
			say("PR #%d: timed out after %s, CI has not ended: %s", n, o.Timeout, what)
			return WaitTimeout
		}
		time.Sleep(o.Interval)
	}
}

// maxFailureLogs is how many failed jobs' logs failureReport reads to tell whether each failed in the same way;
// it prints the failing lines of the first shownFailures.
const (
	maxFailureLogs = 12
	shownFailures  = 3
)

// failureReport returns the lines that say which checks of p failed and why — the failing lines of the failed
// jobs' logs, which it saves under logDir — and the command that runs the failed jobs again when the logs show
// that every failed job stopped on the same unreachable host before any test ran (the first line says so),
// or "" when they do not.
func failureReport(n int, p prState, failed []string, logDir string) (lines []string, rerun string) {
	shown, read := 0, 0
	infra := true
	var hosts map[string]int // the hosts every failed job read so far could not reach
	runs := map[int64]bool{}
	for _, c := range latest(p.StatusCheckRollup) {
		// `check` only collects the other jobs' results; its log says nothing new.
		if c.result() != "failed" || c.name() == gateName {
			continue
		}
		show := shown < shownFailures
		if !show && (!infra || read == maxFailureLogs) {
			infra = false
			continue
		}
		m := reJob.FindStringSubmatch(c.DetailsURL + c.TargetURL)
		if m == nil {
			infra = false
			if show {
				lines = append(lines, fmt.Sprintf("== %s: %s", c.name(), c.DetailsURL+c.TargetURL))
			}
			continue
		}
		if show {
			shown++
		}
		read++
		// The job's own log is readable as soon as the job ends; `gh run view --log` waits for the whole run.
		out, err := gh.Run("api", "repos/"+gh.Repo()+"/actions/jobs/"+m[1]+"/logs")
		if err != nil {
			infra = false
			if show {
				lines = append(lines, fmt.Sprintf("== %s: could not read the log: %v", c.name(), err))
			}
			continue
		}
		file := filepath.Join(logDir, fmt.Sprintf("pr-%d-%s.log", n, m[1]))
		_ = os.WriteFile(file, out, 0o600)
		if show {
			lines = append(lines, fmt.Sprintf("== %s (full log: %s)", c.name(), file))
			lines = append(lines, JobFailures(string(out), 40)...)
		}
		hosts = common(hosts, Unreachable(string(out)))
		infra = infra && len(hosts) > 0
		runs[c.runID()] = true
	}
	head := fmt.Sprintf("PR #%d: CI failed: %s", n, strings.Join(failed, ", "))
	if infra && len(hosts) > 0 {
		rerun = rerunCommand(runs)
		head += fmt.Sprintf("; failed before any test ran: the runner could not reach %s (rerun: %s)", most(hosts), rerun)
	}
	return append([]string{head}, lines...), rerun
}

// common adds the counts of next to those of the hosts that are in both; a nil all takes next as it is.
func common(all, next map[string]int) map[string]int {
	if all == nil {
		return next
	}
	out := map[string]int{}
	for h, c := range all {
		if next[h] > 0 {
			out[h] = c + next[h]
		}
	}
	return out
}

// most returns the host with the highest count, the first by name among equals.
func most(hosts map[string]int) string {
	best := ""
	for h, c := range hosts {
		if best == "" || c > hosts[best] || (c == hosts[best] && h < best) {
			best = h
		}
	}
	return best
}

// rerunCommand is the command that runs the failed jobs of the Actions runs again.
func rerunCommand(runs map[int64]bool) string {
	ids := make([]int64, 0, len(runs))
	for id := range runs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	cmds := make([]string, len(ids))
	for i, id := range ids {
		cmds[i] = fmt.Sprintf("gh run rerun %d --failed", id)
	}
	return strings.Join(cmds, " && ")
}

// prSnapshot is what Watch compares between polls.
type prSnapshot struct {
	ci, mergeable string
	failed        []string
	queued        bool   // auto-merge on, or in the merge queue
	head          string // the commit the checks are of
}

// transitions returns the events between two snapshots of an open pull request; had is false on its first poll.
// A new head is an event, and what the previous head's CI was says nothing about it: the state of the new
// head's CI is reported again even when it is the same.
func transitions(old prSnapshot, had bool, cur prSnapshot) []string {
	var out []string
	dequeued := "dequeued (out of the merge queue without merging: a conflict or a failed merge group)"
	if had && old.head != cur.head {
		out = append(out, "head-moved "+shortSHA(cur.head))
		old = prSnapshot{queued: old.queued}
		dequeued = "dequeued (the push took it out of the merge queue; queue it again)"
	}
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
		out = append(out, dequeued)
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

// Watch prints one line per event of the watched pull requests — opened, merged, closed, head-moved, conflicting,
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
			cur := prSnapshot{ci, p.Mergeable, failed, p.AutoMergeRequest != nil || queue[p.Number], p.HeadRefOid}
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
