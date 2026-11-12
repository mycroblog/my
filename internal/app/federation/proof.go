// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/domain/federation"
)

// ReasonProofMismatch is the reason of a request refused because the integrity proof of its
// activity does not verify (SignatureError.Reason).
const ReasonProofMismatch = "integrity proof does not verify"

// errProofIgnored reports a proof whose key cannot be used for it, which a verifier ignores
// (FEP-8b32).
var errProofIgnored = errors.New("integrity proof ignored")

// VerifyProof checks the integrity proof of a received activity by actor (FEP-8b32,
// ADR-0006 §30), after its HTTP signature verified. A document without a proof to
// check (federation.ReadProof), or whose proof names a key that cannot be resolved or that is
// not an Ed25519 key, passes unchanged: FEP-8b32 tells a verifier to ignore such a proof and
// rely on other authentication, here the HTTP signature. A proof that names another actor's key
// or that does not verify is refused with Status 401, since FEP-8b32 gives the proof precedence
// over the HTTP signature: when the stored key does not verify it, the key is fetched again
// (at most once per key ID per KeyRefreshInterval, as Verify does) and checked once more.
// verified reports a proof that verified; the error is only for unexpected failures.
func (v *SignatureVerifier) VerifyProof(ctx context.Context, r *SignedRequest, actor federation.IRI) (verified bool, rejected *SignatureError, err error) {
	p, err := federation.ReadProof(r.Body, v.clock.Now())
	switch {
	case errors.Is(err, federation.ErrNoProof):
		return false, nil, nil
	case err != nil:
		return false, v.unauthorized(r, err), nil
	}
	check := func(refresh bool) error {
		key, err := v.keys.ResolvePublicKey(ctx, p.VerificationMethod, refresh)
		switch {
		case errors.Is(err, ErrPublicKeyNotFound), errors.Is(err, ErrPublicKeyUnavailable):
			return errProofIgnored
		case err != nil:
			return fmt.Errorf("resolve the key of an integrity proof: %w", err)
		}
		pub, ok := key.Key.(ed25519.PublicKey)
		if !ok {
			return errProofIgnored
		}
		if owner, ok := signer(&VerifiedSignature{KeyID: key.KeyID, Owner: key.Owner}); !ok || owner != actor {
			return fmt.Errorf("%w: the key is not the actor's", federation.ErrProofMismatch)
		}
		return p.Verify(r.Body, pub)
	}
	verr := check(false)
	if errors.Is(verr, federation.ErrProofMismatch) {
		n, err := v.kv.IncrBy(ctx, KeyRefreshNamespace, p.VerificationMethod, 1, KeyRefreshInterval)
		if err != nil {
			return false, nil, fmt.Errorf("throttle key refresh: %w", err)
		}
		if n == 1 {
			verr = check(true)
		}
	}
	switch {
	case verr == nil:
		return true, nil, nil
	case errors.Is(verr, errProofIgnored):
		return false, nil, nil
	case errors.Is(verr, federation.ErrProofMismatch):
		return false, v.unauthorized(r, verr), nil
	}
	return false, nil, verr
}
