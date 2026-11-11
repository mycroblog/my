// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// denial is why a Bash command is refused: the rule that matched and the reason shown to the agent, which names
// the command to run instead.
type denial struct {
	rule   string
	reason string
}

// checker evaluates one Bash command. Paths resolve against the session's working directory and the `cd`s of
// the command; a path counts as the repository's only when it lies in a working tree of this project (one with
// scripts/agentkit.sh) and git does not ignore it.
type checker struct {
	session string // the working directory of the session
	// missingIsRepo treats a path that does not exist as a file of the repository unless its name says otherwise:
	// for evaluating commands recorded in working trees that are gone. A live hook leaves such a path alone.
	missingIsRepo bool
	roots         map[string]string
	listed        map[string]bool
}

// target is a path of the repository named by a command.
type target struct {
	root string // the working tree
	rel  string // slash-separated, relative to root
	abs  string
}

// skipDirs are directories whose files are never the repository's sources: scratch space, dependencies, build
// output, test reports.
var skipDirs = map[string]bool{
	".tmp": true, "node_modules": true, ".git": true, "dist": true, "coverage": true, "test-results": true,
	"playwright-report": true, ".cache": true, "tmp": true, "bin": true,
}

func (c *checker) rootOf(dir string) string {
	if c.roots == nil {
		c.roots = map[string]string{}
	}
	if r, ok := c.roots[dir]; ok {
		return r
	}
	root := ""
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "scripts", "agentkit.sh")); err == nil {
			if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
				root = d
				break
			}
		}
		if d == filepath.Dir(d) {
			break
		}
	}
	c.roots[dir] = root
	return root
}

// locate resolves the word w of a command run in cwd to a path of the repository, or reports false: a value the
// shell computes, a path outside every working tree, scratch space, a log, or a file git ignores.
func (c *checker) locate(cwd string, w word) (target, bool) {
	p := strings.TrimRight(w.text, ")")
	if w.dyn || p == "" || p == "-" || cwd == "" && !filepath.IsAbs(p) {
		return target{}, false
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return target{}, false
		}
		p = filepath.Join(home, p[2:])
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	p = filepath.Clean(p)
	// The directory of a glob is the part before its first pattern character.
	dir := p
	if i := strings.IndexAny(dir, "*?["); w.glob && i >= 0 {
		dir = filepath.Dir(dir[:i] + "x")
	}
	root := c.rootOf(dir)
	if root == "" {
		// The path may not exist (yet): look from its nearest existing ancestor.
		for d := filepath.Dir(dir); root == "" && d != filepath.Dir(d); d = filepath.Dir(d) {
			if _, err := os.Stat(d); err == nil {
				root = c.rootOf(d)
				break
			}
		}
		if root == "" {
			return target{}, false
		}
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return target{}, false
	}
	rel = filepath.ToSlash(rel)
	for _, seg := range strings.Split(rel, "/") {
		if skipDirs[seg] {
			return target{}, false
		}
	}
	switch filepath.Ext(rel) {
	case ".log", ".out", ".output", ".txt", ".jsonl", ".png", ".jpg", ".webp", ".zip", ".gz":
		// Logs, captured output and binaries are never read through agentkit; .txt in the repository is rare
		// and small.
		return target{}, false
	}
	t := target{root: root, rel: rel, abs: p}
	if c.lists(root, rel) {
		return t, true
	}
	if c.missingIsRepo {
		if _, err := os.Lstat(dir); err != nil {
			return t, true
		}
	}
	return target{}, false
}

// lists reports whether git knows a file at (or under, or matching) rel that it does not ignore.
func (c *checker) lists(root, rel string) bool {
	key := root + "\x00" + rel
	if v, ok := c.listed[key]; ok {
		return v
	}
	if c.listed == nil {
		c.listed = map[string]bool{}
	}
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "--", rel)
	cmd.Dir = root
	out, err := cmd.Output()
	v := err == nil && len(out) > 0
	c.listed[key] = v
	return v
}

// agentkit is how the session runs agentkit for the working tree root.
func (c *checker) agentkit(root string) string {
	if root == c.session || root == "" {
		return "scripts/agentkit.sh"
	}
	if rel, err := filepath.Rel(c.session, root); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel) + "/scripts/agentkit.sh"
	}
	return root + "/scripts/agentkit.sh"
}

