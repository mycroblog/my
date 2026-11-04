// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
)

const (
	objHost      = "social.example"
	authorID     = "0192f3b0-0000-7000-8000-00000000000a"
	followerID   = "0192f3b0-0000-7000-8000-00000000000b" // on follower.example
	mentionedID  = "0192f3b0-0000-7000-8000-00000000000c" // on mentioned.example
	blockedID    = "0192f3b0-0000-7000-8000-00000000000d" // on follower.example, blocked by the author
	remoteAuthor = "0192f3b0-0000-7000-8000-00000000000e" // on follower.example
)

var objNow = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

// objWorld is an in-memory content, account, and graph module for Objects.
type objWorld struct {
	statuses map[string]appcontent.Status
	revs     map[string]appcontent.Revision
	deleted  map[string]bool
	ceiling  bool
	actors   map[string]appaccount.Actor
	// followers of the author, newest first.
	followers []string
	blocks    map[[2]string]bool
	suspended map[string]bool
	boosts    map[string]appcontent.Boost
}

func actorOn(id, uri, domain string) appaccount.Actor {
	var a appaccount.Actor
	a.Account.ID, a.Account.URI, a.Account.Domain = id, uri, domain
	return a
}

func newObjWorld() *objWorld {
	w := &objWorld{
		statuses: map[string]appcontent.Status{}, revs: map[string]appcontent.Revision{}, deleted: map[string]bool{},
		actors: map[string]appaccount.Actor{}, blocks: map[[2]string]bool{}, suspended: map[string]bool{},
		boosts: map[string]appcontent.Boost{},
	}
	author := actorOn(authorID, "https://"+objHost+"/ap/actors/"+authorID, "")
	// The author is a local account named "author". The account module's domain types are not
	// this module's to import, so the local account is made by reflection.
	local := reflect.New(reflect.TypeOf(author.Local).Elem())
	local.Elem().FieldByName("Username").SetString("author")
	reflect.ValueOf(&author).Elem().FieldByName("Local").Set(local)
	for _, a := range []appaccount.Actor{
		author,
		actorOn(followerID, "https://follower.example/users/f", "follower.example"),
		actorOn(mentionedID, "https://mentioned.example/users/m", "mentioned.example"),
		actorOn(blockedID, "https://follower.example/users/blocked", "follower.example"),
		actorOn(remoteAuthor, "https://follower.example/users/r", "follower.example"),
	} {
		w.actors[a.Account.ID] = a
	}
	w.followers = []string{followerID}
	w.blocks[[2]string{authorID, blockedID}] = true
	return w
}

// post adds a status of the author in v that mentions mentionedID.
func (w *objWorld) post(id string, v appcontent.Visibility) appcontent.Status {
	st := appcontent.Status{
		ID: id, URI: appcontent.StatusURI(objHost, id),
		AuthorAccountID: authorID, IsLocal: true, Visibility: v, PublishedAt: objNow.Add(-time.Hour),
	}
	w.statuses[id] = st
	w.revs[id] = appcontent.Revision{ID: id + "-r", StatusID: id, Seq: 1, RevisedAt: st.PublishedAt, MentionedAccountIDs: []string{mentionedID}}
	return st
}

func (w *objWorld) ObjectStatus(_ context.Context, id string) (appcontent.Status, appcontent.Revision, bool, error) {
	st, ok := w.statuses[id]
	if !ok {
		return appcontent.Status{}, appcontent.Revision{}, false, appcontent.ErrNotFound
	}
	return st, w.revs[id], w.deleted[id], nil
}

func (w *objWorld) AuthorRestrictions(context.Context, string) (appcontent.AuthorRestrictions, error) {
	return appcontent.AuthorRestrictions{VisibilityCeilingPrivate: w.ceiling}, nil
}

func (w *objWorld) PostCount(context.Context, string) (int64, error) {
	return int64(len(w.statuses)), nil
}

func (w *objWorld) FindByID(_ context.Context, id string) (appaccount.Actor, error) {
	a, ok := w.actors[id]
	if !ok {
		return appaccount.Actor{}, appaccount.ErrNotFound
	}
	return a, nil
}

func (w *objWorld) FindByURI(_ context.Context, uri string) (appaccount.Actor, error) {
	for _, a := range w.actors {
		if a.Account.URI == uri {
			return a, nil
		}
	}
	return appaccount.Actor{}, appaccount.ErrNotFound
}

