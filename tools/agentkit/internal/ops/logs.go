// SPDX-License-Identifier: AGPL-3.0-only

// Package ops wraps the workflow steps of /work-on-issue — claiming and blocking issues, opening pull
// requests, waiting for CI, running the checks and the generators — so that each prints what the agent needs
// to act on and nothing else.
package ops

import (
	"regexp"
	"strings"
)

// errorPatterns match the lines of Go, golangci-lint, TypeScript, Vitest, Playwright, markdownlint, shellcheck,
// Atlas and go-task output that say what failed, and those of a step that failed before any of them ran: apt,
// curl, git, npm, a name that does not resolve, and the error annotation of GitHub Actions.
var errorPatterns = regexp.MustCompile(strings.Join([]string{
	`^\s*--- FAIL`,
	`^FAIL\b`,
	`^panic:`,
	`\.go:\d+(:\d+)?: `,
	`\.(ts|tsx|js|mjs):\d+(:\d+)?`,
	`\berror TS\d+`,
	`^\s*(×|✗|✘|FAIL)\s`,
	`^\s*(Error|AssertionError|TypeError|ReferenceError):`,
	`\bMD\d{3}\b`,
	`\bSC\d{4}\b`,
	`^task: Failed to run task `,
	`\bFAILED after\b`,
	`(?i)^\s*error:`,
	`^\s*Expected:|^\s*Received:|^\s*expected .* (to|but)`,
	`\bis stale\b|\bout of date\b|\bnot tidy\b`,
	`^diff --git `,
	`^E: `,
	`^\s*Err:\d`,
	`Could not resolve|Temporary failure`,
	`\bcurl: \(\d+\)`,
	`^fatal:`,
	`\bnpm ERR!`,
	`##\[error\]`,
}, "|"))

// followers are the matches whose next lines stay with them: a failing test's output, a panic's stack.
var followers = regexp.MustCompile(`^\s*--- FAIL|^panic:|^\s*(Error|AssertionError):|\berror TS\d+|^\s*(×|✗|✘)\s`)

// timestamp strips the prefix GitHub Actions puts on every log line: "<job>\t<step>\t2026-10-22T01:02:03.456Z ".
var timestamp = regexp.MustCompile(`^(?:[^\t]*\t){0,2}\d{4}-\d\d-\d\dT[\d:.]+Z ?`)

// color strips terminal colors, which go-task prints in CI.
var color = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// taskPrefix strips the name go-task puts before each line of a task's output ("[go:test] ").
var taskPrefix = regexp.MustCompile(`^\[[\w:.-]+\] `)

// plain returns a log line without its GitHub Actions timestamp, colors, go-task prefix, and trailing CR.
func plain(l string) string {
	l = color.ReplaceAllString(timestamp.ReplaceAllString(l, ""), "")
	return strings.TrimRight(taskPrefix.ReplaceAllString(l, ""), "\r")
}

// echoed matches the commands go-task prints before running them ("task: [go:test] go test ./..."), which
// say nothing about what failed.
var echoed = regexp.MustCompile(`^task: \[[\w:.-]+\] `)

// gapMark stands for the lines Failures leaves out between two it keeps.
const gapMark = "  ⋮"

// Failures returns the lines of log that say what failed, each with a little context, without duplicates,
// at most limit lines.
func Failures(log string, limit int) []string {
	lines := strings.Split(log, "\n")
	for i, l := range lines {
		lines[i] = plain(l)
	}
	keep := make([]bool, len(lines))
	for i, l := range lines {
		if echoed.MatchString(l) || !errorPatterns.MatchString(l) {
			continue
		}
		keep[i] = true
		if !followers.MatchString(l) {
			continue
		}
		// A panic keeps its stack; anything else keeps the indented lines under it (a test's output).
		panicking := strings.HasPrefix(l, "panic:")
		for j := i + 1; j < len(lines) && j <= i+12; j++ {
			if !panicking && !strings.HasPrefix(lines[j], " ") && !strings.HasPrefix(lines[j], "\t") {
				break
			}
			keep[j] = true
		}
	}
	var out []string
	seen := map[string]bool{}
	gap := false
	for i, l := range lines {
		if !keep[i] {
			gap = true
			continue
		}
		t := strings.TrimSpace(l)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		if gap && len(out) > 0 {
			out = append(out, gapMark)
		}
		gap = false
		if len(out) >= limit {
			out = append(out, "  … more failures; see the full log")
			break
		}
		out = append(out, l)
	}
	return out
}

