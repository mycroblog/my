// SPDX-License-Identifier: AGPL-3.0-only

// Package docslint checks the repository's documents against the rules of AGENTS.md that a machine can check:
// relative links and their #anchors resolve, the English-only documents contain no Japanese outside code, and the
// documents for users and operators neither link to nor cite the design records (ADRs, requirements, architecture)
// and GitHub issues or pull requests, and the index (docs/README.md) links every one of those documents.
// docs/lint.yaml says which files each rule covers and lists the files that do not follow a rule yet; an entry
// there must be removed once its file follows the rule, so the lists only shrink.
//
// Inventory checks the other direction, from the code to the documents: every name users and operators see (a
// command of `my`, a metric, a webhook event type, a build tag, a task) is mentioned in the document they look it up in.
package docslint

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v3"

	"github.com/mycroblog/my/tools/agentkit/internal/ctxmap"
	"github.com/mycroblog/my/tools/agentkit/internal/text"
)

// File is the configuration's path from the repository root.
const File = "docs/lint.yaml"

// AllowJapanese, written on a line (in an HTML or YAML comment) or on the line before it, lets that line contain
// Japanese in an English-only document: an intentional example such as a Japanese UI string.
const AllowJapanese = "docs-lint: allow-japanese"

// Config is docs/lint.yaml.
type Config struct {
	Skip     []string `yaml:"skip"`     // files no rule reads (generated or vendored Markdown)
	English  []string `yaml:"english"`  // files written in English only
	Operator []string `yaml:"operator"` // files for users and operators: no design records, issues, or pull requests
	// Index is the document that links every user and operator document, so that a reader finds each.
	Index struct {
		File  string   `yaml:"file"`  // the index
		Files []string `yaml:"files"` // the documents it links
	} `yaml:"index"`
	Pending struct {
		Japanese    []string `yaml:"japanese"`     // English-only files still in Japanese
		DesignLinks []string `yaml:"design-links"` // operator files that still point at design records
	} `yaml:"pending"`
	// Inventory lists, by inventory, the names that Inventory does not require a document to mention.
	Inventory struct {
		Exclude map[string][]string `yaml:"exclude"`
	} `yaml:"inventory"`
}

// Load reads the configuration of the repository at root.
func Load(root string) (*Config, error) {
	b, err := os.ReadFile(filepath.Join(root, File))
	if err != nil {
		return nil, err
	}
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	return &c, nil
}

// Finding is one violation.
type Finding struct {
	Path string
	Line int // 0 for the whole file
	Rule string
	Msg  string
}

func (f Finding) String() string {
	if f.Line == 0 {
		return fmt.Sprintf("%s: %s: %s", f.Path, f.Rule, f.Msg)
	}
	return fmt.Sprintf("%s:%d: %s: %s", f.Path, f.Line, f.Rule, f.Msg)
}

// The rules.
const (
	RuleLink       = "link"
	RuleJapanese   = "japanese"
	RuleDesignLink = "design-link"
	RulePending    = "pending"
	RuleIndex      = "index"
)

// matchAny reports whether p matches one of patterns and none of the exclusions among them: a pattern that starts
// with "!" takes the paths it matches out of the list.
func matchAny(patterns []string, p string) bool {
	in := false
	for _, pat := range patterns {
		if ex, ok := strings.CutPrefix(pat, "!"); ok {
			if ctxmap.Match(ex, p) {
				return false
			}
		} else if ctxmap.Match(pat, p) {
			in = true
		}
	}
	return in
}