func (w *objWorld) AnyOnDomain(_ context.Context, domain string, ids []string) (bool, error) {
	for _, id := range ids {
		if a, ok := w.actors[id]; ok && a.Account.Domain == domain {
			return true, nil
		}
	}
	return false, nil
}

// ActiveFollowerIDs returns one follower per batch, to walk several batches.
func (w *objWorld) ActiveFollowerIDs(_ context.Context, _ string, before string, _ int) ([]string, string, error) {
	i := 0
	if before != "" {
		i = slices.Index(w.followers, before) + 1
	}
	if i >= len(w.followers) {
		return nil, "", nil
	}
	next := ""
	if i+1 < len(w.followers) {
		next = w.followers[i]
	}
	return w.followers[i : i+1], next, nil
}

func (w *objWorld) Relationship(_ context.Context, accountID, targetID string) (appgraph.Relationship, error) {
	return appgraph.Relationship{
		AccountID: accountID, TargetAccountID: targetID,
		Blocking: w.blocks[[2]string{accountID, targetID}], BlockedBy: w.blocks[[2]string{targetID, accountID}],
	}, nil
}

func (w *objWorld) FederationSuspended(_ context.Context, domain string) (bool, error) {
	return w.suspended[domain], nil
}

// Profile lists the author's public and unlisted statuses, as the timeline does for an
// anonymous viewer, and all of them for the author (a data export).
func (w *objWorld) Profile(_ context.Context, viewer appcontent.Viewer, _ string, f apptimeline.ProfileFilter, _ apptimeline.Query) (apptimeline.Page, error) {
	owner := viewer.Kind == appcontent.ViewerLocal && viewer.AccountID == authorID
	if (viewer.Kind != appcontent.ViewerAnonymous && !owner) || !f.ExcludeBoosts {
		return apptimeline.Page{}, errors.New("the outbox lists the posts an anonymous visitor or the author sees")
	}
	var p apptimeline.Page
	for _, id := range slices.Sorted(maps.Keys(w.statuses)) {
		if v := w.statuses[id].Visibility; owner || v == appcontent.VisibilityPublic || v == appcontent.VisibilityUnlisted {
			p.Entries = append(p.Entries, apptimeline.Entry{ID: id, StatusID: id})
		}
	}
	return p, nil
}

func (w *objWorld) Views(_ context.Context, _ appcontent.Viewer, ids []string) ([]appcontent.StatusView, error) {
	var out []appcontent.StatusView
	for _, id := range ids {
		out = append(out, appcontent.StatusView{
			Status: w.statuses[id], Revision: w.revs[id], Author: w.actors[authorID], Mentions: []appaccount.Actor{w.actors[mentionedID]},
		})
	}
	return out, nil
}

// Parts resolves the mentioned accounts of a revision; the statuses have no media or hashtags.
func (w *objWorld) Parts(_ context.Context, rev appcontent.Revision) (appcontent.RevisionParts, error) {
	var p appcontent.RevisionParts
	for _, id := range rev.MentionedAccountIDs {
		if a, ok := w.actors[id]; ok {
			p.Mentions = append(p.Mentions, a)
		}
	}
	return p, nil
}

func (w *objWorld) EmojiTags(context.Context, []string) ([]appcontent.EmojiTag, error) {
	return nil, nil
}

func (w *objWorld) Pinned(ctx context.Context, viewer appcontent.Viewer, _ string) ([]appcontent.StatusView, error) {
	p, err := w.Profile(ctx, viewer, authorID, apptimeline.ProfileFilter{ExcludeBoosts: true}, apptimeline.Query{})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(p.Entries))
	for _, e := range p.Entries {
		ids = append(ids, e.StatusID)
	}
	return w.Views(ctx, viewer, ids)
}

func newObjects(w *objWorld, sv *appfederation.SignatureVerifier) *appfederation.Objects {
	return appfederation.NewObjects(appfederation.ObjectsDeps{
		Statuses: w, Accounts: w, Graph: w, Timeline: w, Reader: w, Boosts: w, Signatures: sv, Domains: w, Host: objHost,
	})
}

