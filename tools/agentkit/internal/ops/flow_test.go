// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/repo"
	"github.com/mycroblog/my/tools/agentkit/internal/shots"
)

// git runs git in dir and returns its trimmed output.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// commit writes file with body in dir and commits it with subject.
func commit(t *testing.T, dir, file, body, subject string) string {
	t.Helper()
	write(t, filepath.Join(dir, file), body)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", subject)
	return git(t, dir, "rev-parse", "HEAD")
}

// sandbox is a bare origin, its main checkout, and a place for worktrees, with git isolated from the user's
// configuration.
func sandbox(t *testing.T) (main string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "t")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "t@example.test")
	}
	origin := filepath.Join(tmp, "origin.git")
	git(t, tmp, "init", "-q", "--bare", "-b", "main", origin)
	main = filepath.Join(tmp, "main")
	git(t, tmp, "clone", "-q", origin, main)
	git(t, main, "checkout", "-q", "-b", "main")
	// Open reads the context map for the documents of the changed paths.
	write(t, filepath.Join(main, "tools/agentkit/context-map.yaml"), "entries: []\n")
	commit(t, main, "README.md", "# r\n", "chore: start")
	git(t, main, "push", "-q", "-u", "origin", "main")
	return main
}

// addWorktree makes a worktree of main on a new branch from origin/main.
func addWorktree(t *testing.T, main, branch string) *repo.Repo {
	t.Helper()
	dir := filepath.Join(filepath.Dir(main), "wt-"+strings.ReplaceAll(branch, "/", "-"))
	git(t, main, "worktree", "add", "-q", "-b", branch, dir, "origin/main")
	r, err := repo.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// advanceMain commits file to main and pushes it, leaving the worktrees' origin/main up to date.
func advanceMain(t *testing.T, main, file, body string) {
	t.Helper()
	commit(t, main, file, body, "chore: move main")
	git(t, main, "push", "-q", "origin", "main")
}

// mergeToMain merges branch into main with a merge commit, as the merge queue does, and pushes.
func mergeToMain(t *testing.T, main, branch string) {
	t.Helper()
	git(t, main, "merge", "-q", "--no-ff", "-m", "Merge pull request from "+branch, branch)
	git(t, main, "push", "-q", "origin", "main")
}

// fakeGH puts a gh on PATH whose body is the shell script; it appends its arguments to the returned log.
func fakeGH(t *testing.T, script string) (log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "gh.log")
	body := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + log + "'\nSTATE='" + dir + "'\n" + script + "\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func readLog(t *testing.T, log string) string {
	t.Helper()
	b, _ := os.ReadFile(log)
	return string(b)
}

func wantRefusal(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no refusal, want one with %q", parts)
	}
	for _, p := range parts {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("refusal %q lacks %q", err.Error(), p)
		}
	}
}

func TestGateRefusesMainCheckout(t *testing.T) {
	main := sandbox(t)
	git(t, main, "checkout", "-q", "-b", "feat/1-x")
	commit(t, main, "a.txt", "a\n", "feat: a")
	r, err := repo.Open(main)
	if err != nil {
		t.Fatal(err)
	}
	_, err = gate(r, "feat/1-x", "origin/main")
	wantRefusal(t, err, "pr open refused: this is the main checkout", "fix: git switch main && git worktree add .claude/worktrees/feat-1-x feat/1-x")
}

