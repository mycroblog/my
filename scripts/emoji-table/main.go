// SPDX-License-Identifier: AGPL-3.0-only

// Command emoji-table generates, in one run from one emoji-test.txt, the Unicode emoji data of the server and of the
// web client:
//
//   - the table of fully-qualified emoji that the content module normalizes emoji reactions with (ADR-0051 §6), a Go
//     file;
//   - the data of the web client's emoji picker and completion (ADR-0091 §1): a base file of the emoji, their groups,
//     order, versions, and skin tone variants, and a file per locale of their CLDR names and keywords.
//
// `scripts/task.sh emoji:generate` downloads emoji-test.txt (EMOJI_VERSION) and the CLDR annotations (CLDR_VERSION),
// checks their digests, and runs it:
//
//	go run ./scripts/emoji-table -in emoji-test.txt -version 18.0.0 -out internal/domain/content/emoji_table.gen.go \
//	  -cldr .tmp/cldr -cldr-version 49.0.0 -locales en,ja -web-out app/web/src/features/emoji/unicode
//
// The -cldr directory holds annotations/<locale>.json and annotationsDerived/<locale>.json, the files of the same
// paths in the cldr-json packages cldr-annotations-full and cldr-annotations-derived-full.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	in := flag.String("in", "", "the emoji-test.txt to read")
	out := flag.String("out", "", "the Go file of the server's table to write")
	version := flag.String("version", "", "the Unicode version of emoji-test.txt, recorded in the outputs")
	cldrDir := flag.String("cldr", "", "the directory of the CLDR annotations (annotations/<locale>.json, annotationsDerived/<locale>.json)")
	cldrVersion := flag.String("cldr-version", "", "the CLDR version of the annotations, recorded in the outputs")
	locales := flag.String("locales", "en,ja", "the comma-separated locales of the web client's data; en is required, as the fallback")
	webOut := flag.String("web-out", "", "the directory of the web client's data to write")
	flag.Parse()
	if *in == "" || *out == "" || *version == "" || *cldrDir == "" || *cldrVersion == "" || *webOut == "" {
		log.Fatal("emoji-table: -in, -out, -version, -cldr, -cldr-version, and -web-out are required")
	}
	if err := run(*in, *out, *version, *cldrDir, *cldrVersion, strings.Split(*locales, ","), *webOut); err != nil {
		log.Fatalf("emoji-table: %v", err)
	}
}

func run(in, out, version, cldrDir, cldrVersion string, locales []string, webOut string) error {
	src, err := os.ReadFile(in) //nolint:gosec // G304: the input the task passes
	if err != nil {
		return err
	}
	entries, err := parseEmojiTest(src)
	if err != nil {
		return err
	}
	table, err := serverTable(entries, version)
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, table, 0o600); err != nil {
		return err
	}

	set, err := buildEmojiSet(entries)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(webOut, "base.gen.json"), baseJSON(set, version, cldrVersion), 0o600); err != nil {
		return err
	}
	if len(locales) == 0 || locales[0] != "en" {
		return fmt.Errorf("-locales must start with en, the fallback of the other locales")
	}
	var en *annotations
	for _, locale := range locales {
		ann, err := loadAnnotations(cldrDir, locale)
		if err != nil {
			return err
		}
		if en == nil {
			en = ann
		}
		names, fallbacks := buildNames(set, ann, en)
		if fallbacks > 0 {
			fmt.Fprintf(os.Stderr, "emoji-table: %s: %d emoji have no CLDR name and fall back to en or emoji-test.txt\n", locale, fallbacks)
		}
		if err := os.WriteFile(filepath.Join(webOut, locale+".gen.json"), namesJSON(names), 0o600); err != nil {
			return err
		}
	}
	return nil
}
