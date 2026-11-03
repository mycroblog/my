// SPDX-License-Identifier: AGPL-3.0-only

// Command releaseprose writes the prose of a release's notes with Claude, around the facts that
// tools/agentkit/cmd/releasenotes derived from the repository (docs/agents/releasing.md, "Release notes").
// scripts/release-notes.sh runs it on the directory that its `facts` and `prs` commands filled.
//
// The facts are never left to the model: they go in verbatim as input and verbatim in the body. A machine check of the
// prose (check.go) decides whether it joins them. The model is asked again when the check fails, and when every attempt
// fails the body is the facts alone. Only an unusable input directory is an error, and the release's draft then keeps
// the facts it was created with.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	var dir, version string
	var dryRun bool
	flag.StringVar(&dir, "dir", "", "the directory of the facts and the pull requests; body.md is written to it")
	flag.StringVar(&version, "version", "", "the version being released")
	flag.BoolVar(&dryRun, "dry-run", false, "print the input the model would get, and call nothing")
	flag.Parse()
	if err := run(dir, version, dryRun); err != nil {
		fmt.Fprintln(os.Stderr, "releaseprose:", err)
		os.Exit(1)
	}
}

func run(dir, version string, dryRun bool) error {
	in, err := readInput(dir, version)
	if err != nil {
		return err
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	req := assemble(in, hex.EncodeToString(nonce))
	if dryRun {
		fmt.Printf("===== system prompt =====\n%s\n===== user message =====\n%s\n===== %s; %d bytes =====\n", req.system, req.user, req.stats, len(req.user))
		return nil
	}
	out := write(context.Background(), claudeCLI{path: "claude", timeout: 15 * time.Minute}, in, req, 30*time.Second)
	if err := os.WriteFile(filepath.Join(dir, "body.md"), []byte(out.body), 0o600); err != nil {
		return err
	}
	fmt.Println(out.status)
	return nil
}
