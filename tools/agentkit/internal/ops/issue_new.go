// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/mycroblog/my/tools/agentkit/internal/gh"
	"github.com/mycroblog/my/tools/agentkit/internal/text"
)

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

const blockedByQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    issue(number: $number) { blockedBy(first: 100) { nodes { number state } } }
  }
}`

// openBlockers returns the open issues that block issue n (GitHub's issue dependencies, which issue new
// --blocked-by records); a var so that tests replace the gh call.
var openBlockers = func(n int) ([]int, error) {
	owner, name, _ := strings.Cut(gh.Repo(), "/")
	var resp struct {
		Data struct {
			Repository struct {
				Issue struct {
					BlockedBy struct {
						Nodes []struct {
							Number int    `json:"number"`
							State  string `json:"state"`
						} `json:"nodes"`
					} `json:"blockedBy"`
				} `json:"issue"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := gh.JSON(&resp, "api", "graphql", "-f", "query="+blockedByQuery, "-F", "owner="+owner, "-F", "name="+name, "-F", "number="+strconv.Itoa(n)); err != nil {
		return nil, err
	}
	var open []int
	for _, b := range resp.Data.Repository.Issue.BlockedBy.Nodes {
		if b.State == "OPEN" {
			open = append(open, b.Number)
		}
	}
	return open, nil
}

// milestoneTitles is the open milestones as one quoted list for a message.
func milestoneTitles() string {
	var ms []struct {
		Title string `json:"title"`
	}
	if err := gh.JSON(&ms, "api", "repos/"+gh.Repo()+"/milestones?state=open&per_page=100"); err != nil {
		return "unread: " + err.Error()
	}
	if len(ms) == 0 {
		return "none"
	}
	titles := make([]string, len(ms))
	for i, m := range ms {
		titles[i] = strconv.Quote(m.Title)
	}
	return strings.Join(titles, ", ")
}

// NewOptions describe the issue New creates.
type NewOptions struct {
	Type      string // one of gh.IssueTypes
	Title     string
	Body      string
	Parent    int    // the epic the issue is a sub-issue of; 0 for none
	Milestone string // a milestone title; with a parent, the parent's
	Areas     []string
	Priority  string // one of gh.Priorities, or ""
	Ready     bool
	BlockedBy []int // the issues to finish first
}

// reIssueListLine matches a line of a description that lists an issue: a bullet, a task-list item, a numbered
// item, or a table row that starts with an issue reference.
var reIssueListLine = regexp.MustCompile(`^\s*(?:(?:[-*+]|\d+[.)])\s+(?:\[[ xX]\]\s*)?|\|\s*)(?:#\d+|https://github\.com/[^\s/]+/[^\s/]+/issues/\d+)`)

// epicBodyProblem returns why body cannot be an epic's description, or "".
func epicBodyProblem(body string) string {
	var hits []string
	for _, l := range strings.Split(body, "\n") {
		if reIssueListLine.MatchString(l) {
			hits = append(hits, strings.TrimSpace(l))
		}
	}
	if len(hits) == 0 {
		return ""
	}
	return fmt.Sprintf("the description lists issues (%d lines, the first: %q): an epic's tasks are its sub-issues; the description holds the goal and when it is done. "+
		"Create each task with `agentkit issue new --parent <epic>`; order goes into --blocked-by <n>", len(hits), text.Truncate(hits[0], 80))
}

// restIssue is what New reads of an issue through the REST API: the id the sub-issue and dependency calls
// take, and, in the answer of the sub-issue call, the parent's count.
type restIssue struct {
	ID          int64     `json:"id"`
	Number      int       `json:"number"`
	State       string    `json:"state"`
	PullRequest *struct{} `json:"pull_request"`
	Summary     struct {
		Total     int `json:"total"`
		Completed int `json:"completed"`
	} `json:"sub_issues_summary"`
}

func readRESTIssue(n int) (restIssue, error) {
	var f restIssue
	err := gh.JSON(&f, "api", fmt.Sprintf("repos/%s/issues/%d", gh.Repo(), n))
	return f, err
}

