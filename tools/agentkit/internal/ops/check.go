// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/ctxmap"
	"github.com/mycroblog/my/tools/agentkit/internal/docsync"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// task runs `scripts/task.sh args...` in the root of r, with its output combined.
func task(r *repo.Repo, args ...string) ([]byte, error) {
	cmd := exec.Command(filepath.Join(r.Root, "scripts", "task.sh"), args...)
	cmd.Dir = r.Root
	return cmd.CombinedOutput()
}

// runScript runs `scripts/<name> args...` in the root of r, with its output combined.
func runScript(r *repo.Repo, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(filepath.Join(r.Root, "scripts", name), args...)
	cmd.Dir = r.Root
	return cmd.CombinedOutput()
}

// Check runs `scripts/task.sh check:changed BASE=base` with its output in a log file, and returns the lines to
// print: the contexts it checks, and on failure the failing lines. Running it is optional.
func Check(r *repo.Repo, base string) (bool, []string, error) {
	dir, err := r.TmpDir("check")
	if err != nil {
		return false, nil, err
	}
	log := filepath.Join(dir, time.Now().Format("20060102-150405")+".log")
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
	// check:changed skips the tofu context without a tofu binary, but the workers check needs none.
	var workersLines []string
	if wOut, wErr := workersCheck(r); wErr != nil {
		ok = false
		workersLines = append(strings.Split(strings.TrimSpace(string(wOut)), "\n"), "run `agentkit regen` to rewrite deploy/opentofu/modules/workers/job_types.json")
	}
	if ok {
		lines = append(lines, fmt.Sprintf("passed in %s (log: %s)", time.Since(start).Round(time.Second), rel))
		return true, lines, nil
	}
	lines = append(lines, "", "failing lines:")
	if runErr != nil {
		lines = append(lines, Failures(string(out), 80)...)
	}
	lines = append(lines, workersLines...)
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

// workersScript keeps the job types of deploy/opentofu/modules/workers in step with internal/platform/job_types.go.
const workersScript = "tofu-workers-check.sh"

// workersCheck runs the workers check, which needs no tofu binary. A missing script (an older checkout) passes.
func workersCheck(r *repo.Repo) ([]byte, error) {
	script := filepath.Join(r.Root, "scripts", workersScript)
	if _, err := os.Stat(script); err != nil {
		return nil, nil
	}
	return runScript(r, workersScript)
}

// generators maps the sources of generated code to the task that regenerates it (workflow.md §7).
var generators = []struct {
	prefix, target string
	// script, when set, is run as `scripts/<script> args...` in place of the task.
	script string
	args   []string
}{
	{"api/spec/", "api:generate", "", nil},
	{"schema/", "go:sqlc", "", nil},
	{"queries/", "go:sqlc", "", nil},
	{"sqlc.yaml", "go:sqlc", "", nil},
	{"locales/", "i18n:gen", "", nil},
	{"app/ui/src/theme.css", "web:tokens-gen", "", nil},
	{"app/ui/src/tokens.ts", "web:tokens-gen", "", nil},
	{"internal/platform/config/", "go:config-docs", "", nil},
	{"cmd/my/", "go:cli-docs", "", nil},
	{"internal/platform/job_types.go", workersScript + " --write", workersScript, []string{"--write"}},
}

// Regen runs the generators the files changed since base need, in a stable order, and returns one line each.
func Regen(r *repo.Repo, base string, dryRun bool) ([]string, error) {
	changed, err := r.Changed(base)
	if err != nil {
		return nil, err
	}
	need := map[string]bool{}
	var order []string
	run := map[string]func() ([]byte, error){}
	for _, g := range generators {
		for _, f := range changed {
			if strings.HasPrefix(f, g.prefix) && !need[g.target] {
				need[g.target] = true
				order = append(order, g.target)
				if g.script != "" {
					g := g
					run[g.target] = func() ([]byte, error) { return runScript(r, g.script, g.args...) }
				} else {
					t := g.target
					run[t] = func() ([]byte, error) { return task(r, t) }
				}
			}
		}
	}
	if len(order) == 0 {
		return []string{"no generator source changed since " + base}, nil
	}
	var lines []string
	for _, t := range order {
		if dryRun {
			if strings.Contains(t, " ") {
				lines = append(lines, "would run scripts/"+t)
			} else {
				lines = append(lines, "would run scripts/task.sh "+t)
			}
			continue
		}
		start := time.Now()
		out, err := run[t]()
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s: FAILED", t))
			lines = append(lines, Failures(string(out), 30)...)
			return lines, fmt.Errorf("%s failed", t)
		}
		lines = append(lines, fmt.Sprintf("%s: ok (%s)", t, time.Since(start).Round(time.Second)))
	}
	status, _ := repo.Git(r.Root, "status", "--short")
	if s := strings.TrimSpace(status); s != "" {
		lines = append(lines, "changed files now:", s)
	}
	return lines, nil
}
