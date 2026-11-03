// SPDX-License-Identifier: AGPL-3.0-only

// Command releasenotes writes the starting draft of the release notes' sections (docs/operations/releasing.md) from
// the files of the release and of the previous one. scripts/release-notes.sh runs it with the files it took from git,
// and scripts/release-notes-test.sh tests the two together. It has a main of its own, outside agentkit, so that the
// release job compiles only this and its YAML parser.
//
// It writes three files to -out: sections.md (Upgrading, Breaking changes, API changes), mykit.md, and mybot.md (the
// text that goes in each library's section).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// The files read from -old and -new, relative to the repository's root.
const (
	upgradesFile = "internal/platform/setup_dbupgrade.go"
	retiredFile  = "internal/platform/config/retired.go"
	configFile   = "docs/reference/configuration.md"
	openAPIFile  = "api/openapi.yaml"
)

// maxItems caps each list: a GitHub Release body holds 125,000 characters, and the tags exist by the time it is created.
const maxItems = 100

type options struct {
	version, previous, today string
	oldDir, newDir           string
	commitsFile, schemaFile  string
	outDir                   string
}

func main() {
	var o options
	flag.StringVar(&o.version, "version", "", "the version being released")
	flag.StringVar(&o.previous, "previous", "", "the release compared with; empty for the first release")
	flag.StringVar(&o.today, "today", "", "today, as YYYY-MM-DD, to tell which Sunset dates have passed")
	flag.StringVar(&o.oldDir, "old", "", "the files of the previous release")
	flag.StringVar(&o.newDir, "new", "", "the files of this release")
	flag.StringVar(&o.commitsFile, "commits", "", "the commits marked !, one per line: <sha> TAB <subject> TAB <comma-separated paths>")
	flag.StringVar(&o.schemaFile, "schema", "", "the schema files changed since the previous release, one per line")
	flag.StringVar(&o.outDir, "out", "", "the directory to write to")
	flag.Parse()
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "releasenotes:", err)
		os.Exit(1)
	}
}

func run(o options) error {
	cur, err := readFacts(o.newDir, false)
	if err != nil {
		return err
	}
	var d draft
	if o.previous == "" {
		d = firstRelease(o, cur)
	} else {
		old, err := readFacts(o.oldDir, true)
		if err != nil {
			return err
		}
		commits, err := readCommits(o.commitsFile)
		if err != nil {
			return err
		}
		schema, err := readLines(o.schemaFile)
		if err != nil {
			return err
		}
		d = laterRelease(o, old, cur, commits, schema)
	}
	for name, text := range map[string]string{"sections.md": d.sections, "mykit.md": d.mykit, "mybot.md": d.mybot} {
		if err := os.WriteFile(filepath.Join(o.outDir, name), []byte(text), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// facts are what a release's files say.
type facts struct {
	steps    []upgradeStep
	prereqs  []prerequisite
	retired  []retiredSetting
	settings map[string]setting
	api      apiSpec
}

// readFacts reads the files of a release. A file that a previous release does not have yet counts as empty
// (optional); a release's own files must all exist and parse, so that a file that moved never reads as "None".
func readFacts(dir string, optional bool) (facts, error) {
	var f facts
	read := func(file string, parse func(path string) error) error {
		path := filepath.Join(dir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) && optional {
			return nil
		}
		if err := parse(path); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		return nil
	}
	err := read(upgradesFile, func(path string) (err error) {
		f.steps, f.prereqs, err = parseUpgrades(path)
		return err
	})
	if err == nil {
		err = read(retiredFile, func(path string) (err error) {
			f.retired, err = parseRetired(path)
			return err
		})
	}
	if err == nil {
		err = read(configFile, func(path string) (err error) {
			f.settings, err = parseSettings(path)
			return err
		})
	}
	if err == nil {
		err = read(openAPIFile, func(path string) (err error) {
			f.api, err = parseOpenAPI(path)
			return err
		})
	}
	return f, err
}

// commit is a commit marked ! in Conventional Commits.
type commit struct {
	sha, subject string
	paths        []string
}

var breakingSubject = regexp.MustCompile(`^[a-z]+(?:\(([^)]*)\))?!: `)

// areas are the scopes of the subject, or "general" for a commit without a scope.
func (c commit) areas() []string {
	m := breakingSubject.FindStringSubmatch(c.subject)
	if m == nil || m[1] == "" {
		return []string{"general"}
	}
	var areas []string
	for a := range strings.SplitSeq(m[1], ",") {
		areas = append(areas, strings.TrimSpace(a))
	}
	return areas
}

// notHandWritten matches the files of a library that its users do not call by hand: its generated client and its tests.
var notHandWritten = regexp.MustCompile(`(\.gen\.[a-z]+|_test\.go|\.test\.ts)$`)

// touches tells whether the commit is about the library: by its scope, or by a hand-written file.
func (c commit) touches(library string) bool {
	if slices.Contains(c.areas(), library) {
		return true
	}
	for _, p := range c.paths {
		if strings.HasPrefix(p, "pkg/"+library+"/") && !notHandWritten.MatchString(p) {
			return true
		}
	}
	return false
}

func (c commit) item() string { return fmt.Sprintf("`%s` %s", c.sha, c.subject) }

func readCommits(path string) ([]commit, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}
	commits := make([]commit, 0, len(lines))
	for _, line := range lines {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			return nil, fmt.Errorf("%s: not <sha> TAB <subject> TAB <paths>: %q", path, line)
		}
		commits = append(commits, commit{sha: fields[0], subject: fields[1], paths: strings.Split(fields[2], ",")})
	}
	return commits, nil
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.FieldsFunc(string(data), func(r rune) bool { return r == '\n' }), nil
}
