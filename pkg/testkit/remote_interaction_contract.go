// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// RemoteInteractionHarness is one database under test: the harness of the remote posts, with the
// interaction and federation repositories on the same database.
type RemoteInteractionHarness struct {
	RemoteStatusHarness
	Interactions appcontent.InteractionRepository
	Federation   appfederation.Repository
}

// RemoteInteractionFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type RemoteInteractionFactory func(t *testing.T, opts uow.Options) RemoteInteractionHarness

// RemoteInteractionContract verifies the boosts, favourites, and emoji reactions across servers
// (federation.md §3.4, §7.1, ADR-0052) on a database adapter, through the inbox processor with
// the real Announce, Like, EmojiReact, and Undo handlers, the real content, account, graph,
// timeline, notification, and federation modules, and the outbound projectors and fan-out: the
// Announce and Like documents of the major implementations (the federation fixtures) boost and
// favourite local posts with counters and notifications, and their Undo takes them back; the
// Misskey reactions and the Akkoma EmojiReact documents (Unicode, the sender's custom emoji, and a
// third server's) react to local posts, counted per emoji with notifications, and their Undo, by
// the activity's IRI or by what it embeds, takes them back; a remote post that a followed account boosts is stored, from the Announce when it
// embeds the post from the booster's origin and fetched again from its origin otherwise; a boost
// takes the narrower of its addressing and the post's visibility; local-only posts and posts the
// booster may not see or boost are not boosted, and local boosts of local-only posts do not
// federate (ADR-0017); a local favourite of a remote post sends a Like, and its removal an Undo
// Like, to the post's author, both read back as the activity they are; and the final rows do not
// depend on the order or repetition of the activities (NFR-REL-1); switching the Like slot
// between favourites and reactions delivers the Undo of what was in it before the next Like to
// each inbox, also while an earlier delivery retries (ADR-0004 ordering keys), so that an inbox
// of one reaction per account ends with what the sender has; and a local vote in a remote poll
// sends a Create of a Note per choice to the poll's author (FR-112), the votes in a poll being
// decided with the poll locked. The actors and the fetches are
// in-memory stand-ins.
func RemoteInteractionContract(t *testing.T, newHarness RemoteInteractionFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *remoteInteractionEnv)
	}{
		{"BoostsAndFavouritesLocalPostsFromFixtures", remoteInteractsWithLocalPosts},
		{"ReactsToLocalPostsFromFixtures", remoteReactsToLocalPosts},
		{"RefusesReactionsTheActorMayNotMake", remoteRefusesReactions},
		{"BoostsRemotePostsOfFollowedBoosters", remoteBoostsRemotePosts},
		{"RefusesBoostsTheBoosterMayNotMake", remoteRefusesBoosts},
		{"SendsLikesAndUndoLikes", remoteSendsLikes},
		{"SendsReactionsInTheLikeSlot", remoteSendsReactions},
		{"SendsPollVotes", remoteSendsPollVotes},
		{"VotesInLocalPolls", remoteVotesInLocalPolls},
		{"DecidesVotesWithThePollLocked", pollVotesDecidedUnderLock},
		{"QuotesLocalPostsWithApprovals", remoteQuotesLocalPosts},
		{"SendsQuoteRequestsAndTakesTheAnswers", remoteSendsQuoteRequests},
		{"AnswersQuoteRequestsAndRevokesApprovals", remoteAnswersQuoteRequests},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newRemoteInteractionEnv(t, newHarness)) })
	}
	t.Run("OrderAndDuplicates", func(t *testing.T) { remoteInteractionOrder(t, newHarness) })
	t.Run("DeliversSlotSwitchesInOrder", func(t *testing.T) { remoteSlotSwitchOrder(t, newHarness) })
}

type remoteInteractionEnv struct {
	*remoteStatusEnv
	ih           RemoteInteractionHarness
	interactions *appcontent.Interactions
	// interactionDeps are the ports of interactions, for an Interactions on other statuses.
	interactionDeps appcontent.InteractionDeps
	jobs            *fedJobs
	handlers        *queue.JobHandlerRegistry
	// emojiTags are the Emoji tags of the local custom emoji that reactions use, by emoji ID.
	emojiTags map[string]appcontent.EmojiTag
	// pollCloser ends polls and pollUpdates records the Updates of local polls (FR-112).
	pollCloser  *appcontent.PollCloser
	pollUpdates *appfederation.PollUpdates
	// reader presents statuses to viewers and resolves the parts of the Notes the fan-out builds.
	reader *appcontent.Reader
	// quotes sends and receives the approvals of quotes (ADR-0069), nil when the harness stores
	// no quotes.
	quotes *appfederation.QuoteHandlers
}