func (w *objWorld) FindLiveBoost(_ context.Context, id string) (appcontent.Boost, error) {
	b, ok := w.boosts[id]
	if !ok {
		return appcontent.Boost{}, appcontent.ErrNotFound
	}
	return b, nil
}

// TestObjectsAnnounce checks which boosts GET of the Announce serves (ADR-0018 §16).
func TestObjectsAnnounce(t *testing.T) {
	const (
		publicID   = "0192f3b0-0000-7000-8000-000000000301"
		unlistedID = "0192f3b0-0000-7000-8000-000000000302"
		privateID  = "0192f3b0-0000-7000-8000-000000000303"
		localID    = "0192f3b0-0000-7000-8000-000000000304"
		directID   = "0192f3b0-0000-7000-8000-000000000305"
		remoteID   = "0192f3b0-0000-7000-8000-000000000306"
		deletedID  = "0192f3b0-0000-7000-8000-000000000307"
		ofRemoteID = "0192f3b0-0000-7000-8000-000000000308"
		missingID  = "0192f3b0-0000-7000-8000-0000000003ff"
	)
	w := newObjWorld()
	boost := func(id string, v appcontent.Visibility, local bool, st appcontent.Status) {
		w.boosts[id] = appcontent.Boost{
			ID: id, URI: appcontent.ActivityURI(objHost, id), BoosterAccountID: authorID, StatusID: st.ID,
			Visibility: v, IsLocal: local, BoostedAt: objNow,
		}
	}
	status := func(n string, v appcontent.Visibility) appcontent.Status {
		return w.post("0192f3b0-0000-7000-8000-0000000004"+n, v)
	}
	publicStatus := status("01", appcontent.VisibilityPublic)
	boost(publicID, appcontent.VisibilityPublic, true, publicStatus)
	boost(unlistedID, appcontent.VisibilityUnlisted, true, status("02", appcontent.VisibilityUnlisted))
	boost(privateID, appcontent.VisibilityPrivate, true, status("03", appcontent.VisibilityPrivate))
	boost(localID, appcontent.VisibilityLocal, true, status("04", appcontent.VisibilityLocal))
	boost(directID, appcontent.VisibilityDirect, true, status("05", appcontent.VisibilityDirect))
	boost(remoteID, appcontent.VisibilityPublic, false, publicStatus)
	gone := status("07", appcontent.VisibilityPublic)
	w.deleted[gone.ID] = true
	boost(deletedID, appcontent.VisibilityPublic, true, gone)
	ofRemote := status("08", appcontent.VisibilityPublic)
	ofRemote.IsLocal, ofRemote.AuthorAccountID = false, remoteAuthor
	w.statuses[ofRemote.ID] = ofRemote
	boost(ofRemoteID, appcontent.VisibilityPublic, true, ofRemote)
	o := newObjects(w, nil)
	actor := "https://" + objHost + "/ap/actors/" + authorID

	for _, id := range []string{publicID, unlistedID, ofRemoteID} {
		obj, err := o.Announce(t.Context(), appfederation.Requester{}, id)
		if err != nil {
			t.Fatalf("Announce(%s): %v", id, err)
		}
		doc := decodeDoc(t, obj.Document)
		if !obj.Public || obj.Gone || doc["id"] != appcontent.ActivityURI(objHost, id) || doc["type"] != "Announce" ||
			doc["actor"] != actor || doc["object"] != w.statuses[w.boosts[id].StatusID].URI || doc["@context"] == nil ||
			doc["published"] != "2026-10-17T12:00:00Z" {
			t.Errorf("Announce(%s) = %+v, %v", id, obj, doc)
		}
	}
	obj, err := o.Announce(t.Context(), appfederation.Requester{}, ofRemoteID)
	if err != nil {
		t.Fatal(err)
	}
	doc := decodeDoc(t, obj.Document)
	if !slices.Equal(strs(doc["to"]), []string{"https://www.w3.org/ns/activitystreams#Public"}) ||
		!slices.Equal(strs(doc["cc"]), []string{actor + "/followers", "https://follower.example/users/r"}) {
		t.Errorf("Announce addressing = to %v cc %v", doc["to"], doc["cc"])
	}
	for _, id := range []string{privateID, localID, directID, remoteID, deletedID, missingID} {
		if _, err := o.Announce(t.Context(), appfederation.Requester{}, id); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("Announce(%s) = %v, want ErrNotFound", id, err)
		}
	}
	// A block between the signer and the booster hides it from that signer alone.
	blocked := appfederation.Requester{Domain: "follower.example", AccountID: blockedID}
	if _, err := o.Announce(t.Context(), blocked, publicID); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("Announce to a blocked signer = %v, want ErrNotFound", err)
	}
	if _, err := o.Announce(t.Context(), appfederation.Requester{Domain: "follower.example", AccountID: followerID}, publicID); err != nil {
		t.Errorf("Announce to a signer = %v", err)
	}
	// A ceiling on the booster makes the Announce followers-only, which is not served.
	w.ceiling = true
	if _, err := o.Announce(t.Context(), appfederation.Requester{Domain: "follower.example", AccountID: followerID}, publicID); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("Announce under a ceiling = %v, want ErrNotFound", err)
	}
	w.ceiling = false
	// Without the port no Announce is served.
	none := appfederation.NewObjects(appfederation.ObjectsDeps{Statuses: w, Accounts: w, Graph: w, Timeline: w, Reader: w, Domains: w, Host: objHost})
	if _, err := none.Announce(t.Context(), appfederation.Requester{}, publicID); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("Announce without Boosts = %v, want ErrNotFound", err)
	}
}

