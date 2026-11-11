// SPDX-License-Identifier: AGPL-3.0-only

package instancetest

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

// ACMEHarness is a CA for ACMEClientContract.
type ACMEHarness struct {
	Client appinstance.ACMEClient
	// Answer makes the CA find the answers that challenges holds when it validates, until t
	// ends: it serves them where the CA connects, or hands them to a fake.
	Answer func(t *testing.T, challenges *appinstance.ACMEChallenges)
	// Roots are the roots of the chains the CA issues.
	Roots *x509.CertPool
	// RejectedHost is a name the CA refuses to issue for.
	RejectedHost string
	// Profile is a profile the CA offers besides its default, whose certificates live at most
	// ProfileValidity; empty skips its case.
	Profile         string
	ProfileValidity time.Duration
}

// ACMEHarnessFactory returns a harness on a new CA. It registers the cleanup of everything it
// creates with t.Cleanup.
type ACMEHarnessFactory func(t *testing.T) ACMEHarness

// ACMEClientContract verifies an instance.ACMEClient (ADR-0163 §2): an account registered once
// per key, orders answered through TLS-ALPN-01 or HTTP-01 alone that return a chain for the host
// and the key of the order and leave no answer behind, an order with a profile, the renewal
// information of a certificate and an order that replaces it, a revocation, and the CA's problems
// as *instance.ACMEProblem.
func ACMEClientContract(t *testing.T, newHarness ACMEHarnessFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, h ACMEHarness)
	}{
		{"RegistersAnAccountOncePerKey", acmeRegistersAnAccount},
		{"OrdersThroughTLSALPN01", func(t *testing.T, h ACMEHarness) { acmeOrdersThrough(t, h, appinstance.ACMEChallengeTLSALPN01) }},
		{"OrdersThroughHTTP01", func(t *testing.T, h ACMEHarness) { acmeOrdersThrough(t, h, appinstance.ACMEChallengeHTTP01) }},
		{"OrdersWithAProfile", acmeOrdersWithAProfile},
		{"RenewsWithReplaces", acmeRenewsWithReplaces},
		{"RevokesACertificate", acmeRevokes},
		{"ReportsTheProblemOfARefusedOrder", acmeReportsARefusedOrder},
		{"FailsAnUnansweredChallenge", acmeFailsUnanswered},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.run(t, newHarness(t))
		})
	}
}

var acmeHostSeq atomic.Int64

// acmeHost returns a new name to order a certificate for.
func acmeHost() string {
	return "contract-" + strconv.FormatInt(acmeHostSeq.Add(1), 10) + ".acme.test"
}

func newECKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// acmeAccount registers a new account.
func acmeAccount(t *testing.T, h ACMEHarness) appinstance.ACMEAccount {
	t.Helper()
	key := newECKey(t)
	url, err := h.Client.RegisterAccount(t.Context(), key, []string{"mailto:ops@acme.test"})
	if err != nil {
		t.Fatalf("RegisterAccount: %v", err)
	}
	return appinstance.ACMEAccount{URL: url, Key: key}
}

// acmeOrder orders a certificate of host through challenge with challenges answered, and returns
// its leaf.
func acmeOrder(t *testing.T, h ACMEHarness, account appinstance.ACMEAccount, host, challenge string, mod func(*appinstance.ACMEOrderRequest)) *x509.Certificate {
	t.Helper()
	challenges := appinstance.NewACMEChallenges(testkit.NewKeyValueStore(testkit.NewClock(time.Now())))
	h.Answer(t, challenges)
	key := newECKey(t)
	req := appinstance.ACMEOrderRequest{
		Account: account, Host: host, Key: key, Challenges: []string{challenge}, Solver: challenges,
	}
	if mod != nil {
		mod(&req)
	}
	chain, err := h.Client.Order(t.Context(), req)
	if err != nil {
		t.Fatalf("Order %s through %s: %v", host, challenge, err)
	}
	leaf := checkChain(t, h, chain, host)
	if pub, ok := leaf.PublicKey.(*ecdsa.PublicKey); !ok || !pub.Equal(key.Public()) {
		t.Errorf("the certificate of %s is not for the key of the order", host)
	}
	if _, err := challenges.TLSALPN01Certificate(t.Context(), host); !errors.Is(err, kernel.ErrKeyNotFound) {
		t.Errorf("the tls-alpn-01 answer of %s after the order: %v, want none", host, err)
	}
	return leaf
}

