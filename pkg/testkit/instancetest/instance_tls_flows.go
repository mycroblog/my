// SPDX-License-Identifier: AGPL-3.0-only

package instancetest

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/instance"
)

// instanceTLSCases are the cases of InstanceFlows about the TLS certificates that the server
// serves itself (ADR-0163 §4, §7): one account per CA, keys sealed and bound to their rows, and
// the wait before the next order derived from the attempts that have no certificate.
var instanceTLSCases = []struct {
	name string
	run  func(t *testing.T, e *instanceEnv)
}{
	{"RegistersOneTLSAccountAtOnce", instanceRegistersOneTLSAccount},
	{"SealsTheTLSKeysToTheirRows", instanceSealsTLSKeys},
	{"DerivesTheTLSOrderBackoff", instanceDerivesTLSOrderBackoff},
}

func instanceRegistersOneTLSAccount(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	if _, err := e.service.TLSAccount(ctx, tlsDirectory); !errors.Is(err, appinstance.ErrNotFound) {
		t.Fatalf("TLSAccount before a registration = %v, want ErrNotFound", err)
	}
	got := make([]appinstance.TLSKeyPair[instance.TLSAccount], 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range got {
		wg.Go(func() {
			got[i], errs[i] = e.service.RegisterTLSAccount(ctx, tlsDirectory, "https://ca.example/acct/"+string(rune('a'+i)), []byte{byte(i)})
		})
	}
	wg.Wait()
	if errors.Join(errs...) != nil {
		t.Fatalf("RegisterTLSAccount at once: %v", errs)
	}
	if got[0].Record.ID != got[1].Record.ID || !bytes.Equal(got[0].PrivateKey, got[1].PrivateKey) {
		t.Errorf("RegisterTLSAccount at once = %+v and %+v, want the same account", got[0], got[1])
	}
	stored, err := e.service.TLSAccount(ctx, tlsDirectory)
	if err != nil || stored.Record.ID != got[0].Record.ID || !bytes.Equal(stored.PrivateKey, got[0].PrivateKey) {
		t.Errorf("TLSAccount = %+v, %v; want %+v", stored, err, got[0])
	}
}

// tlsChain returns a self-signed PEM certificate for host valid for validity, with its key in
// PKCS #8 DER.
func tlsChain(t *testing.T, host string, serial int64, notBefore time.Time, validity time.Duration) (string, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: notBefore, NotAfter: notBefore.Add(validity),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), pkcs8
}

