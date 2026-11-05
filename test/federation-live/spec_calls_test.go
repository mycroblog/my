// SPDX-License-Identifier: AGPL-3.0-only

package federationlive

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// This file has no build tag: scripts/task.sh api:live-calls runs it on every pull request, where
// the scenarios themselves do not run. It reads the sources and the spec, and starts no server.

// specFile is the bundle of the client API spec, and shellCallers are the scripts of the slow suite
// that call the client API with curl.
const specFile = "../../api/openapi.yaml"

var shellCallers = []string{
	"../../scripts/upgrade-test.sh",
	"../../scripts/restore-drill.sh",
	"../../scripts/docker-smoke.sh",
}

// liveCall is one call of the client API: its method, and its path with each variable segment as {}.
type liveCall struct {
	where, method, path string
}

func (c liveCall) String() string { return c.method + " " + c.path }

// TestLiveCallsAreInTheSpec fails when the scenarios or the scripts call a method and path of the
// client API that api/openapi.yaml does not have.
func TestLiveCallsAreInTheSpec(t *testing.T) {
	spec, err := os.Open(specFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = spec.Close() }()
	ops, err := specOperations(bufio.NewScanner(spec))
	if err != nil {
		t.Fatal(err)
	}

	// Finding nothing would mean that this test no longer recognizes the calls.
	sources, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	var calls []liveCall
	for _, file := range sources {
		src, err := os.ReadFile(file) //nolint:gosec // G304: a file of this directory
		if err != nil {
			t.Fatal(err)
		}
		found, problems := goCalls(file, src)
		calls = append(calls, found...)
		for _, p := range problems {
			t.Error(p)
		}
	}
	if len(calls) == 0 {
		t.Errorf("found no call of localClient.api in %v", sources)
	}
	for _, file := range shellCallers {
		src, err := os.ReadFile(file) //nolint:gosec // G304: a constant path
		if err != nil {
			t.Fatal(err)
		}
		found := shellCalls(file, string(src))
		if len(found) == 0 {
			t.Errorf("found no call of the client API in %s", file)
		}
		calls = append(calls, found...)
	}

	for _, c := range calls {
		if !ops[c.String()] {
			t.Errorf("%s: %s is not in api/openapi.yaml", c.where, c)
		}
	}
}

// TestLiveCallsFindARenamedPath checks that the readers see the call that made this check necessary,
// and that they refuse a path they cannot read.
func TestLiveCallsFindARenamedPath(t *testing.T) {
	src := []byte(`package p
func f() {
	_ = e.local.api(ctx, http.MethodPost, "/admin/relays/"+r.ID+"/unfollow", nil, nil)
	_ = l.api(ctx, http.MethodGet, "/accounts/"+url.PathEscape(id)+"/statuses?"+url.Values{}.Encode(), nil, nil)
	_ = l.api(ctx, http.MethodPost, "/statuses/"+url.PathEscape(id)+"/"+action, nil, nil)
	_ = l.api(ctx, method, "/statuses", nil, nil)
	_ = p.call(ctx, http.MethodGet, "/api/v1/instance", nil, nil)
}
`)
	calls, problems := goCalls("sample.go", src)
	want := []string{"POST /admin/relays/{}/unfollow", "GET /accounts/{}/statuses"}
	if got := callNames(calls); !slices.Equal(got, want) {
		t.Errorf("goCalls = %q, want %q", got, want)
	}
	if len(problems) != 2 {
		t.Errorf("problems = %q, want the variable segment and the variable method", problems)
	}

	shell := `# api METHOD PATH TOKEN [JSON]: a client API call
	curl "${args[@]}" "$base/api/v1$2" || fail "$1 $2 failed"
	out="$(api POST "/accounts/$alice_id/follow" "$bob_token")"
	carol="$(api GET '/accounts/lookup?acct=carol@remote.test&resolve=true' "$alice_token")"
	id="$(curl -fsS -H "Authorization: Bearer $t" -F "file=@$f" "$base/api/v1/media" | jq -er .id)"
	curl -sS -o /dev/null -X POST "$base/api/v1/accounts/$carol/follow" || true
	me="$(curl -fsS "$api_origin/api/v1/accounts/verify_credentials")"
`
	want = []string{
		"POST /accounts/{}/follow", "GET /accounts/lookup", "POST /media", "POST /accounts/{}/follow", "GET /accounts/verify_credentials",
	}
	if got := callNames(shellCalls("sample.sh", shell)); !slices.Equal(got, want) {
		t.Errorf("shellCalls = %q, want %q", got, want)
	}

	spec := "openapi: 3.1.0\npaths:\n  /admin/relays/{id}/follow:\n    post:\n      summary: get:\n    delete:\ncomponents:\n  /no:\n    get:\n"
	ops, err := specOperations(bufio.NewScanner(strings.NewReader(spec)))
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"DELETE /admin/relays/{}/follow", "POST /admin/relays/{}/follow"}
	got := make([]string, 0, len(ops))
	for op := range ops {
		got = append(got, op)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("specOperations = %q, want %q", got, want)
	}
}

