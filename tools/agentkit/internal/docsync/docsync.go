// SPDX-License-Identifier: AGPL-3.0-only

// Package docsync lists the user and operator documents that describe changed paths: the userdocs of the
// context-map.yaml files say which documents describe which paths (ctxmap.Impact).
package docsync

import (
	"fmt"
	"strings"

	"github.com/mycroblog/my/tools/agentkit/internal/ctxmap"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

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
	m, err := ctxmap.Of(r)
	if err != nil {
		return nil, err
	}
	changed, err := r.Changed(base)
	if err != nil {
		return nil, err
	}
	return m.Impact(changed), nil
}