// checkChain parses chain, checks that its leaf covers host and chains to the harness's roots, and
// returns the leaf.
func checkChain(t *testing.T, h ACMEHarness, chain, host string) *x509.Certificate {
	t.Helper()
	var certs []*x509.Certificate
	rest := []byte(chain)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatalf("the chain of %s: %v", host, err)
		}
		certs = append(certs, c)
	}
	if len(certs) == 0 {
		t.Fatalf("the chain of %s has no certificate: %q", host, chain)
	}
	inter := x509.NewCertPool()
	for _, c := range certs[1:] {
		inter.AddCert(c)
	}
	if _, err := certs[0].Verify(x509.VerifyOptions{DNSName: host, Roots: h.Roots, Intermediates: inter}); err != nil {
		t.Errorf("the certificate of %s does not verify: %v", host, err)
	}
	return certs[0]
}

func acmeRegistersAnAccount(t *testing.T, h ACMEHarness) {
	key := newECKey(t)
	first, err := h.Client.RegisterAccount(t.Context(), key, nil)
	if err != nil || first == "" {
		t.Fatalf("RegisterAccount = %q, %v", first, err)
	}
	again, err := h.Client.RegisterAccount(t.Context(), key, nil)
	if err != nil || again != first {
		t.Errorf("RegisterAccount of the same key = %q, %v; want %q", again, err, first)
	}
	other, err := h.Client.RegisterAccount(t.Context(), newECKey(t), []string{"mailto:ops@acme.test"})
	if err != nil || other == first {
		t.Errorf("RegisterAccount of another key = %q, %v; want another account", other, err)
	}
}

func acmeOrdersThrough(t *testing.T, h ACMEHarness, challenge string) {
	account := acmeAccount(t, h)
	acmeOrder(t, h, account, acmeHost(), challenge, nil)
}

func acmeOrdersWithAProfile(t *testing.T, h ACMEHarness) {
	if h.Profile == "" {
		t.Skip("the CA offers no profile")
	}
	account := acmeAccount(t, h)
	leaf := acmeOrder(t, h, account, acmeHost(), appinstance.ACMEChallengeTLSALPN01, func(r *appinstance.ACMEOrderRequest) { r.Profile = h.Profile })
	if life := leaf.NotAfter.Sub(leaf.NotBefore); life > h.ProfileValidity {
		t.Errorf("the certificate of the profile %s lives %v, want at most %v", h.Profile, life, h.ProfileValidity)
	}
}

func acmeRenewsWithReplaces(t *testing.T, h ACMEHarness) {
	account := acmeAccount(t, h)
	host := acmeHost()
	leaf := acmeOrder(t, h, account, host, appinstance.ACMEChallengeTLSALPN01, nil)
	info, err := h.Client.RenewalInfo(t.Context(), leaf)
	if err != nil {
		t.Fatalf("RenewalInfo: %v", err)
	}
	if info.WindowStart.IsZero() || !info.WindowStart.Before(info.WindowEnd) || info.WindowEnd.After(leaf.NotAfter) {
		t.Errorf("RenewalInfo = %+v, want a window that ends by %v", info, leaf.NotAfter)
	}
	renewed := acmeOrder(t, h, account, host, appinstance.ACMEChallengeHTTP01, func(r *appinstance.ACMEOrderRequest) { r.Replaces = leaf })
	if renewed.SerialNumber.Cmp(leaf.SerialNumber) == 0 {
		t.Error("the renewal returned the certificate it replaces")
	}
}

func acmeRevokes(t *testing.T, h ACMEHarness) {
	account := acmeAccount(t, h)
	leaf := acmeOrder(t, h, account, acmeHost(), appinstance.ACMEChallengeTLSALPN01, nil)
	if err := h.Client.Revoke(t.Context(), account, leaf, appinstance.ACMERevocationSuperseded); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	err := h.Client.Revoke(t.Context(), account, leaf, appinstance.ACMERevocationSuperseded)
	var p *appinstance.ACMEProblem
	if !errors.As(err, &p) || p.Type != "urn:ietf:params:acme:error:alreadyRevoked" {
		t.Errorf("Revoke again = %v, want the problem alreadyRevoked", err)
	}
}

