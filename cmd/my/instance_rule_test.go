// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"regexp"
	"strings"
	"testing"
)

var ruleOutput = regexp.MustCompile(`^(created|revised|moved|retired) rule ([0-9a-f-]{36}) at position (\d+) \(revision [0-9a-f-]{36}\)\n$`)

// mustRule runs `my instance rule` and returns the rule ID and position that it prints.
func mustRule(t *testing.T, args ...string) (id, position string) {
	t.Helper()
	out := mustInstance(t, "", append([]string{"rule"}, args...)...)
	m := ruleOutput.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("my instance rule %q stdout = %q", args, out)
	}
	return m[2], m[3]
}

func TestInstanceRules(t *testing.T) {
	newAdminEnv(t)
	kind, pos := mustRule(t, "create", "-hint", "No harassment.", "Be kind")
	if pos != "1" {
		t.Errorf("first rule position = %s, want 1", pos)
	}
	spam, _ := mustRule(t, "create", "No spam")
	if id, _ := mustRule(t, "edit", kind, "親切に"); id != kind {
		t.Errorf("edit changed the rule ID to %s, want %s", id, kind)
	}
	if _, pos := mustRule(t, "move", spam, "1"); pos != "1" {
		t.Errorf("moved position = %s, want 1", pos)
	}

	list := mustInstance(t, "", "rule", "list")
	lines := strings.Split(strings.TrimSpace(list), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[1], spam) || !strings.HasPrefix(lines[2], kind) ||
		!strings.Contains(lines[2], `"親切に"`) || !strings.Contains(lines[2], `"No harassment."`) {
		t.Errorf("list = %q, want the spam rule, then the kind rule with its new text and its hint kept", list)
	}

	mustRule(t, "retire", kind)
	if list := mustInstance(t, "", "rule", "list"); strings.Contains(list, kind) {
		t.Errorf("list after retire = %q, want the retired rule gone", list)
	}
	if list := mustInstance(t, "", "rule", "list", "-all"); !strings.Contains(list, kind) || !strings.Contains(list, "retired") {
		t.Errorf("list -all = %q, want the retired rule", list)
	}

	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"rule"}, 2, "Usage:"},
		{[]string{"rule", "frobnicate"}, 2, "unknown subcommand"},
		{[]string{"rule", "move", spam, "0"}, 2, "not a number from 1"},
		{[]string{"rule", "create", "two\nlines"}, 1, "line break"},
		{[]string{"rule", "edit", kind, "x"}, 1, "retired"},
		{[]string{"rule", "retire", "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f"}, 1, "not found"},
		{[]string{"rule", "edit", spam, ""}, 1, "is blank"},
		{[]string{"rule", "create", "-locale", "ja", "x"}, 2, "flag provided but not defined: -locale"},
	} {
		code, _, stderr := runInstanceCmd(t, "", tc.args...)
		if code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("my instance %q = %d, %q; want %d and %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}