// fallbackLines is how many lines before the error JobFailures shows when no pattern names the failure.
const fallbackLines = 15

// annotation is what GitHub Actions prints when a step fails.
const annotation = "##[error]"

// JobFailures returns the lines of a GitHub Actions job's log that say what failed, at most about limit lines,
// and never none: when no pattern matches anything but the error annotation, it returns the last lines the
// failed step printed before its first annotation, or the end of a log that has none.
func JobFailures(log string, limit int) []string {
	out := Failures(log, limit)
	for _, l := range out {
		if !strings.Contains(l, annotation) && l != gapMark {
			return out
		}
	}
	lines := strings.Split(log, "\n")
	end := len(lines)
	for i, l := range lines {
		lines[i] = plain(l)
		if end == len(lines) && strings.Contains(lines[i], annotation) {
			end = i
		}
	}
	var tail []string
	for i := end - 1; i >= 0 && len(tail) < fallbackLines; i-- {
		// The step's own output starts after the group that shows its command.
		if strings.HasPrefix(lines[i], "##[endgroup]") || strings.HasPrefix(lines[i], "##[group]") {
			break
		}
		if strings.TrimSpace(lines[i]) != "" {
			tail = append([]string{lines[i]}, tail...)
		}
	}
	if end < len(lines) {
		tail = append(tail, lines[end])
	}
	if len(tail) == 0 {
		return []string{"  (the log is empty)"}
	}
	return tail
}

// reachErrors match the lines that say which host a step could not reach: apt, curl, git, Go and Node.
var reachErrors = regexp.MustCompile(strings.Join([]string{
	`Could not resolve '([^']+)'`,
	`Could not resolve host: ([\w.-]+)`,
	`Temporary failure resolving '([^']+)'`,
	`Could not connect to ([\w.-]+):\d+`,
	`Failed to connect to ([\w.-]+) port`,
	`dial tcp: lookup ([\w.-]+)`,
	`getaddrinfo (?:EAI_AGAIN|ENOTFOUND) ([\w.-]+)`,
}, "|"))

// testOutput matches what go test, Vitest and Playwright print once a test runs.
var testOutput = regexp.MustCompile(`^(=== RUN|\s*--- (FAIL|PASS|SKIP)|ok\s+\S+\s+[\d.]+s|FAIL\s|PASS$)|^\s*(✓|×|✗|✘)\s|^\s*Test Files\s|\bRunning \d+ tests? using\b`)

// Unreachable returns the hosts that the failed step of a GitHub Actions job's log could not reach, each with
// the number of lines that name it, when the log shows that the step failed on them before any test ran: the
// lines between the step's command and its first error annotation name a host, and nothing before the
// annotation is the output of a test. It returns nothing otherwise.
func Unreachable(log string) map[string]int {
	hosts := map[string]int{}
	for _, l := range strings.Split(log, "\n") {
		l = plain(l)
		switch {
		case strings.Contains(l, annotation):
			return hosts
		case testOutput.MatchString(l):
			return nil
		case strings.HasPrefix(l, "##[group]Run "):
			// A step that printed such a line and went on did not fail on it.
			hosts = map[string]int{}
		}
		if m := reachErrors.FindStringSubmatch(l); m != nil {
			for _, h := range m[1:] {
				if h != "" {
					hosts[strings.TrimRight(h, ".")]++
				}
			}
		}
	}
	return nil
}
