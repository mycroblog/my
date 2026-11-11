// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ghNew answers the calls of New and Claim. #30 is an open epic in M5, #31 a task, #32 a closed epic, #33 an
// epic without a milestone; the created issue is #99; #7 and #8 are an issue and a pull request to wait for;
// #40 is ready and blocked by the open #7, #41 is neither ready nor in a milestone. A file $STATE/fail-<word>
// makes the call whose arguments contain <word> fail.
const ghNew = `
for f in "$STATE"/fail-*; do
  [ -e "$f" ] || continue
  case "$*" in *"${f##*/fail-}"*) echo "boom" >&2; exit 1 ;; esac
done
epic='"labels":[{"name":"type:epic"}]'
m5='"milestone":{"title":"M5: v0"}'
case "$*" in
  "api user"*) echo me ;;
  "issue view 30 "*) echo '{"number":30,"title":"Epic","state":"OPEN",'"$epic,$m5"'}' ;;
  "issue view 31 "*) echo '{"number":31,"title":"Task","state":"OPEN","labels":[{"name":"type:task"}],'"$m5"'}' ;;
  "issue view 32 "*) echo '{"number":32,"title":"Done","state":"CLOSED",'"$epic,$m5"'}' ;;
  "issue view 33 "*) echo '{"number":33,"title":"Loose","state":"OPEN",'"$epic"',"milestone":null}' ;;
  "issue view 40 "*) echo '{"number":40,"title":"Ready","state":"OPEN","labels":[{"name":"status:ready"}],'"$m5"'}' ;;
  "issue view 41 "*) echo '{"number":41,"title":"Raw","state":"OPEN","labels":[{"name":"type:task"}],"milestone":null}' ;;
  "api repos/mycroblog/my/milestones?state=open&per_page=100") echo '[{"number":3,"title":"M5: v0"},{"number":4,"title":"M6"}]' ;;
  "api repos/mycroblog/my/issues/7") echo '{"id":700,"number":7,"state":"open"}' ;;
  "api repos/mycroblog/my/issues/8") echo '{"id":800,"number":8,"state":"open","pull_request":{}}' ;;
  "api repos/mycroblog/my/issues/99") echo '{"id":9900,"number":99,"state":"open"}' ;;
  "issue create "*) echo "https://github.com/mycroblog/my/issues/99" ;;
  "issue edit "*) ;;
  "api -X POST repos/mycroblog/my/issues/30/sub_issues "*) echo '{"id":3000,"number":30,"sub_issues_summary":{"total":5,"completed":2}}' ;;
  "api -X POST repos/mycroblog/my/issues/99/dependencies/blocked_by "*) echo '{}' ;;
  "api graphql "*blockedBy*"number=40") echo '{"data":{"repository":{"issue":{"blockedBy":{"nodes":[{"number":7,"state":"OPEN"},{"number":6,"state":"CLOSED"}]}}}}}' ;;
  "api graphql "*blockedBy*) echo '{"data":{"repository":{"issue":{"blockedBy":{"nodes":[]}}}}}' ;;
  *) echo "unexpected: $*" >&2; exit 1 ;;
esac`

// calls returns the logged gh calls with the temporary body file's name removed.
func calls(t *testing.T, log string) []string {
	t.Helper()
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(readLog(t, log)), "\n") {
		if i := strings.Index(l, "--body-file "); i >= 0 {
			rest := l[i+len("--body-file "):]
			_, after, _ := strings.Cut(rest, " ")
			l = l[:i] + "--body-file F " + after
		}
		out = append(out, l)
	}
	return out
}

