// SPDX-License-Identifier: AGPL-3.0-only

package docslint

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// RuleInventory is the rule of Inventory.
const RuleInventory = "inventory"

// Item is one name of an inventory and where the code defines it.
type Item struct {
	Name  string
	Where string // path:line, or path
}

// inventory is a set of names that users and operators see, which the code defines and a document must mention.
type inventory struct {
	name string // the key of inventory.exclude in docs/lint.yaml
	what string // what one name is, for the messages
	docs []string
	// collect returns the names the code defines, from the repository's files.
	collect func(root string, files []string) ([]Item, error)
	// mentions returns whether the documents (their text, one per element) mention a name.
	mentions func(docs []string) func(name string) bool
}

// inventories are the names checked by Inventory. The documents are the ones whose readers look the names up.
var inventories = []inventory{
	{
		name: "cli", what: "the command",
		docs:    []string{"README.md", "docs/operations/*.md", "docs/reference/*.md"},
		collect: cliCommands, mentions: cliMentions,
	},
	{
		name: "metrics", what: "the metric",
		docs:    []string{"docs/operations/running.md"},
		collect: metricNames, mentions: nameMentions,
	},
	{
		name: "webhook-events", what: "the webhook event type",
		docs:    []string{"docs/reference/webhooks.md"},
		collect: webhookEvents, mentions: codeSpanMentions,
	},
	{
		name: "build-tags", what: "the build tag",
		docs:    []string{"docs/reference/build-tags.md"},
		collect: buildTags, mentions: codeSpanMentions,
	},
	{
		name: "tasks", what: "the task",
		docs:    []string{"docs/reference/commands.md"},
		collect: tasks, mentions: taskMentions,
	},
}

// Inventory checks that every name of each inventory that the code defines is mentioned in the inventory's
// documents, or excluded in docs/lint.yaml (inventory.exclude.<inventory>); an exclusion must name a name the code
// defines and the documents do not mention, so the lists only shrink.
func Inventory(root string, files []string, c *Config) ([]Finding, error) {
	known := map[string]bool{}
	var out []Finding
	for _, inv := range inventories {
		known[inv.name] = true
		items, err := inv.collect(root, files)
		if err != nil {
			return nil, fmt.Errorf("inventory %s: %w", inv.name, err)
		}
		var docs []string
		for _, f := range files {
			if matchAny(inv.docs, f) {
				b, err := os.ReadFile(filepath.Join(root, f))
				if err != nil {
					return nil, err
				}
				docs = append(docs, string(b))
			}
		}
		mentioned := inv.mentions(docs)
		excluded := map[string]bool{}
		for _, n := range c.Inventory.Exclude[inv.name] {
			excluded[n] = true
		}
		defined := map[string]bool{}
		where := strings.Join(inv.docs, ", ")
		for _, it := range items {
			if defined[it.Name] {
				continue
			}
			defined[it.Name] = true
			if excluded[it.Name] {
				if mentioned(it.Name) {
					out = append(out, Finding{File, 0, RuleInventory, fmt.Sprintf("inventory.exclude.%s lists %s, which %s mentions now; remove it", inv.name, it.Name, where)})
				}
				continue
			}
			if !mentioned(it.Name) {
				out = append(out, Finding{where, 0, RuleInventory, fmt.Sprintf("%s %s (%s) is in none of these documents: describe it there, or list it under inventory.exclude.%s of %s with the reason", inv.what, it.Name, it.Where, inv.name, File)})
			}
		}
		for _, n := range c.Inventory.Exclude[inv.name] {
			if !defined[n] {
				out = append(out, Finding{File, 0, RuleInventory, fmt.Sprintf("inventory.exclude.%s lists %s, which the code does not define; remove it", inv.name, n)})
			}
		}
	}
	for _, k := range sortedKeys(c.Inventory.Exclude) {
		if !known[k] {
			out = append(out, Finding{File, 0, RuleInventory, fmt.Sprintf("inventory.exclude has no inventory %q", k)})
		}
	}
	return out, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// goFiles parses the non-test Go files of files that keep returns true for, with their comments.
func goFiles(root string, files []string, keep func(string) bool) (*token.FileSet, map[string]*ast.File, error) {
	fset := token.NewFileSet()
	out := map[string]*ast.File{}
	for _, f := range files {
		if !strings.HasSuffix(f, ".go") || strings.HasSuffix(f, "_test.go") || !keep(f) {
			continue
		}
		src, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			return nil, nil, err
		}
		af, err := parser.ParseFile(fset, f, src, parser.SkipObjectResolution)
		if err != nil {
			return nil, nil, err
		}
		out[f] = af
	}
	return fset, out, nil
}

