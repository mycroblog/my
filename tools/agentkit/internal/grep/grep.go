// SPDX-License-Identifier: AGPL-3.0-only

// Package grep searches the files of the working tree for a regular expression or a fixed string, the way an
// agent needs it: only the files git tracks (and new files it does not ignore), without generated files, lock
// files, snapshots and tests unless asked, and with an output that is bounded, grouped by file, and names the
// declaration or section each match is in.
package grep

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/mycroblog/my/tools/agentkit/internal/ctxmap"
	"github.com/mycroblog/my/tools/agentkit/internal/extract"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
	"github.com/mycroblog/my/tools/agentkit/internal/text"
)

// Usage is the command line of agentkit grep.
const Usage = `usage: agentkit grep [flags] <pattern> [path|glob ...]
  -i  ignore case        -w  whole words        -F  fixed string, not a regular expression
  -l  files with counts  -o  the matched texts with counts        -q  no output, exit 1 without a match
  -n N        matches to print (40)             -C N  lines of context (also -A, -B)
  --max N     characters to print (6000)        -e PATTERN  a pattern that starts with -
  --kind k,k  go ts md sql yaml json            --include GLOB  only files whose name matches
  --tests     also search tests                 --only-tests
  --gen       also search generated files, fixtures, lock files and snapshots
The pattern is a Go (RE2) regular expression; \| is read as alternation, as grep does. Flags of grep that change
nothing here (-r -n -E -H -h -s -I) are accepted.`

// Options are the flags of agentkit grep.
type Options struct {
	Pattern    string
	Paths      []string
	IgnoreCase bool
	Word       bool
	Fixed      bool
	List       bool
	Only       bool
	Quiet      bool
	Tests      bool
	OnlyTests  bool
	Gen        bool
	Kinds      []string
	Include    []string
	Limit      int // matches to print
	Context    int // lines around a match
	Max        int // characters to print
}

var kindExts = map[string][]string{
	"go":   {".go"},
	"ts":   {".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs"},
	"md":   {".md", ".mdx"},
	"sql":  {".sql"},
	"yaml": {".yaml", ".yml"},
	"json": {".json", ".jsonc"},
}

// rgTypes maps the type names of ripgrep to kinds.
var rgTypes = map[string]string{
	"go": "go", "ts": "ts", "typescript": "ts", "js": "ts", "tsx": "ts", "md": "md", "markdown": "md", "sql": "sql",
	"yaml": "yaml", "yml": "yaml", "json": "json",
}

