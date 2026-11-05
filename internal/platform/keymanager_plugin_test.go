// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	keymanagerplugin "github.com/mycroblog/my/internal/adapter/keymanager/plugin"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

const pluginKeyName = "hsm/transit-1"

// TestMoveBetweenLocalAndAPlugin checks the moves of the sealed values from the local KEK to a plugin and back with
// `my kek rewrap`, with key_manager = plugin built from the configuration: during each move, the key manager of the other
// kind only unwraps.
func TestMoveBetweenLocalAndAPlugin(t *testing.T) {
	t.Parallel()
	h, rec := fakeplugin.JSON(fakeplugin.Options{KeyManager: testkit.NewKeyManager(pluginKeyName)})
	address := fakeplugin.ServeUnix(t, h)
	db := newDatabase(t, true)
	oldKEK, newKEK := randomKEK(t), randomKEK(t)

	signed := proxyURL(t, openApp(t, db, map[string]string{"MY_KEK": oldKEK}, nil, nil))
	if rec.Calls() != 0 {
		t.Errorf("a server with no plugin made %d calls", rec.Calls())
	}

	// To the plugin: the plugin wraps, and the local KEK in kek_previous only unwraps.
	toPlugin := openApp(t, db, map[string]string{"MY_KEY_MANAGER": "plugin", "MY_PLUGIN_KEY_MANAGER_ADDRESS": address, "MY_KEK": "", "MY_KEK_PREVIOUS": oldKEK}, nil, nil)
	if got, want := toPlugin.KeyManager.CurrentKeyID(), "plugin:"+pluginKeyName; got != want {
		t.Errorf("CurrentKeyID() = %q, want %q", got, want)
	}
	if got := proxyURL(t, toPlugin); got != signed {
		t.Errorf("URL while moving to the plugin = %q, want %q", got, signed)
	}
	if r := rewrapSecrets(t, toPlugin); r.Rewrapped != 1 || r.Failed != 0 {
		t.Errorf("rewrap to the plugin = %+v, want 1 rewrapped", r)
	}
	requireAllUnder(t, toPlugin, keymanagerplugin.Kind)

	calls := rec.Calls()
	pluginOnly := openApp(t, db, map[string]string{"MY_KEY_MANAGER": "plugin", "MY_PLUGIN_KEY_MANAGER_ADDRESS": address, "MY_KEK": ""}, nil, nil)
	if got := proxyURL(t, pluginOnly); got != signed {
		t.Errorf("URL on the plugin alone = %q, want %q", got, signed)
	}
	if rec.Calls() == calls {
		t.Error("the App on the plugin alone did not call it")
	}

	// Back to the local KEK: kek wraps, and the plugin only unwraps.
	toLocal := openApp(t, db, map[string]string{"MY_KEK": newKEK, "MY_PLUGIN_KEY_MANAGER_ADDRESS": address}, nil, nil)
	if got := kernel.KeyIDKind(toLocal.KeyManager.CurrentKeyID()); got != "local" {
		t.Errorf("CurrentKeyID() = %q, want a local key ID", toLocal.KeyManager.CurrentKeyID())
	}
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

// passThrough is a plugin that wraps nothing, the case the self-test exists for.
type passThrough struct{ kernel.KeyManager }

func (passThrough) WrapKey(_ context.Context, dek, _ []byte) (string, []byte, error) {
	return pluginKeyName, dek, nil
}

func (passThrough) UnwrapKey(_ context.Context, _ string, wrapped, _ []byte) ([]byte, error) {
	return wrapped, nil
}

// A plugin that fails its self-test stops the start, in both of its roles, and the failure says so.
func TestAPluginThatFailsItsSelfTestStopsTheStart(t *testing.T) {
	t.Parallel()
	h, _ := fakeplugin.JSON(fakeplugin.Options{KeyManager: passThrough{testkit.NewKeyManager(pluginKeyName)}})
	address := fakeplugin.ServeUnix(t, h)
	for name, env := range map[string]map[string]string{
		"it wraps":      {"MY_KEY_MANAGER": "plugin", "MY_PLUGIN_KEY_MANAGER_ADDRESS": address},
		"it only opens": {"MY_PLUGIN_KEY_MANAGER_ADDRESS": address},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env["MY_DATABASE_URL"] = newDatabase(t, true)
			_, err := platform.NewApp(t.Context(), loadConfig(t, env), platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
			if err == nil || !strings.Contains(err.Error(), "key_manager = plugin") || !strings.Contains(err.Error(), "plaintext") {
				t.Errorf("err = %v", err)
			}
		})
	}
}

