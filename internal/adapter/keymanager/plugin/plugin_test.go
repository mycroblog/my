// SPDX-License-Identifier: AGPL-3.0-only

package plugin_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect/v2"

	"github.com/mycroblog/my/internal/adapter/keymanager/local"
	keymanagerplugin "github.com/mycroblog/my/internal/adapter/keymanager/plugin"
	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/kerneltest"
)

const (
	pluginKey = "hsm/transit-1"
	keyID     = "plugin:" + pluginKey
	wrapPath  = "/my.plugin.key_manager.v1.KeyManagerService/WrapKey"
	openPath  = "/my.plugin.key_manager.v1.KeyManagerService/UnwrapKey"
	descPath  = "/my.plugin.key_manager.v1.KeyManagerService/Describe"
)

func newConn(t *testing.T, o client.Options) *client.Conn {
	t.Helper()
	k, ok := kind.ByName(kind.KeyManager)
	if !ok {
		t.Fatal("no key_manager kind")
	}
	o.Kind = k
	conn, err := client.New(o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	return conn
}

func connTo(t *testing.T, h http.Handler) *client.Conn {
	t.Helper()
	return newConn(t, client.Options{Destination: kind.Destination{Address: fakeplugin.ServeUnix(t, h)}})
}

func start(t *testing.T, h http.Handler) *keymanagerplugin.KeyManager {
	t.Helper()
	m, err := keymanagerplugin.Start(t.Context(), connTo(t, h), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// storeOf is the fake whose key store is km, in the form of net/http and JSON.
func storeOf(km kernel.KeyManager) http.Handler {
	h, _ := fakeplugin.JSON(fakeplugin.Options{KeyManager: km})
	return h
}

// The adapter passes the contract of the port against both fakes, over each protocol they speak.
func TestContract(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		protocol string
		fake     func(fakeplugin.Options) (http.Handler, *fakeplugin.Recorder)
	}{
		{"http_and_json", kind.ProtocolJSON, fakeplugin.JSON},
		{"connect_json", kind.ProtocolJSON, fakeplugin.Connect},
		{"connect_proto", kind.ProtocolProto, fakeplugin.Connect},
		{"connect_grpc", kind.ProtocolGRPC, fakeplugin.Connect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, _ := tc.fake(fakeplugin.Options{KeyManager: testkit.NewKeyManager(pluginKey)})
			conn := newConn(t, client.Options{Destination: kind.Destination{Address: fakeplugin.ServeUnix(t, h), Protocol: tc.protocol}})
			m, err := keymanagerplugin.Start(t.Context(), conn, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			kerneltest.KeyManagerContract(t, m)
		})
	}
}

// keyStore is a kernel.KeyManager that tests break one method at a time.
type keyStore struct {
	kernel.KeyManager

	id     func() string
	wrap   func(ctx context.Context, dek, aad []byte) (string, []byte, error)
	unwrap func(ctx context.Context, id string, wrapped, aad []byte) ([]byte, error)
}

func (s *keyStore) CurrentKeyID() string {
	if s.id != nil {
		return s.id()
	}
	return s.KeyManager.CurrentKeyID()
}

func (s *keyStore) WrapKey(ctx context.Context, dek, aad []byte) (string, []byte, error) {
	if s.wrap != nil {
		return s.wrap(ctx, dek, aad)
	}
	return s.KeyManager.WrapKey(ctx, dek, aad)
}

func (s *keyStore) UnwrapKey(ctx context.Context, id string, wrapped, aad []byte) ([]byte, error) {
	if s.unwrap != nil {
		return s.unwrap(ctx, id, wrapped, aad)
	}
	return s.KeyManager.UnwrapKey(ctx, id, wrapped, aad)
}

// A plugin that answers wrongly stops the start at once: waiting does not mend it.
func TestStartStopsOnAWrongAnswer(t *testing.T) {
	t.Parallel()
	good := testkit.NewKeyManager(pluginKey)
	for _, tc := range []struct {
		name  string
		store *keyStore
		want  string
	}{
		{
			name: "wrapped is the key",
			store: &keyStore{
				KeyManager: good,
				wrap:       func(_ context.Context, dek, _ []byte) (string, []byte, error) { return pluginKey, dek, nil },
				unwrap:     func(_ context.Context, _ string, wrapped, _ []byte) ([]byte, error) { return wrapped, nil },
			},
			want: "plaintext",
		},
		{
			name: "wrapped holds the key",
			store: &keyStore{
				KeyManager: good,
				wrap: func(_ context.Context, dek, _ []byte) (string, []byte, error) {
					return pluginKey, append([]byte("v1:"), dek...), nil
				},
				unwrap: func(_ context.Context, _ string, wrapped, _ []byte) ([]byte, error) { return wrapped[3:], nil },
			},
			want: "plaintext",
		},
		{
			name: "AAD ignored",
			store: &keyStore{
				KeyManager: good,
				wrap: func(ctx context.Context, dek, _ []byte) (string, []byte, error) {
					return good.WrapKey(ctx, dek, nil)
				},
				unwrap: func(ctx context.Context, id string, wrapped, _ []byte) ([]byte, error) {
					return good.UnwrapKey(ctx, id, wrapped, nil)
				},
			},
			want: "different AAD opened",
		},
		{
			name: "a different AAD gives UNKNOWN_KEY",
			store: &keyStore{
				KeyManager: good,
				unwrap: func(ctx context.Context, id string, wrapped, aad []byte) ([]byte, error) {
					dek, err := good.UnwrapKey(ctx, id, wrapped, aad)
					if err != nil {
						return nil, kernel.ErrUnknownKey
					}
					return dek, nil
				},
			},
			want: "OUTCOME_UNKNOWN_KEY",
		},
		{
			name:  "two wraps are the same",
			store: sameWrapTwice(good),
			want:  "fresh nonce",
		},
		{
			name: "another key comes back",
			store: &keyStore{
				KeyManager: good,
				unwrap: func(context.Context, string, []byte, []byte) ([]byte, error) {
					return bytes.Repeat([]byte{7}, 32), nil
				},
			},
			want: "another key",
		},
		{
			name: "the round trip does not open",
			store: &keyStore{
				KeyManager: good,
				unwrap: func(context.Context, string, []byte, []byte) ([]byte, error) {
					return nil, kernel.ErrDecrypt
				},
			},
			want: "round trip",
		},
		{
			name:  "no key ID",
			store: &keyStore{KeyManager: good, id: func() string { return "" }},
			want:  "key ID",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			begin := time.Now()
			m, err := keymanagerplugin.Start(t.Context(), connTo(t, storeOf(tc.store)), time.Minute)
			if err == nil {
				t.Fatalf("started on a wrong plugin, with key %q", m.CurrentKeyID())
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to say %q", err, tc.want)
			}
			if elapsed := time.Since(begin); elapsed > 20*time.Second {
				t.Errorf("a wrong answer was retried for %s", elapsed)
			}
		})
	}
}

// sameWrapTwice wraps properly once for each key and repeats that value, as a plugin with a fixed nonce does.
func sameWrapTwice(good kernel.KeyManager) *keyStore {
	values := map[string][]byte{}
	return &keyStore{
		KeyManager: good,
		wrap: func(ctx context.Context, dek, aad []byte) (string, []byte, error) {
			if w, ok := values[string(dek)]; ok {
				return pluginKey, w, nil
			}
			id, w, err := good.WrapKey(ctx, dek, aad)
			values[string(dek)] = w
			return id, w, err
		},
	}
}

// An empty answer from a plugin that forgot its outcome is never a success.
func TestStartStopsOnAnEmptyAnswer(t *testing.T) {
	t.Parallel()
	for name, fake := range map[string]func(fakeplugin.Options) (http.Handler, *fakeplugin.Recorder){"json": fakeplugin.JSON, "connect": fakeplugin.Connect} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, rec := fake(fakeplugin.Options{KeyManager: testkit.NewKeyManager(pluginKey), Broken: true})
			if _, err := keymanagerplugin.Start(t.Context(), connTo(t, h), time.Minute); err == nil || !strings.Contains(err.Error(), "outcome") {
				t.Errorf("err = %v", err)
			}
			if rec.Calls() != 1 {
				t.Errorf("calls = %d, want the one that answered", rec.Calls())
			}
		})
	}
}

