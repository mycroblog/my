// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
)

// TestTranslationByLibreTranslate checks the wiring of the provider libretranslate (ADR-0148 §15):
// the URL of the configuration registers it, the settings enable it, the server calls the URL with
// the key, and no client sees the key.
func TestTranslationByLibreTranslate(t *testing.T) {
	t.Parallel()
	const key = "lt-key-of-the-operator"
	var mu sync.Mutex
	var keys []string
	lt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/languages" {
			_, _ = w.Write([]byte(`[{"code":"en","targets":["ja"]},{"code":"ja","targets":["en"]}]`))
			return
		}
		var req struct {
			Q      []string `json:"q"`
			APIKey string   `json:"api_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		keys = append(keys, req.APIKey)
		mu.Unlock()
		out := make([]string, len(req.Q))
		for i, q := range req.Q {
			out[i] = "[ja] " + q
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"translatedText": out})
	}))
	t.Cleanup(lt.Close)

	path := newDatabase(t, true)
	e := &configuredTranslation{}
	e.c = newStatusesClientWith(t, map[string]string{
		"MY_DATABASE_URL": path, "MY_TRANSLATOR_LIBRETRANSLATE_URL": lt.URL, "MY_TRANSLATOR_LIBRETRANSLATE_API_KEY": key,
	}, platform.AppOptions{}, "alice", "bob", "carol")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	e.db = db

	public := e.c.post("alice", map[string]any{"text": "public words @carol", "visibility": "public", "language": "en"})
	e.set(instance.SettingTranslationProviders, "libretranslate")
	got := e.instance()
	if len(got.Providers) != 1 || got.Providers[0].Id != "libretranslate" || got.Providers[0].Name != "LibreTranslate" ||
		got.Providers[0].Url == nil || *got.Providers[0].Url != "https://libretranslate.com" {
		t.Fatalf("instance translation = %+v", got)
	}
	res, out := e.translate("bob", public.Id)
	if res.StatusCode != http.StatusOK || out.Provider == nil || out.Provider.Id != "libretranslate" {
		t.Fatalf("translate = %d, %+v", res.StatusCode, out)
	}
	if !strings.Contains(out.Content, "[ja] public words") || !strings.Contains(out.Content, "carol") {
		t.Errorf("content = %q", out.Content)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(keys) != 1 || keys[0] != key {
		t.Errorf("keys sent = %q, want the configured key once", keys)
	}
	for name, v := range map[string]any{"instance": got, "translation": out, "overview": e.overview()} {
		body, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), key) || strings.Contains(string(body), lt.URL) {
			t.Errorf("the %s holds the key or the URL: %s", name, body)
		}
	}
}
