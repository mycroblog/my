// SPDX-License-Identifier: AGPL-3.0-only

package feedback

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
