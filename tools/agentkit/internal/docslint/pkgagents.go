// SPDX-License-Identifier: AGPL-3.0-only

package docslint

import (
	"fmt"
	"regexp"
	"strings"
)

// RulePackageAgents is an AGENTS.md of a directory below the root that holds more than pointers to external documents.
const RulePackageAgents = "package-agents"

// PackageAgentsMaxItems is the number of pointers one AGENTS.md of a package holds; a directory that needs more
// has subdirectories the pointers belong to.
const PackageAgentsMaxItems = 12

// packageAgentsHolds is what the findings tell the author.
const packageAgentsHolds = "a package's AGENTS.md only points from a task to the external document that governs it; " +
	"rules belong in the check that enforces them, structure in the code"

// packageAgentsItem is the shape of a pointer.
const packageAgentsItem = "`- <When you do X>: [<name of the documentation>](https://...), search for: <term>, <term>`"

var (
	reListItem  = regexp.MustCompile(`^[-*+]\s+\S`)
	reSearchFor = regexp.MustCompile(`\)\s*,\s*search for:\s*(.*)$`)
)

// isPackageAgents reports whether p is an AGENTS.md below the repository root.
func isPackageAgents(p string) bool {
	return strings.HasSuffix(p, "/AGENTS.md")
}

// packageAgents checks the AGENTS.md of a package, whose text is src: a title line, then at most
// PackageAgentsMaxItems list items, each of one link to the entry point of an external document and the terms to
// search it for.
func packageAgents(p, src string) []Finding {
	var out []Finding
	add := func(n int, format string, args ...any) {
		out = append(out, Finding{p, n, RulePackageAgents, fmt.Sprintf(format, args...)})
	}
	title, items := false, 0
	for i, s := range strings.Split(src, "\n") {
		n := i + 1
		switch {
		case strings.TrimSpace(s) == "":
		case !title:
			title = true
			if m := reHeading.FindStringSubmatch(s); m == nil || len(m[1]) != 1 {
				add(n, "the first line is not a `# <what the package implements or talks to>` title: %s", packageAgentsHolds)
			}
		case reListItem.MatchString(s):
			items++
			if items == PackageAgentsMaxItems+1 {
				add(n, "more than %d pointers: %s; move the pointers of a subdirectory into its own AGENTS.md", PackageAgentsMaxItems, packageAgentsHolds)
			}
			if msg := packageAgentsPointer(s); msg != "" {
				add(n, "%s", msg)
			}
		default:
			add(n, "%s: %s; delete this line, or move a rule into the failure message of its check and anything about the code into the code",
				packageAgentsOther(s), packageAgentsHolds)
		}
	}
	if !title {
		add(0, "empty: %s; write the pointers as %s or delete the file", packageAgentsHolds, packageAgentsItem)
	}
	return out
}

// packageAgentsOther names what a line that is neither the title nor a pointer is.
func packageAgentsOther(s string) string {
	t := strings.TrimSpace(s)
	switch {
	case reHeading.MatchString(s):
		return "a second heading"
	case reFence.MatchString(s):
		return "a code block"
	case strings.HasPrefix(t, "|"):
		return "a table"
	case s != t && reListItem.MatchString(t):
		return "a nested list item"
	}
	return "a line that is not a pointer"
}

// packageAgentsPointer returns what is wrong with the list item s, or "".
func packageAgentsPointer(s string) string {
	one := fmt.Sprintf("a pointer has exactly one link, to the external document that governs the task: write it as %s; %s",
		packageAgentsItem, packageAgentsHolds)
	// A term may be an address the documentation names (an endpoint, a namespace): it is in a code span.
	item, s := s, stripCode(s)
	links := reLink.FindAllStringSubmatch(s, -1)
	if len(links) != 1 {
		return one
	}
	dest := links[0][2]
	if dest == "" {
		dest = links[0][3]
	}
	if !strings.HasPrefix(dest, "https://") {
		return fmt.Sprintf("the link %s is not an https:// link to an external document: %s", dest, packageAgentsHolds)
	}
	// An address written outside the link is a second pointer.
	if len(reAutourl.FindAllString(s, -1)) != 1 {
		return one
	}
	if strings.Contains(dest, "#") {
		return "link the documentation's entry point and name the terms to search for; a link to a section goes stale"
	}
	if m := reSearchFor.FindStringSubmatch(item); m == nil || strings.Trim(m[1], " ,.`") == "" {
		return fmt.Sprintf("no terms after the link: write the pointer as %s, with the exact terms the documentation uses", packageAgentsItem)
	}
	return ""
}