// show is how the session names the path of t on a command line: relative to its working directory when the
// path is under it.
func (c *checker) show(t target) string {
	if rel, err := filepath.Rel(c.session, t.abs); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return t.abs
}

func quote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_-./:=@%+,", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// check returns why cmd is denied, or nil.
func (c *checker) check(cmd string) *denial {
	return c.checkIn(c.session, cmd, 0)
}

func (c *checker) checkIn(cwd, cmd string, depth int) *denial {
	segs := splitCommand(cmd)
	loop, sleeps := false, false
	vars := map[string]string{}
	for i, seg := range segs {
		pc := parseCommand(seg.text, vars)
		if (pc.has("until") || pc.has("while")) && pc.tool != "read" {
			loop = true
		}
		var d *denial
		switch pc.tool {
		case "export", "local", "readonly", "declare":
			for _, a := range pc.args {
				if reAssign.MatchString(a.text) {
					assign(vars, a)
				}
			}
		case "read", "for":
			// The loop variable changes on every turn.
			for _, a := range pc.args {
				if a.text == "in" {
					break
				}
				delete(vars, a.text)
			}
		case "cd", "pushd":
			cwd = chdir(cwd, pc.args)
		case "sleep":
			sleeps = true
		case "pgrep":
			return &denial{"wait-loop", waitReason}
		case "bash", "sh", "zsh":
			if depth < 3 {
				for j := 0; j+1 < len(pc.args); j++ {
					if a := pc.args[j].text; strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.HasSuffix(a, "c") {
						d = c.checkIn(cwd, pc.args[j+1].text, depth+1)
						break
					}
				}
			}
		case "grep", "egrep", "fgrep", "rg", "ag":
			d = c.search(cwd, pc, seg)
		case "git":
			d = c.git(cwd, pc, seg)
		case "sed":
			d = c.sed(cwd, pc, seg)
		case "awk", "gawk":
			d = c.awk(cwd, pc, seg)
		case "head", "tail":
			d = c.headTail(cwd, pc, seg)
		case "cat":
			var next *segment
			if seg.pipedOut && i+1 < len(segs) {
				next = &segs[i+1]
			}
			d = c.cat(cwd, pc, seg, next)
		case "perl":
			d = c.perl(cwd, pc)
		case "python3", "python", "node", "bun", "ruby", "deno":
			d = c.script(cwd, pc, seg)
		case "gh":
			d = ci(pc)
		}
		if d != nil {
			return d
		}
	}
	if loop && sleeps {
		return &denial{"wait-loop", waitReason}
	}
	return nil
}

const waitReason = "Polling for a background command (an until / while loop that sleeps, or pgrep) hangs or ends " +
	"early: `pgrep -f` matches the polling shell's own command line. Run the long command with the Bash tool's " +
	"run_in_background and wait for its completion notification; wait for CI with `scripts/agentkit.sh pr wait <pr>` " +
	"in the background, and for many PRs with `scripts/agentkit.sh pr watch` and the Monitor tool."

// chdir is the working directory after `cd args`, or "" when it cannot be known.
func chdir(cwd string, args []word) string {
	if len(args) == 0 || args[0].dyn || args[0].glob || args[0].text == "-" {
		return ""
	}
	p := strings.TrimRight(args[0].text, ")")
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	if cwd == "" {
		return ""
	}
	return filepath.Join(cwd, p)
}

// ---- searching -------------------------------------------------------------------------------------------

type grepCall struct {
	pattern    word
	hasPattern bool
	targets    []word
	revs       bool // git grep of a revision
	recursive  bool
	flags      []string // the flags agentkit grep takes too
	context    int
	quiet      bool
	count      bool
	unsupp     bool // -v, -L, -z, -f: agentkit grep has no equivalent
	fromFile   bool
}

