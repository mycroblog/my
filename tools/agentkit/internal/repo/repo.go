// SPDX-License-Identifier: AGPL-3.0-only

// Package repo reads the git working tree agentkit runs in: its root, the directory every worktree shares,
// the tracked files with their blob hashes, and the files changed since a base revision.
package repo

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // git's object id, not a security boundary
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Trunk is the branch pull requests target and branches are rebased onto: redesign-web, the integration branch of
// #2863, which reaches the default branch, main, in one merge. Its pull requests do not close issues by keyword, so
// `pr land` closes them itself and leaves their parents open. A variable so that tests can run on either.
var Trunk = "redesign-web"

// Default is the repository's default branch.
const Default = "main"

// Repo is one working tree.
type Repo struct {
	Root   string // the working tree
	Common string // the git directory every worktree of the repository shares
}

// Open finds the working tree that contains dir.
func Open(dir string) (*Repo, error) {
	root, err := Git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not in a git working tree: %w", err)
	}
	common, err := Git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	return &Repo{Root: strings.TrimSpace(root), Common: strings.TrimSpace(common)}, nil
}

// Git runs git in dir and returns its standard output.
func Git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// File is a file of the working tree and the git blob id of its current content.
type File struct {
	Path string
	SHA  string
}

// Files lists the tracked and untracked (not ignored) files of the working tree with the blob ids of their
// current content: the index's id for unmodified files, a hash of the working copy for the others.
// Files deleted from the working tree are left out.
func (r *Repo) Files() ([]File, error) {
	staged, err := Git(r.Root, "ls-files", "-s", "-z")
	if err != nil {
		return nil, err
	}
	shas := map[string]string{}
	for _, rec := range strings.Split(staged, "\x00") {
		// <mode> <sha> <stage>\t<path>
		tab := strings.IndexByte(rec, '\t')
		if tab < 0 {
			continue
		}
		f := strings.Fields(rec[:tab])
		if len(f) == 3 && f[0] != "160000" { // skip submodules
			shas[rec[tab+1:]] = f[1]
		}
	}
	dirty, err := Git(r.Root, "ls-files", "-z", "--modified", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	for _, p := range strings.Split(dirty, "\x00") {
		if p == "" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(r.Root, p))
		if err != nil {
			delete(shas, p) // deleted in the working tree
			continue
		}
		shas[p] = BlobSHA(b)
	}
	out := make([]File, 0, len(shas))
	for p, s := range shas {
		out = append(out, File{Path: p, SHA: s})
	}
	return out, nil
}

// BlobSHA is git's object id of a blob with content b, so that a working copy equal to a committed file
// shares its id.
func BlobSHA(b []byte) string {
	h := sha1.New() //nolint:gosec // git's object id
	h.Write([]byte("blob " + strconv.Itoa(len(b)) + "\x00"))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// Changed lists the files changed since the merge base of base and HEAD: committed, uncommitted and untracked.
func (r *Repo) Changed(base string) ([]string, error) {
	mb, err := Git(r.Root, "merge-base", base, "HEAD")
	if err != nil {
		return nil, err
	}
	diff, err := Git(r.Root, "diff", "--name-only", strings.TrimSpace(mb), "--")
	if err != nil {
		return nil, err
	}
	untracked, err := Git(r.Root, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(diff+untracked, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

// Branch returns the current branch name.
func (r *Repo) Branch() (string, error) {
	b, err := Git(r.Root, "rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(b), err
}

// TmpDir returns (and creates) .tmp/agentkit/<sub> of the working tree, the scratch space the repository ignores.
func (r *Repo) TmpDir(sub string) (string, error) {
	d := filepath.Join(r.Root, ".tmp", "agentkit", sub)
	return d, os.MkdirAll(d, 0o750)
}