// A plugin that does not answer stops the start too. Nothing falls back on the local KEK that is still configured.
func TestAPluginThatDoesNotAnswerStopsTheStart(t *testing.T) {
	t.Parallel()
	// The context ends the wait of 30 seconds, which the adapter's tests cover.
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_KEY_MANAGER": "plugin", "MY_PLUGIN_KEY_MANAGER_ADDRESS": "unix:///nowhere/key_manager.sock", "MY_KEK_PREVIOUS": randomKEK(t)})
	if _, err := platform.NewApp(ctx, cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)}); err == nil || !strings.Contains(err.Error(), "key_manager = plugin") {
		t.Errorf("err = %v", err)
	}
}

// failable is a key store that fails on demand, after the start.
type failable struct {
	kernel.KeyManager

	down chan struct{}
}

var errKeyStoreDown = errors.New("the key store is down")

func (f failable) isDown() bool {
	select {
	case <-f.down:
		return true
	default:
		return false
	}
}

func (f failable) WrapKey(ctx context.Context, dek, aad []byte) (string, []byte, error) {
	if f.isDown() {
		return "", nil, errKeyStoreDown
	}
	return f.KeyManager.WrapKey(ctx, dek, aad)
}

func (f failable) UnwrapKey(ctx context.Context, id string, wrapped, aad []byte) ([]byte, error) {
	if f.isDown() {
		return nil, errKeyStoreDown
	}
	return f.KeyManager.UnwrapKey(ctx, id, wrapped, aad)
}

// While a running server's plugin is down, nothing is sealed: not in plaintext, and not with the local KEK that is still
// configured. The KeyManager of the App sits behind the cache, whose period is off here to reach the plugin.
func TestAPluginThatGoesDownFailsClosed(t *testing.T) {
	t.Parallel()
	store := failable{KeyManager: testkit.NewKeyManager(pluginKeyName), down: make(chan struct{})}
	h, _ := fakeplugin.JSON(fakeplugin.Options{KeyManager: store})
	app := openApp(t, newDatabase(t, true), map[string]string{
		"MY_KEY_MANAGER": "plugin", "MY_PLUGIN_KEY_MANAGER_ADDRESS": fakeplugin.ServeUnix(t, h),
		"MY_KEK_PREVIOUS": randomKEK(t), "MY_KMS_DEK_CACHE_TTL": "0",
	}, nil, nil)
	aad := kernel.AAD("row", "column")
	sealed, err := app.Sealer.Seal(t.Context(), []byte("secret"), aad)
	if err != nil {
		t.Fatal(err)
	}

	close(store.down)
	if got, err := app.Sealer.Seal(t.Context(), []byte("secret"), aad); err == nil || got != nil {
		t.Errorf("Seal = %x, %v", got, err)
	} else if bytes.Contains(got, []byte("secret")) {
		t.Errorf("Seal stored the plaintext")
	}
	if got, err := app.Sealer.Open(t.Context(), sealed, aad); err == nil || got != nil {
		t.Errorf("Open = %q, %v", got, err)
	}
	if got := kernel.KeyIDKind(app.KeyManager.CurrentKeyID()); got != keymanagerplugin.Kind {
		t.Errorf("the current key moved to %q", app.KeyManager.CurrentKeyID())
	}
}