// ParseArgs reads the command line. It takes the flags in any position and the habits of grep and rg:
// combined short flags (-rn, -inE), -A3, --include=*.go, -t go.
func ParseArgs(args []string) (Options, error) {
	o := Options{Limit: 40, Max: 6000}
	havePattern := false
	var pos []string
	num := func(name, v string) (int, error) {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("%s needs a number, not %q\n%s", name, v, Usage)
		}
		return n, nil
	}
	value := func(name, v string) error {
		switch name {
		case "e":
			if havePattern {
				o.Pattern += "|" + v
			} else {
				o.Pattern, havePattern = v, true
			}
		case "n", "m", "limit", "max-count":
			n, err := num("-"+name, v)
			if err != nil {
				return err
			}
			o.Limit = n
		case "A", "B", "C", "context", "after-context", "before-context":
			n, err := num("-"+name, v)
			if err != nil {
				return err
			}
			o.Context = max(o.Context, n)
		case "max":
			n, err := num("--max", v)
			if err != nil {
				return err
			}
			o.Max = n
		case "kind", "t", "type":
			for _, k := range strings.Split(v, ",") {
				if rgTypes[k] == "" {
					return fmt.Errorf("unknown kind %q (go ts md sql yaml json)", k)
				}
				o.Kinds = append(o.Kinds, rgTypes[k])
			}
		case "include", "g", "glob":
			o.Include = append(o.Include, v)
		}
		return nil
	}
	longValue := map[string]bool{
		"limit": true, "max-count": true, "context": true, "after-context": true, "before-context": true, "max": true,
		"kind": true, "type": true, "include": true, "glob": true, "exclude": true, "exclude-dir": true, "color": true,
		"colour": true, "regexp": true,
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			pos = append(pos, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "--"):
			name, v, eq := strings.Cut(a[2:], "=")
			if longValue[name] {
				if !eq {
					if i+1 >= len(args) {
						return o, fmt.Errorf("--%s needs a value\n%s", name, Usage)
					}
					i++
					v = args[i]
				}
				if name == "regexp" {
					name = "e"
				}
				if err := value(name, v); err != nil {
					return o, err
				}
				continue
			}
			switch name {
			case "tests":
				o.Tests = true
			case "only-tests":
				o.Tests, o.OnlyTests = true, true
			case "gen":
				o.Gen = true
			case "ignore-case":
				o.IgnoreCase = true
			case "word-regexp":
				o.Word = true
			case "fixed-strings":
				o.Fixed = true
			case "files-with-matches", "count":
				o.List = true
			case "only-matching":
				o.Only = true
			case "quiet", "silent":
				o.Quiet = true
			case "recursive", "line-number", "extended-regexp", "no-heading", "with-filename", "no-messages", "no-filename", "perl-regexp":
			default:
				return o, fmt.Errorf("unknown flag --%s\n%s", name, Usage)
			}
		case len(a) > 1 && a[0] == '-':
			for j := 1; j < len(a); j++ {
				ch := a[j]
				switch ch {
				case 'i':
					o.IgnoreCase = true
				case 'w':
					o.Word = true
				case 'F':
					o.Fixed = true
				case 'l', 'c':
					o.List = true
				case 'o':
					o.Only = true
				case 'q':
					o.Quiet = true
				case 'r', 'R', 'E', 'H', 'h', 's', 'I', 'P':
				case 'n':
					// grep's -n (line numbers, always on) unless a number follows: then it is the limit. A number
					// that is the last word before any pattern is the pattern.
					rest := a[j+1:]
					if rest == "" && i+1 < len(args) && isNumber(args[i+1]) && (havePattern || len(pos) > 0 || i+2 < len(args)) {
						i++
						rest = args[i]
					}
					if isNumber(rest) {
						if err := value("n", rest); err != nil {
							return o, err
						}
						j = len(a)
					}
				case 'e', 'm', 'A', 'B', 'C', 't', 'g':
					v := a[j+1:]
					if v == "" {
						if i+1 >= len(args) {
							return o, fmt.Errorf("-%c needs a value\n%s", ch, Usage)
						}
						i++
						v = args[i]
					}
					if err := value(string(ch), v); err != nil {
						return o, err
					}
					j = len(a)
				default:
					if isNumber(a[j:]) {
						o.Context, _ = strconv.Atoi(a[j:])
						j = len(a)
						continue
					}
					return o, fmt.Errorf("agentkit grep has no -%c (grep's -v, -L and -z: pipe `git ls-files` or a file into grep)\n%s", ch, Usage)
				}
			}
		default:
			pos = append(pos, a)
		}
	}
	if !havePattern {
		if len(pos) == 0 {
			return o, fmt.Errorf("%s", Usage)
		}
		o.Pattern, pos = pos[0], pos[1:]
	}
	if len(pos) > 0 {
		o.Paths = pos
	}
	if o.Pattern == "" {
		return o, fmt.Errorf("the pattern is empty\n%s", Usage)
	}
	return o, nil
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// compile builds the regular expression of o and says how it read the pattern when that is not as written.
func compile(o Options) (*regexp.Regexp, string, error) {
	p, note := o.Pattern, ""
	if o.Fixed {
		p = regexp.QuoteMeta(p)
	} else {
		// grep's basic syntax writes alternation \| and word ends \< \>.
		if strings.Contains(p, `\|`) {
			p = strings.ReplaceAll(p, `\|`, "|")
			note = `read \| as alternation (-F for a fixed string)`
		}
		p = strings.NewReplacer(`\<`, `\b`, `\>`, `\b`).Replace(p)
		if _, err := regexp.Compile(p); err != nil {
			p = regexp.QuoteMeta(o.Pattern)
			note = "not a regular expression (" + strings.TrimPrefix(err.Error(), "error parsing regexp: ") + "): searched as a fixed string"
		}
	}
	if o.Word {
		p = `\b(?:` + p + `)\b`
	}
	flags := "(?m)"
	if o.IgnoreCase {
		flags = "(?im)"
	}
	re, err := regexp.Compile(flags + p)
	return re, note, err
}

