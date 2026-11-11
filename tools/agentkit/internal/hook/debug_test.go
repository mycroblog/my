// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"os"
	"testing"
)

func TestDebug(t *testing.T) {
	f := os.Getenv("AGENTKIT_HOOK_CMD")
	if f == "" {
		t.Skip()
	}
	b, _ := os.ReadFile(f)
	wd, _ := os.Getwd()
	c := &checker{missingIsRepo: true}
	c.session = c.rootOf(wd)
	cmd := reOldRoot.ReplaceAllString(string(b), c.session)
	for _, s := range splitCommand(cmd) {
		pc := parseCommand(s.text, map[string]string{})
		t.Logf("seg tool=%q args=%d heredoc=%d pin=%v pout=%v text=%.80q", pc.tool, len(pc.args), len(s.heredoc), s.pipedIn, s.pipedOut, s.text)
	}
	t.Logf("verdict: %+v", c.check(cmd))
}
