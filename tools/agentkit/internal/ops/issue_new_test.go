// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mycroblog/my/tools/agentkit/internal/gh"
)

// ghNew answers the calls of New, Claim and Block. #30 is an open epic in M5, #31 a task, #32 a closed epic,
// #33 an epic without a milestone; the created issue is #99; #7 and #8 are an issue and a pull request to wait
// for; #40 is blocked by the open task #7, #41 has no milestone, #42 waits for the decision #50 and the
// investigation #51, #44 is an ADR issue, #45 a spike nothing blocks. The old labels of #41 and #45 say
// nothing. A file $STATE/fail-<word> makes the call whose arguments contain <word> fail.
const ghNew = `
for f in "$STATE"/fail-*; do
  [ -e "$f" ] || continue
  case "$*" in *"${f##*/fail-}"*) echo "boom" >&2; exit 1 ;; esac
done
m5='"milestone":{"title":"M5: v0"}'
native() { echo '{"data":{"repository":{"issueOrPullRequest":{"id":"I_1","issueType":{"name":"'"$1"'"},"blockedBy":{"nodes":['"$2"']}}}}}'; }
case "$*" in
  "api user"*) echo me ;;
  "issue view 30 "*) echo '{"number":30,"title":"Epic","state":"OPEN",'"$m5"'}' ;;
  "issue view 31 "*) echo '{"number":31,"title":"Task","state":"OPEN",'"$m5"'}' ;;
  "issue view 32 "*) echo '{"number":32,"title":"Done","state":"CLOSED",'"$m5"'}' ;;
  "issue view 33 "*) echo '{"number":33,"title":"Loose","state":"OPEN","milestone":null}' ;;
  "issue view 40 "*) echo '{"number":40,"title":"Blocked","state":"OPEN",'"$m5"'}' ;;
  "issue view 41 "*) echo '{"number":41,"title":"Raw","state":"OPEN","labels":[{"name":"status:needs-decision"}],"milestone":null}' ;;
  "issue view 42 "*) echo '{"number":42,"title":"Stuck","state":"OPEN","body":"Depends on #50.",'"$m5"'}' ;;
  "issue view 44 "*) echo '{"number":44,"title":"Pick a queue","state":"OPEN",'"$m5"'}' ;;
  "issue view 45 "*) echo '{"number":45,"title":"Probe","state":"OPEN","labels":[{"name":"type:epic"}],'"$m5"'}' ;;
  *issueOrPullRequest*number=30|*issueOrPullRequest*number=32|*issueOrPullRequest*number=33) native Epic ;;
  *issueOrPullRequest*number=40) native Task '{"number":7,"title":"First","state":"OPEN","issueType":{"name":"Task"}},{"number":6,"title":"Old","state":"CLOSED","issueType":null}' ;;
  *issueOrPullRequest*number=42) native Task '{"number":50,"title":"Pick a queue","state":"OPEN","issueType":{"name":"ADR"}},{"number":51,"title":"Measure it","state":"OPEN","issueType":{"name":"Spike"}}' ;;
  *issueOrPullRequest*number=44) native ADR ;;
  *issueOrPullRequest*number=45) native Spike ;;
  *issueOrPullRequest*) native Task ;;
  *"organization(login"*) echo '{"data":{"organization":{"issueTypes":{"nodes":[{"id":"IT_task","name":"Task","isEnabled":true},{"id":"IT_bug","name":"Bug","isEnabled":true},{"id":"IT_epic","name":"Epic","isEnabled":true},{"id":"IT_spike","name":"Spike","isEnabled":true},{"id":"IT_adr","name":"ADR","isEnabled":true}]},"issueFields":{"nodes":[{"__typename":"IssueFieldSingleSelect","id":"F_prio","name":"Priority","options":[{"id":"O_p0","name":"P0"},{"id":"O_p1","name":"P1"},{"id":"O_p2","name":"P2"}]},{"__typename":"IssueFieldMultiSelect","id":"F_area","name":"Area","options":[{"id":"O_harness","name":"harness"},{"id":"O_docs","name":"docs"}]}]}}}}' ;;
  "api graphql "*mutation*) echo '{}' ;;
  "api repos/mycroblog/my/milestones?state=open&per_page=100") echo '[{"number":3,"title":"M5: v0"},{"number":4,"title":"M6"}]' ;;
  "api repos/mycroblog/my/issues/7") echo '{"id":700,"number":7,"state":"open"}' ;;
  "api repos/mycroblog/my/issues/8") echo '{"id":800,"number":8,"state":"open","pull_request":{}}' ;;
  "api repos/mycroblog/my/issues/99") echo '{"id":9900,"number":99,"state":"open"}' ;;
  "api repos/mycroblog/my/issues/99 --jq .node_id") echo I_99 ;;
  "issue create "*) echo "https://github.com/mycroblog/my/issues/99" ;;
  "issue edit "*) ;;
  "issue comment "*) ;;
  "api -X POST repos/mycroblog/my/issues/30/sub_issues "*) echo '{"id":3000,"number":30,"sub_issues_summary":{"total":5,"completed":2}}' ;;
  "api -X POST repos/mycroblog/my/issues/"*"/dependencies/blocked_by "*) echo '{}' ;;
  *) echo "unexpected: $*" >&2; exit 1 ;;
esac`

