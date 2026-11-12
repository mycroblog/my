// SPDX-License-Identifier: AGPL-3.0-only

// Command emoji-table generates, from one emoji-test.txt, the table of fully-qualified emoji that the content module
// normalizes emoji reactions with (ADR-0051 §6), a Go file.
//
// `scripts/task.sh emoji:generate` downloads emoji-test.txt (EMOJI_VERSION), checks its digest, and runs it:
//
//	go run ./scripts/emoji-table -in emoji-test.txt -version 18.0.0 -out internal/domain/content/emoji_table.gen.go
package main

import (
	"flag"
	"log"
	"os"
)

func main() {
	in := flag.String("in", "", "the emoji-test.txt to read")
	out := flag.String("out", "", "the Go file of the server's table to write")
	version := flag.String("version", "", "the Unicode version of emoji-test.txt, recorded in the output")
	flag.Parse()
	if *in == "" || *out == "" || *version == "" {
		log.Fatal("emoji-table: -in, -out, and -version are required")
	}
	if err := run(*in, *out, *version); err != nil {
		log.Fatalf("emoji-table: %v", err)
	}
}

func run(in, out, version string) error {
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
	return os.WriteFile(out, table, 0o600)
}