func newRemoteInteractionEnv(t *testing.T, newHarness RemoteInteractionFactory) *remoteInteractionEnv {
	t.Helper()
	var ih RemoteInteractionHarness
	rs := newRemoteStatusEnv(t, func(t *testing.T, opts uow.Options) RemoteStatusHarness {
		ih = newHarness(t, opts)
		return ih.RemoteStatusHarness
	})
	e := &remoteInteractionEnv{
		remoteStatusEnv: rs, ih: ih, jobs: &fedJobs{seen: map[string]bool{}}, handlers: &queue.JobHandlerRegistry{},
		emojiTags: map[string]appcontent.EmojiTag{},
	}
	appcontent.RegisterInteractionProjectors(rs.projectors, ih.Interactions)
	e.interactionDeps = appcontent.InteractionDeps{
		UnitOfWork: ih.UnitOfWork, Statuses: ih.Statuses, Interactions: ih.Interactions, Accounts: rs.accounts,
		Relationships: rs.graph, RecordRemoteEmojis: rs.content.RecordRemoteEmojis, Quotes: ih.Quotes, Clock: rs.clock, IDs: rs.ids, Host: contractHost,
		EmojiTags: func(_ context.Context, ids []string) ([]appcontent.EmojiTag, error) {
			var out []appcontent.EmojiTag
			for _, id := range ids {
				if tag, ok := e.emojiTags[id]; ok {
					out = append(out, tag)
				}
			}
			return out, nil
		},
	}
	e.interactions = appcontent.NewInteractions(e.interactionDeps)
	appfederation.RegisterInteractionHandlers(rs.processor, appfederation.InteractionHandlersDeps{
		Interactions: e.interactions, Statuses: rs.statuses,
	})
	rs.votes.target = e.interactions
	outbox := queue.AfterCommit(rs.projectors, e.jobs, nil)
	appfederation.RegisterOutboundProjectors(rs.projectors, appfederation.RecorderDeps{
		Repository: ih.Federation, Outbox: outbox, IDs: rs.ids, Host: contractHost,
	})
	e.pollCloser = appcontent.NewPollCloser(appcontent.PollCloserDeps{
		UnitOfWork: ih.UnitOfWork, Repository: ih.Statuses, Outbox: outbox, Clock: rs.clock,
	})
	e.pollCloser.RegisterProjectors(rs.projectors)
	e.pollUpdates = appfederation.NewPollUpdates(appfederation.PollUpdatesDeps{
		UnitOfWork: ih.UnitOfWork, Statuses: rs.content, Clock: rs.clock,
		Recorder: appfederation.RecorderDeps{Repository: ih.Federation, Outbox: outbox, IDs: rs.ids, Host: contractHost},
	})
	e.reader = appcontent.NewReader(appcontent.ReaderDeps{
		Statuses: ih.Statuses, Interactions: ih.Interactions, Accounts: rs.accounts, Relationships: rs.graph, Clock: rs.clock,
		Quotes: ih.Quotes,
	})
	var noteQuotes appfederation.NoteQuotes
	if ih.Quotes != nil {
		noteQuotes = rs.content
		e.quotes = appfederation.NewQuoteHandlers(appfederation.QuoteHandlersDeps{
			UnitOfWork: ih.UnitOfWork,
			Recorder:   appfederation.RecorderDeps{Repository: ih.Federation, Outbox: outbox, IDs: rs.ids, Host: contractHost},
			Approvals:  e.interactions, Quotes: rs.content, Statuses: rs.content, Accounts: rs.accounts, Fetcher: rs.fetcher, Clock: rs.clock,
		})
		e.quotes.RegisterProjectors(rs.projectors)
		e.quotes.Register(rs.processor)
		appfederation.RegisterFollowHandlers(rs.processor, appfederation.FollowHandlerDeps{Graph: rs.graph, Accounts: rs.remote, Quotes: e.quotes})
	}
	appfederation.NewFanOut(appfederation.FanOutDeps{
		UnitOfWork: ih.UnitOfWork, Repository: ih.Federation, Outbox: outbox,
		Statuses: appfederation.ContentStatuses{Service: rs.content, Interactions: e.interactions},
		Accounts: rs.accounts, Graph: rs.graph, Clock: rs.clock, IDs: rs.ids, Host: contractHost, Parts: e.reader, Quotes: noteQuotes,
	}).RegisterJobs(e.handlers)
	return e
}

// localPost makes the local account author post text with visibility v.
func (e *remoteInteractionEnv) localPost(t *testing.T, author string, v content.Visibility) content.Status {
	t.Helper()
	p, err := e.content.Create(t.Context(), appcontent.CreateInput{
		AuthorAccountID: author, TextInput: appcontent.TextInput{Text: "post"}, Visibility: v,
	})
	if err != nil {
		t.Fatalf("Create(%s): %v", v, err)
	}
	return p.Status
}

// counters returns the boosts and favourites counts of a status.
func (e *remoteInteractionEnv) counters(t *testing.T, statusID string) (boosts, favourites int64) {
	t.Helper()
	c, err := e.h.Statuses.FindCounters(t.Context(), statusID)
	if err != nil {
		t.Fatalf("FindCounters(%s): %v", statusID, err)
	}
	return c.BoostsCount, c.FavouritesCount
}

// liveBoost returns the live boost of a status by an account, false when there is none.
func (e *remoteInteractionEnv) liveBoost(t *testing.T, booster, statusID string) (content.Boost, bool) {
	t.Helper()
	edge, err := e.ih.Interactions.FindBoostEdge(t.Context(), booster, statusID)
	if errors.Is(err, appcontent.ErrNotFound) {
		return content.Boost{}, false
	}
	if err != nil {
		t.Fatalf("FindBoostEdge: %v", err)
	}
	b, err := e.ih.Interactions.FindBoost(t.Context(), edge.BoostID)
	if err != nil {
		t.Fatalf("FindBoost: %v", err)
	}
	return b, true
}

// favourited reports whether an account favourites a status.
func (e *remoteInteractionEnv) favourited(t *testing.T, accountID, statusID string) bool {
	t.Helper()
	_, err := e.ih.Interactions.FindFavourite(t.Context(), accountID, statusID)
	if err != nil && !errors.Is(err, appcontent.ErrNotFound) {
		t.Fatalf("FindFavourite: %v", err)
	}
	return err == nil
}

// notified returns the notifications of type typ that a local account got from source.
func (e *remoteInteractionEnv) notified(t *testing.T, recipient, source string, typ notification.Type) int {
	t.Helper()
	ns, err := e.notifications.List(t.Context(), recipient, appnotification.Query{Limit: appnotification.MaxPageLimit})
	if err != nil {
		t.Fatalf("List notifications: %v", err)
	}
	n := 0
	for _, x := range ns {
		if x.Type == typ && x.SourceAccountID == source {
			n++
		}
	}
	return n
}

// fixtureDocument returns a federation fixture as it is, with http IRIs made https (the inbox
// takes only https actors; the older fixtures were taken from servers on http) and each pair of
// replace applied.
func fixtureDocument(t *testing.T, implementation, version, name string, replace ...string) []byte {
	t.Helper()
	fixtures, err := FederationFixtures()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(fixtures, func(f FederationFixture) bool {
		return f.Implementation == implementation && f.Version == version && f.Name == name
	})
	if i < 0 {
		t.Fatalf("no fixture %s/%s/%s", implementation, version, name)
	}
	s := strings.ReplaceAll(string(fixtures[i].JSON), `"http://`, `"https://`)
	return []byte(strings.NewReplacer(replace...).Replace(s))
}