var (
	grepValueShort = map[byte]bool{'e': true, 'f': true, 'A': true, 'B': true, 'C': true, 'm': true, 'd': true, 'D': true}
	rgValueShort   = map[byte]bool{'e': true, 'f': true, 'A': true, 'B': true, 'C': true, 'm': true, 'g': true, 't': true, 'T': true, 'j': true, 'M': true, 'r': true}
	grepValueLong  = map[string]bool{
		"--include": true, "--exclude": true, "--exclude-dir": true, "--glob": true, "--type": true, "--type-not": true,
		"--max-count": true, "--max-depth": true, "--context": true, "--after-context": true, "--before-context": true,
		"--regexp": true, "--file": true, "--color": true, "--colour": true, "--threads": true, "--max-columns": true,
		"--iglob": true, "--replace": true, "--sort": true, "--sortr": true,
	}
	kindOfExt = map[string]string{
		"go": "go", "ts": "ts", "tsx": "ts", "md": "md", "sql": "sql", "yaml": "yaml", "yml": "yaml", "json": "json",
	}
	reIncludeExt = regexp.MustCompile(`^\*\.(\w+)$`)
)

// parseGrep reads the arguments of grep, rg, ag, or `git grep` (git says which).
func parseGrep(tool string, args []word, git bool) grepCall {
	var g grepCall
	rg := tool == "rg" || tool == "ag"
	g.recursive = rg || git
	if tool == "fgrep" {
		g.flags = append(g.flags, "-F")
	}
	valueShort := grepValueShort
	if rg {
		valueShort = rgValueShort
	}
	value := func(name, v string) {
		switch name {
		case "e", "--regexp":
			if !g.hasPattern {
				g.pattern, g.hasPattern = word{text: v}, true
			} else {
				g.pattern.text += "|" + v
			}
		case "f", "--file":
			g.unsupp = true
		case "A", "B", "C", "--context", "--after-context", "--before-context":
			if n, err := strconv.Atoi(v); err == nil && n > g.context {
				g.context = n
			}
		case "--include", "g", "--glob":
			if m := reIncludeExt.FindStringSubmatch(v); m != nil && kindOfExt[m[1]] != "" {
				g.flags = append(g.flags, "--kind "+kindOfExt[m[1]])
			} else if !strings.HasPrefix(v, "!") {
				g.flags = append(g.flags, "--include "+quote(v))
			}
		case "t", "--type":
			if k := kindOfExt[v]; k != "" {
				g.flags = append(g.flags, "--kind "+k)
			}
		}
	}
	var pos []word
	dashdash := -1
	for i := 0; i < len(args); i++ {
		a := args[i]
		t := a.text
		switch {
		case dashdash >= 0 || t == "-" || !strings.HasPrefix(t, "-"):
			pos = append(pos, a)
		case t == "--":
			dashdash = len(pos)
		case strings.HasPrefix(t, "--"):
			name, v, eq := strings.Cut(t, "=")
			switch name {
			case "--recursive", "--dereference-recursive":
				g.recursive = true
			case "--ignore-case":
				g.flags = append(g.flags, "-i")
			case "--word-regexp":
				g.flags = append(g.flags, "-w")
			case "--fixed-strings":
				g.flags = append(g.flags, "-F")
			case "--files-with-matches":
				g.flags = append(g.flags, "-l")
			case "--only-matching":
				g.flags = append(g.flags, "-o")
			case "--count":
				g.count = true
			case "--quiet", "--silent":
				g.quiet = true
			case "--invert-match", "--files-without-match", "--null-data", "--files", "--null", "--json":
				g.unsupp = true
			}
			if grepValueLong[name] {
				if !eq && i+1 < len(args) {
					i++
					v = args[i].text
				}
				value(name, v)
			}
		default:
			for j := 1; j < len(t); j++ {
				ch := t[j]
				if valueShort[ch] {
					v := t[j+1:]
					if v == "" && i+1 < len(args) {
						i++
						v = args[i].text
						if ch == 'e' {
							g.pattern.dyn = g.pattern.dyn || args[i].dyn
						}
					}
					value(string(ch), v)
					break
				}
				switch ch {
				case 'r', 'R':
					g.recursive = true
				case 'i', 'w', 'F', 'l', 'o':
					g.flags = append(g.flags, "-"+string(ch))
				case 'c':
					g.count = true
				case 'q':
					g.quiet = true
				case 'v', 'L', 'z', 'Z':
					g.unsupp = true
				default:
					if ch >= '0' && ch <= '9' { // -3: three lines of context
						if n, err := strconv.Atoi(t[j:]); err == nil && n > g.context {
							g.context = n
						}
						j = len(t)
					}
				}
			}
		}
	}
	if !g.hasPattern && len(pos) > 0 {
		g.pattern, g.hasPattern = pos[0], true
		pos = pos[1:]
		if dashdash > 0 {
			dashdash--
		}
	}
	if git && dashdash > 0 {
		g.revs = true // git grep <pattern> <rev> -- <paths>
		pos = pos[dashdash:]
	}
	g.targets = pos
	return g
}