func acmeReportsARefusedOrder(t *testing.T, h ACMEHarness) {
	account := acmeAccount(t, h)
	challenges := appinstance.NewACMEChallenges(testkit.NewKeyValueStore(testkit.NewClock(time.Now())))
	_, err := h.Client.Order(t.Context(), appinstance.ACMEOrderRequest{
		Account: account, Host: h.RejectedHost, Key: newECKey(t),
		Challenges: []string{appinstance.ACMEChallengeTLSALPN01}, Solver: challenges,
	})
	var p *appinstance.ACMEProblem
	if !errors.As(err, &p) || p.Type != "urn:ietf:params:acme:error:rejectedIdentifier" {
		t.Errorf("Order of %s = %v, want the problem rejectedIdentifier", h.RejectedHost, err)
	}
}

func acmeFailsUnanswered(t *testing.T, h ACMEHarness) {
	account := acmeAccount(t, h)
	// Nothing answers: the CA's validation fails.
	challenges := appinstance.NewACMEChallenges(testkit.NewKeyValueStore(testkit.NewClock(time.Now())))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	_, err := h.Client.Order(ctx, appinstance.ACMEOrderRequest{
		Account: account, Host: acmeHost(), Key: newECKey(t),
		Challenges: []string{appinstance.ACMEChallengeHTTP01}, Solver: challenges,
	})
	var p *appinstance.ACMEProblem
	if !errors.As(err, &p) {
		t.Errorf("Order without an answer = %v, want a problem of the CA", err)
	}
}

// CheckTLSALPN01Certificate reports whether cert answers the TLS-ALPN-01 challenge ch (RFC 8737
// §3): it names only ch.Host, and its critical acmeIdentifier extension holds the SHA-256 digest
// of ch.KeyAuthorization.
func CheckTLSALPN01Certificate(cert *x509.Certificate, ch appinstance.ACMEChallenge) error {
	if len(cert.DNSNames) != 1 || cert.DNSNames[0] != ch.Host {
		return fmt.Errorf("the tls-alpn-01 certificate names %v, want %s", cert.DNSNames, ch.Host)
	}
	want := sha256.Sum256([]byte(ch.KeyAuthorization))
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 31}) {
			continue
		}
		var got []byte
		if _, err := asn1.Unmarshal(ext.Value, &got); err != nil {
			return err
		}
		if !ext.Critical || !bytes.Equal(got, want[:]) {
			return errors.New("the acmeIdentifier extension does not hold the key authorization")
		}
		return nil
	}
	return errors.New("the tls-alpn-01 certificate has no acmeIdentifier extension")
}

// DialTLSALPN01 validates the TLS-ALPN-01 challenge ch as a CA does: it connects to addr offering
// only acme-tls/1 with ch.Host as the server name, and checks the certificate presented.
func DialTLSALPN01(ctx context.Context, addr string, ch appinstance.ACMEChallenge) error {
	d := tls.Dialer{Config: &tls.Config{
		ServerName: ch.Host, NextProtos: []string{"acme-tls/1"}, MinVersion: tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // G402: the certificate is self-signed and checked below
	}}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	state := conn.(*tls.Conn).ConnectionState()
	if state.NegotiatedProtocol != "acme-tls/1" {
		return fmt.Errorf("negotiated %q, want acme-tls/1", state.NegotiatedProtocol)
	}
	return CheckTLSALPN01Certificate(state.PeerCertificates[0], ch)
}

// GetHTTP01 validates the HTTP-01 challenge ch as a CA does: it requests the resource of its
// token at addr with ch.Host as the host, and compares the body with the key authorization.
func GetHTTP01(ctx context.Context, addr string, ch appinstance.ACMEChallenge) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/.well-known/acme-challenge/"+ch.Token, nil)
	if err != nil {
		return err
	}
	req.Host = ch.Host
	client := &http.Client{
		Transport:     &http.Transport{DialContext: (&net.Dialer{}).DialContext},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK || string(body) != ch.KeyAuthorization {
		return fmt.Errorf("the http-01 resource answered %d %q, want 200 %q", resp.StatusCode, body, ch.KeyAuthorization)
	}
	return nil
}