// TestObjectsCanView checks the decision of object fetches (visibility.md §5.1, data-model §7):
// public and unlisted for anyone, local for the servers of followers, private for those of
// followers and mentioned accounts, direct for those of mentioned accounts.
func TestObjectsCanView(t *testing.T) {
	anon := appfederation.Requester{}
	follower := appfederation.Requester{Domain: "follower.example"}
	followerAccount := appfederation.Requester{Domain: "follower.example", AccountID: followerID}
	mentioned := appfederation.Requester{Domain: "mentioned.example"}
	stranger := appfederation.Requester{Domain: "stranger.example"}
	blocked := appfederation.Requester{Domain: "follower.example", AccountID: blockedID}
	cases := []struct {
		visibility appcontent.Visibility
		visible    []appfederation.Requester
		hidden     []appfederation.Requester
	}{
		{appcontent.VisibilityPublic, []appfederation.Requester{anon, follower, mentioned, stranger}, []appfederation.Requester{blocked}},
		{appcontent.VisibilityUnlisted, []appfederation.Requester{anon, follower, mentioned, stranger}, []appfederation.Requester{blocked}},
		{appcontent.VisibilityLocal, []appfederation.Requester{follower, followerAccount}, []appfederation.Requester{anon, mentioned, stranger, blocked}},
		{appcontent.VisibilityPrivate, []appfederation.Requester{follower, mentioned}, []appfederation.Requester{anon, stranger, blocked}},
		{appcontent.VisibilityDirect, []appfederation.Requester{mentioned}, []appfederation.Requester{anon, follower, stranger, blocked}},
	}
	for _, tc := range cases {
		t.Run(string(tc.visibility), func(t *testing.T) {
			w := newObjWorld()
			// Two followers: the one on follower.example is in the second batch.
			w.followers = []string{remoteAuthor + "-x", followerID}
			w.actors[remoteAuthor+"-x"] = actorOn(remoteAuthor+"-x", "https://elsewhere.example/u", "elsewhere.example")
			st := w.post("0192f3b0-0000-7000-8000-000000000100", tc.visibility)
			o := newObjects(w, nil)
			for _, req := range tc.visible {
				obj, err := o.Status(t.Context(), req, st.ID)
				if err != nil || obj.Gone {
					t.Errorf("Status for %+v = %+v, %v; want the Note", req, obj, err)
				}
				wantPublic := tc.visibility == appcontent.VisibilityPublic || tc.visibility == appcontent.VisibilityUnlisted
				if obj.Public != wantPublic {
					t.Errorf("Status for %+v: Public = %v, want %v", req, obj.Public, wantPublic)
				}
			}
			for _, req := range tc.hidden {
				for name, get := range map[string]func(context.Context, appfederation.Requester, string) (appfederation.Object, error){
					"Status": o.Status, "StatusActivity": o.StatusActivity,
				} {
					if _, err := get(t.Context(), req, st.ID); !errors.Is(err, appfederation.ErrNotFound) {
						t.Errorf("%s for %+v = %v, want ErrNotFound", name, req, err)
					}
				}
			}
		})
	}

	t.Run("a silenced author's public status is followers-only", func(t *testing.T) {
		w := newObjWorld()
		w.ceiling = true
		st := w.post("0192f3b0-0000-7000-8000-000000000101", appcontent.VisibilityPublic)
		o := newObjects(w, nil)
		if _, err := o.Status(t.Context(), anon, st.ID); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("anonymous Status = %v, want ErrNotFound", err)
		}
		obj, err := o.Status(t.Context(), follower, st.ID)
		if err != nil {
			t.Fatalf("Status for a follower's server: %v", err)
		}
		if doc := decodeDoc(t, obj.Document); !slices.Equal(strs(doc["to"]), []string{"https://" + objHost + "/ap/actors/" + authorID + "/followers"}) {
			t.Errorf("to = %v, want the followers only", doc["to"])
		}
	})

	t.Run("missing and remote statuses", func(t *testing.T) {
		w := newObjWorld()
		st := w.post("0192f3b0-0000-7000-8000-000000000102", appcontent.VisibilityPublic)
		st.IsLocal, st.AuthorAccountID = false, remoteAuthor
		w.statuses[st.ID] = st
		o := newObjects(w, nil)
		for _, id := range []string{st.ID, "0192f3b0-0000-7000-8000-0000000001ff"} {
			if _, err := o.Status(t.Context(), follower, id); !errors.Is(err, appfederation.ErrNotFound) {
				t.Errorf("Status(%s) = %v, want ErrNotFound", id, err)
			}
		}
	})
}