var reDecl = regexp.MustCompile(`^\^?(?:func|type|function|class|interface|const|var|export\s+(?:default\s+)?(?:async\s+)?(?:function|const|class|interface|type|enum))\s+(?:\([^)]*\)\s*)?(?:\\b)?([A-Za-z_]\w*)(?:\\b|\\?\(|\\s|\s|\[)*$`)

func (c *checker) search(cwd string, pc command, seg segment) *denial {
	if seg.pipedIn || pc.has("xargs") {
		return nil // a filter on another command's output
	}
	return c.searchCall(cwd, parseGrep(pc.tool, pc.args, false))
}

func (c *checker) searchCall(cwd string, g grepCall) *denial {
	if !g.hasPattern || g.quiet || g.unsupp || g.revs || g.count && !g.recursive {
		return nil
	}
	targets := g.targets
	if len(targets) == 0 {
		if !g.recursive {
			return nil // reads standard input
		}
		targets = []word{{text: "."}}
	}
	var hit []target
	for _, w := range targets {
		if t, ok := c.locate(cwd, w); ok {
			hit = append(hit, t)
		}
	}
	if len(hit) == 0 {
		return nil
	}
	ak := c.agentkit(hit[0].root)
	parts := []string{ak, "grep"}
	seen := map[string]bool{}
	for _, f := range g.flags {
		if !seen[f] {
			seen[f] = true
			parts = append(parts, f)
		}
	}
	if g.count {
		parts = append(parts, "-l")
	}
	if g.context > 0 {
		parts = append(parts, "-C", strconv.Itoa(g.context))
	}
	pat := quote(g.pattern.text)
	if g.pattern.dyn {
		pat = `"` + g.pattern.text + `"`
	}
	if strings.HasPrefix(g.pattern.text, "-") {
		parts = append(parts, "-e")
	}
	parts = append(parts, pat)
	for _, t := range hit {
		if p := c.show(t); p != "." {
			parts = append(parts, quote(p))
		}
	}
	reason := "Search the repository with `" + strings.Join(parts, " ") + "`: it reads the tracked files, leaves out " +
		"generated files and tests (--tests adds them), and prints a bounded list grouped by file with the " +
		"enclosing declaration of each match."
	if m := reDecl.FindStringSubmatch(g.pattern.text); m != nil {
		show := "`" + ak + " show " + m[1] + "` prints that declaration whole"
		if g.context > 0 {
			reason = "Read a declaration with " + show + ". To search instead: `" + strings.Join(parts, " ") + "`."
		} else {
			reason += " To read it: " + show + "."
		}
	}
	return &denial{"search", reason}
}

// ---- git -------------------------------------------------------------------------------------------------

