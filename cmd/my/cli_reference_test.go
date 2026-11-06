// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins && !no_sqlite && !no_postgres && !e2e_fakes

// The reference is of the standard build: a build tag that leaves out or adds a command changes the page.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// cliReferencePath is the generated command reference, relative to the repository root.
const cliReferencePath = "docs/reference/cli.md"

var updateCLIReference = flag.Bool("update", false, "regenerate "+cliReferencePath)

// cliBuildTagNotes are the commands whose work a build tag leaves out, and what such a build does instead. The
// tags only remove or add code, which no single build can run, so the generator names them here.
var cliBuildTagNotes = map[string]string{
	"plugin": "A binary built with the build tag `no_plugins` has no plugins: `my plugin` and its subcommands " +
		"exit with 1 and say so.",
	"db backup": "A binary built with the build tag `no_sqlite` has no SQLite and refuses to back up.",
	"schema apply": "A binary built with the build tag `no_sqlite` or `no_postgres` cannot apply the schema of " +
		"the database that the tag leaves out.",
}

// cliCommand is one command of my, as running it with -help shows it.
type cliCommand struct {
	words []string      // after my: ["admin", "role", "grant"]
	help  string        // what -help prints
	subs  []string      // the subcommands, sorted; none for a command that does the work itself
	flags *flag.FlagSet // the command's own flags, or nil
	child []*cliCommand // the subcommands, in the order of subs
}

func (c *cliCommand) name() string { return strings.TrimSpace("my " + strings.Join(c.words, " ")) }

// walkCLI runs `my <words> -help` for my and each of its subcommands, depth first, and learns each command's
// subcommands and flags through the hooks of clitrace.go.
func walkCLI(t *testing.T) *cliCommand {
	t.Helper()
	defer func() { traceSubcommands, traceFlagSet = nil, nil }()
	var walk func(words []string) *cliCommand
	walk = func(words []string) *cliCommand {
		c := &cliCommand{words: words}
		joined := strings.Join(words, " ")
		traceSubcommands = func(cmd string, subs []string) {
			if cmd == joined {
				c.subs = slices.Sorted(slices.Values(subs))
			}
		}
		traceFlagSet = func(fs *flag.FlagSet) {
			if fs.Name() == c.name() {
				c.flags = fs
			}
		}
		var stdout, stderr bytes.Buffer
		if code := run(append(slices.Clone(words), "-help"), &stdout, &stderr); code != 0 || stdout.Len() > 0 {
			t.Fatalf("%s -help = %d, want 0 and only the help on standard error; stdout:\n%s\nstderr:\n%s",
				c.name(), code, stdout.String(), stderr.String())
		}
		c.help = stderr.String()
		if len(c.subs) > 0 {
			c.flags = nil // the flag set of a command that picks a subcommand is not the subcommand's
		}
		for _, s := range c.subs {
			c.child = append(c.child, walk(append(slices.Clone(words), s)))
		}
		return c
	}
	return walk(nil)
}

// TestCLIReferenceUpToDate fails when the committed command reference differs from the commands of my.
// Regenerate it with `scripts/task.sh go:cli-docs`.
func TestCLIReferenceUpToDate(t *testing.T) {
	root := walkCLI(t)
	for name := range cliBuildTagNotes {
		if find(root, strings.Fields(name)) == nil {
			t.Errorf("cliBuildTagNotes names %q, which is not a command of my", name)
		}
	}
	want := cliReference(t, root)
	path := filepath.Join("..", "..", filepath.FromSlash(cliReferencePath))
	if *updateCLIReference {
		if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path) //nolint:gosec // fixed path inside the repository
	if err != nil {
		t.Fatalf("%v; run `scripts/task.sh go:cli-docs`", err)
	}
	if string(got) != want {
		t.Errorf("%s is stale; run `scripts/task.sh go:cli-docs`", cliReferencePath)
	}
}

func find(c *cliCommand, words []string) *cliCommand {
	if len(words) == 0 {
		return c
	}
	for _, ch := range c.child {
		if ch.words[len(ch.words)-1] == words[0] {
			return find(ch, words[1:])
		}
	}
	return nil
}

// reDocsLine is the last line of a help that links its page on the documentation site.
var reDocsLine = regexp.MustCompile(`(?m)^Documentation: https://\S+/next/([a-z0-9/-]+)/(#[a-z0-9-]+)?\n?`)