// Lint checks files (slash-separated paths from root: the tracked and untracked, not ignored, files) and returns
// the findings in path and line order.
func Lint(root string, files []string, c *Config) ([]Finding, error) {
	l := &linter{root: root, c: c, exists: map[string]bool{}, anchors: map[string]map[string]bool{}}
	for _, f := range files {
		l.exists[f] = true
		for d := path.Dir(f); d != "."; d = path.Dir(d) {
			l.exists[d] = true
		}
	}
	sorted := append([]string(nil), files...)
	sort.Strings(sorted)
	pendingJA := map[string]int{}
	pendingDL := map[string]int{}
	for _, p := range c.Pending.Japanese {
		pendingJA[p] = 0
	}
	for _, p := range c.Pending.DesignLinks {
		pendingDL[p] = 0
	}
	var out []Finding
	for _, p := range sorted {
		if matchAny(c.Skip, p) {
			continue
		}
		md := strings.HasSuffix(p, ".md")
		english := matchAny(c.English, p)
		operator := md && matchAny(c.Operator, p)
		if !md && !english {
			continue
		}
		src, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			return nil, err
		}
		doc := parse(p, string(src), md)
		if md {
			out = append(out, l.links(doc)...)
		}
		if english {
			fs := japanese(doc)
			if _, ok := pendingJA[p]; ok {
				pendingJA[p] = len(fs)
			} else {
				out = append(out, fs...)
			}
		}
		if operator {
			fs := designLinks(doc)
			if _, ok := pendingDL[p]; ok {
				pendingDL[p] = len(fs)
			} else {
				out = append(out, fs...)
			}
		}
	}
	out = append(out, l.index(sorted)...)
	out = append(out, stale(c.Pending.Japanese, pendingJA, l.exists, c.English, "pending.japanese", "contains no Japanese")...)
	out = append(out, stale(c.Pending.DesignLinks, pendingDL, l.exists, c.Operator, "pending.design-links", "points at no design record")...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

// index reports the documents of Index.Files that Index.File does not link to.
func (l *linter) index(files []string) []Finding {
	idx := l.c.Index.File
	if idx == "" {
		return nil
	}
	src, err := os.ReadFile(filepath.Join(l.root, idx))
	if err != nil {
		return []Finding{{File, 0, RuleIndex, fmt.Sprintf("index.file %s does not exist", idx)}}
	}
	linked := map[string]bool{}
	for _, ln := range parse(idx, string(src), true).lines {
		for _, dest := range ln.dests {
			if file, _, ok := target(idx, dest); ok {
				linked[file] = true
			}
		}
	}
	var out []Finding
	for _, f := range files {
		if f != idx && matchAny(l.c.Index.Files, f) && !matchAny(l.c.Skip, f) && !linked[f] {
			out = append(out, Finding{f, 0, RuleIndex, fmt.Sprintf("%s does not link to it: add it to the index", idx)})
		}
	}
	return out
}

// stale reports the entries of a pending list that no longer name a file that breaks the rule.
func stale(list []string, counts map[string]int, exists map[string]bool, scope []string, key, clean string) []Finding {
	var out []Finding
	for _, p := range list {
		switch {
		case !exists[p]:
			out = append(out, Finding{File, 0, RulePending, fmt.Sprintf("%s lists %s, which does not exist; remove it", key, p)})
		case !matchAny(scope, p):
			out = append(out, Finding{File, 0, RulePending, fmt.Sprintf("%s lists %s, which the rule does not cover; remove it", key, p)})
		case counts[p] == 0:
			out = append(out, Finding{File, 0, RulePending, fmt.Sprintf("%s lists %s, which %s now; remove it", key, p, clean)})
		}
	}
	return out
}

// line is one line of a document with what the rules read of it.
type line struct {
	n     int
	prose string   // the text outside code, link destinations, and HTML comments
	dests []string // the link destinations outside code and HTML comments
	allow bool     // AllowJapanese on this line or the line before
}

type document struct {
	path  string
	md    bool
	lines []line
	heads []string // the anchors of the document
}

var (
	reHeading = regexp.MustCompile(`^ {0,3}(#{1,6})\s+(.*?)(?:\s+#+)?\s*$`)
	reFence   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
	// An inline link or image: the text in group 1 and the destination in group 2 or 3; a title may follow.
	reLink    = regexp.MustCompile(`!?\[((?:[^\[\]]|\[[^\[\]]*\])*)\]\(\s*(?:<([^>]*)>|([^\s()]*(?:\([^\s()]*\)[^\s()]*)*))(?:\s+(?:"[^"]*"|'[^']*'|\([^)]*\)))?\s*\)`)
	reRefDef  = regexp.MustCompile(`^ {0,3}\[[^\]]+\]:\s*<?([^\s>]+)>?`)
	reHref    = regexp.MustCompile(`\b(?:href|src)\s*=\s*"([^"]*)"`)
	reAnchor  = regexp.MustCompile(`<a\s[^>]*\b(?:id|name)\s*=\s*"([^"]+)"`)
	reTag     = regexp.MustCompile(`<[^>]+>`)
	reAutourl = regexp.MustCompile(`https?://[^\s<>)\]"'` + "`" + `]+`)
)

// parse splits src into lines and finds the headings, code, and links of a Markdown document. Other files are
// read line by line as prose.
func parse(p, src string, md bool) *document {
	d := &document{path: p, md: md}
	raw := strings.Split(src, "\n")
	fence := ""
	inComment := false
	seen := map[string]int{}
	for i, s := range raw {
		allow := strings.Contains(s, AllowJapanese) || (i > 0 && strings.Contains(raw[i-1], AllowJapanese))
		ln := line{n: i + 1, allow: allow}
		if !md {
			ln.prose = s
			d.lines = append(d.lines, ln)
			continue
		}
		if m := reFence.FindStringSubmatch(s); m != nil {
			switch {
			case fence == "":
				fence = m[1]
			case m[1][0] == fence[0] && len(m[1]) >= len(fence) && strings.TrimSpace(s) == m[1]:
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		visible, comment := splitComments(s, &inComment)
		for _, m := range reAnchor.FindAllStringSubmatch(visible, -1) {
			d.heads = append(d.heads, m[1])
		}
		if m := reHeading.FindStringSubmatch(visible); m != nil {
			a := text.Slug(headingText(m[2]))
			if c := seen[a]; c > 0 {
				seen[a] = c + 1
				a += fmt.Sprintf("-%d", c)
			} else {
				seen[a] = 1
			}
			d.heads = append(d.heads, a)
		}
		code := stripCode(visible)
		if m := reRefDef.FindStringSubmatch(code); m != nil {
			ln.dests = append(ln.dests, m[1])
			code = ""
		}
		prose := reLink.ReplaceAllStringFunc(code, func(l string) string {
			m := reLink.FindStringSubmatch(l)
			dest := m[2]
			if dest == "" {
				dest = m[3]
			}
			ln.dests = append(ln.dests, dest)
			return " " + m[1] + " "
		})
		for _, m := range reHref.FindAllStringSubmatch(prose, -1) {
			ln.dests = append(ln.dests, m[1])
		}
		prose = reTag.ReplaceAllString(prose, " ")
		ln.prose = prose + " " + stripCode(comment)
		d.lines = append(d.lines, ln)
	}
	return d
}

// splitComments separates the text of s outside HTML comments from the text inside them, carrying an open
// comment over to the next line.
func splitComments(s string, in *bool) (visible, comment string) {
	var v, c strings.Builder
	for s != "" {
		if *in {
			end := strings.Index(s, "-->")
			if end < 0 {
				c.WriteString(s)
				return v.String(), c.String()
			}
			c.WriteString(s[:end] + " ")
			s = s[end+3:]
			*in = false
			continue
		}
		start := strings.Index(s, "<!--")
		if start < 0 {
			v.WriteString(s)
			break
		}
		v.WriteString(s[:start] + " ")
		s = s[start+4:]
		*in = true
	}
	return v.String(), c.String()
}

// stripCode replaces the code spans of s (a run of backticks up to the next run of the same length) with a space.
func stripCode(s string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '`')
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		n := i
		for n < len(s) && s[n] == '`' {
			n++
		}
		run := s[i:n]
		end := -1
		for j := n; j < len(s); {
			k := strings.Index(s[j:], run)
			if k < 0 {
				break
			}
			k += j
			if (k+len(run) == len(s) || s[k+len(run)] != '`') && (k == 0 || s[k-1] != '`') {
				end = k
				break
			}
			j = k + 1
			for j < len(s) && s[j] == '`' {
				j++
			}
		}
		if end < 0 {
			b.WriteString(s[:n])
			s = s[n:]
			continue
		}
		b.WriteString(s[:i] + " ")
		s = s[end+len(run):]
	}
}

// headingText is the text GitHub makes an anchor of: links replaced by their text and HTML tags dropped.
func headingText(h string) string {
	h = reLink.ReplaceAllString(h, "$1")
	return reTag.ReplaceAllString(h, "")
}

var reScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// target resolves a relative destination of the document at p to a repository path and an anchor. ok is false
// for a URL with a scheme and for an empty destination.
func target(p, dest string) (file, anchor string, ok bool) {
	if dest == "" || reScheme.MatchString(dest) || strings.HasPrefix(dest, "//") {
		return "", "", false
	}
	file, anchor, _ = strings.Cut(dest, "#")
	file, _, _ = strings.Cut(file, "?")
	if u, err := url.PathUnescape(file); err == nil {
		file = u
	}
	if u, err := url.PathUnescape(anchor); err == nil {
		anchor = u
	}
	switch {
	case file == "":
		file = p
	case strings.HasPrefix(file, "/"):
		file = path.Clean(strings.TrimPrefix(file, "/"))
	default:
		file = path.Clean(path.Join(path.Dir(p), file))
	}
	return file, anchor, true
}

type linter struct {
	root    string
	c       *Config
	exists  map[string]bool
	anchors map[string]map[string]bool
}

func (l *linter) anchorsOf(p string) map[string]bool {
	if a, ok := l.anchors[p]; ok {
		return a
	}
	a := map[string]bool{}
	if src, err := os.ReadFile(filepath.Join(l.root, p)); err == nil {
		for _, h := range parse(p, string(src), true).heads {
			a[strings.ToLower(h)] = true
		}
	}
	l.anchors[p] = a
	return a
}

// links checks that the relative links of a Markdown document point at files of the repository and, for a
// Markdown file, at one of its headings.
func (l *linter) links(d *document) []Finding {
	var out []Finding
	for _, ln := range d.lines {
		for _, dest := range ln.dests {
			file, anchor, ok := target(d.path, dest)
			if !ok {
				continue
			}
			if file == ".." || strings.HasPrefix(file, "../") {
				out = append(out, Finding{d.path, ln.n, RuleLink, fmt.Sprintf("%s points outside the repository", dest)})
				continue
			}
			if file != "." && !l.exists[file] {
				out = append(out, Finding{d.path, ln.n, RuleLink, fmt.Sprintf("%s: %s does not exist", dest, file)})
				continue
			}
			if anchor == "" || !strings.HasSuffix(file, ".md") {
				continue
			}
			if !l.anchorsOf(file)[strings.ToLower(anchor)] {
				out = append(out, Finding{d.path, ln.n, RuleLink, fmt.Sprintf("%s: %s has no heading #%s (agentkit outline %s)", dest, file, anchor, file)})
			}
		}
	}
	return out
}

// isJapanese reports whether r is a letter or punctuation of Japanese text: kana, kanji, the CJK symbols, and
// the full-width forms.
func isJapanese(r rune) bool {
	return unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) ||
		(r >= 0x3000 && r <= 0x303f) || (r >= 0xff00 && r <= 0xffef)
}

