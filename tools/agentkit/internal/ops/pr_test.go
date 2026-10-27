// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"reflect"
	"strconv"
	"testing"
)

func TestTransitions(t *testing.T) {
	pending := prSnapshot{ci: "pending", mergeable: "MERGEABLE"}
	queued := prSnapshot{ci: "passed", mergeable: "MERGEABLE", queued: true}
	cases := []struct {
		name string
		old  prSnapshot
		had  bool
		cur  prSnapshot
		want []string
	}{
		{"first poll, pending", prSnapshot{}, false, pending, nil},
		{"ci passes", pending, true, prSnapshot{ci: "passed", mergeable: "MERGEABLE"}, []string{"ci-green"}},
		{"ci fails", pending, true, prSnapshot{ci: "failed", mergeable: "MERGEABLE", failed: []string{"go"}}, []string{"ci-failed go"}},
		{"ci cancelled", pending, true, prSnapshot{ci: "cancelled", mergeable: "MERGEABLE", failed: []string{"go"}}, []string{"ci-cancelled go"}},
		{"enters the queue", prSnapshot{ci: "passed", mergeable: "MERGEABLE"}, true, queued, []string{"queued"}},
		{"leaves the queue open", queued, true, prSnapshot{ci: "passed", mergeable: "CONFLICTING"}, []string{"conflicting", "dequeued (out of the merge queue without merging: a conflict or a failed merge group)"}},
		{"stays queued", queued, true, queued, nil},
		{"first poll, already queued", prSnapshot{}, false, queued, nil},
	}
	for _, c := range cases {
		if got := transitions(c.old, c.had, c.cur); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func run(workflow, name, conclusion string, job int) check {
	status := "COMPLETED"
	if conclusion == "" {
		status = "IN_PROGRESS"
	}
	return check{
		Typename:     "CheckRun",
		Name:         name,
		WorkflowName: workflow,
		Status:       status,
		Conclusion:   conclusion,
		DetailsURL:   "https://github.com/mycroblog/my/actions/runs/1/job/" + strconv.Itoa(job),
	}
}

func TestSummarize(t *testing.T) {
	gate := run("CI", "check", "SUCCESS", 99)
	cases := []struct {
		name        string
		cs          []check
		want        string
		wantNames   []string
		wantPending bool
	}{
		{"no checks", nil, "none", nil, false},
		{"cancelled run replaced by a passing one", []check{
			run("CI", "lint", "CANCELLED", 10), run("CI", "lint", "SUCCESS", 20), gate,
		}, "passed", nil, false},
		{"newer run listed first", []check{
			run("CI", "lint", "SUCCESS", 20), run("CI", "lint", "CANCELLED", 10), gate,
		}, "passed", nil, false},
		{"cancelled run replaced by a running one", []check{
			run("CI", "lint", "CANCELLED", 10), run("CI", "lint", "", 20),
		}, "pending", nil, true},
		{"failure of the newer run", []check{
			run("CI", "lint", "CANCELLED", 10), run("CI", "lint", "FAILURE", 20), run("CI", "test", "SUCCESS", 21),
		}, "failed", []string{"lint"}, false},
		{"failure of an older run replaced by a pass", []check{
			run("CI", "lint", "FAILURE", 10), run("CI", "lint", "SUCCESS", 20), gate,
		}, "passed", nil, false},
		{"only cancelled checks left", []check{
			run("CI", "test", "CANCELLED", 11), run("CI", "lint", "CANCELLED", 10),
		}, "cancelled", []string{"lint", "test"}, false},
		{"same name in two workflows", []check{
			run("CI", "check", "SUCCESS", 20), run("Nightly", "check", "FAILURE", 10),
		}, "failed", []string{"check"}, false},
		{"status context", []check{
			{Typename: "StatusContext", Context: "ci/legacy", State: "SUCCESS"}, gate,
		}, "passed", nil, false},
		{"other checks passed, the gate has not reported", []check{
			run("CI", "lint", "SUCCESS", 20), run("CI", "test", "SUCCESS", 21),
		}, "pending", nil, true},
		{"only a status context passed", []check{
			{Typename: "StatusContext", Context: "ci/legacy", State: "SUCCESS"},
		}, "pending", nil, true},
		{"a check of another workflow named check is not the gate", []check{
			run("Nightly", "check", "SUCCESS", 20), run("CI", "lint", "SUCCESS", 21),
		}, "pending", nil, true},
		{"the gate is running", []check{
			run("CI", "lint", "SUCCESS", 20), run("CI", "check", "", 22),
		}, "pending", nil, true},
		{"the gate passed", []check{
			run("CI", "lint", "SUCCESS", 20), run("CI", "check", "SUCCESS", 22),
		}, "passed", nil, false},
	}
	for _, c := range cases {
		got, names, pending := summarize(c.cs)
		if got != c.want || !reflect.DeepEqual(names, c.wantNames) || pending != c.wantPending {
			t.Errorf("%s: got (%s, %q, %v), want (%s, %q, %v)", c.name, got, names, pending, c.want, c.wantNames, c.wantPending)
		}
	}
}