func TestGateHistory(t *testing.T) {
	main := sandbox(t)

	merged := addWorktree(t, main, "feat/1-merge")
	commit(t, merged.Root, "a.txt", "a\n", "feat: a")
	advanceMain(t, main, "m.txt", "m\n")
	git(t, merged.Root, "merge", "-q", "-m", "Merge origin/main", "origin/main")
	_, err := gate(merged, "feat/1-merge", "origin/main")
	wantRefusal(t, err, "origin/main..HEAD has 1 merge commit(s)", "Merge origin/main", "fix: git rebase origin/main")

	fix := addWorktree(t, main, "feat/2-fixup")
	commit(t, fix.Root, "b.txt", "b\n", "feat: b")
	commit(t, fix.Root, "b.txt", "bb\n", "fixup! feat: b")
	_, err = gate(fix, "feat/2-fixup", "origin/main")
	wantRefusal(t, err, `1 commit(s) to fold`, `"fixup! feat: b"`, "fix: GIT_SEQUENCE_EDITOR=true git rebase -i --autosquash origin/main")

	conflict := addWorktree(t, main, "feat/3-conflict")
	commit(t, conflict.Root, "c.txt", "mine\n", "feat: c")
	advanceMain(t, main, "c.txt", "theirs\n")
	_, err = gate(conflict, "feat/3-conflict", "origin/main")
	wantRefusal(t, err, "feat/3-conflict conflicts with origin/main in c.txt", "fix: git rebase origin/main")

	// Behind main without a conflict, and nothing a quick check covers: the gate passes.
	clean := addWorktree(t, main, "feat/4-clean")
	commit(t, clean.Root, "d.txt", "d\n", "feat: d")
	advanceMain(t, main, "e.txt", "e\n")
	if looked, err := gate(clean, "feat/4-clean", "origin/main"); err != nil || looked != nil {
		t.Errorf("a clean branch must pass, got %v, %v", looked, err)
	}
}

func TestGateRunsFastCheck(t *testing.T) {
	main := sandbox(t)
	r := addWorktree(t, main, "feat/5-fmt")
	write(t, filepath.Join(r.Root, "go.mod"), "module example.test/m\n\ngo 1.22\n")
	commit(t, r.Root, "x/x.go", "package x\n\nfunc  F() {}\n", "feat: x")
	_, err := gate(r, "feat/5-fmt", "origin/main")
	wantRefusal(t, err, "the quick checks fail", "FAIL  gofmt", "x/x.go", "fix: gofmt -w", "ok    go vet (1 packages)")

	commit(t, r.Root, "x/x.go", "package x\n\nfunc F() { undefined() }\n", "feat: x2")
	ok, lines, err := CheckFast(r, "origin/main")
	if err != nil || ok {
		t.Fatalf("a package that does not compile must fail, got %v %q %v", ok, lines, err)
	}
	if out := strings.Join(lines, "\n"); !strings.Contains(out, "FAIL  go vet") || !strings.Contains(out, "undefined") {
		t.Errorf("got %q", out)
	}

	commit(t, r.Root, "x/x.go", "package x\n\nfunc F() {}\n", "feat: x3")
	if ok, lines, err := CheckFast(r, "origin/main"); err != nil || !ok || len(lines) != 3 {
		t.Errorf("want gofmt, go vet, and the summary to pass, got %v %q %v", ok, lines, err)
	}
}

func names(plan [][]fastStep) []string {
	var out []string
	for _, g := range plan {
		for _, s := range g {
			out = append(out, s.name)
		}
	}
	return out
}

