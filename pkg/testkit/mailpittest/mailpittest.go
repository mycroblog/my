// SPDX-License-Identifier: AGPL-3.0-only

// Package mailpittest gives tests a Mailpit server (https://mailpit.axllent.org/, MIT), the
// development SMTP server that keeps what it receives and shows it over a REST API, for the
// integration tests of mail = smtp (#1026): the tests send through the real adapter and read
// back, over the API, what arrived.
//
// Each New starts a container of its own, with the options of the test: STARTTLS with a
// certificate of a CA made for the test, and SMTP authentication with one user. Mailpit starts
// in about a second, so tests do not share servers and see only their own messages.
//
// The tests need Docker and run only with TEST_MAIL=container (scripts/task.sh go:test-mail;
// CI in the slow suite): without it New skips the test, and with it a Docker that cannot start
// the container fails the test.
package mailpittest

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// The environment variables of the tests; see the package documentation.
const (
	EnvMail  = "TEST_MAIL"
	EnvImage = "TEST_MAILPIT_IMAGE"
)

// DefaultImage is the container image of the tests, pinned by tag and digest. The development
// compose file (deploy/docker/mailpit/compose.yaml) pins the same image.
const DefaultImage = "axllent/mailpit:v1.31.4@sha256:b68349e3a014b90c5610bfb26b2ae36f3892d7b8cf25ee140c6c71c98d2fcf48"

// Options configure the server of one test.
type Options struct {
	// STARTTLS offers STARTTLS and requires it before MAIL FROM; without it the server speaks
	// plain SMTP only.
	STARTTLS bool
	// Username and Password, when Username is set, require SMTP authentication as that user.
	Username string
	Password string
}

// Server is the Mailpit server of one test.
type Server struct {
	// Host and Port are the SMTP endpoint.
	Host string
	Port int
	// TLSConfig trusts the server's certificate, which is valid for Host, for the client side
	// of STARTTLS.
	TLSConfig *tls.Config
	// API is the base URL of the REST API, such as http://127.0.0.1:32768.
	API string
}

// New starts a server with o for t and stops it when t ends. It skips t unless TEST_MAIL is
// "container".
func New(t testing.TB, o Options) *Server {
	t.Helper()
	if os.Getenv(EnvMail) != "container" {
		t.Skipf("the Mailpit tests run with %s=container (scripts/task.sh go:test-mail)", EnvMail)
	}
	image := os.Getenv(EnvImage)
	if image == "" {
		image = DefaultImage
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := []string{"--smtp-disable-rdns"}
	var files []testcontainers.ContainerFile
	var roots *x509.CertPool
	if o.STARTTLS {
		var certPEM, keyPEM []byte
		roots, certPEM, keyPEM = certificate(t)
		files = append(files,
			testcontainers.ContainerFile{Reader: bytes.NewReader(certPEM), ContainerFilePath: "/certs/cert.pem", FileMode: 0o644},
			testcontainers.ContainerFile{Reader: bytes.NewReader(keyPEM), ContainerFilePath: "/certs/key.pem", FileMode: 0o644},
		)
		cmd = append(cmd, "--smtp-tls-cert", "/certs/cert.pem", "--smtp-tls-key", "/certs/key.pem", "--smtp-require-starttls")
	}
	if o.Username != "" {
		files = append(files, testcontainers.ContainerFile{
			Reader: bytes.NewReader([]byte(o.Username + ":" + o.Password + "\n")), ContainerFilePath: "/auth/smtp", FileMode: 0o644,
		})
		cmd = append(cmd, "--smtp-auth-file", "/auth/smtp")
	}
	c, err := testcontainers.Run(ctx, image,
		testcontainers.WithExposedPorts("1025/tcp", "8025/tcp"),
		testcontainers.WithCmdArgs(cmd...),
		testcontainers.WithFiles(files...),
		testcontainers.WithWaitStrategy(wait.ForAll(
			wait.ForListeningPort("1025/tcp"),
			wait.ForHTTP("/readyz").WithPort("8025/tcp"),
		).WithDeadline(time.Minute)),
	)
	if c != nil {
		t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	}
	if err != nil {
		t.Fatalf("mailpittest: start container %s: %v", image, err)
	}
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("mailpittest: container host: %v", err)
	}
	// The certificate names 127.0.0.1 and localhost; Docker publishes the ports on the loopback.
	if host == "localhost" {
		host = "127.0.0.1"
	}
	smtpPort, err := c.MappedPort(ctx, "1025/tcp")
	if err != nil {
		t.Fatalf("mailpittest: SMTP port: %v", err)
	}
	apiPort, err := c.MappedPort(ctx, "8025/tcp")
	if err != nil {
		t.Fatalf("mailpittest: API port: %v", err)
	}
	s := &Server{
		Host: host, Port: int(smtpPort.Num()),
		API: "http://" + net.JoinHostPort(host, strconv.Itoa(int(apiPort.Num()))),
	}
	if roots != nil {
		s.TLSConfig = &tls.Config{RootCAs: roots, ServerName: host, MinVersion: tls.VersionTLS12}
	}
	return s
}

// certificate returns a new CA as a pool and a certificate that it signed for 127.0.0.1 and
// localhost with its key, in PEM.
func certificate(t testing.TB) (*x509.CertPool, []byte, []byte) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "mailpittest CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return pool,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

// Address is an address as the API shows it.
type Address struct {
	Name    string
	Address string
}

// Summary is a message in a list of the API.
type Summary struct {
	ID        string
	MessageID string
	From      Address
	To        []Address
	Subject   string
}

// Message is a message as Mailpit parsed it.
type Message struct {
	ID        string
	MessageID string
	From      Address
	To        []Address
	ReplyTo   []Address
	Subject   string
	Date      time.Time
	Text      string
	HTML      string
}

// Messages returns the messages that the server received, the newest first.
func (s *Server) Messages(t testing.TB) []Summary {
	t.Helper()
	var list struct{ Messages []Summary }
	s.getJSON(t, "/api/v1/messages", &list)
	return list.Messages
}

// Message returns the message of id as Mailpit parsed it.
func (s *Server) Message(t testing.TB, id string) Message {
	t.Helper()
	var m Message
	s.getJSON(t, "/api/v1/message/"+url.PathEscape(id), &m)
	return m
}

// Raw returns the source of the message of id, as the server received it.
func (s *Server) Raw(t testing.TB, id string) []byte {
	t.Helper()
	return s.get(t, "/api/v1/message/"+url.PathEscape(id)+"/raw")
}

// WaitFor waits up to timeout for the n-th message (from 1) to the address to, and returns it.
func (s *Server) WaitFor(t testing.TB, to string, n int, timeout time.Duration) Message {
	t.Helper()
	for deadline := time.Now().Add(timeout); ; {
		var found []Summary
		for _, m := range s.Messages(t) {
			for _, a := range m.To {
				if a.Address == to {
					found = append(found, m)
				}
			}
		}
		if len(found) >= n {
			// The list is the newest first.
			return s.Message(t, found[len(found)-n].ID)
		}
		if time.Now().After(deadline) {
			t.Fatalf("mailpittest: %d messages to %s after %v, want %d", len(found), to, timeout, n)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (s *Server) getJSON(t testing.TB, path string, v any) {
	t.Helper()
	if err := json.Unmarshal(s.get(t, path), v); err != nil {
		t.Fatalf("mailpittest: GET %s: %v", path, err)
	}
}

func (s *Server) get(t testing.TB, path string) []byte {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, s.API+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatalf("mailpittest: GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("mailpittest: GET %s = %d %s (%v)", path, resp.StatusCode, b, err)
	}
	return b
}
