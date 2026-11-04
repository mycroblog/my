// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/federation"
)

// The acceptance window of ADR-0006 and ADR-0019 §7.
const (
	// SignatureMaxAge is how old a draft-cavage Date (or created) may be, and how long an
	// RFC 9421 signature without expires stays valid after created.
	SignatureMaxAge = 12 * time.Hour
	// SignatureClockSkew is how far a signature may lie in the future, and how long after
	// its expiry it is still accepted.
	SignatureClockSkew = time.Hour
	// KeyRefreshInterval is how often a key ID's owner is fetched again after a signature
	// failed to verify with the stored key (key rotation).
	KeyRefreshInterval = 5 * time.Minute
	// MinRSAKeyBits is the smallest RSA modulus accepted.
	MinRSAKeyBits = 2048
)

// KeyRefreshNamespace is the KeyValueStore namespace that throttles key refreshes per key ID.
const KeyRefreshNamespace = "federation.key_refresh"

// Accept-Signature values (RFC 9421 §5.1) sent with 401 responses: the components the RFC 9421
// profile of ADR-0006 requires, with created.
const (
	acceptSignatureGET  = `sig1=("@method" "@target-uri");created`
	acceptSignaturePOST = `sig1=("@method" "@target-uri" "content-digest");created`
)

// ErrSignatureRejected matches every *SignatureError.
var ErrSignatureRejected = errors.New("HTTP signature rejected")

// SignatureError reports a request that the signature policy refuses. The inbox answers with
// Status and, when it is set, the Accept-Signature header.
type SignatureError struct {
	// Status is 400 (unparsable signature), 401 (missing, insufficient, or invalid signature),
	// or 503 (the key's origin cannot be reached now).
	Status int
	// AcceptSignature is the Accept-Signature field value for a 401 response.
	AcceptSignature string
	Err             error
}

func (e *SignatureError) Error() string {
	return fmt.Sprintf("HTTP signature rejected (%d): %v", e.Status, e.Err)
}

// Unwrap returns the cause.
func (e *SignatureError) Unwrap() error { return e.Err }

// Is makes errors.Is(err, ErrSignatureRejected) hold.
func (e *SignatureError) Is(target error) bool { return target == ErrSignatureRejected }

// The reasons of a refused signature (SignatureError.Reason), a fixed vocabulary for logs.
const (
	ReasonNoSignature          = "no signature"
	ReasonMalformedSignature   = "malformed signature"
	ReasonSignaturePolicy      = "signature does not meet the policy"
	ReasonPublicKeyNotFound    = "public key not found"
	ReasonPublicKeyUnavailable = "public key unavailable"
	ReasonSignatureMismatch    = "signature does not verify"
)

// Reason names why the signature was refused, from the fixed vocabulary above. Logs record it
// instead of the error itself: Err quotes values of the request's signature headers (keyId,
// algorithms, digests, Date), which logs must not hold verbatim (ADR-0019).
func (e *SignatureError) Reason() string {
	switch {
	case errors.Is(e.Err, ErrNoSignature):
		return ReasonNoSignature
	case errors.Is(e.Err, ErrMalformedSignature), e.Status == http.StatusBadRequest:
		return ReasonMalformedSignature
	case errors.Is(e.Err, ErrPublicKeyUnavailable):
		return ReasonPublicKeyUnavailable
	case errors.Is(e.Err, ErrPublicKeyNotFound):
		return ReasonPublicKeyNotFound
	case errors.Is(e.Err, ErrSignatureMismatch):
		return ReasonSignatureMismatch
	case errors.Is(e.Err, federation.ErrProofMismatch):
		return ReasonProofMismatch
	}
	return ReasonSignaturePolicy
}

// PublicKey is a resolved remote key.
type PublicKey struct {
	KeyID string
	// Owner is the URI of the actor that owns the key.
	Owner string
	// Key is *rsa.PublicKey or ed25519.PublicKey.
	Key crypto.PublicKey
}

// PublicKeyResolver finds the public key of a key ID: from account_keys, fetching the owner's
// document when the key is unknown or stale (ADR-0006).
type PublicKeyResolver interface {
	// ResolvePublicKey returns the key named keyID. With refresh it fetches the owner's
	// document again even when a stored key is fresh. It fails with an error wrapping
	// ErrPublicKeyNotFound when the key cannot be obtained, and ErrPublicKeyUnavailable when its
	// origin is not reachable now (circuit breaker).
	ResolvePublicKey(ctx context.Context, keyID string, refresh bool) (PublicKey, error)
}

