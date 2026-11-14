// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInstanceTheme(t *testing.T) {
	newAdminEnv(t)
	if out := mustInstance(t, "", "theme", "get"); !strings.Contains(out, `"tokens": []`) {
		t.Errorf("get without a theme = %q", out)
	}
	out := mustInstance(t, `{"tokens":[{"token":"radius-control","value":"2px"},{"token":"color-surface","light":"#FFFDF5","dark":"#0a0a0a"}]}`,
		"theme", "set")
	var got themeDocument
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("set output %q: %v", out, err)
	}
	want := []themeTokenDocument{{Token: "color-surface", Light: "#fffdf5", Dark: "#0a0a0a"}, {Token: "radius-control", Value: "2px"}}
	if len(got.Tokens) != 2 || got.Tokens[0] != want[0] || got.Tokens[1] != want[1] || got.Revision != "" {
		t.Errorf("set = %+v, want %+v", got, want)
	}
	if out := mustInstance(t, "", "theme", "get"); !strings.Contains(out, `"#fffdf5"`) {
		t.Errorf("get = %q", out)
	}
	// The draft is not in effect until it is published.
	if out := mustInstance(t, "", "theme", "get", "-published"); strings.Contains(out, `"#fffdf5"`) {
		t.Errorf("get -published before publish = %q", out)
	}
	out = mustInstance(t, "", "theme", "publish")
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.Revision == "" || len(got.Tokens) != 2 {
		t.Errorf("publish = %q, %v", out, err)
	}
	// The printed set, revision included, reads back as a draft.
	mustInstance(t, out, "theme", "set")
	if out := mustInstance(t, "", "theme", "get", "-published"); !strings.Contains(out, got.Revision) {
		t.Errorf("get -published = %q, want revision %s", out, got.Revision)
	}
	mustInstance(t, `{"tokens":[]}`, "theme", "set")
	mustInstance(t, "", "theme", "publish")
	if out := mustInstance(t, "", "theme", "restore"); !strings.Contains(out, `"#fffdf5"`) {
		t.Errorf("restore = %q", out)
	}

	for _, tc := range []struct {
		stdin string
		args  []string
		code  int
		want  string
	}{
		{"", []string{"theme"}, 2, "Usage:"},
		{"", []string{"theme", "frobnicate"}, 2, "unknown subcommand"},
		{"not json", []string{"theme", "set"}, 1, "not JSON"},
		{`{"tokens":[],"custom_css":""}`, []string{"theme", "set"}, 1, "not JSON"},
		{`{"tokens":[]} {"tokens":[]}`, []string{"theme", "set"}, 1, "exactly one JSON value"},
		{`{"tokens":[]} garbage`, []string{"theme", "set"}, 1, "exactly one JSON value"},
		{`{"tokens":[{"token":"color-text","light":"#ffffff","dark":"#ffffff"}]}`, []string{"theme", "set"}, 1, "color-text on color-surface"},
	} {
		code, _, stderr := runInstanceCmd(t, tc.stdin, tc.args...)
		if code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("my instance %q = %d, %q; want %d and %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}

func TestInstanceThemeWithoutADraft(t *testing.T) {
	newAdminEnv(t)
	for _, tc := range []struct {
		sub, want string
	}{
		{"publish", "no draft is saved"},
		{"restore", "no other set was published"},
	} {
		code, _, stderr := runInstanceCmd(t, "", "theme", tc.sub)
		if code != 1 || !strings.Contains(stderr, tc.want) {
			t.Errorf("my instance theme %s = %d, %q; want 1 and %q", tc.sub, code, stderr, tc.want)
		}
	}
}
