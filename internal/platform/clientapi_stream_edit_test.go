// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// edit edits the local status id of author to text through the content service.
func (e *timelineAPI) edit(t *testing.T, author timelineUser, id, text string) {
	t.Helper()
	if _, err := e.app.Content.Edit(t.Context(), appcontent.EditInput{
		AccountID: author.accountID, StatusID: id, TextInput: appcontent.TextInput{Text: text, Language: "en"},
	}); err != nil {
		t.Fatalf("Edit(%s, %q): %v", id, text, err)
	}
}

// editByAPI edits the status id of author to text with PUT /statuses/{id}; author's token has the
// write scope.
func (e *timelineAPI) editByAPI(t *testing.T, author timelineUser, id, text string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"text": text, "language": "en"})
	if err != nil {
		t.Fatal(err)
	}
	if res := e.listCall(t, author, http.MethodPut, "/statuses/"+id, string(body)); res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /statuses/%s = %d", id, res.StatusCode)
	}
}

// subscribe waits for n subscriptions, so that no signal is published before them.
func (e *timelineAPI) subscribe(t *testing.T, n int) {
	t.Helper()
	if !testkit.Eventually(t, 5*time.Second, func() bool { return e.app.Streams.Len() >= n }) {
		t.Fatalf("%d of %d streams subscribed", e.app.Streams.Len(), n)
	}
}

// streamedEdits reads the events of a stream until the status_update of last, and returns its
// status_update and delete events in order, as "status_update:<id>" and "delete:<id>", with the
// Status of each status_update by ID. Other events, and the events of the statuses of skip, which
// the stream may or may not send, are left out. An edit that the stream does not send would come
// before the status_update of last: reading up to it proves that none was sent.
func streamedEdits(t *testing.T, e *timelineAPI, name string, events <-chan sseEvent, last string, skip ...string) ([]string, map[string]clientapi.Status) {
	t.Helper()
	var got []string
	statuses := map[string]clientapi.Status{}
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("%s ended before the status_update of %s; got %v", name, last, got)
			}
			var id string
			switch ev.name {
			case "status_update":
				e.spec.ValidateSchema(t, "Status", ev.data)
				var st clientapi.Status
				if err := json.Unmarshal(ev.data, &st); err != nil {
					t.Fatal(err)
				}
				id = st.Id
				statuses[id] = st
			case "delete":
				var data struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal(ev.data, &data); err != nil {
					t.Fatal(err)
				}
				id = data.ID
			default:
				continue
			}
			if !slices.Contains(skip, id) {
				got = append(got, ev.name+":"+id)
			}
			if ev.name == "status_update" && id == last {
				return got, statuses
			}
		case <-timeout:
			t.Fatalf("%s: no status_update of %s within 10 seconds; got %v", name, last, got)
		}
	}
}

// updates returns "status_update:<id>" for each of ids.
func updates(ids ...string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = "status_update:" + id
	}
	return out
}

