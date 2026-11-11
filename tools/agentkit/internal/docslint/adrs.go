// SPDX-License-Identifier: AGPL-3.0-only

package docslint

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/mycroblog/my/tools/agentkit/internal/adr"
	"github.com/mycroblog/my/tools/agentkit/internal/text"
)

// The rules of the ADRs and of the citations of them.
const (
	RuleADRCitation = "adr-citation"
	RuleADRSection  = "adr-section"
	RuleADRFormat   = "adr-format"
)

const adrSec = `(\d+(?:\.\d+)*|[A-Z]\b)`

var (
	reADRFile = regexp.MustCompile(`^docs/adr/(\d{4})-[^/]*\.md$`)
	// A citation with its sections: ADR-0070, ADR-0070 §4, ADR-0070§4.1, §5 and §6.
	reADRCite = regexp.MustCompile(`\bADR-(\d{4})\b((?:\s*§\s*` + adrSec + `(?:(?:\s*(?:,|、|・|and|と)\s*)§\s*` + adrSec + `)*)?)`)
	reADRSec  = regexp.MustCompile(`§\s*` + adrSec)
	// What a § follows when it belongs to another document (ADR-0018 §3, RFC 9421 の §2, data-model§4.4, web-client（§8）).
	reSecOwner = regexp.MustCompile(`[\w./-]\s*[の（(]?\s*$`)
	// What a § follows when it continues a list of sections (§3, §4).
	reSecList = regexp.MustCompile(`(?:,|、|・|/|〜|-|–|and|と|or|か)\s*$`)
	reQuote   = regexp.MustCompile(`^\s*>`)
)

// adrShapeScope is the files the shape rule reads, as the patterns of docs/lint.yaml.
var adrShapeScope = []string{"docs/adr/*.md"}