func (c *checker) git(cwd string, pc command, _ segment) *denial {
	args := pc.args
	for len(args) > 0 {
		t := args[0].text
		switch {
		case t == "-C" && len(args) > 1:
			cwd = chdir(cwd, args[1:2])
			args = args[2:]
		case (t == "-c" || t == "--git-dir" || t == "--work-tree") && len(args) > 1:
			args = args[2:]
		case strings.HasPrefix(t, "-"):
			args = args[1:]
		default:
			goto sub
		}
	}
	return nil
sub:
	sub, rest := args[0].text, args[1:]
	has := func(names ...string) bool {
		for _, a := range rest {
			for _, n := range names {
				if a.text == n || strings.HasPrefix(a.text, n+"=") {
					return true
				}
			}
		}
		return false
	}
	switch sub {
	case "grep":
		if pc.has("xargs") {
			return nil
		}
		g := parseGrep("grep", rest, true)
		// Without `--`, a word after the pattern that is not a path is a revision.
		if !g.revs {
			for _, w := range g.targets {
				if w.dyn || w.glob || strings.HasPrefix(w.text, ":") {
					continue
				}
				if cwd == "" {
					return nil
				}
				if _, err := os.Lstat(filepath.Join(cwd, w.text)); err != nil && !c.missingIsRepo {
					return nil
				}
				if c.missingIsRepo && reRev.MatchString(w.text) {
					return nil
				}
			}
		}
		var paths []word
		for _, w := range g.targets {
			if !strings.HasPrefix(w.text, ":") { // :!exclude and other pathspec magic
				paths = append(paths, w)
			}
		}
		g.targets = paths
		return c.searchCall(cwd, g)
	case "merge":
		if has("--ff-only", "--abort", "--continue", "--quit") {
			return nil
		}
		for _, a := range rest {
			if isMain(a.text) {
				return &denial{"merge-main", rebaseReason}
			}
		}
	case "pull":
		if has("--rebase", "-r", "--ff-only") {
			return nil
		}
		return &denial{"merge-main", "`git pull` makes a merge commit when the branch has its own commits. " + rebaseReason +
			" On a checkout of main itself, `git pull --ff-only`."}
	}
	return nil
}

var reRev = regexp.MustCompile(`^([0-9a-f]{7,40}|HEAD.*|origin/.*|main|v\d.*|\$.*)$`)

func isMain(ref string) bool {
	switch ref {
	case "main", "origin/main", "upstream/main", "refs/heads/main", "refs/remotes/origin/main":
		return true
	}
	return false
}

const rebaseReason = "Bring a branch up to date by rebasing, not by merging main into it: " +
	"`git fetch origin && git rebase origin/main`, resolve conflicts in generated files with `scripts/agentkit.sh regen` " +
	"(then `git add` them and `git rebase --continue`), and push with `git push --force-with-lease`."

// ---- reading ---------------------------------------------------------------------------------------------

var reSedPrint = regexp.MustCompile(`^\s*(\d+)\s*(?:,\s*(\+?)(\d+|\$))?\s*([pq])\s*$`)

func (c *checker) readReason(t target, from, to int) string {
	ak, p := c.agentkit(t.root), c.show(t)
	rng := "<from>-<to>"
	read := "offset and limit"
	if from > 0 {
		if to >= from {
			rng = fmt.Sprintf("%d-%d", from, to)
			read = fmt.Sprintf("offset %d and limit %d", from, to-from+1)
		} else {
			rng = fmt.Sprintf("%d-<to>", from)
			read = fmt.Sprintf("offset %d", from)
		}
	}
	return fmt.Sprintf("Read part of a file with the Read tool (file_path %s, %s) or `%s show %s:%s`; "+
		"`%s outline %s` lists its declarations or sections with their line ranges, and `%s show <Name>` prints one declaration.",
		t.abs, read, ak, p, rng, ak, p, ak)
}

func (c *checker) sed(cwd string, pc command, seg segment) *denial {
	var scripts, files []word
	quiet, inPlace := false, false
	for i := 0; i < len(pc.args); i++ {
		t := pc.args[i].text
		switch {
		case t == "--in-place" || strings.HasPrefix(t, "--in-place="):
			inPlace = true
		case t == "--quiet" || t == "--silent":
			quiet = true
		case t == "-e" || t == "--expression":
			if i+1 < len(pc.args) {
				i++
				scripts = append(scripts, pc.args[i])
			}
		case t == "-f":
			i++
		case strings.HasPrefix(t, "--"):
		case strings.HasPrefix(t, "-") && len(t) > 1:
			for j := 1; j < len(t); j++ {
				switch t[j] {
				case 'n':
					quiet = true
				case 'i':
					inPlace = true
					j = len(t) // -i.bak
				case 'e':
					if i+1 < len(pc.args) {
						i++
						scripts = append(scripts, pc.args[i])
					}
				}
			}
		case len(scripts) == 0:
			scripts = append(scripts, pc.args[i])
		default:
			files = append(files, pc.args[i])
		}
	}
	if inPlace {
		if pc.has("xargs") {
			return nil
		}
		for _, f := range files {
			if t, ok := c.locate(cwd, f); ok {
				return c.writeDenial(t, "sed -i")
			}
		}
		return nil
	}
	if !quiet || seg.pipedIn || len(files) == 0 || len(scripts) == 0 {
		return nil
	}
	from, to := 0, 0
	for _, s := range scripts {
		for _, part := range strings.Split(s.text, ";") {
			if strings.TrimSpace(part) == "" {
				continue
			}
			m := reSedPrint.FindStringSubmatch(part)
			if m == nil {
				return nil // not a plain line range
			}
			if m[4] != "p" || from > 0 {
				continue
			}
			from, _ = strconv.Atoi(m[1])
			to = from
			if n, err := strconv.Atoi(m[3]); err == nil {
				to = n
				if m[2] == "+" {
					to = from + n
				}
			} else if m[3] == "$" {
				to = 0
			}
		}
	}
	if from == 0 {
		return nil
	}
	for _, f := range files {
		if t, ok := c.locate(cwd, f); ok {
			return &denial{"read-range", c.readReason(t, from, to)}
		}
	}
	return nil
}