// A sidecar that comes up after the server is waited for.
func TestStartWaitsForASidecar(t *testing.T) {
	t.Parallel()
	if keymanagerplugin.StartWait != 30*time.Second {
		t.Errorf("StartWait = %s", keymanagerplugin.StartWait)
	}
	dir, err := os.MkdirTemp("", "mykm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "p.sock")
	conn := newConn(t, client.Options{Destination: kind.Destination{Address: "unix://" + path}})

	srv := &http.Server{Handler: storeOf(testkit.NewKeyManager(pluginKey)), ReadHeaderTimeout: time.Minute}
	t.Cleanup(func() { _ = srv.Close() })
	go func() {
		time.Sleep(1200 * time.Millisecond)
		l, err := new(net.ListenConfig).Listen(context.Background(), "unix", path)
		if err != nil {
			return
		}
		_ = srv.Serve(l)
	}()
	m, err := keymanagerplugin.Start(t.Context(), conn, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if m.CurrentKeyID() != keyID {
		t.Errorf("CurrentKeyID = %q", m.CurrentKeyID())
	}
}

// A plugin that answers 503 at first is still coming up, too.
func TestStartWaitsForAPluginThatIsNotReady(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	store := storeOf(testkit.NewKeyManager(pluginKey))
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			http.Error(w, "starting", http.StatusServiceUnavailable)
			return
		}
		store.ServeHTTP(w, r)
	})
	if _, err := keymanagerplugin.Start(t.Context(), connTo(t, h), time.Minute); err != nil {
		t.Fatal(err)
	}
}