func stringLit(e ast.Expr) (string, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	return s, err == nil
}

func pos(fset *token.FileSet, p token.Pos) string {
	ps := fset.Position(p)
	return fmt.Sprintf("%s:%d", ps.Filename, ps.Line)
}

// --- my commands ---------------------------------------------------------------------------------------------

// cliDir holds the server binary, whose subcommands dispatch by hand: `switch` on the subcommand's word
// (`switch sub`, `switch cmd`, or `args[0] == "word"` cases) and calls of the helper dispatch("admin account", args,
// []string{"list", ...}, ...), whose first argument names the command.
const cliDir = "cmd/my/"

var reWord = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// cliCommands returns every command and subcommand of `my`, as "my admin role grant".
func cliCommands(root string, files []string) ([]Item, error) {
	fset, parsed, err := goFiles(root, files, func(f string) bool { return path.Dir(f)+"/" == cliDir })
	if err != nil {
		return nil, err
	}
	funcs := map[string]*ast.FuncDecl{}
	for _, f := range sortedKeys(parsed) {
		for _, d := range parsed[f].Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil {
				funcs[fd.Name.Name] = fd
			}
		}
	}
	found := map[string]string{}
	add := func(words []string, p token.Pos) {
		for i := range words {
			name := "my " + strings.Join(words[:i+1], " ")
			if _, ok := found[name]; !ok {
				found[name] = pos(fset, p)
			}
		}
	}
	// dispatch calls name their command themselves.
	for _, fd := range funcs {
		ast.Inspect(fd, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 3 {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); !ok || id.Name != "dispatch" {
				return true
			}
			prefix, ok := stringLit(call.Args[0])
			list, isList := call.Args[2].(*ast.CompositeLit)
			if !ok || !isList {
				return true
			}
			for _, el := range list.Elts {
				if w, ok := stringLit(el); ok && reWord.MatchString(w) {
					add(append(strings.Fields(prefix), w), el.Pos())
				}
			}
			return true
		})
	}
	// The switches, from `run` down through the run* functions their cases call.
	seen := map[string]bool{}
	var walk func(fd *ast.FuncDecl, words []string)
	walk = func(fd *ast.FuncDecl, words []string) {
		key := fd.Name.Name + " " + strings.Join(words, " ")
		if seen[key] || len(words) > 8 {
			return
		}
		seen[key] = true
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			for _, st := range sw.Body.List {
				cc := st.(*ast.CaseClause)
				for _, w := range caseWords(sw.Tag, cc) {
					sub := append(append([]string(nil), words...), w)
					add(sub, cc.Pos())
					for _, callee := range calledRuns(cc, funcs, fd.Name.Name) {
						walk(callee, sub)
					}
				}
			}
			return true
		})
	}
	if run := funcs["run"]; run != nil {
		walk(run, nil)
	}
	out := make([]Item, 0, len(found))
	for _, name := range sortedKeys(found) {
		out = append(out, Item{name, found[name]})
	}
	return out, nil
}

// subcommandExprs are the expressions a switch on the subcommand's word compares.
var subcommandExprs = map[string]bool{"sub": true, "cmd": true, "args[0]": true, "fs.Arg(0)": true}

// caseWords returns the subcommand words a case of a switch on tag compares with.
func caseWords(tag ast.Expr, cc *ast.CaseClause) []string {
	var out []string
	for _, e := range cc.List {
		var lit ast.Expr
		switch {
		case tag != nil && subcommandExprs[exprString(tag)]:
			lit = e
		case tag == nil:
			if be, ok := e.(*ast.BinaryExpr); ok && (be.Op == token.EQL || be.Op == token.NEQ) && subcommandExprs[exprString(be.X)] {
				lit = be.Y
			}
		}
		if lit == nil {
			continue
		}
		if w, ok := stringLit(lit); ok && reWord.MatchString(w) {
			out = append(out, w)
		}
	}
	return out
}

// calledRuns returns the run* functions of funcs that the case calls, other than self.
func calledRuns(cc *ast.CaseClause, funcs map[string]*ast.FuncDecl, self string) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	for _, st := range cc.Body {
		ast.Inspect(st, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); ok && strings.HasPrefix(id.Name, "run") && id.Name != self && funcs[id.Name] != nil {
				out = append(out, funcs[id.Name])
			}
			return true
		})
	}
	return out
}

