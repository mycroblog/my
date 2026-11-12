// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// fastStep is one check of `agentkit check`: a command that a pull request's preflight or a quick tool decides.
type fastStep struct {
	name string
	dir  string // relative to the working tree
	argv []string
	env  []string
	// listing marks a command that prints the offending files and exits 0 (gofmt -l).
	listing bool
}

// tsExts are the extensions `bun fmt` formats (FMT_FILES of tasks/web.yml).
var tsExts = map[string]bool{".ts": true, ".tsx": true, ".js": true, ".mjs": true}

// tsRoots are the TypeScript packages `bun fmt` formats.
var tsRoots = []string{"app/", "pkg/mykit/ts/", "pkg/mybot/ts/", "tools/i18n/"}

// tofuRoot is the directory tofu:fmt-check formats (tasks/tofu.yml), and tofuSuffixes are the files `tofu fmt`
// reads there.
const tofuRoot = "deploy/opentofu/"

var tofuSuffixes = []string{".tf", ".tofu", ".tfvars", ".tftest.hcl", ".tofutest.hcl"}

func isTofu(path string) bool {
	if !strings.HasPrefix(path, tofuRoot) {
		return false
	}
	for _, s := range tofuSuffixes {
		if strings.HasSuffix(path, s) {
			return true
		}
	}
	return false
}

// tofuConsistency are the checks of tofu:check that compare files and run no tofu: the configuration keys, the
// backend contract, the coverage of the settings, and the module sources against what the modules declare.
var tofuConsistency = []string{"tofu:env-check", "tofu:backend-fields", "tofu:coverage", "tofu:sources"}

// contextsScript prints the check contexts of a list of changed paths; the merge queue picks its jobs with it.
const contextsScript = "scripts/changed-contexts.sh"

// tofuContext reports whether the changed files select the tofu context of the merge queue, which more than the
// files of deploy/opentofu do: the configuration reference, the job types, and the guides are read by its checks.
func tofuContext(root string, changed []string) bool {
	script := filepath.Join(root, contextsScript)
	if _, err := os.Stat(script); err != nil || len(changed) == 0 {
		return false
	}
	list, err := os.CreateTemp("", "agentkit-changed-*")
	if err != nil {
		return false
	}
	defer os.Remove(list.Name())
	_, werr := list.WriteString(strings.Join(changed, "\n") + "\n")
	if cerr := list.Close(); werr != nil || cerr != nil {
		return false
	}
	out, err := exec.Command(script, "--files", list.Name()).Output()
	if err != nil {
		return false
	}
	for _, c := range strings.Fields(string(out)) {
		if c == "tofu" {
			return true
		}
	}
	return false
}

// stale maps the sources of committed generated files to the quick check that fails when those files are
// stale; `regen` rewrites them.
var stale = []struct {
	prefix string
	step   fastStep
}{
	{"api/spec/", fastStep{name: "generated: api:generate-check", argv: []string{"scripts/task.sh", "api:generate-check"}}},
	{"schema/", fastStep{name: "generated: go:sqlc-check", argv: []string{"scripts/task.sh", "go:sqlc-check"}}},
	{"queries/", fastStep{name: "generated: go:sqlc-check", argv: []string{"scripts/task.sh", "go:sqlc-check"}}},
	{"sqlc.yaml", fastStep{name: "generated: go:sqlc-check", argv: []string{"scripts/task.sh", "go:sqlc-check"}}},
	{"locales/", fastStep{name: "i18n:lint", argv: []string{"scripts/task.sh", "i18n:lint"}}},
	{"tools/i18n/", fastStep{name: "i18n:lint", argv: []string{"scripts/task.sh", "i18n:lint"}}},
	{"app/ui/src/theme.css", fastStep{name: "generated: web:tokens", argv: []string{"scripts/task.sh", "web:tokens"}}},
	{"app/ui/src/tokens.ts", fastStep{name: "generated: web:tokens", argv: []string{"scripts/task.sh", "web:tokens"}}},
	{"internal/platform/config/", fastStep{name: "generated: config docs", env: []string{"GOWORK=off"},
		argv: []string{"go", "test", "./internal/platform/config", "-run", "^Test(Reference|ExampleEnv|SettingsJSON)UpToDate$"}}},
	{"cmd/my/", fastStep{name: "generated: cli docs", env: []string{"GOWORK=off"},
		argv: []string{"go", "test", "./cmd/my", "-run", "^TestCLIReferenceUpToDate$"}}},
	{"internal/platform/job_types.go", fastStep{name: "generated: " + workersScript, argv: []string{"scripts/" + workersScript}}},
}

// goModule returns the directory of the Go module that holds the file path, relative to root ("." for the
// server module), or "" when no go.mod is above it.
func goModule(root, path string) string {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(root, dir, "go.mod")); err == nil {
			return dir
		}
		if dir == "." || dir == "/" {
			return ""
		}
	}
}