// A plugin that never answers ends the wait with an error, which stops the start.
func TestStartGivesUp(t *testing.T) {
	t.Parallel()
	const wait = 1500 * time.Millisecond
	check := func(t *testing.T, conn *client.Conn) {
		t.Helper()
		begin := time.Now()
		m, err := keymanagerplugin.Start(t.Context(), conn, wait)
		elapsed := time.Since(begin)
		if err == nil || m != nil {
			t.Fatalf("Start = %v, %v", m, err)
		}
		if !strings.Contains(err.Error(), "did not answer") {
			t.Errorf("err = %v", err)
		}
		if elapsed < wait {
			t.Errorf("gave up after %s, before the wait of %s", elapsed, wait)
		}
		if elapsed > wait+20*time.Second {
			t.Errorf("gave up after %s", elapsed)
		}
	}
	t.Run("nothing listens", func(t *testing.T) {
		t.Parallel()
		check(t, newConn(t, client.Options{Destination: kind.Destination{Address: "unix:///nowhere/key_manager.sock"}}))
	})
	t.Run("the plugin hangs", func(t *testing.T) {
		t.Parallel()
		h, _ := fakeplugin.JSON(fakeplugin.Options{KeyManager: testkit.NewKeyManager(pluginKey), Block: true})
		check(t, connTo(t, h))
	})
}

// A refusal of the credentials and a plugin of another kind will not change in a moment, so the start stops at once.
func TestStartDoesNotWaitForARefusal(t *testing.T) {
	t.Parallel()
	refusal, _ := fakeplugin.JSON(fakeplugin.Options{KeyManager: testkit.NewKeyManager(pluginKey), Token: "right"})
	otherKind, _ := fakeplugin.JSON(fakeplugin.Options{})
	for name, tc := range map[string]struct {
		h     http.Handler
		token string
		code  connect.Code
	}{
		"credentials": {refusal, "wrong", connect.CodeUnauthenticated},
		"other kind":  {otherKind, "", connect.CodeUnimplemented},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			conn := newConn(t, client.Options{Destination: kind.Destination{Address: fakeplugin.ServeUnix(t, tc.h), Token: tc.token}})
			begin := time.Now()
			_, err := keymanagerplugin.Start(t.Context(), conn, time.Minute)
			if connect.CodeOf(err) != tc.code {
				t.Errorf("err = %v", err)
			}
			if elapsed := time.Since(begin); elapsed > 20*time.Second {
				t.Errorf("retried for %s", elapsed)
			}
		})
	}
}

// The context of the start ends the wait.
func TestStartEndsWithItsContext(t *testing.T) {
	t.Parallel()
	conn := newConn(t, client.Options{Destination: kind.Destination{Address: "unix:///nowhere/key_manager.sock"}})
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if _, err := keymanagerplugin.Start(ctx, conn, time.Hour); err == nil {
		t.Error("no error")
	}
}

