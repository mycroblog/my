// SPDX-License-Identifier: AGPL-3.0-only

package kb

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mycroblog/my/tools/agentkit/internal/repo"
	"github.com/mycroblog/my/tools/agentkit/internal/store"
)

// fixture builds a small repository with a document, an operation, a handler, a service, an adapter, a
// query, and a table, and opens its knowledge graph.
func fixture(t *testing.T) *KB {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"docs/architecture/data-model/README.md":                 "# データモデル\n\n## 節の一覧\n\n- [4. テーブル](04-tables/README.md)\n- [5. 状態](05-states-and-policies.md)\n",
		"docs/architecture/data-model/04-tables/README.md":       "## 4. テーブル\n\n- [4.4 content](04-content.md)\n\n### 4.0 所有モジュール\n\nモジュールごとの表。\n",
		"docs/architecture/data-model/04-tables/04-content.md":   "### 4.4 content\n\n`custom_emoji_requests` は申請を記録する。\n\n- 申請は承認されるまで出ない。\n- 投票は別の話。\n",
		"docs/architecture/data-model/05-states-and-policies.md": "## 5. 状態\n\n別の節。\n",
		"docs/architecture/web-client/README.md":                 "# Web クライアントの設計規約\n\n## 節の一覧\n\n## 1. React の基本\n\nフックの規則。\n",
		"docs/architecture/web-client/14-admin.md":               "## 14. 管理画面\n\n### 14.2 区分\n\nスタッフの画面は 2 つの区分に分ける。\n",
		"docs/adr/0070-split.md":                                 "# ADR-0070: 分割\n\n- Status: Accepted\n\n## Decision Drivers\n\n速さ。\n\n## Decision（決定）\n\n申請を別のリソースにする。\n",
		"api/spec/openapi.yaml":                                  "paths:\n  /custom_emoji_requests:\n    $ref: ./paths/custom_emoji_requests.yaml\n",
		"api/spec/paths/custom_emoji_requests.yaml":              "post:\n  operationId: createCustomEmojiRequest\n  summary: Request a custom emoji\n",
		"internal/interface/clientapi/requests.go":               "package clientapi\n\n// CreateCustomEmojiRequest serves POST /custom_emoji_requests.\nfunc (s *Server) CreateCustomEmojiRequest() error {\n\treturn s.emoji.SubmitRequest()\n}\n",
		"internal/app/content/requests.go":                       "package content\n\n// SubmitRequest files a request.\nfunc (s *Service) SubmitRequest() error {\n\ts.now = s.clock.Now()\n\treturn s.repo.InsertCustomEmojiRequest()\n}\n",
		"internal/adapter/db/sqlite/requests.go":                 "package sqlite\n\nfunc (r *Repo) InsertCustomEmojiRequest() error {\n\treturn r.q.InsertCustomEmojiRequest()\n}\n",
		"internal/adapter/system/clock.go":                       "package system\n\nfunc (Clock) Now() int { return 0 }\n",
		"queries/sqlite/requests.sql":                            "-- name: InsertCustomEmojiRequest :exec\nINSERT INTO custom_emoji_requests (id) VALUES (?);\n",
		"schema/sqlite/04-content.sql":                           "CREATE TABLE custom_emoji_requests (\n  id TEXT PRIMARY KEY\n);\n",
	}
	for p, c := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-qm", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	r, err := repo.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(r)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return &KB{S: s}
}

func TestResolveAndShow(t *testing.T) {
	k := fixture(t)
	cases := map[string]string{
		"ADR-0070#decision":                "申請を別のリソースにする",
		"data-model§4.4":                   "申請は承認されるまで出ない",
		"table:custom_emoji_requests":      "CREATE TABLE custom_emoji_requests",
		"custom_emoji_requests":            "CREATE TABLE custom_emoji_requests",
		"op:createCustomEmojiRequest":      "operationId: createCustomEmojiRequest",
		"SubmitRequest":                    "func (s *Service) SubmitRequest",
		"schema/sqlite/04-content.sql:2-2": "id TEXT PRIMARY KEY",
		// A document split into one file per section: the section number, an anchor, or the name finds the file.
		"web-client§14.2":                   "スタッフの画面は 2 つの区分に分ける",
		"web-client.md§1":                   "フックの規則",
		"web-client#14-管理画面":                "スタッフの画面は 2 つの区分に分ける",
		"docs/architecture/data-model#5-状態": "別の節",
		"data-model§5":                      "別の節",
		"web-client":                        "# Web クライアントの設計規約",
		"data-model.md":                     "- [4. テーブル](04-tables/README.md)",
		"docs/architecture/data-model/04-tables/04-content.md": "### 4.4 content",
		"data-model§4":   "モジュールごとの表",
		"data-model§4.0": "モジュールごとの表",
	}
	for ref, want := range cases {
		var b bytes.Buffer
		if err := k.Show(&b, ref, 12000); err != nil {
			t.Errorf("Show(%q): %v", ref, err)
			continue
		}
		if !strings.Contains(b.String(), want) {
			t.Errorf("Show(%q) = %q, want it to contain %q", ref, b.String(), want)
		}
	}
	var b bytes.Buffer
	if err := k.Show(&b, "data-model§4.4", 5); err != nil || !strings.Contains(b.String(), "too big to print") {
		t.Errorf("a section over the limit should print its outline: %q %v", b.String(), err)
	}
}

func TestFindAndFocus(t *testing.T) {
	k := fixture(t)
	var b bytes.Buffer
	if err := k.Find(&b, "申請 承認", nil, "", 5); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "data-model/04-tables/04-content.md") {
		t.Errorf("Find = %q", b.String())
	}
	ns, err := k.Resolve("data-model§4.4")
	if err != nil || len(ns) != 1 {
		t.Fatalf("Resolve: %v %v", ns, err)
	}
	ex, kept := k.Focus(ns[0], []string{"承認"}, 1000)
	if kept != 1 || !strings.Contains(ex, "承認されるまで") || strings.Contains(ex, "投票は別の話") {
		t.Errorf("Focus = %q (kept %d)", ex, kept)
	}
}

func TestTrace(t *testing.T) {
	k := fixture(t)
	var b bytes.Buffer
	if err := k.Trace(&b, "op:createCustomEmojiRequest", 6); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"POST /custom_emoji_requests", "Server.CreateCustomEmojiRequest", "Service.SubmitRequest", "Repo.InsertCustomEmojiRequest", "query:InsertCustomEmojiRequest", "table:custom_emoji_requests"} {
		if !strings.Contains(out, want) {
			t.Errorf("trace lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Clock.Now") {
		t.Errorf("trace follows the generic Now:\n%s", out)
	}
}

func TestRefs(t *testing.T) {
	k := fixture(t)
	var b bytes.Buffer
	if err := k.Refs(&b, "custom_emoji_requests", 20); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "query:InsertCustomEmojiRequest") || !strings.Contains(b.String(), "data-model/04-tables/04-content.md") {
		t.Errorf("Refs = %s", b.String())
	}
}

func TestShowRefusesPathsOutsideTheRoot(t *testing.T) {
	k := fixture(t)
	root := k.S.Repo.Root
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("TOPSECRET\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.txt")); err != nil {
		t.Skip("symlinks are not available:", err)
	}
	rel, err := filepath.Rel(root, outside)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{outside, rel, rel + ":1-1", "link.txt", "link.txt:1-1"} {
		var b bytes.Buffer
		if err := k.Show(&b, ref, 12000); err == nil || strings.Contains(b.String(), "TOPSECRET") {
			t.Errorf("Show(%q) = %q, %v; want an error and no content", ref, b.String(), err)
		}
	}
}
