// SPDX-License-Identifier: AGPL-3.0-only

package gh

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeOrg is the organization's answer: the types, the Priority and Area fields, and a text field.
const fakeOrg = `{"data":{"organization":{"issueTypes":{"nodes":[{"id":"IT_task","name":"Task","isEnabled":true},{"id":"IT_bug","name":"Bug","isEnabled":true},{"id":"IT_old","name":"Spike","isEnabled":false}]},` +
	`"issueFields":{"nodes":[{"__typename":"IssueFieldSingleSelect","id":"F_prio","name":"Priority","options":[{"id":"O_p0","name":"P0"},{"id":"O_p1","name":"P1"}]},` +
	`{"__typename":"IssueFieldMultiSelect","id":"F_area","name":"Area","options":[{"id":"O_web","name":"web"},{"id":"O_docs","name":"docs"}]},{"__typename":"IssueFieldText"}]}}}}`

// fake puts a gh on PATH that logs its arguments and answers the calls of this file.
func fake(t *testing.T) (log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "gh.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + log + "'\ncase \"$*\" in\n" +
		"  *'organization(login'*) echo '" + fakeOrg + "' ;;\n" +
		"  'api repos/mycroblog/my/issues/5 --jq .node_id') echo I_5 ;;\n" +
		"  'issue view 5 '*) echo '{\"number\":5,\"title\":\"Five\",\"state\":\"OPEN\"}' ;;\n" +
		"  'issue view 6 '*) echo '{\"number\":6,\"title\":\"A PR\",\"state\":\"MERGED\"}' ;;\n" +
		"  *issueOrPullRequest*number=5) echo '{\"data\":{\"repository\":{\"issueOrPullRequest\":{\"id\":\"I_5\",\"issueType\":{\"name\":\"Task\"},\"issueFieldValues\":{\"nodes\":[" +
		"{\"name\":\"P1\",\"field\":{\"name\":\"Priority\"}},{\"options\":[{\"name\":\"web\"},{\"name\":\"docs\"}],\"field\":{\"name\":\"Area\"}},{}]}," +
		"\"blockedBy\":{\"nodes\":[{\"number\":7,\"title\":\"Pick a queue\",\"state\":\"OPEN\",\"issueType\":{\"name\":\"ADR\"}},{\"number\":8,\"title\":\"Done\",\"state\":\"CLOSED\",\"issueType\":null}]}}}}}' ;;\n" +
		"  *issueOrPullRequest*number=6) echo '{\"data\":{\"repository\":{\"issueOrPullRequest\":{}}}}' ;;\n" +
		"  *mutation*O_p0*) echo refused >&2; exit 1 ;;\n" +
		"  *mutation*) echo '{}' ;;\n" +
		"  *) echo \"unexpected: $*\" >&2; exit 1 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AGENTKIT_REPO", "")
	Forget()
	t.Cleanup(Forget)
	return log
}

// The seam of an issue's type, priority, areas and blockers: what the rest of agentkit asks.
func TestClassifyReads(t *testing.T) {
	fake(t)
	is, err := GetIssue(5)
	if err != nil {
		t.Fatal(err)
	}
	if is.Type != "Task" || is.IsEpic() || is.Priority() != "P1" || !reflect.DeepEqual(is.Areas(), []string{"web", "docs"}) || is.ID != "I_5" {
		t.Errorf("got type %q, priority %q, areas %q, id %q", is.Type, is.Priority(), is.Areas(), is.ID)
	}
	if open := is.OpenBlockers(); len(open) != 1 || open[0].Waits() != "waits for the decision #7: Pick a queue" || !NeedsDecision(is) {
		t.Errorf("open blockers %+v, needs decision %v", open, NeedsDecision(is))
	}
	// Labels say nothing: only the type and the fields count.
	old := &Issue{Labels: []Label{{Name: "type:epic"}, {Name: "priority:p0"}, {Name: "area:web"}}}
	if old.IsEpic() || old.Priority() != "" || old.Areas() != nil || NeedsDecision(old) {
		t.Error("an issue is classified by its labels")
	}
	pr, err := GetIssue(6)
	if err != nil || pr.Type != "" || pr.Title != "A PR" {
		t.Errorf("a pull request read as an issue: %+v, %v", pr, err)
	}
	for typ, want := range map[string]string{"Epic": "an epic is done through its sub-issues", "ADR": "an ADR issue is decided by the decider", "Spike": "", "Task": "", "": ""} {
		if got := (&Issue{Type: typ}).Unclaimable(); !strings.Contains(got, want) || (want == "") != (got == "") {
			t.Errorf("Unclaimable of %q = %q, want %q", typ, got, want)
		}
	}
	for b, want := range map[Blocker]string{
		{Number: 3, Title: "Try it", Type: "Spike"}:  "waits for the investigation #3: Try it",
		{Number: 4, Title: "Build it", Type: "Task"}: "waits for #4, which is open",
		{Number: 5, Title: "Old"}:                    "waits for #5, which is open",
	} {
		if got := b.Waits(); got != want {
			t.Errorf("Waits = %q, want %q", got, want)
		}
	}
	if EpicFilter() != "--search type:Epic" {
		t.Errorf("EpicFilter = %q", EpicFilter())
	}
}

// What reaches gh: ids resolved by name, in one query of the organization a process.
func TestClassifyWrites(t *testing.T) {
	log := fake(t)
	for _, err := range []error{SetType(5, "bug"), SetPriority(5, "p1"), SetAreas(5, []string{"web", "docs"}), CheckAreas([]string{"docs"})} {
		if err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		"updateIssueIssueType(input: {issueId: $issue, issueTypeId: $type}) { issue { number } } } -f issue=I_5 -f type=IT_bug\n",
		"singleSelectOptionId: $option}]}) { issue { number } } } -f issue=I_5 -f field=F_prio -f option=O_p1\n",
		"multiSelectOptionIds: $options}]}) { issue { number } } } -f issue=I_5 -f field=F_area -f options[]=O_web -f options[]=O_docs\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("gh calls lack %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "organization(login"); n != 1 {
		t.Errorf("the organization was read %d times, want once", n)
	}
	if n := strings.Count(got, ".node_id"); n != 1 {
		t.Errorf("the issue's id was read %d times, want once", n)
	}

	for name, c := range map[string]struct {
		err  error
		want []string
	}{
		"a type the organization lacks": {SetType(5, "epic"), []string{`no issue type "Epic"`, `"Bug", "Task"`, "organization's issue types settings"}},
		"a disabled type":               {SetType(5, "spike"), []string{`no issue type "Spike"`}},
		"an option the field lacks":     {SetPriority(5, "p2"), []string{`"Priority"`, `no option "P2"`, `"P0", "P1"`, "organization's issue fields settings"}},
		"an unknown area":               {CheckAreas([]string{"web", "area51"}), []string{`"Area"`, `no option "area51"`, `"docs", "web"`}},
		"a field of the other kind":     {setField(5, "Area", false, "web"), []string{`no single-select issue field "Area"`, "organization's issue fields settings"}},
		"a missing field":               {setField(5, "Size", false, "L"), []string{`no single-select issue field "Size"`, `"Area", "Priority"`}},
		"a refused mutation":            {SetPriority(5, "p0"), []string{"refused", "to do it by hand: set Priority to P0 on the page of #5"}},
	} {
		for _, w := range c.want {
			if c.err == nil || !strings.Contains(c.err.Error(), w) {
				t.Errorf("%s: the error does not say %q: %v", name, w, c.err)
			}
		}
	}
}