// cliReference renders the reference page.
func cliReference(t *testing.T, root *cliCommand) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(`<!-- Code generated from the commands of cmd/my by "scripts/task.sh go:cli-docs". DO NOT EDIT. -->

# Command reference

` + "`my`" + ` is the single binary of the server. ` + "`my serve`" + ` runs the server; the other commands set up, operate,
and upgrade an instance from the command line, against the same database and storage as the server.
Every command prints its help with ` + "`-help`" + `. This page lists each command of the standard build with its usage,
its flags and their defaults, and its help.

Most commands read the configuration as the server does: from the environment, and from the TOML file named by
` + "`-config`" + ` or ` + "`MY_CONFIG`" + ` ([configuration reference](configuration.md)). A command exits with 2 when its command
line is wrong, without changing anything.

A binary built with build tags that leave adapters out has the same commands; the ones that need what a tag left out
are marked below and fail with a message that names the tag. A test build adds ` + "`my dev push-service`" + `
([build tags](build-tags.md)).
`)
	var render func(c, parent *cliCommand)
	render = func(c, parent *cliCommand) {
		level := "###"
		if len(c.words) <= 1 {
			level = "##"
		}
		blocks := []string{fmt.Sprintf("%s `%s`", level, c.name())}
		if note, ok := cliBuildTagNotes[strings.Join(c.words, " ")]; ok {
			blocks = append(blocks, note)
		}
		help, docs := splitDocsLine(t, c.help)
		if len(c.subs) == 0 {
			blocks = append(blocks, "```text\n"+strings.Join(synopsis(c, help), "\n")+"\n```")
			if table := flagTable(c.flags); table != "" {
				blocks = append(blocks, table)
			}
		} else {
			subs := make([]string, len(c.child))
			for i, ch := range c.child {
				subs[i] = fmt.Sprintf("[`%s`](#%s)", ch.words[len(ch.words)-1], anchor(ch.name()))
			}
			blocks = append(blocks, "Subcommands: "+strings.Join(subs, ", ")+".")
		}
		parentHelp := ""
		if parent != nil {
			parentHelp, _ = splitDocsLine(t, parent.help)
		}
		switch {
		case help == parentHelp:
			blocks = append(blocks, fmt.Sprintf("Its help is the help of [`%s`](#%s).", parent.name(), anchor(parent.name())))
			docs = "" // the parent's
		case strings.HasPrefix(help, "Usage of "+c.name()+":"):
			// The flag package's own help: the usage and the flags above say it all.
		default:
			blocks = append(blocks, "```text\n"+strings.TrimRight(help, "\n")+"\n```")
		}
		if docs != "" {
			blocks = append(blocks, docs)
		}
		b.WriteString("\n" + strings.Join(blocks, "\n\n") + "\n")
		for _, ch := range c.child {
			render(ch, c)
		}
	}
	render(root, nil)
	return b.String()
}

// splitDocsLine returns the help without its line that links the documentation site, and that link as a link to
// the document in the repository, relative to the reference. The link names the document by its path, so that the
// reference changes only with the commands.
func splitDocsLine(t *testing.T, help string) (string, string) {
	t.Helper()
	m := reDocsLine.FindStringSubmatch(help)
	if m == nil {
		return help, ""
	}
	rest := strings.TrimRight(strings.Replace(help, m[0], "", 1), "\n") + "\n"
	doc := "docs/" + m[1] + ".md"
	if _, err := os.Stat(filepath.Join("..", "..", filepath.FromSlash(doc))); err != nil {
		t.Fatalf("the help links %s, whose document %s is missing: %v", m[0], doc, err)
	}
	rel, err := filepath.Rel(filepath.Dir(cliReferencePath), doc)
	if err != nil {
		t.Fatal(err)
	}
	return rest, fmt.Sprintf("Documentation: [%s](%s%s)", doc, filepath.ToSlash(rel), m[2])
}

// synopsis returns the lines of the usage in help that start with the command's name, each with the lines that
// continue it, or the name alone when the help has none.
func synopsis(c *cliCommand, help string) []string {
	name := c.name()
	lines := strings.Split(help, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		line := strings.TrimPrefix(lines[i], "Usage:")
		text := strings.TrimLeft(line, " ")
		if text != name && !strings.HasPrefix(text, name+" ") {
			continue
		}
		if j := strings.Index(text, "   ("); j > 0 { // the pointer to the subcommand's own help
			text = strings.TrimRight(text[:j], " ")
		}
		out = append(out, text)
		indent := len(line) - len(text)
		for i+1 < len(lines) {
			next := strings.TrimLeft(lines[i+1], " ")
			if len(lines[i+1])-len(next) <= indent || next == "" || !strings.ContainsAny(next[:1], "[-<") {
				break
			}
			i++
			out = append(out, strings.Repeat(" ", len(name)+1)+next)
		}
	}
	if len(out) > 0 {
		return out
	}
	if c.flags != nil && hasFlags(c.flags) {
		return []string{name + " [flags]"}
	}
	return []string{name}
}

func hasFlags(fs *flag.FlagSet) bool {
	n := 0
	fs.VisitAll(func(*flag.Flag) { n++ })
	return n > 0
}

// flagTable returns the table of the flags of fs, or "" when it has none.
func flagTable(fs *flag.FlagSet) string {
	if fs == nil || !hasFlags(fs) {
		return ""
	}
	rows := []string{"| Flag | Default | Description |", "| --- | --- | --- |"}
	fs.VisitAll(func(f *flag.Flag) {
		kind, usage := flag.UnquoteUsage(f)
		flagCell := "`-" + f.Name
		if kind != "" {
			flagCell += " <" + kind + ">"
		}
		flagCell += "`"
		def := ""
		if f.DefValue != "" {
			def = "`" + f.DefValue + "`"
		}
		usage = strings.ReplaceAll(strings.ReplaceAll(usage, "|", `\|`), "\n", " ")
		rows = append(rows, fmt.Sprintf("| %s | %s | %s |", flagCell, def, usage))
	})
	return strings.Join(rows, "\n")
}

// anchor returns the anchor of a heading `my admin role` on the documentation site and on GitHub.
func anchor(name string) string { return strings.ReplaceAll(name, " ", "-") }