func TestFastPlan(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"go.mod", "internal/a/a.go", "internal/a/testdata/t.go", "tools/agentkit/go.mod", "tools/agentkit/x/x.go",
		"app/web/src/a.tsx", "pkg/mykit/ts/src/b.ts", "docs/a.md", "scripts/a.sh", "locales/en/x.json"} {
		write(t, filepath.Join(root, f), "x")
	}
	cases := []struct {
		changed []string
		want    []string
	}{
		{[]string{"internal/a/a.go", "internal/a/testdata/t.go", "internal/gone/g.go"}, []string{"gofmt", "go vet (1 packages)"}},
		{[]string{"tools/agentkit/x/x.go"}, []string{"gofmt", "go vet tools/agentkit (1 packages)"}},
		{[]string{"app/web/src/a.tsx", "app/web/src/gone.tsx"}, []string{"bun fmt", "tsc app/web", "oxlint (1 files)"}},
		{[]string{"pkg/mykit/ts/src/b.ts"}, []string{"bun fmt"}},
		{[]string{"docs/a.md"}, []string{"docs:lint", "docs:rules"}},
		{[]string{"scripts/a.sh"}, []string{"shellcheck"}},
		{[]string{"locales/en/x.json"}, []string{"i18n:lint"}},
		{[]string{"schema/sqlite/01-a.sql", "queries/a.sql", "api/spec/a.yaml", "internal/platform/job_types.go"},
			[]string{"gofmt", "generated: api:generate-check", "generated: go:sqlc-check", "generated: " + workersScript}},
		{[]string{"deploy/x.yaml"}, nil},
	}
	for _, c := range cases {
		if c.changed[len(c.changed)-1] == "internal/platform/job_types.go" {
			write(t, filepath.Join(root, "internal/platform/job_types.go"), "x")
		}
		got := names(fastPlan(root, c.changed))
		if c.changed[0] == "schema/sqlite/01-a.sql" {
			// job_types.go is a Go file of a package too.
			got = slicesWithout(got, "go vet (1 packages)")
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: got %q, want %q", c.changed, got, c.want)
		}
	}
}

func slicesWithout(in []string, drop string) []string {
	var out []string
	for _, s := range in {
		if s != drop {
			out = append(out, s)
		}
	}
	return out
}

