// SPDX-License-Identifier: AGPL-3.0-only

// Package gcpkms implements the KeyManager port of internal/app/kernel with Google Cloud KMS
// (`key_manager = gcpkms`, `kms_key`, `kms_key_previous`; ADR-0121 §1).
//
// It wraps a data-encryption key with the Encrypt method of a symmetric CryptoKey and unwraps it
// with Decrypt, passing the AAD as additionalAuthenticatedData, so the key-encryption key never
// leaves Cloud KMS. A key ID is "gcpkms:" followed by the resource name of the CryptoKey, without
// a version: Decrypt on the CryptoKey picks the version that encrypted the ciphertext, so a
// rotation in Cloud KMS keeps the key ID and needs no rewrap. The client authenticates with
// Application Default Credentials.
package gcpkms

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"log/slog"
	"regexp"
	"strings"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Kind is the kind of the key IDs of this KeyManager (kernel.KeyIDKind).
const Kind = "gcpkms"

const keyIDPrefix = Kind + ":"

// MaxKeyNameLen is the longest CryptoKey resource name: a key ID is at most 255 bytes,
// and the prefix "gcpkms:" takes 7 of them.
const MaxKeyNameLen = 255 - len(keyIDPrefix)

// keyName matches the resource name of a CryptoKey, without a version. The project may be a
// domain-scoped ID ("example.com:project"); key ring and key IDs are at most 63 characters.
var keyName = regexp.MustCompile(`^projects/[^/\s]+/locations/[a-z0-9-]+/keyRings/[A-Za-z0-9_-]{1,63}/cryptoKeys/[A-Za-z0-9_-]{1,63}$`)

// CheckKeyName reports whether name is the resource name of a CryptoKey,
// projects/{project}/locations/{location}/keyRings/{ring}/cryptoKeys/{key}, without a version,
// short enough for a key ID.
func CheckKeyName(name string) error {
	if strings.Contains(name, "/cryptoKeyVersions/") {
		return errors.New("must name the CryptoKey without a version (drop /cryptoKeyVersions/...)")
	}
	if !keyName.MatchString(name) {
		return errors.New("must be the resource name of a Cloud KMS CryptoKey, projects/{project}/locations/{location}/keyRings/{ring}/cryptoKeys/{key}")
	}
	if len(name) > MaxKeyNameLen {
		return fmt.Errorf("must be at most %d bytes, got %d", MaxKeyNameLen, len(name))
	}
	return nil
}

// KeyID returns the key ID of the CryptoKey name.
func KeyID(name string) string { return keyIDPrefix + name }

// Options configure a KeyManager.
type Options struct {
	// Key is the resource name of the CryptoKey that wraps (kms_key).
	Key string
	// PreviousKeys are CryptoKeys that only unwrap (kms_key_previous).
	PreviousKeys []string
	// ClientOptions are passed to the Cloud KMS client, after the defaults (tests only:
	// an endpoint of a test server without authentication).
	ClientOptions []option.ClientOption
}

// KeyManager wraps with the CryptoKey Key and unwraps with Key or any of PreviousKeys.
// It never prints a key: String and LogValue show only the current key ID.
type KeyManager struct {
	client  *kms.KeyManagementClient
	current string
	names   map[string]string // key ID -> CryptoKey name
}

var _ kernel.KeyManager = (*KeyManager)(nil)

// New returns a KeyManager on a Cloud KMS client with Application Default Credentials.
// Every key must pass CheckKeyName, and no key may be given twice. Close releases the client.
func New(ctx context.Context, opts Options) (*KeyManager, error) {
	km := &KeyManager{names: make(map[string]string, 1+len(opts.PreviousKeys))}
	for i, name := range append([]string{opts.Key}, opts.PreviousKeys...) {
		if err := CheckKeyName(name); err != nil {
			return nil, fmt.Errorf("gcpkms key manager: key #%d %q %w", i, name, err)
		}
		id := KeyID(name)
		if _, dup := km.names[id]; dup {
			return nil, fmt.Errorf("gcpkms key manager: key %q is given more than once", name)
		}
		km.names[id] = name
		if i == 0 {
			km.current = id
		}
	}
	client, err := kms.NewKeyManagementClient(ctx, opts.ClientOptions...)
	if err != nil {
		return nil, fmt.Errorf("gcpkms key manager: %w", err)
	}
	km.client = client
	return km, nil
}

// Close releases the Cloud KMS client.
func (k *KeyManager) Close() error { return k.client.Close() }

// CurrentKeyID returns the ID of the CryptoKey that WrapKey uses.
func (k *KeyManager) CurrentKeyID() string { return k.current }

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

