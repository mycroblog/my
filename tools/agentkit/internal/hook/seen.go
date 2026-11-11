// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/repo"
	"github.com/mycroblog/my/tools/agentkit/internal/shots"
)

// recordSeen handles PostToolUse for Read: a read of a screenshot under .tmp/agentkit/shots is recorded in
// .tmp/agentkit/seen.json, which `agentkit pr open` requires for a change of the web UI. It prints nothing and
// fails open.
func recordSeen(in Input) {
	if in.ToolName != "Read" {
		return
	}
	var ri readInput
	if json.Unmarshal(in.ToolInput, &ri) != nil || ri.FilePath == "" {
		return
	}
	p := ri.FilePath
	if !filepath.IsAbs(p) {
		p = filepath.Join(in.Cwd, p)
	}
	if filepath.Ext(p) != ".png" || !strings.Contains(filepath.ToSlash(p), "/"+shots.Dir+"/") {
		return
	}
	r, err := repo.Open(filepath.Dir(p))
	if err != nil {
		return
	}
	rel, err := filepath.Rel(r.Root, p)
	if err != nil || !strings.HasPrefix(filepath.ToSlash(rel), shots.Dir+"/") {
		return
	}
	rel = filepath.ToSlash(rel)
	fingerprint := ""
	if changed, err := r.Changed("origin/main"); err == nil {
		if src := shots.Sources(changed); shots.Fresh(r.Root, rel, src) {
			fingerprint = shots.Fingerprint(r.Root, src)
		}
	}
	_ = shots.Record(r.Root, rel, time.Now(), fingerprint)
}