// japanese reports the lines of d with Japanese outside code, link destinations, and allowed lines.
func japanese(d *document) []Finding {
	var out []Finding
	for _, ln := range d.lines {
		if ln.allow {
			continue
		}
		if i := strings.IndexFunc(ln.prose, isJapanese); i >= 0 {
			r := []rune(strings.TrimSpace(ln.prose[i:]))
			out = append(out, Finding{d.path, ln.n, RuleJapanese, fmt.Sprintf("Japanese text %q in an English-only document (write it in English, put an example in code, or mark it with %q)", string(r[:min(len(r), 20)]), AllowJapanese)})
		}
	}
	return out
}

var (
	designDirs = []string{"docs/adr/", "docs/requirements/", "docs/architecture/"}
	reGitHub   = regexp.MustCompile(`^https?://github\.com/mycroblog/my/(?:issues|pull|discussions)/\d+|^https?://github\.com/mycroblog/my/(?:blob|tree)/[^/]+/docs/(?:adr|requirements|architecture)/`)
	reCiteADR  = regexp.MustCompile(`\bADR-\d{4}\b`)
	reCiteReq  = regexp.MustCompile(`\b(?:NFR-[A-Z]+-\d+|NFR-\d{3}|FR-\d{3})\b`)
	reCiteNum  = regexp.MustCompile(`(?:^|[\s(（、,])(#\d{1,5})\b`)
)