func TestNewSubIssue(t *testing.T) {
	log := fakeGH(t, ghNew)
	lines, err := New(NewOptions{Type: "task", Title: "Add it", Body: "Goal.\n\n- [ ] works like #12\n", Parent: 30,
		Areas: []string{"harness", "area:docs"}, Priority: "p1", Ready: true, BlockedBy: []int{7}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://github.com/mycroblog/my/issues/99", "sub-issue of #30 (3 of 5 open)", "blocked by #7 (open)"}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("got %q, want %q", lines, want)
	}
	wantCalls := []string{
		"issue view 30 -R mycroblog/my --json number,title,body,state,url,updatedAt,labels,assignees,comments,milestone",
		"api repos/mycroblog/my/issues/7",
		"issue create -R mycroblog/my --title Add it --body-file F --milestone M5: v0 --label area:harness --label area:docs",
		"issue edit 99 -R mycroblog/my --add-label type:task",
		"issue edit 99 -R mycroblog/my --add-label priority:p1",
		"api repos/mycroblog/my/issues/99",
		"api -X POST repos/mycroblog/my/issues/30/sub_issues -F sub_issue_id=9900",
		"api -X POST repos/mycroblog/my/issues/99/dependencies/blocked_by -F issue_id=700",
		"issue edit 99 -R mycroblog/my --add-label status:ready",
	}
	if got := calls(t, log); !reflect.DeepEqual(got, wantCalls) {
		t.Errorf("gh calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(wantCalls, "\n"))
	}
}

func TestNewEpicAndLooseIssue(t *testing.T) {
	log := fakeGH(t, ghNew)
	lines, err := New(NewOptions{Type: "epic", Title: "Search", Body: "Goal: search works.\n\nDone when #12 style queries answer.\n", Milestone: "M6"})
	if err != nil || len(lines) != 1 {
		t.Fatalf("lines %q, err %v", lines, err)
	}
	if _, err := New(NewOptions{Type: "bug", Title: "Crash", Body: "It crashes.", Milestone: "M5: v0"}); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(calls(t, log), "\n")
	for _, want := range []string{
		"issue create -R mycroblog/my --title Search --body-file F --milestone M6",
		"issue edit 99 -R mycroblog/my --add-label type:epic",
		"issue create -R mycroblog/my --title Crash --body-file F --milestone M5: v0",
		"issue edit 99 -R mycroblog/my --add-label type:bug",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("gh calls lack %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "sub_issues") || strings.Contains(got, "status:ready") {
		t.Errorf("an issue without a parent or --ready was linked or marked:\n%s", got)
	}
}

func TestNewRefusals(t *testing.T) {
	ok := NewOptions{Type: "task", Title: "Add it", Body: "Goal."}
	with := func(f func(*NewOptions)) NewOptions {
		o := ok
		f(&o)
		return o
	}
	cases := []struct {
		name string
		o    NewOptions
		want []string
	}{
		{"unknown type", with(func(o *NewOptions) { o.Type = "feature" }), []string{`--type "feature"`, "task, bug, spike, adr, epic"}},
		{"no body", with(func(o *NewOptions) { o.Body = " \n"; o.Parent = 30 }), []string{"--body-file"}},
		{"title prefix", with(func(o *NewOptions) { o.Title = "[Task] Add it"; o.Parent = 30 }), []string{"without one"}},
		{"priority", with(func(o *NewOptions) { o.Priority = "high"; o.Parent = 30 }), []string{"p0, p1, p2"}},
		{"task without a parent", ok, []string{"needs --parent <epic>", "gh issue list -R mycroblog/my --label type:epic", `"M5: v0", "M6"`}},
		{"parent is not an epic", with(func(o *NewOptions) { o.Parent = 31 }), []string{"#31 Task is not an epic", "--label type:epic"}},
		{"parent is closed", with(func(o *NewOptions) { o.Parent = 32 }), []string{"#32 Done is closed", "gh issue reopen 32"}},
		{"parent has no milestone", with(func(o *NewOptions) { o.Parent = 33 }), []string{`gh issue edit 33 -R mycroblog/my --milestone "<title>"`, `"M5: v0", "M6"`}},
		{"another milestone than the parent's", with(func(o *NewOptions) { o.Parent = 30; o.Milestone = "M6" }), []string{`the parent #30 ("M5: v0")`, "leave --milestone out"}},
		{"unknown milestone", with(func(o *NewOptions) { o.Milestone = "M9" }), []string{`"M9" is not an open milestone`, `"M5: v0", "M6"`, "creates no milestone"}},
		{"epic without a milestone", with(func(o *NewOptions) { o.Type = "epic" }), []string{"an epic needs --milestone", `"M5: v0", "M6"`}},
		{"epic with a parent", with(func(o *NewOptions) { o.Type = "epic"; o.Parent = 30 }), []string{"an epic has no parent"}},
		{"epic with a task list", with(func(o *NewOptions) {
			o.Type = "epic"
			o.Milestone = "M6"
			o.Body = "Goal.\n\n- [ ] #12 first\n- [x] #13\n"
		}),
			[]string{"2 lines", `"- [ ] #12 first"`, "an epic's tasks are its sub-issues", "agentkit issue new --parent <epic>", "--blocked-by"}},
		{"blocked by a pull request", with(func(o *NewOptions) { o.Parent = 30; o.BlockedBy = []int{8} }), []string{"--blocked-by 8", "pull request"}},
		{"blocked by nothing known", with(func(o *NewOptions) { o.Parent = 30; o.BlockedBy = []int{5} }), []string{"--blocked-by 5"}},
	}
	for _, c := range cases {
		log := fakeGH(t, ghNew)
		lines, err := New(c.o)
		if err == nil {
			t.Errorf("%s: created %q", c.name, lines)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%s: the refusal does not say %q:\n%v", c.name, w, err)
			}
		}
		if got := readLog(t, log); strings.Contains(got, "issue create") || strings.Contains(got, "-X POST") || strings.Contains(got, "issue edit") {
			t.Errorf("%s: a refusal changed something:\n%s", c.name, got)
		}
	}
}

func TestEpicBodyProblem(t *testing.T) {
	for body, bad := range map[string]bool{
		"Goal: federation works.\n\nDone when a post reaches Mastodon (see #12).": false,
		"- the inbox verifies signatures\n- [ ] delivery retries":                 false,
		"| Area | Notes |\n| --- | --- |\n| inbox | like #12 |":                   false,
		"- [ ] #123":                           true,
		"* #123 the inbox (after #122)":        true,
		"1. #123 inbox":                        true,
		"  - [x] #123":                         true,
		"| #123 | inbox |":                     true,
		"- https://github.com/o/r/issues/123":  true,
		"Tasks:\n\n- [ ] #1\n- [ ] #2\n\nDone": true,
	} {
		if got := epicBodyProblem(body) != ""; got != bad {
			t.Errorf("epicBodyProblem(%q) refused = %v, want %v", body, got, bad)
		}
	}
}

func TestNewReportsWhatIsLeft(t *testing.T) {
	log := fakeGH(t, ghNew)
	write(t, filepath.Join(filepath.Dir(log), "fail-sub_issues"), "")
	lines, err := New(NewOptions{Type: "task", Title: "Add it", Body: "Goal.", Parent: 30, Ready: true})
	if err == nil {
		t.Fatalf("a failed link must be an error, got %q", lines)
	}
	if len(lines) != 1 || lines[0] != "https://github.com/mycroblog/my/issues/99" {
		t.Errorf("the URL of the created issue is not printed: %q", lines)
	}
	for _, w := range []string{"#99 is created (do not create it again)", "gh api -X POST repos/mycroblog/my/issues/30/sub_issues -F sub_issue_id=", "then mark it ready"} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("the error does not say %q:\n%v", w, err)
		}
	}
	if strings.Contains(readLog(t, log), "status:ready") {
		t.Error("an issue that is not linked to its parent must not be marked ready")
	}
}

