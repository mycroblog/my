// SPDX-License-Identifier: AGPL-3.0-only

// Package plugin is the kernel.KeyManager that delegates to an operator's plugin, such as one before Vault or an HSM
// (`key_manager = plugin`, ADR-0146 §11, §14). The KEK stays with the plugin, which sees each data-encryption key in
// plaintext, so the plugin sits in the same trust boundary as the KEK.
//
// A key ID is "plugin:" followed by the plugin's own ID, so kernel.NewKeyRouter moves sealed values to and from the local
// KEK as it does for a KMS.
//
// It fails closed: every failed call, deadline, unknown outcome, and malformed answer is an error. Nothing here stores,
// falls back to another key, or opens a value by any other way, and the only retry is the wait of Start, for a sidecar
// that comes up after the server. The caller's job retries the rest.
//
// A DEK, an AAD, and a wrapped value reach no log, error, or metric: nothing here logs, its errors are fixed texts and the
// Connect code of the client, which drops all a plugin says in a failure, and no error quotes a value of a response
// (ADR-0146 §10).
package plugin

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"connectrpc.com/connect/v2"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	keymanagerv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/key_manager/v1"
	keymanagerv1connect "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/key_manager/v1/key_managerv1connect"
	"github.com/mycroblog/my/internal/app/kernel"
)

// Kind is the kind of the key IDs of this KeyManager (kernel.KeyIDKind).
const Kind = "plugin"

const keyIDPrefix = Kind + ":"

// MaxPluginKeyIDLen is the longest ID a plugin may give a KEK: a key ID is at most 255 bytes, and the prefix takes 7.
const MaxPluginKeyIDLen = 255 - len(keyIDPrefix)

// maxWrapped is the longest wrapped key, which a sealed value holds behind a length of 2 bytes (kernel.Sealer).
const maxWrapped = 0xffff

// StartWait is how long Start waits for a plugin that does not answer, for a sidecar that comes up after the server
// (ADR-0146 §14).
const StartWait = 30 * time.Second

// startInterval is the pause between the attempts of Start.
const startInterval = time.Second

// selfTestDEKSize is the length of the key that the self-test throws away, that of a real data-encryption key.
const selfTestDEKSize = 32

// KeyManager is the kernel.KeyManager of a plugin. It is safe for concurrent use.
type KeyManager struct {
	conn    *client.Conn
	client  keymanagerv1connect.KeyManagerServiceClient
	current atomic.Pointer[string]
}

var _ kernel.KeyManager = (*KeyManager)(nil)

// New returns the KeyManager that reaches its plugin through conn. It makes no call and has no current key until Describe
// returns: the server starts one with Start.
func New(conn *client.Conn) *KeyManager {
	return &KeyManager{conn: conn, client: keymanagerv1connect.NewKeyManagerServiceClient(conn.RPC)}
}

// Start returns the KeyManager of conn after it asked the plugin for its current key and passed SelfTest. It tries again
// for up to wait while the plugin does not answer. An answer that fails the test ends it at once, since waiting does not
// mend a plugin that answers wrongly. An error means the server must not start.
func Start(ctx context.Context, conn *client.Conn, wait time.Duration) (*KeyManager, error) {
	m := New(conn)
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	for {
		err := m.start(ctx)
		if err == nil {
			return m, nil
		}
		if !unreachable(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("the plugin did not answer in %s: %w", wait, err)
		case <-time.After(startInterval):
		}
	}
}

func (m *KeyManager) start(ctx context.Context) error {
	if err := m.Describe(ctx); err != nil {
		return err
	}
	return m.SelfTest(ctx)
}

// unreachable reports whether a failure is one that a plugin still coming up gives. A plugin that answers with anything
// else, such as a refusal of the credentials, will not answer differently in a moment.
func unreachable(err error) bool {
	switch connect.CodeOf(err) {
	case connect.CodeUnavailable, connect.CodeDeadlineExceeded:
		return true
	default:
		return false
	}
}

// Close closes the idle connections to the plugin.
func (m *KeyManager) Close() error {
	m.conn.Close()
	return nil
}

// Describe asks the plugin for the ID of its current KEK, which CurrentKeyID returns from then on (ADR-0146 §11).
func (m *KeyManager) Describe(ctx context.Context) error {
	res, err := m.client.Describe(ctx, new(keymanagerv1.DescribeRequest))
	if err != nil {
		return fmt.Errorf("key manager plugin: describe: %w", err)
	}
	if res.GetOutcome() != keymanagerv1.Outcome_OUTCOME_OK {
		return notOK("describe", res.GetOutcome())
	}
	id, err := fullKeyID(res.GetKeyId())
	if err != nil {
		return fmt.Errorf("key manager plugin: describe: %w", err)
	}
	m.current.Store(&id)
	return nil
}

// CurrentKeyID implements kernel.KeyManager: the ID of Describe, or of the last WrapKey if the KEK rotated since.
func (m *KeyManager) CurrentKeyID() string {
	if id := m.current.Load(); id != nil {
		return *id
	}
	return ""
}

