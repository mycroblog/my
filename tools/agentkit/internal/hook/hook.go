// SPDX-License-Identifier: AGPL-3.0-only

// Package hook implements the Claude Code hooks of .claude/settings.json.
//
// PreToolUse (Bash, Read) denies the calls that an agentkit command or a built-in tool does at a fraction of the
// tokens, and says which: searching the repository with grep, paging a file with sed -n / head / tail, rewriting
// a file from the shell, merging main into a branch, polling CI, reading a whole document of tens of thousands
// of tokens, and the polling loops that wait for a background command and misfire (rules.go; shell.go splits a
// compound command into the simple commands the rules read). PostToolUse (Bash) cuts an
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

var reAgentkit = regexp.MustCompile(`(^|[\s/])(agentkit|agentkit\.sh)\b`)

func emit(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}

func deny(w io.Writer, reason string) error {
	return emit(w, map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": reason,
	}})
}

// bigDoc returns the deny reason for reading the whole of the repository's file t, or "" when it is small enough.
func bigDoc(t target, agentkit, shown string) string {
	st, err := os.Stat(t.abs)
	if err != nil || st.IsDir() || st.Size() < bigFile() {
		return ""
	}
	if filepath.Ext(t.abs) != ".md" && st.Size() < 2*bigFile() {
		return ""
	}
	b, err := os.ReadFile(t.abs)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s is %d KB (~%s tokens); reading all of it stays in your context for the rest of the session. "+
		"Read only the part you need: `%s outline %s` lists its sections or declarations with line ranges, "+
		"then `%s show '%s#<anchor>'` (or a name, or %s:<from>-<to>), or Read with offset and limit.",
		t.rel, st.Size()/1000, text.Ktok(text.Tokens(string(b))), agentkit, shown, agentkit, shown, shown)
}

// Pre handles PreToolUse: it denies the call or prints nothing. Anything unexpected, a panic included, leaves
// the call alone.
func Pre(in Input, w io.Writer) (err error) {
	defer func() {
		if recover() != nil {
			err = nil
		}
	}()
	c := &checker{session: in.Cwd}
	switch in.ToolName {
	case "Read":
		var ri readInput
		if json.Unmarshal(in.ToolInput, &ri) != nil || ri.Offset != nil || ri.Limit != nil {
			return nil
		}
		// Files outside the repository and under .tmp/ (context packs, saved output) are read whole.
		if t, ok := c.locate(in.Cwd, word{text: ri.FilePath}); ok {
			if reason := bigDoc(t, c.agentkit(t.root), c.show(t)); reason != "" {
				return deny(w, reason)
			}
		}
	case "Bash":
		var bi bashInput
		if json.Unmarshal(in.ToolInput, &bi) != nil {
			return nil
		}
		if d := c.check(bi.Command); d != nil {
			return deny(w, d.reason)
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
		recordSeen(in)
		return Post(in, w)
	}
	return fmt.Errorf("unknown hook %q (pre or post)", event)
}
