// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/ctxmap"
	"github.com/mycroblog/my/tools/agentkit/internal/docsync"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// treeKey identifies the content of the working tree (every path and its blob id), so that committing the
// checked changes keeps the key.
func treeKey(r *repo.Repo) string {
	files, err := r.Files()
	if err != nil {
		return ""
	}
	lines := make([]string, len(files))
	for i, f := range files {
		lines[i] = f.Path + " " + f.SHA
	}
	sort.Strings(lines)
	h := sha256.New()
	h.Write([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(h.Sum(nil))
}

func passedFile(r *repo.Repo) string {
	return filepath.Join(r.Root, ".tmp", "agentkit", "check", "passed")
}

// evidenceKey identifies what a pass of `agentkit check` covers: the content of the working tree, the commit of
// base it was compared with, and the files changed since the merge base (which decide the contexts checked). A
// pass computed against another base, or for another set of changed contexts, does not match. It is "" when any
// part cannot be read.
func evidenceKey(r *repo.Repo, base string) string {
	tree := treeKey(r)
	if tree == "" {
		return ""
	}
	baseSHA, err := repo.Git(r.Root, "rev-parse", "--verify", "--quiet", base+"^{commit}")
	if err != nil {
		return ""
	}
	mb, err := repo.Git(r.Root, "merge-base", base, "HEAD")
	if err != nil {
		return ""
	}
	changed, err := r.Changed(base)
	if err != nil {
		return ""
	}
	sort.Strings(changed)
	h := sha256.New()
	h.Write([]byte(tree + "\n" + strings.TrimSpace(baseSHA) + "\n" + strings.TrimSpace(mb) + "\n" + strings.Join(changed, "\n")))
	return hex.EncodeToString(h.Sum(nil))
}

// CheckPassed reports whether `agentkit check` passed on the current content of the working tree, compared with
// the same commit of base.
func CheckPassed(r *repo.Repo, base string) bool {
	key := evidenceKey(r, base)
	b, err := os.ReadFile(passedFile(r))
	return key != "" && err == nil && strings.TrimSpace(string(b)) == key
}

// task runs `scripts/task.sh args...` in the root of r, with its output combined.
func task(r *repo.Repo, args ...string) ([]byte, error) {
	cmd := exec.Command(filepath.Join(r.Root, "scripts", "task.sh"), args...)
	cmd.Dir = r.Root
	return cmd.CombinedOutput()
}

// Check runs `scripts/task.sh check:changed BASE=base` with its output in a log file, and returns the lines to
// print: the contexts it checks, and on failure the failing lines. It records a pass for `pr open`.
func Check(r *repo.Repo, base string) (bool, []string, error) {
	dir, err := r.TmpDir("check")
	if err != nil {
		return false, nil, err
	}
	log := filepath.Join(dir, time.Now().Format("20060102-150405")+".log")
	key := evidenceKey(r, base)
	start := time.Now()
	out, runErr := task(r, "check:changed", "BASE="+base)
	if err := os.WriteFile(log, out, 0o600); err != nil {
		return false, nil, err
	}
	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		l = plain(l)
		if strings.HasPrefix(l, "contexts changed since") || strings.HasPrefix(l, "also run") || strings.HasPrefix(l, "no check covers") {
			lines = append(lines, l)
		}
	}
	lines = append(lines, docImpact(r, base)...)
	rel, _ := filepath.Rel(r.Root, log)
	ok := runErr == nil
	if ok {
		if key != "" {
			_ = os.WriteFile(passedFile(r), []byte(key+"\n"), 0o600)
		}
		lines = append(lines, fmt.Sprintf("passed in %s (log: %s)", time.Since(start).Round(time.Second), rel))
		return true, lines, nil
	}
	_ = os.Remove(passedFile(r))
	lines = append(lines, "", "failing lines:")
	lines = append(lines, Failures(string(out), 80)...)
	lines = append(lines, fmt.Sprintf("FAILED after %s (full log: %s)", time.Since(start).Round(time.Second), rel))
	return false, lines, nil
}

// docImpact returns the summary lines of the user and operator documents that describe the changed paths and that
// the change does not touch: each needs a change, or a reason in the pull request (`pr open --doc-reason`).
func docImpact(r *repo.Repo, base string) []string {
	ims, err := docsync.Of(r, base)
	if err != nil {
		return []string{"docs impact: " + err.Error()}
	}
	var open []ctxmap.Impact
	for _, im := range ims {
		if !im.Changed {
			open = append(open, im)
		}
	}
	if len(open) == 0 {
		return nil
	}
	lines := []string{"documents that describe the changed paths and are not changed (update each, or give `pr open --doc-reason '<path>=<reason>'`):"}
	return append(lines, docsync.Lines(open)...)
}

// generators maps the sources of generated code to the task that regenerates it (workflow.md §7).
var generators = []struct {
	prefix, target string
}{
	{"api/spec/", "api:generate"},
	{"schema/", "go:sqlc"},
	{"queries/", "go:sqlc"},
	{"sqlc.yaml", "go:sqlc"},
	{"locales/", "i18n:gen"},
	{"app/ui/src/theme.css", "web:tokens-gen"},
	{"app/ui/src/tokens.ts", "web:tokens-gen"},
	{"internal/platform/config/", "go:config-docs"},
}

// Regen runs the generators the files changed since base need, in a stable order, and returns one line each.
func Regen(r *repo.Repo, base string, dryRun bool) ([]string, error) {
	changed, err := r.Changed(base)
	if err != nil {
		return nil, err
	}
	need := map[string]bool{}
	var order []string
	for _, g := range generators {
		for _, f := range changed {
			if strings.HasPrefix(f, g.prefix) && !need[g.target] {
				need[g.target] = true
				order = append(order, g.target)
			}
		}
	}
	if len(order) == 0 {
		return []string{"no generator source changed since " + base}, nil
	}
	var lines []string
	for _, t := range order {
		if dryRun {
			lines = append(lines, "would run scripts/task.sh "+t)
			continue
		}
		start := time.Now()
		out, err := task(r, t)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s: FAILED", t))
			lines = append(lines, Failures(string(out), 30)...)
			return lines, fmt.Errorf("scripts/task.sh %s failed", t)
		}
		lines = append(lines, fmt.Sprintf("%s: ok (%s)", t, time.Since(start).Round(time.Second)))
	}
	status, _ := repo.Git(r.Root, "status", "--short")
	if s := strings.TrimSpace(status); s != "" {
		lines = append(lines, "changed files now:", s)
	}
	return lines, nil
}
