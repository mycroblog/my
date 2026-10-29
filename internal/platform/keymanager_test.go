// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"testing"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/rewrap"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit"
)

const secretsColumn = "instance_secrets.sealed_secret"

func randomKEK(t *testing.T) string {
	t.Helper()
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	return base64.StdEncoding.EncodeToString(k)
}

// openApp builds an App on dbPath with the KEKs of env, lets edit change the loaded
// configuration, and closes the App when the test ends.
func openApp(t *testing.T, dbPath string, env map[string]string, kms kernel.KeyManager, edit func(*config.Config)) *platform.App {
	t.Helper()
	env["MY_DATABASE_URL"] = dbPath
	cfg := loadConfig(t, env)
	if edit != nil {
		edit(cfg)
	}
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), KeyManager: kms})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

func proxyURL(t *testing.T, app *platform.App) string {
	t.Helper()
	if err := app.MediaProxy.LoadKeys(t.Context()); err != nil {
		t.Fatalf("LoadKeys: %v", err)
	}
	return app.MediaProxy.URL("0190f5a4-8c4e-7000-8000-000000000001", content.ProxyFull)
}

// rewrapSecrets runs `my kek rewrap` on app and returns the report of the instance secrets.
func rewrapSecrets(t *testing.T, app *platform.App) rewrap.ColumnReport {
	t.Helper()
	reports, err := app.Rewrap.Rewrap(t.Context(), nil)
	if err != nil {
		t.Fatalf("Rewrap: %v", err)
	}
	for _, r := range reports {
		if r.Column == secretsColumn {
			return r
		}
	}
	t.Fatalf("Rewrap reported no %s: %+v", secretsColumn, reports)
	return rewrap.ColumnReport{}
}

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

// TestMediaProxyKeyOutlivesKEKs checks that the media proxy signs with the same key after a
// restart, a rotation of the KEK with `my kek rewrap`, and a move from the local KEK to a KMS,
// so that the URLs signed before keep working.
func TestMediaProxyKeyOutlivesKEKs(t *testing.T) {
	t.Parallel()
	db := newDatabase(t, true)
	oldKEK, newKEK := randomKEK(t), randomKEK(t)

	signed := proxyURL(t, openApp(t, db, map[string]string{"MY_KEK": oldKEK}, nil, nil))
	if got := proxyURL(t, openApp(t, db, map[string]string{"MY_KEK": oldKEK}, nil, nil)); got != signed {
		t.Errorf("URL after a restart = %q, want %q", got, signed)
	}

	// A rotation of the KEK: the new one is current, the old one only unwraps until the rewrap.
	rotated := openApp(t, db, map[string]string{"MY_KEK": newKEK, "MY_KEK_PREVIOUS": oldKEK}, nil, nil)
	if got := proxyURL(t, rotated); got != signed {
		t.Errorf("URL during the rotation = %q, want %q", got, signed)
	}
	if r := rewrapSecrets(t, rotated); r.Rewrapped != 1 || r.Failed != 0 {
		t.Errorf("rewrap of the rotation = %+v, want 1 rewrapped", r)
	}
	if got := proxyURL(t, openApp(t, db, map[string]string{"MY_KEK": newKEK}, nil, nil)); got != signed {
		t.Errorf("URL after the rotation = %q, want %q", got, signed)
	}

	// A move to a KMS: the test KeyManager stands in for it, and the local KEK only unwraps.
	kms := testkit.NewKeyManager("gcpkms:projects/p/locations/global/keyRings/r/cryptoKeys/k")
	toKMS := func(c *config.Config) {
		c.KeyManager, c.KMSKey = "gcpkms", "projects/p/locations/global/keyRings/r/cryptoKeys/k"
	}
	moving := openApp(t, db, map[string]string{"MY_KEK": newKEK}, kms, toKMS)
	if got := proxyURL(t, moving); got != signed {
		t.Errorf("URL while moving to the KMS = %q, want %q", got, signed)
	}
	if r := rewrapSecrets(t, moving); r.Rewrapped != 1 || r.Failed != 0 {
		t.Errorf("rewrap to the KMS = %+v, want 1 rewrapped", r)
	}
	status, err := moving.Rewrap.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range status {
		if c.Outdated() != 0 {
			t.Errorf("status of %s after the rewrap = %+v, want every value under %s", c.Column, c, kms.CurrentKeyID())
		}
	}
	// Without the local KEK, the KMS alone opens the key.
	kmsOnly := openApp(t, db, map[string]string{}, kms, func(c *config.Config) { toKMS(c); c.KEK = nil })
	if got := proxyURL(t, kmsOnly); got != signed {
		t.Errorf("URL after the move to the KMS = %q, want %q", got, signed)
	}
}

// TestMediaProxyKeyWithoutTheSchema checks that an App builds before the schema is applied and
// loads the media proxy's key only once the schema is there.
func TestMediaProxyKeyWithoutTheSchema(t *testing.T) {
	t.Parallel()
	app := openApp(t, newDatabase(t, false), map[string]string{}, nil, nil)
	if err := app.MediaProxy.LoadKeys(t.Context()); err == nil {
		t.Error("LoadKeys succeeded without the schema")
	}
}
