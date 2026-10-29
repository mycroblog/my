// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"context"
	"fmt"
	"strings"
)

// KeyIDKind returns the kind of KeyManager that a key ID names: the part before its first colon,
// such as "local", "gcpkms", or "awskms". It returns "" for an ID without one.
func KeyIDKind(keyID string) string {
	kind, _, ok := strings.Cut(keyID, ":")
	if !ok {
		return ""
	}
	return kind
}

// NewKeyRouter returns a KeyManager that wraps with current and unwraps with the KeyManager of
// the kind of the key ID (KeyIDKind): current for its own kind, and each of openers for the kind
// of its CurrentKeyID. It moves the sealed values from one kind of KeyManager to another
// (ADR-0121 §4): with the new one as current and the old one as an opener, `my kek rewrap`
// re-wraps every value under the new one. Each KeyManager must have a different kind.
func NewKeyRouter(current KeyManager, openers ...KeyManager) (KeyManager, error) {
	if len(openers) == 0 {
		return current, nil
	}
	r := &keyRouter{current: current, byKind: map[string]KeyManager{}}
	for _, km := range append([]KeyManager{current}, openers...) {
		kind := KeyIDKind(km.CurrentKeyID())
		if kind == "" {
			return nil, fmt.Errorf("key router: key ID %q has no kind", km.CurrentKeyID())
		}
		if _, dup := r.byKind[kind]; dup {
			return nil, fmt.Errorf("key router: two key managers of kind %q", kind)
		}
		r.byKind[kind] = km
	}
	return r, nil
}

type keyRouter struct {
	current KeyManager
	byKind  map[string]KeyManager
}

func (r *keyRouter) CurrentKeyID() string { return r.current.CurrentKeyID() }

func (r *keyRouter) WrapKey(ctx context.Context, dek, aad []byte) (string, []byte, error) {
	return r.current.WrapKey(ctx, dek, aad)
}

func (r *keyRouter) UnwrapKey(ctx context.Context, keyID string, wrapped, aad []byte) ([]byte, error) {
	km, ok := r.byKind[KeyIDKind(keyID)]
	if !ok {
		return nil, fmt.Errorf("key router: no key manager for key %q: %w", keyID, ErrUnknownKey)
	}
	return km.UnwrapKey(ctx, keyID, wrapped, aad)
}

// String describes the router without its keys.
func (r *keyRouter) String() string {
	return fmt.Sprintf("kernel.KeyRouter{current: %s, kinds: %d}", r.current.CurrentKeyID(), len(r.byKind))
}