func callNames(calls []liveCall) []string {
	names := make([]string, 0, len(calls))
	for _, c := range calls {
		names = append(names, c.String())
	}
	return names
}

var (
	pathParameter = regexp.MustCompile(`\{[^}]*\}`)
	specPath      = regexp.MustCompile(`^  ['"]?(/[^'":]*)['"]?:$`)
	specMethod    = regexp.MustCompile(`^    (get|put|post|delete|patch|head|options):`)
)

// specOperations reads the operations of the spec as "METHOD /path/{}". The bundle is generated, so
// its layout is fixed: each path is a key at two spaces under paths, and each method a key at four.
func specOperations(lines *bufio.Scanner) (map[string]bool, error) {
	lines.Buffer(nil, 1<<20)
	ops := map[string]bool{}
	in, path := false, ""
	for lines.Scan() {
		line := lines.Text()
		if !in {
			in = line == "paths:"
			continue
		}
		if line != "" && line[0] != ' ' {
			break
		}
		if m := specPath.FindStringSubmatch(line); m != nil {
			path = pathParameter.ReplaceAllString(m[1], "{}")
		} else if m := specMethod.FindStringSubmatch(line); m != nil && path != "" {
			ops[strings.ToUpper(m[1])+" "+path] = true
		}
	}
	if err := lines.Err(); err != nil {
		return nil, err
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("no operations in %s", specFile)
	}
	return ops, nil
}

// goCalls returns the calls of localClient.api in a Go source, and a problem for each one whose
// method or path it cannot read. A call found by its name needs no types, so the check builds none of
// the scenarios' dependencies.
func goCalls(file string, src []byte) ([]liveCall, []string) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, []string{err.Error()}
	}
	var calls []liveCall
	var problems []string
	ast.Inspect(f, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := c.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "api" || len(c.Args) != 5 {
			return true
		}
		where := fset.Position(c.Pos()).String()
		method, ok := goMethod(c.Args[1])
		if !ok {
			problems = append(problems, where+": write the method of localClient.api as http.MethodX, for api/openapi.yaml to be checked")
			return true
		}
		path, ok := goPath(c.Args[2])
		if !ok {
			problems = append(problems, where+": write the path of localClient.api with literals, url.PathEscape, and x.ID, for api/openapi.yaml to be checked")
			return true
		}
		path, _, _ = strings.Cut(path, "?")
		calls = append(calls, liveCall{where: where, method: method, path: path})
		return true
	})
	return calls, problems
}

func goMethod(e ast.Expr) (string, bool) {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "http" || !strings.HasPrefix(sel.Sel.Name, "Method") {
		return "", false
	}
	return strings.ToUpper(strings.TrimPrefix(sel.Sel.Name, "Method")), true
}

// goPath reads a path made of string literals, an ID (x.ID, url.PathEscape(x)) for a variable
// segment, and url.Values{...}.Encode() for the query, whose text does not matter.
func goPath(e ast.Expr) (string, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		l, ok := goPath(e.X)
		if !ok {
			return "", false
		}
		r, ok := goPath(e.Y)
		return l + r, ok
	case *ast.SelectorExpr:
		return "{}", true
	case *ast.CallExpr:
		sel, ok := e.Fun.(*ast.SelectorExpr)
		switch {
		case !ok:
			return "", false
		case sel.Sel.Name == "PathEscape":
			return "{}", true
		case sel.Sel.Name == "Encode":
			return "", true
		}
	}
	return "", false
}

var (
	shellVariable = regexp.MustCompile(`\$\{?\w+\}?`)
	// shellAPI is a call of the scripts' api helper: api METHOD PATH TOKEN [JSON].
	shellAPI = regexp.MustCompile(`\bapi (GET|POST|PUT|PATCH|DELETE) ['"]?(/[^'"\s]*)`)
	// shellCurl is a curl of the client API outside that helper, whose own "$base/api/v1$2" does not match.
	shellCurl = regexp.MustCompile(`\$(?:base|api_origin)/api/v1(/[^'"\s]*)`)
	curlX     = regexp.MustCompile(` -X (\w+)`)
)

// shellCalls returns the calls of the client API in a shell script.
func shellCalls(file, src string) []liveCall {
	var calls []liveCall
	for i, line := range strings.Split(src, "\n") {
		where := fmt.Sprintf("%s:%d", file, i+1)
		add := func(method, path string) {
			path, _, _ = strings.Cut(path, "?")
			calls = append(calls, liveCall{where: where, method: method, path: shellVariable.ReplaceAllString(path, "{}")})
		}
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		for _, m := range shellAPI.FindAllStringSubmatch(line, -1) {
			add(m[1], m[2])
		}
		if m := shellCurl.FindStringSubmatch(line); m != nil {
			method := http.MethodGet
			if x := curlX.FindStringSubmatch(line); x != nil {
				method = x[1]
			} else if strings.Contains(line, " -F ") {
				method = http.MethodPost
			}
			add(method, m[1])
		}
	}
	return calls
}