func instanceSealsTLSKeys(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	const host = "social.example"
	attempt, err := e.service.RecordTLSOrderAttempt(ctx, appinstance.TLSOrder{Host: host, DirectoryURL: tlsDirectory})
	if err != nil {
		t.Fatal(err)
	}
	notBefore := e.clock.Now().Truncate(time.Second)
	chain, key := tlsChain(t, host, 0xabc, notBefore, 90*24*time.Hour)
	if _, err := e.service.RecordTLSCertificate(ctx, attempt, "not a chain", key); err == nil {
		t.Error("RecordTLSCertificate of a chain that is not PEM = nil, want an error")
	}
	recorded, err := e.service.RecordTLSCertificate(ctx, attempt, chain, key)
	if err != nil {
		t.Fatalf("RecordTLSCertificate: %v", err)
	}
	if recorded.Serial != "abc" || !recorded.NotBefore.Equal(notBefore) || !recorded.NotAfter.Equal(notBefore.Add(90*24*time.Hour)) ||
		recorded.AttemptID != attempt.ID || recorded.Host != host {
		t.Errorf("RecordTLSCertificate = %+v, want the serial and validity of the leaf", recorded)
	}
	current, err := e.service.CurrentTLSCertificate(ctx, host)
	if err != nil || current.Record.ID != recorded.ID || !bytes.Equal(current.PrivateKey, key) || current.Record.ChainPEM != chain {
		t.Fatalf("CurrentTLSCertificate = %+v, %v; want the certificate recorded with its key", current, err)
	}
	if bytes.Contains(current.Record.SealedPrivateKey, key) {
		t.Error("the stored key is not sealed")
	}
	if _, err := e.sealer.Open(ctx, current.Record.SealedPrivateKey, appinstance.TLSCertificateAAD("another-id")); !errors.Is(err, kernel.ErrDecrypt) {
		t.Errorf("Open of the certificate key with another row's AAD = %v, want ErrDecrypt", err)
	}
	account, err := e.service.RegisterTLSAccount(ctx, tlsDirectory, "https://ca.example/acct/1", []byte("account key"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(account.Record.SealedPrivateKey, []byte("account key")) {
		t.Error("the stored account key is not sealed")
	}
	if _, err := e.sealer.Open(ctx, account.Record.SealedPrivateKey, appinstance.TLSAccountAAD("another-id")); !errors.Is(err, kernel.ErrDecrypt) {
		t.Errorf("Open of the account key with another row's AAD = %v, want ErrDecrypt", err)
	}

	if err := e.service.RecordTLSCertificateRevocation(ctx, recorded.ID, "lost", ""); err == nil {
		t.Error("RecordTLSCertificateRevocation with an unknown reason = nil, want an error")
	}
	if err := e.service.RecordTLSCertificateRevocation(ctx, recorded.ID, instance.TLSRevocationOperator, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.CurrentTLSCertificate(ctx, host); !errors.Is(err, appinstance.ErrNotFound) {
		t.Errorf("CurrentTLSCertificate after the revocation = %v, want ErrNotFound", err)
	}
}

func instanceDerivesTLSOrderBackoff(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	const host = "social.example"
	backoff := func() instance.TLSOrderBackoff {
		t.Helper()
		b, err := e.service.TLSOrderBackoff(ctx, host)
		if err != nil {
			t.Fatalf("TLSOrderBackoff: %v", err)
		}
		return b
	}
	attempt := func() instance.TLSOrderAttempt {
		t.Helper()
		e.clock.Advance(time.Minute)
		a, err := e.service.RecordTLSOrderAttempt(ctx, appinstance.TLSOrder{Host: host, DirectoryURL: tlsDirectory})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	if b := backoff(); b.Failures != 0 || !b.NotBefore.IsZero() || !b.Allows(e.clock.Now()) {
		t.Errorf("TLSOrderBackoff without an attempt = %+v, want none", b)
	}
	waits := []time.Duration{time.Minute, 10 * time.Minute, 100 * time.Minute, 24 * time.Hour, 24 * time.Hour}
	for n, wait := range waits {
		a := attempt()
		if n%2 == 0 {
			// The others never finished: their process died before writing a result.
			long := strings.Repeat("é", instance.MaxTLSOrderFailureDetail+10)
			if err := e.service.RecordTLSOrderFailure(ctx, a.ID, "urn:ietf:params:acme:error:connection", long); err != nil {
				t.Fatal(err)
			}
		}
		b := backoff()
		if b.Failures != n+1 || !b.NotBefore.Equal(a.OccurredAt.Add(wait)) {
			t.Errorf("TLSOrderBackoff after %d attempts = %+v, want %d failures until %s", n+1, b, n+1, a.OccurredAt.Add(wait))
		}
		if b.Allows(a.OccurredAt.Add(wait-time.Second)) || !b.Allows(a.OccurredAt.Add(wait)) {
			t.Errorf("TLSOrderBackoff after %d attempts allows an order before %s", n+1, a.OccurredAt.Add(wait))
		}
	}

	// A certificate ends the wait; only the attempts after it count again.
	issued := attempt()
	chain, key := tlsChain(t, host, 1, e.clock.Now(), 90*24*time.Hour)
	if _, err := e.service.RecordTLSCertificate(ctx, issued, chain, key); err != nil {
		t.Fatal(err)
	}
	if b := backoff(); b.Failures != 0 || !b.NotBefore.IsZero() {
		t.Errorf("TLSOrderBackoff after a certificate = %+v, want none", b)
	}
	renewal := attempt()
	if b := backoff(); b.Failures != 1 || !b.NotBefore.Equal(renewal.OccurredAt.Add(time.Minute)) {
		t.Errorf("TLSOrderBackoff after a renewal without a result = %+v, want 1 failure", b)
	}
}