// ADRs checks the ADRs and the citations of them, in path and line order: each ADR-NNNN in files exists and is not
// a retired number, each ADR-NNNN §k is a heading of that ADR, a bare §k inside an ADR is a heading of the same ADR,
// and an ADR has a Decision section, no quoted answers, and the shape of a decision (adrShape) unless
// Pending.ADRShape lists it.
func ADRs(root string, files []string, c *Config) ([]Finding, error) {
	aliases, err := adr.LoadAliases(root)
	if err != nil {
		return nil, err
	}
	sorted := append([]string(nil), files...)
	sort.Strings(sorted)
	arts, err := newArtifacts(root, sorted)
	if err != nil {
		return nil, err
	}
	exists := map[string]bool{}
	for _, p := range sorted {
		exists[p] = true
	}
	pending := map[string]int{}
	for _, p := range c.Pending.ADRShape {
		pending[p] = 0
	}
	docs := map[string]*document{} // by number
	for _, p := range sorted {
		m := reADRFile.FindStringSubmatch(p)
		if m == nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			return nil, err
		}
		docs[m[1]] = parse(p, string(b), true)
	}
	var out []Finding
	for _, id := range sortedKeys(aliases) {
		to := strings.TrimPrefix(aliases[id].To, "ADR-")
		if docs[to] == nil {
			out = append(out, Finding{adr.AliasFile, 0, RuleADRCitation, fmt.Sprintf("%s -> %s: no such ADR", id, aliases[id].To)})
		}
	}
	for _, p := range sorted {
		if p == adr.AliasFile || matchAny(c.ADRs.Exclude, p) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil || bytes.IndexByte(b, 0) >= 0 {
			continue // a directory link or a binary file holds no citation
		}
		out = append(out, adrCitations(p, string(b), docs, aliases)...)
		if m := reADRFile.FindStringSubmatch(p); m != nil {
			out = append(out, bareSections(docs[m[1]])...)
			out = append(out, adrFormat(docs[m[1]])...)
			_, fs := adrShape(docs[m[1]], string(b), arts)
			if _, ok := pending[p]; ok {
				pending[p] = len(fs)
			} else {
				out = append(out, fs...)
			}
		}
	}
	out = append(out, stale(c.Pending.ADRShape, pending, exists, adrShapeScope, "pending.adr-shape", "has the shape of an ADR")...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

// adrCitations checks the citations in the text of file p.
func adrCitations(p, src string, docs map[string]*document, aliases adr.Aliases) []Finding {
	var out []Finding
	for i, ln := range strings.Split(src, "\n") {
		for _, m := range reADRCite.FindAllStringSubmatch(ln, -1) {
			id := "ADR-" + m[1]
			if _, retired := aliases[id]; retired {
				out = append(out, Finding{p, i + 1, RuleADRCitation, fmt.Sprintf("%s is retired: cite %s", id, aliases.Current(id))})
				continue
			}
			d := docs[m[1]]
			if d == nil {
				out = append(out, Finding{p, i + 1, RuleADRCitation, id + " does not exist"})
				continue
			}
			for _, s := range reADRSec.FindAllStringSubmatch(m[2], -1) {
				if !hasSection(d, s[1]) {
					out = append(out, Finding{p, i + 1, RuleADRCitation, fmt.Sprintf("%s has no §%s", id, s[1])})
				}
			}
		}
	}
	return out
}

// hasSection reports whether a heading of d has the number sec.
func hasSection(d *document, sec string) bool {
	for _, ln := range d.lines {
		if ln.prose == "" {
			continue // a line of a code block
		}
		if m := reHeading.FindStringSubmatch(ln.shown); m != nil && text.SectionNumber(headingText(m[2])) == sec {
			return true
		}
	}
	return false
}

// bareSections checks the §k of an ADR that name no other document. A § in a list takes its owner from the one
// before it.
func bareSections(d *document) []Finding {
	var out []Finding
	for _, ln := range d.lines {
		prev, last := false, 0
		for _, m := range reADRSec.FindAllStringSubmatchIndex(ln.prose, -1) {
			before := ln.prose[:m[0]]
			inList := prev && reSecList.MatchString(strings.TrimRight(before[last:], " "))
			owned := reSecOwner.MatchString(before)
			if inList {
				owned = prev
			}
			prev, last = owned, m[1]
			if owned {
				continue
			}
			if sec := ln.prose[m[2]:m[3]]; !hasSection(d, sec) {
				out = append(out, Finding{d.path, ln.n, RuleADRSection, fmt.Sprintf("this ADR has no §%s", sec)})
			}
		}
	}
	return out
}

// adrFormat checks that an ADR has the Decision section that `agentkit show` and `ctx` read, and that it quotes no
// answers: git has the discussion.
func adrFormat(d *document) []Finding {
	var out []Finding
	if !slices.ContainsFunc(d.heads, func(h string) bool { return strings.HasPrefix(h, "decision") }) {
		out = append(out, Finding{d.path, 0, RuleADRFormat, "lacks the section ## Decision（決定）"})
	}
	for _, ln := range d.lines {
		switch {
		case ln.prose == "":
		case reQuote.MatchString(ln.shown):
			out = append(out, Finding{d.path, ln.n, RuleADRFormat, "a quotation: state the decision itself"})
		case strings.Contains(ln.prose, "決定者の回答"):
			out = append(out, Finding{d.path, ln.n, RuleADRFormat, "the decider's answers do not belong in an ADR"})
		}
	}
	return out
}

// ADRShapeOf is one ADR with its measures and whether it breaks the shape rule.
type ADRShapeOf struct {
	Path string
	ADRShape
	Breaks bool
}

// ADRShapes measures the ADRs among files, in path order.
func ADRShapes(root string, files []string) ([]ADRShapeOf, error) {
	sorted := append([]string(nil), files...)
	sort.Strings(sorted)
	arts, err := newArtifacts(root, sorted)
	if err != nil {
		return nil, err
	}
	var out []ADRShapeOf
	for _, p := range sorted {
		if !reADRFile.MatchString(p) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			return nil, err
		}
		s, fs := adrShape(parse(p, string(b), true), string(b), arts)
		out = append(out, ADRShapeOf{p, s, len(fs) > 0})
	}
	return out, nil
}
