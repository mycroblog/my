// SPDX-License-Identifier: AGPL-3.0-only

package activitypub_test

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/activitypub"
	"github.com/mycroblog/my/pkg/testkit"
)

const (
	noteStatusID = "0192f3b0-2c3d-7e4f-8a00-1122334455aa"
	bobID        = "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e80" // a remote account on remote.example
)

// noteWorld is the content, account, and graph modules behind appfederation.Objects with one
// public status of alice: a content warning, a mention of a remote account, a hashtag, and an
// image, the parts that the peers' create-note.json documents have.
type noteWorld struct {
	actors map[string]appaccount.Actor
	status appcontent.Status
	rev    appcontent.Revision
	parts  appcontent.RevisionParts
}

func newNoteWorld() *noteWorld {
	alice := appaccount.Actor{
		Account: account.Account{ID: aliceID, URI: account.ActorURI(host, aliceID), ActorType: account.ActorPerson},
		Local:   &account.LocalAccount{AccountID: aliceID, Username: "alice", UsernameNormalized: "alice"},
		Handle:  &account.Handle{AccountID: aliceID, Username: "alice", UsernameNormalized: "alice"},
	}
	bob := appaccount.Actor{
		Account: account.Account{ID: bobID, URI: "https://remote.example/users/bob", Domain: "remote.example", ActorType: account.ActorPerson},
		Handle:  &account.Handle{AccountID: bobID, Username: "bob", UsernameNormalized: "bob", Domain: "remote.example"},
	}
	published := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	body := `<p><span class="h-card"><a href="https://remote.example/@bob" class="u-url mention">@<span>bob</span></a></span> hello <a href="https://` + host + `/tags/fediverse" class="mention hashtag" rel="tag">#<span>FediVerse</span></a></p>`
	return &noteWorld{
		actors: map[string]appaccount.Actor{aliceID: alice, bobID: bob},
		status: appcontent.Status{
			ID: noteStatusID, URI: appcontent.StatusURI(host, noteStatusID),
			AuthorAccountID: aliceID, IsLocal: true, Visibility: appcontent.VisibilityPublic, PublishedAt: published,
		},
		rev: appcontent.Revision{
			ID: testkit.NamedID("note-revision"), StatusID: noteStatusID, Seq: 1, SpoilerText: "a content warning", Language: "en", RevisedAt: published,
			Bodies: []content.Body{{Language: "en", ContentHTML: body}}, MentionedAccountIDs: []string{bobID},
		},
		parts: appcontent.RevisionParts{
			Mentions: []appaccount.Actor{bob},
			Tags:     []content.Tag{{ID: testkit.NamedID("tag-fediverse"), Name: "FediVerse", NameNormalized: "fediverse"}},
			Attachments: []appcontent.MediaView{{
				Media: appcontent.Media{
					Result: &content.MediaProcessingResult{
						Outcome: content.MediaReady, MediaType: content.MediaTypeImage, Width: 1280, Height: 960, Blurhash: "UFHV6nWB2yk8pyo0adR*.7kCMdnjt7jtxuWB",
					},
					Focus: &content.MediaFocus{X: 0.5, Y: -0.25},
				},
				Description: "A cat on a sofa",
				Image: appcontent.Image{Still: appcontent.ImageVariants{
					Small:  appcontent.ImageVariant{URL: "https://files." + host + "/media/r/small.webp", ContentType: "image/webp", Width: 640, Height: 480},
					Medium: appcontent.ImageVariant{URL: "https://files." + host + "/media/r/medium.webp", ContentType: "image/webp", Width: 1280, Height: 960},
					Large:  appcontent.ImageVariant{URL: "https://files." + host + "/media/r/large.jpg", ContentType: "image/jpeg", Width: 1280, Height: 960},
				}},
			}},
		},
	}
}

func (w *noteWorld) ObjectStatus(_ context.Context, id string) (appcontent.Status, appcontent.Revision, bool, error) {
	if id != w.status.ID {
		return appcontent.Status{}, appcontent.Revision{}, false, appcontent.ErrNotFound
	}
	return w.status, w.rev, false, nil
}

func (w *noteWorld) AuthorRestrictions(context.Context, string) (appcontent.AuthorRestrictions, error) {
	return appcontent.AuthorRestrictions{}, nil
}

func (w *noteWorld) PostCount(context.Context, string) (int64, error) { return 1, nil }

func (w *noteWorld) FindByID(_ context.Context, id string) (appaccount.Actor, error) {
	a, ok := w.actors[id]
	if !ok {
		return appaccount.Actor{}, appaccount.ErrNotFound
	}
	return a, nil
}

