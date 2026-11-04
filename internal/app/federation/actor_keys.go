// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/federation"
)

// maxKeyDocuments bounds the separate key documents fetched for one actor: keys that the actor's
// publicKey names by id only (ADR-0006).
const maxKeyDocuments = 2

// keyDocumentTypes are the types of a document that is a key rather than an actor.
var keyDocumentTypes = []string{"Key", "CryptographicKey", "Multikey"}

// rawKey is the part of a key object that the normalized actor does not carry: the key
// documents and the FEP-521a Multikey entries.
type rawKey struct {
	ID                 string          `json:"id"`
	Type               json.RawMessage `json:"type"`
	Owner              json.RawMessage `json:"owner"`
	Controller         json.RawMessage `json:"controller"`
	PublicKeyPem       string          `json:"publicKeyPem"`
	PublicKeyMultibase string          `json:"publicKeyMultibase"`
}

// rawActorKeys is the part of an actor document that declares keys.
type rawActorKeys struct {
	PublicKey       json.RawMessage `json:"publicKey"`
	AssertionMethod json.RawMessage `json:"assertionMethod"`
}

// objects reads a property that is one value or an array of them, each a string or an object.
func objects(raw json.RawMessage) (ids []string, objs []rawKey) {
	var many []json.RawMessage
	if err := json.Unmarshal(raw, &many); err != nil {
		many = []json.RawMessage{raw}
	}
	for _, v := range many {
		var s string
		if json.Unmarshal(v, &s) == nil {
			ids = append(ids, s)
			continue
		}
		var k rawKey
		if json.Unmarshal(v, &k) == nil {
			objs = append(objs, k)
		}
	}
	return ids, objs
}

// firstIRI reads a property that is an IRI or an object with an id, canonicalized.
func firstIRI(raw json.RawMessage) federation.IRI {
	ids, objs := objects(raw)
	for _, k := range objs {
		ids = append(ids, k.ID)
	}
	for _, id := range ids {
		if iri, err := CanonicalIRI(id); err == nil {
			return iri
		}
	}
	return ""
}

func (k rawKey) types() []string {
	var many []string
	if json.Unmarshal(k.Type, &many) == nil {
		return many
	}
	var one string
	if json.Unmarshal(k.Type, &one) == nil {
		return []string{one}
	}
	return nil
}

func (k rawKey) isType(names ...string) bool {
	for _, t := range k.types() {
		for _, n := range names {
			if t == n {
				return true
			}
		}
	}
	return false
}

// keyOwner returns the owner (or controller) of a key document, or "" when raw is not one.
func keyOwner(raw []byte) federation.IRI {
	var k rawKey
	if json.Unmarshal(raw, &k) != nil || !k.isType(keyDocumentTypes...) {
		return ""
	}
	if owner := firstIRI(k.Owner); owner != "" {
		return owner
	}
	return firstIRI(k.Controller)
}

// actorKeys returns the keys an actor document declares for the actor (ADR-0006): the
// publicKey entries with a PEM, the key documents that publicKey names by id only (fetched, at
// most maxKeyDocuments), and the Ed25519 keys of FEP-521a assertionMethod Multikey entries. A
// key must have the actor's origin and be owned by the actor.
func (r *RemoteActors) actorKeys(ctx context.Context, a *federation.Actor, raw []byte) []appaccount.RemoteKeyInput {
	var out []appaccount.RemoteKeyInput
	seen := map[federation.IRI]bool{}
	add := func(id, owner federation.IRI, pub crypto.PublicKey) {
		if id == "" || pub == nil || seen[id] || !federation.SameOrigin(id, a.ID) || (owner != "" && owner != a.ID) {
			return
		}
		seen[id] = true
		out = append(out, appaccount.RemoteKeyInput{KeyID: string(id), PublicKey: pub})
	}
	for _, k := range a.PublicKeys {
		pub, err := parsePublicKeyPEM(k.PEM)
		if err == nil {
			add(k.ID, k.Owner, pub)
		}
	}
	var doc rawActorKeys
	if json.Unmarshal(raw, &doc) != nil {
		return out
	}
	ids, _ := objects(doc.PublicKey)
	fetched := 0
	for _, id := range ids {
		iri, err := CanonicalIRI(id)
		if err != nil || seen[iri] || !federation.SameOrigin(iri, a.ID) || fetched >= maxKeyDocuments {
			continue
		}
		fetched++
		got, err := r.d.Fetcher.FetchActor(ctx, id)
		if err != nil || got.Document.ID() != iri {
			continue
		}
		var k rawKey
		if json.Unmarshal(got.Raw, &k) != nil || !k.isType(keyDocumentTypes...) {
			continue
		}
		if pub, err := parsePublicKeyPEM(k.PublicKeyPem); err == nil {
			add(iri, keyOwner(got.Raw), pub)
		}
	}
	_, methods := objects(doc.AssertionMethod)
	for _, m := range methods {
		if !m.isType(federation.MultikeyType) {
			continue
		}
		pub, err := federation.ParseEd25519Multikey(m.PublicKeyMultibase)
		if err != nil {
			continue
		}
		id, err := CanonicalIRI(m.ID)
		if err != nil {
			continue
		}
		add(id, firstIRI(m.Controller), pub)
	}
	return out
}

// parsePublicKeyPEM parses a PKIX "PUBLIC KEY" block, or a PKCS #1 "RSA PUBLIC KEY" block that
// some implementations publish, into an RSA or Ed25519 key.
func parsePublicKeyPEM(s string) (crypto.PublicKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(s)))
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	var pub any
	var err error
	switch block.Type {
	case "PUBLIC KEY":
		pub, err = x509.ParsePKIXPublicKey(block.Bytes)
	case "RSA PUBLIC KEY":
		pub, err = x509.ParsePKCS1PublicKey(block.Bytes)
	default:
		return nil, fmt.Errorf("PEM block %q", block.Type)
	}
	if err != nil {
		return nil, err
	}
	switch k := pub.(type) {
	case *rsa.PublicKey, ed25519.PublicKey:
		return k, nil
	}
	return nil, fmt.Errorf("unsupported public key %T", pub)
}
