// SPDX-License-Identifier: AGPL-3.0-only

package adr

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func write(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, Dir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestList(t *testing.T) {
	root := t.TempDir()
	write(t, root, "README.md", "# Architecture Decision Records\n")
	write(t, root, "template.md", "# ADR-NNNN: タイトル\n\n- Status: Proposed\n")
	write(t, root, "0002-b.md", "# ADR-0002: 二つ目\n\n- Status: Superseded by ADR-0003\n- Date: 2026-10-01\n")
	write(t, root, "0001-a.md", "# ADR-0001: 一つ目\n\n- Status: Accepted（§2 を ADR-0003 が置き換える）\n")
	write(t, root, "0003-c.md", "# ADR-0003: 三つ目\n\n- Status: Proposed <!-- Proposed | Accepted -->\n")

	got, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []ADR{
		{Number: "0001", Title: "一つ目", Status: "Accepted", Path: "docs/adr/0001-a.md"},
		{Number: "0002", Title: "二つ目", Status: "Superseded", Path: "docs/adr/0002-b.md"},
		{Number: "0003", Title: "三つ目", Status: "Proposed", Path: "docs/adr/0003-c.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %+v, want %+v", got, want)
	}
	if f := Filter(got, "proposed"); len(f) != 1 || f[0].Number != "0003" {
		t.Errorf("Filter(proposed) = %+v", f)
	}
	if f := Filter(got, ""); len(f) != 3 {
		t.Errorf("Filter(\"\") = %+v", f)
	}
}

func TestListRejectsMalformed(t *testing.T) {
	root := t.TempDir()
	write(t, root, "0001-a.md", "no heading\n")
	if _, err := List(root); err == nil {
		t.Fatal("List accepted an ADR without a title and status")
	}
}

func TestAliases(t *testing.T) {
	root := t.TempDir()
	if as, err := LoadAliases(root); err != nil || as != nil {
		t.Fatalf("no file: %v %v", as, err)
	}
	write(t, root, "aliases.yaml", "# nothing yet\n")
	if as, err := LoadAliases(root); err != nil || len(as) != 0 {
		t.Fatalf("empty file: %v %v", as, err)
	}
	write(t, root, "aliases.yaml", `aliases:
  ADR-0099: {to: ADR-0074, offset: 10, sections: {"2": "4"}, anchors: {usage: usage-payjp}}
  ADR-0124: {to: ADR-0070, section: "4"}
  ADR-0001: {to: ADR-0002}
`)
	as, err := LoadAliases(root)
	if err != nil {
		t.Fatal(err)
	}
	for ref, want := range map[string]string{
		"ADR-0099":          "ADR-0074",
		"ADR-0099§5":        "ADR-0074§15",
		"ADR-0099 §5.2":     "ADR-0074§15.2",
		"ADR-0099§2.1":      "ADR-0074§4.1",
		"ADR-0099#usage":    "ADR-0074#usage-payjp",
		"ADR-0099#decision": "ADR-0074#decision",
		"ADR-0099#nope":     "ADR-0074",
		"ADR-0124§3":        "ADR-0070§4",
		"ADR-0001§3":        "ADR-0002",
		"ADR-0074§5":        "ADR-0074§5",
		"data-model§4.4":    "data-model§4.4",
	} {
		if got := as.Ref(ref); got != want {
			t.Errorf("Ref(%q) = %q, want %q", ref, got, want)
		}
	}
	got := as.CurrentAll([]string{"ADR-0099", "ADR-0074", "ADR-0003", "ADR-0124", "ADR-0070"})
	if want := []string{"ADR-0074", "ADR-0003", "ADR-0070"}; !reflect.DeepEqual(got, want) {
		t.Errorf("CurrentAll = %v, want %v", got, want)
	}

	for name, body := range map[string]string{
		"chain":   "aliases:\n  ADR-0001: {to: ADR-0002}\n  ADR-0002: {to: ADR-0003}\n",
		"bad id":  "aliases:\n  ADR-1: {to: ADR-0002}\n",
		"unknown": "aliases:\n  ADR-0001: {to: ADR-0002, nope: 1}\n",
	} {
		write(t, root, "aliases.yaml", body)
		if _, err := LoadAliases(root); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
