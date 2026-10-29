// SPDX-License-Identifier: AGPL-3.0-only

// Package awskms implements the KeyManager port of internal/app/kernel with AWS KMS
// (`key_manager = awskms`, `kms_key`, `kms_key_previous`; ADR-0121 §1).
//
// It wraps a data-encryption key with the Encrypt operation of a symmetric KMS key and unwraps
// it with Decrypt, so the key-encryption key never leaves AWS KMS. The AAD goes into the
// encryption context as {"my-aad": base64url(aad)}; it holds only row IDs and column names, so
// that CloudTrail logs the context in plain text reveals no secret. A key ID is "awskms:"
// followed by the ARN of the key: New resolves an alias or a key ID to the ARN with DescribeKey,
// because an alias can be pointed at another key, and Decrypt names the key, so that AWS KMS
// refuses a ciphertext of another key (IncorrectKeyException). A rotation in AWS KMS keeps the
// ARN and the old key material, so it keeps the key ID and needs no rewrap. The client takes its
// credentials and region from the aws.Config that the caller loads with the SDK's default chain.
package awskms

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/smithy-go"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Kind is the kind of the key IDs of this KeyManager (kernel.KeyIDKind).
const Kind = "awskms"

const keyIDPrefix = Kind + ":"

// MaxKeyNameLen is the longest key name: a key ID is at most 255 bytes, and the prefix "awskms:"
// takes 7 of them.
const MaxKeyNameLen = 255 - len(keyIDPrefix)

// ContextKey is the key of the encryption context that holds the AAD, encoded with base64url
// without padding. An empty AAD gives no encryption context.
const ContextKey = "my-aad"

var (
	keyARN   = regexp.MustCompile(`^arn:aws[a-z-]*:kms:[a-z0-9-]+:[0-9]{12}:key/(?:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|mrk-[0-9a-f]{32})$`)
	aliasARN = regexp.MustCompile(`^arn:aws[a-z-]*:kms:[a-z0-9-]+:[0-9]{12}:alias/[A-Za-z0-9/_-]+$`)
	alias    = regexp.MustCompile(`^alias/[A-Za-z0-9/_-]+$`)
	keyUUID  = regexp.MustCompile(`^(?:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|mrk-[0-9a-f]{32})$`)
)

// CheckKeyName reports whether name names an AWS KMS key: its ARN, an alias ARN, an alias name
// (alias/...), or a key ID, short enough for a key ID. AWS managed keys (alias/aws/...) are
// refused: only the AWS service that owns one may use it.
func CheckKeyName(name string) error {
	if len(name) > MaxKeyNameLen {
		return fmt.Errorf("must be at most %d bytes, got %d", MaxKeyNameLen, len(name))
	}
	if !keyARN.MatchString(name) && !aliasARN.MatchString(name) && !alias.MatchString(name) && !keyUUID.MatchString(name) {
		return errors.New("must be the ARN, an alias (alias/...), or the key ID of an AWS KMS key")
	}
	if strings.HasPrefix(name, "alias/aws/") || strings.Contains(name, ":alias/aws/") {
		return errors.New("must not be an AWS managed key (alias/aws/...): create a customer managed key")
	}
	return nil
}

// KeyID returns the key ID of the key ARN.
func KeyID(keyARN string) string { return keyIDPrefix + keyARN }

// Options configure a KeyManager.
type Options struct {
	// Key names the key that wraps (kms_key), in a format that CheckKeyName accepts.
	Key string
	// PreviousKeys name keys that only unwrap (kms_key_previous).
	PreviousKeys []string
}

// KeyManager wraps with the key Key and unwraps with Key or any of PreviousKeys.
// It never prints a key: String and LogValue show only the current key ID.
type KeyManager struct {
	current string
	keys    map[string]key // key ID -> key
}

type key struct {
	arn    string
	client *kms.Client // of the key's region
}

var _ kernel.KeyManager = (*KeyManager)(nil)