// class is why a file is left out by default.
type class int

const (
	source class = iota
	test
	generated
)

var lockFiles = map[string]bool{
	"go.sum": true, "go.work.sum": true, "bun.lock": true, "bun.lockb": true, "package-lock.json": true,
	"pnpm-lock.yaml": true, "yarn.lock": true, "Cargo.lock": true, ".terraform.lock.hcl": true,
}

func classify(p string, ignore []string) class {
	base := path.Base(p)
	slashed := "/" + p
	switch {
	case lockFiles[base], strings.HasSuffix(base, ".lock"), strings.HasSuffix(base, ".snap"), strings.HasSuffix(base, ".min.js"),
		strings.HasSuffix(base, ".gen.go"), strings.HasSuffix(base, "_gen.go"), strings.HasSuffix(base, ".pb.go"),
		strings.HasSuffix(base, ".gen.ts"), strings.HasSuffix(base, ".svg"),
		strings.Contains(slashed, "/node_modules/"), strings.Contains(slashed, "/__snapshots__/"),
		strings.Contains(slashed, "/__screenshots__/"):
		return generated
	}
	for _, pat := range ignore {
		if ctxmap.Match(pat, p) {
			return generated
		}
	}
	switch {
	case strings.HasSuffix(base, "_test.go"), strings.Contains(base, ".test."), strings.Contains(base, ".spec."),
		strings.Contains(base, ".stories."), strings.Contains(slashed, "/e2e/"), strings.Contains(slashed, "/testdata/"),
		strings.Contains(slashed, "/__tests__/"):
		return test
	}
	return source
}

// hit is the matches of one file.
type hit struct {
	path     string // relative to the repository root, or as given for a file outside it
	abs      string
	class    class
	explicit bool  // named on the command line
	lines    []int // the first matching lines, 1-based
	count    int   // matching lines
	only     map[string]int
}

const maxFileSize = 4 << 20

// Run searches the working tree r from the directory cwd and prints the result. It reports whether anything
// matched.
func Run(r *repo.Repo, cwd string, o Options, w io.Writer) (bool, error) {
	re, note, err := compile(o)
	if err != nil {
		return false, err
	}
	var ignore []string
	if m, err := ctxmap.Load(r.Root); err == nil {
		ignore = m.Ignore
	}
	files, missing, err := list(r, cwd, o.Paths)
	if err != nil {
		return false, err
	}
	exts := map[string]bool{}
	for _, k := range o.Kinds {
		for _, e := range kindExts[k] {
			exts[e] = true
		}
	}
	var todo []*hit
	for _, f := range files {
		if len(exts) > 0 && !exts[path.Ext(f.path)] {
			continue
		}
		if len(o.Include) > 0 && !included(o.Include, f.path) {
			continue
		}
		f.class = classify(f.path, ignore)
		todo = append(todo, f)
	}
	keep := max(o.Limit, 1)
	if o.Only {
		keep = 0
	}
	scan(todo, re, keep, o.Only)

	// Tests and generated files count apart unless asked for, or named, or nothing else matched.
	var shown, tests, gen []*hit
	for _, h := range todo {
		if h.count == 0 {
			continue
		}
		switch {
		case h.explicit, h.class == source && !o.OnlyTests, h.class == test && o.Tests, h.class == generated && o.Gen:
			shown = append(shown, h)
		case h.class == test:
			tests = append(tests, h)
		case h.class == generated:
			gen = append(gen, h)
		}
	}
	var notes []string
	if note != "" {
		notes = append(notes, note)
	}
	for _, m := range missing {
		notes = append(notes, fmt.Sprintf("%s: no tracked file (a path git ignores or outside the repository is searched with grep)", m))
	}
	if len(shown) == 0 && !o.OnlyTests {
		switch {
		case len(tests) > 0:
			shown, tests = tests, nil
			notes = append(notes, "no match outside tests: these are the matches in tests")
		case len(gen) > 0:
			shown, gen = gen, nil
			notes = append(notes, "no match in sources: these are the matches in generated files, fixtures, lock files and snapshots")
		}
	}
	if o.Quiet {
		return len(shown) > 0, nil
	}
	sort.SliceStable(shown, func(i, j int) bool {
		if shown[i].class != shown[j].class {
			return shown[i].class < shown[j].class
		}
		return shown[i].path < shown[j].path
	})
	p := &printer{w: w, o: o, re: re, root: r.Root}
	switch {
	case len(shown) == 0:
		fmt.Fprintf(w, "no match in %d files\n", len(todo))
	case o.Only:
		p.only(shown)
	case o.List:
		p.list(shown)
	default:
		p.matches(shown)
	}
	if n, f := total(tests); n > 0 {
		notes = append(notes, fmt.Sprintf("%d more in %d test files (--tests)", n, f))
	}
	if n, f := total(gen); n > 0 {
		notes = append(notes, fmt.Sprintf("%d more in %d generated files (--gen)", n, f))
	}
	for _, n := range notes {
		fmt.Fprintln(w, "note: "+n)
	}
	return len(shown) > 0, nil
}

