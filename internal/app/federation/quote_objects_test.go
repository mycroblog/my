// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
)

// fakeObjectQuotes serves the approvals in auths; the rest of NoteQuotes presents no quotes.
type fakeObjectQuotes struct {
	auths map[string]appcontent.QuoteAuthorization
}

func (f fakeObjectQuotes) FindQuote(context.Context, string) (appcontent.Quote, appcontent.QuoteState, error) {
	return appcontent.Quote{}, appcontent.QuoteState{}, appcontent.ErrNotFound
}

func (f fakeObjectQuotes) FindInteractionPolicy(context.Context, string) (appcontent.InteractionPolicy, error) {
	return nil, nil
}

func (f fakeObjectQuotes) FederatedQuotePage(_ context.Context, uri string) (string, bool, error) {
	return uri, true, nil
}

func (f fakeObjectQuotes) ReplyAuthorization(context.Context, string) (string, error) { return "", nil }

func (f fakeObjectQuotes) InteractionAuthorizationOf(context.Context, appcontent.InteractionKind, string, string) (string, error) {
	return "", nil
}

func (f fakeObjectQuotes) FindInteractionAuthorization(_ context.Context, kind appcontent.InteractionKind, id string) (appcontent.InteractionAuthorization, error) {
	a, ok := f.auths[id]
	if !ok || a.Kind != kind {
		return appcontent.InteractionAuthorization{}, appcontent.ErrNotFound
	}
	return a, nil
}

// TestObjectsQuoteAuthorization checks the GET of a QuoteAuthorization of this server
// (ADR-0069 §1): the approval by the quoted author, a Tombstone with Gone once revoked or once
// the quoted status is deleted, and ErrNotFound for one that does not exist or is of a
// local-visibility status.
func TestObjectsQuoteAuthorization(t *testing.T) {
	w := newObjWorld()
	quoted := w.post("0192f3b0-0000-7000-8000-0000000000a1", appcontent.VisibilityPublic)
	gone := w.post("0192f3b0-0000-7000-8000-0000000000a2", appcontent.VisibilityPublic)
	localOnly := w.post("0192f3b0-0000-7000-8000-0000000000a3", appcontent.VisibilityLocal)
	localGone := w.post("0192f3b0-0000-7000-8000-0000000000a4", appcontent.VisibilityLocal)
	w.deleted[localGone.ID] = true
	w.deleted[gone.ID] = true
	revokedAt := objNow
	const quoting = "https://remote.example/notes/1"
	auths := map[string]appcontent.QuoteAuthorization{
		"0192f3b0-0000-7000-8000-0000000000b1": {ID: "0192f3b0-0000-7000-8000-0000000000b1", Kind: appcontent.InteractionQuote, InteractingURI: quoting, TargetStatusID: quoted.ID, CreatedAt: objNow.Add(-time.Minute)},
		"0192f3b0-0000-7000-8000-0000000000b2": {ID: "0192f3b0-0000-7000-8000-0000000000b2", Kind: appcontent.InteractionQuote, InteractingURI: quoting, TargetStatusID: quoted.ID, RevokedAt: &revokedAt},
		"0192f3b0-0000-7000-8000-0000000000b3": {ID: "0192f3b0-0000-7000-8000-0000000000b3", Kind: appcontent.InteractionQuote, InteractingURI: quoting, TargetStatusID: gone.ID},
		"0192f3b0-0000-7000-8000-0000000000b4": {ID: "0192f3b0-0000-7000-8000-0000000000b4", Kind: appcontent.InteractionQuote, InteractingURI: quoting, TargetStatusID: localOnly.ID},
		"0192f3b0-0000-7000-8000-0000000000b5": {ID: "0192f3b0-0000-7000-8000-0000000000b5", Kind: appcontent.InteractionQuote, InteractingURI: quoting, TargetStatusID: localOnly.ID, RevokedAt: &revokedAt},
		"0192f3b0-0000-7000-8000-0000000000b6": {ID: "0192f3b0-0000-7000-8000-0000000000b6", Kind: appcontent.InteractionQuote, InteractingURI: quoting, TargetStatusID: localGone.ID},
	}
	o := appfederation.NewObjects(appfederation.ObjectsDeps{
		Statuses: w, Accounts: w, Graph: w, Timeline: w, Reader: w, Domains: w, Host: objHost, Quotes: fakeObjectQuotes{auths: auths},
	})

	obj, err := o.Authorization(t.Context(), appcontent.InteractionQuote, "0192f3b0-0000-7000-8000-0000000000b1")
	if err != nil || obj.Gone || !obj.Public {
		t.Fatalf("QuoteAuthorization = %+v, %v; want a public document", obj, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(obj.Document, &doc); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"id": "https://" + objHost + "/ap/quote_authorizations/0192f3b0-0000-7000-8000-0000000000b1", "type": "QuoteAuthorization",
		"attributedTo": "https://" + objHost + "/ap/actors/" + authorID, "interactingObject": quoting, "interactionTarget": quoted.URI,
	}
	for k, v := range want {
		if doc[k] != v {
			t.Errorf("%s = %v, want %v", k, doc[k], v)
		}
	}
	for _, id := range []string{"0192f3b0-0000-7000-8000-0000000000b2", "0192f3b0-0000-7000-8000-0000000000b3"} {
		obj, err := o.Authorization(t.Context(), appcontent.InteractionQuote, id)
		if err != nil || !obj.Gone {
			t.Errorf("QuoteAuthorization(%s) = %+v, %v; want Gone", id, obj, err)
		}
	}
	// An approval of a reply is served at its own IRI, as a ReplyAuthorization.
	auths["0192f3b0-0000-7000-8000-0000000000c1"] = appcontent.InteractionAuthorization{
		ID: "0192f3b0-0000-7000-8000-0000000000c1", Kind: appcontent.InteractionReply, InteractingURI: quoting, TargetStatusID: quoted.ID,
	}
	obj, err = o.Authorization(t.Context(), appcontent.InteractionReply, "0192f3b0-0000-7000-8000-0000000000c1")
	if err != nil || obj.Gone {
		t.Fatalf("Authorization(reply) = %+v, %v; want a document", obj, err)
	}
	if err := json.Unmarshal(obj.Document, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["type"] != "ReplyAuthorization" || doc["id"] != "https://"+objHost+"/ap/reply_authorizations/0192f3b0-0000-7000-8000-0000000000c1" {
		t.Errorf("Authorization(reply) = %v, want a ReplyAuthorization at its IRI", doc)
	}
	if _, err := o.Authorization(t.Context(), appcontent.InteractionAnnounce, "0192f3b0-0000-7000-8000-0000000000c1"); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("Authorization of another kind = %v, want ErrNotFound", err)
	}
	for _, id := range []string{
		"0192f3b0-0000-7000-8000-0000000000b9", // missing
		"0192f3b0-0000-7000-8000-0000000000b4", // of a local-visibility post
		"0192f3b0-0000-7000-8000-0000000000b5", // revoked, of a local-visibility post
		"0192f3b0-0000-7000-8000-0000000000b6", // of a deleted local-visibility post
	} {
		if _, err := o.Authorization(t.Context(), appcontent.InteractionQuote, id); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("QuoteAuthorization(%s) = %v, want ErrNotFound", id, err)
		}
	}
}