var reAwkRange = regexp.MustCompile(`^\s*NR\s*(>=?|==)\s*(\d+)\s*(?:&&\s*NR\s*(<=?)\s*(\d+)\s*)?(?:\{\s*print\s*(?:\$0)?\s*;?\s*\})?\s*$`)

func (c *checker) awk(cwd string, pc command, seg segment) *denial {
	if seg.pipedIn || len(pc.args) < 2 || strings.HasPrefix(pc.args[0].text, "-") {
		return nil
	}
	m := reAwkRange.FindStringSubmatch(pc.args[0].text)
	if m == nil {
		return nil
	}
	from, _ := strconv.Atoi(m[2])
	to, _ := strconv.Atoi(m[4])
	if m[1] == "==" {
		to = from
	}
	for _, f := range pc.args[1:] {
		if t, ok := c.locate(cwd, f); ok {
			return &denial{"read-range", c.readReason(t, from, to)}
		}
	}
	return nil
}

func (c *checker) headTail(cwd string, pc command, seg segment) *denial {
	if seg.pipedIn || pc.has("xargs") {
		return nil
	}
	n, fromStart := 10, false
	var files []word
	for i := 0; i < len(pc.args); i++ {
		t := pc.args[i].text
		switch {
		case t == "-f" || t == "-F" || t == "--follow" || strings.HasPrefix(t, "-c") || strings.HasPrefix(t, "--bytes"):
			return nil
		case t == "-n" || t == "--lines":
			if i+1 < len(pc.args) {
				i++
				v := pc.args[i].text
				fromStart = strings.HasPrefix(v, "+")
				n, _ = strconv.Atoi(strings.TrimLeft(v, "+-"))
			}
		case strings.HasPrefix(t, "--lines="):
			n, _ = strconv.Atoi(strings.TrimLeft(strings.TrimPrefix(t, "--lines="), "+-"))
		case strings.HasPrefix(t, "-n"):
			fromStart = strings.HasPrefix(t[2:], "+")
			n, _ = strconv.Atoi(strings.TrimLeft(t[2:], "+-"))
		case strings.HasPrefix(t, "-") && len(t) > 1:
			if v, err := strconv.Atoi(t[1:]); err == nil {
				n = v
			}
		case strings.HasPrefix(t, "+") && pc.tool == "tail":
			fromStart = true
			n, _ = strconv.Atoi(t[1:])
		default:
			files = append(files, pc.args[i])
		}
	}
	for _, f := range files {
		t, ok := c.locate(cwd, f)
		if !ok {
			continue
		}
		if st, err := os.Stat(t.abs); err == nil && st.IsDir() {
			continue
		}
		switch {
		case pc.tool == "head":
			return &denial{"read-range", c.readReason(t, 1, n)}
		case fromStart:
			return &denial{"read-range", c.readReason(t, n, 0)}
		}
		return &denial{"read-range", c.readReason(t, 0, 0)}
	}
	return nil
}

