// SPDX-License-Identifier: AGPL-3.0-only

package extract

import (
	"strings"
	"testing"
)

func ids(r Result) map[string]Node {
	out := map[string]Node{}
	for _, n := range r.Nodes {
		out[n.ID] = n
	}
	return out
}

func hasEdge(r Result, src, dst string) bool {
	for _, e := range r.Edges {
		if e.Src == src && e.Dst == dst {
			return true
		}
	}
	return false
}

func TestMarkdownADR(t *testing.T) {
	src := strings.Join([]string{
		"# ADR-0070: 権限の分割",
		"",
		"- Status: Accepted（§1 を除く）",
		"",
		"## Decision（決定）",
		"",
		"`custom_emojis` を FR-915 のとおり変える。[概要](../architecture/overview.md#6-リポジトリ構成)",
		"",
		"### 1. 権限",
		"",
		"```go",
		"# not a heading",
		"```",
		"",
		"## Consequences",
		"",
		"#925 を見る。",
	}, "\n")
	r := Markdown("docs/adr/0070-permission-split.md", []byte(src))
	n := ids(r)
	adr, ok := n["ADR-0070"]
	if !ok || adr.Meta != "Accepted" || adr.Start != 1 || adr.End != 17 {
		t.Errorf("ADR node = %+v", adr)
	}
	dec, ok := n["doc:docs/adr/0070-permission-split.md#decision決定"]
	if !ok || dec.Start != 5 || dec.OwnEnd != 8 || dec.End != 14 {
		t.Errorf("Decision node = %+v (want lines 5, own end 8, end 14)", dec)
	}
	if _, ok := n["ADR-0070#decision決定"]; !ok {
		t.Error("no ADR-0070#decision決定 alias")
	}
	sub := n["doc:docs/adr/0070-permission-split.md#1-権限"]
	if sub.Meta != "§1" {
		t.Errorf("section number = %q, want §1", sub.Meta)
	}
	if len(n) != 8 { // 4 sections and their ADR-0070 aliases; the fenced "# not a heading" is not one
		t.Errorf("got %d nodes: %v", len(n), n)
	}
	id := dec.ID
	for _, dst := range []string{"FR-915", "ident:custom_emojis", "doc:docs/architecture/overview.md#6-リポジトリ構成"} {
		if !hasEdge(r, id, dst) {
			t.Errorf("no edge %s -> %s", id, dst)
		}
	}
	if !hasEdge(r, "doc:docs/adr/0070-permission-split.md#consequences", "issue:925") {
		t.Error("no edge to issue:925")
	}
}

func TestMarkdownRequirements(t *testing.T) {
	src := "# 機能要件\n\n| ID | 要件 | 優先度 |\n| --- | --- | --- |\n| FR-915 | 絵文字の作成の権限 | S |\n| NFR-COST-1 | コスト | M |\n"
	n := ids(Markdown("docs/requirements/01-functional.md", []byte(src)))
	if r := n["FR-915"]; r.Kind != "requirement" || r.Start != 5 {
		t.Errorf("FR-915 = %+v", r)
	}
	if _, ok := n["NFR-COST-1"]; !ok {
		t.Error("no NFR-COST-1")
	}
}

func TestGo(t *testing.T) {
	src := `// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/domain/content"
)

// Service runs the commands of FR-915.
type Service struct{ repo Repo }

// SubmitRequest files a request.
func (s *Service) SubmitRequest(ctx context.Context) error {
	return s.repo.InsertCustomEmojiRequest(ctx, content.New())
}

const (
	A = 1
	B = 2
)
`
	r := Go("internal/app/content/request.go", []byte(src))
	n := ids(r)
	m, ok := n["sym:internal/app/content.Service.SubmitRequest"]
	if !ok || m.Name != "SubmitRequest" || m.Start != 14 || m.End != 17 {
		t.Errorf("method = %+v", m)
	}
	if _, ok := n["sym:internal/app/content.B"]; !ok {
		t.Error("the second name of a const block has no node")
	}
	if !hasEdge(r, m.ID, "name:InsertCustomEmojiRequest") {
		t.Error("no call edge")
	}
	if !hasEdge(r, "file:internal/app/content/request.go", "pkg:internal/domain/content") {
		t.Error("no import edge")
	}
	if !hasEdge(r, "sym:internal/app/content.Service", "FR-915") {
		t.Error("no mention edge from the doc comment")
	}
}

