// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"path"
	"regexp"
	"strings"
)

// segment is one simple command of a compound shell command.
type segment struct {
	text     string // the command, without the bodies of its here-documents
	heredoc  string // the bodies of its here-documents
	pipedIn  bool   // reads the output of the segment before it
	pipedOut bool   // its output goes to the next segment
}

var reHeredoc = regexp.MustCompile(`^<<-?\s*(["']?)([A-Za-z_][A-Za-z0-9_]*)(["']?)`)

// splitCommand cuts a shell command at ; && || | & and newlines, outside quotes, $( ) and here-documents.
func splitCommand(cmd string) []segment {
	var (
		segs    []segment
		cur     strings.Builder
		body    strings.Builder
		quote   byte
		depth   int
		pipedIn bool
		tags    []string
	)
	flush := func(sep string) {
		t := strings.TrimSpace(cur.String())
		if t != "" {
			segs = append(segs, segment{text: t, heredoc: body.String(), pipedIn: pipedIn, pipedOut: sep == "|"})
		}
		pipedIn = sep == "|"
		cur.Reset()
		body.Reset()
	}
	n := len(cmd)
	for i := 0; i < n; {
		c := cmd[i]
		if quote != 0 {
			cur.WriteByte(c)
			if c == '\\' && quote == '"' && i+1 < n {
				cur.WriteByte(cmd[i+1])
				i += 2
				continue
			}
			if c == quote {
				quote = 0
			}
			i++
			continue
		}
		switch {
		case c == '\\' && i+1 < n:
			if cmd[i+1] != '\n' {
				cur.WriteByte(c)
				cur.WriteByte(cmd[i+1])
			}
			i += 2
		case c == '"' || c == '\'':
			quote = c
			cur.WriteByte(c)
			i++
		case c == '$' && i+1 < n && cmd[i+1] == '(':
			depth++
			cur.WriteString("$(")
			i += 2
		case c == '(' && depth > 0:
			depth++
			cur.WriteByte(c)
			i++
		case c == ')' && depth > 0:
			depth--
			cur.WriteByte(c)
			i++
		case depth > 0:
			cur.WriteByte(c)
			i++
		case c == '<' && strings.HasPrefix(cmd[i:], "<<") && !strings.HasPrefix(cmd[i:], "<<<"):
			if m := reHeredoc.FindStringSubmatch(cmd[i:]); m != nil {
				tags = append(tags, m[2])
				cur.WriteString(m[0])
				i += len(m[0])
			} else {
				cur.WriteByte(c)
				i++
			}
		case c == '\n':
			if len(tags) > 0 {
				rest := cmd[i+1:]
				for _, tag := range tags {
					end, next := heredocEnd(rest, tag)
					body.WriteString(rest[:end])
					rest = rest[next:]
				}
				tags = nil
				flush("\n")
				i = n - len(rest)
				continue
			}
			flush("\n")
			i++
		case c == ';':
			flush(";")
			i++
		case c == '&' && i+1 < n && cmd[i+1] == '&':
			flush("&&")
			i += 2
		case c == '|' && i+1 < n && cmd[i+1] == '|':
			flush("||")
			i += 2
		case c == '|':
			if i+1 < n && cmd[i+1] == '&' {
				i++
			}
			flush("|")
			i++
		case c == '&' && !(i > 0 && (cmd[i-1] == '<' || cmd[i-1] == '>')) && !(i+1 < n && cmd[i+1] == '>'):
			flush("&")
			i++
		default:
			cur.WriteByte(c)
			i++
		}
	}
	flush("")
	return segs
}

// heredocEnd finds the line that is tag alone in s: where the body ends and where the text after it starts.
// A here-document without its end runs to the end of s.
func heredocEnd(s, tag string) (end, next int) {
	for pos := 0; pos < len(s); {
		nl := strings.IndexByte(s[pos:], '\n')
		line := s[pos:]
		after := len(s)
		if nl >= 0 {
			line = s[pos : pos+nl]
			after = pos + nl
		}
		if strings.TrimSpace(line) == tag {
			return pos, after
		}
		if nl < 0 {
			break
		}
		pos += nl + 1
	}
	return len(s), len(s)
}

// word is one word of a simple command after quote removal.
type word struct {
	text  string
	dyn   bool // has an unquoted or double-quoted $ or a backtick: its value is not known
	glob  bool // has an unquoted * ? or [
	redir bool // a redirection operator (>, 2>&1, <), possibly with its target attached
}