func exprString(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return exprString(x.X) + "." + x.Sel.Name
	case *ast.IndexExpr:
		return exprString(x.X) + "[" + exprString(x.Index) + "]"
	case *ast.BasicLit:
		return x.Value
	case *ast.CallExpr:
		args := make([]string, len(x.Args))
		for i, a := range x.Args {
			args[i] = exprString(a)
		}
		return exprString(x.Fun) + "(" + strings.Join(args, ", ") + ")"
	}
	return ""
}

// reMyCommand finds a command line of `my` in a document: the words after `my`, a word written as alternatives
// (`my job list|retry|delete`) included. Flags, placeholders, and the end of a code span end the words.
var reMyCommand = regexp.MustCompile(`(?:^|[^A-Za-z0-9_./-])my((?:[ \t]+[a-z][a-z0-9-]*(?:\|[a-z][a-z0-9-]*)*)+)`)

// cliMentions returns whether a document mentions a command: a command line in it starts with the command's
// words, or it has a command line of the parent command and a code span that starts with the last word (a list of
// the subcommands of `my admin supporters`: "`status`: ...").
func cliMentions(docs []string) func(string) bool {
	set := map[string]bool{}
	for _, text := range docs {
		lines := map[string]bool{}
		for _, m := range reMyCommand.FindAllStringSubmatch(text, -1) {
			prefixes := []string{"my"}
			for _, tok := range strings.Fields(m[1]) {
				var next []string
				for _, p := range prefixes {
					for _, alt := range strings.Split(tok, "|") {
						next = append(next, p+" "+alt)
					}
				}
				if len(next) > 64 {
					break
				}
				for _, p := range next {
					lines[p] = true
				}
				prefixes = next
			}
		}
		words := codeSpanWords(text)
		for p := range lines {
			set[p] = true
			for _, w := range words {
				set[p+" "+w] = true
			}
		}
	}
	return func(name string) bool { return set[name] }
}

// codeSpanWords returns the first word of each code span of text that could be a subcommand's word.
func codeSpanWords(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range reCodeSpan.FindAllStringSubmatch(text, -1) {
		if f := strings.Fields(m[1]); len(f) > 0 && reWord.MatchString(f[0]) && !seen[f[0]] {
			seen[f[0]] = true
			out = append(out, f[0])
		}
	}
	return out
}

// --- metrics -------------------------------------------------------------------------------------------------

// metricNames returns the names of the metrics the server defines: the Name of every kernel.Instrument literal in
// the server's packages.
func metricNames(root string, files []string) ([]Item, error) {
	keep := func(f string) bool {
		if !strings.HasPrefix(f, "internal/") && !strings.HasPrefix(f, "cmd/") {
			return false
		}
		b, err := os.ReadFile(filepath.Join(root, f))
		return err == nil && strings.Contains(string(b), "Instrument{")
	}
	fset, parsed, err := goFiles(root, files, keep)
	if err != nil {
		return nil, err
	}
	var out []Item
	for _, f := range sortedKeys(parsed) {
		ast.Inspect(parsed[f], func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok || (exprString(cl.Type) != "kernel.Instrument" && exprString(cl.Type) != "Instrument") {
				return true
			}
			for _, el := range cl.Elts {
				if kv, ok := el.(*ast.KeyValueExpr); ok && exprString(kv.Key) == "Name" {
					if s, ok := stringLit(kv.Value); ok {
						out = append(out, Item{s, pos(fset, kv.Pos())})
					}
				}
			}
			return true
		})
	}
	return out, nil
}

// nameMentions returns whether the text has a name as a whole word: not part of a longer dotted or underscored
// name.
func nameMentions(docs []string) func(string) bool {
	text := strings.Join(docs, "\n")
	return func(name string) bool {
		for i := 0; ; {
			j := strings.Index(text[i:], name)
			if j < 0 {
				return false
			}
			start, end := i+j, i+j+len(name)
			before := start == 0 || !isNameByte(text[start-1]) && text[start-1] != '.'
			after := end == len(text) || !isNameByte(text[end]) && (text[end] != '.' || end+1 == len(text) || !isNameByte(text[end+1]))
			if before && after {
				return true
			}
			i = start + 1
		}
	}
}

