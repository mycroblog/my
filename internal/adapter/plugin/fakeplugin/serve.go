// SPDX-License-Identifier: AGPL-3.0-only

package fakeplugin

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// protocols are what a Connect server's plain listener takes: HTTP/1.1 and h2c.
func protocols() *http.Protocols {
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetHTTP2(true)
	p.SetUnencryptedHTTP2(true)
	return p
}

// ServeUnix serves h on a Unix socket until the test ends and returns its address.
func ServeUnix(t testing.TB, h http.Handler) string {
	t.Helper()
	// t.TempDir can exceed the length a socket's path may have.
	dir, err := os.MkdirTemp("", "myplugin")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "p.sock")
	l, err := new(net.ListenConfig).Listen(t.Context(), "unix", path)
	if err != nil {
		t.Fatal(err)
	}
	serve(t, l, h)
	return "unix://" + path
}

// ServeTCP serves h over plain TCP on loopback until the test ends and returns its address.
func ServeTCP(t testing.TB, h http.Handler) string {
	t.Helper()
	l, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serve(t, l, h)
	return "http://" + l.Addr().String()
}

func serve(t testing.TB, l net.Listener, h http.Handler) {
	t.Helper()
	s := &http.Server{Handler: h, Protocols: protocols(), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { _ = s.Close() })
}

// TLSFiles are the files of a test certificate authority, a client certificate, and a server's.
type TLSFiles struct {
	CAFile, CertFile, KeyFile string
	Server                    *tls.Config
}

// NewTLSFiles writes a new test CA and client certificate to files, and returns the server's TLS configuration, whose
// certificate holds 127.0.0.1. With requireClient, the server demands a client certificate from the CA.
func NewTLSFiles(t testing.TB, requireClient bool) TLSFiles {
	t.Helper()
	dir := t.TempDir()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fakeplugin CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	issue := func(serial int64, usage x509.ExtKeyUsage) ([]byte, *ecdsa.PrivateKey) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "fakeplugin"},
			NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
			IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
		}
		der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		return der, key
	}
	write := func(name, blockType string, der []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	files := TLSFiles{CAFile: write("ca.pem", "CERTIFICATE", caDER)}
	clientDER, clientKey := issue(2, x509.ExtKeyUsageClientAuth)
	clientKeyDER, err := x509.MarshalECPrivateKey(clientKey)
	if err != nil {
		t.Fatal(err)
	}
	files.CertFile = write("client.pem", "CERTIFICATE", clientDER)
	files.KeyFile = write("client-key.pem", "EC PRIVATE KEY", clientKeyDER)

	serverDER, serverKey := issue(3, x509.ExtKeyUsageServerAuth)
	files.Server = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{{Certificate: [][]byte{serverDER}, PrivateKey: serverKey}},
	}
	if requireClient {
		pool := x509.NewCertPool()
		pool.AddCert(caCert)
		files.Server.ClientAuth = tls.RequireAndVerifyClientCert
		files.Server.ClientCAs = pool
	}
	return files
}

// ServeTLS serves h over TLS with config until the test ends and returns its address.
func ServeTLS(t testing.TB, h http.Handler, config *tls.Config) string {
	t.Helper()
	s := httptest.NewUnstartedServer(h)
	s.EnableHTTP2 = true
	s.TLS = config
	s.StartTLS()
	t.Cleanup(s.Close)
	return s.URL
}