func (c *checker) cat(cwd string, pc command, seg segment, next *segment) *denial {
	if seg.pipedIn || pc.has("xargs") {
		return nil
	}
	var files []target
	for _, a := range pc.args {
		if strings.HasPrefix(a.text, "-") {
			continue
		}
		if t, ok := c.locate(cwd, a); ok {
			if st, err := os.Stat(t.abs); err == nil && st.IsDir() {
				continue
			}
			files = append(files, t)
		}
	}
	if len(files) == 0 {
		return nil
	}
	if !seg.pipedOut {
		if len(pc.writes) > 0 {
			return nil
		}
		for _, t := range files {
			if reason := bigDoc(t, c.agentkit(t.root), c.show(t)); reason != "" {
				return &denial{"big-file", reason}
			}
		}
		return nil
	}
	t := files[0]
	ak, p := c.agentkit(t.root), c.show(t)
	if next != nil {
		nc := parseCommand(next.text, nil)
		switch nc.tool {
		case "grep", "egrep", "fgrep", "rg":
			g := parseGrep(nc.tool, nc.args, false)
			if g.quiet || g.unsupp || g.count {
				return nil
			}
			if g.hasPattern && len(g.targets) == 0 {
				g.targets = []word{{text: t.abs}}
				if d := c.searchCall(cwd, g); d != nil {
					return d
				}
			}
		case "head", "tail", "sed":
			return &denial{"read-range", "`cat <file> |` copies the whole file through a pipe. " + c.readReason(t, 0, 0)}
		}
	}
	return &denial{"read-range", fmt.Sprintf("`cat <file> |` copies the whole file through a pipe: give %s to the next command as "+
		"an argument (`jq . %s`, `wc -l %s`), search it with `%s grep <pattern> %s`, or read it with the Read tool.", p, p, p, ak, p)}
}

// ---- writing ---------------------------------------------------------------------------------------------

func (c *checker) writeDenial(t target, how string) *denial {
	p := c.show(t)
	if strings.HasPrefix(t.rel, "locales/") {
		return &denial{"write", fmt.Sprintf("Message catalogs are not edited by hand or with %s: add, rename and remove messages with "+
			"`scripts/i18n.sh add|mv|rm` (they keep every locale of %s in step), then `scripts/task.sh i18n:gen`.", how, t.rel)}
	}
	return &denial{"write", fmt.Sprintf("Change %s with the Edit tool (old_string / new_string; replace_all for every occurrence), not "+
		"%s: the edit is checked against the file's current text and shown as a diff. Write a new file with the Write tool.", p, how)}
}

func (c *checker) perl(cwd string, pc command) *denial {
	if pc.has("xargs") {
		return nil
	}
	inPlace, script := false, false
	var files []word
	for i := 0; i < len(pc.args); i++ {
		t := pc.args[i].text
		switch {
		case strings.HasPrefix(t, "-") && !strings.HasPrefix(t, "--") && len(t) > 1:
			flags := t[1:]
			if j := strings.IndexByte(flags, 'i'); j >= 0 && !strings.ContainsAny(flags[:j], "eEIM") {
				inPlace = true
			}
			if strings.ContainsAny(flags, "eE") && strings.IndexAny(flags, "eE") == len(flags)-1 {
				i++
				script = true
			}
		case !script:
			script = true
		default:
			files = append(files, pc.args[i])
		}
	}
	if !inPlace {
		return nil
	}
	for _, f := range files {
		if t, ok := c.locate(cwd, f); ok {
			return c.writeDenial(t, "perl -pi")
		}
	}
	return nil
}

var (
	// A write whose target is a string literal, in Python, JavaScript and Ruby.
	reWriteLit = regexp.MustCompile("\\bopen\\(\\s*f?[\"']([^\"'\\n]+)[\"']\\s*,\\s*[\"'][wa]" +
		"|(?:writeFileSync|writeFile|Bun\\.write|File\\.write)\\(\\s*[\"'`]([^\"'`\\n]+)[\"'`]" +
		"|Path\\(\\s*f?[\"']([^\"'\\n]+)[\"']\\s*\\)\\.write_(?:text|bytes)\\(")
	// Any write.
	reWriteAny = regexp.MustCompile(`\bopen\([^()\n]*,\s*["'][wa]\+?b?["']|\.write_(?:text|bytes)\(|writeFileSync\(|\bwriteFile\(|Bun\.write\(|File\.write\(`)
	// A string that looks like a path; the {parts} of an f-string or a template stand for anything.
	rePathLit = regexp.MustCompile("[\"'`]([\\w@.~/*{}$-]+\\.\\w{1,6})[\"'`]")
	reSubst   = regexp.MustCompile(`\$?\{[^{}]*\}`)
)