// fastPlan picks the checks of the changed files: each inner list runs in order, the lists side by side. Files
// that no longer exist select the checks of their area but are not passed to a tool.
func fastPlan(root string, changed []string) [][]fastStep {
	exists := func(p string) bool {
		st, err := os.Stat(filepath.Join(root, p))
		return err == nil && !st.IsDir()
	}
	var goFiles, tsFiles, appTS, shFiles, tofuFiles []string
	pkgs := map[string]map[string]bool{} // module -> package directories
	md, maps := false, false
	for _, f := range changed {
		ext := filepath.Ext(f)
		switch {
		case filepath.Base(f) == "context-map.yaml":
			maps = true
		case ext == ".go":
			mod := goModule(root, f)
			if mod == "" {
				continue
			}
			if exists(f) {
				goFiles = append(goFiles, f)
			}
			dir := filepath.Dir(f)
			if strings.Contains("/"+dir+"/", "/testdata/") {
				continue
			}
			if ms, _ := filepath.Glob(filepath.Join(root, dir, "*.go")); len(ms) == 0 {
				continue
			}
			if pkgs[mod] == nil {
				pkgs[mod] = map[string]bool{}
			}
			rel, _ := filepath.Rel(mod, dir)
			pkgs[mod]["./"+filepath.ToSlash(rel)] = true
		case tsExts[ext]:
			for _, r := range tsRoots {
				if strings.HasPrefix(f, r) {
					if exists(f) {
						tsFiles = append(tsFiles, f)
					}
					if r == "app/" {
						appTS = append(appTS, strings.TrimPrefix(f, "app/"))
					}
				}
			}
		case ext == ".md":
			md = true
		case ext == ".sh" && exists(f):
			shFiles = append(shFiles, f)
		case isTofu(f) && exists(f):
			tofuFiles = append(tofuFiles, f)
		}
	}
	var plan [][]fastStep
	if len(goFiles) > 0 {
		plan = append(plan, []fastStep{{name: "gofmt", argv: append([]string{"gofmt", "-l"}, goFiles...), listing: true}})
	}
	mods := make([]string, 0, len(pkgs))
	for m := range pkgs {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	for _, m := range mods {
		dirs := make([]string, 0, len(pkgs[m]))
		for d := range pkgs[m] {
			dirs = append(dirs, d)
		}
		sort.Strings(dirs)
		name := fmt.Sprintf("go vet (%d packages)", len(dirs))
		if m != "." {
			name = fmt.Sprintf("go vet %s (%d packages)", m, len(dirs))
		}
		// vet compiles the packages and their tests, so it is the build too.
		plan = append(plan, []fastStep{{name: name, dir: m, argv: append([]string{"go", "vet"}, dirs...), env: []string{"GOWORK=off", "CGO_ENABLED=0"}}})
	}
	if len(tsFiles) > 0 || len(appTS) > 0 {
		var ts []fastStep
		if len(tsFiles) > 0 {
			ts = append(ts, fastStep{name: "bun fmt", argv: append([]string{"bun", "fmt", "--check"}, tsFiles...)})
		}
		if len(appTS) > 0 {
			// web:typecheck generates the files tsc and Oxlint read, so Oxlint runs after it.
			ts = append(ts, fastStep{name: "tsc app/web", argv: []string{"scripts/task.sh", "web:typecheck"}})
			var lint []string
			for _, f := range appTS {
				if exists("app/" + f) {
					lint = append(lint, f)
				}
			}
			if len(lint) > 0 {
				ts = append(ts, fastStep{name: fmt.Sprintf("oxlint (%d files)", len(lint)), dir: "app", argv: append([]string{"bun", "run", "lint"}, lint...)})
			}
		}
		plan = append(plan, ts)
	}
	if md {
		plan = append(plan,
			[]fastStep{{name: "docs:lint", argv: []string{"scripts/task.sh", "docs:lint"}}},
			[]fastStep{{name: "docs:rules", argv: []string{"scripts/task.sh", "docs:rules"}}})
	}
	// A context map that changed, or a document one of them may point at, is checked as the merge queue's repo job
	// checks it; it takes about a second.
	if maps || md {
		plan = append(plan, []fastStep{{name: "agentkit:map-check", argv: []string{"scripts/task.sh", "agentkit:map-check"}}})
	}
	if len(shFiles) > 0 {
		// Only the changed scripts are inputs, so a script that sources another would report SC1091 (not followed),
		// which shell:lint, given every script, does not; the merge queue's shell:lint still checks the sourcing.
		plan = append(plan, []fastStep{{name: "shellcheck", argv: append([]string{"shellcheck", "-e", "SC1091"}, shFiles...)}})
	}
	if len(tofuFiles) > 0 {
		// As the pull request's tofu-fmt job. Formatting loads no provider, and tofu refuses to start when the
		// plugin cache directory of the environment does not exist yet.
		plan = append(plan, []fastStep{{name: "tofu fmt", argv: append([]string{"tofu", "fmt", "-check"}, tofuFiles...), env: []string{"TF_PLUGIN_CACHE_DIR="}}})
	}
	if tofuContext(root, changed) {
		// What init, validate, and the tests of tofu:check decide is the merge queue's.
		for _, task := range tofuConsistency {
			plan = append(plan, []fastStep{{name: task, argv: []string{"scripts/task.sh", task}}})
		}
	}
	picked := map[string]bool{}
	for _, s := range stale {
		for _, f := range changed {
			if strings.HasPrefix(f, s.prefix) && !picked[s.step.name] {
				picked[s.step.name] = true
				plan = append(plan, []fastStep{s.step})
			}
		}
	}
	return plan
}

// fastResult is the outcome of one step.
type fastResult struct {
	state string // ok, FAIL, skip
	took  time.Duration
	lines []string
}

func runFast(root string, s fastStep) fastResult {
	start := time.Now()
	cmd := exec.Command(s.argv[0], s.argv[1:]...)
	if strings.HasPrefix(s.argv[0], "scripts/") {
		cmd = exec.Command(filepath.Join(root, s.argv[0]), s.argv[1:]...)
	}
	cmd.Dir = filepath.Join(root, s.dir)
	cmd.Env = append(os.Environ(), s.env...)
	out, err := cmd.CombinedOutput()
	took := time.Since(start)
	var notFound *exec.Error
	if errors.As(err, &notFound) {
		return fastResult{"skip", took, []string{s.argv[0] + " is not installed; CI runs this check"}}
	}
	text := strings.TrimSpace(string(out))
	if err == nil && (!s.listing || text == "") {
		return fastResult{"ok", took, nil}
	}
	lines := Failures(text, 30)
	if len(lines) == 0 {
		for _, l := range strings.Split(text, "\n") {
			if l = plain(l); strings.TrimSpace(l) != "" && !echoed.MatchString(l) {
				lines = append(lines, l)
			}
		}
		if len(lines) > 30 {
			lines = lines[len(lines)-30:]
		}
	}
	return fastResult{"FAIL", took, lines}
}

// fastFix is the command that repairs what a failed step reports, by the step's name.
func fastFix(name string) string {
	switch {
	case name == "gofmt":
		return "gofmt -w <the files above>"
	case name == "bun fmt":
		return "scripts/task.sh web:fmt"
	case name == "tofu fmt":
		return "tofu fmt <the files above>"
	case strings.HasPrefix(name, "generated:"), name == "i18n:lint":
		return "scripts/agentkit.sh regen"
	}
	return ""
}

// CheckFast runs the quick checks of the files changed since the merge base with base — formatting, go vet of
// the changed packages, tsc and Oxlint, the catalogs, Markdown, shellcheck, the formatting and the consistency of the OpenTofu modules, and that committed generated files
// are current — and returns one line per check with the failing lines of those that failed. The tests and the
// rest of the full check are the merge queue's.
func CheckFast(r *repo.Repo, base string) (bool, []string, error) {
	changed, err := r.Changed(base)
	if err != nil {
		return false, nil, err
	}
	plan := fastPlan(r.Root, changed)
	if len(plan) == 0 {
		return true, []string{fmt.Sprintf("ok    no quick check covers the %d files changed since %s; the merge queue runs the full check", len(changed), base)}, nil
	}
	start := time.Now()
	results := make([][]fastResult, len(plan))
	var wg sync.WaitGroup
	for i, group := range plan {
		results[i] = make([]fastResult, len(group))
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j, s := range group {
				results[i][j] = runFast(r.Root, s)
			}
		}()
	}
	wg.Wait()
	var lines, failed []string
	for i, group := range plan {
		for j, s := range group {
			res := results[i][j]
			lines = append(lines, fmt.Sprintf("%-5s %s (%s)", res.state, s.name, res.took.Round(100*time.Millisecond)))
			for _, l := range res.lines {
				lines = append(lines, "      "+l)
			}
			if res.state == "FAIL" {
				failed = append(failed, s.name)
				if fix := fastFix(s.name); fix != "" {
					lines = append(lines, "      fix: "+fix)
				}
			}
		}
	}
	took := time.Since(start).Round(100 * time.Millisecond)
	if len(failed) > 0 {
		lines = append(lines, fmt.Sprintf("FAILED in %s: %s", took, strings.Join(failed, ", ")))
		return false, lines, nil
	}
	lines = append(lines, fmt.Sprintf("passed in %s (quick checks only: the merge queue runs the tests and the full check; `check --full` runs them here)", took))
	return true, lines, nil
}
