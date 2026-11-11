// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/mycroblog/my/tools/agentkit/internal/adr"
	"github.com/mycroblog/my/tools/agentkit/internal/gh"
	"github.com/mycroblog/my/tools/agentkit/internal/text"
)

var reStatus = regexp.MustCompile(`(?m)^- Status:\s*([A-Za-z]+)`)

// ADRStatus reads the status of ADR id (ADR-0070) from docs/adr in root, following a retired number to the ADR
// that holds it now; "" when the ADR does not exist.
func ADRStatus(root, id string) string {
	aliases, err := adr.LoadAliases(root)
	if err != nil {
		return "unknown"
	}
	num := strings.TrimPrefix(aliases.Current(id), "ADR-")
	ms, _ := filepath.Glob(filepath.Join(root, "docs", "adr", num+"-*.md"))
	if len(ms) == 0 {
		return ""
	}
	b, err := os.ReadFile(ms[0])
	if err != nil {
		return ""
	}
	if m := reStatus.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return "unknown"
}

// Me returns the login of the gh user.
func Me() (string, error) {
	out, err := gh.Run("api", "user", "--jq", ".login")
	return strings.TrimSpace(string(out)), err
}

// Claim takes issue n by the rules of workflow.md §1: open, of a type that is taken as work, nobody else
// assigned, in a milestone, every referenced ADR Accepted, nothing it waits for still open (the issues that
// block it on GitHub — a decision, an investigation, or other work — and those its description names). It
// assigns the gh user and comments the plan. It returns the line to print.
func Claim(root string, n int, plan string) (string, error) {
	is, err := gh.GetIssue(n)
	if err != nil {
		return "", err
	}
	me, err := Me()
	if err != nil {
		return "", err
	}
	var stop []string
	if is.State != "OPEN" {
		stop = append(stop, "the issue is "+is.State)
	}
	if why := is.Unclaimable(); why != "" {
		stop = append(stop, why)
	}
	mine := false
	for _, a := range is.Assignees {
		if a.Login == me {
			mine = true
		} else {
			stop = append(stop, "assigned to @"+a.Login)
		}
	}
	for _, id := range text.ADRs(is.Body) {
		if st := ADRStatus(root, id); st != "Accepted" {
			if st == "" {
				st = "missing from this tree"
			}
			stop = append(stop, fmt.Sprintf("%s is %s", id, st))
		}
	}
	if is.Milestone == nil {
		stop = append(stop, fmt.Sprintf("it has no milestone (gh issue edit %d -R %s --milestone \"<title>\"; open milestones: %s)", n, gh.Repo(), milestoneTitles()))
	}
	blocking := map[string]bool{}
	for _, b := range is.OpenBlockers() {
		blocking[strconv.Itoa(b.Number)] = true
		stop = append(stop, b.Waits())
	}
	var named []string
	for _, d := range text.Dependencies(is.Body) {
		if !blocking[d] {
			named = append(named, d)
		}
	}
	waits, err := openDependencies(named)
	if err != nil {
		return "", fmt.Errorf("do not take #%d: %w (workflow.md §1)", n, err)
	}
	stop = append(stop, waits...)
	if len(stop) > 0 {
		return "", fmt.Errorf("do not take #%d: %s (workflow.md §1)", n, strings.Join(stop, "; "))
	}
	did := []string{}
	if !mine {
		if _, err := gh.Run("issue", "edit", strconv.Itoa(n), "-R", gh.Repo(), "--add-assignee", "@me"); err != nil {
			return "", err
		}
		did = append(did, "assigned @"+me)
	} else {
		did = append(did, "already assigned to @"+me)
	}
	if plan != "" {
		if _, err := gh.Run("issue", "comment", strconv.Itoa(n), "-R", gh.Repo(), "--body", "Plan: "+plan); err != nil {
			return "", err
		}
		did = append(did, "plan commented")
	}
	return fmt.Sprintf("claimed #%d %s (%s)", n, is.Title, strings.Join(did, ", ")), nil
}

// itemState is the state ("OPEN", "CLOSED", "MERGED") of the issue or pull request number d; a var so that tests
// replace the gh calls.
var itemState = func(kind, d string) (string, error) {
	var st struct {
		State string `json:"state"`
	}
	err := gh.JSON(&st, kind, "view", d, "-R", gh.Repo(), "--json", "state")
	return st.State, err
}