func TestClaimRefusals(t *testing.T) {
	fakeGH(t, ghNew)
	_, err := Claim(t.TempDir(), 41, "")
	for _, w := range []string{"do not take #41", "not labeled status:ready", `it has no milestone (gh issue edit 41 -R mycroblog/my --milestone "<title>"; open milestones: "M5: v0", "M6")`} {
		if err == nil || !strings.Contains(err.Error(), w) {
			t.Errorf("the refusal does not say %q: %v", w, err)
		}
	}
	// The dependency issue new --blocked-by records stops the claim while the blocker is open.
	_, err = Claim(t.TempDir(), 40, "")
	if err == nil || !strings.Contains(err.Error(), "do not take #40: waits for #7, which is open (") {
		t.Errorf("an issue blocked by an open issue was not refused: %v", err)
	}
}

func TestClaim(t *testing.T) {
	log := fakeGH(t, ghNew)
	orig := openBlockers
	t.Cleanup(func() { openBlockers = orig })
	openBlockers = func(int) ([]int, error) { return nil, nil }
	line, err := Claim(t.TempDir(), 40, "")
	if err != nil || line != "claimed #40 Ready (assigned @me)" {
		t.Errorf("line %q, err %v", line, err)
	}
	if !strings.Contains(readLog(t, log), "issue edit 40 -R mycroblog/my --add-assignee @me") {
		t.Error("the issue was not assigned")
	}
}