// New returns a KeyManager on clients of cfg, with the credentials of cfg. Every key must pass
// CheckKeyName, and no key may be given twice. New calls DescribeKey on every key, in the region
// of its ARN or else of cfg, to resolve it to the ARN of a symmetric encryption key; a key that
// does not exist, is not enabled, or is not for symmetric encryption is an error.
func New(ctx context.Context, cfg aws.Config, opts Options) (*KeyManager, error) {
	km := &KeyManager{keys: make(map[string]key, 1+len(opts.PreviousKeys))}
	clients := map[string]*kms.Client{}
	for i, name := range append([]string{opts.Key}, opts.PreviousKeys...) {
		if err := CheckKeyName(name); err != nil {
			return nil, fmt.Errorf("awskms key manager: key #%d %q %w", i, name, err)
		}
		region := cfg.Region
		if a, err := arn.Parse(name); err == nil {
			region = a.Region
		}
		if region == "" {
			return nil, fmt.Errorf("awskms key manager: key %q: no region: give the key ARN, or set AWS_REGION", name)
		}
		client, ok := clients[region]
		if !ok {
			client = kms.NewFromConfig(cfg, func(o *kms.Options) { o.Region = region })
			clients[region] = client
		}
		resolved, err := describe(ctx, client, name)
		if err != nil {
			return nil, err
		}
		id := KeyID(resolved)
		if _, dup := km.keys[id]; dup {
			return nil, fmt.Errorf("awskms key manager: key %q is given more than once (it is %s)", name, resolved)
		}
		km.keys[id] = key{arn: resolved, client: client}
		if i == 0 {
			km.current = id
		}
	}
	return km, nil
}

// describe resolves name to the ARN of an enabled symmetric encryption key.
func describe(ctx context.Context, client *kms.Client, name string) (string, error) {
	out, err := client.DescribeKey(ctx, &kms.DescribeKeyInput{KeyId: aws.String(name)})
	if err != nil {
		return "", callError("describe", name, err)
	}
	m := out.KeyMetadata
	if m == nil || aws.ToString(m.Arn) == "" {
		return "", fmt.Errorf("awskms key manager: describe key %q: the response has no key ARN", name)
	}
	resolved := aws.ToString(m.Arn)
	if !keyARN.MatchString(resolved) {
		return "", fmt.Errorf("awskms key manager: describe key %q: unexpected key ARN %q", name, resolved)
	}
	if m.KeyUsage != types.KeyUsageTypeEncryptDecrypt || m.KeySpec != types.KeySpecSymmetricDefault {
		return "", fmt.Errorf("awskms key manager: key %q (%s) is not a symmetric encryption key (key spec %s, key usage %s): create one with the key spec SYMMETRIC_DEFAULT and the key usage ENCRYPT_DECRYPT", name, resolved, m.KeySpec, m.KeyUsage)
	}
	if m.KeyState != types.KeyStateEnabled {
		return "", fmt.Errorf("awskms key manager: key %q (%s) is %s, not Enabled", name, resolved, m.KeyState)
	}
	return resolved, nil
}

// CurrentKeyID returns the ID of the key that WrapKey uses.
func (k *KeyManager) CurrentKeyID() string { return k.current }

// encryptionContext returns the encryption context that carries aad.
func encryptionContext(aad []byte) map[string]string {
	if len(aad) == 0 {
		return nil
	}
	return map[string]string{ContextKey: base64.RawURLEncoding.EncodeToString(aad)}
}

// WrapKey encrypts dek with the current key, authenticating aad.
func (k *KeyManager) WrapKey(ctx context.Context, dek, aad []byte) (string, []byte, error) {
	cur := k.keys[k.current]
	out, err := cur.client.Encrypt(ctx, &kms.EncryptInput{
		KeyId:               aws.String(cur.arn),
		Plaintext:           dek,
		EncryptionContext:   encryptionContext(aad),
		EncryptionAlgorithm: types.EncryptionAlgorithmSpecSymmetricDefault,
	})
	if err != nil {
		return "", nil, callError("encrypt", k.current, err)
	}
	if len(out.CiphertextBlob) == 0 {
		return "", nil, fmt.Errorf("awskms key manager: encrypt with key %q: the response has no ciphertext", k.current)
	}
	return k.current, out.CiphertextBlob, nil
}

