// SPDX-License-Identifier: AGPL-3.0-only

// Package jsonld canonicalizes JSON-LD documents for the linked data signatures of public posts
// (ADR-0006): URDNA2015 to N-Quads with github.com/piprate/json-gold, whose contexts resolve only
// from the documents bundled here (contexts/), never from the network. It implements
// federation.Canonicalizer and keeps the JSON-LD library out of the application layer.
package jsonld

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/piprate/json-gold/ld"

	appfederation "github.com/mycroblog/my/internal/app/federation"
)

// The bundled contexts, by their URLs: ActivityStreams 2.0, the security vocabulary, and the
// identity vocabulary of the signature options (contexts/README.md).
//
//go:embed contexts/*.jsonld
var bundled embed.FS

var contextFiles = map[string]string{
	"https://www.w3.org/ns/activitystreams": "contexts/as.jsonld",
	"https://w3id.org/security/v1":          "contexts/security-v1.jsonld",
	"https://w3id.org/identity/v1":          "contexts/identity-v1.jsonld",
}

// Canonicalizer implements federation.Canonicalizer with json-gold.
type Canonicalizer struct {
	loader loader
}

var _ appfederation.Canonicalizer = (*Canonicalizer)(nil)

// New returns the canonicalizer on the bundled contexts. It fails when a bundled context cannot
// be read, which only a broken build does.
func New() (*Canonicalizer, error) {
	l := loader{docs: map[string][]byte{}}
	for url, file := range contextFiles {
		b, err := bundled.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("jsonld: the bundled context %s: %w", url, err)
		}
		if !json.Valid(b) {
			return nil, fmt.Errorf("jsonld: the bundled context %s is not JSON", url)
		}
		l.docs[url] = b
	}
	return &Canonicalizer{loader: l}, nil
}

// Canonicalize implements federation.Canonicalizer: the canonical N-Quads of document by
// URDNA2015, in JSON-LD 1.1 processing mode. Numbers are read as JSON numbers (float64), as the
// JSON-LD processors of Mastodon and Misskey read them.
func (c *Canonicalizer) Canonicalize(document map[string]any) (string, error) {
	b, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	var input any
	if err := json.Unmarshal(b, &input); err != nil {
		return "", err
	}
	opts := ld.NewJsonLdOptions("")
	opts.Algorithm = ld.AlgorithmURDNA2015
	opts.Format = "application/n-quads"
	opts.DocumentLoader = c.loader
	out, err := ld.NewJsonLdProcessor().Normalize(input, opts)
	if err != nil {
		return "", fmt.Errorf("jsonld: canonicalize: %w", err)
	}
	s, ok := out.(string)
	if !ok {
		return "", fmt.Errorf("jsonld: canonicalize returned %T", out)
	}
	return s, nil
}

// loader is the json-gold document loader of the bundled contexts. Any other URL fails: the
// documents are this server's own, and loading a context from the network would let a remote
// server change what a signature covers.
type loader struct {
	docs map[string][]byte
}

// LoadDocument implements ld.DocumentLoader. Each load reads the context anew, so that
// concurrent canonicalizations share nothing that the processor may change.
func (l loader) LoadDocument(u string) (*ld.RemoteDocument, error) {
	b, ok := l.docs[strings.TrimSuffix(u, "#")]
	if !ok {
		return nil, ld.NewJsonLdError(ld.LoadingDocumentFailed, fmt.Sprintf("the context %q is not bundled", u))
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, ld.NewJsonLdError(ld.LoadingDocumentFailed, err)
	}
	return &ld.RemoteDocument{DocumentURL: u, Document: doc}, nil
}