// TestStreamsSendTheEdits opens user and timeline streams on the real database and checks that an
// edited status reaches, as one status_update with the edited Status as each viewer sees it, the
// users who may see it and no one else: its author, its author's followers, and the accounts a
// private or direct post mentions, once for a follower it also mentions; not a viewer whom a block
// or a mute keeps from it, not a token without read:direct for a direct post, not a non-follower
// for a private one, and not an account that the edit stopped mentioning. The timeline streams
// receive the edits of the statuses their timeline lists.
func TestStreamsSendTheEdits(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	e := newTimelineAPI(t)
	alice, bob, dave, erin, frank := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "dave"), e.user(t, "erin"), e.user(t, "frank")
	carol := e.userWithScope(t, "carol", "read write")
	bobNarrow, erinNarrow := bob, erin
	bobNarrow.token = issueTokens(t, e.app, bob.userID, "read:statuses read:notifications").AccessToken
	erinNarrow.token = issueTokens(t, e.app, erin.userID, "read:statuses read:notifications").AccessToken
	e.follow(t, alice, bob)
	e.follow(t, frank, bob)
	e.follow(t, dave, bob)
	if _, err := e.app.Graph.Block(ctx, bob.accountID, dave.accountID); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if _, err := e.app.Graph.Mute(ctx, frank.accountID, bob.accountID, appgraph.MuteInput{}); err != nil {
		t.Fatalf("Mute: %v", err)
	}
	private := e.post(t, bob, "for my followers @erin @alice", domcontent.VisibilityPrivate, "en")
	unlisted := e.post(t, bob, "quietly", domcontent.VisibilityUnlisted, "en")
	public := e.post(t, bob, "for everyone", domcontent.VisibilityPublic, "en")
	direct := e.post(t, bob, "secret @erin", domcontent.VisibilityDirect, "en")
	dropped := e.post(t, bob, "hi @erin", domcontent.VisibilityPrivate, "en")
	if _, err := e.app.Interactions.Favourite(ctx, alice.accountID, public); err != nil {
		t.Fatalf("Favourite: %v", err)
	}
	users := map[string]timelineUser{"alice": alice, "bob": bob, "carol": carol, "dave": dave, "erin": erin, "frank": frank}
	// Each user's own private post: its edit ends the user's streams.
	markers := map[string]string{}
	for name, u := range users {
		markers[name] = e.post(t, u, "marker", domcontent.VisibilityPrivate, "en")
	}
	last := e.post(t, carol, "the last", domcontent.VisibilityPublic, "en")

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	streamCtx, disconnect := context.WithCancel(ctx)
	defer disconnect()
	viewers := map[string]timelineUser{"bob without read:direct": bobNarrow, "erin without read:direct": erinNarrow}
	for name, u := range users {
		viewers[name] = u
	}
	streams := map[string]<-chan sseEvent{}
	for name, viewer := range viewers {
		streams[name] = openTimelineStream(streamCtx, t, e, srv, viewer, "/streaming")
	}
	timelines := map[string]<-chan sseEvent{
		"carol's local timeline":     openTimelineStream(streamCtx, t, e, srv, carol, "/streaming/timelines/local"),
		"carol's federated timeline": openTimelineStream(streamCtx, t, e, srv, carol, "/streaming/timelines/federated"),
	}
	e.subscribe(t, len(streams)+len(timelines))

	e.edit(t, bob, private, "edited for my followers @erin @alice")
	e.edit(t, bob, unlisted, "edited quietly")
	e.edit(t, bob, public, "edited for everyone")
	e.edit(t, bob, direct, "edited secret @erin")
	e.edit(t, bob, dropped, "edited hi")
	// bob's marker after those of alice and frank, who follow him; carol's through the API.
	for _, name := range []string{"alice", "dave", "erin", "frank", "bob"} {
		e.edit(t, users[name], markers[name], "edited marker")
	}
	e.editByAPI(t, carol, markers["carol"], "edited marker")
	e.editByAPI(t, carol, last, "edited the last")

	// The public and unlisted posts may also reach a user who may see them without following bob.
	for name, tc := range map[string]struct {
		marker string
		want   []string
		skip   []string
	}{
		"alice":                    {markers["alice"], updates(private, unlisted, public, dropped, markers["alice"]), nil},
		"bob":                      {markers["bob"], updates(private, unlisted, public, direct, dropped, markers["bob"]), nil},
		"bob without read:direct":  {markers["bob"], updates(private, unlisted, public, dropped, markers["bob"]), nil},
		"carol":                    {markers["carol"], updates(markers["carol"]), []string{unlisted, public}},
		"dave":                     {markers["dave"], updates(markers["dave"]), nil},
		"erin":                     {markers["erin"], updates(private, direct, markers["erin"]), []string{unlisted, public}},
		"erin without read:direct": {markers["erin"], updates(private, markers["erin"]), []string{unlisted, public}},
		"frank":                    {markers["frank"], updates(markers["frank"]), nil},
	} {
		got, statuses := streamedEdits(t, e, name, streams[name], tc.marker, tc.skip...)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s stream = %v, want %v", name, got, tc.want)
		}
		for id, st := range statuses {
			if !strings.Contains(st.Content, "edited") || st.EditedAt == nil {
				t.Errorf("%s: status_update of %s = %q, edited_at %v; want the edited status", name, id, st.Content, st.EditedAt)
			}
		}
		if st, ok := statuses[public]; ok {
			if want := name == "alice"; st.Favourited == nil || *st.Favourited != want {
				t.Errorf("%s: status_update of the public post has favourited %v, want %v as %s sees it", name, st.Favourited, want, name)
			}
		}
	}
	for name, events := range timelines {
		if got, want := streamedEdits1(t, e, name, events, last), updates(public, last); !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
}

// streamedEdits1 is streamedEdits without the statuses.
func streamedEdits1(t *testing.T, e *timelineAPI, name string, events <-chan sseEvent, last string, skip ...string) []string {
	t.Helper()
	got, _ := streamedEdits(t, e, name, events, last, skip...)
	return got
}