// remoteAddressed returns an activity of typ as JSON, addressed to the public and the actor's
// followers as Mastodon addresses a public boost.
func remoteAddressed(t *testing.T, typ, id, actor string, object any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams", "id": id, "type": typ, "actor": actor, "object": object,
		"to": []string{string(federation.PublicIRI)}, "cc": []string{actor + "/followers"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// remoteAnnounce returns a public Announce as JSON.
func remoteAnnounce(t *testing.T, id, actor string, object any) []byte {
	t.Helper()
	return remoteAddressed(t, "Announce", id, actor, object)
}

func remoteInteractsWithLocalPosts(t *testing.T, e *remoteInteractionEnv) {
	author := e.user(t, "author")
	post := e.localPost(t, author, content.VisibilityPublic)
	at := func(uri string) []string { return []string{uri, post.URI} }

	for _, tc := range []struct {
		implementation, version, name string
		actor, object                 string
		visibility                    content.Visibility
	}{
		{"mastodon", "circa-2018", "announce", "https://mastodon.example.org/users/admin", "https://mastodon.example.org/@admin/99541947525187367", content.VisibilityPublic},
		{"gotosocial", "0.22", "announce", "https://gotosocial.example/users/the_mighty_zork", "https://gotosocial.example/users/the_mighty_zork/statuses/01FCTA44PW9H1TB328S9AQXKDS", content.VisibilityPrivate},
	} {
		t.Run("Announce/"+tc.implementation, func(t *testing.T) {
			booster := e.actor(t, tc.actor)
			doc := fixtureDocument(t, tc.implementation, tc.version, tc.name, at(tc.object)...)
			e.expect(t, doc, appfederation.InboxHandled)
			b, ok := e.liveBoost(t, booster, post.ID)
			if !ok || b.IsLocal || b.Visibility != tc.visibility || b.Federates() {
				t.Fatalf("boost = %+v (%t), want a remote %s boost that does not federate", b, ok, tc.visibility)
			}
			if boosts, _ := e.counters(t, post.ID); boosts != 1 {
				t.Errorf("boosts = %d, want 1", boosts)
			}
			if n := e.notified(t, author, booster, notification.TypeBoost); n != 1 {
				t.Errorf("boost notifications = %d, want 1", n)
			}
			// Another Announce of the same post while the boost is live changes nothing.
			again := remoteAnnounce(t, tc.actor+"/announces/again", tc.actor, post.URI)
			e.expect(t, again, appfederation.InboxHandled)
			if got, _ := e.liveBoost(t, booster, post.ID); got.ID != b.ID {
				t.Errorf("boost after a second Announce = %+v, want %+v", got, b)
			}
			// The Undo by the Announce's IRI takes it back, once.
			e.expect(t, remoteActivity(t, "Undo", b.URI+"#undo", tc.actor, b.URI), appfederation.InboxHandled)
			e.expect(t, remoteActivity(t, "Undo", b.URI+"#undo-again", tc.actor, b.URI), appfederation.InboxHandled)
			if _, ok := e.liveBoost(t, booster, post.ID); ok {
				t.Error("boost live after its Undo")
			}
			if r, err := e.ih.Interactions.FindBoostRetraction(t.Context(), b.ID); err != nil || r.Origin != content.RetractedByRemote {
				t.Errorf("retraction = %+v, %v; want one by the remote booster", r, err)
			}
			if boosts, _ := e.counters(t, post.ID); boosts != 0 {
				t.Errorf("boosts after the Undo = %d, want 0", boosts)
			}
		})
	}

	const mastodonPost = "https://remote.example/objects/eb92579d-3417-42a8-8652-2492c2d4f454"
	for _, tc := range []struct {
		name, actor string
		like, undo  []byte
	}{
		{
			"mastodon", "https://mastodon.example.org/users/admin",
			fixtureDocument(t, "mastodon", "circa-2018", "like", at(mastodonPost)...),
			fixtureDocument(t, "mastodon", "circa-2018", "undo-like", at(mastodonPost)...),
		},
		{
			// A Like whose content is no emoji is a favourite, as Misskey and Akkoma take it.
			"content that is no emoji", "https://misskey.example/users/8x8yep20u2",
			likeWithContent(t, "https://misskey.example/likes/plain", "https://misskey.example/users/8x8yep20u2", post.URI, "hello", ""),
			remoteActivity(t, "Undo", "https://misskey.example/likes/plain/undo", "https://misskey.example/users/8x8yep20u2", map[string]any{
				"type": "Like", "id": "https://misskey.example/likes/other", "actor": "https://misskey.example/users/8x8yep20u2", "object": post.URI,
			}),
		},
	} {
		t.Run("Like/"+tc.name, func(t *testing.T) {
			fan := e.actor(t, tc.actor)
			e.expect(t, tc.like, appfederation.InboxHandled)
			if !e.favourited(t, fan, post.ID) {
				t.Fatal("no favourite after the Like")
			}
			if _, favourites := e.counters(t, post.ID); favourites != 1 {
				t.Errorf("favourites = %d, want 1", favourites)
			}
			if n := e.notified(t, author, fan, notification.TypeFavourite); n != 1 {
				t.Errorf("favourite notifications = %d, want 1", n)
			}
			e.expect(t, tc.undo, appfederation.InboxHandled)
			if e.favourited(t, fan, post.ID) {
				t.Error("favourite after the Undo")
			}
			if _, favourites := e.counters(t, post.ID); favourites != 0 {
				t.Errorf("favourites after the Undo = %d, want 0", favourites)
			}
		})
	}

	// A Like of a post this server does not have, and the Undo of a Like it never got (which
	// leaves a tombstone that drops the Like arriving after it).
	const bob = "https://remote.example/users/bob"
	e.actor(t, bob)
	e.expect(t, remoteActivity(t, "Like", bob+"#likes/1", bob, "https://"+contractHost+"/ap/statuses/missing"), appfederation.InboxDropped)
	e.expect(t, remoteActivity(t, "Undo", bob+"#likes/2/undo", bob, bob+"#likes/2"), appfederation.InboxTombstoned)
	e.expect(t, remoteActivity(t, "Like", bob+"#likes/2", bob, post.URI), appfederation.InboxDropped)
	if _, favourites := e.counters(t, post.ID); favourites != 0 {
		t.Errorf("favourites after a Like that was taken back first = %d, want 0", favourites)
	}
}

func remoteBoostsRemotePosts(t *testing.T, e *remoteInteractionEnv) {
	const (
		alice = "https://remote.example/users/alice"
		carol = "https://other.example/users/carol"
	)
	reader := e.user(t, "reader")
	aliceID, carolID := e.actor(t, alice), e.actor(t, carol)
	published := remoteNow.Add(-time.Hour)

	// alice is not followed: her boost of a post this server does not have stores nothing, and
	// fetches nothing.
	unknown := carol + "/notes/1"
	e.fetcher.serve(t, unknown, remoteNote(unknown, carol, published, nil))
	e.expect(t, remoteAnnounce(t, alice+"/announces/1", alice, unknown), appfederation.InboxDropped)
	if e.stored(t, unknown) || slices.Contains(e.fetcher.fetched, unknown) {
		t.Fatalf("%s stored or fetched for a booster no local account follows", unknown)
	}

	e.follow(t, reader, aliceID)
	// A post by its IRI is fetched from its origin.
	announce := remoteAnnounce(t, alice+"/announces/2", alice, unknown)
	var v map[string]any
	if err := json.Unmarshal(announce, &v); err != nil {
		t.Fatal(err)
	}
	v["published"] = remoteNow.Add(-time.Minute).Format(time.RFC3339)
	v["to"], v["cc"] = []string{string(federation.PublicIRI)}, []string{alice + "/followers", carol}
	announce, _ = json.Marshal(v)
	e.expect(t, announce, appfederation.InboxHandled)
	st := e.post(t, unknown).Status
	if st.AuthorAccountID != carolID || st.IsLocal {
		t.Fatalf("boosted post = %+v, want carol's", st)
	}
	b, ok := e.liveBoost(t, aliceID, st.ID)
	if !ok || b.Visibility != content.VisibilityPublic || !b.BoostedAt.Equal(remoteNow.Add(-time.Minute)) {
		t.Fatalf("boost = %+v (%t), want alice's public boost at the Announce's time", b, ok)
	}
	home, err := e.timeline.Home(t.Context(), appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: reader}, apptimeline.Query{Limit: apptimeline.MaxLimit})
	if err != nil || !slices.ContainsFunc(home.Entries, func(en apptimeline.Entry) bool { return en.Kind == timeline.KindBoost && en.ID == b.ID }) {
		t.Errorf("home of alice's follower = %+v, %v; want the boost", home.Entries, err)
	}

	// A post embedded from another origin is fetched again: the embedded copy is not trusted.
	forged := carol + "/notes/2"
	e.fetcher.serve(t, forged, remoteNote(forged, carol, published, map[string]any{"content": "<p>real</p>"}))
	e.expect(t, remoteAnnounce(t, alice+"/announces/3", alice,
		remoteNote(forged, carol, published, map[string]any{"content": "<p>forged</p>"})), appfederation.InboxHandled)
	if rev := e.post(t, forged).Revision; !strings.Contains(rev.Bodies[0].ContentHTML, "real") {
		t.Errorf("boosted post embedded from another origin = %q, want the one its origin serves", rev.Bodies[0].ContentHTML)
	}

	// A post embedded from the booster's origin is taken as it is.
	own := alice + "/notes/1"
	e.expect(t, remoteAnnounce(t, alice+"/announces/4", alice,
		remoteNote(own, alice, published, map[string]any{"content": "<p>embedded</p>"})), appfederation.InboxHandled)
	if slices.Contains(e.fetcher.fetched, own) {
		t.Errorf("%s fetched although the Announce embeds it from the booster's origin", own)
	}
	if rev := e.post(t, own).Revision; !strings.Contains(rev.Bodies[0].ContentHTML, "embedded") {
		t.Errorf("boosted own post = %q, want the embedded one", rev.Bodies[0].ContentHTML)
	}

	// An Announce to the followers only makes a followers-only boost of a public post; one of
	// someone else's followers-only post is refused.
	toFollowers := carol + "/notes/3"
	e.fetcher.serve(t, toFollowers, remoteNote(toFollowers, carol, published, nil))
	doc := remoteAnnounce(t, alice+"/announces/5", alice, toFollowers)
	if err := json.Unmarshal(doc, &v); err != nil {
		t.Fatal(err)
	}
	v["to"] = []string{alice + "/followers"}
	doc, _ = json.Marshal(v)
	e.expect(t, doc, appfederation.InboxHandled)
	if b, ok := e.liveBoost(t, aliceID, e.post(t, toFollowers).Status.ID); !ok || b.Visibility != content.VisibilityPrivate {
		t.Errorf("boost addressed to the followers = %+v (%t), want private", b, ok)
	}
	private := carol + "/notes/4"
	e.fetcher.serve(t, private, remoteNote(private, carol, published, map[string]any{"to": []string{carol + "/followers"}, "cc": []string{}}))
	e.expect(t, remoteAnnounce(t, alice+"/announces/6", alice, private), appfederation.InboxDropped)
	if st := e.post(t, private).Status; st.Visibility != content.VisibilityPrivate {
		t.Errorf("visibility = %s, want private", st.Visibility)
	} else if _, ok := e.liveBoost(t, aliceID, st.ID); ok {
		t.Error("boost of someone else's followers-only post")
	}
}

func remoteRefusesBoosts(t *testing.T, e *remoteInteractionEnv) {
	const alice = "https://remote.example/users/alice"
	author := e.user(t, "author")
	aliceID := e.actor(t, alice)
	localOnly := e.localPost(t, author, content.VisibilityLocal)
	private := e.localPost(t, author, content.VisibilityPrivate)

	// A local-only post stays on this server: remote accounts do not boost it (ADR-0017), even
	// those that follow its author.
	e.expect(t, remoteAnnounce(t, alice+"/announces/1", alice, localOnly.URI), appfederation.InboxDropped)
	e.expect(t, remoteAnnounce(t, alice+"/announces/2", alice, private.URI), appfederation.InboxDropped)
	// Nor does a remote account favourite a followers-only post it cannot see.
	e.expect(t, remoteActivity(t, "Like", alice+"#likes/1", alice, private.URI), appfederation.InboxDropped)
	for _, st := range []content.Status{localOnly, private} {
		if _, ok := e.liveBoost(t, aliceID, st.ID); ok {
			t.Errorf("boost of a %s post by a remote account", st.Visibility)
		}
	}
	if e.favourited(t, aliceID, private.ID) {
		t.Error("favourite of a followers-only post by a remote account that does not follow its author")
	}

	// A local account's boost of a local-only post records no activity (ADR-0017); its boost of
	// a public post records the Announce.
	booster := e.user(t, "booster")
	b, err := e.interactions.Boost(t.Context(), booster, localOnly.ID)
	if err != nil {
		t.Fatalf("Boost(local) = %v", err)
	}
	if got, err := e.ih.Federation.ListOutboundActivitiesOf(t.Context(), federation.OutboundBoost, b.ID); err != nil || len(got) != 0 {
		t.Errorf("activities of the boost of a local-only post = %+v, %v; want none", got, err)
	}
	public := e.localPost(t, author, content.VisibilityPublic)
	b, err = e.interactions.Boost(t.Context(), booster, public.ID)
	if err != nil {
		t.Fatalf("Boost(public) = %v", err)
	}
	if got, err := e.ih.Federation.ListOutboundActivitiesOf(t.Context(), federation.OutboundBoost, b.ID); err != nil || len(got) != 1 || got[0].Type != federation.ActivityAnnounce {
		t.Errorf("activities of the boost of a public post = %+v, %v; want the Announce", got, err)
	}
}

func remoteSendsLikes(t *testing.T, e *remoteInteractionEnv) {
	const alice = "https://remote.example/users/alice"
	fan := e.user(t, "fan")
	aliceID := e.actor(t, alice)
	e.follow(t, fan, aliceID)
	id := alice + "/notes/1"
	e.expect(t, remoteActivity(t, "Create", id+"/activity", alice, remoteNote(id, alice, remoteNow.Add(-time.Hour), nil)), appfederation.InboxHandled)
	st := e.post(t, id).Status

	f, err := e.interactions.Favourite(t.Context(), fan, st.ID)
	if err != nil {
		t.Fatalf("Favourite = %v", err)
	}
	e.clock.Advance(time.Minute)
	if err := e.interactions.Unfavourite(t.Context(), fan, st.ID); err != nil {
		t.Fatalf("Unfavourite = %v", err)
	}
	sent := e.sent(t, st.ID)
	if len(sent) != 2 || sent[0].Type != federation.ActivityLike || sent[1].Type != federation.ActivityUndo {
		t.Fatalf("activities = %+v, want a Like and its Undo", sent)
	}
	fanIRI := federation.IRI(e.localIRI(fan))
	for _, s := range sent {
		if !slices.Equal(s.Inboxes, []string{alice + "/inbox"}) {
			t.Errorf("%s inboxes = %v, want the author's", s.Type, s.Inboxes)
		}
		// The documents read back as what they are.
		doc, err := appfederation.Normalize(s.Raw, e.clock.Now())
		if err != nil || doc.Activity == nil || doc.Activity.Actor != fanIRI {
			t.Fatalf("Normalize(%s) = %+v, %v; want an activity of the fan", s.Raw, doc, err)
		}
		like := doc.Activity
		if s.Type == federation.ActivityUndo {
			embedded := like.Object().Untrusted()
			if embedded == nil || embedded.Activity == nil {
				t.Fatalf("Undo %s does not embed the Like", s.Raw)
			}
			like = embedded.Activity
		}
		if like.Type != federation.ActivityLike || like.ID != federation.IRI(f.URI) || like.Object().ID != federation.IRI(st.URI) {
			t.Errorf("Like = %+v, want %s of %s", like, f.URI, st.URI)
		}
	}
	// A favourite of a local post sends nothing.
	local := e.localPost(t, e.user(t, "author"), content.VisibilityPublic)
	if _, err := e.interactions.Favourite(t.Context(), fan, local.ID); err != nil {
		t.Fatalf("Favourite(local) = %v", err)
	}
	if got := e.sent(t, local.ID); len(got) != 0 {
		t.Errorf("activities of the favourite of a local post = %+v, want none", got)
	}
}

// remoteSendsReactions checks the Like and Undo Like of a local account's emoji reactions to a
// remote post (ADR-0052 §3): each Like carries the emoji as its content and _misskey_reaction,
// with the Emoji tag of a custom emoji, and filling the Like slot sends the Undo of what was in it
// before the new Like. Read back, each is the reaction it sends.
func remoteSendsReactions(t *testing.T, e *remoteInteractionEnv) {
	ctx := t.Context()
	const alice = "https://remote.example/users/alice"
	fan := e.user(t, "fan")
	aliceID := e.actor(t, alice)
	e.follow(t, fan, aliceID)
	id := alice + "/notes/1"
	e.expect(t, remoteActivity(t, "Create", id+"/activity", alice, remoteNote(id, alice, remoteNow.Add(-time.Hour), nil)), appfederation.InboxHandled)
	st := e.post(t, id).Status
	var blobcat content.CustomEmoji
	err := e.ih.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		blobcat, err = e.ih.Statuses.EnsureCustomEmoji(ctx, tx, content.CustomEmoji{ID: e.ids.NewID(), Shortcode: "blobcat", CreatedAt: e.clock.Now()})
		return err
	})
	if err != nil {
		t.Fatalf("store a local custom emoji: %v", err)
	}
	e.emojiTags[blobcat.ID] = appcontent.EmojiTag{
		ID: blobcat.ID, Shortcode: "blobcat", Local: true, Updated: e.clock.Now(), MediaType: "image/png",
		URL: "https://" + contractHost + "/media/blobcat.png",
	}

	steps := []struct {
		name string
		run  func() error
	}{
		{"react", func() error { _, err := e.interactions.React(ctx, fan, st.ID, "👍"); return err }},
		{"react with a custom emoji", func() error { _, err := e.interactions.React(ctx, fan, st.ID, ":blobcat:"); return err }},
		{"favourite", func() error { _, err := e.interactions.Favourite(ctx, fan, st.ID); return err }},
		{"react again", func() error { _, err := e.interactions.React(ctx, fan, st.ID, "🎉"); return err }},
		{"unreact", func() error { return e.interactions.Unreact(ctx, fan, st.ID) }},
	}
	for _, s := range steps {
		e.clock.Advance(time.Minute)
		if err := s.run(); err != nil {
			t.Fatalf("%s = %v", s.name, err)
		}
	}
	// What each activity says: the emoji of a Like ("" for a favourite), or of the Like an Undo
	// takes back.
	want := []string{"Like 👍", "Undo 👍", "Like :blobcat:", "Undo :blobcat:", "Like ", "Undo ", "Like 🎉", "Undo 🎉"}
	var got []string
	actor := federation.IRI(e.localIRI(fan))
	for _, s := range e.sent(t, st.ID) {
		if !slices.Equal(s.Inboxes, []string{alice + "/inbox"}) {
			t.Errorf("%s inboxes = %v, want the author's", s.Type, s.Inboxes)
		}
		doc, err := appfederation.Normalize(s.Raw, e.clock.Now())
		if err != nil || doc.Activity == nil || doc.Activity.Actor != actor {
			t.Fatalf("Normalize(%s) = %+v, %v; want an activity of the fan", s.Raw, doc, err)
		}
		like := doc.Activity
		if s.Type == federation.ActivityUndo {
			embedded := like.Object().Untrusted()
			if embedded == nil || embedded.Activity == nil {
				t.Fatalf("Undo %s does not embed the Like", s.Raw)
			}
			like = embedded.Activity
		}
		if like.Type != federation.ActivityLike || like.Object().ID != federation.IRI(st.URI) {
			t.Errorf("Like = %+v, want one of %s", like, st.URI)
		}
		got = append(got, string(s.Type)+" "+like.Reaction)
		if like.Reaction == ":blobcat:" {
			if len(like.Emojis) != 1 || like.Emojis[0].ID != federation.IRI(appcontent.CustomEmojiURI(contractHost, blobcat.ID)) ||
				like.Emojis[0].IconURL == "" {
				t.Errorf("Emoji tags of %s = %+v, want blobcat with its image", s.Type, like.Emojis)
			}
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("activities = %q, want %q", got, want)
	}
	// A reaction to a local post sends nothing.
	local := e.localPost(t, e.user(t, "author"), content.VisibilityPublic)
	if _, err := e.interactions.React(ctx, fan, local.ID, "👍"); err != nil {
		t.Fatalf("React(local) = %v", err)
	}
	if got := e.sent(t, local.ID); len(got) != 0 {
		t.Errorf("activities of the reaction to a local post = %+v, want none", got)
	}
}