func isNameByte(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// --- webhook event types -------------------------------------------------------------------------------------

const webhookDir = "internal/domain/webhook/"

// webhookEvents returns the event types of the server webhooks: the constants of type EventType.
func webhookEvents(root string, files []string) ([]Item, error) {
	fset, parsed, err := goFiles(root, files, func(f string) bool { return path.Dir(f)+"/" == webhookDir })
	if err != nil {
		return nil, err
	}
	var out []Item
	for _, f := range sortedKeys(parsed) {
		for _, d := range parsed[f].Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				if vs.Type == nil || exprString(vs.Type) != "EventType" {
					continue
				}
				for _, v := range vs.Values {
					if s, ok := stringLit(v); ok {
						out = append(out, Item{s, pos(fset, v.Pos())})
					}
				}
			}
		}
	}
	return out, nil
}

var reCodeSpan = regexp.MustCompile("`([^`\n]+)`")

// codeSpanMentions returns whether the text has a name as a whole code span.
func codeSpanMentions(docs []string) func(string) bool {
	set := map[string]bool{}
	for _, m := range reCodeSpan.FindAllStringSubmatch(strings.Join(docs, "\n"), -1) {
		set[strings.TrimSpace(m[1])] = true
	}
	return func(name string) bool { return set[name] }
}

// --- build tags ----------------------------------------------------------------------------------------------

// buildTags returns the tags that leave a part out of the server: the `no_*` tags of the `//go:build` lines of
// the server module's Go files (not of the modules of their own, such as tools/agentkit and pkg/mykit).
func buildTags(root string, files []string) ([]Item, error) {
	var modules []string
	for _, f := range files {
		if path.Base(f) == "go.mod" && f != "go.mod" {
			modules = append(modules, path.Dir(f)+"/")
		}
	}
	found := map[string]string{}
	for _, f := range files {
		if !strings.HasSuffix(f, ".go") || hasAnyPrefix(f, modules) {
			continue
		}
		src, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			return nil, err
		}
		for i, l := range strings.Split(string(src), "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "package ") {
				break
			}
			if !constraint.IsGoBuild(l) {
				continue
			}
			expr, err := constraint.Parse(l)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", f, i+1, err)
			}
			expr.Eval(func(tag string) bool {
				if _, ok := found[tag]; !ok && strings.HasPrefix(tag, "no_") {
					found[tag] = fmt.Sprintf("%s:%d", f, i+1)
				}
				return true
			})
		}
	}
	out := make([]Item, 0, len(found))
	for _, tag := range sortedKeys(found) {
		out = append(out, Item{tag, found[tag]})
	}
	return out, nil
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// --- tasks ---------------------------------------------------------------------------------------------------

// taskfile is the part of a go-task file that names its tasks.
type taskfile struct {
	Includes map[string]yaml.Node `yaml:"includes"`
	Tasks    map[string]struct {
		Desc     string `yaml:"desc"`
		Internal bool   `yaml:"internal"`
	} `yaml:"tasks"`
}

// tasks returns the tasks that `scripts/task.sh --list` lists: the tasks of Taskfile.yml and of the files it
// includes (`<namespace>:<task>`) that have a description and are not internal.
func tasks(root string, _ []string) ([]Item, error) {
	var out []Item
	var read func(file, ns string) error
	read = func(file, ns string) error {
		b, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			return err
		}
		var tf taskfile
		if err := yaml.Unmarshal(b, &tf); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		for _, name := range sortedKeys(tf.Tasks) {
			if t := tf.Tasks[name]; t.Desc != "" && !t.Internal {
				out = append(out, Item{ns + name, file})
			}
		}
		for _, inc := range sortedKeys(tf.Includes) {
			node := tf.Includes[inc]
			var target string
			switch node.Kind {
			case yaml.ScalarNode:
				target = node.Value
			case yaml.MappingNode:
				var m struct {
					Taskfile string `yaml:"taskfile"`
				}
				if err := node.Decode(&m); err != nil {
					return err
				}
				target = m.Taskfile
			}
			if target == "" {
				continue
			}
			if err := read(path.Clean(path.Join(path.Dir(file), target)), ns+inc+":"); err != nil {
				return err
			}
		}
		return nil
	}
	if err := read("Taskfile.yml", ""); err != nil {
		return nil, err
	}
	return out, nil
}

// taskMentions returns whether the text has a code span that starts with the task's name (`docs:adr-new TITLE=...`).
func taskMentions(docs []string) func(string) bool {
	set := map[string]bool{}
	for _, m := range reCodeSpan.FindAllStringSubmatch(strings.Join(docs, "\n"), -1) {
		if f := strings.Fields(m[1]); len(f) > 0 {
			set[f[0]] = true
		}
	}
	return func(name string) bool { return set[name] }
}