// TestStreamsApplyTheKeywordFiltersToTheEdits checks that an edit that a hide filter now matches
// reaches the home and timeline streams of the filter's owner as a delete of the status, and one
// that a warn filter matches as a status_update whose Status carries the filter.
func TestStreamsApplyTheKeywordFiltersToTheEdits(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	e := newTimelineAPI(t)
	alice, bob := e.userWithScope(t, "alice", "read write"), e.user(t, "bob")
	e.follow(t, alice, bob)
	e.keywordFilter(t, alice, `{"title":"Spoilers","action":"warn","contexts":["home","public"],"keywords":[{"keyword":"Spoiler","whole_word":true}]}`)
	e.keywordFilter(t, alice, `{"title":"Ads","action":"hide","contexts":["home","public"],"keywords":[{"keyword":"SALE","whole_word":false}]}`)
	hidden := e.post(t, bob, "hello", domcontent.VisibilityPublic, "en")
	warned := e.post(t, bob, "hi", domcontent.VisibilityPublic, "en")

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	streamCtx, disconnect := context.WithCancel(ctx)
	defer disconnect()
	streams := map[string]<-chan sseEvent{
		"alice's home":           openTimelineStream(streamCtx, t, e, srv, alice, "/streaming"),
		"alice's local timeline": openTimelineStream(streamCtx, t, e, srv, alice, "/streaming/timelines/local"),
	}
	e.subscribe(t, len(streams))

	e.edit(t, bob, hidden, "everything on sale")
	e.edit(t, bob, warned, "a big spoiler!")

	for name, events := range streams {
		got, statuses := streamedEdits(t, e, name, events, warned)
		if want := []string{"delete:" + hidden, "status_update:" + warned}; !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
		if got, want := filteredBy(statuses[warned]), []string{"Spoilers:Spoiler"}; !slices.Equal(got, want) {
			t.Errorf("%s: filtered of the warned edit = %v, want %v", name, got, want)
		}
	}
}

// TestStreamsSendTheEditOfAHeldReplyToItsAuthorAlone checks that the edit of a local reply held
// until the author of its parent approves it (ADR-0155 §7) reaches its author only: not the
// author's followers, the author of the parent, or the local timeline.
func TestStreamsSendTheEditOfAHeldReplyToItsAuthorAlone(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	e := newTimelineAPI(t)
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	e.follow(t, carol, bob)
	e.follow(t, carol, alice)
	parent := e.post(t, alice, "reply after I approve", domcontent.VisibilityPublic, "en")
	if _, err := e.app.Content.SetInteractionPolicy(ctx, alice.accountID, parent, appcontent.InteractionPolicyChange{
		domcontent.InteractionReply: {Automatic: domcontent.AudienceNobody, Manual: domcontent.AudienceEveryone},
	}); err != nil {
		t.Fatalf("SetInteractionPolicy: %v", err)
	}
	p, err := e.app.Content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: bob.accountID, TextInput: appcontent.TextInput{Text: "may I?", Language: "en"},
		Visibility: domcontent.VisibilityPublic, InReplyToID: parent,
	})
	if err != nil && !errors.Is(err, appcontent.ErrInteractionAwaitsApproval) {
		t.Fatalf("Create(reply): %v", err)
	}
	reply := p.Status.ID
	st, err := e.app.Content.FindStatus(ctx, reply)
	if err != nil {
		t.Fatalf("FindStatus(%q): %v", reply, err)
	}
	if held, err := e.app.Content.Unpublished(ctx, st); err != nil || !held {
		t.Fatalf("Unpublished(reply) = %v, %v; want a held reply", held, err)
	}
	users := map[string]timelineUser{"alice": alice, "bob": bob, "carol": carol}
	markers := map[string]string{}
	for name, u := range users {
		markers[name] = e.post(t, u, "marker", domcontent.VisibilityPrivate, "en")
	}
	last := e.post(t, carol, "the last", domcontent.VisibilityPublic, "en")

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	streamCtx, disconnect := context.WithCancel(ctx)
	defer disconnect()
	streams := map[string]<-chan sseEvent{}
	for name, viewer := range users {
		streams[name] = openTimelineStream(streamCtx, t, e, srv, viewer, "/streaming")
	}
	carolsLocal := openTimelineStream(streamCtx, t, e, srv, carol, "/streaming/timelines/local")
	e.subscribe(t, len(streams)+1)

	e.edit(t, bob, reply, "may I, please?")
	// carol follows the others: her own marker comes before theirs.
	for _, name := range []string{"carol", "alice", "bob"} {
		e.edit(t, users[name], markers[name], "edited marker")
	}
	e.edit(t, carol, last, "edited the last")

	for name, want := range map[string][]string{
		"alice": updates(markers["alice"]),
		"bob":   updates(reply, markers["bob"]),
		"carol": updates(markers["carol"]),
	} {
		if got := streamedEdits1(t, e, name, streams[name], markers[name]); !slices.Equal(got, want) {
			t.Errorf("%s stream = %v, want %v", name, got, want)
		}
	}
	if got, want := streamedEdits1(t, e, "carol's local timeline", carolsLocal, last), updates(last); !slices.Equal(got, want) {
		t.Errorf("carol's local timeline = %v, want %v without the held reply", got, want)
	}
}

