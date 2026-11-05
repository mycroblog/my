// SPDX-License-Identifier: AGPL-3.0-only

package client_test

import (
	"crypto/tls"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
)

// serveRenewable serves the fake plugin over TLS with the configuration that current holds at each handshake.
func serveRenewable(t *testing.T, current *atomic.Pointer[tls.Config]) string {
	t.Helper()
	h, _ := fakeplugin.Connect(fakeplugin.Options{})
	return fakeplugin.ServeTLS(t, h, &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			c := current.Load().Clone()
			c.NextProtos = []string{"h2", "http/1.1"}
			return c, nil
		},
	})
}

// overwrite writes the file src over dst, as a certificate manager renews a file in place, with a later time, so that a
// renewal within the file system's resolution of times still counts as a change.
func overwrite(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src) //nolint:gosec // G304: a file the test wrote
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o600); err != nil { //nolint:gosec // G703: a file the test wrote
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(dst, later, later); err != nil {
		t.Fatal(err)
	}
}

// renew writes the files of from over the files of to.
func renew(t *testing.T, from, to fakeplugin.TLSFiles) {
	t.Helper()
	overwrite(t, from.CAFile, to.CAFile)
	overwrite(t, from.CertFile, to.CertFile)
	overwrite(t, from.KeyFile, to.KeyFile)
}

// renewable starts a plugin that demands a client certificate and a Conn to it with the files of old, and returns the
// Conn and what moves the plugin to another CA.
func renewable(t *testing.T, old fakeplugin.TLSFiles, interval time.Duration) (*client.Conn, *atomic.Pointer[tls.Config]) {
	t.Helper()
	current := new(atomic.Pointer[tls.Config])
	current.Store(old.Server)
	conn := newConn(t, client.Options{Kind: probeKind(t), Destination: kind.Destination{
		Address: serveRenewable(t, current), TLSCAFile: old.CAFile, TLSCertFile: old.CertFile, TLSKeyFile: old.KeyFile,
	}})
	conn.SetReloadInterval(interval)
	return conn, current
}

// A client certificate and a CA renewed in place reach the next connection with no restart.
func TestRenewedTLSFilesNeedNoRestart(t *testing.T) {
	old := fakeplugin.NewTLSFiles(t, true)
	renewed := fakeplugin.NewTLSFiles(t, true)
	conn, current := renewable(t, old, 0)
	wantEcho(t, conn)

	// The plugin moves to a new CA: its own certificate and the one it demands of the client both.
	current.Store(renewed.Server)
	conn.Close()
	if _, err := call(t, conn); err == nil {
		t.Fatal("a call passed with the files of the old CA")
	}

	renew(t, renewed, old)
	conn.Close()
	wantEcho(t, conn)
}

// Until the interval passes, the files are not looked at again.
func TestTLSFilesAreLookedAtOnceAnInterval(t *testing.T) {
	old := fakeplugin.NewTLSFiles(t, true)
	renewed := fakeplugin.NewTLSFiles(t, true)
	conn, current := renewable(t, old, time.Hour)
	wantEcho(t, conn)
	current.Store(renewed.Server)
	renew(t, renewed, old)
	conn.Close()
	if _, err := call(t, conn); err == nil {
		t.Fatal("the files were read again before the interval passed")
	}
}

// A renewal caught half way, the certificate written and its key not yet, keeps the pair that works.
func TestAHalfRenewedPairKeepsTheOldOne(t *testing.T) {
	old := fakeplugin.NewTLSFiles(t, true)
	renewed := fakeplugin.NewTLSFiles(t, true)
	conn, _ := renewable(t, old, 0)
	overwrite(t, renewed.CertFile, old.CertFile)
	conn.Close()
	wantEcho(t, conn)
}

// With a CA file, the server's name is still checked against its certificate.
func TestTheServerNameIsCheckedAgainstTheCAFile(t *testing.T) {
	files := fakeplugin.NewTLSFiles(t, false)
	h, rec := fakeplugin.Connect(fakeplugin.Options{})
	addr := strings.Replace(fakeplugin.ServeTLS(t, h, files.Server), "127.0.0.1", "localhost", 1)
	conn := newConn(t, client.Options{Kind: probeKind(t), Destination: kind.Destination{Address: addr, TLSCAFile: files.CAFile}})
	if _, err := call(t, conn); err == nil {
		t.Error("a certificate for 127.0.0.1 passed for localhost")
	}
	if rec.Calls() != 0 {
		t.Errorf("calls = %d", rec.Calls())
	}
}
