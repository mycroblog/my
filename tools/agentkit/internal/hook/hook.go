// SPDX-License-Identifier: AGPL-3.0-only

// Package hook implements the Claude Code hooks of .claude/settings.json.
//
// PreToolUse (Bash, Read) denies the calls whose output costs far more than the agentkit command that answers
// the same question — reading a whole document of tens of thousands of tokens, watching CI with
// `gh pr checks --watch` — and the polling loops that wait for a background command and misfire, and hints at agentkit find for repository-wide greps. PostToolUse (Bash) cuts an
// output longer than AGENTKIT_MAX_OUTPUT characters to its head and tail and saves the whole of it to a
// file the agent can grep. Both fail open: anything unexpected leaves the call alone.
package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/repo"
	"github.com/mycroblog/my/tools/agentkit/internal/text"
)

// Input is the part of the hook JSON the hooks read.
type Input struct {
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolResponse json.RawMessage `json:"tool_response"`
	Cwd          string          `json:"cwd"`
}

type bashInput struct {
	Command string `json:"command"`
}

type readInput struct {
	FilePath string `json:"file_path"`
	Offset   *int   `json:"offset"`
	Limit    *int   `json:"limit"`
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

// bigFile is the size from which reading a whole document is denied (AGENTKIT_BIG_FILE, bytes).
func bigFile() int64 { return int64(envInt("AGENTKIT_BIG_FILE", 40000)) }

var (
	// gh in command position only (the start of a line, or after ; & | or a subshell), not in a quoted string.
	reWatch = regexp.MustCompile(`(?m)(^|[;&|(])\s*gh\s+(pr\s+checks\b[^\n|;&]*--watch|run\s+watch\b)`)
	// a loop (until / while) whose body sleeps, and pgrep, both in command position: waiting for a background
	// command this way hangs or ends early (`pgrep -f` matches the polling shell's own command line).
	reLoop     = regexp.MustCompile(`(?m)(^|[;&|(])\s*((do|then|!)\s+)*(until|while)\s`)
	reSleep    = regexp.MustCompile(`(?m)(^|[;&|(])\s*((do|then)\s+)*sleep\s`)
	rePgrep    = regexp.MustCompile(`(?m)(^|[;&|(])\s*((until|while|if|do|then|!)\s+)*pgrep\b`)
	reCat      = regexp.MustCompile(`^\s*cat\s+([^\s|;&<>]+)\s*$`)
	reGrepAll  = regexp.MustCompile(`\b(grep\s+(-\w*[rR]\w*|--recursive)|rg)\b`)
	reLimited  = regexp.MustCompile(`\|\s*(head|tail|wc|sort\s+-u\s*\|\s*head)|\s-(\w*[lcq]\w*)\b|--count|--files-with-matches|--max-count|-m\s*\d`)
	reAgentkit = regexp.MustCompile(`(^|[\s/])(agentkit|agentkit\.sh)\b`)
)

func emit(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}

func deny(w io.Writer, reason string) error {
	return emit(w, map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": reason,
	}})
}

func hint(w io.Writer, msg string) error {
	return emit(w, map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PreToolUse", "additionalContext": msg,
	}})
}