// Plain TCP is refused for this kind, on loopback too, before any call.
func TestPlainTCPIsRefused(t *testing.T) {
	t.Parallel()
	h, rec := fakeplugin.JSON(fakeplugin.Options{KeyManager: testkit.NewKeyManager(pluginKey)})
	k, _ := kind.ByName(kind.KeyManager)
	for _, address := range []string{fakeplugin.ServeTCP(t, h), "http://localhost:8080", "http://[::1]:8080", "http://vault.internal:8200"} {
		if _, err := client.New(client.Options{Kind: k, Destination: kind.Destination{Address: address}}); err == nil || !strings.Contains(err.Error(), "must not be http://") {
			t.Errorf("%s: err = %v", address, err)
		}
	}
	if rec.Calls() != 0 {
		t.Errorf("%d calls over plain TCP", rec.Calls())
	}
}

// TLS and a bearer token work, for a plugin on another machine.
func TestTLSWithAToken(t *testing.T) {
	t.Parallel()
	h, _ := fakeplugin.Connect(fakeplugin.Options{KeyManager: testkit.NewKeyManager(pluginKey), Token: "right"})
	files := fakeplugin.NewTLSFiles(t, false)
	address := fakeplugin.ServeTLS(t, h, files.Server)
	conn := newConn(t, client.Options{Destination: kind.Destination{Address: address, Token: "right", TLSCAFile: files.CAFile}})
	m, err := keymanagerplugin.Start(t.Context(), conn, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	kerneltest.KeyManagerContract(t, m)
}

func TestKeyIDs(t *testing.T) {
	t.Parallel()
	longest := strings.Repeat("k", keymanagerplugin.MaxPluginKeyIDLen)
	if keymanagerplugin.MaxPluginKeyIDLen != 248 {
		t.Errorf("MaxPluginKeyIDLen = %d", keymanagerplugin.MaxPluginKeyIDLen)
	}
	for _, name := range []string{"a", "transit/keys/my:1", longest} {
		m := start(t, storeOf(testkit.NewKeyManager(name)))
		id := m.CurrentKeyID()
		if id != "plugin:"+name || len(id) > 255 {
			t.Errorf("CurrentKeyID = %q", id)
		}
		if kind := kernel.KeyIDKind(id); kind != keymanagerplugin.Kind {
			t.Errorf("KeyIDKind(%q) = %q", id, kind)
		}
		// A sealed value holds the whole ID.
		s := kernel.NewSealer(m)
		sealed, err := s.Seal(t.Context(), []byte("x"), []byte("aad"))
		if err != nil {
			t.Fatal(err)
		}
		if got, err := kernel.SealedKeyID(sealed); err != nil || got != id {
			t.Errorf("SealedKeyID = %q, %v", got, err)
		}
	}
	for name, id := range map[string]string{
		"empty":     "",
		"too long":  longest + "k",
		"a space":   "my key",
		"new line":  "key\nforged log line",
		"not ASCII": "鍵",
	} {
		_, err := keymanagerplugin.Start(t.Context(), connTo(t, storeOf(&keyStore{KeyManager: testkit.NewKeyManager(pluginKey), id: func() string { return id }})), time.Minute)
		if err == nil {
			t.Errorf("%s: no error", name)
			continue
		}
		if id != "" && strings.Contains(err.Error(), id) {
			t.Errorf("%s: the error quotes the plugin: %v", name, err)
		}
	}
}

// An ID of another kind never reaches the plugin.
func TestUnwrapOfAnotherKindMakesNoCall(t *testing.T) {
	t.Parallel()
	h, rec := fakeplugin.JSON(fakeplugin.Options{KeyManager: testkit.NewKeyManager(pluginKey)})
	m := start(t, h)
	before := rec.Calls()
	for _, id := range []string{"local:0123456789abcdef", "awskms:" + pluginKey, pluginKey, "plugin:", ""} {
		if _, err := m.UnwrapKey(t.Context(), id, []byte("wrapped"), []byte("aad")); !errors.Is(err, kernel.ErrUnknownKey) {
			t.Errorf("%q: err = %v", id, err)
		}
	}
	if rec.Calls() != before {
		t.Errorf("%d calls", rec.Calls()-before)
	}
}

// A KEK that rotated in the plugin shows in the next wrap, and a value of the KEK before still opens.
func TestRotationInThePlugin(t *testing.T) {
	t.Parallel()
	var current atomic.Pointer[testkit.KeyRing]
	current.Store(testkit.NewKeyRing(pluginKey))
	store := &keyStore{
		KeyManager: testkit.NewKeyManager("unused"),
		id:         func() string { return current.Load().CurrentKeyID() },
		wrap: func(ctx context.Context, dek, aad []byte) (string, []byte, error) {
			return current.Load().WrapKey(ctx, dek, aad)
		},
		unwrap: func(ctx context.Context, id string, wrapped, aad []byte) ([]byte, error) {
			return current.Load().UnwrapKey(ctx, id, wrapped, aad)
		},
	}
	m := start(t, storeOf(store))
	dek, aad := bytes.Repeat([]byte{1}, 32), []byte("aad")
	id, wrapped, err := m.WrapKey(t.Context(), dek, aad)
	if err != nil || id != keyID {
		t.Fatalf("WrapKey = %q, %v", id, err)
	}

	current.Store(testkit.NewKeyRing("hsm/transit-2", pluginKey))
	if got := m.CurrentKeyID(); got != keyID {
		t.Errorf("CurrentKeyID before a wrap = %q", got)
	}
	if id, _, err := m.WrapKey(t.Context(), dek, aad); err != nil || id != "plugin:hsm/transit-2" {
		t.Fatalf("WrapKey = %q, %v", id, err)
	}
	if got := m.CurrentKeyID(); got != "plugin:hsm/transit-2" {
		t.Errorf("CurrentKeyID = %q", got)
	}
	got, err := m.UnwrapKey(t.Context(), keyID, wrapped, aad)
	if err != nil || !bytes.Equal(got, dek) {
		t.Errorf("UnwrapKey of the KEK before = %v, %v", got, err)
	}
}

// override stands in front of a plugin once it started, and answers what a test sets.
type override struct {
	next http.Handler
	fn   atomic.Pointer[http.HandlerFunc]
}

func (o *override) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if fn := o.fn.Load(); fn != nil {
		(*fn)(w, r)
		return
	}
	o.next.ServeHTTP(w, r)
}