var (
	// ErrPublicKeyNotFound reports a key ID whose key cannot be obtained.
	ErrPublicKeyNotFound = errors.New("public key not found")
	// ErrPublicKeyUnavailable reports a key whose origin is temporarily not reachable.
	ErrPublicKeyUnavailable = errors.New("public key unavailable")
)

// UnknownPublicKeys is the PublicKeyResolver that knows no key: every signature fails to
// verify, and object fetches see every requester as anonymous. Tests use it where no key
// should resolve; the server uses RemoteActors.
type UnknownPublicKeys struct{}

// ResolvePublicKey implements PublicKeyResolver.
func (UnknownPublicKeys) ResolvePublicKey(_ context.Context, keyID string, _ bool) (PublicKey, error) {
	return PublicKey{}, fmt.Errorf("%w: %s", ErrPublicKeyNotFound, keyID)
}

// VerifiedSignature is a signature that passed the policy and verified.
type VerifiedSignature struct {
	Scheme SignatureScheme
	KeyID  string
	// Owner is the actor that owns the key; the inbox compares it with the activity's actor.
	Owner string
}

// SignatureVerifier applies the receive profile of ADR-0006 (docs/architecture/federation.md
// §2, §3.1): it parses the signature, checks the required components, the time window, and the
// body digest without any I/O (Check), then resolves the key and verifies the signature,
// refetching a rotated key at most once per KeyRefreshInterval (Verify).
type SignatureVerifier struct {
	verifier Verifier
	keys     PublicKeyResolver
	kv       kernel.KeyValueStore
	clock    kernel.Clock
}

// NewSignatureVerifier returns a SignatureVerifier. kv throttles key refreshes.
func NewSignatureVerifier(v Verifier, keys PublicKeyResolver, kv kernel.KeyValueStore, clock kernel.Clock) *SignatureVerifier {
	return &SignatureVerifier{verifier: v, keys: keys, kv: kv, clock: clock}
}

// Parse returns the signature of r. A request without one fails with Status 401 and a
// malformed one with 400.
func (v *SignatureVerifier) Parse(r *SignedRequest) (*Signature, error) {
	sig, err := v.verifier.Parse(r)
	switch {
	case errors.Is(err, ErrNoSignature):
		return nil, v.unauthorized(r, err)
	case err != nil:
		return nil, &SignatureError{Status: http.StatusBadRequest, Err: err}
	}
	return sig, nil
}

// Check applies the parts of the policy that need no key: the required components and
// parameters, the declared algorithm, the time window, and the body digest. A refused
// request fails with Status 401.
func (v *SignatureVerifier) Check(r *SignedRequest, sig *Signature) error {
	now := v.clock.Now()
	var err error
	switch sig.Scheme {
	case SchemeCavage:
		err = checkCavage(r, sig, now)
	case SchemeRFC9421:
		err = checkRFC9421(r, sig, now)
	default:
		err = fmt.Errorf("unknown signature scheme %q", sig.Scheme)
	}
	if err == nil {
		err = checkDigests(r, sig)
	}
	if err != nil {
		return v.unauthorized(r, err)
	}
	return nil
}

// Verify resolves the key of sig and verifies it, as Check passed it. When the stored key does
// not verify, it fetches the key again (at most once per key ID per KeyRefreshInterval) and
// verifies once more. A refused request comes back as the *SignatureError, with Status 401, or
// 503 when the key's origin is not reachable now; the error is only for unexpected failures.
// They are separate results so that a caller can log an unexpected failure without the
// signature header values that a refusal quotes.
func (v *SignatureVerifier) Verify(ctx context.Context, r *SignedRequest, sig *Signature) (*VerifiedSignature, *SignatureError, error) {
	key, rejected, err := v.resolve(ctx, r, sig.KeyID, false)
	if rejected != nil || err != nil {
		return nil, rejected, err
	}
	verr := v.verifyWith(r, sig, key)
	if verr != nil && errors.Is(verr, ErrSignatureMismatch) {
		n, err := v.kv.IncrBy(ctx, KeyRefreshNamespace, sig.KeyID, 1, KeyRefreshInterval)
		if err != nil {
			return nil, nil, fmt.Errorf("throttle key refresh: %w", err)
		}
		if n == 1 {
			fresh, rejected, err := v.resolve(ctx, r, sig.KeyID, true)
			if rejected != nil || err != nil {
				return nil, rejected, err
			}
			key, verr = fresh, v.verifyWith(r, sig, fresh)
		}
	}
	if verr != nil {
		return nil, v.unauthorized(r, verr), nil
	}
	return &VerifiedSignature{Scheme: sig.Scheme, KeyID: sig.KeyID, Owner: key.Owner}, nil, nil
}