func (c *checker) script(cwd string, pc command, seg segment) *denial {
	code := seg.heredoc
	for i := 0; i+1 < len(pc.args); i++ {
		if t := pc.args[i].text; t == "-c" || t == "-e" || t == "--eval" || t == "-p" {
			code = pc.args[i+1].text
			break
		}
	}
	if code == "" || !reWriteAny.MatchString(code) {
		return nil
	}
	how := "a " + pc.tool + " script"
	file := func(s string) (target, bool) {
		w := word{text: reSubst.ReplaceAllString(s, "*")}
		w.glob = strings.ContainsAny(w.text, "*?[")
		if strings.Contains(w.text, "$") {
			return target{}, false
		}
		t, ok := c.locate(cwd, w)
		return t, ok && (w.glob || c.isFile(t))
	}
	lits := 0
	for _, m := range reWriteLit.FindAllStringSubmatch(code, -1) {
		lits++
		if t, ok := file(m[1] + m[2] + m[3]); ok {
			return c.writeDenial(t, how)
		}
	}
	if len(reWriteAny.FindAllString(code, -1)) <= lits {
		return nil // every write names its target, and none is the repository's
	}
	// A write to a variable: the files the script names, or is given as arguments, are what it rewrites.
	for _, m := range rePathLit.FindAllStringSubmatch(code, -1) {
		if t, ok := file(m[1]); ok {
			return c.writeDenial(t, how)
		}
	}
	for _, a := range pc.args {
		if strings.HasPrefix(a.text, "-") || a.text == code || a.text == "/dev/stdin" {
			continue
		}
		if t, ok := c.locate(cwd, a); ok && c.isFile(t) {
			return c.writeDenial(t, how)
		}
	}
	return nil
}

// isFile reports whether t is an existing file (or, when missing paths count, not a known directory).
func (c *checker) isFile(t target) bool {
	st, err := os.Stat(t.abs)
	if err != nil {
		return c.missingIsRepo
	}
	return !st.IsDir()
}

// ---- CI --------------------------------------------------------------------------------------------------

const ciReason = "Polling CI prints its status into your context on every call. Run `scripts/agentkit.sh pr wait <pr>` " +
	"in the background (Bash run_in_background): it prints one line when CI passes, or the failing lines of the failed " +
	"jobs (exit 3 on a conflict). After it reports a failure, `gh run view <run> --log-failed` reads more of the log. " +
	"An orchestrator watching many PRs uses `scripts/agentkit.sh pr watch` with the Monitor tool."

func ci(pc command) *denial {
	var pos []string
	text := ""
	for _, a := range pc.args {
		text += " " + a.text
		if !strings.HasPrefix(a.text, "-") {
			pos = append(pos, a.text)
		}
	}
	if len(pos) < 2 {
		return nil
	}
	has := func(names ...string) bool {
		for _, a := range pc.args {
			for _, n := range names {
				if a.text == n || strings.HasPrefix(a.text, n+"=") {
					return true
				}
			}
		}
		return false
	}
	deny := &denial{"ci-poll", ciReason}
	switch pos[0] + " " + pos[1] {
	case "pr checks", "run watch":
		return deny
	case "run view":
		if !has("--log-failed", "--log") {
			return deny
		}
	case "run list":
		if !has("--workflow", "-w") {
			return deny
		}
	case "pr view", "pr list":
		if strings.Contains(text, "statusCheckRollup") {
			return deny
		}
	}
	if pos[0] == "api" && (strings.Contains(text, "/check-runs") || strings.Contains(text, "/check-suites") ||
		strings.Contains(text, "/actions/runs")) && !strings.Contains(text, "/logs") && !strings.Contains(text, "/artifacts") {
		return deny
	}
	return nil
}
