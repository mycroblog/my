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
	out := mustInstance(t, `{"tokens":[{"token":"radius-md","value":"2px"},{"token":"color-surface","light":"#FFFDF5","dark":"#0a0a0a"}],"custom_css":"a{}"}`,
		"theme", "set")
	var got themeDocument
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("set output %q: %v", out, err)
	}
	want := themeDocument{
		Tokens:    []themeTokenDocument{{Token: "color-surface", Light: "#fffdf5", Dark: "#0a0a0a"}, {Token: "radius-md", Value: "2px"}},
		CustomCSS: "a{}",
	}
	if len(got.Tokens) != 2 || got.Tokens[0] != want.Tokens[0] || got.Tokens[1] != want.Tokens[1] || got.CustomCSS != want.CustomCSS {
		t.Errorf("set = %+v, want %+v", got, want)
	}
	if out := mustInstance(t, "", "theme", "get"); !strings.Contains(out, `"#fffdf5"`) {
		t.Errorf("get = %q", out)
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
		{`{"tokens":[]} {"tokens":[]}`, []string{"theme", "set"}, 1, "exactly one JSON value"},
		{`{"tokens":[]} garbage`, []string{"theme", "set"}, 1, "exactly one JSON value"},
		{`{"tokens":[{"token":"color-fg","light":"#ffffff","dark":"#ffffff"}],"custom_css":""}`, []string{"theme", "set"}, 1, "color-fg on color-surface"},
	} {
		code, _, stderr := runInstanceCmd(t, tc.stdin, tc.args...)
		if code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("my instance %q = %d, %q; want %d and %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}