// WrapKey implements kernel.KeyManager. It returns a value only when the plugin wrapped the key: an answer that holds the
// plaintext of the key, whatever the self-test saw at start, is an error, so that nothing is stored in plaintext.
func (m *KeyManager) WrapKey(ctx context.Context, dek, aad []byte) (string, []byte, error) {
	if len(dek) == 0 {
		return "", nil, errors.New("key manager plugin: wrap: no key to wrap")
	}
	res, err := m.client.WrapKey(ctx, &keymanagerv1.WrapKeyRequest{Dek: dek, Aad: aad})
	if err != nil {
		return "", nil, fmt.Errorf("key manager plugin: wrap: %w", err)
	}
	if res.GetOutcome() != keymanagerv1.Outcome_OUTCOME_OK {
		return "", nil, notOK("wrap", res.GetOutcome())
	}
	id, err := fullKeyID(res.GetKeyId())
	if err != nil {
		return "", nil, fmt.Errorf("key manager plugin: wrap: %w", err)
	}
	wrapped := res.GetWrapped()
	switch {
	case len(wrapped) == 0:
		return "", nil, errors.New("key manager plugin: wrap: the wrapped value is empty")
	case len(wrapped) > maxWrapped:
		return "", nil, fmt.Errorf("key manager plugin: wrap: the wrapped value is over %d bytes", maxWrapped)
	case bytes.Contains(wrapped, dek):
		return "", nil, errors.New("key manager plugin: wrap: the wrapped value holds the key in plaintext")
	}
	m.current.Store(&id)
	return id, wrapped, nil
}

// UnwrapKey implements kernel.KeyManager. An ID that is not the plugin's fails with kernel.ErrUnknownKey with no call:
// what a plugin holds has IDs of this kind only.
func (m *KeyManager) UnwrapKey(ctx context.Context, keyID string, wrapped, aad []byte) ([]byte, error) {
	name, ok := strings.CutPrefix(keyID, keyIDPrefix)
	if !ok || name == "" {
		return nil, fmt.Errorf("key manager plugin: %w", kernel.ErrUnknownKey)
	}
	res, err := m.client.UnwrapKey(ctx, &keymanagerv1.UnwrapKeyRequest{KeyId: name, Wrapped: wrapped, Aad: aad})
	if err != nil {
		return nil, fmt.Errorf("key manager plugin: unwrap: %w", err)
	}
	switch res.GetOutcome() {
	case keymanagerv1.Outcome_OUTCOME_OK:
		if len(res.GetDek()) == 0 {
			return nil, errors.New("key manager plugin: unwrap: the key is empty")
		}
		return res.GetDek(), nil
	case keymanagerv1.Outcome_OUTCOME_UNKNOWN_KEY:
		return nil, fmt.Errorf("key manager plugin: %w", kernel.ErrUnknownKey)
	case keymanagerv1.Outcome_OUTCOME_DECRYPT:
		return nil, fmt.Errorf("key manager plugin: %w", kernel.ErrDecrypt)
	default:
		return nil, notOK("unwrap", res.GetOutcome())
	}
}

// SelfTest checks with a random key it throws away that the plugin does what the secrets of the server rely on, and stops
// a plugin that passes values through or ignores the AAD (ADR-0146 §14): a wrap and an unwrap give the key back, an unwrap
// with a different AAD gives DECRYPT, two wraps of a key differ, and the wrapped value is not the key, which WrapKey itself
// checks. Its errors keep the Connect code of a failed call.
func (m *KeyManager) SelfTest(ctx context.Context) error {
	dek := make([]byte, selfTestDEKSize)
	_, _ = rand.Read(dek) // crypto/rand.Read never returns an error
	// The AAD is no secret, and a fixed one would let a plugin that remembers its last call pass.
	aad, other := selfTestAAD(), selfTestAAD()

	id, wrapped, err := m.WrapKey(ctx, dek, aad)
	if err != nil {
		return fmt.Errorf("self-test: %w", err)
	}
	opened, err := m.UnwrapKey(ctx, id, wrapped, aad)
	if err != nil {
		return fmt.Errorf("self-test: the round trip failed: %w", err)
	}
	if !bytes.Equal(opened, dek) {
		return errors.New("key manager plugin: self-test: the round trip gave back another key")
	}
	switch _, err := m.UnwrapKey(ctx, id, wrapped, other); {
	case err == nil:
		return errors.New("key manager plugin: self-test: a different AAD opened the key; the plugin must answer OUTCOME_DECRYPT")
	case errors.Is(err, kernel.ErrUnknownKey):
		return errors.New("key manager plugin: self-test: a different AAD gave OUTCOME_UNKNOWN_KEY, not OUTCOME_DECRYPT")
	case !errors.Is(err, kernel.ErrDecrypt):
		return fmt.Errorf("self-test: a different AAD did not give OUTCOME_DECRYPT: %w", err)
	}
	_, again, err := m.WrapKey(ctx, dek, aad)
	if err != nil {
		return fmt.Errorf("self-test: %w", err)
	}
	if bytes.Equal(again, wrapped) {
		return errors.New("key manager plugin: self-test: two wraps of a key gave the same value; the plugin must use a fresh nonce")
	}
	return nil
}

func selfTestAAD() []byte {
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce) // crypto/rand.Read never returns an error
	return kernel.AAD("key manager plugin self-test", string(nonce))
}

// fullKeyID returns the key ID of a plugin's ID. The ID goes into each sealed value and into logs, so it is short visible
// ASCII, and the error does not quote what the plugin said.
func fullKeyID(name string) (string, error) {
	if name == "" || len(name) > MaxPluginKeyIDLen || strings.ContainsFunc(name, func(r rune) bool { return r <= ' ' || r > '~' }) {
		return "", fmt.Errorf("the key ID is not 1 to %d bytes of visible ASCII", MaxPluginKeyIDLen)
	}
	return keyIDPrefix + name, nil
}

// notOK returns the error of an outcome that does not fit the call: an empty response, or a value this server does not
// know, is a failure (ADR-0146 §10).
func notOK(call string, outcome keymanagerv1.Outcome) error {
	return fmt.Errorf("key manager plugin: %s: the outcome is %d, which is not a result of the call", call, int32(outcome))
}
