// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestTLSKeysOutliveKEKs checks that `my kek status` counts the sealed keys of the ACME accounts
// and the TLS certificates, and that `my kek rewrap` re-wraps them so that the new KEK alone opens
// them.
func TestTLSKeysOutliveKEKs(t *testing.T) {
	t.Parallel()
	db := newDatabase(t, true)
	oldKEK, newKEK := randomKEK(t), randomKEK(t)
	const directory = "https://ca.example/directory"

	first := openApp(t, db, map[string]string{"MY_KEK": oldKEK}, nil, nil)
	accountKey := []byte("account key")
	if _, err := first.Instance.RegisterTLSAccount(t.Context(), directory, "https://ca.example/acct/1", accountKey); err != nil {
		t.Fatalf("RegisterTLSAccount: %v", err)
	}
	cert := testkit.NewTLSCertificate(t, 42, time.Now().Add(-time.Hour), 90*24*time.Hour, "example.com")
	chain, certKey := string(cert.CertPEM), cert.KeyPKCS8
	attempt, err := first.Instance.RecordTLSOrderAttempt(t.Context(), appinstance.TLSOrder{Host: "example.com", DirectoryURL: directory})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Instance.RecordTLSCertificate(t.Context(), attempt, chain, certKey); err != nil {
		t.Fatalf("RecordTLSCertificate: %v", err)
	}

	rotated := openApp(t, db, map[string]string{"MY_KEK": newKEK, "MY_KEK_PREVIOUS": oldKEK}, nil, nil)
	columns := []string{"tls_acme_accounts.sealed_private_key", "tls_certificates.sealed_private_key"}
	status, err := rotated.Rewrap.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range status {
		for _, want := range columns {
			if c.Column == want && (c.Total() != 1 || c.Outdated() != 1) {
				t.Errorf("status of %s before the rewrap = %+v, want 1 value under the old KEK", c.Column, c)
			}
		}
	}
	reports, err := rotated.Rewrap.Rewrap(t.Context(), nil)
	if err != nil {
		t.Fatalf("Rewrap: %v", err)
	}
	rewrapped := map[string]int{}
	for _, r := range reports {
		rewrapped[r.Column] = r.Rewrapped
	}
	for _, c := range columns {
		if rewrapped[c] != 1 {
			t.Errorf("rewrap of %s = %d values, want 1 (%+v)", c, rewrapped[c], reports)
		}
	}
	requireAllUnder(t, rotated, "local")

	after := openApp(t, db, map[string]string{"MY_KEK": newKEK}, nil, nil)
	checkTLSKeys(t, after, directory, accountKey, certKey)
}

func checkTLSKeys(t *testing.T, app *platform.App, directory string, accountKey, certKey []byte) {
	t.Helper()
	account, err := app.Instance.TLSAccount(t.Context(), directory)
	if err != nil || !bytes.Equal(account.PrivateKey, accountKey) {
		t.Errorf("TLSAccount after the rewrap = %x, %v; want the key registered", account.PrivateKey, err)
	}
	cert, err := app.Instance.CurrentTLSCertificate(t.Context(), "example.com")
	if err != nil || !bytes.Equal(cert.PrivateKey, certKey) {
		t.Errorf("CurrentTLSCertificate after the rewrap = %x, %v; want the key recorded", cert.PrivateKey, err)
	}
}
