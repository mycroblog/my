// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"strings"
	"testing"
)

func TestFailures(t *testing.T) {
	log := strings.Join([]string{
		"go\tgo test\t2026-10-22T01:02:03.456Z ok  \tgithub.com/mycroblog/my/internal/app/content\t1.2s",
		"go\tgo test\t2026-10-22T01:02:03.456Z --- FAIL: TestSubmitRequest (0.01s)",
		"go\tgo test\t2026-10-22T01:02:03.456Z     request_test.go:42: got 2 requests, want 1",
		"go\tgo test\t2026-10-22T01:02:03.456Z FAIL",
		"go\tgo test\t2026-10-22T01:02:03.456Z FAIL\tgithub.com/mycroblog/my/internal/app/content\t0.3s",
		"unrelated noise",
		"src/features/a.tsx(3,1): error TS2322: Type 'string' is not assignable to type 'number'.",
		`task: Failed to run task "go:test": exit status 1`,
	}, "\n")
	got := Failures(log, 50)
	joined := strings.Join(got, "\n")
	for _, want := range []string{"--- FAIL: TestSubmitRequest", "request_test.go:42: got 2 requests", "error TS2322", `task: Failed to run task "go:test"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("Failures lacks %q:\n%s", want, joined)
		}
	}
	for _, unwanted := range []string{"unrelated noise", "2026-10-22T", "ok  \t"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("Failures keeps %q:\n%s", unwanted, joined)
		}
	}
}

func TestFailuresMapCheck(t *testing.T) {
	log := "2026-10-22T02:44:27.1Z bin/agentkit map check\n" +
		"2026-10-22T02:44:27.2Z error: app/web/src/features/supporters/: matches strict pattern \"app/web/src/features/*\" but no entry names it\n" +
		"2026-10-22T02:44:27.3Z task: Failed to run task \"agentkit:map-check\": exit status 1\n"
	if got := strings.Join(Failures(log, 10), "\n"); !strings.Contains(got, "features/supporters/: matches strict pattern") {
		t.Errorf("Failures lacks the map check problem:\n%s", got)
	}
}

// TestFailuresTaskOutput checks the local output of go-task: the task name before each line, which must not
// hide a failing test's output, and the commands it echoes, which are not failures.
func TestFailuresTaskOutput(t *testing.T) {
	log := strings.Join([]string{
		"task: [go:test] go test ./...",
		"[go:test] --- FAIL: TestSubmitRequest (0.01s)",
		"[go:test]     request_test.go:42: got 2 requests, want 1",
		"task: [go:vet] echo '    x_test.go:3: not a failure'",
		"[go:vet] ok",
		"\x1b[31mtask: Failed to run task \"check\": task: Failed to run task \"go:test\": exit status 1\x1b[0m",
	}, "\n")
	got := Failures(log, 50)
	joined := strings.Join(got, "\n")
	for _, want := range []string{"--- FAIL: TestSubmitRequest", "    request_test.go:42: got 2 requests", `Failed to run task "go:test"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("Failures lacks %q:\n%s", want, joined)
		}
	}
	for _, unwanted := range []string{"[go:test]", "not a failure", "\x1b["} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("Failures keeps %q:\n%s", unwanted, joined)
		}
	}
}

// TestJobFailuresSetupStep reads the logs of jobs that failed in `apt-get install` because the runner could not
// resolve its mirror.
func TestJobFailuresSetupStep(t *testing.T) {
	for _, name := range []string{"job-apt-unresolved-1.txt", "job-apt-unresolved-2.txt", "job-apt-unresolved-3.txt"} {
		log := fixture(t, name)
		got := JobFailures(log, 40)
		joined := strings.Join(got, "\n")
		for _, want := range []string{
			" http://azure.archive.ubuntu.com/ubuntu noble InRelease",
			"  Could not resolve 'azure.archive.ubuntu.com'",
			"E: Unable to locate package unzip",
			"##[error]Process completed with exit code 100.",
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("%s: JobFailures lacks %q:\n%s", name, want, joined)
			}
		}
		if len(got) > 41 || strings.Contains(joined, "2026-11-11T") || strings.Contains(joined, "Ign:") {
			t.Errorf("%s: JobFailures keeps too much:\n%s", name, joined)
		}
		hosts := Unreachable(log)
		if most(hosts) != "azure.archive.ubuntu.com" || hosts["security.ubuntu.com"] == 0 {
			t.Errorf("%s: Unreachable = %v", name, hosts)
		}
	}
}