// TestObjectsPageStatusURI checks the redirect of a post page that asks for ActivityPub
// (ADR-0018): only a live local status that an anonymous visitor may see has an id to go to.
func TestObjectsPageStatusURI(t *testing.T) {
	w := newObjWorld()
	public := w.post("0192f3b0-0000-7000-8000-000000000a01", appcontent.VisibilityPublic)
	unlisted := w.post("0192f3b0-0000-7000-8000-000000000a02", appcontent.VisibilityUnlisted)
	private := w.post("0192f3b0-0000-7000-8000-000000000a03", appcontent.VisibilityPrivate)
	deleted := w.post("0192f3b0-0000-7000-8000-000000000a04", appcontent.VisibilityPublic)
	w.deleted[deleted.ID] = true
	remote := w.post("0192f3b0-0000-7000-8000-000000000a05", appcontent.VisibilityPublic)
	remote.IsLocal, remote.AuthorAccountID = false, remoteAuthor
	w.statuses[remote.ID] = remote
	o := newObjects(w, nil)
	for _, st := range []appcontent.Status{public, unlisted} {
		if got, err := o.PageStatusURI(t.Context(), st.ID); err != nil || got != st.URI {
			t.Errorf("PageStatusURI(%s) = %q, %v, want %q", st.Visibility, got, err, st.URI)
		}
	}
	for _, id := range []string{private.ID, deleted.ID, remote.ID, "0192f3b0-0000-7000-8000-000000000aff"} {
		if got, err := o.PageStatusURI(t.Context(), id); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("PageStatusURI(%s) = %q, %v, want ErrNotFound", id, got, err)
		}
	}
}

func decodeDoc(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return doc
}