// words splits a simple command into words the way a POSIX shell does, without expanding anything.
func words(s string) []word {
	var (
		out   []word
		cur   strings.Builder
		w     word
		open  bool
		quote byte
	)
	end := func() {
		if open {
			w.text = cur.String()
			out = append(out, w)
		}
		cur.Reset()
		w, open = word{}, false
	}
	n := len(s)
	for i := 0; i < n; i++ {
		c := s[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
		case quote == '"':
			switch {
			case c == '"':
				quote = 0
			case c == '\\' && i+1 < n && strings.IndexByte("\"\\$`", s[i+1]) >= 0:
				i++
				cur.WriteByte(s[i])
			default:
				if c == '$' || c == '`' {
					w.dyn = true
				}
				cur.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote, open = c, true
		case c == '\\' && i+1 < n:
			i++
			open = true
			cur.WriteByte(s[i])
		case c == ' ' || c == '\t' || c == '\n':
			end()
		case c == '$' && i+1 < n && s[i+1] == '(':
			// $( ... ) is one opaque part of the word.
			depth, j := 0, i
			for ; j < n; j++ {
				if s[j] == '(' {
					depth++
				} else if s[j] == ')' {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			if j >= n {
				j = n - 1
			}
			cur.WriteString(s[i : j+1])
			w.dyn, open = true, true
			i = j
		default:
			switch c {
			case '$', '`':
				w.dyn = true
			case '*', '?', '[':
				w.glob = true
			case '>', '<':
				if !open || isDigits(cur.String()) || cur.String() == "&" {
					w.redir = true
				}
			}
			open = true
			cur.WriteByte(c)
		}
	}
	end()
	return out
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// command is a simple command reduced to the tool it runs.
type command struct {
	tool     string   // base name of the program: grep, sed, git, ...
	args     []word   // its arguments, redirections removed
	keywords []string // the shell keywords and wrappers before it: until, while, do, timeout, xargs, ...
	writes   []word   // the targets of its > and >> redirections
}

var (
	reAssign  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	reTimeArg = regexp.MustCompile(`^(-.*|\d+(\.\d+)?[smhd]?)$`)
	wrappers  = map[string]bool{
		"timeout": true, "time": true, "sudo": true, "nice": true, "env": true, "command": true, "exec": true,
		"nohup": true, "then": true, "do": true, "else": true, "elif": true, "{": true, "(": true, "!": true,
		"if": true, "while": true, "until": true, "xargs": true,
	}
	xargsValue = map[string]bool{"-I": true, "-n": true, "-P": true, "-d": true, "-L": true, "-s": true, "-E": true}
)

// parseCommand strips redirections, variable assignments and wrappers (timeout, env, xargs, do, then, ...)
// from a segment and names the program it runs.
//
// vars holds the variables the command itself has set to a constant so far (`f=docs/x.md; sed -i ... "$f"`):
// a word made only of them is no longer unknown, and an assignment in seg updates them.
func parseCommand(seg string, vars map[string]string) command {
	var c command
	var ws []word
	all := words(seg)
	for i := range all {
		all[i] = expand(all[i], vars)
	}
	for i := 0; i < len(all); i++ {
		w := all[i]
		if !w.redir {
			ws = append(ws, w)
			continue
		}
		op := strings.TrimLeft(w.text, "0123456789&")
		target := strings.TrimLeft(op, "<>|")
		out := strings.HasPrefix(op, ">")
		if target == "" && i+1 < len(all) {
			i++
			target = all[i].text
			w = all[i]
		}
		if out && !strings.HasPrefix(target, "&") && target != "" {
			c.writes = append(c.writes, word{text: target, dyn: w.dyn, glob: w.glob})
		}
	}
	i := 0
	for i < len(ws) {
		t := strings.TrimLeft(ws[i].text, "(")
		if t == "" {
			i++
			continue
		}
		if reAssign.MatchString(t) {
			assign(vars, ws[i])
			i++
			continue
		}
		if !wrappers[t] {
			break
		}
		c.keywords = append(c.keywords, t)
		i++
		switch t {
		case "timeout":
			for i < len(ws) && reTimeArg.MatchString(ws[i].text) {
				i++
			}
		case "xargs":
			for i < len(ws) && strings.HasPrefix(ws[i].text, "-") {
				if xargsValue[ws[i].text] {
					i++
				}
				i++
			}
		}
	}
	if i >= len(ws) {
		return c
	}
	c.tool = path.Base(strings.TrimLeft(ws[i].text, "("))
	c.args = ws[i+1:]
	return c
}

var reVar = regexp.MustCompile(`\$\{(\w+)\}|\$(\w+)`)

// expand replaces the variables of w that vars knows; w stays unknown when any other expansion is left.
func expand(w word, vars map[string]string) word {
	if !w.dyn || len(vars) == 0 || strings.Contains(w.text, "$(") || strings.Contains(w.text, "`") {
		return w
	}
	known := true
	s := reVar.ReplaceAllStringFunc(w.text, func(m string) string {
		v, ok := vars[strings.Trim(m, "${}")]
		if !ok {
			known = false
		}
		return v
	})
	if known && !strings.Contains(s, "$") {
		w.text, w.dyn = s, false
	}
	return w
}

// assign records the assignment w (NAME=value) in vars, or forgets NAME when the value is not a constant.
func assign(vars map[string]string, w word) {
	name, value, ok := strings.Cut(strings.TrimLeft(w.text, "("), "=")
	if !ok || vars == nil {
		return
	}
	if w.dyn || w.glob {
		delete(vars, name)
		return
	}
	vars[name] = value
}

func (c command) has(keyword string) bool {
	for _, k := range c.keywords {
		if k == keyword {
			return true
		}
	}
	return false
}