func (w *noteWorld) FindByURI(context.Context, string) (appaccount.Actor, error) {
	return appaccount.Actor{}, appaccount.ErrNotFound
}

func (w *noteWorld) AnyOnDomain(context.Context, string, []string) (bool, error) { return false, nil }

func (w *noteWorld) ActiveFollowerIDs(context.Context, string, string, int) ([]string, string, error) {
	return nil, "", nil
}

func (w *noteWorld) Relationship(_ context.Context, accountID, targetID string) (appgraph.Relationship, error) {
	return appgraph.Relationship{AccountID: accountID, TargetAccountID: targetID}, nil
}

func (w *noteWorld) Profile(context.Context, appcontent.Viewer, string, apptimeline.ProfileFilter, apptimeline.Query) (apptimeline.Page, error) {
	return apptimeline.Page{Entries: []apptimeline.Entry{{StatusID: w.status.ID}}}, nil
}

// view is the status as the reader presents it to the outbox and featured collections.
func (w *noteWorld) view() appcontent.StatusView {
	return appcontent.StatusView{
		Status: w.status, Revision: w.rev, Author: w.actors[aliceID],
		Attachments: w.parts.Attachments, Mentions: w.parts.Mentions, Tags: w.parts.Tags,
	}
}

func (w *noteWorld) Views(context.Context, appcontent.Viewer, []string) ([]appcontent.StatusView, error) {
	return []appcontent.StatusView{w.view()}, nil
}

func (w *noteWorld) Pinned(context.Context, appcontent.Viewer, string) ([]appcontent.StatusView, error) {
	return []appcontent.StatusView{w.view()}, nil
}

func (w *noteWorld) EmojiTags(context.Context, []string) ([]appcontent.EmojiTag, error) {
	return nil, nil
}

func (w *noteWorld) Parts(context.Context, appcontent.Revision) (appcontent.RevisionParts, error) {
	return w.parts, nil
}

// noteRequired lists the properties of a Create and its Note that each implementation reads
// when it imports a remote status (Mastodon: ActivityPub::Activity::Create and
// ActivityPub::Parser::StatusParser / MediaAttachmentParser; GoToSocial: internal/ap/extract.go;
// Misskey: ApNoteService.createNote).
var noteRequired = map[string][]string{
	"mastodon": {
		"@context", "id", "type", "actor", "published", "to", "cc",
		"object.id", "object.type", "object.attributedTo", "object.url", "object.published", "object.to", "object.cc",
		"object.summary", "object.sensitive", "object.content", "object.contentMap",
		"object.attachment[].type", "object.attachment[].mediaType", "object.attachment[].url", "object.attachment[].name",
		"object.attachment[].blurhash", "object.attachment[].focalPoint", "object.attachment[].width", "object.attachment[].height",
		"object.tag[].type", "object.tag[].href", "object.tag[].name",
	},
	"gotosocial": {
		"@context", "id", "type", "actor", "to", "cc",
		"object.id", "object.type", "object.attributedTo", "object.url", "object.published", "object.to", "object.cc",
		"object.summary", "object.sensitive", "object.content", "object.contentMap",
		"object.attachment[].mediaType", "object.attachment[].url", "object.attachment[].name",
		"object.attachment[].blurhash", "object.attachment[].focalPoint",
		"object.tag[].type", "object.tag[].href", "object.tag[].name",
	},
	"misskey": {
		"@context", "id", "type", "actor", "published", "to", "cc",
		"object.id", "object.type", "object.attributedTo", "object.published", "object.to", "object.cc",
		"object.summary", "object.sensitive", "object.content",
		"object.attachment[].type", "object.attachment[].mediaType", "object.attachment[].url", "object.attachment[].name",
		"object.tag[].type", "object.tag[].href", "object.tag[].name",
	},
}

// TestCreateNoteMatchesPeers checks the Create of a local status, as the object GET serves it
// with the Note builder that the fan-out delivers too (federation.md §7.4, §10), against the
// Create that each implementation publishes.
func TestCreateNoteMatchesPeers(t *testing.T) {
	s := newNoteServer(t, newNoteWorld(), nil)

	rec := s.get(t, "/ap/objects/"+noteStatusID+"/activity", acceptMastodon)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET the Create = %d %s", rec.Code, rec.Body)
	}
	ours := decode(t, rec.Body.Bytes())
	for _, peer := range peers {
		checkShape(t, peer, ours, readPeer(t, peer, "create-note.json"), noteRequired[peer])
	}
	note := ours["object"].(map[string]any)
	tags := note["tag"].([]any)
	if m := tags[0].(map[string]any); m["type"] != "Mention" || m["name"] != "@bob@remote.example" || m["href"] != "https://remote.example/users/bob" {
		t.Errorf("mention = %v", m)
	}
	if h := tags[1].(map[string]any); h["type"] != "Hashtag" || h["name"] != "#FediVerse" || h["href"] != "https://"+host+"/tags/fediverse" {
		t.Errorf("hashtag = %v", h)
	}
}