func total(hs []*hit) (matches, files int) {
	for _, h := range hs {
		matches += h.count
	}
	return matches, len(hs)
}

func included(globs []string, p string) bool {
	for _, g := range globs {
		if strings.HasPrefix(g, "!") {
			continue
		}
		if ok, _ := path.Match(g, path.Base(p)); ok || ctxmap.Match(g, p) || ctxmap.Match("**/"+g, p) {
			return true
		}
	}
	return false
}

// list resolves the paths of the command line to files: a file is taken as it is, whatever git thinks of it;
// a directory or a glob stands for the files under it that git tracks or does not ignore. No path means the
// directory cwd.
func list(r *repo.Repo, cwd string, paths []string) (files []*hit, missing []string, err error) {
	seen := map[string]bool{}
	add := func(rel, abs string, explicit bool) {
		if !seen[abs] {
			seen[abs] = true
			files = append(files, &hit{path: rel, abs: abs, explicit: explicit})
		}
	}
	var specs []string
	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, p)
		}
		if st, err := os.Stat(abs); err == nil && st.Mode().IsRegular() {
			rel, err := filepath.Rel(r.Root, abs)
			if err != nil || !filepath.IsLocal(rel) {
				rel = p
			}
			add(filepath.ToSlash(rel), abs, true)
			continue
		}
		specs = append(specs, p)
	}
	if len(specs) == 0 && len(paths) > 0 {
		return files, nil, nil
	}
	ls := func(specs ...string) ([]string, error) {
		args := append([]string{"ls-files", "-z", "--full-name", "--cached", "--others", "--exclude-standard", "--"}, specs...)
		out, err := repo.Git(cwd, args...)
		if err != nil {
			return nil, err
		}
		return strings.Split(strings.TrimSuffix(out, "\x00"), "\x00"), nil
	}
	for _, spec := range specs {
		got, err := ls(spec)
		if err != nil || len(got) == 1 && got[0] == "" {
			missing = append(missing, spec)
			continue
		}
		for _, rel := range got {
			add(rel, filepath.Join(r.Root, filepath.FromSlash(rel)), false)
		}
	}
	if len(paths) == 0 {
		got, err := ls()
		if err != nil {
			return nil, nil, err
		}
		for _, rel := range got {
			if rel != "" {
				add(rel, filepath.Join(r.Root, filepath.FromSlash(rel)), false)
			}
		}
	}
	return files, missing, nil
}

// scan fills the matches of every file, reading them in parallel. It keeps the first keep matching lines of
// each file; with only, it counts the matched texts instead.
func scan(files []*hit, re *regexp.Regexp, keep int, only bool) {
	var wg sync.WaitGroup
	ch := make(chan *hit, 256)
	for range max(runtime.NumCPU(), 2) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for h := range ch {
				scanFile(h, re, keep, only)
			}
		}()
	}
	for _, h := range files {
		ch <- h
	}
	close(ch)
	wg.Wait()
}

