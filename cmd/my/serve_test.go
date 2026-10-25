// SPDX-License-Identifier: AGPL-3.0-only

//go:build unix

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for applying the schema

	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/schema"
)

func TestServeArguments(t *testing.T) {
	var stderr bytes.Buffer
	if code := run([]string{"serve", "extra"}, &bytes.Buffer{}, &stderr); code != 2 || !strings.Contains(stderr.String(), `unexpected argument "extra"`) {
		t.Errorf("serve extra = %d %q, want 2", code, stderr.String())
	}
	t.Setenv("MY_HOST", "")
	t.Setenv("LANGUAGE", "")
	t.Setenv("LC_ALL", "C")
	stderr.Reset()
	if code := run([]string{"serve"}, &bytes.Buffer{}, &stderr); code != 1 || !strings.Contains(stderr.String(), "The required setting MY_HOST is not set.") {
		t.Errorf("serve with a bad configuration = %d %q, want 1", code, stderr.String())
	}
}

// `my serve` with a SQLite configuration answers the health checks and stops on SIGTERM.
func TestServeStopsOnSIGTERM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), schema.SQLite); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	kek := make([]byte, 32)
	_, _ = rand.Read(kek)
	addr := freeAddr(t)
	t.Setenv("MY_HOST", "example.test")
	t.Setenv("MY_DATABASE_URL", path)
	t.Setenv("MY_BLOB_FS_DIR", t.TempDir())
	t.Setenv("MY_KEK", base64.StdEncoding.EncodeToString(kek))
	t.Setenv("MY_LISTEN", addr)

	var stderr bytes.Buffer
	done := make(chan int, 1)
	go func() { done <- run([]string{"serve"}, &bytes.Buffer{}, &stderr) }()

	waitReady(t, "http://"+addr+"/readyz", done)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("serve exited with %d, want 0; stderr:\n%s", code, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop after SIGTERM")
	}
	if !strings.Contains(stderr.String(), `"msg":"stopped"`) {
		t.Errorf("stderr lacks the stop log:\n%s", stderr.String())
	}
}

// `my serve` in the maintenance mode starts with adapter settings that are invalid or unreachable, never assembles
// the normal server, answers the probes with 200 and the client API with the maintenance problem, and stops on SIGTERM.
func TestServeMaintenanceOpensNoAdapter(t *testing.T) {
	newApp = func(context.Context, *config.Config, platform.AppOptions) (*platform.App, error) {
		t.Error("the maintenance mode assembled the normal server")
		return nil, errors.New("not in the maintenance mode")
	}
	t.Cleanup(func() { newApp = platform.NewApp })
	addr := freeAddr(t)
	t.Setenv("MY_MAINTENANCE", "true")
	t.Setenv("MY_HOST", "example.test")
	t.Setenv("MY_LISTEN", addr)
	t.Setenv("MY_MAINTENANCE_MESSAGE", "Back soon.")
	t.Setenv("MY_DB", "postgres")
	t.Setenv("MY_DATABASE_URL", "postgres://unreachable.invalid:5432/my?connect_timeout=1")
	t.Setenv("MY_QUEUE", "nosuchqueue")
	t.Setenv("MY_BLOB", "nosuchblob")
	t.Setenv("MY_KV", "valkey")
	t.Setenv("MY_KEK", "")

	var stderr bytes.Buffer
	done := make(chan int, 1)
	go func() { done <- run([]string{"serve"}, &bytes.Buffer{}, &stderr) }()
	waitReady(t, "http://"+addr+"/readyz", done)

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/api/v1/instance", nil)
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("Content-Type") != "application/problem+json" ||
		!strings.Contains(string(body), `"message":"Back soon."`) || resp.Header.Get("X-Request-Id") == "" {
		t.Errorf("GET /api/v1/instance = %d %v %s", resp.StatusCode, resp.Header, body)
	}
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("serve exited with %d, want 0; stderr:\n%s", code, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop after SIGTERM")
	}
	if !strings.Contains(stderr.String(), `"msg":"started in maintenance mode"`) {
		t.Errorf("stderr lacks the start log:\n%s", stderr.String())
	}
}

// freeAddr returns a local address with a port that was free a moment ago.
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// waitReady polls url until it answers 200, failing when serve exits first.
func waitReady(t *testing.T, url string, done <-chan int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case code := <-done:
			t.Fatalf("serve exited with %d before it was ready", code)
		default:
		}
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if resp, err := http.DefaultTransport.RoundTrip(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("serve did not become ready")
}
