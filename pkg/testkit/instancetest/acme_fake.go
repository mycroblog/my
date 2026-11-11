// SPDX-License-Identifier: AGPL-3.0-only

package instancetest

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
)

// FakeACMEDirectory is the directory URL of a FakeACMEClient.
const FakeACMEDirectory = "https://acme.fake.test/directory"

// FakeACMERejectedHost is a name a FakeACMEClient refuses to issue for.
const FakeACMERejectedHost = "rejected.fake.test"

// FakeACMEProfile is the profile a FakeACMEClient offers besides its default: certificates of 6
// days instead of 90.
const FakeACMEProfile = "short"

// FakeACMEClient is an instance.ACMEClient that is its own CA, in memory. An order presents the
// answer of its first challenge type with the order's solver, has Validate check it (as a CA
// would connect to the server), and signs a certificate with the fake's root. It passes
// ACMEClientContract.
type FakeACMEClient struct {
	// Validate checks the answer of a challenge after the solver presented it, as the CA connects
	// to the server; nil reaches nothing, as when nothing answers. An error fails the order with
	// the problem incorrectResponse.
	Validate func(ctx context.Context, ch appinstance.ACMEChallenge) error
	// OnOrder, when set, runs in each order after its account is checked and before its
	// challenge: a test blocks there to hold an order open.
	OnOrder func(ctx context.Context, host string)
	// Directory is the directory URL of the fake, FakeACMEDirectory when empty.
	Directory string
	// Clock dates the certificates; nil is the wall clock.
	Clock kernel.Clock
	// ChallengeType, when the order offers it, is the type of challenge answered; otherwise the
	// order's first type is.
	ChallengeType string
	// RenewalWindow, when set, answers RenewalInfo for the certificates of the fake instead of
	// the default window: a test moves the window, or returns an error wrapping
	// instance.ErrACMERenewalInfoUnsupported for a CA without ARI.
	RenewalWindow func(leaf *x509.Certificate) (appinstance.ACMERenewalInfo, error)

	root     *x509.Certificate
	rootKey  *ecdsa.PrivateKey
	rootPEM  []byte
	mu       sync.Mutex
	fail     error
	accounts map[string]string
	revoked  map[string]bool
	orders   []string
	replaces []string
	serial   int64
}

var _ appinstance.ACMEClient = (*FakeACMEClient)(nil)

// NewFakeACMEClient returns a fake CA with a new root.
func NewFakeACMEClient(t testing.TB) *FakeACMEClient {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Fake ACME root"},
		NotBefore: time.Now().Add(-24 * 365 * time.Hour), NotAfter: time.Now().Add(24 * 365 * time.Hour),
		KeyUsage: x509.KeyUsageCertSign, IsCA: true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &FakeACMEClient{
		root: root, rootKey: key, rootPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		accounts: map[string]string{}, revoked: map[string]bool{}, serial: 1,
	}
}

// Roots returns the root that the fake's certificates chain to.
func (f *FakeACMEClient) Roots() *x509.CertPool {
	p := x509.NewCertPool()
	p.AddCert(f.root)
	return p
}

// FailOrders makes every order fail with err from now on; nil lets them succeed again.
func (f *FakeACMEClient) FailOrders(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = err
}

// Orders returns the hosts of the orders placed, in order, failed ones included.
func (f *FakeACMEClient) Orders() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.orders)
}

// Replaces returns the serial numbers of the certificates that the orders of Orders replace, ""
// for an order that replaces none.
func (f *FakeACMEClient) Replaces() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.replaces)
}

// DirectoryURL implements instance.ACMEClient.
func (f *FakeACMEClient) DirectoryURL() string {
	if f.Directory != "" {
		return f.Directory
	}
	return FakeACMEDirectory
}

func keyID(key crypto.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(der), nil
}

// RegisterAccount implements instance.ACMEClient.
func (f *FakeACMEClient) RegisterAccount(_ context.Context, key crypto.Signer, _ []string) (string, error) {
	id, err := keyID(key.Public())
	if err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if url, ok := f.accounts[id]; ok {
		return url, nil
	}
	url := "https://acme.fake.test/account/" + strconv.Itoa(len(f.accounts)+1)
	f.accounts[id] = url
	return url, nil
}

func (f *FakeACMEClient) checkAccount(a appinstance.ACMEAccount) error {
	id, err := keyID(a.Key.Public())
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if url, ok := f.accounts[id]; !ok || url != a.URL {
		return &appinstance.ACMEProblem{Type: "urn:ietf:params:acme:error:accountDoesNotExist", Detail: "no such account"}
	}
	return nil
}

func (f *FakeACMEClient) now() time.Time {
	if f.Clock != nil {
		return f.Clock.Now()
	}
	return time.Now()
}

