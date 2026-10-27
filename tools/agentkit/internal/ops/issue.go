// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/mycroblog/my/tools/agentkit/internal/gh"
	"github.com/mycroblog/my/tools/agentkit/internal/text"
)

var reStatus = regexp.MustCompile(`(?m)^- Status:\s*([A-Za-z]+)`)

// ADRStatus reads the status of ADR id (ADR-0070) from docs/adr in root; "" when the ADR does not exist.
func ADRStatus(root, id string) string {
	num := strings.TrimPrefix(id, "ADR-")
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

// Claim takes issue n by the rules of workflow.md §1: open, agent:ready, not status:needs-decision, nobody
// else assigned, every referenced ADR Accepted, nothing it waits for still open. It assigns the gh user and
// comments the plan. It returns the line to print.
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
	if !is.HasLabel("agent:ready") {
		stop = append(stop, "not labeled agent:ready")
	}
	if is.HasLabel("status:needs-decision") {
		stop = append(stop, "labeled status:needs-decision")
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
	waits, err := openDependencies(text.Dependencies(is.Body))
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

// Block comments why the work stops and labels the issue agent:blocked (workflow.md §8).
func Block(n int, reason string) (string, error) {
	if strings.TrimSpace(reason) == "" {
		return "", fmt.Errorf("a reason is required: what you tried, the decision needed, the options, your recommendation")
	}
	if _, err := gh.Run("issue", "comment", strconv.Itoa(n), "-R", gh.Repo(), "--body", reason); err != nil {
		return "", err
	}
	if _, err := gh.Run("issue", "edit", strconv.Itoa(n), "-R", gh.Repo(), "--add-label", "agent:blocked"); err != nil {
		return "", err
	}
	return fmt.Sprintf("blocked #%d: commented and labeled agent:blocked", n), nil
}
