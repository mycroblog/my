// SPDX-License-Identifier: AGPL-3.0-only

package ctxmap

import (
	"sort"
	"strings"
)

// Impact is one user or operator document that describes changed paths.
type Impact struct {
	Doc     string   // the document's path
	Refs    []string // the userdocs references that name it (<path> or <path>#<anchor>), sorted
	Paths   []string // the changed paths it describes, sorted
	Changed bool     // the change touches the document itself
}

// Impact returns the userdocs of the entries of the changed paths, one per document in path order. Paths that
// match Ignore or Userdocs.Skip (generated files, tests) describe nothing a document says, and are passed over.
func (m *Map) Impact(changed []string) []Impact {
	touched := map[string]bool{}
	for _, p := range changed {
		touched[p] = true
	}
	byDoc := map[string]*Impact{}
	for _, p := range changed {
		if m.ignored(p) || matchAny(m.Userdocs.Skip, p) {
			continue
		}
		for _, e := range m.For(p) {
			for _, ref := range e.Userdocs {
				doc, _, _ := strings.Cut(ref, "#")
				if doc == p {
					continue
				}
				im := byDoc[doc]
				if im == nil {
					im = &Impact{Doc: doc, Changed: touched[doc]}
					byDoc[doc] = im
				}
				im.Refs = appendNew(im.Refs, ref)
				im.Paths = appendNew(im.Paths, p)
			}
		}
	}
	out := make([]Impact, 0, len(byDoc))
	for _, doc := range sortedKeys(byDoc) {
		im := byDoc[doc]
		sort.Strings(im.Refs)
		sort.Strings(im.Paths)
		out = append(out, *im)
	}
	return out
}

func appendNew(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}
