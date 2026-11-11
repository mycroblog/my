// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func run(t *testing.T, event string, in map[string]any) map[string]any {
	t.Helper()
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run(event, bytes.NewReader(b), &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		return nil
	}
	var v map[string]any
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("hook printed invalid JSON %q: %v", out.String(), err)
	}
	return v["hookSpecificOutput"].(map[string]any)
}

// fixture makes a working tree of the project (it has scripts/agentkit.sh) with sources, documents, catalogs,
// scratch space git ignores, and a directory outside it.
func fixture(t *testing.T) (root, outside string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, outside = filepath.Join(base, "my"), filepath.Join(base, "elsewhere")
	big := "# Big\n\n" + strings.Repeat("本文。\n", 20000)
	files := map[string]string{
		"my/scripts/agentkit.sh":                     "#!/bin/sh\n",
		"my/.gitignore":                              ".tmp/\nnode_modules/\n*.log\n",
		"my/internal/app/content/service.go":         "package content\n\nfunc Create() {}\n",
		"my/internal/app/content/service_test.go":    "package content\n",
		"my/app/web/src/routes/login.tsx":            "export function Login() {}\n",
		"my/app/web/package.json":                    "{}\n",
		"my/docs/adr/0069-quote-posts.md":            "# ADR\n",
		"my/docs/architecture/federation.md":         big,
		"my/locales/en/client.json":                  "{}\n",
		"my/locales/ja/client.json":                  "{}\n",
		"my/api/openapi.yaml":                        "openapi: 3.1.0\n",
		"my/tools/agentkit/go.mod":                   "module x\n",
		"my/.tmp/agentkit/ctx/916.md":                big,
		"my/.tmp/check.log":                          "ok\n",
		"my/.tmp/edit.py":                            "print(1)\n",
		"my/app/node_modules/react/index.js":         "module.exports = {}\n",
		"my/check.log":                               "ok\n",
		"elsewhere/big.md":                           big,
		"elsewhere/pkg/mod/example.com/x@v1/x.go":    "package x\n",
		"elsewhere/tool-results/b1.txt":              big,
		"elsewhere/scratchpad/prompt.txt":            "x\n",
		"elsewhere/scratchpad/docs/adr/0001-copy.md": "# copy\n",
	}
	for p, body := range files {
		full := filepath.Join(base, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	return root, outside
}

func TestSplitCommand(t *testing.T) {
	cases := []struct {
		cmd  string
		want []string
	}{
		{"cd x && a | b; c", []string{"cd x", "a>", ">b", "c"}},
		{`grep -n "a|b; c" f || echo 'x && y'`, []string{`grep -n "a|b; c" f`, `echo 'x && y'`}},
		{"echo $(git ls-files | head -1) | wc -l", []string{"echo $(git ls-files | head -1)>", ">wc -l"}},
		{"cat > f <<'EOF'\nnever `gh pr checks --watch`; grep -rn x .\nEOF\nls 2>&1 | tail -1", []string{"cat > f <<'EOF'", "ls 2>&1>", ">tail -1"}},
		{"sleep 5 & wait", []string{"sleep 5", "wait"}},
		{"a \\\n  --flag | b", []string{"a   --flag>", ">b"}},
		{"python3 - <<EOF | tee x\nprint(1)\nEOF", []string{"python3 - <<EOF>", ">tee x"}},
	}
	for _, c := range cases {
		var got []string
		for _, s := range splitCommand(c.cmd) {
			text := s.text
			if s.pipedIn {
				text = ">" + text
			}
			if s.pipedOut {
				text += ">"
			}
			got = append(got, text)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q:\n got %q\nwant %q", c.cmd, got, c.want)
		}
	}
	segs := splitCommand("python3 - <<'EOF'\nopen('a','w')\nEOF\necho done")
	if len(segs) != 2 || segs[0].heredoc != "open('a','w')\n" || segs[1].text != "echo done" {
		t.Errorf("here-document body: %+v", segs)
	}
}

func TestParseCommand(t *testing.T) {
	vars := map[string]string{}
	parseCommand("f=docs/a.md", vars)
	c := parseCommand(`GOWORK=off timeout 30 sed -i 's/a b/c/' "$f" $other 2>/dev/null >out.txt`, vars)
	if c.tool != "sed" || len(c.args) != 4 || c.args[1].text != "s/a b/c/" || c.args[2].text != "docs/a.md" || c.args[2].dyn || !c.args[3].dyn {
		t.Errorf("sed: %+v", c)
	}
	if len(c.writes) != 2 || c.writes[1].text != "out.txt" || !c.has("timeout") {
		t.Errorf("redirections and wrappers: %+v", c)
	}
	c = parseCommand(`until ! /usr/bin/pgrep -f "x y" >/dev/null`, nil)
	if c.tool != "pgrep" || !c.has("until") {
		t.Errorf("until: %+v", c)
	}
	c = parseCommand(`xargs -I{} grep -l 'a>b' src/*.go`, nil)
	if c.tool != "grep" || !c.has("xargs") || c.args[1].text != "a>b" || !c.args[2].glob {
		t.Errorf("xargs: %+v", c)
	}
}

// The commands are those of past sessions, with their paths set in the fixture.
func TestPreBash(t *testing.T) {
	root, outside := fixture(t)
	cases := []struct {
		name, cmd string
		rule      string   // "" when the command is left alone
		say       []string // what the reason names
	}{
		// Searching the repository.
		{"grep -rn", `grep -rn "voice.room.details\|opened_at" --include=*.tsx . | grep -v "\.gen\." | head`, "search",
			[]string{`scripts/agentkit.sh grep --kind ts 'voice.room.details\|opened_at'`}},
		{"grep one file", `grep -n "web:test\|web:install" docs/adr/0069-quote-posts.md | head`, "search",
			[]string{`scripts/agentkit.sh grep 'web:test\|web:install' docs/adr/0069-quote-posts.md`}},
		{"grep after cd", `cd app/web && grep -rn "ComposeEntry" src --include=*.tsx | grep -v test | head`, "search",
			[]string{`grep --kind ts ComposeEntry app/web/src`}},
		{"grep -i -l glob", `ls docs; grep -il 'quote' docs/adr/*.md`, "search", []string{"grep -i -l quote 'docs/adr/*.md'"}},
		{"grep declaration", `grep -n "^func Create" -A45 internal/app/content/service.go`, "search",
			[]string{"scripts/agentkit.sh show Create", "-C 45"}},
		{"grep export function", `grep -n "export function Login" -B5 -A20 app/web/src/routes/login.tsx | head -40`, "search",
			[]string{"scripts/agentkit.sh show Login"}},
		{"rg", `rg -n "Announce" internal -g '*.go'`, "search", []string{"grep --kind go Announce internal"}},
		{"rg no path", `rg Foo | head -20`, "search", []string{"scripts/agentkit.sh grep Foo`"}},
		{"git grep", `git grep -n -i lambda -- docs scripts | cut -c1-180`, "search", []string{"grep -i lambda docs scripts"}},
		{"grep absolute", `grep -n "    StatusPage:" -A12 ` + root + `/api/openapi.yaml`, "search", []string{"-C 12", "api/openapi.yaml"}},
		{"grep in sh -c", `timeout 60 bash -c 'grep -rn Foo internal'`, "search", nil},
		{"grep variable set here", `f=internal/app/content/service.go; grep -n 'Create' "$f"`, "search", nil},
		{"cat pipe grep", `cat internal/app/content/service.go | grep -n Create`, "search", []string{"grep Create"}},
		{"grep as a filter", `go test ./internal/... 2>&1 | grep -E "^(---|FAIL|ok)" | grep -v "^ok" | head -20`, "", nil},
		{"grep a diff", `git diff HEAD -- api/openapi.yaml | grep "^[-+]" | head -20`, "", nil},
		{"grep a log under .tmp", `grep -n -- '--- FAIL\|^FAIL' .tmp/check.log | head`, "", nil},
		{"grep an ignored log", `grep -c "✓" check.log`, "", nil},
		{"grep scratch outside", `grep -nE "[0-9]+ failed" ` + outside + `/scratchpad/prompt.txt | head`, "", nil},
		{"grep the module cache", `grep -rn "func filterAlgorithm" -A12 ` + outside + `/pkg/mod/example.com/x@v1/`, "", nil},
		{"grep node_modules", `cd app && grep -rn "stream" node_modules/react/index.js 2>/dev/null | head`, "", nil},
		{"grep -q", `grep -q Create internal/app/content/service.go && echo yes`, "", nil},
		{"grep -c one file", `grep -c Create internal/app/content/service.go`, "", nil},
		{"grep unknown variable", `for f in $FILES; do grep -n "FR-727" $f; done`, "", nil},
		{"xargs grep", `git diff --name-only | xargs grep -ln "about"`, "", nil},
		{"git grep a revision", `git grep -n "metrics.NewService" origin/main -- internal/ | head`, "", nil},
		{"grep stdin", "grep -c x <<'EOF'\nx\nEOF", "", nil},
		{"grep in a message", "git commit -q -F - <<'EOF'\nfix: stop\ngrep -rn foo .\nEOF", "", nil},
		{"agentkit grep", `scripts/agentkit.sh grep -i Create internal`, "", nil},

		// Reading part of a file.
		{"sed -n", `sed -n 41,75p docs/adr/0069-quote-posts.md`, "read-range",
			[]string{"offset 41 and limit 35", "scripts/agentkit.sh show docs/adr/0069-quote-posts.md:41-75", "outline"}},
		{"sed -n after cd", `cd app/web/src && sed -n '1,80p' routes/login.tsx`, "read-range", []string{"show app/web/src/routes/login.tsx:1-80"}},
		{"sed -n with quit", `sed -n '200,300p;300q' docs/architecture/federation.md`, "read-range", []string{":200-300"}},
		{"head", `head -40 internal/app/content/service.go`, "read-range", []string{"offset 1 and limit 40"}},
		{"tail", `tail -30 internal/app/content/service.go; ls`, "read-range", []string{"Read tool"}},
		{"cat pipe head", `cat docs/adr/0069-quote-posts.md | head -8`, "read-range", []string{"Read tool"}},
		{"cat pipe jq", `cat app/web/package.json | jq .scripts`, "read-range", []string{"jq . app/web/package.json"}},
		{"awk NR", `awk 'NR>=20 && NR<=400' internal/app/content/service.go`, "read-range", []string{":20-400"}},
		{"sed on a pipe", `git show HEAD:go.mod | sed -n 1,20p`, "", nil},
		{"sed -n a log", `sed -n 1,80p .tmp/check.log`, "", nil},
		{"sed -n node_modules", `cd app && sed -n 1,80p node_modules/react/index.js`, "", nil},
		{"sed -n outside", `sed -n 1095,1140p ` + outside + `/pkg/mod/example.com/x@v1/x.go`, "", nil},
		{"sed -n a pattern range", `sed -n '/^func Create/,/^}/p' internal/app/content/service.go`, "", nil},
		{"tail a task output", `tail -5 ` + outside + `/tool-results/b1.txt`, "", nil},
		{"tail on a pipe", `go test ./... 2>&1 | tail -20`, "", nil},
		{"tail -f", `tail -f .tmp/check.log`, "", nil},
		{"cat small file", `cat internal/app/content/service.go`, "", nil},
		{"cat a heredoc into a file", "cat > docs/new.md <<'EOF'\n# New\nEOF", "", nil},
		{"cat the context pack", `cat .tmp/agentkit/ctx/916.md | head -400`, "", nil},

		// Writing files from the shell.
		{"sed -i", `sed -i 's|^export const autoplayKey = |const autoplayKey = |' app/web/src/routes/login.tsx && make check`, "write",
			[]string{"Edit tool", "app/web/src/routes/login.tsx"}},
		{"sed -i -E after cd", `cd app/web && sed -i -E 's#a#b#' src/routes/login.tsx`, "write", []string{"Edit tool"}},
		{"sed -i variable set here", `f=docs/adr/0069-quote-posts.md; sed -i -e '3s/Proposed/Accepted/' -e '48,49d' $f`, "write", nil},
		{"sed -i locales", `sed -i '/"status.quote.open"/d' locales/en/client.json locales/ja/client.json`, "write", []string{"scripts/i18n.sh add|mv|rm"}},
		{"perl -pi", `perl -pi -e 's/\bAccountMoved\b/Moved/g' internal/app/content/service.go`, "write", []string{"Edit tool"}},
		{"perl -0pi", `perl -0pi -e 's/a\nb/c/' internal/app/content/service.go`, "write", nil},
		{"python heredoc", "python3 - <<'EOF'\np='docs/adr/0069-quote-posts.md'\ns=open(p).read()\ns=s.replace('a','b')\nopen(p,'w').write(s)\nEOF", "write",
			[]string{"Edit tool", "docs/adr/0069-quote-posts.md"}},
		{"python literal target", `python3 -c "open('internal/app/content/service.go','w').write('x')"`, "write", nil},
		{"python f-string catalog", "python3 - <<'EOF'\nimport json\nfor loc in ['en','ja']:\n    p=f\"locales/{loc}/client.json\"\n    d=json.load(open(p))\n    json.dump(d,open(p,'w'))\nEOF", "write",
			[]string{"scripts/i18n.sh"}},
		{"python argument", "python3 - docs/adr/0069-quote-posts.md <<'EOF'\nimport sys\np=sys.argv[1]\ns=open(p).read()\nopen(p,'w').write(s)\nEOF", "write", nil},
		{"node", `node -e "const fs=require('fs');fs.writeFileSync('app/web/package.json', '{}')"`, "write", nil},
		{"sed -i under .tmp", `sed -i 's/a/b/' .tmp/edit.py && python3 .tmp/edit.py`, "", nil},
		{"sed -i outside", `sed -i 's/EXTRA EXTRA (/EXTRA (/' ` + outside + `/scratchpad/prompt.txt`, "", nil},
		{"sed without -i", `sed 's/\x1b\[[0-9;]*m//g' .tmp/check.log | tail -3`, "", nil},
		{"python reads the repository", "python3 - <<'EOF'\nimport json\nd=json.load(open('locales/en/client.json'))\nprint(len(d))\nEOF", "", nil},
		{"python writes scratch", "python3 - <<'EOF'\ns=open('docs/adr/0069-quote-posts.md').read()\nopen('.tmp/out.md','w').write(s)\nEOF", "", nil},
		{"python writes outside", "python3 -c \"\np='" + outside + "/scratchpad/prompt.txt'\ns=open(p).read()\nopen(p,'w').write(s)\"", "", nil},
		{"a script file", `python3 .tmp/edit.py docs/adr/0069-quote-posts.md`, "", nil},
		{"xargs sed -i", `git ls-files '*.go' | xargs sed -i 's/a/b/'`, "", nil},

		// Merging main.
		{"merge origin/main", `git fetch -q origin && git merge --no-edit origin/main 2>&1 | tail -3`, "merge-main",
			[]string{"git fetch origin && git rebase origin/main", "scripts/agentkit.sh regen", "git push --force-with-lease"}},
		{"merge main -m", `git merge origin/main -q -m "Merge remote-tracking branch 'origin/main' into feat/1445"`, "merge-main", nil},
		{"git -C merge", `git -C ` + root + ` merge --no-commit origin/main 2>&1 | tail -20`, "merge-main", nil},
		{"pull", `git pull -q`, "merge-main", []string{"git pull --ff-only"}},
		{"pull origin main", `git pull origin main`, "merge-main", nil},
		{"merge --ff-only main", `git merge -q --ff-only origin/main`, "", nil},
		{"merge a feature branch", `git merge --no-ff -q task/1001-media-viewer -m "Merge #1001 into epic/987-design-system"`, "", nil},
		{"merge --ff-only feature", `git merge -q --ff-only feat/2334-settings-pages`, "", nil},
		{"merge --abort", `git merge --abort`, "", nil},
		{"merge-base", `git merge-base --is-ancestor origin/main HEAD; echo uptodate=$?`, "", nil},
		{"merge-tree", `git merge-tree --write-tree HEAD origin/main >/dev/null; echo conflict=$?`, "", nil},
		{"pull --ff-only", `git switch -q main; git pull -q --ff-only`, "", nil},
		{"pull --rebase", `git pull --rebase origin main`, "", nil},
		{"rebase", `git fetch origin && git rebase origin/main`, "", nil},

		// Polling CI.
		{"pr checks", `gh pr checks 523 -R mycroblog/my 2>&1 | grep -v pass`, "ci-poll", []string{"scripts/agentkit.sh pr wait <pr>", "--log-failed"}},
		{"pr checks --watch", `sleep 30; gh pr checks 766 --watch --interval 30 2>&1 | tail -25`, "ci-poll", nil},
		{"run watch", `git push && gh run watch 123`, "ci-poll", nil},
		{"run view status", `gh run view 41033446660 --json status,conclusion --jq '"\(.status) \(.conclusion)"'`, "ci-poll", nil},
		{"run list", `gh run list -R mycroblog/my --branch task/2359-readme --limit 4 --json databaseId,status`, "ci-poll", nil},
		{"api check-runs", `gh api repos/mycroblog/my/commits/abc/check-runs --jq '.check_runs[].status'`, "ci-poll", nil},
		{"api actions runs", `gh api "repos/mycroblog/my/actions/runs/$id/jobs?per_page=50" --jq '.jobs[]'`, "ci-poll", nil},
		{"pr view rollup", `gh pr view 2036 --json statusCheckRollup --jq '[.statusCheckRollup[].conclusion]|unique'`, "ci-poll", nil},
		{"run view --log-failed", `gh run view 39225398023 -R mycroblog/my --log-failed 2>&1 | grep -B2 -A12 "EndsOnBroken" | head -40`, "", nil},
		{"run list of a workflow", `gh run list --workflow nightly.yml --limit 3 --json databaseId,conclusion`, "", nil},
		{"api job logs", `gh api repos/mycroblog/my/actions/jobs/129763696463/logs > .tmp/preflight.log 2>&1`, "", nil},
		{"pr view", `gh pr view 937 --json mergeable -q .mergeable`, "", nil},
		{"pr merge --auto", `gh pr merge 1688 -R mycroblog/my --auto --merge`, "", nil},
		{"pr wait", `scripts/agentkit.sh pr wait 2309 > .tmp/wait.log 2>&1`, "", nil},
		{"checks in a document", "cat > .tmp/doc.md <<'EOF'\nnever `gh pr checks --watch`\nEOF", "", nil},

		// Creating issues.
		{"issue create", `gh issue create -R mycroblog/my --title "Add the thing" --body-file .tmp/b.md --label type:task`, "issue-create",
			[]string{"scripts/agentkit.sh issue new --type task|bug|spike|adr|epic --title 'Add the thing' --body-file <file> --parent <epic>", "--blocked-by"}},
		{"issue create -t after cd", `cd docs && gh issue create -t Fix --body x`, "issue-create", []string{"--title Fix "}},
		{"issue create --title=", `gh issue create --title="A b" -F f`, "issue-create", []string{"--title 'A b'"}},
		{"issue create without a title", `url=$(gh issue create --body-file f)`, "issue-create", []string{"--title <title>"}},
		{"issue new", `gh issue new --web`, "issue-create", nil},
		{"issue edit", `gh issue edit 12 --milestone "M5: v0 Release"`, "", nil},
		{"issue comment", `gh issue comment 12 --body "create"`, "", nil},
		{"issue close", `gh issue close 12 -R mycroblog/my`, "", nil},
		{"issue view", `gh issue view 12 --json title`, "", nil},
		{"issue list", `gh issue list --search "issue create"`, "", nil},
		{"agentkit issue new", `scripts/agentkit.sh issue new --type task --title x --body-file f --parent 30`, "", nil},
		{"issue create in a message", "git commit -q -F - <<'EOF'\nnever `gh issue create`\nEOF", "", nil},

		// Waiting for a background command.
		{"until pgrep", `until ! pgrep -f "check:changed" >/dev/null; do sleep 10; done; tail -5 .tmp/check.log`, "wait-loop", []string{"run_in_background"}},
		{"pgrep", `pgrep -f 'go test' && echo running`, "wait-loop", nil},
		{"until grep sleep", `until grep -q '^ok' .tmp/out.txt; do sleep 5; done`, "wait-loop", nil},
		{"while sleep multiline", "while true\ndo\n  sleep 30\ndone", "wait-loop", nil},
		{"timeout bash -c until", `timeout 590 bash -c 'until grep -qiE "^(check summary|summary)|passed:|failed:" .tmp/i622-check.log; do sleep 10; done'`, "wait-loop", nil},
		{"timeout sh -c while", `timeout 3000 sh -c 'while ! [ -s ` + outside + `/tool-results/b1.txt ]; do sleep 20; done'`, "wait-loop", nil},
		{"while read", `git ls-files | while read f; do wc -l "$f"; done`, "", nil},
		{"sleep once", `sleep 2; docker logs my2243 2>&1 | tail -4`, "", nil},
		{"pgrep in a string", `echo 'never use pgrep' && grep -n sleep .tmp/check.log`, "", nil},
		{"loop in a message", "git commit -q -F - <<'EOF'\nIt waited with\n`until ! pgrep -f x; do sleep 10; done`.\nEOF", "", nil},

		// Whole big documents.
		{"cat big doc", `cat docs/architecture/federation.md`, "big-file", []string{"scripts/agentkit.sh outline docs/architecture/federation.md"}},
		{"cat big doc among others", `ls docs && cat docs/adr/0069-quote-posts.md docs/architecture/federation.md`, "big-file", nil},
		{"cat the context pack whole", `cat .tmp/agentkit/ctx/916.md`, "", nil},
		{"cat big file outside", `cat ` + outside + `/big.md`, "", nil},
		{"cat harness output", `cat ` + outside + `/tool-results/b1.txt`, "", nil},

		{"other", `go test ./...`, "", nil},
		{"unbalanced quote", `echo "unterminated; grep -rn x .`, "", nil},
	}
	for _, c := range cases {
		ck := &checker{session: root}
		d := ck.check(c.cmd)
		got := ""
		if d != nil {
			got = d.rule
		}
		if got != c.rule {
			t.Errorf("%s: %q: rule %q, want %q (%v)", c.name, c.cmd, got, c.rule, d)
			continue
		}
		for _, s := range c.say {
			if !strings.Contains(d.reason, s) {
				t.Errorf("%s: the reason does not name %q:\n%s", c.name, s, d.reason)
			}
		}
		// The hook prints the same verdict, and never an allow.
		out := run(t, "pre", map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": c.cmd}, "cwd": root})
		switch {
		case c.rule == "" && out != nil:
			t.Errorf("%s: the hook printed %v", c.name, out)
		case c.rule != "" && (out == nil || out["permissionDecision"] != "deny" || out["permissionDecisionReason"] != d.reason):
			t.Errorf("%s: the hook printed %v", c.name, out)
		}
	}
}

// A session whose working directory is not the root of the working tree is told how to reach agentkit from it.
func TestPreBashElsewhere(t *testing.T) {
	root, outside := fixture(t)
	sub := &checker{session: filepath.Join(root, "app", "web")}
	if d := sub.check(`grep -rn Login src`); d == nil || !strings.Contains(d.reason, root+"/scripts/agentkit.sh grep Login src`") {
		t.Errorf("from a subdirectory: %v", d)
	}
	out := &checker{session: outside}
	if d := out.check(`grep -rn Login .`); d != nil {
		t.Errorf("a search outside the repository was denied: %v", d)
	}
	if d := out.check(`cd ` + root + ` && grep -rn Login app`); d == nil || !strings.Contains(d.reason, root+"/scripts/agentkit.sh grep Login "+root+"/app") {
		t.Errorf("cd into the repository: %v", d)
	}
	if d := out.check(`cd "$W" && grep -rn Login app`); d != nil {
		t.Errorf("an unknown working directory was denied: %v", d)
	}
}

func TestPreRead(t *testing.T) {
	root, outside := fixture(t)
	read := func(p string, more map[string]any) map[string]any {
		in := map[string]any{"file_path": p}
		for k, v := range more {
			in[k] = v
		}
		return run(t, "pre", map[string]any{"tool_name": "Read", "tool_input": in, "cwd": root})
	}
	big := filepath.Join(root, "docs/architecture/federation.md")
	if out := read(big, nil); out == nil || out["permissionDecision"] != "deny" {
		t.Errorf("reading a whole big document was not denied: %v", out)
	}
	for name, out := range map[string]map[string]any{
		"in part":          read(big, map[string]any{"offset": 10, "limit": 50}),
		"a small document": read(filepath.Join(root, "docs/adr/0069-quote-posts.md"), nil),
		"the context pack": read(filepath.Join(root, ".tmp/agentkit/ctx/916.md"), nil),
		"outside":          read(filepath.Join(outside, "big.md"), nil),
		"harness output":   read(filepath.Join(outside, "tool-results/b1.txt"), nil),
		"missing":          read(filepath.Join(root, "docs/missing.md"), nil),
	} {
		if out != nil {
			t.Errorf("reading %s was denied: %v", name, out)
		}
	}
}

func TestPost(t *testing.T) {
	dir := t.TempDir()
	long := strings.Repeat("a line of output\n", 5000)
	out := run(t, "post", map[string]any{
		"tool_name": "Bash", "cwd": dir,
		"tool_input":    map[string]any{"command": "go test -v ./..."},
		"tool_response": map[string]any{"stdout": long, "stderr": "", "interrupted": false, "isImage": false},
	})
	if out == nil {
		t.Fatal("a long output was not cut")
	}
	resp := out["updatedToolOutput"].(map[string]any)
	stdout := resp["stdout"].(string)
	if len(stdout) > 20000 || !strings.Contains(stdout, "lines (") || resp["interrupted"] != false {
		t.Errorf("cut output is wrong: %d characters, interrupted=%v", len(stdout), resp["interrupted"])
	}
	files, _ := filepath.Glob(filepath.Join(dir, ".tmp", "agentkit", "out", "*.txt"))
	if len(files) != 1 {
		t.Fatalf("the whole output was not saved: %v", files)
	}
	if b, _ := os.ReadFile(files[0]); string(b) != long {
		t.Error("the saved output differs from the original")
	}
	short := run(t, "post", map[string]any{
		"tool_name": "Bash", "cwd": dir, "tool_input": map[string]any{"command": "ls"},
		"tool_response": map[string]any{"stdout": "a\nb\n", "stderr": ""},
	})
	if short != nil {
		t.Error("a short output was changed")
	}
	own := run(t, "post", map[string]any{
		"tool_name": "Bash", "cwd": dir, "tool_input": map[string]any{"command": "scripts/agentkit.sh ctx 916"},
		"tool_response": map[string]any{"stdout": long, "stderr": ""},
	})
	if own != nil {
		t.Error("agentkit's own output was cut")
	}
}