func (o *override) set(fn http.HandlerFunc) { o.fn.Store(&fn) }

// answer answers each RPC with its JSON body, 200 when it has one, and otherwise fails.
func answer(bodies map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, ok := bodies[r.URL.Path]
		if !ok {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

// startedWithOverride returns a started KeyManager and what stands in front of its plugin.
func startedWithOverride(t *testing.T, o client.Options) (*keymanagerplugin.KeyManager, *override) {
	t.Helper()
	front := &override{next: storeOf(testkit.NewKeyManager(pluginKey))}
	o.Destination.Address = fakeplugin.ServeUnix(t, front)
	m, err := keymanagerplugin.Start(t.Context(), newConn(t, o), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return m, front
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// Every failure at run time is an error of the call: a Sealer stores nothing, and the current key stays.
func TestFailuresAtRunTimeFailClosed(t *testing.T) {
	t.Parallel()
	tooLong := b64(bytes.Repeat([]byte{1}, 0x10000))
	for _, tc := range []struct {
		name string
		wrap string
		open string
	}{
		{name: "an error of the plugin"},
		{name: "an empty answer", wrap: `{}`, open: `{}`},
		{name: "an outcome of no name", wrap: `{"outcome":"OUTCOME_FROM_THE_FUTURE","keyId":"k","wrapped":"AQ=="}`, open: `{"outcome":"OUTCOME_FROM_THE_FUTURE","dek":"AQ=="}`},
		{name: "an outcome of no value", wrap: `{"outcome":99,"keyId":"k","wrapped":"AQ=="}`, open: `{"outcome":99,"dek":"AQ=="}`},
		{name: "the outcome of no result", wrap: `{"outcome":"OUTCOME_UNSPECIFIED","keyId":"k","wrapped":"AQ=="}`, open: `{"outcome":"OUTCOME_UNSPECIFIED","dek":"AQ=="}`},
		{name: "OK and nothing else", wrap: `{"outcome":"OUTCOME_OK"}`, open: `{"outcome":"OUTCOME_OK"}`},
		{name: "not JSON", wrap: `<html>`, open: `<html>`},
		{name: "not base64", wrap: `{"outcome":"OUTCOME_OK","keyId":"k","wrapped":"!"}`, open: `{"outcome":"OUTCOME_OK","dek":"!"}`},
		{name: "an empty wrapped value", wrap: fmt.Sprintf(`{"outcome":"OUTCOME_OK","keyId":%q}`, pluginKey)},
		{name: "a wrapped value too long", wrap: fmt.Sprintf(`{"outcome":"OUTCOME_OK","keyId":%q,"wrapped":%q}`, pluginKey, tooLong)},
		{name: "no key ID", wrap: fmt.Sprintf(`{"outcome":"OUTCOME_OK","wrapped":%q}`, b64([]byte("wrapped")))},
		{name: "a key ID too long", wrap: fmt.Sprintf(`{"outcome":"OUTCOME_OK","keyId":%q,"wrapped":%q}`, strings.Repeat("k", 249), b64([]byte("wrapped")))},
		{name: "an answer over the limit", wrap: fmt.Sprintf(`{"outcome":"OUTCOME_OK","keyId":%q,"wrapped":%q}`, pluginKey, strings.Repeat("A", 200*1024))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, front := startedWithOverride(t, client.Options{})
			s := kernel.NewSealer(m)
			aad := []byte("aad")
			sealed, err := s.Seal(t.Context(), []byte("secret"), aad)
			if err != nil {
				t.Fatal(err)
			}

			bodies := map[string]string{}
			if tc.wrap != "" {
				bodies[wrapPath] = tc.wrap
			}
			if tc.open != "" {
				bodies[openPath] = tc.open
			}
			front.set(answer(bodies))
			if got, err := s.Seal(t.Context(), []byte("secret"), aad); err == nil || got != nil {
				t.Errorf("Seal = %x, %v, want only an error", got, err)
			}
			if m.CurrentKeyID() != keyID {
				t.Errorf("CurrentKeyID changed to %q", m.CurrentKeyID())
			}
			if tc.wrap != "" && tc.open == "" {
				return
			}
			got, err := s.Open(t.Context(), sealed, aad)
			if err == nil || got != nil {
				t.Errorf("Open = %q, %v, want only an error", got, err)
			}
			if errors.Is(err, kernel.ErrDecrypt) || errors.Is(err, kernel.ErrUnknownKey) {
				t.Errorf("a failure reads as a result of the port: %v", err)
			}
		})
	}
}

// A plugin that begins to pass keys through, after a self-test it passed, has nothing stored.
func TestAWrappedValueThatHoldsTheKeyIsNeverStored(t *testing.T) {
	t.Parallel()
	m, front := startedWithOverride(t, client.Options{})
	front.set(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DEK []byte `json:"dek"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"outcome": "OUTCOME_OK", "keyId": pluginKey, "wrapped": append([]byte("v1:"), req.DEK...)})
	})
	if got, err := kernel.NewSealer(m).Seal(t.Context(), []byte("secret"), []byte("aad")); err == nil || got != nil {
		t.Errorf("Seal = %x, %v", got, err)
	}
}

// A call that outlives its deadline fails, counted by its kind and code, whatever the plugin does.
func TestADeadlineEndsACall(t *testing.T) {
	t.Parallel()
	meter := testkit.NewMeter()
	m, front := startedWithOverride(t, client.Options{Destination: kind.Destination{Timeout: 2 * time.Second}, Meter: meter})
	s := kernel.NewSealer(m)
	sealed, err := s.Seal(t.Context(), []byte("secret"), []byte("aad"))
	if err != nil {
		t.Fatal(err)
	}
	front.set(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	if _, err := s.Seal(t.Context(), []byte("secret"), []byte("aad")); connect.CodeOf(err) != connect.CodeDeadlineExceeded {
		t.Errorf("Seal = %v", err)
	}
	if _, err := s.Open(t.Context(), sealed, []byte("aad")); connect.CodeOf(err) != connect.CodeDeadlineExceeded {
		t.Errorf("Open = %v", err)
	}
	if got := meter.Sum("my.plugin.failures", map[string]string{client.LabelKind: kind.KeyManager, client.LabelCode: "deadline_exceeded"}); got != 2 {
		t.Errorf("failures = %v", got)
	}
}

// A plugin that is gone fails the calls, and neither the local KEK nor any other key takes over: the router only opens
// what the local KEK itself sealed.
func TestNothingFallsBackToAnotherKey(t *testing.T) {
	t.Parallel()
	m, front := startedWithOverride(t, client.Options{})
	kek := bytes.Repeat([]byte{9}, 32)
	localKM, err := local.New(kek)
	if err != nil {
		t.Fatal(err)
	}
	router, err := kernel.NewKeyRouter(m, localKM)
	if err != nil {
		t.Fatal(err)
	}
	if router.CurrentKeyID() != keyID {
		t.Fatalf("CurrentKeyID = %q", router.CurrentKeyID())
	}
	aad := []byte("aad")
	byPlugin, err := kernel.NewSealer(router).Seal(t.Context(), []byte("secret"), aad)
	if err != nil {
		t.Fatal(err)
	}
	byLocal, err := kernel.NewSealer(localKM).Seal(t.Context(), []byte("old secret"), aad)
	if err != nil {
		t.Fatal(err)
	}

	front.set(answer(nil))
	s := kernel.NewSealer(router)
	if got, err := s.Seal(t.Context(), []byte("secret"), aad); err == nil || got != nil {
		t.Errorf("Seal while the plugin is gone = %x, %v", got, err)
	}
	if got, err := s.Open(t.Context(), byPlugin, aad); err == nil || got != nil {
		t.Errorf("Open while the plugin is gone = %q, %v", got, err)
	}
	if got, err := s.Rewrap(t.Context(), byLocal, aad); err == nil || got != nil {
		t.Errorf("Rewrap while the plugin is gone = %x, %v", got, err)
	}
	if got, err := s.Open(t.Context(), byLocal, aad); err != nil || string(got) != "old secret" {
		t.Errorf("Open of a local value = %q, %v", got, err)
	}
}

// logBuffer keeps what the default logger writes while a test runs.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLogs points the default logger at a buffer. A test that calls it does not run in parallel.
func captureLogs(t *testing.T) *logBuffer {
	t.Helper()
	b := new(logBuffer)
	before := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(b, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(before) })
	return b
}

// recorder keeps the bodies a plugin saw, in which the key material is.
type recorder struct {
	next   http.Handler
	bodies chan []byte
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.bodies <- body
	req.Body = io.NopCloser(bytes.NewReader(body))
	r.next.ServeHTTP(w, req)
}

// No DEK, AAD, or wrapped value reaches an error, a log, or a metric, whatever a plugin answers: not even when the plugin
// echoes them in every place it can.
func TestNoKeyMaterialInErrorsLogsOrMetrics(t *testing.T) {
	logs := captureLogs(t)
	meter := testkit.NewMeter()

	dek := bytes.Repeat([]byte("D3K!"), 8)
	aad := []byte("the-aad-of-a-secret-row")
	wrapped := []byte("the-wrapped-value-of-the-key")
	forms := func(b []byte) []string {
		return []string{string(b), hex.EncodeToString(b), b64(b), base64.RawURLEncoding.EncodeToString(b), fmt.Sprintf("%v", b), fmt.Sprintf("%q", b)}
	}
	var secrets []string
	for _, b := range [][]byte{dek, aad, wrapped} {
		secrets = append(secrets, forms(b)...)
	}
	echo := strings.Join([]string{string(dek), hex.EncodeToString(dek), b64(dek), string(aad), b64(aad), string(wrapped), b64(wrapped)}, " ")

	check := func(t *testing.T, what string, text string) {
		t.Helper()
		for _, s := range secrets {
			if strings.Contains(text, s) {
				t.Errorf("%s holds key material %q: %s", what, s, text)
			}
		}
	}
	echoJSON, err := json.Marshal(echo)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		fn   http.HandlerFunc
	}{
		{"a Connect error", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, `{"code":"internal","message":%s,"details":[{"type":"x","value":%q}]}`, echoJSON, b64(dek))
		}},
		{"an unavailable plugin", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintf(w, `{"code":"unavailable","message":%s}`, echoJSON)
		}},
		{"a page", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, echo, http.StatusBadGateway) }},
		{"not JSON", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(echo))
		}},
		{"a value in a bad field", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"outcome":%s,"keyId":%s,"wrapped":%s,"dek":%s}`, echoJSON, echoJSON, echoJSON, echoJSON)
		}},
		{"a key ID that holds it", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"outcome":"OUTCOME_OK","keyId":%q,"wrapped":%q}`, string(dek)+" "+string(aad), b64(wrapped))
		}},
		{"the key given back as wrapped", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"outcome":"OUTCOME_OK","keyId":%q,"wrapped":%q}`, pluginKey, b64(dek))
		}},
		{"a refusal", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"outcome":"OUTCOME_DECRYPT"}`))
		}},
		{"a hang", func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, front := startedWithOverride(t, client.Options{Destination: kind.Destination{Timeout: time.Second}, Meter: meter})
			front.set(tc.fn)

			_, gotWrapped, err := m.WrapKey(t.Context(), dek, aad)
			if tc.name != "a refusal" {
				if err == nil {
					t.Errorf("WrapKey succeeded, with %x", gotWrapped)
				} else {
					check(t, "the error of WrapKey", err.Error()+fmt.Sprintf(" %+v %#v", err, err))
				}
			}
			got, err := m.UnwrapKey(t.Context(), keyID, wrapped, aad)
			if tc.name != "a key ID that holds it" && tc.name != "the key given back as wrapped" && err == nil {
				t.Errorf("UnwrapKey succeeded, with %x", got)
			}
			if err != nil {
				check(t, "the error of UnwrapKey", err.Error()+fmt.Sprintf(" %+v %#v", err, err))
			}
		})
	}

	// What a wrong plugin answers at start, and what stops the start, says none of it either.
	t.Run("the start", func(t *testing.T) {
		front := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == descPath {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"outcome":"OUTCOME_OK","keyId":%q}`, pluginKey)
				return
			}
			var req struct {
				DEK []byte `json:"dek"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"outcome":"OUTCOME_OK","keyId":%q,"wrapped":%q}`, pluginKey, b64(req.DEK))
		})
		seen := &recorder{next: front, bodies: make(chan []byte, 16)}
		_, err := keymanagerplugin.Start(t.Context(), connTo(t, seen), time.Minute)
		if err == nil {
			t.Fatal("started")
		}
		close(seen.bodies)
		for body := range seen.bodies {
			var req struct {
				DEK []byte `json:"dek"`
			}
			if err2 := json.Unmarshal(body, &req); err2 == nil && len(req.DEK) > 0 {
				for _, form := range forms(req.DEK) {
					if strings.Contains(err.Error(), form) {
						t.Errorf("the error of the start holds the key: %v", err)
					}
				}
			}
		}
	})

	check(t, "the log", logs.String())
	for _, p := range meter.Measurements("my.plugin.failures", nil) {
		for k, v := range p.Labels {
			check(t, "the label "+k, v)
		}
	}
	if len(meter.Measurements("my.plugin.failures", nil)) == 0 {
		t.Error("no failure was counted")
	}
	if err := meter.Err(); err != nil {
		t.Error(err)
	}
}

// A DEK cache in front keeps a secret in use open through a short outage, as with a KMS, and never hides a new failure.
func TestTheCacheOfDEKsCoversAnOutage(t *testing.T) {
	t.Parallel()
	m, front := startedWithOverride(t, client.Options{})
	clock := testkit.NewClock(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC))
	cached := kernel.NewDEKCache(m, kernel.DEKCacheOptions{TTL: 10 * time.Minute, Entries: 8, Clock: clock})
	s := kernel.NewSealer(cached)
	aad := []byte("aad")
	sealed, err := s.Seal(t.Context(), []byte("secret"), aad)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(t.Context(), sealed, aad); err != nil {
		t.Fatal(err)
	}
	front.set(answer(nil))
	if got, err := s.Open(t.Context(), sealed, aad); err != nil || string(got) != "secret" {
		t.Errorf("Open within the period = %q, %v", got, err)
	}
	if _, err := s.Seal(t.Context(), []byte("new"), aad); err == nil {
		t.Error("Seal needs the plugin, and it succeeded")
	}
	clock.Advance(11 * time.Minute)
	if got, err := s.Open(t.Context(), sealed, aad); err == nil {
		t.Errorf("Open after the period = %q", got)
	}
}
