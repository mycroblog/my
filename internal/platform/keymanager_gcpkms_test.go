// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_gcpkms

package platform_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/mycroblog/my/internal/adapter/keymanager/gcpkms"
	"github.com/mycroblog/my/internal/adapter/keymanager/gcpkms/gcpkmstest"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform"
)

// requireAllUnder fails unless `my kek status` counts every sealed value of app under the key
// ID of the KMS kind.
func requireAllUnder(t *testing.T, app *platform.App, kind string) {
	t.Helper()
	status, err := app.Rewrap.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range status {
		if c.Outdated() != 0 {
			t.Errorf("status of %s = %+v, want every value under the current key", c.Column, c)
		}
	}
	if got := kernel.KeyIDKind(app.KeyManager.CurrentKeyID()); got != kind {
		t.Errorf("current key ID %q, want one of kind %q", app.KeyManager.CurrentKeyID(), kind)
	}
}

// TestMoveBetweenLocalAndGCPKMS checks the moves of the sealed values from the local KEK to
// Cloud KMS and back with `my kek rewrap`, with the gcpkms adapter on a stand-in for Cloud KMS:
// during each move the key manager of the other kind only unwraps.
func TestMoveBetweenLocalAndGCPKMS(t *testing.T) {
	t.Parallel()
	const key = "projects/my-project/locations/global/keyRings/my/cryptoKeys/kek"
	srv, opts := gcpkmstest.Start(t, key)
	kms, err := gcpkms.New(t.Context(), gcpkms.Options{Key: key, ClientOptions: opts})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = kms.Close() })
	db := newDatabase(t, true)
	oldKEK, newKEK := randomKEK(t), randomKEK(t)

	signed := proxyURL(t, openApp(t, db, map[string]string{"MY_KEK": oldKEK}, nil, nil))

	// To Cloud KMS: kms_key wraps, and the local KEK in kek_previous only unwraps.
	toKMS := openApp(t, db, map[string]string{"MY_KEY_MANAGER": "gcpkms", "MY_KMS_KEY": key, "MY_KEK_PREVIOUS": oldKEK}, kms, nil)
	if got := proxyURL(t, toKMS); got != signed {
		t.Errorf("URL while moving to Cloud KMS = %q, want %q", got, signed)
	}
	if r := rewrapSecrets(t, toKMS); r.Rewrapped != 1 || r.Failed != 0 {
		t.Errorf("rewrap to Cloud KMS = %+v, want 1 rewrapped", r)
	}
	requireAllUnder(t, toKMS, gcpkms.Kind)
	calls := srv.Calls()
	kmsOnly := openApp(t, db, map[string]string{"MY_KEY_MANAGER": "gcpkms", "MY_KMS_KEY": key}, kms, nil)
	if got := proxyURL(t, kmsOnly); got != signed {
		t.Errorf("URL on Cloud KMS alone = %q, want %q", got, signed)
	}
	if srv.Calls() == calls {
		t.Error("the App on Cloud KMS alone did not call Cloud KMS")
	}

	// Back to the local KEK: kek wraps, and kms_key only unwraps.
	toLocal := openApp(t, db, map[string]string{"MY_KEK": newKEK, "MY_KMS_KEY": key}, kms, nil)
	if got := proxyURL(t, toLocal); got != signed {
		t.Errorf("URL while moving back to the local KEK = %q, want %q", got, signed)
	}
	if r := rewrapSecrets(t, toLocal); r.Rewrapped != 1 || r.Failed != 0 {
		t.Errorf("rewrap to the local KEK = %+v, want 1 rewrapped", r)
	}
	requireAllUnder(t, toLocal, "local")
	if got := proxyURL(t, openApp(t, db, map[string]string{"MY_KEK": newKEK}, nil, nil)); got != signed {
		t.Errorf("URL on the local KEK alone = %q, want %q", got, signed)
	}
}

// TestGCPKMSKeyManagerFromConfig checks that key_manager = gcpkms builds the Cloud KMS key
// manager of kms_key with the local KEK behind it, and that with key_manager = local a Cloud KMS
// kms_key only unwraps.
func TestGCPKMSKeyManagerFromConfig(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", writeServiceAccount(t))
	const key = "projects/my-project/locations/global/keyRings/my/cryptoKeys/kek"
	kek := randomKEK(t)
	app := openApp(t, newDatabase(t, false), map[string]string{"MY_KEY_MANAGER": "gcpkms", "MY_KMS_KEY": key, "MY_KEK_PREVIOUS": kek}, nil, nil)
	if got, want := app.KeyManager.CurrentKeyID(), gcpkms.KeyID(key); got != want {
		t.Errorf("CurrentKeyID() = %q, want %q", got, want)
	}
	app = openApp(t, newDatabase(t, false), map[string]string{"MY_KEK": kek, "MY_KMS_KEY_PREVIOUS": key}, nil, nil)
	if got := kernel.KeyIDKind(app.KeyManager.CurrentKeyID()); got != "local" {
		t.Errorf("CurrentKeyID() = %q, want a local key ID", app.KeyManager.CurrentKeyID())
	}
}

// writeServiceAccount writes the key file of a service account that exists nowhere, so that the
// Cloud KMS client finds Application Default Credentials without reaching Google Cloud.
func writeServiceAccount(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(map[string]string{ //nolint:gosec // G101: a throwaway key generated above, of an account that exists nowhere
		"type": "service_account", "project_id": "my-project", "private_key_id": "test",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email": "my@my-project.iam.gserviceaccount.com", "client_id": "1",
		"token_uri": "https://oauth2.googleapis.com/token",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "service-account.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