// resolve returns the key of keyID, the *SignatureError of a key that cannot be obtained, or
// an unexpected error.
func (v *SignatureVerifier) resolve(ctx context.Context, r *SignedRequest, keyID string, refresh bool) (PublicKey, *SignatureError, error) {
	key, err := v.keys.ResolvePublicKey(ctx, keyID, refresh)
	switch {
	case errors.Is(err, ErrPublicKeyUnavailable):
		return PublicKey{}, &SignatureError{Status: http.StatusServiceUnavailable, Err: err}, nil
	case errors.Is(err, ErrPublicKeyNotFound):
		return PublicKey{}, v.unauthorized(r, err), nil
	case err != nil:
		return PublicKey{}, nil, fmt.Errorf("resolve public key: %w", err)
	}
	return key, nil, nil
}

// verifyWith verifies sig with key, trying every algorithm the policy allows for it and, for
// draft-cavage requests with a query string, the (request-target) without the query.
func (v *SignatureVerifier) verifyWith(r *SignedRequest, sig *Signature, key PublicKey) error {
	algs, err := algorithmsFor(sig, key.Key)
	if err != nil {
		return err
	}
	excludeQuery := []bool{false}
	if sig.Scheme == SchemeCavage && r.URL.RawQuery != "" {
		excludeQuery = append(excludeQuery, true)
	}
	var last error
	for _, alg := range algs {
		for _, eq := range excludeQuery {
			err := v.verifier.Verify(r, sig, VerifyOptions{PublicKey: key.Key, Algorithm: alg, ExcludeQuery: eq})
			if err == nil {
				return nil
			}
			last = err
		}
	}
	return last
}

// algorithmsFor returns the algorithms to try for sig with key, in order. The declared
// algorithm must match the key type; without one, the key type decides (an RSA key tries
// rsa-v1_5-sha256, then rsa-pss-sha512 for RFC 9421).
func algorithmsFor(sig *Signature, key crypto.PublicKey) ([]string, error) {
	switch k := key.(type) {
	case *rsa.PublicKey:
		if k.N.BitLen() < MinRSAKeyBits {
			return nil, fmt.Errorf("%w: RSA key of %d bits", ErrSignatureMismatch, k.N.BitLen())
		}
		switch {
		case sig.Scheme == SchemeCavage && (sig.Algorithm == "" || sig.Algorithm == "rsa-sha256" || sig.Algorithm == "hs2019"):
			return []string{AlgRSAPKCS1SHA256}, nil
		case sig.Scheme == SchemeRFC9421 && sig.Algorithm == "":
			return []string{AlgRSAPKCS1SHA256, AlgRSAPSSSHA512}, nil
		case sig.Scheme == SchemeRFC9421 && (sig.Algorithm == AlgRSAPKCS1SHA256 || sig.Algorithm == AlgRSAPSSSHA512):
			return []string{sig.Algorithm}, nil
		}
	case ed25519.PublicKey:
		switch {
		case sig.Scheme == SchemeCavage && (sig.Algorithm == "" || sig.Algorithm == "hs2019"):
			return []string{AlgEd25519}, nil
		case sig.Scheme == SchemeRFC9421 && (sig.Algorithm == "" || sig.Algorithm == AlgEd25519):
			return []string{AlgEd25519}, nil
		}
	default:
		return nil, fmt.Errorf("%w: unsupported key type %T", ErrSignatureMismatch, key)
	}
	return nil, fmt.Errorf("%w: algorithm %q does not match the key", ErrSignatureMismatch, sig.Algorithm)
}

func (v *SignatureVerifier) unauthorized(r *SignedRequest, err error) *SignatureError {
	accept := acceptSignatureGET
	if r.Method == http.MethodPost {
		accept = acceptSignaturePOST
	}
	return &SignatureError{Status: http.StatusUnauthorized, AcceptSignature: accept, Err: err}
}

