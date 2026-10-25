// SPDX-License-Identifier: AGPL-3.0-only

// Package docsync keeps the user and operator documents in step with the code they describe. The userdocs of
// tools/agentkit/context-map.yaml say which documents describe which paths (ctxmap.Impact); a pull request that
// changes the paths either changes each such document or gives, in the "Documentation" section of its
// description, one line per document saying why it needs no change:
//
//   - docs/operations/email.md: no change — only the log wording changed, which it does not quote
//
// Section writes that section for `agentkit pr open`, and Verify checks it in the pull request's preflight.
package docsync

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mycroblog/my/tools/agentkit/internal/ctxmap"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

// Heading is the heading of the section in the pull request's description.
const Heading = "## Documentation"

// MinReasonWords is the fewest words a reason has: enough to say what the document covers and why the change
// leaves it true.
const MinReasonWords = 3

var (
	reComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	// "- <path>: no change — <reason>", the path in backticks or not, the dash an em or en dash, a hyphen, or a colon.
	reReason = regexp.MustCompile("^\\s*[-*]\\s+`?([^\\s`:]+)`?\\s*:\\s*(?i:no change)(?:\\s*[—–:-]\\s*|\\s+)(.*?)\\s*$")
)

// Reasons returns the reasons the Documentation section of body gives, by document path. Text in HTML comments
// (the template's instructions) does not count.
func Reasons(body string) map[string]string {
	out := map[string]string{}
	in := false
	for _, l := range strings.Split(reComment.ReplaceAllString(body, ""), "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "#") {
			in = t == Heading
			continue
		}
		if !in {
			continue
		}
		if m := reReason.FindStringSubmatch(l); m != nil {
			out[m[1]] = m[2]
		}
	}
	return out
}

// Verify returns one line per document of ims that the change neither touches nor explains with a reason of
// MinReasonWords words or more.
func Verify(ims []ctxmap.Impact, reasons map[string]string) []string {
	var out []string
	for _, im := range ims {
		if im.Changed {
			continue
		}
		reason, ok := reasons[im.Doc]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("%s describes %s: change it in this pull request, or add the line `- %s: no change — <reason>` under %q of the description", im.Doc, paths(im.Paths), im.Doc, Heading))
		case len(strings.Fields(reason)) < MinReasonWords:
			out = append(out, fmt.Sprintf("%s: the reason %q is too short: say in %d words or more why the document stays true", im.Doc, reason, MinReasonWords))
		}
	}
	return out
}

// Section returns the Documentation section of a pull request's description for ims: each changed document,
// and each other one with its reason from reasons. problems are Verify's for the section: the documents that are
// neither changed nor explained.
func Section(ims []ctxmap.Impact, reasons map[string]string) (section string, problems []string) {
	var b strings.Builder
	b.WriteString(Heading + "\n\n")
	if len(ims) == 0 {
		b.WriteString("No user or operator document describes the changed paths.\n")
		return b.String(), nil
	}
	for _, im := range ims {
		if im.Changed {
			fmt.Fprintf(&b, "- %s: changed\n", im.Doc)
			continue
		}
		fmt.Fprintf(&b, "- %s: no change — %s\n", im.Doc, strings.TrimSpace(reasons[im.Doc]))
	}
	return b.String(), Verify(ims, reasons)
}

// paths names the first few of ps.
func paths(ps []string) string {
	if len(ps) <= 3 {
		return strings.Join(ps, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(ps[:3], ", "), len(ps)-3)
}

// Lines returns one line per document of ims for a summary: whether the change touches it, and what it describes.
func Lines(ims []ctxmap.Impact) []string {
	var out []string
	for _, im := range ims {
		state := "not changed"
		if im.Changed {
			state = "changed"
		}
		out = append(out, fmt.Sprintf("  %s (%s): describes %s", im.Doc, state, paths(im.Paths)))
	}
	return out
}

// Of returns the documents that describe the paths changed since the merge base of base and HEAD (the working
// tree and untracked files included).
func Of(r *repo.Repo, base string) ([]ctxmap.Impact, error) {
	m, err := ctxmap.Load(r.Root)
	if err != nil {
		return nil, err
	}
	changed, err := r.Changed(base)
	if err != nil {
		return nil, err
	}
	return m.Impact(changed), nil
}
