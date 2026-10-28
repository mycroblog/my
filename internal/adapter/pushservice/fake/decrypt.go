// SPDX-License-Identifier: AGPL-3.0-only

package fake

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

// This file reads the aes128gcm content coding (RFC 8188) of a Web Push message with the keys of
// RFC 8291 §3. It is written apart from internal/adapter/webpush, which encrypts, so that a mistake
// of one side does not hide in the other: the tests check both against RFC 8291 Appendix A.

const (
	// saltBytes, rsBytes, and idLenBytes are the fields of the coding's header (RFC 8188 §2.1).
	saltBytes  = 16
	rsBytes    = 4
	idLenBytes = 1
	// pointBytes is an uncompressed P-256 point, the key ID that RFC 8291 §4 requires.
	pointBytes = 65
	// tagBytes is the AES-GCM tag of a record.
	tagBytes = 16
	// minRecordSize is the smallest record size RFC 8188 §2.1 allows.
	minRecordSize = 18
)

// Decrypt returns the plaintext of body, an aes128gcm message for the user agent whose private key
// is ua and whose authentication secret is auth. RFC 8291 §4 requires a single record whose
// key ID is the application server's ephemeral public key.
func Decrypt(body []byte, ua *ecdh.PrivateKey, auth []byte) ([]byte, error) {
	if len(body) < saltBytes+rsBytes+idLenBytes {
		return nil, errors.New("the body is shorter than the aes128gcm header")
	}
	salt := body[:saltBytes]
	rs := binary.BigEndian.Uint32(body[saltBytes : saltBytes+rsBytes])
	idLen := int(body[saltBytes+rsBytes])
	rest := body[saltBytes+rsBytes+idLenBytes:]
	if rs < minRecordSize {
		return nil, fmt.Errorf("the record size %d is below %d", rs, minRecordSize)
	}
	if idLen != pointBytes || len(rest) < idLen {
		return nil, fmt.Errorf("the key ID is %d bytes; Web Push needs the %d-byte public key of the application server", idLen, pointBytes)
	}
	asPublic, err := ecdh.P256().NewPublicKey(rest[:idLen])
	if err != nil {
		return nil, fmt.Errorf("the key ID is not a P-256 public key: %w", err)
	}
	record := rest[idLen:]
	if len(record) < tagBytes+1 {
		return nil, errors.New("the record is shorter than its tag and delimiter")
	}
	if uint64(len(record)) > uint64(rs) {
		return nil, fmt.Errorf("the body has more than one record of %d bytes; Web Push sends one", rs)
	}

	secret, err := ua.ECDH(asPublic)
	if err != nil {
		return nil, fmt.Errorf("ECDH: %w", err)
	}
	// RFC 8291 §3.4: the input keying material mixes the ECDH secret and the authentication
	// secret, bound to both public keys.
	keyInfo := append(append([]byte("WebPush: info\x00"), ua.PublicKey().Bytes()...), asPublic.Bytes()...)
	prkKey, err := hkdf.Extract(sha256.New, secret, auth)
	if err != nil {
		return nil, err
	}
	ikm, err := hkdf.Expand(sha256.New, prkKey, string(keyInfo), 32)
	if err != nil {
		return nil, err
	}
	// RFC 8188 §2.2 and §2.3: the content-encryption key and the nonce.
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// The first record's sequence number is 0, so its nonce is the derived nonce as is.
	padded, err := gcm.Open(nil, nonce, record, nil)
	if err != nil {
		return nil, errors.New("the record does not decrypt with the subscription's keys")
	}
	// RFC 8188 §2: the last record ends with the delimiter 0x02 followed by zero or more 0x00.
	end := bytes.TrimRight(padded, "\x00")
	if len(end) == 0 || end[len(end)-1] != 0x02 {
		return nil, errors.New("the record does not end with the delimiter of the last record")
	}
	return end[:len(end)-1], nil
}