// designLinks reports the links of an operator document to design records, issues, and pull requests, and the
// identifiers that cite them (ADR-0001, FR-101, #123) outside code.
func designLinks(d *document) []Finding {
	var out []Finding
	for _, ln := range d.lines {
		dests := append(append([]string(nil), ln.dests...), reAutourl.FindAllString(ln.prose, -1)...)
		for _, dest := range dests {
			if reGitHub.MatchString(dest) {
				out = append(out, Finding{d.path, ln.n, RuleDesignLink, fmt.Sprintf("link to %s: explain the behaviour here instead of pointing at an issue, a pull request, or a design record", dest)})
				continue
			}
			file, _, ok := target(d.path, dest)
			if !ok {
				continue
			}
			for _, dir := range designDirs {
				if strings.HasPrefix(file+"/", dir) {
					out = append(out, Finding{d.path, ln.n, RuleDesignLink, fmt.Sprintf("link to %s: operator documents explain the behaviour themselves instead of pointing at design records", file)})
					break
				}
			}
		}
		var cites []string
		cites = append(cites, reCiteADR.FindAllString(ln.prose, -1)...)
		cites = append(cites, reCiteReq.FindAllString(ln.prose, -1)...)
		for _, m := range reCiteNum.FindAllStringSubmatch(ln.prose, -1) {
			cites = append(cites, m[1])
		}
		if len(cites) > 0 {
			out = append(out, Finding{d.path, ln.n, RuleDesignLink, fmt.Sprintf("cites %s: operator documents do not refer to ADRs, requirements, issues, or pull requests", strings.Join(text.Uniq(cites), ", "))})
		}
	}
	return out
}