// UnwrapKey decrypts a key wrapped under the key that keyID names.
func (k *KeyManager) UnwrapKey(ctx context.Context, keyID string, wrapped, aad []byte) ([]byte, error) {
	key, ok := k.keys[keyID]
	if !ok {
		return nil, fmt.Errorf("awskms key manager: key %q is not configured: %w", keyID, kernel.ErrUnknownKey)
	}
	if len(wrapped) == 0 {
		return nil, fmt.Errorf("awskms key manager: key %q: empty ciphertext: %w", keyID, kernel.ErrDecrypt)
	}
	out, err := key.client.Decrypt(ctx, &kms.DecryptInput{
		KeyId:               aws.String(key.arn),
		CiphertextBlob:      wrapped,
		EncryptionContext:   encryptionContext(aad),
		EncryptionAlgorithm: types.EncryptionAlgorithmSpecSymmetricDefault,
	})
	if err != nil {
		return nil, callError("decrypt", keyID, err)
	}
	if got := aws.ToString(out.KeyId); got != key.arn {
		return nil, fmt.Errorf("awskms key manager: decrypt with key %q: decrypted with another key %q: %w", keyID, got, kernel.ErrDecrypt)
	}
	return out.Plaintext, nil
}

// callError maps an error of an AWS KMS call. A ciphertext or an encryption context that does
// not authenticate (InvalidCiphertextException), or a ciphertext of another key
// (IncorrectKeyException), wraps kernel.ErrDecrypt. A missing permission or key, or a key that
// is disabled, pending deletion, or not for symmetric encryption, is a mistake of the
// configuration or of the operation of the KMS and is returned with a hint. Throttling
// (ThrottlingException, which the SDK already retried), the internal errors of AWS KMS, and
// failures of the transport are transient, and the call may be repeated. The returned error
// keeps the SDK's error (errors.As with smithy.APIError).
func callError(op, keyID string, err error) error {
	var (
		invalid   *types.InvalidCiphertextException
		incorrect *types.IncorrectKeyException
		notFound  *types.NotFoundException
		disabled  *types.DisabledException
		state     *types.KMSInvalidStateException
		usage     *types.InvalidKeyUsageException
		api       smithy.APIError
	)
	switch {
	case op == "decrypt" && (errors.As(err, &invalid) || errors.As(err, &incorrect)):
		return fmt.Errorf("awskms key manager: %s with key %q: %w: %w", op, keyID, kernel.ErrDecrypt, err)
	case errors.As(err, &notFound):
		return fmt.Errorf("awskms key manager: %s with key %q: the key does not exist in this account and region: %w", op, keyID, err)
	case errors.As(err, &disabled):
		return fmt.Errorf("awskms key manager: %s with key %q: the key is disabled: %w", op, keyID, err)
	case errors.As(err, &state):
		return fmt.Errorf("awskms key manager: %s with key %q: the key is pending deletion or not usable: %w", op, keyID, err)
	case errors.As(err, &usage):
		return fmt.Errorf("awskms key manager: %s with key %q: the key is not for symmetric encryption: %w", op, keyID, err)
	case errors.As(err, &api) && isAccessError(api.ErrorCode()):
		return fmt.Errorf("awskms key manager: %s with key %q: the IAM identity lacks kms:Encrypt, kms:Decrypt, or kms:DescribeKey on the key, or its credentials are not valid: %w", op, keyID, err)
	}
	return fmt.Errorf("awskms key manager: %s with key %q: %w", op, keyID, err)
}

func isAccessError(code string) bool {
	switch code {
	case "AccessDeniedException", "UnrecognizedClientException", "InvalidSignatureException", "ExpiredTokenException", "IncompleteSignature", "MissingAuthenticationToken":
		return true
	}
	return false
}

// String describes the KeyManager without its keys.
func (k *KeyManager) String() string {
	return fmt.Sprintf("awskms.KeyManager{current: %s, keys: %d}", k.current, len(k.keys))
}

// LogValue keeps the KeyManager's state out of logs, like the other key managers (ADR-0019 §6).
func (k *KeyManager) LogValue() slog.Value {
	return slog.GroupValue(slog.String("current", k.current), slog.Int("keys", len(k.keys)))
}
