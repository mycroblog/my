// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/domain/federation"
)

// Linked data signatures (RsaSignature2017) of public posts (ADR-0006, federation.md §7.3): a
// relay forwards the Create, Update, and Delete of a public post as they are, signed by the
// relay, and Mastodon and Misskey take such a forwarded activity only when the document carries
// the author's linked data signature. The signature is made only for this use, never checked on
// what this server receives (a relayed activity is fetched again from its origin, ADR-0006).

// Canonicalizer canonicalizes JSON-LD documents (URDNA2015, as N-Quads), resolving their
// contexts only from the documents bundled with the server, never from the network. The
// implementation is internal/adapter/jsonld; it keeps the JSON-LD library out of this package
// and passes testkit.CanonicalizerContract.
type Canonicalizer interface {
	Canonicalize(document map[string]any) (string, error)
}

// LinkedDataSignatureType is the type of the linked data signatures this server makes.
const LinkedDataSignatureType = "RsaSignature2017"

// identityContext is the context of the signature options, as Mastodon and Misskey hash them.
const identityContext = "https://w3id.org/identity/v1"

// ErrLinkedDataSignature reports a linked data signature that cannot be made or does not verify.
var ErrLinkedDataSignature = errors.New("federation: linked data signature")

// SignLinkedData returns document with the RsaSignature2017 signature of key, created at created,
// in its signature property, as Mastodon's LinkedDataSignature#sign! makes it: the SHA-256 (in
// hex) of the canonical signature options (type, id, and signatureValue left out, with the
// identity context) followed by that of the canonical document (signature left out) are signed
// with RSASSA-PKCS1-v1_5 and SHA-256. The signature is deterministic, so every delivery of one
// activity sends the same bytes.
func SignLinkedData(document []byte, c Canonicalizer, key SigningKey, created time.Time) ([]byte, error) {
	doc, err := decodeJSONObject(document)
	if err != nil {
		return nil, err
	}
	delete(doc, "signature")
	options := map[string]any{
		"type":    LinkedDataSignatureType,
		"creator": key.KeyID,
		"created": federation.FormatTime(created),
	}
	digest, err := linkedDataDigest(doc, options, c)
	if err != nil {
		return nil, err
	}
	if _, ok := key.PrivateKey.Public().(*rsa.PublicKey); !ok {
		return nil, fmt.Errorf("%w: the key %s is not an RSA key", ErrLinkedDataSignature, key.KeyID)
	}
	sig, err := key.PrivateKey.Sign(rand.Reader, digest, crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLinkedDataSignature, err)
	}
	options["signatureValue"] = base64.StdEncoding.EncodeToString(sig)
	doc["signature"] = options
	return encodeJSON(doc)
}

// VerifyLinkedData checks the RsaSignature2017 signature of document with the public key pub, as
// Mastodon's LinkedDataSignature#verify_actor! does. It is for the tests of SignLinkedData: this
// server never trusts a received document for its linked data signature.
func VerifyLinkedData(document []byte, c Canonicalizer, pub *rsa.PublicKey) error {
	doc, err := decodeJSONObject(document)
	if err != nil {
		return err
	}
	sig, ok := doc["signature"].(map[string]any)
	if !ok || sig["type"] != LinkedDataSignatureType {
		return fmt.Errorf("%w: no %s", ErrLinkedDataSignature, LinkedDataSignatureType)
	}
	value, _ := sig["signatureValue"].(string)
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return fmt.Errorf("%w: signatureValue: %w", ErrLinkedDataSignature, err)
	}
	options := map[string]any{}
	for k, v := range sig {
		options[k] = v
	}
	delete(doc, "signature")
	digest, err := linkedDataDigest(doc, options, c)
	if err != nil {
		return err
	}
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest, raw); err != nil {
		return fmt.Errorf("%w: %w", ErrLinkedDataSignature, err)
	}
	return nil
}

// linkedDataDigest returns the SHA-256 of what an RsaSignature2017 signs.
func linkedDataDigest(doc, options map[string]any, c Canonicalizer) ([]byte, error) {
	opts := map[string]any{"@context": identityContext}
	for k, v := range options {
		switch k {
		case "type", "id", "signatureValue":
			continue
		}
		opts[k] = v
	}
	optionsHash, err := canonicalHash(opts, c)
	if err != nil {
		return nil, err
	}
	documentHash, err := canonicalHash(doc, c)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(optionsHash + documentHash))
	return sum[:], nil
}

// canonicalHash returns the SHA-256, in hex, of the canonical N-Quads of doc.
func canonicalHash(doc map[string]any, c Canonicalizer) (string, error) {
	nquads, err := c.Canonicalize(doc)
	if err != nil {
		return "", fmt.Errorf("%w: canonicalize: %w", ErrLinkedDataSignature, err)
	}
	if nquads == "" {
		return "", fmt.Errorf("%w: the document has no linked data", ErrLinkedDataSignature)
	}
	sum := sha256.Sum256([]byte(nquads))
	return hex.EncodeToString(sum[:]), nil
}

// decodeJSONObject reads a JSON object with its numbers as they are written.
func decodeJSONObject(b []byte) (map[string]any, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var doc map[string]any
	if err := d.Decode(&doc); err != nil || doc == nil {
		return nil, fmt.Errorf("%w: the document is not a JSON object", ErrLinkedDataSignature)
	}
	return doc, nil
}

// encodeJSON writes v without escaping HTML characters, as the documents are written.
func encodeJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// addressedTo returns the to and cc of a document.
func addressedTo(document []byte) federation.Audience {
	var head struct {
		To json.RawMessage `json:"to"`
		Cc json.RawMessage `json:"cc"`
	}
	if json.Unmarshal(document, &head) != nil {
		return federation.Audience{}
	}
	return federation.Audience{To: iris(head.To), Cc: iris(head.Cc)}
}

// iris reads an IRI or an array of IRIs.
func iris(raw json.RawMessage) []federation.IRI {
	var list []federation.IRI
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var one federation.IRI
	if json.Unmarshal(raw, &one) == nil && one != "" {
		return []federation.IRI{one}
	}
	return nil
}