func oneOf(list []string, s string) bool { return contains(list, s) }

var reIssueURL = regexp.MustCompile(`/issues/(\d+)\s*$`)

const newUsage = "agentkit issue new --type task|bug|spike|adr|epic --title <t> --body-file <f> [--parent <epic>] [--milestone <title>] [--area <a>]... [--priority p0|p1|p2] [--blocked-by <n>]... [--ready]"

// New creates an issue the one way the repository takes: typed, in a milestone, and — unless it is an epic or
// found outside every epic — a sub-issue of an open epic, whose milestone it gets. It checks everything before
// it creates anything, and returns the lines to print: the URL, then the links it made.
func New(o NewOptions) ([]string, error) {
	if !oneOf(gh.IssueTypes, o.Type) {
		return nil, fmt.Errorf("--type %q: one of %s (%s)", o.Type, strings.Join(gh.IssueTypes, ", "), newUsage)
	}
	o.Title = strings.TrimSpace(o.Title)
	if o.Title == "" || strings.TrimSpace(o.Body) == "" {
		return nil, fmt.Errorf("--title and a non-empty --body-file are required (%s)", newUsage)
	}
	if strings.HasPrefix(o.Title, "[") {
		return nil, fmt.Errorf("the title %q starts with a prefix: --type records the type, so write the title without one", o.Title)
	}
	if o.Priority != "" && !oneOf(gh.Priorities, o.Priority) {
		return nil, fmt.Errorf("--priority %q: one of %s", o.Priority, strings.Join(gh.Priorities, ", "))
	}
	epics := fmt.Sprintf("gh issue list -R %s %s", gh.Repo(), gh.EpicFilter())
	if o.Type == "epic" {
		if o.Parent != 0 {
			return nil, fmt.Errorf("an epic has no parent: give it --milestone <title> instead of --parent")
		}
		if p := epicBodyProblem(o.Body); p != "" {
			return nil, fmt.Errorf("%s", p)
		}
	}
	switch {
	case o.Parent == 0 && o.Milestone == "" && o.Type == "epic":
		return nil, fmt.Errorf("an epic needs --milestone <title>; open milestones: %s", milestoneTitles())
	case o.Parent == 0 && o.Milestone == "":
		return nil, fmt.Errorf("a %s needs --parent <epic> (it becomes the epic's sub-issue and gets its milestone; %s), or --milestone <title> when it belongs to no epic; open milestones: %s",
			o.Type, epics, milestoneTitles())
	case o.Parent != 0:
		pa, err := gh.GetIssue(o.Parent)
		if err != nil {
			return nil, fmt.Errorf("could not read the parent #%d: %w", o.Parent, err)
		}
		switch {
		case !pa.IsEpic():
			return nil, fmt.Errorf("the parent #%d %s is not an epic: pass the epic the work belongs to (%s), or --milestone <title> without --parent for an issue outside every epic",
				o.Parent, pa.Title, epics)
		case pa.State != "OPEN":
			return nil, fmt.Errorf("the parent #%d %s is closed: pass an open epic (%s), or reopen it (gh issue reopen %d -R %s)", o.Parent, pa.Title, epics, o.Parent, gh.Repo())
		case pa.Milestone == nil:
			return nil, fmt.Errorf("the parent #%d %s has no milestone, so its sub-issues would have none: gh issue edit %d -R %s --milestone \"<title>\" (open milestones: %s), then run this again",
				o.Parent, pa.Title, o.Parent, gh.Repo(), milestoneTitles())
		case o.Milestone != "" && o.Milestone != pa.Milestone.Title:
			return nil, fmt.Errorf("--milestone %q is not the milestone of the parent #%d (%q): a sub-issue gets its parent's milestone, so leave --milestone out",
				o.Milestone, o.Parent, pa.Milestone.Title)
		}
		o.Milestone = pa.Milestone.Title
	default:
		if open := milestoneTitles(); !strings.Contains(open, strconv.Quote(o.Milestone)) {
			return nil, fmt.Errorf("--milestone %q is not an open milestone; open milestones: %s (issue new creates no milestone)", o.Milestone, open)
		}
	}
	blockers := make([]restIssue, 0, len(o.BlockedBy))
	for _, b := range o.BlockedBy {
		f, err := readRESTIssue(b)
		if err != nil {
			return nil, fmt.Errorf("--blocked-by %d: could not read the issue: %w", b, err)
		}
		if f.PullRequest != nil {
			return nil, fmt.Errorf("--blocked-by %d: that is a pull request; pass the issue it closes", b)
		}
		blockers = append(blockers, f)
	}

	file, err := os.CreateTemp("", "agentkit-issue-*.md")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err := file.WriteString(o.Body); err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	args := []string{"issue", "create", "-R", gh.Repo(), "--title", o.Title, "--body-file", file.Name(), "--milestone", o.Milestone}
	for _, a := range o.Areas {
		args = append(args, "--label", "area:"+strings.TrimPrefix(a, "area:"))
	}
	out, err := gh.Run(args...)
	if err != nil {
		return nil, err
	}
	url := strings.TrimSpace(string(out))
	lines := []string{url}
	m := reIssueURL.FindStringSubmatch(url)
	if m == nil {
		return lines, fmt.Errorf("gh printed no issue URL (%q): find the issue it created and set its type, parent and dependencies by hand", url)
	}
	n, _ := strconv.Atoi(m[1])

	// From here the issue exists: a failure says what is left to do instead of creating it again.
	var left []string
	if err := gh.SetType(n, o.Type); err != nil {
		left = append(left, fmt.Sprintf("set the type %s: %v", o.Type, err))
	}
	if o.Priority != "" {
		if err := gh.SetPriority(n, o.Priority); err != nil {
			left = append(left, fmt.Sprintf("set the priority %s: %v", o.Priority, err))
		}
	}
	if o.Parent != 0 {
		if pa, err := linkSubIssue(o.Parent, n); err != nil {
			left = append(left, fmt.Sprintf("link it as a sub-issue of #%d: gh api -X POST repos/%s/issues/%d/sub_issues -F sub_issue_id=$(gh api repos/%s/issues/%d --jq .id) (%v)",
				o.Parent, gh.Repo(), o.Parent, gh.Repo(), n, err))
		} else {
			lines = append(lines, fmt.Sprintf("sub-issue of #%d (%d of %d open)", o.Parent, pa.Summary.Total-pa.Summary.Completed, pa.Summary.Total))
		}
	}
	for _, b := range blockers {
		if _, err := gh.Run("api", "-X", "POST", fmt.Sprintf("repos/%s/issues/%d/dependencies/blocked_by", gh.Repo(), n), "-F", "issue_id="+strconv.FormatInt(b.ID, 10)); err != nil {
			left = append(left, fmt.Sprintf("record that #%d blocks it: gh api -X POST repos/%s/issues/%d/dependencies/blocked_by -F issue_id=%d (%v)", b.Number, gh.Repo(), n, b.ID, err))
			continue
		}
		lines = append(lines, fmt.Sprintf("blocked by #%d (%s)", b.Number, b.State))
	}
	// Ready last: an issue is not offered to take before its parent and its blockers are recorded.
	if o.Ready {
		if len(left) > 0 {
			left = append(left, "then mark it ready")
		} else if err := gh.MarkReady(n); err != nil {
			left = append(left, fmt.Sprintf("mark it ready: %v", err))
		}
	}
	if len(left) > 0 {
		return lines, fmt.Errorf("#%d is created (do not create it again); still to do: %s", n, strings.Join(left, "; "))
	}
	return lines, nil
}

// linkSubIssue makes issue n a sub-issue of parent and returns the parent as the API answers it.
func linkSubIssue(parent, n int) (restIssue, error) {
	var pa restIssue
	me, err := readRESTIssue(n)
	if err != nil {
		return pa, err
	}
	err = gh.JSON(&pa, "api", "-X", "POST", fmt.Sprintf("repos/%s/issues/%d/sub_issues", gh.Repo(), parent), "-F", "sub_issue_id="+strconv.FormatInt(me.ID, 10))
	return pa, err
}
