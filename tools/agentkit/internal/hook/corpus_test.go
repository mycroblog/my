// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// reOldRoot matches the working trees past sessions ran in, which are gone: the corpus is evaluated as if every
// command had run in this one.
var reOldRoot = regexp.MustCompile(`/home/[^/\s"';&|()<>]+/src/github\.com/[^/\s"';&|()<>]+/(?:my-(?:wt|worktrees)/[^/\s"';&|()<>]+|my[^/\s"';&|()<>]*)(?:/\.claude/worktrees/[^/\s"';&|()<>]+)?`)

// TestCorpus runs the rules over recorded Bash commands and writes what each would deny. It is a measuring tool,
// not a check: set AGENTKIT_HOOK_CORPUS to a file of JSON lines with the command in "c", and AGENTKIT_HOOK_RESULT
// to where the verdicts go ({"i": line, "rule": ..., "reason": ...} per command).
func TestCorpus(t *testing.T) {
	corpus := os.Getenv("AGENTKIT_HOOK_CORPUS")
	if corpus == "" {
		t.Skip("AGENTKIT_HOOK_CORPUS is not set")
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	c := &checker{missingIsRepo: true}
	c.session = c.rootOf(wd)
	if c.session == "" {
		t.Fatal("not in a working tree with scripts/agentkit.sh")
	}
	f, err := os.Open(filepath.Clean(corpus)) //nolint:gosec // the corpus the developer names
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	out := os.Stdout
	if p := os.Getenv("AGENTKIT_HOOK_RESULT"); p != "" {
		if out, err = os.Create(filepath.Clean(p)); err != nil { //nolint:gosec // the file the developer names
			t.Fatal(err)
		}
		defer func() { _ = out.Close() }()
	}
	enc := json.NewEncoder(out)
	counts := map[string]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	n := 0
	for i := 0; sc.Scan(); i++ {
		var row struct {
			C string `json:"c"`
		}
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		n++
		cmd := reOldRoot.ReplaceAllString(row.C, c.session)
		rule, reason := "", ""
		func() {
			defer func() {
				if recover() != nil {
					rule = "panic"
				}
			}()
			if d := c.check(cmd); d != nil {
				rule, reason = d.rule, d.reason
			}
		}()
		counts[rule]++
		_ = enc.Encode(map[string]any{"i": i, "rule": rule, "reason": reason})
	}
	rules := make([]string, 0, len(counts))
	for r := range counts {
		rules = append(rules, r)
	}
	sort.Strings(rules)
	for _, r := range rules {
		t.Logf("%-12s %6d  %.1f%%", r, counts[r], 100*float64(counts[r])/float64(n))
	}
	if counts["panic"] > 0 {
		t.Errorf("%d commands made a rule panic", counts["panic"])
	}
}
