// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"testing"
	"time"

	kvvalkey "github.com/mycroblog/my/internal/adapter/kv/valkey"
	"github.com/mycroblog/my/internal/adapter/system"
	"github.com/mycroblog/my/internal/adapter/valkey"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/valkeytest"
)

// With kv = valkey the server starts, is ready while the Valkey server answers, and keeps the
// key-value store's entries there under valkey_key_prefix.
func TestServerRunsOnValkeyKV(t *testing.T) {
	t.Parallel()
	v := valkeytest.New(t)
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_KV": "valkey",
		"MY_VALKEY_URL": v.URL, "MY_VALKEY_KEY_PREFIX": v.KeyPrefix,
	})
	s := start(t, cfg, platform.ServeOptions{}, nil)
	// A readiness check is bounded (3 seconds, ADR-0088 §15) and answers 503 past it, as a probe of a
	// loaded process may: the server is ready once a check passes, as a probe that tries again sees.
	var code int
	var body string
	ready := testkit.Eventually(t, 15*time.Second, func() bool {
		resp, b := s.do(t, http.MethodGet, platform.PathReadiness, nil)
		code, body = resp.StatusCode, b
		return code == http.StatusOK
	})
	if !ready {
		t.Fatalf("readiness = %d %s, want 200; log:\n%s", code, body, s.logs.String())
	}
	ctx := t.Context()
	if err := s.app.KV.Set(ctx, "test", "k", []byte("v"), time.Hour); err != nil {
		t.Fatal(err)
	}
	c, err := valkey.Open(ctx, valkey.Options{URL: v.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	direct := kvvalkey.New(c, v.KeyPrefix, system.Clock{})
	if got, err := direct.Get(ctx, "test", "k"); err != nil || string(got) != "v" {
		t.Fatalf("Get on the Valkey server = %q, %v; want the entry the App set", got, err)
	}
}
