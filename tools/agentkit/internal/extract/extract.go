// SPDX-License-Identifier: AGPL-3.0-only

// Package extract turns one file of the repository into the nodes and edges of the knowledge graph.
// Every extractor reads only the file it is given, so that its result can be cached by the file's blob id.
//
// Node ids:
//
//	doc:<path>#<anchor>     a Markdown section (also ADR-0016#<anchor>, FR-001, NFR-COST-1)
//	sym:<dir>.<Name>        a Go declaration (sym:<dir>.<Recv>.<Name> for methods)
//	ts:<path>:<Name>        a top-level TypeScript declaration
//	table:<name>            a table of schema/<NN>-<module>/<dialect>.sql
//	query:<Name>            a sqlc query
//	op:<operationId>        a client API operation
//	schema:<Name>           a client API component
//	msg:<ns>:<key>@<locale> a message of locales/{en,ja}
//	file:<path>             a whole file
//
// Edge destinations may also be name:<Name> (a call or use by name), ident:<text> (a code span in a
// document), pkg:<dir> (a Go import), issue:<n>, and api:<method> <path> (a client API call by path).
package extract

import (
	"path"
	"strings"
)

// Version changes whenever an extractor's output changes, so that cached results are rebuilt.
const Version = "2"

// Node is one addressable piece of a file.
type Node struct {
	ID     string
	Name   string // the short name searched by `show <name>` and used to resolve name: edges
	Kind   string
	Title  string
	Start  int // first line, 1-based
	OwnEnd int // last line of the node's own text (before its first child section)
	End    int // last line, children included
	Body   string
	Meta   string
}

// Edge links a node (or the file, src "file:<path>") to another node id.
type Edge struct {
	Src, Dst, Rel string
}

// Result is what one file contributes to the graph.
type Result struct {
	Kind  string
	Nodes []Node
	Edges []Edge
}

// Kind classifies a path; "" means the file is not indexed.
func Kind(p string) string {
	base := path.Base(p)
	switch {
	case strings.HasSuffix(p, ".md"):
		if strings.HasPrefix(p, "app/") && strings.Contains(p, "/node_modules/") {
			return ""
		}
		return "md"
	case strings.HasSuffix(p, ".gen.go"), strings.Contains(p, "gen/"), strings.HasSuffix(p, "_gen.go"):
		return ""
	case strings.HasSuffix(p, ".go"):
		return "go"
	case strings.HasPrefix(p, "app/") && (strings.HasSuffix(p, ".ts") || strings.HasSuffix(p, ".tsx")):
		if strings.HasSuffix(p, ".gen.ts") || strings.HasSuffix(p, ".d.ts") {
			return ""
		}
		return "ts"
	case path.Dir(path.Dir(p)) == "schema" && (base == "sqlite.sql" || base == "postgres.sql"):
		return "schema"
	case strings.HasPrefix(p, "queries/") && strings.HasSuffix(p, ".sql"):
		return "query"
	case p == "api/spec/openapi.yaml":
		return "openapi-root"
	case strings.HasPrefix(p, "api/spec/paths/"):
		return "openapi-path"
	case strings.HasPrefix(p, "api/spec/components/"):
		return "openapi-component"
	case (strings.HasPrefix(p, "locales/en/") || strings.HasPrefix(p, "locales/ja/")) && strings.HasSuffix(base, ".json"):
		return "locale"
	}
	return ""
}

// Extract builds the nodes and edges of the file at p with content src.
func Extract(p string, src []byte) Result {
	k := Kind(p)
	var r Result
	switch k {
	case "md":
		r = Markdown(p, src)
	case "go":
		r = Go(p, src)
	case "ts":
		r = TS(p, src)
	case "schema":
		r = Schema(p, src)
	case "query":
		r = Queries(p, src)
	case "openapi-root":
		r = OpenAPIRoot(p, src)
	case "openapi-path":
		r = OpenAPIPath(p, src)
	case "openapi-component":
		r = OpenAPIComponent(p, src)
	case "locale":
		r = Locale(p, src)
	}
	r.Kind = k
	return r
}

func lineCount(src []byte) int {
	n := strings.Count(string(src), "\n")
	if len(src) > 0 && src[len(src)-1] != '\n' {
		n++
	}
	return n
}

// mentions adds the document identifiers written in s as edges from src.
func mentions(src, s string, idents bool) []Edge {
	var out []Edge
	for _, id := range requirements(s) {
		out = append(out, Edge{src, id, "mentions"})
	}
	for _, id := range adrs(s) {
		out = append(out, Edge{src, id, "mentions"})
	}
	if idents {
		for _, id := range codeSpans(s) {
			out = append(out, Edge{src, "ident:" + id, "mentions"})
		}
	}
	return out
}
