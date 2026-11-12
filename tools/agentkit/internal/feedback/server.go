// SPDX-License-Identifier: AGPL-3.0-only

package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ServerFile is the file of Dir in which the dev server records its URL and pid while it listens
// (web/devtools/src/vite.ts); it is not a batch.
const ServerFile = "server.json"

// StartCommand starts the dev server of the catalog page.
const StartCommand = "scripts/task.sh web:catalog"

// ErrNoServer is returned when no dev server is recorded, or the recorded one does not answer.
var ErrNoServer = errors.New("no dev server runs: start it with " + StartCommand)

// server is what the dev server records in ServerFile.
type server struct {
	URL string `json:"url"`
	PID int    `json:"pid"`
}

func recorded(dir string) (server, error) {
	var s server
	data, err := os.ReadFile(filepath.Join(dir, ServerFile))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("%s: %w", ServerFile, err)
	}
	return s, nil
}

// Recorded returns the URL the dev server recorded in dir, or "" when it recorded none.
func Recorded(dir string) (string, error) {
	s, err := recorded(dir)
	return s.URL, err
}

// Stop stops the dev server of the worktree at root, before the worktree is removed, and returns its pid, or 0 when
// it stopped none: none is recorded, its process is gone (a stale pid), or the process is not one of the worktree
// (the pid went to another process). It signals only a process whose working directory or command line is inside
// root, and waits up to wait for it to exit.
func Stop(root string, wait time.Duration) (int, error) {
	s, err := recorded(Dir(root))
	if err != nil || s.PID <= 0 || s.PID == os.Getpid() || !alive(s.PID) || !ofWorktree(s.PID, root) {
		return 0, err
	}
	p, err := os.FindProcess(s.PID)
	if err != nil {
		return 0, err
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		return 0, nil // it exited in between
	}
	for deadline := time.Now().Add(wait); alive(s.PID); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			return s.PID, fmt.Errorf("the dev server (pid %d) did not stop within %s: stop it with kill %d", s.PID, wait, s.PID)
		}
	}
	return s.PID, nil
}

// alive reports whether a process has pid.
func alive(pid int) bool {
	p, err := os.FindProcess(pid)
	return err == nil && p.Signal(syscall.Signal(0)) == nil
}

// ofWorktree reports whether the process pid works in root: its working directory, or an absolute path on its
// command line, is inside root. Where /proc is missing, it reads the command line from ps. When it can read
// neither, the process is not the worktree's.
func ofWorktree(pid int, root string) bool {
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	proc := filepath.Join("/proc", strconv.Itoa(pid))
	if cwd, err := os.Readlink(filepath.Join(proc, "cwd")); err == nil && within(cwd, root) {
		return true
	}
	var args []string
	if data, err := os.ReadFile(filepath.Join(proc, "cmdline")); err == nil {
		args = strings.Split(string(data), "\x00")
	} else if out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output(); err == nil {
		args = strings.Fields(string(out))
	}
	for _, a := range args {
		if filepath.IsAbs(a) && within(filepath.Clean(a), root) {
			return true
		}
	}
	return false
}

// within reports whether path is dir or below it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// URL returns the URL of the running dev server: the recorded one, when it answers an HTTP request (any status) with
// client. Otherwise it returns ErrNoServer.
func URL(ctx context.Context, dir string, client *http.Client) (string, error) {
	url, err := Recorded(dir)
	if err != nil {
		return "", err
	}
	if url == "" {
		return "", ErrNoServer
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("%s: %w", ServerFile, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		// A server that stopped without removing the file (a killed process).
		return "", ErrNoServer
	}
	_ = resp.Body.Close()
	return url, nil
}