func checkCavage(r *SignedRequest, sig *Signature, now time.Time) error {
	switch sig.Algorithm {
	case "", "rsa-sha256", "hs2019":
	default:
		return fmt.Errorf("unsupported draft-cavage algorithm %q", sig.Algorithm)
	}
	if sig.KeyID == "" {
		return errors.New("missing keyId")
	}
	required := []string{"(request-target)", "host"}
	if r.Method == http.MethodPost {
		required = append(required, "digest")
	}
	for _, c := range required {
		if !sig.Covers(c) {
			return fmt.Errorf("signature does not cover %s", c)
		}
	}
	if !sig.Covers("date") && !sig.Covers("(created)") {
		return errors.New("signature covers neither date nor (created)")
	}
	if sig.Covers("date") {
		date, err := http.ParseTime(r.Header.Get("Date"))
		if err != nil {
			return fmt.Errorf("unparsable Date header: %w", err)
		}
		if err := checkAge(date, now); err != nil {
			return fmt.Errorf("date: %w", err)
		}
	}
	if sig.Covers("(created)") {
		if sig.Created.IsZero() {
			return errors.New("(created) covered without a created parameter")
		}
		if err := checkAge(sig.Created, now); err != nil {
			return fmt.Errorf("created: %w", err)
		}
	}
	if !sig.Expires.IsZero() && now.After(sig.Expires.Add(SignatureClockSkew)) {
		return errors.New("signature expired")
	}
	return nil
}

func checkAge(t, now time.Time) error {
	if t.After(now.Add(SignatureClockSkew)) {
		return errors.New("in the future")
	}
	if t.Before(now.Add(-SignatureMaxAge)) {
		return errors.New("too old")
	}
	return nil
}

func checkRFC9421(r *SignedRequest, sig *Signature, now time.Time) error {
	switch sig.Algorithm {
	case "", AlgRSAPKCS1SHA256, AlgRSAPSSSHA512, AlgEd25519:
	default:
		return fmt.Errorf("unsupported RFC 9421 algorithm %q", sig.Algorithm)
	}
	if sig.KeyID == "" {
		return errors.New("missing keyid parameter")
	}
	if sig.Created.IsZero() {
		return errors.New("missing created parameter")
	}
	if !sig.Covers("@method") {
		return errors.New("signature does not cover @method")
	}
	if !sig.Covers("@target-uri") {
		if !sig.Covers("@authority") || !sig.Covers("@path") {
			return errors.New("signature covers neither @target-uri nor @authority and @path")
		}
		if r.URL.RawQuery != "" && !sig.Covers("@query") {
			return errors.New("signature does not cover @query")
		}
	}
	if r.Method == http.MethodPost && !sig.Covers("content-digest") {
		return errors.New("signature does not cover content-digest")
	}
	if sig.Created.After(now.Add(SignatureClockSkew)) {
		return errors.New("created is in the future")
	}
	expires := sig.Expires
	if expires.IsZero() {
		expires = sig.Created.Add(SignatureMaxAge)
	}
	if now.After(expires.Add(SignatureClockSkew)) {
		return errors.New("signature expired")
	}
	return nil
}

// checkDigests checks the declared body digests when the signature covers them. POST requests
// must carry one (Check has made sure that the signature covers it). Digests in algorithms
// other than SHA-256 and SHA-512 are ignored; at least one known digest must be present, and
// every known one must match the body.
func checkDigests(r *SignedRequest, sig *Signature) error {
	component := "digest"
	if sig.Scheme == SchemeRFC9421 {
		component = "content-digest"
	}
	if !sig.Covers(component) {
		return nil
	}
	var sum256, sum512 []byte
	known := 0
	for _, d := range sig.Digests {
		var want []byte
		switch d.Algorithm {
		case "sha-256":
			if sum256 == nil {
				s := sha256.Sum256(r.Body)
				sum256 = s[:]
			}
			want = sum256
		case "sha-512":
			if sum512 == nil {
				s := sha512.Sum512(r.Body)
				sum512 = s[:]
			}
			want = sum512
		default:
			continue
		}
		known++
		if len(d.Value) != len(want) || subtle.ConstantTimeCompare(d.Value, want) != 1 {
			return fmt.Errorf("%s digest does not match the body", d.Algorithm)
		}
	}
	if known == 0 {
		return errors.New("no SHA-256 or SHA-512 body digest")
	}
	return nil
}