func checksum(b []byte) *wrapperspb.Int64Value {
	return wrapperspb.Int64(int64(crc32.Checksum(b, castagnoli)))
}

// errCorrupted reports a request or response that the CRC32C checksums found damaged in
// transit. It is transient: the call may be repeated.
var errCorrupted = errors.New("gcpkms key manager: the request or the response was corrupted in transit")

// WrapKey encrypts dek with the current CryptoKey, authenticating aad.
func (k *KeyManager) WrapKey(ctx context.Context, dek, aad []byte) (string, []byte, error) {
	resp, err := k.client.Encrypt(ctx, &kmspb.EncryptRequest{
		Name:                              k.names[k.current],
		Plaintext:                         dek,
		PlaintextCrc32C:                   checksum(dek),
		AdditionalAuthenticatedData:       aad,
		AdditionalAuthenticatedDataCrc32C: checksum(aad),
	})
	if err != nil {
		return "", nil, k.callError("encrypt", k.current, err)
	}
	if !resp.GetVerifiedPlaintextCrc32C() || !resp.GetVerifiedAdditionalAuthenticatedDataCrc32C() ||
		resp.GetCiphertextCrc32C().GetValue() != checksum(resp.GetCiphertext()).GetValue() {
		return "", nil, errCorrupted
	}
	return k.current, resp.GetCiphertext(), nil
}

// UnwrapKey decrypts a key wrapped under the CryptoKey that keyID names.
func (k *KeyManager) UnwrapKey(ctx context.Context, keyID string, wrapped, aad []byte) ([]byte, error) {
	name, ok := k.names[keyID]
	if !ok {
		return nil, fmt.Errorf("gcpkms key manager: key %q is not configured: %w", keyID, kernel.ErrUnknownKey)
	}
	if len(wrapped) == 0 {
		return nil, fmt.Errorf("gcpkms key manager: key %q: empty ciphertext: %w", keyID, kernel.ErrDecrypt)
	}
	resp, err := k.client.Decrypt(ctx, &kmspb.DecryptRequest{
		Name:                              name,
		Ciphertext:                        wrapped,
		CiphertextCrc32C:                  checksum(wrapped),
		AdditionalAuthenticatedData:       aad,
		AdditionalAuthenticatedDataCrc32C: checksum(aad),
	})
	if err != nil {
		return nil, k.callError("decrypt", keyID, err)
	}
	if resp.GetPlaintextCrc32C().GetValue() != checksum(resp.GetPlaintext()).GetValue() {
		return nil, errCorrupted
	}
	return resp.GetPlaintext(), nil
}

// callError maps an error of a Cloud KMS call. A ciphertext or an AAD that does not
// authenticate (INVALID_ARGUMENT) wraps kernel.ErrDecrypt. A missing permission, key, or enabled
// key version is a mistake of the configuration or of the operation of the KMS and is returned
// with a hint; quotas (RESOURCE_EXHAUSTED) and the other failures are transient, and the call
// may be repeated. The returned error keeps the gRPC status (status.FromError).
func (k *KeyManager) callError(op, keyID string, err error) error {
	switch status.Code(err) {
	case codes.InvalidArgument:
		if op == "decrypt" {
			return fmt.Errorf("gcpkms key manager: %s with key %q: %w: %w", op, keyID, kernel.ErrDecrypt, err)
		}
	case codes.PermissionDenied, codes.Unauthenticated:
		return fmt.Errorf("gcpkms key manager: %s with key %q: the service account lacks roles/cloudkms.cryptoKeyEncrypterDecrypter on the key, or the key does not exist: %w", op, keyID, err)
	case codes.NotFound:
		return fmt.Errorf("gcpkms key manager: %s with key %q: the key does not exist: %w", op, keyID, err)
	case codes.FailedPrecondition:
		return fmt.Errorf("gcpkms key manager: %s with key %q: the key version is not enabled, or the key is not for symmetric encryption: %w", op, keyID, err)
	}
	return fmt.Errorf("gcpkms key manager: %s with key %q: %w", op, keyID, err)
}

// String describes the KeyManager without its keys.
func (k *KeyManager) String() string {
	return fmt.Sprintf("gcpkms.KeyManager{current: %s, keys: %d}", k.current, len(k.names))
}

// LogValue keeps the KeyManager's state out of logs, like the other key managers (ADR-0019 §6).
func (k *KeyManager) LogValue() slog.Value {
	return slog.GroupValue(slog.String("current", k.current), slog.Int("keys", len(k.names)))
}