// newNoteServer returns the handler over appfederation.Objects on w and quotes.
func newNoteServer(t *testing.T, w *noteWorld, quotes appfederation.ObjectQuotes) *server {
	t.Helper()
	objects := appfederation.NewObjects(appfederation.ObjectsDeps{
		Statuses: w, Accounts: w, Graph: w, Timeline: w, Reader: w, Quotes: quotes, Host: host, AccountDomain: accountDomain,
	})
	s := &server{mux: http.NewServeMux(), accounts: newAccounts(t), objects: newObjects(), graph: &fakeGraph{}}
	activitypub.New(activitypub.Options{
		Host: host, AccountDomain: accountDomain, SoftwareVersion: "0.1.0", SourceURL: "https://github.com/mycroblog/my",
		Accounts: s.accounts, Objects: objects, Graph: s.graph, Problem: problem,
	}).Register(s.mux)
	return s
}

// noteQuote is the approved quote of a remote status that the status of a noteWorld makes, with
// the policy that followers may quote it.
type noteQuote struct{}

const (
	noteQuotedURI  = "https://remote.example/users/bob/statuses/1"
	noteQuotedPage = "https://remote.example/@bob/1"
	noteQuoteAuth  = "https://remote.example/users/bob/quote_authorizations/1"
)

func (noteQuote) FindQuote(_ context.Context, statusID string) (appcontent.Quote, appcontent.QuoteState, error) {
	return appcontent.Quote{StatusID: statusID, QuotedURI: noteQuotedURI},
		appcontent.QuoteState{StatusID: statusID, Approval: appcontent.QuoteApprovalAccepted, AuthorizationURI: noteQuoteAuth}, nil
}

func (noteQuote) FindQuotePolicy(_ context.Context, statusID string) (appcontent.StatusQuotePolicy, error) {
	return appcontent.StatusQuotePolicy{StatusID: statusID, Policy: appcontent.QuotePolicyFollowers}, nil
}

func (noteQuote) FederatedQuotePage(context.Context, string) (string, bool, error) {
	return noteQuotedPage, true, nil
}

func (noteQuote) FindQuoteAuthorization(context.Context, string) (appcontent.QuoteAuthorization, error) {
	return appcontent.QuoteAuthorization{}, appcontent.ErrNotFound
}

// TestCollectionsServeTheQuotingNote checks a quoting status in the outbox and featured: the Note
// is the one its IRI serves (federation.md §7.4), and the collection's context defines its terms.
func TestCollectionsServeTheQuotingNote(t *testing.T) {
	s := newNoteServer(t, newNoteWorld(), noteQuote{})
	rec := s.get(t, "/ap/objects/"+noteStatusID, acceptMastodon)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET the Note = %d %s", rec.Code, rec.Body)
	}
	own := decode(t, rec.Body.Bytes())
	delete(own, "@context")
	if own["quote"] != noteQuotedURI || own["quoteAuthorization"] != noteQuoteAuth || own["interactionPolicy"] == nil {
		t.Fatalf("Note = %v", own)
	}

	rec = s.get(t, "/ap/actors/"+aliceID+"/featured", acceptMastodon)
	checkGolden(t, "collection-featured-quote.json", rec.Body.Bytes())
	items := decode(t, rec.Body.Bytes())["orderedItems"].([]any)
	if len(items) != 1 || !reflect.DeepEqual(items[0], own) {
		t.Errorf("featured Note = %v, want %v", items, own)
	}

	rec = s.get(t, "/ap/actors/"+aliceID+"/outbox?page=true", acceptMastodon)
	checkGolden(t, "collection-outbox-quote.json", rec.Body.Bytes())
	items = decode(t, rec.Body.Bytes())["orderedItems"].([]any)
	if len(items) != 1 || !reflect.DeepEqual(items[0].(map[string]any)["object"], own) {
		t.Errorf("outbox items = %v, want the Create of %v", items, own)
	}
}
