// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// keyAccounts stores one fetched remote actor and its first key. The methods ResolvePublicKey
// does not reach are those of the embedded nil interface. Like a database, every method fails
// with the context's error once it is done, and storing can take a while (delay).
type keyAccounts struct {
	RemoteAccounts
	delay time.Duration

	mu    sync.Mutex
	actor *account.Account
	key   *account.Key
}

func (a *keyAccounts) CurrentKey(ctx context.Context, keyID string) (appaccount.Key, error) {
	if err := ctx.Err(); err != nil {
		return appaccount.Key{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.key == nil || a.key.KeyID != keyID {
		return appaccount.Key{}, appaccount.ErrNotFound
	}
	return *a.key, nil
}

func (a *keyAccounts) FindByURI(context.Context, string) (appaccount.Actor, error) {
	return appaccount.Actor{}, appaccount.ErrNotFound
}

func (a *keyAccounts) UpsertRemoteActor(ctx context.Context, _ uow.Tx, in appaccount.RemoteActorInput) (appaccount.Account, error) {
	select {
	case <-time.After(a.delay):
	case <-ctx.Done():
		return appaccount.Account{}, ctx.Err()
	}
	if len(in.Keys) == 0 {
		return appaccount.Account{}, errors.New("the actor has no key")
	}
	der, err := x509.MarshalPKIXPublicKey(in.Keys[0].PublicKey)
	if err != nil {
		return appaccount.Account{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actor = &account.Account{ID: "remote-1", URI: in.URI, Domain: in.Domain}
	a.key = &account.Key{
		AccountID: a.actor.ID, KeyID: in.Keys[0].KeyID, Algorithm: account.KeyRSASHA256,
		PublicKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})),
	}
	return *a.actor, nil
}

func (a *keyAccounts) FindByID(ctx context.Context, id string) (appaccount.Actor, error) {
	if err := ctx.Err(); err != nil {
		return appaccount.Actor{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.actor == nil || a.actor.ID != id {
		return appaccount.Actor{}, appaccount.ErrNotFound
	}
	return appaccount.Actor{Account: *a.actor, Keys: []account.Key{*a.key}}, nil
}

func (a *keyAccounts) RecordRemoteFetchFailure(context.Context, string) error { return nil }

// keyUnitOfWork runs fn with no transaction, failing as a database does once ctx is done.
type keyUnitOfWork struct{ uow.UnitOfWork }

func (keyUnitOfWork) Tx(ctx context.Context, fn func(uow.Tx) error) error {
	if err := fn(nil); err != nil {
		return err
	}
	return ctx.Err()
}

// keyInstances has seen every server already.
type keyInstances struct{ InstanceRepository }

func (keyInstances) InsertRemoteInstance(context.Context, uow.Tx, string, time.Time) (bool, error) {
	return false, nil
}

const keyActorURI = "https://remote.example/users/alice"

// keyActorNet serves the actor alice at remote.example with a new RSA key, after wait returns and
// unless the request was cut by then.
func keyActorNet(t *testing.T, wait func(*http.Request)) *redirectNet {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := json.Marshal(map[string]any{
		"@context":          []string{"https://www.w3.org/ns/activitystreams", "https://w3id.org/security/v1"},
		"id":                keyActorURI,
		"type":              "Person",
		"preferredUsername": "alice",
		"inbox":             keyActorURI + "/inbox",
		"publicKey": map[string]string{
			"id":           keyActorURI + "#main-key",
			"owner":        keyActorURI,
			"publicKeyPem": string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	n := newRedirectNet()
	n.handle("remote.example", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/alice" {
			http.NotFound(w, r)
			return
		}
		wait(r)
		if r.Context().Err() != nil {
			return // a request cut on the way gets no document, as on the network
		}
		w.Header().Set("Content-Type", "application/activity+json")
		_, _ = w.Write(doc)
	})
	return n
}

func keyResolver(t *testing.T, n *redirectNet, accounts *keyAccounts, timeout time.Duration) *RemoteActors {
	t.Helper()
	return &RemoteActors{d: RemoteActorsDeps{
		UnitOfWork: keyUnitOfWork{}, Accounts: accounts, Instances: keyInstances{},
		Fetcher: testFetcher(t, n), HTTP: n, Clock: fixedClock{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), KeyResolveTimeout: timeout,
	}}
}

// A server that does not answer within the timeout makes the key unavailable (503, which the
// sender retries), never an unexpected error.
func TestResolvePublicKeyOfASlowServerIsUnavailable(t *testing.T) {
	n := keyActorNet(t, func(r *http.Request) { <-r.Context().Done() })
	r := keyResolver(t, n, &keyAccounts{}, 50*time.Millisecond)
	_, err := r.ResolvePublicKey(t.Context(), keyActorURI+"#main-key", false)
	if !errors.Is(err, ErrPublicKeyUnavailable) {
		t.Fatalf("ResolvePublicKey() = %v, want ErrPublicKeyUnavailable", err)
	}
}

// The timeout bounds the fetch alone: storing the fetched actor, and reading it back, go on
// after it.
func TestResolvePublicKeyStoresPastTheTimeout(t *testing.T) {
	n := keyActorNet(t, func(*http.Request) {})
	accounts := &keyAccounts{delay: 100 * time.Millisecond}
	r := keyResolver(t, n, accounts, 50*time.Millisecond)
	key, err := r.ResolvePublicKey(t.Context(), keyActorURI+"#main-key", false)
	if err != nil {
		t.Fatalf("ResolvePublicKey() = %v, want the stored key", err)
	}
	if key.Owner != keyActorURI || key.KeyID != keyActorURI+"#main-key" {
		t.Errorf("ResolvePublicKey() = %+v, want alice's main key", key)
	}
}

// When the request itself ends, its error is not taken for an unavailable server.
func TestResolvePublicKeyOfAnEndedRequestIsNotUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	n := keyActorNet(t, func(r *http.Request) {
		cancel()
		<-r.Context().Done()
	})
	r := keyResolver(t, n, &keyAccounts{}, time.Minute)
	_, err := r.ResolvePublicKey(ctx, keyActorURI+"#main-key", false)
	if err == nil || errors.Is(err, ErrPublicKeyUnavailable) {
		t.Fatalf("ResolvePublicKey() = %v, want an error other than ErrPublicKeyUnavailable", err)
	}
}