func TestJobFailuresFallback(t *testing.T) {
	step := []string{"##[group]Run make thing", "make thing", "##[endgroup]"}
	for i := 1; i <= 20; i++ {
		step = append(step, "2026-10-22T01:02:03.456Z building part "+strings.Repeat("i", i))
	}
	failed := append(append([]string{}, step...), "", "##[error]Process completed with exit code 2.", "##[group]Run post", "cleaning", "##[error]again")
	got := JobFailures(strings.Join(failed, "\n"), 40)
	if len(got) != fallbackLines+1 || got[0] != "building part "+strings.Repeat("i", 6) || got[fallbackLines] != "##[error]Process completed with exit code 2." {
		t.Errorf("want the %d lines before the first annotation and the annotation, got %d:\n%s", fallbackLines, len(got), strings.Join(got, "\n"))
	}

	short := JobFailures("##[group]Run make thing\n##[endgroup]\nnothing to make\n##[error]Process completed with exit code 2.\n", 40)
	if strings.Join(short, "\n") != "nothing to make\n##[error]Process completed with exit code 2." {
		t.Errorf("the tail must stop at the step's command, got %q", short)
	}
	if got := JobFailures(strings.Join(step, "\n"), 40); len(got) != fallbackLines || got[fallbackLines-1] != "building part "+strings.Repeat("i", 20) {
		t.Errorf("a log without an annotation gives its last lines, got %q", got)
	}
	if got := JobFailures("", 40); len(got) != 1 {
		t.Errorf("an empty log gives one line that says so, got %q", got)
	}
	// A line that names the failure needs no fallback.
	if got := JobFailures("noise\n--- FAIL: TestX (0.00s)\n##[error]Process completed with exit code 1.\n", 40); strings.Contains(strings.Join(got, "\n"), "noise") {
		t.Errorf("got %q", got)
	}
}

func TestFailuresSetupErrors(t *testing.T) {
	for _, l := range []string{
		"E: Unable to locate package unzip", "Err:4 http://security.ubuntu.com/ubuntu noble-security InRelease",
		"  Could not resolve 'security.ubuntu.com'", "Temporary failure in name resolution",
		"curl: (6) Could not resolve host: example.test", "fatal: unable to access 'https://example.test/': The requested URL returned error: 503",
		"npm ERR! code ECONNRESET", "error: failed to fetch", "##[error]The operation was canceled.",
	} {
		if got := Failures("ok so far\n"+l+"\ndone\n", 10); len(got) != 1 || got[0] != l {
			t.Errorf("Failures(%q) = %q", l, got)
		}
	}
}

func TestUnreachable(t *testing.T) {
	apt := fixture(t, "job-apt-unresolved-1.txt")
	cases := []struct {
		name, log string
		want      string
	}{
		{"a test ran first", "ok  \texample.test/m\t0.3s\n" + apt, ""},
		{"a failing test", "--- FAIL: TestX (0.00s)\n    x_test.go:3: dial tcp: lookup db.example.test: no such host\n##[error]Process completed with exit code 1.\n", ""},
		{"no annotation", "curl: (6) Could not resolve host: example.test\n", ""},
		{"an earlier step that went on", "##[group]Run a\ncurl: (6) Could not resolve host: example.test\n##[group]Run b\nboom\n##[error]Process completed with exit code 1.\n", ""},
		{"no network error", "##[group]Run a\nE: Unable to locate package unzip\n##[error]Process completed with exit code 100.\n", ""},
		{"curl", "##[group]Run a\ncurl: (6) Could not resolve host: example.test\n##[error]Process completed with exit code 6.\n", "example.test"},
		{"git", "fatal: unable to access 'https://example.test/x/': Failed to connect to example.test port 443 after 130000 ms\n##[error]exit 128\n", "example.test"},
	}
	for _, c := range cases {
		if got := most(Unreachable(c.log)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFailuresCap(t *testing.T) {
	log := strings.Repeat("FAIL\tpkg/x\n--- FAIL: TestY\n", 100)
	if got := Failures(log, 10); len(got) > 12 {
		t.Errorf("Failures returned %d lines, want at most about 10", len(got))
	}
}
