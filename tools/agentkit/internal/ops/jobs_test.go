// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ghJobs answers the calls of Jobs: $STATE/pr holds pull request 20, $STATE/run<id> a run and $STATE/jobs<id>
// its jobs, one JSON object each as `--jq .jobs[]` prints them.
const ghJobs = `
case "$1 $2" in
  "pr view") cat "$STATE/pr" ;;
  "api --paginate") id=${3#repos/mycroblog/my/actions/runs/}; cat "$STATE/jobs${id%%/*}" ;;
  "api repos/mycroblog/my/actions/runs/"*) cat "$STATE/run${2#repos/mycroblog/my/actions/runs/}" ;;
  *) echo "unexpected: $*" >&2; exit 1 ;;
esac`

// jobJSON is a job that started at 10:00:00 and ran for the given seconds; a conclusion of "" still runs.
func jobJSON(name, conclusion string, seconds int, steps ...string) string {
	status, end := "completed", `"`+jobTime(seconds)+`"`
	if conclusion == "" {
		status, end = "in_progress", "null"
	}
	return `{"name":"` + name + `","status":"` + status + `","conclusion":"` + conclusion + `","started_at":"` + jobTime(0) +
		`","completed_at":` + end + `,"steps":[` + strings.Join(steps, ",") + `]}` + "\n"
}

func stepJSON(name string, from, to int) string {
	return `{"name":"` + name + `","status":"completed","conclusion":"success","started_at":"` + jobTime(from) +
		`","completed_at":"` + jobTime(to) + `"}`
}

func jobTime(seconds int) string {
	return time.Date(2026, 1, 2, 10, 0, seconds, 0, time.UTC).Format(time.RFC3339)
}

func TestJobs(t *testing.T) {
	state := filepath.Dir(fakeGH(t, ghJobs))
	// Run 1 of CI was replaced by run 2; the other workflow has run 3.
	rollup := checkJSON("old", "CANCELLED", 70) + "," +
		strings.ReplaceAll(checkJSON("check", "FAILURE", 82), "/runs/1/", "/runs/2/") + "," +
		strings.ReplaceAll(strings.ReplaceAll(checkJSON("links", "", 90), "/runs/1/", "/runs/3/"), `"CI"`, `"Docs"`)
	write(t, filepath.Join(state, "pr"), prJSON("OPEN", "MERGEABLE", waitHead, rollup, false))
	write(t, filepath.Join(state, "run2"), `{"name":"CI","status":"completed","conclusion":"failure","run_started_at":"`+
		jobTime(0)+`","updated_at":"`+jobTime(700)+`"}`)
	write(t, filepath.Join(state, "jobs2"),
		jobJSON("lint", "success", 45)+
			jobJSON("contracts", "skipped", 0)+
			jobJSON("server-test", "failure", 400, stepJSON("Set up job", 0, 5), stepJSON("go test", 100, 390),
				stepJSON("Restore the cache", 5, 60), stepJSON("Install", 60, 100))+
			jobJSON("tofu", "success", 180, stepJSON("tofu test", 0, 170))+
			jobJSON("e2e", "skipped", 0)+
			jobJSON("check", "failure", 3))
	write(t, filepath.Join(state, "run3"), `{"name":"Docs","status":"in_progress","conclusion":null,"run_started_at":"`+
		jobTime(0)+`","updated_at":"`+jobTime(10)+`"}`)
	write(t, filepath.Join(state, "jobs3"), jobJSON("links", "", 0)+
		`{"name":"deploy","status":"queued","conclusion":null,"started_at":null,"completed_at":null,"steps":[]}`+"\n")

	now := time.Date(2026, 1, 2, 10, 4, 0, 0, time.UTC)
	var out bytes.Buffer
	if err := Jobs(&out, 20, JobsOptions{Limit: 3, Now: now}); err != nil {
		t.Fatal(err)
	}
	// A job of exactly the threshold lists no steps, and the jobs past the limit are counted.
	want := strings.Join([]string{
		"CI run 2 (failed): 11m40s wall, 4 jobs ran, 2 skipped",
		"    6m40s  failed     server-test",
		"             4m50s  go test",
		"               55s  Restore the cache",
		"               40s  Install",
		"     3m0s  passed     tofu",
		"      45s  passed     lint",
		"  … 1 faster jobs (-n 4)",
		"Docs run 3 (running): 4m0s wall, 2 jobs ran, 0 skipped",
		"     4m0s  running    links",
		"       0s  queued     deploy",
		"",
	}, "\n")
	if out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", out.String(), want)
	}

	// --run reads that run alone, without the pull request.
	if err := os.Remove(filepath.Join(state, "pr")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Jobs(&out, 20, JobsOptions{Run: 3, Now: now}); err != nil || !strings.HasPrefix(out.String(), "Docs run 3 (running)") {
		t.Errorf("--run 3: %v\n%s", err, out.String())
	}
}

func TestJobsWithoutARun(t *testing.T) {
	state := filepath.Dir(fakeGH(t, ghJobs))
	write(t, filepath.Join(state, "pr"), prJSON("OPEN", "MERGEABLE", waitHead, "", false))
	var out bytes.Buffer
	if err := Jobs(&out, 20, JobsOptions{}); err == nil || !strings.Contains(err.Error(), "no workflow run on its head 012345678") {
		t.Errorf("got %v", err)
	}
}