func TestSubjectIssues(t *testing.T) {
	got := subjectIssues([]string{"feat(tls): renew (#2828, #2829)", "fix: x (#12)", "docs: see (#3) above", "feat: again (#12)"})
	if want := []int{2828, 2829, 12}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// shot writes a screenshot and the manifest of root, taken at the given time.
func shot(t *testing.T, root, name string, taken time.Time) string {
	t.Helper()
	rel := shots.Dir + "/" + name
	write(t, filepath.Join(root, rel), "png")
	write(t, filepath.Join(root, shots.Dir, "manifest.json"),
		`{"shots":[{"file":"`+rel+`","story":"screens-a--normal","width":390,"takenAt":"`+taken.Format(time.RFC3339Nano)+`"}]}`)
	return rel
}

func TestScreens(t *testing.T) {
	main := sandbox(t)
	r := addWorktree(t, main, "feat/6-ui")
	commit(t, r.Root, "app/web/src/a.stories.tsx", "story\n", "test: story")
	if looked, err := screens(r, "origin/main"); err != nil || looked != nil {
		t.Fatalf("a change of stories alone needs no screenshot, got %v, %v", looked, err)
	}
	commit(t, r.Root, "app/web/src/a.tsx", "one\n", "feat: a")
	edited := time.Now()
	if err := os.Chtimes(filepath.Join(r.Root, "app/web/src/a.tsx"), edited, edited); err != nil {
		t.Fatal(err)
	}
	_, err := screens(r, "origin/main")
	wantRefusal(t, err, "the change touches the web UI (app/web/src/a.tsx) and no screenshot is newer than app/web/src/a.tsx",
		"fix: run `scripts/task.sh web:shot -- --changed`", "Read tool")

	rel := shot(t, r.Root, "a-390.png", edited.Add(-time.Minute))
	_, err = screens(r, "origin/main")
	wantRefusal(t, err, "no screenshot is newer")

	shot(t, r.Root, "a-390.png", edited.Add(time.Minute))
	_, err = screens(r, "origin/main")
	wantRefusal(t, err, "were not looked at since they were taken ("+rel+")", "fix: open each with the Read tool")

	// Read before it was taken again does not count.
	if err := shots.Record(r.Root, rel, edited, ""); err != nil {
		t.Fatal(err)
	}
	_, err = screens(r, "origin/main")
	wantRefusal(t, err, "were not looked at")

	src := []string{"app/web/src/a.tsx"}
	if err := shots.Record(r.Root, rel, edited.Add(2*time.Minute), shots.Fingerprint(r.Root, src)); err != nil {
		t.Fatal(err)
	}
	if looked, err := screens(r, "origin/main"); err != nil || !reflect.DeepEqual(looked, []string{rel}) {
		t.Fatalf("a screenshot read after it was taken must pass, got %v, %v", looked, err)
	}

	// A rebase rewrites the file without changing it: the screenshot still shows it.
	later := edited.Add(time.Hour)
	if err := os.Chtimes(filepath.Join(r.Root, "app/web/src/a.tsx"), later, later); err != nil {
		t.Fatal(err)
	}
	if looked, err := screens(r, "origin/main"); err != nil || len(looked) != 1 {
		t.Errorf("unchanged sources keep their screenshots, got %v, %v", looked, err)
	}
	// An edit does not.
	write(t, filepath.Join(r.Root, "app/web/src/a.tsx"), "two\n")
	if err := os.Chtimes(filepath.Join(r.Root, "app/web/src/a.tsx"), later, later); err != nil {
		t.Fatal(err)
	}
	_, err = screens(r, "origin/main")
	wantRefusal(t, err, "no screenshot is newer")
}

const ghIssues = `
case "$1 $2" in
  "issue view")
    case "$3" in
      12) echo '{"number":12,"title":"First","body":"## Acceptance criteria\\n- [x] one","state":"OPEN","url":"https://github.com/o/r/issues/12"}' ;;
      15) echo '{"number":15,"title":"Second","body":"","state":"OPEN","url":"https://github.com/o/r/issues/15"}' ;;
      99) echo '{"number":99,"title":"A PR","body":"","state":"MERGED","url":"https://github.com/o/r/pull/99"}' ;;
      *) echo "not found" >&2; exit 1 ;;
    esac ;;
  "api graphql") echo '{"data":{"repository":{"issueOrPullRequest":{}}}}' ;;
  "pr create") cat > "$STATE/body"; echo "https://github.com/o/r/pull/20" ;;
  "issue comment") ;;
  *) echo "unexpected: $*" >&2; exit 1 ;;
esac`

func TestOpenClosesEveryIssue(t *testing.T) {
	main := sandbox(t)
	log := fakeGH(t, ghIssues)
	r := addWorktree(t, main, "feat/12-phase")
	commit(t, r.Root, "a.txt", "a\n", "feat: first (#12)")
	commit(t, r.Root, "b.txt", "b\n", "feat: second (#15, #99)")
	commit(t, r.Root, "c.txt", "c\n", "fix: third (#404)")
	url, err := Open(r, OpenOptions{Summary: "S"})
	if err != nil || url != "https://github.com/o/r/pull/20" {
		t.Fatalf("got %q, %v", url, err)
	}
	body, _ := os.ReadFile(filepath.Join(filepath.Dir(log), "body"))
	for _, want := range []string{"Closes #12\nCloses #15\n\n## Changes", "#12 First\n\n- [x] one\n\n#15 Second\n\n- [ ] (the issue has no checklist)"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the description lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(string(body), "#99") && strings.Contains(string(body), "Closes #99") {
		t.Errorf("a pull request number must not be closed:\n%s", body)
	}
	if strings.Contains(string(body), "## Screens") {
		t.Errorf("no UI change, no Screens section:\n%s", body)
	}
	calls := readLog(t, log)
	for _, want := range []string{"issue comment 12", "issue comment 15", "--head feat/12-phase"} {
		if !strings.Contains(calls, want) {
			t.Errorf("gh was not called with %q:\n%s", want, calls)
		}
	}
	if got := git(t, main, "ls-remote", "--heads", "origin", "feat/12-phase"); got == "" {
		t.Error("the branch was not pushed")
	}

	// Given issues replace the ones read from the branch; one that cannot be read stops before the push.
	other := addWorktree(t, main, "feat/7-other")
	commit(t, other.Root, "d.txt", "d\n", "feat: d")
	if _, err := Open(other, OpenOptions{Issues: []int{404}}); err == nil {
		t.Error("an unreadable --issue must fail")
	}
	if got := git(t, main, "ls-remote", "--heads", "origin", "feat/7-other"); got != "" {
		t.Error("a refused pull request must not push")
	}
}

func TestOpenListsScreens(t *testing.T) {
	main := sandbox(t)
	log := fakeGH(t, ghIssues)
	r := addWorktree(t, main, "feat/12-ui")
	commit(t, r.Root, "app/web/src/a.css", "a{}\n", "feat: look (#12)")
	rel := shot(t, r.Root, "a-390.png", time.Now().Add(time.Minute))
	if err := shots.Record(r.Root, rel, time.Now().Add(2*time.Minute), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(r, OpenOptions{}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(filepath.Dir(log), "body"))
	if want := "## Screens\n\n- `" + rel + "`\n"; !strings.Contains(string(body), want) {
		t.Errorf("the description lacks %q:\n%s", want, body)
	}
}

func TestTidy(t *testing.T) {
	main := sandbox(t)
	fakeGH(t, `
case "$*" in
  *"--head feat/3-closed "*) echo '[{"number":30,"state":"CLOSED","headRefOid":"'"$(cat "$STATE/closed")"'"}]' ;;
  *"--head feat/4-ahead "*) echo '[{"number":40,"state":"CLOSED","headRefOid":"0000000000000000000000000000000000000000"}]' ;;
  *"--head feat/5-open "*) echo '[{"number":50,"state":"OPEN","headRefOid":"x"}]' ;;
  *) echo '[]' ;;
esac`)
	state := filepath.Dir(readPath(t))

	done := addWorktree(t, main, "feat/1-merged")
	commit(t, done.Root, "a.txt", "a\n", "feat: a")
	mergeToMain(t, main, "feat/1-merged")

	dirty := addWorktree(t, main, "feat/2-dirty")
	commit(t, dirty.Root, "b.txt", "b\n", "feat: b")
	mergeToMain(t, main, "feat/2-dirty")
	write(t, filepath.Join(dirty.Root, "wip.txt"), "wip\n")

	closed := addWorktree(t, main, "feat/3-closed")
	write(t, filepath.Join(state, "closed"), commit(t, closed.Root, "c.txt", "c\n", "feat: c"))

	ahead := addWorktree(t, main, "feat/4-ahead")
	commit(t, ahead.Root, "d.txt", "d\n", "feat: d")

	open := addWorktree(t, main, "feat/5-open")
	commit(t, open.Root, "e.txt", "e\n", "feat: e")

	fresh := addWorktree(t, main, "feat/6-fresh") // no commit of its own, no pull request
	mine := addWorktree(t, main, "feat/7-mine")
	commit(t, mine.Root, "f.txt", "f\n", "feat: f")
	mergeToMain(t, main, "feat/7-mine")

	r, err := repo.Open(mine.Root)
	if err != nil {
		t.Fatal(err)
	}
	if n := Leftovers(r); n != 2 {
		t.Errorf("Leftovers = %d, want the 2 merged branches that are not the caller's", n)
	}
	dry, err := Tidy(r, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(dry, "\n"), "would remove worktree "+done.Root+" and branch feat/1-merged (merged into origin/main)") {
		t.Errorf("dry run: %q", dry)
	}
	if _, err := os.Stat(done.Root); err != nil {
		t.Fatal("a dry run removed a worktree")
	}
	lines, err := Tidy(r, false)
	if err != nil {
		t.Fatal(err)
	}
	out := strings.Join(lines, "\n")
	for _, want := range []string{
		"removed worktree " + done.Root + " and branch feat/1-merged (merged into origin/main)",
		"kept feat/2-dirty: 1 uncommitted files in " + dirty.Root,
		"removed worktree " + closed.Root + " and branch feat/3-closed (PR #30 closed)",
		"kept feat/4-ahead: it has commits that PR #40 closed does not hold",
		"kept feat/7-mine: you are in its worktree (cd " + main + " && scripts/agentkit.sh tidy)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("tidy lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "feat/5-open") || strings.Contains(out, "feat/6-fresh") {
		t.Errorf("a branch with an open pull request or none is not tidied:\n%s", out)
	}
	branches := git(t, main, "branch", "--format=%(refname:short)")
	if strings.Contains(branches, "feat/1-merged") || strings.Contains(branches, "feat/3-closed") {
		t.Errorf("branches left: %s", branches)
	}
	for _, keep := range []string{"feat/2-dirty", "feat/4-ahead", "feat/5-open", "feat/6-fresh", "feat/7-mine"} {
		if !strings.Contains(branches, keep) {
			t.Errorf("%s was removed", keep)
		}
	}
	for _, dir := range []string{dirty.Root, ahead.Root, open.Root, fresh.Root} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("%s was removed", dir)
		}
	}
}

// readPath returns the fake gh's path.
func readPath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("gh")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// ghLand answers the calls of Land for pull request 20 on feat/12-x, which closes #12, a sub-issue of #30 in a
// milestone. $STATE/pr holds the pull request's JSON.
const ghLand = `
case "$1 $2" in
  "pr view")
    case "$*" in
      *closingIssuesReferences*) echo '{"body":"Closes #12, fixes #13","closingIssuesReferences":[{"number":12}]}' ;;
      *) n=$(cat "$STATE/views" 2>/dev/null || echo 0); n=$((n+1)); echo "$n" > "$STATE/views"
         if [ -f "$STATE/pr$n" ]; then cat "$STATE/pr$n"; else cat "$STATE/pr"; fi ;;
    esac ;;
  "pr merge") ;;
  "run list") echo '[{"databaseId":7,"headBranch":"gh-readonly-queue/main/pr-21-abc","conclusion":"failure"},{"databaseId":8,"headBranch":"gh-readonly-queue/main/pr-20-abc","conclusion":"failure"}]' ;;
  "run view") echo '{"jobs":[{"databaseId":81,"name":"server-test","conclusion":"failure"},{"databaseId":82,"name":"check","conclusion":"failure"}]}' ;;
  "api graphql")
    case "$*" in
      *mergeQueueEntry*)
        if [ -f "$STATE/queued" ]; then q='{"state":"QUEUED"}'; else q=null; fi
        echo '{"data":{"repository":{"pullRequests":{"pageInfo":{"hasNextPage":false},"nodes":[{"number":20,"mergeQueueEntry":'"$q"'}]}}}}' ;;
      *number=12) echo '{"data":{"repository":{"issue":{"number":12,"title":"First","state":"CLOSED","milestone":{"number":3,"title":"M5"},"parent":{"number":30,"title":"Epic","state":"OPEN","subIssuesSummary":{"total":2,"completed":'"$(cat "$STATE/completed")"'},"subIssues":{"nodes":[{"number":12,"state":"CLOSED"},{"number":14,"state":"'"$(cat "$STATE/sibling")"'"}]}}}}}}' ;;
      *number=13) echo '{"data":{"repository":{"issue":{"number":13,"title":"Other","state":"'"$(cat "$STATE/other" 2>/dev/null || echo OPEN)"'","milestone":null,"parent":null}}}}' ;;
      *number=30) echo '{"data":{"repository":{"issue":{"number":30,"title":"Epic","state":"CLOSED","milestone":{"number":3,"title":"M5"},"parent":null}}}}' ;;
    esac ;;
  "issue close") ;;
  "api repos/mycroblog/my/milestones/3") cat "$STATE/milestone" 2>/dev/null || echo '{"open_issues":4,"closed_issues":9,"state":"open"}' ;;
  "api -X") [ "$*" = "api -X PATCH repos/mycroblog/my/milestones/3 -f state=closed" ] || { echo "unexpected: $*" >&2; exit 1; } ;;
  "api repos/mycroblog/my/actions/jobs/"*)
    id=${2#repos/mycroblog/my/actions/jobs/}; id=${id%/logs}
    if [ -f "$STATE/job$id" ]; then cat "$STATE/job$id"; else echo "--- FAIL: TestThing (0.00s)"; fi ;;
  *) echo "unexpected: $*" >&2; exit 1 ;;
esac`

func prJSON(state, mergeable, oid, rollup string, auto bool) string {
	a := "null"
	if auto {
		a = `{"enabledAt":"x"}`
	}
	return `{"number":20,"title":"feat: x","state":"` + state + `","mergeable":"` + mergeable + `","url":"u","headRefOid":"` + oid +
		`","headRefName":"feat/12-x","statusCheckRollup":[` + rollup + `],"autoMergeRequest":` + a + `}`
}

func landSetup(t *testing.T) (main string, wt *repo.Repo, state, log, head string) {
	t.Helper()
	main = sandbox(t)
	log = fakeGH(t, ghLand)
	state = filepath.Dir(log)
	wt = addWorktree(t, main, "feat/12-x")
	head = commit(t, wt.Root, "a.txt", "a\n", "feat: x (#12)")
	git(t, wt.Root, "push", "-q", "-u", "origin", "HEAD")
	write(t, filepath.Join(state, "completed"), "1")
	write(t, filepath.Join(state, "sibling"), "OPEN")
	orig := settle
	settle = time.Millisecond
	t.Cleanup(func() { settle = orig })
	return main, wt, state, log, head
}

func land(t *testing.T, dir string) (int, string) {
	t.Helper()
	r, err := repo.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := Land(&out, r, 20, LandOptions{Timeout: 5 * time.Second, Interval: time.Millisecond, LogDir: t.TempDir()})
	return code, out.String()
}

func TestLandMergesCleansUpAndReports(t *testing.T) {
	main, wt, state, log, head := landSetup(t)
	// Open and pending, then in the queue, then merged.
	write(t, filepath.Join(state, "pr"), prJSON("OPEN", "MERGEABLE", head, "", true))
	write(t, filepath.Join(state, "pr4"), prJSON("MERGED", "UNKNOWN", head, "", false))
	mergeToMain(t, main, "feat/12-x")
	code, out := land(t, main)
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	last := lines[len(lines)-3:]
	want := []string{
		"merged: PR #20 feat: x (closes #12, #13)",
		"cleaned up: removed worktree " + wt.Root + " and branch feat/12-x",
		"remains: #30 Epic: 1 of 2 sub-issues open (#14); #13 is still open (gh issue close 13 -R mycroblog/my); milestone M5: 4 open, 9 closed",
	}
	if !reflect.DeepEqual(last, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(last, "\n"), strings.Join(want, "\n"))
	}
	calls := readLog(t, log)
	if !strings.Contains(calls, "pr merge 20 -R mycroblog/my --auto --merge") {
		t.Errorf("auto-merge was not turned on:\n%s", calls)
	}
	if strings.Contains(calls, "issue close") {
		t.Errorf("a parent with an open sub-issue must stay open:\n%s", calls)
	}
	if _, err := os.Stat(wt.Root); err == nil {
		t.Error("the worktree is still there")
	}
	if b := git(t, main, "branch", "--list", "feat/12-x"); b != "" {
		t.Error("the branch is still there")
	}
}