func scanFile(h *hit, re *regexp.Regexp, keep int, only bool) {
	st, err := os.Lstat(h.abs)
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxFileSize && !h.explicit {
		return
	}
	b, err := os.ReadFile(h.abs)
	if err != nil || bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0 {
		return // unreadable or binary
	}
	locs := re.FindAllIndex(b, -1)
	if len(locs) == 0 {
		return
	}
	line, pos, last := 1, 0, 0
	for _, loc := range locs {
		// \s and [^x] can run over a line end: the match belongs to the line of its first visible character.
		start, end := loc[0], loc[1]
		for start < end && (b[start] == '\n' || b[start] == '\r' || b[start] == ' ' || b[start] == '\t') && bytes.IndexByte(b[start:end], '\n') >= 0 {
			start++
		}
		if i := bytes.IndexByte(b[start:end], '\n'); i >= 0 {
			end = start + i
		}
		line += bytes.Count(b[pos:start], []byte{'\n'})
		pos = start
		if only {
			if h.only == nil {
				h.only = map[string]int{}
			}
			h.only[string(b[start:end])]++
		}
		if line == last {
			continue
		}
		last = line
		h.count++
		if len(h.lines) < keep {
			h.lines = append(h.lines, line)
		}
	}
}

// printer writes the result within the limits of o.
type printer struct {
	w    io.Writer
	o    Options
	re   *regexp.Regexp
	root string
	used int
}

func (p *printer) printf(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	p.used += len(s)
	_, _ = io.WriteString(p.w, s)
}

const lineWidth = 180

// excerpt cuts a long line to the part around its first match.
func (p *printer) excerpt(line string) string {
	line = strings.TrimRight(line, "\r\n")
	trimmed := strings.TrimLeft(line, " \t")
	if p.o.Context > 0 {
		// With context the lines are read as code: keep the indentation, a tab as two spaces.
		trimmed = strings.Repeat("  ", strings.Count(line[:len(line)-len(trimmed)], "\t")) +
			strings.ReplaceAll(line[:len(line)-len(trimmed)], "\t", "") + trimmed
	}
	if utf8.RuneCountInString(trimmed) <= lineWidth {
		return trimmed
	}
	start := 0
	if loc := p.re.FindStringIndex(trimmed); loc != nil {
		before := utf8.RuneCountInString(trimmed[:loc[0]])
		if before > lineWidth*2/3 {
			start = before - lineWidth/3
		}
	}
	r := []rune(trimmed)
	end := min(start+lineWidth, len(r))
	s := string(r[start:end])
	if start > 0 {
		s = "…" + s
	}
	if end < len(r) {
		s += "…"
	}
	return s
}

// where names the innermost declaration or section of each line of a Go, TypeScript or Markdown file.
func where(p string, src []byte) func(line int) string {
	switch extract.Kind(p) {
	case "go", "ts", "md":
	default:
		return func(int) string { return "" }
	}
	nodes := extract.Extract(p, src).Nodes
	return func(line int) string {
		best := -1
		for i, n := range nodes {
			if n.Kind == "file" || n.Start > line || n.End < line || n.End == n.Start {
				continue
			}
			if strings.HasSuffix(p, ".md") && !strings.HasPrefix(n.ID, "doc:") {
				continue
			}
			if best < 0 || n.End-n.Start < nodes[best].End-nodes[best].Start {
				best = i
			}
		}
		if best < 0 {
			return ""
		}
		n := nodes[best]
		name := n.Name
		switch {
		case strings.HasPrefix(n.ID, "doc:"):
			name = "#" + n.Name
		case n.Meta != "" && strings.HasPrefix(n.ID, "sym:"):
			name = n.Meta
		}
		return fmt.Sprintf("%s %d-%d", name, n.Start, n.End)
	}
}