func TestSQL(t *testing.T) {
	schema := "-- Requests.\nCREATE TABLE custom_emoji_requests (\n  id TEXT PRIMARY KEY,\n  emoji_id TEXT REFERENCES custom_emojis(id)\n);\n"
	r := Schema("schema/sqlite/04-content.sql", []byte(schema))
	if n := ids(r)["table:custom_emoji_requests"]; n.Start != 1 || n.End != 5 {
		t.Errorf("table = %+v", n)
	}
	if !hasEdge(r, "table:custom_emoji_requests", "table:custom_emojis") {
		t.Error("no reference edge")
	}
	q := "-- name: InsertRequest :exec\n-- Inserts one.\nINSERT INTO custom_emoji_requests (id) VALUES (?);\n\n-- name: Pending :many\nSELECT r.id FROM custom_emoji_requests r JOIN custom_emoji_pending_requests p ON p.request_id = r.id;\n"
	r = Queries("queries/sqlite/request.sql", []byte(q))
	if n := ids(r)["query:InsertRequest"]; n.End != 3 {
		t.Errorf("query = %+v", n)
	}
	if !hasEdge(r, "query:Pending", "table:custom_emoji_pending_requests") {
		t.Error("no join edge")
	}
}

func TestOpenAPIAndTS(t *testing.T) {
	r := OpenAPIRoot("api/spec/openapi.yaml", []byte("paths:\n  /accounts/delete:\n    $ref: ./paths/accounts_delete.yaml\n"))
	if !hasEdge(r, "file:api/spec/paths/accounts_delete.yaml", "apipath:/accounts/delete") {
		t.Errorf("no serves edge: %v", r.Edges)
	}
	p := "post:\n  operationId: deleteOwnAccount\n  summary: Delete it\n  requestBody:\n    content:\n      application/json:\n        schema:\n          $ref: ../components/schemas/DeleteOwnAccountRequest.yaml\n"
	r = OpenAPIPath("api/spec/paths/accounts_delete.yaml", []byte(p))
	if n := ids(r)["op:deleteOwnAccount"]; n.Meta != "post" || n.Title != "POST Delete it" {
		t.Errorf("op = %+v", n)
	}
	if !hasEdge(r, "op:deleteOwnAccount", "schema:DeleteOwnAccountRequest") {
		t.Error("no $ref edge")
	}
	ts := "import { x } from \"../../lib/x\";\n\n/** Deletes. */\nexport function DeleteButton() {\n  const m = useMutation(api.query.mutationOptions(\"post\", \"/accounts/delete\"));\n  return <Button onClick={m} />;\n}\n"
	r = TS("app/web/src/features/settings/DeleteButton.tsx", []byte(ts))
	id := "ts:app/web/src/features/settings/DeleteButton.tsx:DeleteButton"
	if n := ids(r)[id]; n.Start != 3 {
		t.Errorf("declaration = %+v (want the comment's line 3)", n)
	}
	for _, dst := range []string{"api:post /accounts/delete", "name:Button", "name:useMutation"} {
		if !hasEdge(r, id, dst) {
			t.Errorf("no edge to %s", dst)
		}
	}
	if !hasEdge(r, "file:app/web/src/features/settings/DeleteButton.tsx", "file:app/web/src/lib/x") {
		t.Error("no import edge")
	}
}

func TestLocale(t *testing.T) {
	src := "{\n  \"admin.report.title\": {\n    \"defaultMessage\": \"Report\",\n    \"description\": \"d\"\n  }\n}\n"
	r := Locale("locales/ja/client/admin/report.json", []byte(src))
	if len(r.Nodes) != 1 || r.Nodes[0].ID != "msg:client:admin.report.title@ja" || r.Nodes[0].Title != "Report" {
		t.Errorf("Locale nodes = %+v", r.Nodes)
	}
}

func TestKind(t *testing.T) {
	for p, want := range map[string]string{
		"internal/interface/clientapi/api.gen.go":              "",
		"internal/adapter/db/sqlite/accountgen/queries.go":     "",
		"app/web/src/routeTree.gen.ts":                         "",
		"app/web/src/App.tsx":                                  "ts",
		"locales/en-XA/client/settings.json":                   "",
		"locales/ja/client/admin/report.json":                  "locale",
		"queries/sqlite/queue/jobs.sql":                        "query",
		"docs/architecture/data-model/04-tables/04-content.md": "md",
		"internal/app/content/custom_emoji_request_test.go":    "go",
		"schema/postgres/04-content.sql":                       "schema",
		"schema/postgres.bootstrap.sql":                        "",
	} {
		if got := Kind(p); got != want {
			t.Errorf("Kind(%q) = %q, want %q", p, got, want)
		}
	}
}
