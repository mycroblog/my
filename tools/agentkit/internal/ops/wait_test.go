// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// fixture reads a log of testdata: the lines around the failure of a real job.
func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ghWait answers the calls of Wait for pull request 20: $STATE/pr holds the pull request's JSON ($STATE/pr<k>
// from its k-th read on), and $STATE/job<id> the log of a job.
const ghWait = `
case "$1 $2" in
  "pr view")
    n=$(cat "$STATE/views" 2>/dev/null || echo 0); n=$((n+1)); echo "$n" > "$STATE/views"
    while [ "$n" -gt 0 ] && [ ! -f "$STATE/pr$n" ]; do n=$((n-1)); done
    if [ "$n" -gt 0 ]; then cat "$STATE/pr$n"; else cat "$STATE/pr"; fi ;;
  "api repos/mycroblog/my/actions/jobs/"*)
    id=${2#repos/mycroblog/my/actions/jobs/}; cat "$STATE/job${id%/logs}" ;;
  *) echo "unexpected: $*" >&2; exit 1 ;;
esac`

// checkJSON is a check run of the job in run 1 of CI; an empty conclusion is a running one.
func checkJSON(name, conclusion string, job int) string {
	status := "COMPLETED"
	if conclusion == "" {
		status = "IN_PROGRESS"
	}
	return fmt.Sprintf(`{"__typename":"CheckRun","name":%q,"workflowName":"CI","status":%q,"conclusion":%q,"detailsUrl":"https://github.com/o/r/actions/runs/1/job/%d"}`,
		name, status, conclusion, job)
}

const waitHead = "0123456789abcdef0123456789abcdef01234567"

// wait runs Wait for pull request 20 with a heartbeat at every poll and returns its exit status and its lines.
func wait(t *testing.T, o WaitOptions) (int, []string) {
	t.Helper()
	var out bytes.Buffer
	o.Interval, o.Heartbeat, o.LogDir = time.Millisecond, time.Nanosecond, t.TempDir()
	code := Wait(&out, 20, o)
	return code, strings.Split(strings.TrimSpace(out.String()), "\n")
}