// Order implements instance.ACMEClient.
func (f *FakeACMEClient) Order(ctx context.Context, req appinstance.ACMEOrderRequest) (string, error) {
	if req.Solver == nil || len(req.Challenges) == 0 || req.Key == nil {
		return "", errors.New("fake acme: an order needs a solver, a challenge type, and a key")
	}
	f.mu.Lock()
	f.orders = append(f.orders, req.Host)
	replaces := ""
	if req.Replaces != nil {
		replaces = req.Replaces.SerialNumber.String()
	}
	f.replaces = append(f.replaces, replaces)
	fail := f.fail
	f.mu.Unlock()
	if err := f.checkAccount(req.Account); err != nil {
		return "", err
	}
	if req.Host == FakeACMERejectedHost {
		return "", &appinstance.ACMEProblem{Type: "urn:ietf:params:acme:error:rejectedIdentifier", Detail: "the name is refused"}
	}
	if f.OnOrder != nil {
		f.OnOrder(ctx, req.Host)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if fail != nil {
		return "", fail
	}
	token := make([]byte, 16)
	_, _ = rand.Read(token)
	typ := req.Challenges[0]
	if slices.Contains(req.Challenges, f.ChallengeType) {
		typ = f.ChallengeType
	}
	ch := appinstance.ACMEChallenge{Type: typ, Host: req.Host, Token: hex.EncodeToString(token)}
	ch.KeyAuthorization = ch.Token + ".fake-thumbprint"
	if err := req.Solver.Present(ctx, ch); err != nil {
		return "", fmt.Errorf("fake acme: present: %w", err)
	}
	defer func() { _ = req.Solver.CleanUp(context.WithoutCancel(ctx), ch) }()
	validate := f.Validate
	if validate == nil {
		validate = func(context.Context, appinstance.ACMEChallenge) error { return errors.New("nothing answers") }
	}
	if err := validate(ctx, ch); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", &appinstance.ACMEProblem{Type: "urn:ietf:params:acme:error:incorrectResponse", Detail: err.Error()}
	}
	return f.issue(req)
}

func (f *FakeACMEClient) issue(req appinstance.ACMEOrderRequest) (string, error) {
	f.mu.Lock()
	f.serial++
	serial := f.serial
	f.mu.Unlock()
	life := 90 * 24 * time.Hour
	if req.Profile == FakeACMEProfile {
		life = 6 * 24 * time.Hour
	}
	notBefore := f.now().Add(-time.Minute).Truncate(time.Second)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: req.Host}, DNSNames: []string{req.Host},
		NotBefore: notBefore, NotAfter: notBefore.Add(life),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, f.root, req.Key.Public(), f.rootKey)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})) + string(f.rootPEM), nil
}

// Revoke implements instance.ACMEClient.
func (f *FakeACMEClient) Revoke(_ context.Context, a appinstance.ACMEAccount, leaf *x509.Certificate, _ appinstance.ACMERevocationReason) error {
	if err := f.checkAccount(a); err != nil {
		return err
	}
	if err := leaf.CheckSignatureFrom(f.root); err != nil {
		return &appinstance.ACMEProblem{Type: "urn:ietf:params:acme:error:malformed", Detail: "not issued here"}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	serial := leaf.SerialNumber.String()
	if f.revoked[serial] {
		return &appinstance.ACMEProblem{Type: "urn:ietf:params:acme:error:alreadyRevoked", Detail: "already revoked"}
	}
	f.revoked[serial] = true
	return nil
}

// RenewalInfo implements instance.ACMEClient: the window is RenewalWindow's, by default the last
// third of the lifetime but its last tenth, with a Retry-After of 6 hours.
func (f *FakeACMEClient) RenewalInfo(_ context.Context, leaf *x509.Certificate) (appinstance.ACMERenewalInfo, error) {
	if err := leaf.CheckSignatureFrom(f.root); err != nil {
		return appinstance.ACMERenewalInfo{}, &appinstance.ACMEProblem{Type: "urn:ietf:params:acme:error:malformed", Detail: "not issued here"}
	}
	if f.RenewalWindow != nil {
		return f.RenewalWindow(leaf)
	}
	life := leaf.NotAfter.Sub(leaf.NotBefore)
	return appinstance.ACMERenewalInfo{
		WindowStart: leaf.NotBefore.Add(life * 2 / 3), WindowEnd: leaf.NotAfter.Add(-life / 10),
		RetryAfter: f.now().Add(6 * time.Hour),
	}, nil
}

// ChallengeValidator returns a Validate of FakeACMEClient that reads the answers from challenges
// the way the listeners of the server answer the CA: the HTTP-01 body of the token, and the
// TLS-ALPN-01 certificate of the host.
func ChallengeValidator(challenges *appinstance.ACMEChallenges) func(ctx context.Context, ch appinstance.ACMEChallenge) error {
	return func(ctx context.Context, ch appinstance.ACMEChallenge) error {
		switch ch.Type {
		case appinstance.ACMEChallengeHTTP01:
			got, err := challenges.HTTP01(ctx, ch.Token)
			if err != nil {
				return err
			}
			if got != ch.KeyAuthorization {
				return fmt.Errorf("the http-01 answer is %q, want %q", got, ch.KeyAuthorization)
			}
			return nil
		case appinstance.ACMEChallengeTLSALPN01:
			c, err := challenges.TLSALPN01Certificate(ctx, ch.Host)
			if err != nil {
				return err
			}
			return CheckTLSALPN01Certificate(c.Leaf, ch)
		}
		return fmt.Errorf("the %q challenge is not validated", ch.Type)
	}
}

// FakeACMEHarness is the ACMEHarness of a new FakeACMEClient.
func FakeACMEHarness(t *testing.T) ACMEHarness {
	f := NewFakeACMEClient(t)
	return ACMEHarness{
		Client: f,
		Answer: func(_ *testing.T, challenges *appinstance.ACMEChallenges) {
			f.Validate = ChallengeValidator(challenges)
		},
		Roots: f.Roots(), RejectedHost: FakeACMERejectedHost, Profile: FakeACMEProfile,
		ProfileValidity: 6 * 24 * time.Hour,
	}
}
