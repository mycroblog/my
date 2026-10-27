// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/app/kernel"
)

// The limit of a scrubbed stack (ADR-0103 §3); kernel.ScrubErrorMessage limits the message.
const (
	maxErrorStackBytes = 16 << 10
)

// modulePrefix starts the function names of this repository in a stack.
const modulePrefix = "github.com/mycroblog/my/"

// fingerprintFrames is how many of this repository's functions of a stack make its fingerprint.
const fingerprintFrames = 5

// scrubGoStack keeps the debug.Stack() form of stack (the goroutine headers, the function lines,
// the file positions with their +0x offsets, the created by lines) and replaces the arguments
// of each function line with (...), the form Go prints for an inlined frame: they are the
// values of the registers, which may point at personal data and change on every run. It drops
// the frames of runtime/debug.Stack, of panic, and of the reporter itself, and cuts the stack
// at 16 KiB on a line boundary (ADR-0103 §3).
func scrubGoStack(stack string) string {
	if stack == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(stack, "\n"), "\n")
	var b strings.Builder
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !isFunctionLine(line) {
			writeStackLine(&b, line)
			continue
		}
		fn := frameFunction(line)
		if isReporterFrame(fn) {
			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "\t") {
				i++ // its file position
			}
			continue
		}
		writeStackLine(&b, scrubArguments(line))
	}
	return cutLines(b.String(), maxErrorStackBytes)
}

func writeStackLine(b *strings.Builder, line string) {
	b.WriteString(line)
	b.WriteByte('\n')
}

// isFunctionLine reports whether line of a debug.Stack() names a function: not a goroutine
// header, not a file position (indented with a tab), not a created by line, and not empty.
func isFunctionLine(line string) bool {
	return line != "" && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "goroutine ") &&
		!strings.HasPrefix(line, "created by ") && strings.HasSuffix(line, ")")
}

// frameFunction returns the function name of a function line, without its arguments.
func frameFunction(line string) string {
	if i := strings.LastIndexByte(line, '('); i > 0 {
		return line[:i]
	}
	return line
}

// scrubArguments replaces the argument list at the end of a function line with (...). An empty
// list stays (). The arguments hold no parenthesis, so the last one opens them.
func scrubArguments(line string) string {
	i := strings.LastIndexByte(line, '(')
	if i < 0 || line[i:] == "()" {
		return line
	}
	return line[:i] + "(...)"
}

// reporterFrames are the functions of the reporting itself, which a stack leaves out: they are
// the same in every report and say nothing about the error.
var reporterFrames = []string{
	"runtime/debug.Stack",
	"panic",
	"runtime.gopanic",
	modulePrefix + "internal/app/kernel.RecoverPanic",
	modulePrefix + "internal/app/kernel.NewErrorEvent",
	modulePrefix + "internal/app/kernel.WithStack",
	modulePrefix + "internal/app/kernel.NopErrorReporter.Recover",
	modulePrefix + "internal/app/kernel.(NopErrorReporter).Recover",
	modulePrefix + "internal/app/kernel.Go.func1.1",
	modulePrefix + "internal/platform.(*ErrorReporter).",
}

func isReporterFrame(fn string) bool {
	for _, f := range reporterFrames {
		if fn == f || strings.HasSuffix(f, ".") && strings.HasPrefix(fn, f) {
			return true
		}
	}
	return false
}

// cutLines returns s cut to at most limit bytes on a line boundary.
func cutLines(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	s = s[:limit]
	if i := strings.LastIndexByte(s, '\n'); i > 0 {
		return s[:i+1]
	}
	return truncateUTF8(s, limit)
}

// truncateUTF8 returns s cut to at most limit bytes without splitting a character.
func truncateUTF8(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit]
}

// scrubEvent scrubs ev's message and stack (ADR-0103 §3). The stack of a server event is a Go
// stack; the stack of the web client is only cut, and its own parsing belongs to its endpoint.
func scrubEvent(ev *kernel.ErrorEvent) {
	ev.Message = kernel.ScrubErrorMessage(ev.Message)
	if ev.Source == kernel.ErrorSourceServer {
		ev.Stack = scrubGoStack(ev.Stack)
	} else {
		ev.Stack = cutLines(ev.Stack, maxErrorStackBytes)
	}
}

// fingerprint returns the fingerprint of the scrubbed ev (ADR-0103 §1): the first 32 hex digits
// of the SHA-256 of its source, its place, the last type of its chain, and the names of the
// first five functions of this repository in its stack, or its scrubbed message when the stack
// has none. Line numbers are left out, so that unrelated lines do not change it.
func fingerprint(ev kernel.ErrorEvent) string {
	lastType := ev.Type
	if i := strings.LastIndex(lastType, " > "); i >= 0 {
		lastType = lastType[i+len(" > "):]
	}
	frames := moduleFrames(ev.Stack, fingerprintFrames)
	tail := strings.Join(frames, "\n")
	if len(frames) == 0 {
		tail = ev.Message
	}
	h := sha256.New()
	for _, part := range []string{string(ev.Source), ev.Where, lastType, tail} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// moduleFrames returns the names of the first n functions of this repository in a Go stack.
func moduleFrames(stack string, n int) []string {
	var frames []string
	for line := range strings.Lines(stack) {
		line = strings.TrimSuffix(line, "\n")
		if !isFunctionLine(line) {
			continue
		}
		if fn := frameFunction(line); strings.HasPrefix(fn, modulePrefix) {
			frames = append(frames, fn)
			if len(frames) == n {
				break
			}
		}
	}
	return frames
}