// ghClassify is fakeGH after forgetting what the process read of the organization, so that each test sees its
// own calls.
func ghClassify(t *testing.T) (log string) {
	t.Helper()
	gh.Forget()
	t.Cleanup(gh.Forget)
	return fakeGH(t, ghNew)
}

// calls returns the logged gh calls with the temporary body file's name removed and each GraphQL document cut
// to its operation.
func calls(t *testing.T, log string) []string {
	t.Helper()
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(readLog(t, log)), "\n") {
		if i := strings.Index(l, "--body-file "); i >= 0 {
			rest := l[i+len("--body-file "):]
			_, after, _ := strings.Cut(rest, " ")
			l = l[:i] + "--body-file F " + after
		}
		if q, vars, ok := strings.Cut(l, " } } -f "); ok && strings.HasPrefix(q, "api graphql -f query=mutation") {
			op := strings.Fields(strings.SplitN(q, "{ ", 2)[1])[0]
			l = "mutation " + strings.SplitN(op, "(", 2)[0] + " -f " + vars
		}
		switch {
		case strings.Contains(l, "issueOrPullRequest"):
			l = "query issue " + l[strings.LastIndex(l, "number="):]
		case strings.Contains(l, "organization(login"):
			l = "query organization"
		}
		out = append(out, l)
	}
	return out
}