// TestStreamsSendTheRemoteEdits checks that a remote Update of a stored post reaches the local
// users who may see it: the accounts a private or direct post mentions, not one that the revision
// no longer mentions nor one that it never did; and the federated timeline its public post.
func TestStreamsSendTheRemoteEdits(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	e := newTimelineAPI(t)
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	const actor = "https://remote.example/users/carol"
	var carol account.Account
	err := e.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		carol, err = e.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: actor, Domain: "remote.example", Type: "Person", PreferredUsername: "carol", Name: "Carol",
			ProfileURL: "https://remote.example/@carol", InboxURL: actor + "/inbox", FollowersURL: actor + "/followers",
			Document: []byte(`{"id":"` + actor + `"}`),
			Handle:   appaccount.HandleCheck{Outcome: account.HandleVerified, Username: "carol", Domain: "remote.example"},
		})
		return err
	})
	if err != nil {
		t.Fatalf("UpsertRemoteActor: %v", err)
	}
	published := time.Now().UTC().Add(-time.Hour)
	input := func(n string, v domcontent.Visibility, html string, at time.Time, mentions []string) appcontent.RemoteStatusInput {
		return appcontent.RemoteStatusInput{
			URI: actor + "/statuses/" + n, AuthorAccountID: carol.ID, AuthorDomain: "remote.example", Visibility: v,
			PublishedAt: published, RevisedAt: at, Bodies: []appcontent.RemoteBody{{Language: "en", HTML: html}},
			MentionedAccountIDs: mentions,
		}
	}
	mentionsAlice := []string{alice.accountID}
	posts := []struct {
		name              string
		v                 domcontent.Visibility
		mentions, revised []string // of the first and the second revision
	}{
		{"public", domcontent.VisibilityPublic, nil, nil},
		{"private", domcontent.VisibilityPrivate, mentionsAlice, mentionsAlice},
		{"direct", domcontent.VisibilityDirect, mentionsAlice, mentionsAlice},
		{"dropped", domcontent.VisibilityPrivate, mentionsAlice, nil},
		{"unrelated", domcontent.VisibilityPrivate, nil, nil},
	}
	ids := map[string]string{}
	for _, r := range posts {
		err := e.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			p, err := e.app.Content.IngestRemoteStatus(ctx, tx, input(r.name, r.v, "<p>hello</p>", published, r.mentions))
			ids[r.name] = p.Status.ID
			return err
		})
		if err != nil {
			t.Fatalf("IngestRemoteStatus(%s): %v", r.name, err)
		}
	}
	users := map[string]timelineUser{"alice": alice, "bob": bob}
	markers := map[string]string{}
	for name, u := range users {
		markers[name] = e.post(t, u, "marker", domcontent.VisibilityPrivate, "en")
	}
	last := e.post(t, bob, "the last", domcontent.VisibilityPublic, "en")

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	streamCtx, disconnect := context.WithCancel(ctx)
	defer disconnect()
	streams := map[string]<-chan sseEvent{}
	for name, viewer := range users {
		streams[name] = openTimelineStream(streamCtx, t, e, srv, viewer, "/streaming")
	}
	federated := openTimelineStream(streamCtx, t, e, srv, bob, "/streaming/timelines/federated")
	e.subscribe(t, len(streams)+1)

	revised := time.Now().UTC()
	for _, r := range posts {
		var made bool
		err := e.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			made, err = e.app.Content.ReviseRemoteStatus(ctx, tx, input(r.name, r.v, "<p>edited</p>", revised, r.revised))
			return err
		})
		if err != nil || !made {
			t.Fatalf("ReviseRemoteStatus(%s) = %v, %v; want a revision", r.name, made, err)
		}
	}
	e.edit(t, alice, markers["alice"], "edited marker")
	e.edit(t, bob, markers["bob"], "edited marker")
	e.edit(t, bob, last, "edited the last")

	// The public post may also reach a user who may see it without following carol.
	for name, want := range map[string][]string{
		"alice": updates(ids["private"], ids["direct"], markers["alice"]),
		"bob":   updates(markers["bob"]),
	} {
		got, statuses := streamedEdits(t, e, name, streams[name], markers[name], ids["public"])
		if !slices.Equal(got, want) {
			t.Errorf("%s stream = %v, want %v", name, got, want)
		}
		for id, st := range statuses {
			if !strings.Contains(st.Content, "edited") || st.EditedAt == nil {
				t.Errorf("%s: status_update of %s = %q, edited_at %v; want the edited status", name, id, st.Content, st.EditedAt)
			}
		}
	}
	if got, want := streamedEdits1(t, e, "bob's federated timeline", federated, last), updates(ids["public"], last); !slices.Equal(got, want) {
		t.Errorf("bob's federated timeline = %v, want %v", got, want)
	}
}