func strs(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func TestObjectsDocuments(t *testing.T) {
	w := newObjWorld()
	st := w.post("0192f3b0-0000-7000-8000-000000000200", appcontent.VisibilityPublic)
	rev := w.revs[st.ID]
	rev.Seq, rev.RevisedAt = 2, objNow
	w.revs[st.ID] = rev
	o := newObjects(w, nil)
	actor := "https://" + objHost + "/ap/actors/" + authorID
	// The url is the page under the author's current username, derived on read (ADR-0018).
	pageURL := "https://" + objHost + "/@author/status/" + st.ID

	obj, err := o.Status(t.Context(), appfederation.Requester{}, st.ID)
	if err != nil {
		t.Fatal(err)
	}
	note := decodeDoc(t, obj.Document)
	if note["@context"] == nil || note["id"] != st.URI || note["type"] != "Note" || note["attributedTo"] != actor ||
		note["url"] != pageURL || note["updated"] != "2026-10-17T12:00:00Z" {
		t.Errorf("Note = %v", note)
	}
	if !slices.Equal(strs(note["to"]), []string{"https://www.w3.org/ns/activitystreams#Public"}) ||
		!slices.Equal(strs(note["cc"]), []string{actor + "/followers", "https://mentioned.example/users/m"}) {
		t.Errorf("Note addressing = to %v cc %v", note["to"], note["cc"])
	}
	if obj.URL != pageURL || !obj.Public || obj.Gone {
		t.Errorf("Object = %+v", obj)
	}

	obj, err = o.StatusActivity(t.Context(), appfederation.Requester{}, st.ID)
	if err != nil {
		t.Fatal(err)
	}
	create := decodeDoc(t, obj.Document)
	if create["id"] != st.URI+"/activity" || create["type"] != "Create" || create["actor"] != actor ||
		create["published"] != "2026-10-17T11:00:00Z" || create["object"].(map[string]any)["id"] != st.URI {
		t.Errorf("Create = %v", create)
	}

	t.Run("deleted", func(t *testing.T) {
		w.deleted[st.ID] = true
		defer delete(w.deleted, st.ID)
		for suffix, get := range map[string]func(context.Context, appfederation.Requester, string) (appfederation.Object, error){
			"": o.Status, "/activity": o.StatusActivity,
		} {
			obj, err := get(t.Context(), appfederation.Requester{}, st.ID)
			if err != nil || !obj.Gone {
				t.Fatalf("deleted%s = %+v, %v; want Gone", suffix, obj, err)
			}
			if doc := decodeDoc(t, obj.Document); doc["id"] != st.URI+suffix || doc["type"] != "Tombstone" || doc["@context"] == nil {
				t.Errorf("Tombstone = %v", doc)
			}
		}
		hidden := w.post("0192f3b0-0000-7000-8000-000000000201", appcontent.VisibilityDirect)
		w.deleted[hidden.ID] = true
		if _, err := o.Status(t.Context(), appfederation.Requester{Domain: "stranger.example"}, hidden.ID); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("a deleted status the requester could not see = %v, want ErrNotFound", err)
		}
	})

	t.Run("outbox and featured", func(t *testing.T) {
		w := newObjWorld()
		for i, v := range []appcontent.Visibility{
			appcontent.VisibilityPublic, appcontent.VisibilityLocal, appcontent.VisibilityUnlisted,
			appcontent.VisibilityPrivate, appcontent.VisibilityDirect,
		} {
			w.post("0192f3b0-0000-7000-8000-00000000030"+string(rune('0'+i)), v)
		}
		o := newObjects(w, nil)
		page, err := o.Outbox(t.Context(), authorID, "")
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, it := range page.Items {
			if it.Type != "Create" || it.Context != nil {
				t.Errorf("outbox item = %+v, want an embedded Create", it)
			}
			got = append(got, string(it.ID))
		}
		want := []string{
			appcontent.StatusURI(objHost, "0192f3b0-0000-7000-8000-000000000300") + "/activity",
			appcontent.StatusURI(objHost, "0192f3b0-0000-7000-8000-000000000302") + "/activity",
		}
		if !slices.Equal(got, want) {
			t.Errorf("outbox = %v, want %v", got, want)
		}
		if total, err := o.OutboxTotal(t.Context(), authorID); err != nil || total != 5 {
			t.Errorf("OutboxTotal = %d, %v; want 5", total, err)
		}
		notes, err := o.Featured(t.Context(), authorID)
		if err != nil || len(notes) != 2 || notes[0].Type != "Note" {
			t.Errorf("Featured = %+v, %v; want 2 Notes", notes, err)
		}
	})
}

// fakeVerifier accepts every draft-cavage signature whose keyId is in the Signature header.
type fakeVerifier struct{}

func (fakeVerifier) Parse(r *appfederation.SignedRequest) (*appfederation.Signature, error) {
	keyID := r.Header.Get("Signature")
	if keyID == "" {
		return nil, appfederation.ErrNoSignature
	}
	return &appfederation.Signature{
		Scheme: appfederation.SchemeCavage, KeyID: keyID, Algorithm: "rsa-sha256",
		Components: []string{"(request-target)", "host", "date"},
	}, nil
}

func (fakeVerifier) Verify(*appfederation.SignedRequest, *appfederation.Signature, appfederation.VerifyOptions) error {
	return nil
}