// openDependencies returns a line for each of the issues or pull requests in deps that is still open. A
// dependency whose state cannot be read is an error: it must not count as done.
func openDependencies(deps []string) ([]string, error) {
	var stop []string
	for _, d := range deps {
		num, _ := strconv.Atoi(d)
		st, err := itemState("issue", d)
		if err == nil {
			if st == "OPEN" {
				stop = append(stop, fmt.Sprintf("waits for #%d, which is open", num))
			}
			continue
		}
		st, prErr := itemState("pr", d)
		if prErr != nil {
			return nil, fmt.Errorf("could not read the state of the dependency #%d (issue: %w; pull request: %w)", num, err, prErr)
		}
		if st == "OPEN" {
			stop = append(stop, fmt.Sprintf("waits for PR #%d, which is open", num))
		}
	}
	return stop, nil
}

// BlockOptions say what issue N waits for: the existing issue On, or a new issue of Type with Title and Reason.
type BlockOptions struct {
	N      int
	On     int    // an existing issue to wait for; 0 to create one
	Type   string // one of gh.BlockerTypes: adr (a decision of the decider) or spike (an investigation)
	Title  string
	Reason string // the new issue's description
}

const blockUsage = "agentkit issue block <n> --type adr|spike --title <what is needed> --reason-file <f>, or issue block <n> --on <issue>"

// Block records what stops the work on issue o.N (workflow.md §8) as an issue that blocks it: the existing
// issue o.On, or a new one of o.Type in the milestone of o.N, whose description is the reason and which it
// links in a comment. It returns the lines to print.
func Block(o BlockOptions) ([]string, error) {
	if o.On != 0 {
		if o.On == o.N {
			return nil, fmt.Errorf("--on %d: an issue does not wait for itself", o.On)
		}
		b, err := readRESTIssue(o.On)
		if err != nil {
			return nil, fmt.Errorf("--on %d: could not read the issue: %w", o.On, err)
		}
		if b.PullRequest != nil {
			return nil, fmt.Errorf("--on %d: that is a pull request; pass the issue it closes", o.On)
		}
		if err := addBlocker(o.N, b); err != nil {
			return nil, err
		}
		return []string{fmt.Sprintf("blocked #%d: it waits for #%d (%s)", o.N, b.Number, b.State)}, nil
	}
	o.Title = strings.TrimSpace(o.Title)
	switch {
	case !oneOf(gh.BlockerTypes, o.Type):
		return nil, fmt.Errorf("--type %q: adr when the decider has to decide, spike when an investigation comes first (%s)", o.Type, blockUsage)
	case o.Title == "":
		return nil, fmt.Errorf("--title is required: the decision or the investigation needed, in one line (%s)", blockUsage)
	case strings.TrimSpace(o.Reason) == "":
		return nil, fmt.Errorf("a reason is required: what you tried, the decision needed, the options, your recommendation (%s)", blockUsage)
	}
	is, err := gh.GetIssue(o.N)
	if err != nil {
		return nil, err
	}
	if is.Milestone == nil {
		return nil, fmt.Errorf("#%d has no milestone, so the issue it waits for would have none: gh issue edit %d -R %s --milestone \"<title>\" (open milestones: %s), then run this again",
			o.N, o.N, gh.Repo(), milestoneTitles())
	}
	lines, err := New(NewOptions{Type: o.Type, Title: o.Title, Body: o.Reason, Milestone: is.Milestone.Title})
	if err != nil {
		return lines, err
	}
	m, _ := strconv.Atoi(reIssueURL.FindStringSubmatch(lines[0])[1])
	b, err := readRESTIssue(m)
	if err == nil {
		err = addBlocker(o.N, b)
	}
	if err != nil {
		return lines, fmt.Errorf("#%d is created (do not create it again); still to do: agentkit issue block %d --on %d (%w)", m, o.N, m, err)
	}
	waits := gh.Blocker{Number: m, Title: o.Title, Type: gh.TypeName(o.Type)}.Waits()
	lines = append(lines, fmt.Sprintf("blocked #%d: it %s", o.N, waits))
	if _, err := gh.Run("issue", "comment", strconv.Itoa(o.N), "-R", gh.Repo(), "--body", fmt.Sprintf("This %s\n\n%s", waits, lines[0])); err != nil {
		return lines, fmt.Errorf("#%d blocks #%d, but the comment that links it failed: %w", m, o.N, err)
	}
	return lines, nil
}

// addBlocker records on GitHub that issue n waits for b.
func addBlocker(n int, b restIssue) error {
	path := fmt.Sprintf("repos/%s/issues/%d/dependencies/blocked_by", gh.Repo(), n)
	if _, err := gh.Run("api", "-X", "POST", path, "-F", "issue_id="+strconv.FormatInt(b.ID, 10)); err != nil {
		return fmt.Errorf("record that #%d blocks #%d: gh api -X POST %s -F issue_id=%d (%w)", b.Number, n, path, b.ID, err)
	}
	return nil
}