type remoteInteractionSent struct {
	Type    federation.ActivityType
	Raw     []byte
	Inboxes []string
}

// sent runs the pending fan-out jobs of boosts and favourites and returns the activities
// recorded about the favourites of a status, oldest first, with their documents and inboxes.
func (e *remoteInteractionEnv) sent(t *testing.T, statusID string) []remoteInteractionSent {
	t.Helper()
	ctx := t.Context()
	fanOut, _, ok := e.handlers.Lookup(appfederation.JobFanOut)
	if !ok {
		t.Fatal("no handler for " + appfederation.JobFanOut)
	}
	for {
		nj, ok := e.jobs.pop()
		if !ok {
			break
		}
		if nj.Type != appfederation.JobFanOut {
			continue
		}
		var p struct {
			ID string `json:"outbound_activity_id"`
		}
		if err := json.Unmarshal(nj.Payload, &p); err != nil {
			t.Fatal(err)
		}
		act, err := e.ih.Federation.FindOutboundActivity(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if act.ObjectKind != federation.OutboundFavourite && act.ObjectKind != federation.OutboundBoost {
			continue // the Create of a local post
		}
		j, err := nj.Build(e.ids.NewID(), e.clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		if r := fanOut(ctx, j); !r.IsDone() {
			t.Fatalf("fan-out %s = %+v (%v), want Done", j.Payload, r, r.Err())
		}
	}
	acts, err := e.ih.Federation.ListOutboundActivitiesOf(ctx, federation.OutboundFavourite, statusID)
	if err != nil {
		t.Fatalf("ListOutboundActivitiesOf: %v", err)
	}
	out := []remoteInteractionSent{}
	for _, a := range acts {
		s := remoteInteractionSent{Type: a.Type}
		if s.Raw, err = e.ih.Federation.FindOutboundDocument(ctx, a.ID); err != nil {
			t.Fatalf("FindOutboundDocument(%s): %v", a.ID, err)
		}
		ds, err := e.ih.Federation.ListDeliveries(ctx, a.ID, "", 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range ds {
			s.Inboxes = append(s.Inboxes, string(d.InboxURL))
		}
		out = append(out, s)
	}
	return out
}

// remoteInteractionOrder processes an Announce and a Like with their Undo, and their repeats, in
// every order of inboxSequences, each on a new database: whatever the order, the boost and the
// favourite end up taken back and the counters at zero (NFR-REL-1).
func remoteInteractionOrder(t *testing.T, newHarness RemoteInteractionFactory) {
	const alice = "https://remote.example/users/alice"
	for _, kind := range []string{"Announce", "Like", "EmojiReact"} {
		t.Run(kind, func(t *testing.T) {
			for _, seq := range inboxSequences(2) {
				e := newRemoteInteractionEnv(t, newHarness)
				post := e.localPost(t, e.user(t, "author"), content.VisibilityPublic)
				aliceID := e.actor(t, alice)
				act := alice + "/" + strings.ToLower(kind) + "s/1"
				doc := remoteAddressed(t, kind, act, alice, post.URI)
				if kind == "EmojiReact" {
					doc = likeWithContent(t, act, alice, post.URI, "👍", "EmojiReact")
				}
				docs := [][]byte{
					doc,
					remoteActivity(t, "Undo", act+"#undo", alice, act),
				}
				uris := make([]federation.IRI, len(docs))
				for i, doc := range docs {
					uris[i] = e.record(t, doc)
				}
				for _, i := range seq {
					if _, err := e.processor.Process(t.Context(), uris[i], federation.TrustSigned); err != nil {
						t.Fatalf("order %v: Process(%s) = %v", seq, uris[i], err)
					}
				}
				boosts, favourites := e.counters(t, post.ID)
				_, boosted := e.liveBoost(t, aliceID, post.ID)
				if got := e.reactionCounts(t, post.ID); len(got) != 0 {
					t.Errorf("order %v: reaction counts %v; want none", seq, got)
				}
				if boosted || e.favourited(t, aliceID, post.ID) || boosts != 0 || favourites != 0 {
					t.Errorf("order %v: boosted %t, favourited %t, counters %d/%d; want everything taken back",
						seq, boosted, e.favourited(t, aliceID, post.ID), boosts, favourites)
				}
			}
		})
	}
}

// likeWithContent returns a Like (or an activity of typ) of object by actor with content and,
// unless it is "", _misskey_reaction.
func likeWithContent(t *testing.T, id, actor, object, text, typ string) []byte {
	t.Helper()
	if typ == "" {
		typ = "Like"
	}
	v := map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams", "id": id, "type": typ, "actor": actor, "object": object,
		"content": text,
	}
	if typ == "Like" && text != "" && text != "hello" {
		v["_misskey_reaction"] = text
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// reactionCounts returns the reaction counts of a status by the name of their emoji: the
// Unicode emoji, or "shortcode@domain" of a custom emoji.
func (e *remoteInteractionEnv) reactionCounts(t *testing.T, statusID string) map[string]int64 {
	t.Helper()
	counts, err := e.ih.Interactions.ListReactionCounts(t.Context(), statusID, content.MaxStatusReactions)
	if err != nil {
		t.Fatalf("ListReactionCounts: %v", err)
	}
	out := map[string]int64{}
	for _, c := range counts {
		out[e.emojiName(t, c.Emoji)] = c.Count
	}
	return out
}

// emojiName returns the Unicode emoji, or "shortcode@domain" of a custom emoji.
func (e *remoteInteractionEnv) emojiName(t *testing.T, emoji content.ReactionEmoji) string {
	t.Helper()
	if emoji.CustomEmojiID == "" {
		return emoji.Unicode
	}
	c, err := e.h.Statuses.FindCustomEmoji(t.Context(), emoji.CustomEmojiID)
	if err != nil {
		t.Fatalf("FindCustomEmoji: %v", err)
	}
	return c.Shortcode + "@" + c.Domain
}

// reactionsOf returns the names of the emoji that an account reacts to a status with.
func (e *remoteInteractionEnv) reactionsOf(t *testing.T, accountID, statusID string) []string {
	t.Helper()
	rs, err := e.ih.Interactions.ListAccountStatusReactions(t.Context(), accountID, statusID)
	if err != nil {
		t.Fatalf("ListAccountStatusReactions: %v", err)
	}
	var out []string
	for _, r := range rs {
		out = append(out, e.emojiName(t, r.Emoji))
	}
	slices.Sort(out)
	return out
}

// reactionNotifications returns the emoji of the reaction notifications that a local account
// got from source, sorted.
func (e *remoteInteractionEnv) reactionNotifications(t *testing.T, recipient, source string) []string {
	t.Helper()
	ns, err := e.notifications.List(t.Context(), recipient, appnotification.Query{Limit: appnotification.MaxPageLimit})
	if err != nil {
		t.Fatalf("List notifications: %v", err)
	}
	var out []string
	for _, n := range ns {
		if n.Type == notification.TypeReaction && n.SourceAccountID == source {
			out = append(out, e.emojiName(t, content.ReactionEmoji{Unicode: n.ReactionEmoji, CustomEmojiID: n.ReactionCustomEmojiID}))
		}
	}
	slices.Sort(out)
	return out
}

func remoteReactsToLocalPosts(t *testing.T, e *remoteInteractionEnv) {
	author := e.user(t, "author")
	post := e.localPost(t, author, content.VisibilityPublic)
	at := func(uri string) []string { return []string{uri, post.URI} }
	check := func(t *testing.T, accountID string, want map[string]int64, mine ...string) {
		t.Helper()
		if got := e.reactionCounts(t, post.ID); !maps.Equal(got, want) {
			t.Errorf("reaction counts = %v, want %v", got, want)
		}
		if got := e.reactionsOf(t, accountID, post.ID); !slices.Equal(got, mine) {
			t.Errorf("reactions of the actor = %v, want %v", got, mine)
		}
		if _, favourites := e.counters(t, post.ID); favourites != 0 || e.favourited(t, accountID, post.ID) {
			t.Errorf("favourites = %d, want reactions only", favourites)
		}
	}

	t.Run("Misskey", func(t *testing.T) {
		const actor = "https://misskey.example/users/asfo7no84zv70002"
		fan := e.actor(t, actor)
		like := fixtureDocument(t, "misskey", "2026.10", "like-reaction", at("https://remote.example/ap/objects/01a148e2-4400-7a88-ba6a-6710b1274540")...)
		e.expect(t, like, appfederation.InboxHandled)
		check(t, fan, map[string]int64{"👍": 1}, "👍")
		if got := e.reactionNotifications(t, author, fan); !slices.Equal(got, []string{"👍"}) {
			t.Errorf("reaction notifications = %v, want one", got)
		}
		// The same Like again changes nothing.
		e.expect(t, like, appfederation.InboxHandled)
		check(t, fan, map[string]int64{"👍": 1}, "👍")

		// A custom emoji of the sender's server (circa 2022, with a colon-wrapped name).
		const old = "https://misskey.example/users/8x8yep20u2"
		oldFan := e.actor(t, old)
		e.expect(t, fixtureDocument(t, "misskey", "circa-2022", "like-reaction", at("https://remote.example/objects/89937a53-2692-4631-bb62-770091267391")...), appfederation.InboxHandled)
		check(t, oldFan, map[string]int64{"👍": 1, "hanapog@misskey.example": 1}, "hanapog@misskey.example")

		// Misskey takes a reaction back with an Undo that embeds the Like.
		e.expect(t, remoteActivity(t, "Undo", "https://misskey.example/likes/asfoaqef4zv7000t/undo", actor, map[string]any{
			"type": "Like", "id": "https://misskey.example/likes/asfoaqef4zv7000t", "actor": actor, "object": post.URI,
			"content": "👍", "_misskey_reaction": "👍",
		}), appfederation.InboxHandled)
		check(t, fan, map[string]int64{"hanapog@misskey.example": 1})
		// An Undo whose id is unknown takes back the reaction that it embeds, by its post and emoji.
		e.expect(t, remoteActivity(t, "Undo", "https://misskey.example/likes/unknown/undo", old, map[string]any{
			"type": "Like", "id": "https://misskey.example/likes/unknown", "actor": old, "object": post.URI,
			"content": ":hanapog:", "_misskey_reaction": ":hanapog:",
			"tag": []any{map[string]any{
				"id": "https://misskey.example/emojis/hanapog", "type": "Emoji", "name": ":hanapog:",
				"icon": map[string]any{"type": "Image", "url": "https://misskey.example/files/webpublic-8f8a9768-7264-4171-88d6-2356aabeadcd"},
			}},
		}), appfederation.InboxHandled)
		check(t, oldFan, map[string]int64{})
	})

	t.Run("Akkoma", func(t *testing.T) {
		const actor = "https://example.org/users/akko"
		fan := e.actor(t, actor)
		object := at("https://remote.example/objects/9f0e93499d8314a9")
		for _, name := range []string{"emoji-react", "emoji-react-custom", "emoji-react-remote"} {
			e.expect(t, fixtureDocument(t, "akkoma", "3.20", name, object...), appfederation.InboxHandled)
		}
		// Unicode, its own server's emoji (an Emoji without an id), and a third server's.
		mine := []string{"hug@other.example", "mouse@example.org", "🧡"}
		check(t, fan, map[string]int64{"🧡": 1, "mouse@example.org": 1, "hug@other.example": 1}, mine...)
		if got := e.reactionNotifications(t, author, fan); !slices.Equal(got, mine) {
			t.Errorf("reaction notifications = %v, want one per emoji", got)
		}
		// Akkoma takes a reaction back by its IRI alone.
		e.expect(t, fixtureDocument(t, "akkoma", "3.20", "undo-emoji-react"), appfederation.InboxHandled)
		check(t, fan, map[string]int64{"mouse@example.org": 1, "hug@other.example": 1}, "hug@other.example", "mouse@example.org")
		// Reacting again with the same emoji does not notify again.
		e.expect(t, likeWithContent(t, actor+"#react/again", actor, post.URI, "🧡", "EmojiReact"), appfederation.InboxHandled)
		if got := e.reactionNotifications(t, author, fan); !slices.Equal(got, mine) {
			t.Errorf("reaction notifications after reacting again = %v, want %v", got, mine)
		}
	})

	t.Run("Normalized", func(t *testing.T) {
		// The text and emoji presentations of the same emoji count together.
		other := e.localPost(t, author, content.VisibilityPublic)
		for i, text := range []string{"\u2764", "\u2764\uFE0F"} {
			actor := fmt.Sprintf("https://remote.example/users/heart%d", i)
			e.actor(t, actor)
			e.expect(t, likeWithContent(t, actor+"/likes/1", actor, other.URI, text, ""), appfederation.InboxHandled)
		}
		if got := e.reactionCounts(t, other.ID); !maps.Equal(got, map[string]int64{"\u2764\uFE0F": 2}) {
			t.Errorf("reaction counts = %v, want two of the fully-qualified heart", got)
		}
	})
}

func remoteRefusesReactions(t *testing.T, e *remoteInteractionEnv) {
	const alice = "https://remote.example/users/alice"
	author := e.user(t, "author")
	aliceID := e.actor(t, alice)
	public := e.localPost(t, author, content.VisibilityPublic)
	private := e.localPost(t, author, content.VisibilityPrivate)

	// An EmojiReact with no emoji is dropped; a Like with no emoji is a favourite.
	e.expect(t, likeWithContent(t, alice+"/react/1", alice, public.URI, "hello", "EmojiReact"), appfederation.InboxDropped)
	// A custom emoji without its Emoji tag is no reaction.
	e.expect(t, likeWithContent(t, alice+"/react/2", alice, public.URI, ":blobcat:", "EmojiReact"), appfederation.InboxDropped)
	// Nor does a remote account react to a followers-only post it cannot see.
	e.expect(t, likeWithContent(t, alice+"/react/3", alice, private.URI, "👍", "EmojiReact"), appfederation.InboxDropped)
	for _, st := range []content.Status{public, private} {
		if got := e.reactionsOf(t, aliceID, st.ID); len(got) != 0 {
			t.Errorf("reactions to the %s post = %v, want none", st.Visibility, got)
		}
	}
	// A blocked account's reaction is dropped.
	ctx := t.Context()
	if _, err := e.graph.Block(ctx, author, aliceID); err != nil {
		t.Fatalf("Block: %v", err)
	}
	e.expect(t, likeWithContent(t, alice+"/react/4", alice, public.URI, "👍", "EmojiReact"), appfederation.InboxDropped)
	if got := e.reactionCounts(t, public.ID); len(got) != 0 {
		t.Errorf("reaction counts after a blocked account reacted = %v, want none", got)
	}
}