// ownerKeys resolves every key ID to its owner without the fragment; keys under /unknown are
// not found.
type ownerKeys struct{ key *rsa.PublicKey }

func (k ownerKeys) ResolvePublicKey(_ context.Context, keyID string, _ bool) (appfederation.PublicKey, error) {
	u, err := url.Parse(keyID)
	if err != nil || u.Path == "/unknown" {
		return appfederation.PublicKey{}, appfederation.ErrPublicKeyNotFound
	}
	owner := u.Query().Get("owner")
	if owner == "" {
		u.Fragment = ""
		owner = u.String()
	}
	return appfederation.PublicKey{KeyID: keyID, Owner: owner, Key: k.key}, nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestObjectsAuthenticate(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	w := newObjWorld()
	w.suspended["suspended.example"] = true
	// The KV store only throttles refetches after a mismatch, which the fake verifier never reports.
	sv := appfederation.NewSignatureVerifier(fakeVerifier{}, ownerKeys{key: &priv.PublicKey}, nil, fixedClock{objNow})
	o := newObjects(w, sv)
	origin := &url.URL{Scheme: "https", Host: objHost}
	request := func(keyID string) *appfederation.SignedRequest {
		r, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://internal/ap/objects/x", nil)
		r.Header.Set("Date", objNow.Format(http.TimeFormat))
		if keyID != "" {
			r.Header.Set("Signature", keyID)
		}
		return appfederation.NewSignedRequest(origin, r, nil)
	}
	for _, tc := range []struct {
		name, keyID string
		want        appfederation.Requester
	}{
		{"unsigned", "", appfederation.Requester{}},
		{"known signer", "https://follower.example/users/f#main-key", appfederation.Requester{Domain: "follower.example", AccountID: followerID}},
		{"unknown signer", "https://Stranger.Example/users/s#main-key", appfederation.Requester{Domain: "stranger.example"}},
		{"unknown key", "https://follower.example/unknown", appfederation.Requester{}},
		{"owner on another host", "https://follower.example/key?owner=https://other.example/users/o", appfederation.Requester{}},
		{"suspended domain", "https://suspended.example/users/s#main-key", appfederation.Requester{}},
		{"this server", "https://" + objHost + "/ap/actors/" + authorID + "#main-key", appfederation.Requester{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := o.Authenticate(t.Context(), request(tc.keyID))
			if err != nil || got != tc.want {
				t.Errorf("Authenticate = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}

	t.Run("expired signature", func(t *testing.T) {
		r := request("https://follower.example/users/f#main-key")
		r.Header.Set("Date", objNow.Add(-24*time.Hour).Format(http.TimeFormat))
		if got, err := o.Authenticate(t.Context(), r); err != nil || !got.Anonymous() {
			t.Errorf("Authenticate = %+v, %v; want anonymous", got, err)
		}
	})

	t.Run("no key can be resolved", func(t *testing.T) {
		sv := appfederation.NewSignatureVerifier(fakeVerifier{}, appfederation.UnknownPublicKeys{}, nil, fixedClock{objNow})
		got, err := newObjects(w, sv).Authenticate(t.Context(), request("https://follower.example/users/f#main-key"))
		if err != nil || !got.Anonymous() {
			t.Errorf("Authenticate = %+v, %v; want anonymous", got, err)
		}
	})
}

// fetchSetting is an appfederation.FetchSettings with a fixed value.
type fetchSetting struct {
	on  bool
	err error
}

func (s fetchSetting) AuthorizedFetch(context.Context) (bool, error) { return s.on, s.err }

func TestObjectsAuthorizedFetch(t *testing.T) {
	if on, err := appfederation.NewObjects(appfederation.ObjectsDeps{}).AuthorizedFetch(t.Context()); on || err != nil {
		t.Errorf("without settings = %v, %v; want off", on, err)
	}
	for _, s := range []fetchSetting{{on: true}, {on: false}, {err: errors.New("database is down")}} {
		on, err := appfederation.NewObjects(appfederation.ObjectsDeps{Settings: s}).AuthorizedFetch(t.Context())
		if on != s.on || !errors.Is(err, s.err) {
			t.Errorf("AuthorizedFetch = %v, %v; want %v, %v", on, err, s.on, s.err)
		}
	}
}