// bigDoc returns the deny reason for reading the whole of file p, or "" when it is small enough.
func bigDoc(cwd, p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	st, err := os.Stat(p)
	if err != nil || st.IsDir() || st.Size() < bigFile() {
		return ""
	}
	ext := filepath.Ext(p)
	if ext != ".md" && st.Size() < 2*bigFile() {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	rel := p
	if r, err := repo.Open(filepath.Dir(p)); err == nil {
		if x, err := filepath.Rel(r.Root, p); err == nil {
			rel = x
		}
	}
	return fmt.Sprintf("%s is %d KB (~%s tokens); reading all of it stays in your context for the rest of the session. "+
		"Read only the part you need: `scripts/agentkit.sh outline %s` lists its sections or declarations with line ranges, "+
		"then `scripts/agentkit.sh show '%s#<anchor>'` (or a name, or %s:<from>-<to>), or Read with offset and limit.",
		rel, st.Size()/1000, text.Ktok(text.Tokens(string(b))), rel, rel, rel)
}

// Pre handles PreToolUse.
func Pre(in Input, w io.Writer) error {
	switch in.ToolName {
	case "Read":
		var ri readInput
		if json.Unmarshal(in.ToolInput, &ri) != nil || ri.Offset != nil || ri.Limit != nil {
			return nil
		}
		if reason := bigDoc(in.Cwd, ri.FilePath); reason != "" {
			return deny(w, reason)
		}
	case "Bash":
		var bi bashInput
		if json.Unmarshal(in.ToolInput, &bi) != nil {
			return nil
		}
		cmd := bi.Command
		if reWatch.MatchString(cmd) {
			return deny(w, "Watching CI prints a status table on every poll into your context. "+
				"Run `scripts/agentkit.sh pr wait <pr>` in the background instead: it prints one line when CI passes, "+
				"or the failing lines of the failed jobs (exit 3 on a conflict). An orchestrator watching many PRs uses "+
				"`scripts/agentkit.sh pr watch` with the Monitor tool.")
		}
		if rePgrep.MatchString(cmd) || (reLoop.MatchString(cmd) && reSleep.MatchString(cmd)) {
			return deny(w, "Polling for a background command (an until / while loop that sleeps, or pgrep) hangs or ends "+
				"early: `pgrep -f` matches the polling shell's own command line. Run the long command with the Bash tool's "+
				"run_in_background and wait for its completion notification; wait for CI with `scripts/agentkit.sh pr wait <pr>` "+
				"in the background, and for many PRs with `scripts/agentkit.sh pr watch` and the Monitor tool.")
		}
		if m := reCat.FindStringSubmatch(cmd); m != nil {
			if reason := bigDoc(in.Cwd, m[1]); reason != "" {
				return deny(w, reason)
			}
		}
		if reGrepAll.MatchString(cmd) && !reLimited.MatchString(cmd) && !reAgentkit.MatchString(cmd) {
			return hint(w, "Tip: `scripts/agentkit.sh find <words> [--kind ...] [--under <path>]` ranks document sections and "+
				"code declarations with line ranges and sizes, and `scripts/agentkit.sh refs <name>` lists what uses a name; "+
				"an unbounded recursive grep returns lines that stay in your context.")
		}
	}
	return nil
}

// Post handles PostToolUse for Bash: an output over the limit is cut to its head and tail, the whole saved.
func Post(in Input, w io.Writer) error {
	if in.ToolName != "Bash" {
		return nil
	}
	var bi bashInput
	if json.Unmarshal(in.ToolInput, &bi) != nil || reAgentkit.MatchString(bi.Command) {
		return nil // agentkit's own output is already within its budget
	}
	var resp map[string]any
	if json.Unmarshal(in.ToolResponse, &resp) != nil {
		return nil
	}
	stdout, ok := resp["stdout"].(string)
	if !ok {
		return nil
	}
	stderr, _ := resp["stderr"].(string)
	limit := envInt("AGENTKIT_MAX_OUTPUT", 20000)
	if len(stdout)+len(stderr) <= limit {
		return nil
	}
	root := in.Cwd
	if r, err := repo.Open(in.Cwd); err == nil {
		root = r.Root
	}
	dir := filepath.Join(root, ".tmp", "agentkit", "out")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil
	}
	file := filepath.Join(dir, time.Now().Format("20060102-150405.000")+".txt")
	whole := stdout
	if stderr != "" {
		whole += "\n--- stderr ---\n" + stderr
	}
	if os.WriteFile(file, []byte(whole), 0o600) != nil {
		return nil
	}
	rel, _ := filepath.Rel(root, file)
	resp["stdout"] = Cut(stdout, limit*2/3, rel)
	if stderr != "" {
		resp["stderr"] = Cut(stderr, limit/3, rel)
	}
	return emit(w, map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PostToolUse", "updatedToolOutput": resp,
	}})
}

// Cut keeps the first two thirds and the last third of limit characters of s, on line boundaries, and says
// where the whole is.
func Cut(s string, limit int, file string) string {
	if len(s) <= limit {
		return s
	}
	head := s[:limit*2/3]
	if i := strings.LastIndexByte(head, '\n'); i > 0 {
		head = head[:i+1]
	}
	tail := s[len(s)-limit/3:]
	if i := strings.IndexByte(tail, '\n'); i >= 0 {
		tail = tail[i+1:]
	}
	omitted := strings.Count(s[len(head):len(s)-len(tail)], "\n")
	return fmt.Sprintf("%s\n[agentkit hook: %d lines (%d characters) cut; the whole output is in %s — grep it or print a range with sed -n 'A,Bp']\n\n%s",
		head, omitted, len(s)-len(head)-len(tail), file, tail)
}

// Run reads the hook JSON from r and runs the hook named event ("pre" or "post").
func Run(event string, r io.Reader, w io.Writer) error {
	var in Input
	if err := json.NewDecoder(r).Decode(&in); err != nil {
		return nil // fail open
	}
	if in.Cwd == "" {
		in.Cwd, _ = os.Getwd()
	}
	switch event {
	case "pre":
		return Pre(in, w)
	case "post":
		return Post(in, w)
	}
	return fmt.Errorf("unknown hook %q (pre or post)", event)
}