func TestLandClosesFinishedParent(t *testing.T) {
	main, wt, state, log, head := landSetup(t)
	write(t, filepath.Join(state, "pr"), prJSON("MERGED", "UNKNOWN", head, "", false))
	write(t, filepath.Join(state, "completed"), "2")
	write(t, filepath.Join(state, "sibling"), "CLOSED")
	mergeToMain(t, main, "feat/12-x")
	// From inside the branch's worktree: nothing is removed, and the line says what to run.
	code, out := land(t, wt.Root)
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for _, want := range []string{
		"cleaned up: kept feat/12-x: you are in its worktree (cd " + main + " && scripts/agentkit.sh tidy)",
		"closed #30 Epic (every sub-issue is closed)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	calls := readLog(t, log)
	if !strings.Contains(calls, "issue close 30 -R mycroblog/my --comment Every sub-issue is closed; #20 merged the last one.") {
		t.Errorf("the parent was not closed:\n%s", calls)
	}
	if strings.Contains(calls, "pr merge") {
		t.Errorf("a merged pull request needs no auto-merge:\n%s", calls)
	}
}

func TestLandClosesFinishedMilestone(t *testing.T) {
	main, _, state, log, head := landSetup(t)
	write(t, filepath.Join(state, "pr"), prJSON("MERGED", "UNKNOWN", head, "", false))
	write(t, filepath.Join(state, "completed"), "2")
	write(t, filepath.Join(state, "sibling"), "CLOSED")
	write(t, filepath.Join(state, "other"), "CLOSED")
	write(t, filepath.Join(state, "milestone"), `{"open_issues":0,"closed_issues":13,"state":"open"}`)
	mergeToMain(t, main, "feat/12-x")
	code, out := land(t, main)
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	want := "remains: closed #30 Epic (every sub-issue is closed); #13: no parent issue and no milestone, so nothing tracks what the work was part of; " +
		"closed milestone M5 (each of its 13 issues is closed)\n"
	if !strings.HasSuffix(out, want) {
		t.Errorf("got\n%s\nwant the last line\n%s", out, want)
	}
	if calls := readLog(t, log); !strings.Contains(calls, "api -X PATCH repos/mycroblog/my/milestones/3 -f state=closed") {
		t.Errorf("the milestone was not closed:\n%s", calls)
	}
}

func TestLandFailures(t *testing.T) {
	failed := `{"__typename":"CheckRun","name":"preflight","workflowName":"CI","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://github.com/o/r/actions/runs/1/job/81"}`
	cases := []struct {
		name  string
		setup func(state, head string)
		code  int
		want  []string
	}{
		{"conflict", func(state, head string) {
			write(t, filepath.Join(state, "pr"), prJSON("OPEN", "CONFLICTING", head, "", false))
		}, LandConflict, []string{"PR #20 conflicts with its base", "fix: in the branch's worktree: git fetch origin && git rebase origin/main"}},
		{"ci failed", func(state, head string) {
			write(t, filepath.Join(state, "pr"), prJSON("OPEN", "MERGEABLE", head, failed, true))
		}, 1, []string{"PR #20: CI failed: preflight\n", "--- FAIL: TestThing", "fix: repair it in the branch's worktree"}},
		{"ci failed before any test ran", func(state, head string) {
			write(t, filepath.Join(state, "pr"), prJSON("OPEN", "MERGEABLE", head, failed, true))
			write(t, filepath.Join(state, "job81"), fixture(t, "job-apt-unresolved-1.txt"))
		}, 1, []string{
			"PR #20: CI failed: preflight; failed before any test ran: the runner could not reach azure.archive.ubuntu.com (rerun: gh run rerun 1 --failed)\n",
			"E: Unable to locate package unzip", "fix: gh run rerun 1 --failed; then run pr land again",
		}},
		{"dropped by the queue", func(state, head string) {
			write(t, filepath.Join(state, "pr"), prJSON("OPEN", "MERGEABLE", head, "", false))
			write(t, filepath.Join(state, "pr2"), prJSON("OPEN", "MERGEABLE", head, "", true))
		}, LandDropped, []string{"PR #20 left the merge queue without merging", "== server-test (merge group", "--- FAIL: TestThing", "fix: in the branch's worktree: git fetch origin && git rebase origin/main"}},
		{"closed", func(state, head string) {
			write(t, filepath.Join(state, "pr"), prJSON("CLOSED", "UNKNOWN", head, "", false))
		}, 1, []string{"PR #20 is closed without merging"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			main, wt, state, _, head := landSetup(t)
			c.setup(state, head)
			code, out := land(t, main)
			if code != c.code {
				t.Errorf("exit %d, want %d:\n%s", code, c.code, out)
			}
			for _, want := range c.want {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
			if _, err := os.Stat(wt.Root); err != nil {
				t.Error("a pull request that did not merge keeps its worktree")
			}
		})
	}
}
