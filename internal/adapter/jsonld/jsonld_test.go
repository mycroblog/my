// SPDX-License-Identifier: AGPL-3.0-only

package jsonld_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/jsonld"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestCanonicalizerContract compares the canonicalization and the linked data signature with
// jsonld.js, the processor of Misskey, on the bundled contexts.
func TestCanonicalizerContract(t *testing.T) {
	testkit.CanonicalizerContract(t, func(t *testing.T) appfederation.Canonicalizer {
		c, err := jsonld.New()
		if err != nil {
			t.Fatalf("New = %v", err)
		}
		return c
	})
}

// TestCollectionContextDefinesItemTerms canonicalizes the featured collection of a quoting Note, as
// a JSON-LD consumer reads it: the terms of its embedded items must survive (federation.md §10).
func TestCollectionContextDefinesItemTerms(t *testing.T) {
	c, err := jsonld.New()
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	b, err := os.ReadFile("../../interface/activitypub/testdata/golden/collection-featured-quote.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	quads, err := c.Canonicalize(doc)
	if err != nil {
		t.Fatalf("Canonicalize = %v", err)
	}
	for _, term := range []string{
		"https://www.w3.org/ns/activitystreams#sensitive",
		"http://joinmastodon.org/ns#blurhash",
		"http://joinmastodon.org/ns#focalPoint",
		"https://w3id.org/fep/044f#quote",
		"https://w3id.org/fep/044f#quoteAuthorization",
		"https://gotosocial.org/ns#interactionPolicy",
	} {
		if !strings.Contains(quads, "<"+term+">") {
			t.Errorf("the canonical form drops %s:\n%s", term, quads)
		}
	}
}