func (p *printer) matches(shown []*hit) {
	matches, _ := total(shown)
	perFile := p.o.Limit
	if len(shown) > 1 {
		perFile = max(3, p.o.Limit/4)
	}
	printed, printedFiles := 0, 0
	var left []*hit
	for _, h := range shown {
		if printed >= p.o.Limit || p.used >= p.o.Max {
			left = append(left, h)
			continue
		}
		b, err := os.ReadFile(h.abs)
		if err != nil {
			continue
		}
		lines := strings.SplitAfter(string(b), "\n")
		in := where(h.path, b)
		if h.count > 1 {
			p.printf("%s (%d)\n", h.path, h.count)
		} else {
			p.printf("%s\n", h.path)
		}
		printedFiles++
		n, lastWhere, lastLine := 0, "", 0
		for _, ln := range h.lines {
			if n >= perFile || printed >= p.o.Limit || p.used >= p.o.Max {
				break
			}
			if ln > len(lines) {
				continue
			}
			from := max(ln-p.o.Context, lastLine+1, 1)
			if p.o.Context > 0 && lastLine > 0 && from > lastLine+1 {
				p.printf("  --\n")
			}
			for c := from; c < ln; c++ {
				p.printf("  %d- %s\n", c, p.excerpt(lines[c-1]))
			}
			suffix := ""
			if wh := in(ln); wh != "" && wh != lastWhere {
				suffix, lastWhere = "  ["+wh+"]", wh
			}
			p.printf("  %d: %s%s\n", ln, p.excerpt(lines[ln-1]), suffix)
			lastLine = ln
			for c := ln + 1; c <= min(ln+p.o.Context, len(lines)); c++ {
				if isMatch(h.lines, c) {
					break
				}
				p.printf("  %d- %s\n", c, p.excerpt(lines[c-1]))
				lastLine = c
			}
			n++
			printed++
		}
		if rest := h.count - n; rest > 0 {
			p.printf("  … %d more here\n", rest)
		}
	}
	if printed == matches {
		if printedFiles > 1 {
			fmt.Fprintf(p.w, "%d matches in %d files\n", matches, len(shown))
		}
		return
	}
	fmt.Fprintf(p.w, "%d of %d matches in %d files shown", printed, matches, len(shown))
	if len(left) > 0 {
		sort.SliceStable(left, func(i, j int) bool { return left[i].count > left[j].count })
		var names []string
		for _, h := range left[:min(len(left), 8)] {
			names = append(names, fmt.Sprintf("%s (%d)", h.path, h.count))
		}
		more := ""
		if len(left) > 8 {
			more = fmt.Sprintf(", … %d more files", len(left)-8)
		}
		fmt.Fprintf(p.w, "; not opened: %s%s", strings.Join(names, ", "), more)
	}
	fmt.Fprintln(p.w, ". Narrow with a path, --kind or a longer pattern; -l lists the files; -n and --max raise the limits.")
}

func isMatch(lines []int, ln int) bool {
	i := sort.SearchInts(lines, ln)
	return i < len(lines) && lines[i] == ln
}

func (p *printer) list(shown []*hit) {
	sort.SliceStable(shown, func(i, j int) bool { return shown[i].count > shown[j].count })
	limit := max(p.o.Limit, 40) * 2
	for i, h := range shown {
		if i >= limit || p.used >= p.o.Max {
			fmt.Fprintf(p.w, "… %d more files\n", len(shown)-i)
			break
		}
		p.printf("%s (%d)\n", h.path, h.count)
	}
	matches, _ := total(shown)
	fmt.Fprintf(p.w, "%d matching lines in %d files\n", matches, len(shown))
}

func (p *printer) only(shown []*hit) {
	counts := map[string]int{}
	for _, h := range shown {
		for s, n := range h.only {
			counts[s] += n
		}
	}
	keys := text.SortedKeys(counts)
	sort.SliceStable(keys, func(i, j int) bool { return counts[keys[i]] > counts[keys[j]] })
	for i, s := range keys {
		if i >= p.o.Limit || p.used >= p.o.Max {
			fmt.Fprintf(p.w, "… %d more distinct texts\n", len(keys)-i)
			break
		}
		p.printf("%6d  %s\n", counts[s], text.Truncate(s, lineWidth))
	}
	fmt.Fprintf(p.w, "%d distinct texts in %d files\n", len(keys), len(shown))
}