func TestNewSubIssue(t *testing.T) {
	log := ghClassify(t)
	lines, err := New(NewOptions{Type: "task", Title: "Add it", Body: "Goal.\n\n- [ ] works like #12\n", Parent: 30,
		Areas: []string{"harness", "area:docs"}, Priority: "p1", BlockedBy: []int{7}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://github.com/mycroblog/my/issues/99", "sub-issue of #30 (3 of 5 open)", "blocked by #7 (open)"}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("got %q, want %q", lines, want)
	}
	wantCalls := []string{
		"query organization",
		"issue view 30 -R mycroblog/my --json number,title,body,state,url,updatedAt,labels,assignees,comments,milestone",
		"query issue number=30",
		"api repos/mycroblog/my/issues/7",
		"issue create -R mycroblog/my --title Add it --body-file F --milestone M5: v0",
		"api repos/mycroblog/my/issues/99 --jq .node_id",
		"mutation updateIssueIssueType -f issue=I_99 -f type=IT_task",
		"mutation setIssueFieldValue -f issue=I_99 -f field=F_prio -f option=O_p1",
		"mutation setIssueFieldValue -f issue=I_99 -f field=F_area -f options[]=O_harness -f options[]=O_docs",
		"api repos/mycroblog/my/issues/99",
		"api -X POST repos/mycroblog/my/issues/30/sub_issues -F sub_issue_id=9900",
		"api -X POST repos/mycroblog/my/issues/99/dependencies/blocked_by -F issue_id=700",
	}
	if got := calls(t, log); !reflect.DeepEqual(got, wantCalls) {
		t.Errorf("gh calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(wantCalls, "\n"))
	}
}

func TestNewEpicAndLooseIssue(t *testing.T) {
	log := ghClassify(t)
	lines, err := New(NewOptions{Type: "epic", Title: "Search", Body: "Goal: search works.\n\nDone when #12 style queries answer.\n", Milestone: "M6"})
	if err != nil || len(lines) != 1 {
		t.Fatalf("lines %q, err %v", lines, err)
	}
	if _, err := New(NewOptions{Type: "bug", Title: "Crash", Body: "It crashes.", Milestone: "M5: v0"}); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(calls(t, log), "\n")
	for _, want := range []string{
		"issue create -R mycroblog/my --title Search --body-file F --milestone M6\n",
		"mutation updateIssueIssueType -f issue=I_99 -f type=IT_epic",
		"issue create -R mycroblog/my --title Crash --body-file F --milestone M5: v0\n",
		"mutation updateIssueIssueType -f issue=I_99 -f type=IT_bug",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("gh calls lack %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "sub_issues") || strings.Contains(got, "setIssueFieldValue") || strings.Contains(got, "-label") {
		t.Errorf("an issue without a parent, a priority or an area was linked, given a field or labeled:\n%s", got)
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
		{"unknown area", with(func(o *NewOptions) { o.Areas = []string{"docs", "moon"}; o.Parent = 30 }), []string{`--area`, `no option "moon"`, `"docs", "harness"`}},
		{"task without a parent", ok, []string{"needs --parent <epic>", "gh issue list -R mycroblog/my --search type:Epic", `"M5: v0", "M6"`}},
		{"parent is not an epic", with(func(o *NewOptions) { o.Parent = 31 }), []string{"#31 Task is not an epic", "--search type:Epic"}},
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
		log := ghClassify(t)
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
		if got := readLog(t, log); strings.Contains(got, "issue create") || strings.Contains(got, "-X POST") || strings.Contains(got, "issue edit") || strings.Contains(got, "mutation") {
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
	log := ghClassify(t)
	write(t, filepath.Join(filepath.Dir(log), "fail-sub_issues"), "")
	write(t, filepath.Join(filepath.Dir(log), "fail-updateIssueIssueType"), "")
	lines, err := New(NewOptions{Type: "task", Title: "Add it", Body: "Goal.", Parent: 30, Priority: "p2"})
	if err == nil {
		t.Fatalf("a failed link must be an error, got %q", lines)
	}
	if len(lines) != 1 || lines[0] != "https://github.com/mycroblog/my/issues/99" {
		t.Errorf("the URL of the created issue is not printed: %q", lines)
	}
	for _, w := range []string{"#99 is created (do not create it again)", "set the type task", "to do it by hand: set the type Task on the page of #99",
		"gh api -X POST repos/mycroblog/my/issues/30/sub_issues -F sub_issue_id="} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("the error does not say %q:\n%v", w, err)
		}
	}
	if !strings.Contains(readLog(t, log), "field=F_prio -f option=O_p2") {
		t.Error("a failed step stopped the ones after it")
	}
}

func TestClaimRefusals(t *testing.T) {
	for n, want := range map[int][]string{
		41: {"do not take #41: it has no milestone (gh issue edit 41 -R mycroblog/my --milestone \"<title>\"; open milestones: \"M5: v0\", \"M6\") (workflow.md §1)"},
		// The dependency issue new --blocked-by records stops the claim while the blocker is open.
		40: {"do not take #40: waits for #7, which is open (workflow.md §1)"},
		// What issue block records: a decision and an investigation; the description naming #50 adds no line.
		42: {"do not take #42: waits for the decision #50: Pick a queue; waits for the investigation #51: Measure it (workflow.md §1)"},
		44: {"do not take #44: an ADR issue is decided by the decider; draft the ADR in a pull request and link it ("},
		30: {"do not take #30: an epic is done through its sub-issues ("},
	} {
		log := ghClassify(t)
		_, err := Claim(t.TempDir(), n, "")
		for _, w := range want {
			if err == nil || !strings.Contains(err.Error(), w) {
				t.Errorf("#%d: the refusal does not say %q: %v", n, w, err)
			}
		}
		if strings.Contains(readLog(t, log), "issue edit") {
			t.Errorf("#%d: a refused claim assigned the issue", n)
		}
	}
}

// A spike is taken like a task, and nothing but the type, the milestone and the blockers decides: no label.
func TestClaim(t *testing.T) {
	log := ghClassify(t)
	line, err := Claim(t.TempDir(), 45, "")
	if err != nil || line != "claimed #45 Probe (assigned @me)" {
		t.Errorf("line %q, err %v", line, err)
	}
	if !strings.Contains(readLog(t, log), "issue edit 45 -R mycroblog/my --add-assignee @me") {
		t.Error("the issue was not assigned")
	}
}

func TestBlock(t *testing.T) {
	log := ghClassify(t)
	lines, err := Block(BlockOptions{N: 31, Type: "adr", Title: "Pick a queue", Reason: "Tried A.\n\nOptions: A, B. Recommend B.\n"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://github.com/mycroblog/my/issues/99", "blocked #31: it waits for the decision #99: Pick a queue"}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("got %q, want %q", lines, want)
	}
	got := strings.Join(calls(t, log), "\n")
	for _, w := range []string{
		"issue create -R mycroblog/my --title Pick a queue --body-file F --milestone M5: v0\n",
		"mutation updateIssueIssueType -f issue=I_99 -f type=IT_adr\n",
		"api -X POST repos/mycroblog/my/issues/31/dependencies/blocked_by -F issue_id=9900\n",
		"issue comment 31 -R mycroblog/my --body This waits for the decision #99: Pick a queue",
		"https://github.com/mycroblog/my/issues/99",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("gh calls lack %q:\n%s", w, got)
		}
	}
	if strings.Contains(got, "sub_issues") || strings.Contains(got, "-label") {
		t.Errorf("the blocker got a parent or a label:\n%s", got)
	}

	log = ghClassify(t)
	lines, err = Block(BlockOptions{N: 31, Type: "spike", Title: "Measure it", Reason: "Unknown cost."})
	if err != nil || lines[1] != "blocked #31: it waits for the investigation #99: Measure it" || !strings.Contains(readLog(t, log), "type=IT_spike") {
		t.Errorf("lines %q, err %v", lines, err)
	}

	log = ghClassify(t)
	lines, err = Block(BlockOptions{N: 31, On: 7})
	if err != nil || !reflect.DeepEqual(lines, []string{"blocked #31: it waits for #7 (open)"}) {
		t.Errorf("lines %q, err %v", lines, err)
	}
	if got := calls(t, log); !reflect.DeepEqual(got, []string{"api repos/mycroblog/my/issues/7", "api -X POST repos/mycroblog/my/issues/31/dependencies/blocked_by -F issue_id=700"}) {
		t.Errorf("--on made the calls %q", got)
	}
}

func TestBlockRefusals(t *testing.T) {
	ok := BlockOptions{N: 31, Type: "adr", Title: "Pick a queue", Reason: "Options: A, B."}
	for name, c := range map[string]struct {
		o    BlockOptions
		want string
	}{
		"no type":                {BlockOptions{N: 31, Title: "t", Reason: "r"}, "adr when the decider has to decide, spike when an investigation comes first"},
		"a type that is work":    {BlockOptions{N: 31, Type: "task", Title: "t", Reason: "r"}, `--type "task"`},
		"no title":               {BlockOptions{N: 31, Type: "adr", Reason: "r"}, "--title is required"},
		"no reason":              {BlockOptions{N: 31, Type: "spike", Title: "t", Reason: " "}, "a reason is required"},
		"no milestone":           {BlockOptions{N: 41, Type: "adr", Title: "t", Reason: "r"}, "#41 has no milestone"},
		"on itself":              {BlockOptions{N: 31, On: 31}, "does not wait for itself"},
		"on a pull request":      {BlockOptions{N: 31, On: 8}, "that is a pull request"},
		"on an issue not there":  {BlockOptions{N: 31, On: 5}, "--on 5: could not read the issue"},
		"the issue is not there": {BlockOptions{N: 5, Type: "adr", Title: "t", Reason: "r"}, "issue view 5"},
	} {
		log := ghClassify(t)
		lines, err := Block(c.o)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %q, %v; want a refusal that says %q", name, lines, err, c.want)
		}
		if got := readLog(t, log); strings.Contains(got, "issue create") || strings.Contains(got, "-X POST") || strings.Contains(got, "issue comment") {
			t.Errorf("%s: a refusal changed something:\n%s", name, got)
		}
	}

	// A failure after the issue exists says how to finish, not to start again.
	log := ghClassify(t)
	write(t, filepath.Join(filepath.Dir(log), "fail-dependencies"), "")
	lines, err := Block(ok)
	if err == nil || !strings.Contains(err.Error(), "#99 is created (do not create it again); still to do: agentkit issue block 31 --on 99") || len(lines) != 1 {
		t.Errorf("lines %q, err %v", lines, err)
	}
}