func TestWaitSaysThatItWaits(t *testing.T) {
	state := filepath.Dir(fakeGH(t, ghWait))
	running := checkJSON("lint", "", 81) + "," + checkJSON("changes", "SUCCESS", 80)
	write(t, filepath.Join(state, "pr"), prJSON("OPEN", "MERGEABLE", waitHead, running, false))
	write(t, filepath.Join(state, "pr4"), prJSON("OPEN", "MERGEABLE", waitHead,
		checkJSON("lint", "SUCCESS", 81)+","+checkJSON("changes", "SUCCESS", 80)+","+checkJSON("check", "SUCCESS", 82), false))
	code, lines := wait(t, WaitOptions{Timeout: 5 * time.Second})
	want := []string{
		"waiting for CI of #20 at 012345678: 1 checks pending",
		"still waiting: 1 pending, 1 passed",
		"still waiting: 1 pending, 1 passed",
		"PR #20: CI passed (3 checks, mergeable mergeable)",
	}
	if code != 0 || strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("exit %d, got\n%s\nwant\n%s", code, strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

// TestWaitPassedAtOnce checks that a wait with nothing to wait for prints its result alone.
func TestWaitPassedAtOnce(t *testing.T) {
	state := filepath.Dir(fakeGH(t, ghWait))
	write(t, filepath.Join(state, "pr"), prJSON("OPEN", "MERGEABLE", waitHead, checkJSON("check", "SUCCESS", 82), false))
	if code, lines := wait(t, WaitOptions{Timeout: 5 * time.Second}); code != 0 || len(lines) != 1 || lines[0] != "PR #20: CI passed (1 checks, mergeable mergeable)" {
		t.Errorf("exit %d, got %q", code, lines)
	}
}

func TestWaitTimeout(t *testing.T) {
	cases := []struct {
		name, rollup string
		opts         WaitOptions
		first, last  string
	}{
		{"checks running", checkJSON("lint", "", 81) + "," + checkJSON("server-test", "", 83) + "," + checkJSON("changes", "SUCCESS", 80), WaitOptions{},
			"waiting for CI of #20 at 012345678: 2 checks pending",
			"PR #20: timed out after 30ms, CI has not ended: still pending: lint, server-test"},
		{"no check", "", WaitOptions{},
			"waiting for CI of #20 at 012345678: no check has started yet",
			"PR #20: timed out after 30ms, CI has not ended: no check has started"},
		{"the gate has not reported", checkJSON("lint", "SUCCESS", 81), WaitOptions{},
			"waiting for CI of #20 at 012345678: 0 checks pending",
			"PR #20: timed out after 30ms, CI has not ended: the required check `check` has not reported"},
		{"a failure with --all", checkJSON("lint", "FAILURE", 81) + "," + checkJSON("server-test", "", 83), WaitOptions{All: true},
			"waiting for CI of #20 at 012345678: 1 checks pending",
			"PR #20: timed out after 30ms, CI has not ended: still pending: server-test; failed: lint"},
		{"the push has not arrived", checkJSON("check", "SUCCESS", 82), WaitOptions{Branch: "feat/12-x", Head: "fedcba9876543210"},
			"waiting for CI of #20 at fedcba987: GitHub has not seen the push yet",
			"PR #20: timed out after 30ms, CI has not ended: the pull request was never read at its head"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state := filepath.Dir(fakeGH(t, ghWait))
			write(t, filepath.Join(state, "pr"), prJSON("OPEN", "MERGEABLE", waitHead, c.rollup, false))
			c.opts.Timeout = 30 * time.Millisecond
			code, lines := wait(t, c.opts)
			if code != WaitTimeout || lines[0] != c.first || lines[len(lines)-1] != c.last {
				t.Errorf("exit %d, got\n%s\nwant the first line\n%s\nand the last\n%s", code, strings.Join(lines, "\n"), c.first, c.last)
			}
		})
	}
}

// TestWaitUnreadable checks that a wait that gh never answers still says that it started and how it ended.
func TestWaitUnreadable(t *testing.T) {
	fakeGH(t, `echo "HTTP 502: Bad Gateway" >&2; exit 1`)
	code, lines := wait(t, WaitOptions{Timeout: 30 * time.Millisecond})
	if code != WaitTimeout || !strings.HasPrefix(lines[0], "waiting for CI of #20: gh could not read it yet (") ||
		lines[len(lines)-1] != "PR #20: timed out after 30ms, CI has not ended: the pull request was never read at its head" {
		t.Errorf("exit %d, got\n%s", code, strings.Join(lines, "\n"))
	}
}

func TestWaitFailedBeforeAnyTest(t *testing.T) {
	const claim = "failed before any test ran"
	apt := []string{"job-apt-unresolved-1.txt", "job-apt-unresolved-2.txt", "job-apt-unresolved-3.txt", "job-apt-unresolved-1.txt"}
	rollup := func(jobs int) string {
		cs := []string{checkJSON("check", "FAILURE", 99), checkJSON("changes", "SUCCESS", 80)}
		for i := 0; i < jobs; i++ {
			cs = append(cs, checkJSON(fmt.Sprintf("ui-test (%d/%d)", i+1, jobs), "FAILURE", 81+i))
		}
		return strings.Join(cs, ",")
	}
	setup := func(t *testing.T, jobs int) string {
		state := filepath.Dir(fakeGH(t, ghWait))
		write(t, filepath.Join(state, "pr"), prJSON("OPEN", "MERGEABLE", waitHead, rollup(jobs), false))
		for i := 0; i < jobs; i++ {
			write(t, filepath.Join(state, fmt.Sprintf("job%d", 81+i)), fixture(t, apt[i]))
		}
		return state
	}

	t.Run("every job on the same host", func(t *testing.T) {
		setup(t, 4)
		code, lines := wait(t, WaitOptions{Timeout: 5 * time.Second})
		want := "PR #20: CI failed: check, ui-test (1/4), ui-test (2/4), ui-test (3/4), ui-test (4/4); " +
			"failed before any test ran: the runner could not reach azure.archive.ubuntu.com (rerun: gh run rerun 1 --failed)"
		if code != 1 || lines[0] != want {
			t.Errorf("exit %d, got the first line\n%s\nwant\n%s", code, lines[0], want)
		}
		out := strings.Join(lines, "\n")
		if n := strings.Count(out, "\n== "); n != 3 {
			t.Errorf("want the sections of 3 jobs, got %d:\n%s", n, out)
		}
		for _, want := range []string{"Could not resolve 'azure.archive.ubuntu.com'", "E: Unable to locate package unzip", "##[error]Process completed with exit code 100."} {
			if n := strings.Count(out, want); n < 3 {
				t.Errorf("%q is in %d sections, want each of the 3:\n%s", want, n, out)
			}
		}
		if len(lines) > 3*45 {
			t.Errorf("%d lines: the report is not bounded", len(lines))
		}
	})
	t.Run("one job failed in a test", func(t *testing.T) {
		state := setup(t, 4)
		write(t, filepath.Join(state, "job84"), "--- FAIL: TestThing (0.00s)\n##[error]Process completed with exit code 1.\n")
		code, lines := wait(t, WaitOptions{Timeout: 5 * time.Second})
		if code != 1 || strings.Contains(lines[0], claim) || !strings.HasPrefix(lines[0], "PR #20: CI failed: check, ui-test (1/4)") {
			t.Errorf("exit %d, got the first line %q", code, lines[0])
		}
	})
	t.Run("a log that cannot be read", func(t *testing.T) {
		state := setup(t, 2)
		if err := os.Remove(filepath.Join(state, "job82")); err != nil {
			t.Fatal(err)
		}
		code, lines := wait(t, WaitOptions{Timeout: 5 * time.Second})
		if code != 1 || strings.Contains(lines[0], claim) || !strings.Contains(strings.Join(lines, "\n"), "== ui-test (2/2): could not read the log") {
			t.Errorf("exit %d, got\n%s", code, strings.Join(lines, "\n"))
		}
	})
}

const (
	olderHead = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	newerHead = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// TestWaitEndsWhenTheHeadMoves checks that a push during the wait ends it: the checks of the head it waits for
// are cancelled, and the new head's are not the ones it was asked about.
func TestWaitEndsWhenTheHeadMoves(t *testing.T) {
	running := checkJSON("lint", "", 81)
	failed := checkJSON("lint", "FAILURE", 91) + "," + checkJSON("check", "FAILURE", 92)
	cases := []struct {
		name   string
		opts   WaitOptions
		states []string // the pull request from its k-th read on
		want   []string
	}{
		{"in the branch's worktree", WaitOptions{Branch: "feat/12-x", Head: waitHead},
			[]string{prJSON("OPEN", "MERGEABLE", waitHead, running, false), "", prJSON("OPEN", "MERGEABLE", newerHead, failed, false)},
			[]string{
				"waiting for CI of #20 at 012345678: 1 checks pending",
				"still waiting: 1 pending, 0 passed",
				"PR #20: the head moved from 012345678 to bbbbbbbbb while waiting; run pr wait again for the new head",
			}},
		{"from another checkout", WaitOptions{Branch: "main", Head: "fedcba9876543210"},
			[]string{prJSON("OPEN", "MERGEABLE", waitHead, running, false), prJSON("OPEN", "MERGEABLE", newerHead, failed, false)},
			[]string{
				"waiting for CI of #20 at 012345678: 1 checks pending",
				"PR #20: the head moved from 012345678 to bbbbbbbbb while waiting; run pr wait again for the new head",
			}},
		// GitHub never showed the head the wait started at: a second push went past it.
		{"before GitHub saw the first push", WaitOptions{Branch: "feat/12-x", Head: waitHead},
			[]string{prJSON("OPEN", "MERGEABLE", olderHead, failed, false), "", prJSON("OPEN", "MERGEABLE", newerHead, running, false)},
			[]string{
				"waiting for CI of #20 at 012345678: GitHub has not seen the push yet",
				"still waiting: the checks of the pull request's head could not be read yet",
				"PR #20: the head moved from 012345678 to bbbbbbbbb while waiting; run pr wait again for the new head",
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state := filepath.Dir(fakeGH(t, ghWait))
			for i, s := range c.states {
				if s != "" {
					write(t, filepath.Join(state, fmt.Sprintf("pr%d", i+1)), s)
				}
			}
			c.opts.Timeout = 5 * time.Second
			code, lines := wait(t, c.opts)
			if code != WaitMoved || strings.Join(lines, "\n") != strings.Join(c.want, "\n") {
				t.Errorf("exit %d, got\n%s\nwant\n%s", code, strings.Join(lines, "\n"), strings.Join(c.want, "\n"))
			}
		})
	}
}

// TestWaitForThePush checks that the older head GitHub still shows right after a push is not a move: the wait
// goes on, does not read that head's failure, and reports the pushed head's result.
func TestWaitForThePush(t *testing.T) {
	state := filepath.Dir(fakeGH(t, ghWait))
	write(t, filepath.Join(state, "pr1"), prJSON("OPEN", "MERGEABLE", olderHead, checkJSON("lint", "FAILURE", 91), false))
	write(t, filepath.Join(state, "pr3"), prJSON("OPEN", "MERGEABLE", waitHead, checkJSON("check", "SUCCESS", 82), false))
	code, lines := wait(t, WaitOptions{Branch: "feat/12-x", Head: waitHead, Timeout: 5 * time.Second})
	want := []string{
		"waiting for CI of #20 at 012345678: GitHub has not seen the push yet",
		"still waiting: the checks of the pull request's head could not be read yet",
		"PR #20: CI passed (1 checks, mergeable mergeable)",
	}
	if code != 0 || strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("exit %d, got\n%s\nwant\n%s", code, strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

// landStates writes the pull request that ghLand answers at each read, the last one from then on.
func landStates(t *testing.T, state string, prs ...string) {
	t.Helper()
	for i, p := range prs[:len(prs)-1] {
		write(t, filepath.Join(state, fmt.Sprintf("pr%d", i+1)), p)
	}
	write(t, filepath.Join(state, "pr"), prs[len(prs)-1])
}

// TestLandFollowsTheHead checks that a push while Land waits does not end it or leave it waiting for the old
// head: it says that the head moved, once, and goes on with the new head's checks.
func TestLandFollowsTheHead(t *testing.T) {
	failed := checkJSON("lint", "FAILURE", 91)
	running := checkJSON("lint", "", 81)
	t.Run("auto-merge stays on", func(t *testing.T) {
		main, _, state, log, head := landSetup(t)
		landStates(t, state,
			prJSON("OPEN", "MERGEABLE", head, running, false),
			prJSON("OPEN", "MERGEABLE", head, running, true),
			prJSON("OPEN", "MERGEABLE", newerHead, running, true),
			prJSON("OPEN", "MERGEABLE", newerHead, running, true),
			prJSON("OPEN", "MERGEABLE", newerHead, failed, true))
		code, out := land(t, main)
		if code != 1 || !strings.Contains(out, "PR #20: CI failed: lint") {
			t.Errorf("exit %d, want the failure of the new head:\n%s", code, out)
		}
		if n := strings.Count(out, "PR #20: the head moved to bbbbbbbbb; following it\n"); n != 1 {
			t.Errorf("the move is said %d times, want once:\n%s", n, out)
		}
		if n := strings.Count(readLog(t, log), "pr merge 20"); n != 1 {
			t.Errorf("auto-merge was turned on %d times, want once: the push kept it", n)
		}
	})
	t.Run("the push took it out of the merge queue", func(t *testing.T) {
		main, _, state, log, head := landSetup(t)
		landStates(t, state,
			prJSON("OPEN", "MERGEABLE", head, "", false),
			prJSON("OPEN", "MERGEABLE", head, "", true),
			prJSON("OPEN", "MERGEABLE", newerHead, running, false),
			prJSON("OPEN", "MERGEABLE", newerHead, running, true),
			prJSON("CLOSED", "UNKNOWN", newerHead, "", false))
		code, out := land(t, main)
		want := "PR #20: the head moved to bbbbbbbbb; following it (the push turned auto-merge off: turned on again)\n"
		if code != 1 || strings.Count(out, want) != 1 || !strings.Contains(out, "PR #20 is closed without merging") || strings.Contains(out, "left the merge queue") {
			t.Errorf("exit %d, got\n%s\nwant once\n%s", code, out, want)
		}
		if n := strings.Count(readLog(t, log), "pr merge 20 -R mycroblog/my --auto --merge"); n != 2 {
			t.Errorf("auto-merge was turned on %d times, want 2", n)
		}
	})
	// GitHub still shows the commit before the push: its failure is not the pushed head's, and catching up is
	// not a move.
	t.Run("the push has not arrived", func(t *testing.T) {
		main, _, state, _, head := landSetup(t)
		landStates(t, state,
			prJSON("OPEN", "MERGEABLE", olderHead, failed, true),
			prJSON("OPEN", "MERGEABLE", olderHead, failed, true),
			prJSON("OPEN", "MERGEABLE", olderHead, failed, true),
			prJSON("OPEN", "MERGEABLE", head, running, true),
			prJSON("CLOSED", "UNKNOWN", head, "", false))
		code, out := land(t, main)
		if code != 1 || !strings.Contains(out, "PR #20 is closed without merging") || strings.Contains(out, "moved") || strings.Contains(out, "CI failed") {
			t.Errorf("exit %d, got\n%s", code, out)
		}
	})
}

// ghWatch answers the calls of Watch: $STATE/list<k> holds the open pull requests from the k-th list on, and
// $STATE/pr the pull request once it is not among them.
const ghWatch = `
case "$1 $2" in
  "pr list")
    n=$(cat "$STATE/lists" 2>/dev/null || echo 0); n=$((n+1)); echo "$n" > "$STATE/lists"
    while [ ! -f "$STATE/list$n" ]; do n=$((n-1)); done
    cat "$STATE/list$n" ;;
  "pr view") cat "$STATE/pr" ;;
  "api graphql") echo '{"data":{"repository":{"pullRequests":{"pageInfo":{"hasNextPage":false},"nodes":[]}}}}' ;;
  *) echo "unexpected: $*" >&2; exit 1 ;;
esac`

// TestWatchReportsANewHead checks that a push is an event of Watch, and that the new head's CI is reported
// although the previous head's had the same result.
func TestWatchReportsANewHead(t *testing.T) {
	state := filepath.Dir(fakeGH(t, ghWatch))
	failed := checkJSON("lint", "FAILURE", 91)
	write(t, filepath.Join(state, "list1"), "["+prJSON("OPEN", "MERGEABLE", olderHead, failed, false)+"]")
	write(t, filepath.Join(state, "list3"), "["+prJSON("OPEN", "MERGEABLE", newerHead, failed, false)+"]")
	write(t, filepath.Join(state, "list5"), "[]")
	write(t, filepath.Join(state, "pr"), prJSON("MERGED", "UNKNOWN", newerHead, "", false))
	var out bytes.Buffer
	if err := Watch(&out, []int{20}, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	var events []string
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		_, e, _ := strings.Cut(l, " ") // the time of day
		events = append(events, e)
	}
	want := []string{
		"#20 ci-failed lint — feat: x",
		"#20 head-moved bbbbbbbbb — feat: x",
		"#20 ci-failed lint — feat: x",
		"#20 merged — feat: x",
	}
	if strings.Join(events, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(events, "\n"), strings.Join(want, "\n"))
	}
}

func TestLandSaysThatItWaits(t *testing.T) {
	main, _, state, _, head := landSetup(t)
	write(t, filepath.Join(state, "pr"), prJSON("OPEN", "MERGEABLE", head, checkJSON("lint", "", 81)+","+checkJSON("changes", "SUCCESS", 80), true))
	r, err := repo.Open(main)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := Land(&out, r, 20, LandOptions{Timeout: 30 * time.Millisecond, Interval: time.Millisecond, Heartbeat: time.Nanosecond, LogDir: t.TempDir()})
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	want := []string{
		"PR #20: auto-merge on; waiting for its CI and the merge queue",
		"waiting for CI of #20 at " + head[:9] + ": 1 checks pending",
		"still waiting: 1 pending, 1 passed",
	}
	last := "PR #20: timed out after 30ms, not merged: still pending: lint; run pr land again to keep waiting"
	if code != LandTimeout || len(lines) < 4 || strings.Join(lines[:3], "\n") != strings.Join(want, "\n") || lines[len(lines)-1] != last {
		t.Errorf("exit %d, got\n%s\nwant first\n%s\nand last\n%s", code, out.String(), strings.Join(want, "\n"), last)
	}
}
