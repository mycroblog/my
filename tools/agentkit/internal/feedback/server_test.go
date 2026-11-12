// SPDX-License-Identifier: AGPL-3.0-only

package feedback

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func record(t *testing.T, dir, url string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ServerFile), []byte(`{"url": "`+url+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

var probe = &http.Client{Timeout: time.Second}

func TestURLWithoutAServer(t *testing.T) {
	if _, err := URL(context.Background(), Dir(t.TempDir()), probe); !errors.Is(err, ErrNoServer) {
		t.Fatalf("URL = %v, want ErrNoServer", err)
	}
}

func TestURLOfAStoppedServer(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL + "/"
	srv.Close()
	dir := Dir(t.TempDir())
	record(t, dir, url)
	if _, err := URL(context.Background(), dir, probe); !errors.Is(err, ErrNoServer) {
		t.Fatalf("URL = %v, want ErrNoServer", err)
	}
}

func TestURLOfARunningServer(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	dir := Dir(t.TempDir())
	record(t, dir, srv.URL+"/")
	write(t, dir, "20261112-153045-0001")
	if got, err := URL(context.Background(), dir, probe); err != nil || got != srv.URL+"/" {
		t.Fatalf("URL = %q, %v", got, err)
	}
	// The server file is not a batch.
	if ids, err := IDs(dir); err != nil || len(ids) != 1 || ids[0] != "20261112-153045-0001" {
		t.Fatalf("IDs = %v, %v", ids, err)
	}
}

// sleeper starts a process that sleeps in dir, and returns its pid and a channel closed when it exits.
func sleeper(t *testing.T, dir string) (int, <-chan struct{}) {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
	})
	return cmd.Process.Pid, done
}

func recordPID(t *testing.T, root string, pid int) {
	t.Helper()
	if err := os.MkdirAll(Dir(root), 0o750); err != nil {
		t.Fatal(err)
	}
	body := `{"url": "http://localhost:5173/", "pid": ` + strconv.Itoa(pid) + `}`
	if err := os.WriteFile(filepath.Join(Dir(root), ServerFile), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStopTheServerOfTheWorktree(t *testing.T) {
	root := t.TempDir()
	pid, done := sleeper(t, root)
	recordPID(t, root, pid)
	if got, err := Stop(root, 5*time.Second); err != nil || got != pid {
		t.Fatalf("Stop = %d, %v, want %d", got, err, pid)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the dev server still runs")
	}
}

func TestStopLeavesAProcessOutsideTheWorktree(t *testing.T) {
	root := t.TempDir()
	pid, done := sleeper(t, t.TempDir())
	recordPID(t, root, pid)
	if got, err := Stop(root, time.Second); err != nil || got != 0 {
		t.Fatalf("Stop = %d, %v, want 0", got, err)
	}
	select {
	case <-done:
		t.Fatal("Stop killed a process outside the worktree")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestStopWithAStalePID(t *testing.T) {
	root := t.TempDir()
	pid, done := sleeper(t, root)
	p, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	_ = p.Kill()
	<-done
	recordPID(t, root, pid)
	if got, err := Stop(root, time.Second); err != nil || got != 0 {
		t.Fatalf("Stop = %d, %v, want 0", got, err)
	}
}

func TestStopWithoutAServer(t *testing.T) {
	if got, err := Stop(t.TempDir(), time.Second); err != nil || got != 0 {
		t.Fatalf("Stop = %d, %v, want 0", got, err)
	}
}
