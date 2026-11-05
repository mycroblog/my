// SPDX-License-Identifier: AGPL-3.0-only

package contract

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	keymanagerplugin "github.com/mycroblog/my/internal/adapter/keymanager/plugin"
	"github.com/mycroblog/my/internal/app/kernel"
)

// keyManagerCases check a key manager plugin through the server's adapter, so that they hold it to what the secrets need.
// They only ever send keys they make up and throw away (ADR-0146 §16). What they wrap stays with the plugin's key, so they
// have no side effect but the calls, which a KMS behind the plugin may bill.
var keyManagerCases = []Case{
	{Name: "describe", Run: keyManagerDescribe},
	{Name: "self_test", Run: keyManagerSelfTest},
	{Name: "unwrap_changed_value", Run: keyManagerChangedValue},
	{Name: "unwrap_unknown_key", Run: keyManagerUnknownKey},
}

// keyManagerUnknownID is the ID of a KEK that no plugin holds.
const keyManagerUnknownID = "plugin:my-plugin-test-no-such-key"

// keyManagerOf returns the adapter of the plugin, which has asked the plugin for its current key.
func keyManagerOf(ctx context.Context, env Env) (*keymanagerplugin.KeyManager, error) {
	m := keymanagerplugin.New(env.Conn)
	if err := m.Describe(ctx); err != nil {
		return nil, describeCallError(env, err)
	}
	return m, nil
}

// keyManagerDescribe asks for the current key, in the form the server stores.
func keyManagerDescribe(ctx context.Context, env Env) error {
	_, err := keyManagerOf(ctx, env)
	return err
}

// keyManagerSelfTest runs what the server runs at each start, whose failure stops the start.
func keyManagerSelfTest(ctx context.Context, env Env) error {
	m, err := keyManagerOf(ctx, env)
	if err != nil {
		return err
	}
	if err := m.SelfTest(ctx); err != nil {
		return describeCallError(env, err)
	}
	return nil
}

// wrapThrownAwayKey wraps a random key under the plugin's current KEK.
func wrapThrownAwayKey(ctx context.Context, env Env, m *keymanagerplugin.KeyManager, aad []byte) (string, []byte, error) {
	dek := make([]byte, 32)
	_, _ = rand.Read(dek) // crypto/rand.Read never returns an error
	id, wrapped, err := m.WrapKey(ctx, dek, aad)
	if err != nil {
		return "", nil, describeCallError(env, err)
	}
	return id, wrapped, nil
}

// keyManagerChangedValue checks that a wrapped value with a changed byte or cut short is refused, which is what tells a
// tampered database from a good one.
func keyManagerChangedValue(ctx context.Context, env Env) error {
	m, err := keyManagerOf(ctx, env)
	if err != nil {
		return err
	}
	aad := kernel.AAD("my plugin test", "wrapped")
	id, wrapped, err := wrapThrownAwayKey(ctx, env, m, aad)
	if err != nil {
		return err
	}
	changed := append([]byte(nil), wrapped...)
	changed[len(changed)/2] ^= 0x01
	for _, tc := range []struct {
		what  string
		value []byte
	}{{"a changed byte", changed}, {"a shorter value", wrapped[:len(wrapped)-1]}} {
		switch _, err := m.UnwrapKey(ctx, id, tc.value, aad); {
		case errors.Is(err, kernel.ErrDecrypt):
		case err == nil:
			return fmt.Errorf("%s opened: the outcome is OUTCOME_OK, not OUTCOME_DECRYPT", tc.what)
		case errors.Is(err, kernel.ErrUnknownKey):
			return fmt.Errorf("%s: the outcome is OUTCOME_UNKNOWN_KEY, not OUTCOME_DECRYPT", tc.what)
		default:
			return fmt.Errorf("%s: %w", tc.what, describeCallError(env, err))
		}
	}
	return nil
}

// keyManagerUnknownKey checks that a KEK the plugin does not hold is reported as such: the server tells an operator that a
// KEK is gone, which is no damaged value.
func keyManagerUnknownKey(ctx context.Context, env Env) error {
	m, err := keyManagerOf(ctx, env)
	if err != nil {
		return err
	}
	aad := kernel.AAD("my plugin test", "wrapped")
	_, wrapped, err := wrapThrownAwayKey(ctx, env, m, aad)
	if err != nil {
		return err
	}
	switch _, err := m.UnwrapKey(ctx, keyManagerUnknownID, wrapped, aad); {
	case errors.Is(err, kernel.ErrUnknownKey):
		return nil
	case err == nil:
		return errors.New("a key the plugin does not hold opened: the outcome is OUTCOME_OK, not OUTCOME_UNKNOWN_KEY")
	case errors.Is(err, kernel.ErrDecrypt):
		return errors.New("a key the plugin does not hold: the outcome is OUTCOME_DECRYPT, not OUTCOME_UNKNOWN_KEY")
	default:
		return describeCallError(env, err)
	}
}
