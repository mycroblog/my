// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/platform"
)

// pprofGet reads path from the listener at addr.
func pprofGet(t *testing.T, addr net.Addr, path string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr.String()+path, nil)
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

// gzipped reports whether b starts as a gzip stream, the encoding of a profile that go tool pprof
// reads.
func gzipped(b []byte) bool { return len(b) > 2 && b[0] == 0x1f && b[1] == 0x8b }

// TestPprofListener serves the Go runtime's profiles on their own listener, in every role, never
// on the public one, and until the process stops.
func TestPprofListener(t *testing.T) {
	t.Parallel()
	for _, role := range []string{"all", "web", "worker"} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			cfg := loadConfig(t, map[string]string{
				"MY_DATABASE_URL": newDatabase(t, true), "MY_ROLE": role, "MY_PPROF_LISTEN": "127.0.0.1:6060",
			})
			pln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			s := start(t, cfg, platform.ServeOptions{PprofListener: pln}, nil)

			if status, body := pprofGet(t, pln.Addr(), platform.PathPprof); status != http.StatusOK || !strings.Contains(string(body), "heap") {
				t.Errorf("GET %s = %d, want the index of the profiles", platform.PathPprof, status)
			}
			for _, profile := range []string{"heap", "allocs", "goroutine"} {
				status, body := pprofGet(t, pln.Addr(), platform.PathPprof+profile)
				if status != http.StatusOK || !gzipped(body) {
					t.Errorf("GET %s%s = %d and %d bytes, want a gzipped profile", platform.PathPprof, profile, status, len(body))
				}
			}
			// The process takes one CPU profile at a time, and the roles run in this one.
			if role == "all" {
				if status, body := pprofGet(t, pln.Addr(), platform.PathPprof+"profile?seconds=1"); status != http.StatusOK || !gzipped(body) {
					t.Errorf("GET %sprofile = %d and %d bytes, want a CPU profile", platform.PathPprof, status, len(body))
				}
				pprofSamples(t, pln.Addr())
			}

			// The public listener answers these paths as any other it does not know.
			if _, body := s.do(t, http.MethodGet, platform.PathPprof, nil); strings.Contains(body, "goroutine") {
				t.Errorf("the public listener serves the index of the profiles")
			}
			if _, body := s.do(t, http.MethodGet, platform.PathPprof+"heap", nil); gzipped([]byte(body)) {
				t.Error("the public listener serves the heap profile")
			}

			s.stop()
			select {
			case err := <-s.done:
				s.done <- err
			case <-time.After(10 * time.Second):
				t.Fatal("Serve did not return")
			}
			if conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(t.Context(), "tcp", pln.Addr().String()); err == nil {
				_ = conn.Close()
				t.Error("the listener of the profiles is open after Serve returned")
			}
		})
	}
}

// TestPprofListenerOff opens no listener for the profiles unless pprof_listen names one.
func TestPprofListenerOff(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	pln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := start(t, cfg, platform.ServeOptions{PprofListener: pln}, nil)
	if resp, _ := s.do(t, http.MethodGet, platform.PathLiveness, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d", platform.PathLiveness, resp.StatusCode)
	}
	if conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(t.Context(), "tcp", pln.Addr().String()); err == nil {
		_ = conn.Close()
		t.Error("a listener serves the profiles without pprof_listen")
	}
	if strings.Contains(s.logs.String(), "serving profiles") {
		t.Error("the log says the profiles are served")
	}
}

// pprofSamples checks that the block and mutex profiles sample only while a request with seconds
// takes them: the runtime's mutex sampling is on during the request and off after, and each
// answers a gzipped profile.
func pprofSamples(t *testing.T, addr net.Addr) {
	t.Helper()
	if rate := runtime.SetMutexProfileFraction(-1); rate != 0 {
		t.Fatalf("mutex sampling is %d before any request, want off", rate)
	}
	type answer struct {
		status int
		body   []byte
	}
	done := make(chan answer, 1)
	go func() {
		status, body := pprofGet(t, addr, platform.PathPprof+"mutex?seconds=1")
		done <- answer{status, body}
	}()
	sampling := false
	for deadline := time.Now().Add(5 * time.Second); !sampling && time.Now().Before(deadline); {
		sampling = runtime.SetMutexProfileFraction(-1) > 0
		runtime.Gosched()
	}
	if !sampling {
		t.Error("mutex sampling stayed off during GET mutex?seconds=1")
	}
	if a := <-done; a.status != http.StatusOK || !gzipped(a.body) {
		t.Errorf("GET mutex?seconds=1 = %d and %d bytes, want a gzipped profile", a.status, len(a.body))
	}
	if rate := runtime.SetMutexProfileFraction(-1); rate != 0 {
		t.Errorf("mutex sampling is %d after the request, want off", rate)
	}
	if status, body := pprofGet(t, addr, platform.PathPprof+"block?seconds=1"); status != http.StatusOK || !gzipped(body) {
		t.Errorf("GET block?seconds=1 = %d and %d bytes, want a gzipped profile", status, len(body))
	}
}
