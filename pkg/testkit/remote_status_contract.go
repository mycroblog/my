// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/kernel"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/domain/notification"
)

// RemoteStatusHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the identity, account, graph, content, timeline, notification, inbox, and
// federation repositories on it.
type RemoteStatusHarness struct {
	AccountHarness
	Graph         appgraph.Repository
	Statuses      appcontent.Repository
	Media         appcontent.MediaRepository
	Timeline      apptimeline.Repository
	Notifications appnotification.Repository
	Inbox         appfederation.InboxRepository
	// Federation and Instances serve the fetches of remote posts and threads.
	Federation appfederation.Repository
	Instances  appfederation.InstanceRepository
	// Pins serves the pins that the featured collections of remote accounts become; without it,
	// the cases of featured collections are skipped.
	Pins appcontent.InteractionRepository
	// Quotes stores the quotes of posts (ADR-0069); without it, the cases of quotes are skipped.
	Quotes appcontent.QuoteRepository
}

// RemoteStatusFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type RemoteStatusFactory func(t *testing.T, opts uow.Options) RemoteStatusHarness

// RemoteStatusContract verifies the ingestion of remote posts (federation.md §3.3, §3.4,
// §6.3, ADR-0034) on a database adapter, through the inbox processor with the real Create, Update,
// and Delete handlers and the real content, account, graph, timeline, and notification modules:
// the Notes of the major implementations (the federation fixtures) are ingested with IDs whose
// time is their published time; Creates unrelated to a local account are not stored, while
// follows, mentions, addressing, and replies into local threads bring posts in; the declared
// attachments, custom emoji, hashtags, mentions, and polls are recorded and the HTML sanitized;
// an old Update makes no revision and a recent Update of an unknown post is its Create; Deletes
// leave tombstones; and the timeline and notification projectors see remote posts. The parent of a
// reply is fetched one level; an opened thread fetches its ancestors and replies within
// ThreadDepth and ThreadMaxItems; fetched posts must be attributed within their origin; failed
// fetches are not repeated; Resolver resolves handles and the URLs of accounts and posts; the
// featured collections of fetched actors become their pins, fetched again on an Add or Remove to
// them by their owner (ADR-0044); the quotes of posts are recorded, their quoted posts fetched one
// level, and their approvals verified, revoked, and presented per viewer (ADR-0069); and a
// post with NUL in every string is stored with U+FFFD (federation.md §4). The
// actors and the fetches are in-memory stand-ins, and the network of the fetch cases is in memory.
func RemoteStatusContract(t *testing.T, newHarness RemoteStatusFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *remoteStatusEnv)
	}{
		{"IngestsNotesOfMajorImplementations", remoteIngestsFixtures},
		{"StoresOnlyCreatesRelatedToLocalAccounts", remoteScope},
		{"RecordsTheDeclaredParts", remoteRecordsParts},
		{"KeepsTheAuthorAndTheOrigin", remoteChecksAuthor},
		{"TakesTheAddressingOfTheActivityOnlyFromTheAuthor", remoteTakesAddressingFromTheAuthor},
		{"RevisesOnlyWithNewerUpdates", remoteRevises},
		{"TakesARecentUpdateAsItsCreate", remoteUpdateAsCreate},
		{"DeletesIntoATombstone", remoteDeletes},
		{"FeedsTimelinesAndNotifications", remoteFeeds},
		{"FetchesTheParentOfAReplyOneLevel", remoteFetchesParents},
		{"RejectsFetchedPostsOfAnotherOrigin", remoteRejectsForeignAttribution},
		{"FetchesThreadsWithinBounds", remoteFetchesThreadsWithinBounds},
		{"RemembersFailedFetches", remoteRemembersFailedFetches},
		{"ResolvesHandlesAndURLsOfAccountsAndPosts", remoteResolves},
		{"FetchesTheRecentPostsOfOutboxesOfMajorImplementations", remoteFetchesOutboxFixtures},
		{"FetchesOutboxesWithinBounds", remoteFetchesOutboxesWithinBounds},
		{"StartsOutboxFetchesOnlyWhenAllowed", remoteStartsOutboxFetches},
		{"PinsTheFeaturedPostsOfMajorImplementations", remotePinsFeaturedFixtures},
		{"MirrorsFeaturedCollectionsWithinBounds", remoteMirrorsFeaturedWithinBounds},
		{"AppliesAddAndRemoveOfFeaturedCollections", remoteAppliesFeaturedChanges},
		{"StoresStringsWithNUL", remoteStatusStoresStringsWithNUL},
		{"RefetchesStoredPostsOnRequest", remoteRefetches},
		{"RecordsTheQuotesOfMajorImplementations", remoteRecordsQuoteFixtures},
		{"FetchesQuotedPostsAndVerifiesApprovals", remoteVerifiesQuotes},
		{"RevokesAndDetachesQuotes", remoteRevokesAndDetachesQuotes},
		{"PresentsQuotesPerViewer", remotePresentsQuotes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newRemoteStatusEnv(t, newHarness)) })
	}
}

type remoteStatusEnv struct {
	*accountEnv
	h             RemoteStatusHarness
	remote        *appaccount.Service // with a sanitizer, for storing remote actors
	actors        *remoteStatusActors
	fetcher       *inboxFetcher
	graph         *appgraph.Service
	content       *appcontent.Service
	timeline      *apptimeline.Service
	notifications *appnotification.Service
	processor     *appfederation.InboxProcessor
	statuses      *appfederation.StatusHandlers
	// votes records the votes in local polls once an environment with interactions sets them.
	votes *lateVotes
	// projectors are the projectors of the harness's UnitOfWork.
	projectors *uow.Projectors
}

// lateVotes records the remote votes in local polls through target, which an environment with
// the interactions service sets after the status handlers exist; without it, no status has a
// poll to vote in.
type lateVotes struct{ target appfederation.RemotePollVotes }

func (v *lateVotes) RecordRemotePollVote(ctx context.Context, in appcontent.RemotePollVoteInput) (bool, error) {
	if v.target == nil {
		return false, appcontent.ErrNoPoll
	}
	return v.target.RecordRemotePollVote(ctx, in)
}

func newRemoteStatusEnv(t *testing.T, newHarness RemoteStatusFactory) *remoteStatusEnv {
	t.Helper()
	var (
		h        RemoteStatusHarness
		captured uow.Options
	)
	ae := newAccountEnv(t, func(t *testing.T, opts uow.Options) AccountHarness {
		captured = opts
		h = newHarness(t, opts)
		return h.AccountHarness
	})
	ae.clock.Set(remoteNow)
	e := &remoteStatusEnv{accountEnv: ae, h: h, projectors: captured.Projectors}
	e.remote = appaccount.NewService(appaccount.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Accounts, Clock: ae.clock, IDs: ae.ids,
		Sealer: kernel.NewSealer(NewKeyManager("remote-status")), Host: contractHost, Sanitizer: remoteSanitizer{},
	})
	e.graph = appgraph.NewService(appgraph.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Graph, Accounts: ae.accounts, Clock: ae.clock, IDs: ae.ids, Host: contractHost,
	})
	appgraph.RegisterProjectors(captured.Projectors, h.Graph)
	appcontent.RegisterProjectors(captured.Projectors, h.Statuses)
	if h.Quotes != nil {
		appcontent.RegisterQuoteProjectors(captured.Projectors, h.Quotes)
	}
	apptimeline.RegisterProjectors(captured.Projectors, h.Timeline)
	appnotification.RegisterProjectors(captured.Projectors, appnotification.ProjectorDeps{
		Repository: h.Notifications, Accounts: ae.accounts, Relationships: e.graph, IDs: ae.ids,
	})
	media := appcontent.NewMediaService(appcontent.MediaDeps{
		UnitOfWork: h.UnitOfWork, Repository: h.Media, Clock: ae.clock, IDs: ae.ids,
	})
	e.content = appcontent.NewService(appcontent.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Statuses, Accounts: ae.accounts, Relationships: e.graph,
		MaxStatusLength: func(context.Context) (int, error) { return 500, nil },
		Sanitizer:       remoteSanitizer{}, RemoteMedia: media, Quotes: h.Quotes,
		Clock: ae.clock, IDs: ae.ids, Host: contractHost,
	})
	e.timeline = apptimeline.NewService(apptimeline.Deps{Repository: h.Timeline, Clock: ae.clock})
	e.notifications = appnotification.NewService(appnotification.Deps{UnitOfWork: h.UnitOfWork, Repository: h.Notifications, Clock: ae.clock})
	e.actors = &remoteStatusActors{accounts: e.remote, served: map[string]appaccount.RemoteActorInput{}}
	e.fetcher = &inboxFetcher{clock: ae.clock, docs: map[string][]byte{}, errs: map[string]error{}}
	e.processor = appfederation.NewInboxProcessor(appfederation.InboxProcessorDeps{
		UnitOfWork: h.UnitOfWork, Repository: h.Inbox, Actors: e.actors, Fetcher: e.fetcher, Clock: ae.clock,
	})
	e.votes = &lateVotes{}
	e.statuses = appfederation.NewStatusHandlers(appfederation.StatusHandlersDeps{
		UnitOfWork: h.UnitOfWork, Statuses: e.content, Accounts: e.remote, Actors: e.actors, Fetcher: e.fetcher,
		Followers: e.graph, Votes: e.votes, Clock: ae.clock, Host: contractHost,
	})
	e.statuses.Register(e.processor)
	return e
}

// remoteNow is the time of the remote status tests: the posts they make are published shortly
// before it.
var remoteNow = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

// remoteStatusActors resolves the actors that the test serves, storing them on first use as
// RemoteActors does; any other actor cannot be fetched.
type remoteStatusActors struct {
	accounts *appaccount.Service
	mu       sync.Mutex
	served   map[string]appaccount.RemoteActorInput
	resolved []string
}

func (a *remoteStatusActors) Resolve(ctx context.Context, uri string) (appaccount.Actor, error) {
	a.mu.Lock()
	in, ok := a.served[uri]
	a.resolved = append(a.resolved, uri)
	a.mu.Unlock()
	if actor, err := a.accounts.FindByURI(ctx, uri); err == nil {
		return actor, nil
	}
	if !ok {
		return appaccount.Actor{}, fmt.Errorf("%w: %s", appfederation.ErrFetchNotFound, uri)
	}
	return a.accounts.SaveRemoteActor(ctx, in)
}

// serve makes the actor uri resolvable, with its followers collection at uri/followers.
func (a *remoteStatusActors) serve(uri string) {
	iri := federation.IRI(uri)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.served[uri] = appaccount.RemoteActorInput{
		URI: uri, Domain: iri.Host(), Type: "Person", PreferredUsername: uri[strings.LastIndexByte(uri, '/')+1:],
		InboxURL: uri + "/inbox", FollowersURL: uri + "/followers", Document: []byte(`{"id":"` + uri + `"}`),
	}
}

// actor stores the remote actor uri and returns its account ID.
func (e *remoteStatusEnv) actor(t *testing.T, uri string) string {
	t.Helper()
	e.actors.serve(uri)
	a, err := e.actors.Resolve(t.Context(), uri)
	if err != nil {
		t.Fatalf("Resolve(%s) = %v", uri, err)
	}
	return a.Account.ID
}

// user registers a local account and returns its ID.
func (e *remoteStatusEnv) user(t *testing.T, username string) string {
	t.Helper()
	return e.register(t, username).AccountID
}

func (e *remoteStatusEnv) localIRI(accountID string) string {
	return account.ActorURI(contractHost, accountID)
}

// follow makes the local account follower an active follower of the remote account followee, as
// an accepted Follow would.
func (e *remoteStatusEnv) follow(t *testing.T, follower, followee string) {
	t.Helper()
	ctx := t.Context()
	f := graph.Follow{
		ID: e.ids.NewID(), FollowerAccountID: follower, FolloweeAccountID: followee, RequestedAt: e.clock.Now(),
	}
	f.URI = graph.FollowURI(contractHost, f.ID)
	prefs := graph.DefaultPreferences(f.ID)
	acc := graph.Acceptance{FollowID: f.ID, AcceptedBy: graph.AcceptedByRemote, AcceptedAt: f.RequestedAt}
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := e.h.Graph.InsertFollow(ctx, tx, f); err != nil {
			return err
		}
		if err := e.h.Graph.UpsertPreferences(ctx, tx, prefs); err != nil {
			return err
		}
		if err := e.h.Graph.InsertAcceptance(ctx, tx, acc); err != nil {
			return err
		}
		return tx.Emit(ctx, appgraph.Followed{Follow: f, Acceptance: acc, Preferences: prefs})
	})
	if err != nil {
		t.Fatalf("follow %s → %s: %v", follower, followee, err)
	}
}

// record records doc as received, signed by its actor, without processing it.
func (e *remoteStatusEnv) record(t *testing.T, doc []byte) federation.IRI {
	t.Helper()
	return e.recordAs(t, doc, federation.TrustSigned)
}

// recordAs records doc as received with trust.
func (e *remoteStatusEnv) recordAs(t *testing.T, doc []byte, trust federation.Trust) federation.IRI {
	t.Helper()
	ctx := t.Context()
	env, err := federation.ReadEnvelope(doc, appfederation.CanonicalHost)
	if err != nil {
		t.Fatalf("ReadEnvelope(%s): %v", doc, err)
	}
	a := federation.ReceivedActivity{
		ActivityURI: env.ActivityURI(doc), ActorURI: env.Actor, Type: env.Type, Scheme: "cavage", Trust: trust,
		ReceivedAt: e.clock.Now(),
	}
	err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ok, err := e.h.Inbox.InsertReceivedActivity(ctx, tx, a)
		if err != nil || !ok {
			return err
		}
		return e.h.Inbox.InsertReceivedDocument(ctx, tx, a.ActivityURI, doc)
	})
	if err != nil {
		t.Fatalf("record %s: %v", a.ActivityURI, err)
	}
	return a.ActivityURI
}

// deliver records doc as received, signed by its actor, and processes it.
func (e *remoteStatusEnv) deliver(t *testing.T, doc []byte) appfederation.InboxOutcome {
	t.Helper()
	uri := e.record(t, doc)
	got, err := e.processor.Process(t.Context(), uri, federation.TrustSigned)
	if err != nil {
		t.Fatalf("Process(%s): %v", uri, err)
	}
	return got
}

// expect delivers doc and fails unless the outcome is want.
func (e *remoteStatusEnv) expect(t *testing.T, doc []byte, want appfederation.InboxOutcome) {
	t.Helper()
	if got := e.deliver(t, doc); got != want {
		t.Errorf("processing %s = %q, want %q", doc, got, want)
	}
}

// post returns the stored status with uri and its current revision, failing when there is none.
func (e *remoteStatusEnv) post(t *testing.T, uri string) appcontent.Post {
	t.Helper()
	st, err := e.h.Statuses.FindStatusByURI(t.Context(), uri)
	if err != nil {
		t.Fatalf("FindStatusByURI(%s) = %v", uri, err)
	}
	rev, err := e.h.Statuses.FindCurrentRevision(t.Context(), st.ID)
	if err != nil {
		t.Fatalf("FindCurrentRevision(%s) = %v", uri, err)
	}
	return appcontent.Post{Status: st, Revision: rev}
}

func (e *remoteStatusEnv) stored(t *testing.T, uri string) bool {
	t.Helper()
	_, err := e.h.Statuses.FindStatusByURI(t.Context(), uri)
	if err != nil && !errors.Is(err, appcontent.ErrNotFound) {
		t.Fatalf("FindStatusByURI(%s) = %v", uri, err)
	}
	return err == nil
}

// remoteActivity returns an activity as JSON.
func remoteActivity(t *testing.T, typ, id, actor string, object any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams", "id": id, "type": typ, "actor": actor, "object": object,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// remoteNote returns a public Note of author published at published, with fields added.
func remoteNote(id, author string, published time.Time, fields map[string]any) map[string]any {
	n := map[string]any{
		"id": id, "type": "Note", "attributedTo": author, "content": "<p>hello</p>", "published": published.Format(time.RFC3339),
		"to": []string{string(federation.PublicIRI)}, "cc": []string{author + "/followers"},
	}
	for k, v := range fields {
		n[k] = v
	}
	return n
}

// fixtureActivity returns a federation fixture as a Create: an activity as it is, an object
// wrapped in a Create of its author, with http IRIs made https.
func fixtureActivity(t *testing.T, implementation, version, name string) (doc []byte, actor, object string) {
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
	// The inbox takes only https actors; the older fixtures were taken from servers on http.
	raw := []byte(strings.ReplaceAll(string(fixtures[i].JSON), `"http://`, `"https://`))
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if v["type"] == "Create" {
		obj := v["object"].(map[string]any)
		return raw, v["actor"].(string), obj["id"].(string)
	}
	author, _ := v["attributedTo"].(string)
	if author == "" {
		author, _ = v["actor"].(string)
	}
	id := v["id"].(string)
	return remoteActivity(t, "Create", id+"#create", author, v), author, id
}

func remoteIngestsFixtures(t *testing.T, e *remoteStatusEnv) {
	reader := e.user(t, "reader")
	for _, tc := range []struct {
		implementation, version, name string
		visibility                    content.Visibility
		language, spoiler, text       string
		published                     string
	}{
		{"mastodon", "circa-2018", "create-note", content.VisibilityPublic, "und", "cw", "bob", "2018-02-12T14:08:20Z"},
		{"mastodon", "circa-2023", "note-language", content.VisibilityPublic, "ja", "", "tag", "2023-01-11T15:31:01Z"},
		{"misskey", "circa-2022", "create-note", content.VisibilityPublic, "und", "", "meow", "2022-08-01T11:06:49.568Z"},
		{"misskey", "circa-2019", "question", content.VisibilityPublic, "und", "", "How are your notifications now?", "2019-09-05T05:35:32.541Z"},
		{"gotosocial", "0.22", "create-note", content.VisibilityPublic, "en", "introduction post", "hello everyone!", "2021-10-20T10:40:37Z"},
		{"akkoma", "3.20", "note-quote", content.VisibilityUnlisted, "en", "", "Look at that!", "2024-04-06T23:40:28Z"},
	} {
		t.Run(tc.implementation+"/"+tc.version+"/"+tc.name, func(t *testing.T) {
			doc, actor, object := fixtureActivity(t, tc.implementation, tc.version, tc.name)
			author := e.actor(t, actor)
			// Unrelated, the post is not stored.
			e.expect(t, doc, appfederation.InboxDropped)
			if e.stored(t, object) {
				t.Fatalf("%s was stored before a local account followed its author", object)
			}
			e.follow(t, reader, author)
			// The same activity is not recorded twice: a new delivery of it (another id) is.
			var v map[string]any
			if err := json.Unmarshal(doc, &v); err != nil {
				t.Fatal(err)
			}
			v["id"] = fmt.Sprint(v["id"]) + "-again"
			again, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			e.expect(t, again, appfederation.InboxHandled)
			p := e.post(t, object)
			st, rev := p.Status, p.Revision
			want, err := time.Parse(time.RFC3339, tc.published)
			if err != nil {
				t.Fatal(err)
			}
			if got, ok := kernel.UUIDv7Time(st.ID); !ok || !got.Equal(want.Truncate(time.Millisecond)) {
				t.Errorf("ID %s has time %v, want the published time %v (#71)", st.ID, got, want)
			}
			if !st.PublishedAt.Equal(want.Truncate(time.Millisecond)) || st.AuthorAccountID != author || st.IsLocal {
				t.Errorf("status = %+v, want published %v by %s, remote", st, want, author)
			}
			if st.Visibility != tc.visibility {
				t.Errorf("visibility = %s, want %s", st.Visibility, tc.visibility)
			}
			if rev.Seq != 1 || rev.Language != tc.language || rev.SpoilerText != tc.spoiler || len(rev.Bodies) == 0 ||
				!strings.Contains(rev.Bodies[0].ContentHTML, tc.text) {
				t.Errorf("revision = %+v, want language %s, CW %q, and a body with %q", rev, tc.language, tc.spoiler, tc.text)
			}
			if rev.Remote == nil || rev.Remote.HTMLPolicyVersion != (remoteSanitizer{}).PolicyVersion() || rev.Remote.ActivityURI == "" {
				t.Errorf("remote revision = %+v, want the activity and the policy version", rev.Remote)
			}
		})
	}
	// The poll of the Misskey Question.
	p := e.post(t, "https://misskey.example/notes/7x9tmrp97i")
	if poll := p.Revision.Poll; poll == nil || poll.Multiple || len(poll.Options) != 2 || poll.Options[1].Title != "Broken af" || poll.ClosesAt != nil {
		t.Fatalf("poll = %+v, want two single-choice options without an end", poll)
	}
	tallies, err := e.content.PollTallies(t.Context(), p.Revision.Poll.ID)
	one := int64(1)
	want := []content.PollTally{
		{PollID: p.Revision.Poll.ID, Position: 0, Votes: 0, VotersCount: &one},
		{PollID: p.Revision.Poll.ID, Position: 1, Votes: 1, VotersCount: &one},
	}
	if err != nil || !reflect.DeepEqual(tallies, want) {
		t.Errorf("PollTallies = %+v, %v; want %+v", tallies, err, want)
	}
}

func remoteScope(t *testing.T, e *remoteStatusEnv) {
	const alice = "https://remote.example/users/alice"
	e.actor(t, alice)
	local := e.user(t, "local")
	published := remoteNow.Add(-time.Hour)
	note := func(n int, fields map[string]any) (string, []byte) {
		id := fmt.Sprintf("%s/notes/%d", alice, n)
		return id, remoteActivity(t, "Create", id+"/activity", alice, remoteNote(id, alice, published, fields))
	}

	// Unrelated: no local follower, mention, addressing, or local thread.
	unrelated, doc := note(1, nil)
	e.expect(t, doc, appfederation.InboxDropped)
	// Mentioning a local account.
	mention, doc := note(2, map[string]any{"tag": []any{map[string]any{"type": "Mention", "href": e.localIRI(local)}}})
	e.expect(t, doc, appfederation.InboxHandled)
	// Addressing a local account without a tag: a direct message.
	addressed, doc := note(3, map[string]any{"to": []string{e.localIRI(local)}, "cc": []string{}})
	e.expect(t, doc, appfederation.InboxHandled)
	// A reply to a local post, and a reply to that reply (a thread with a local post).
	mine, err := e.content.Create(t.Context(), appcontent.CreateInput{
		AuthorAccountID: local, TextInput: appcontent.TextInput{Text: "local post"}, Visibility: content.VisibilityPublic,
	})
	if err != nil {
		t.Fatal(err)
	}
	reply, doc := note(4, map[string]any{"inReplyTo": mine.Status.URI})
	e.expect(t, doc, appfederation.InboxHandled)
	nested, doc := note(5, map[string]any{"inReplyTo": reply})
	e.expect(t, doc, appfederation.InboxHandled)
	// A reply to a remote post outside any local thread is not.
	offThread, doc := note(6, map[string]any{"inReplyTo": "https://remote.example/users/alice/notes/999"})
	e.expect(t, doc, appfederation.InboxDropped)

	for uri, want := range map[string]bool{unrelated: false, mention: true, addressed: true, reply: true, nested: true, offThread: false} {
		if got := e.stored(t, uri); got != want {
			t.Errorf("stored(%s) = %v, want %v", uri, got, want)
		}
	}
	if p := e.post(t, addressed); p.Status.Visibility != content.VisibilityDirect || !slices.Contains(p.Revision.MentionedAccountIDs, local) {
		t.Errorf("addressed post = %+v, want direct and mentioning %s", p, local)
	}
	if p := e.post(t, nested); p.Status.ConversationURI != mine.Status.URI {
		t.Errorf("conversation of the nested reply = %q, want the local post's %q", p.Status.ConversationURI, mine.Status.URI)
	}
	// Following the author brings the next post in.
	e.follow(t, local, e.actor(t, alice))
	followed, doc := note(7, nil)
	e.expect(t, doc, appfederation.InboxHandled)
	if !e.stored(t, followed) {
		t.Errorf("a post of a followed author was not stored")
	}
}

func remoteRecordsParts(t *testing.T, e *remoteStatusEnv) {
	const alice, bob = "https://remote.example/users/alice", "https://other.example/users/bob"
	aliceID := e.follower(t, alice)
	bobID := e.actor(t, bob)
	const carol = "https://third.example/users/carol" // unknown, resolved on use
	e.actors.serve(carol)
	local := e.user(t, "local")
	id := alice + "/notes/parts"
	end := remoteNow.Add(24 * time.Hour)
	e.expect(t, remoteActivity(t, "Create", id+"/activity", alice, remoteNote(id, alice, remoteNow.Add(-time.Minute), map[string]any{
		"type":       "Question",
		"url":        "https://remote.example/@alice/parts",
		"content":    `<p>hi <script>alert(1)</script><a href="https://remote.example/tags/Go">#Go</a> :blob:</p>`,
		"contentMap": map[string]any{"en": `<p>hi <script>alert(1)</script>#Go :blob:</p>`, "ja": "<p>やあ</p>"},
		"summary":    "spoiler",
		"sensitive":  true,
		"to":         []string{alice + "/followers"},
		"cc":         []string{bob, e.localIRI(local)},
		"tag": []any{
			map[string]any{"type": "Hashtag", "name": "#Go"},
			map[string]any{"type": "Hashtag", "name": "#go"},
			map[string]any{"type": "Mention", "href": bob},
			map[string]any{"type": "Mention", "href": carol},
			map[string]any{"type": "Mention", "href": "https://gone.example/users/x"},
			map[string]any{"type": "Emoji", "id": "https://remote.example/emojis/1", "name": ":blob:", "icon": map[string]any{"type": "Image", "url": "https://remote.example/files/blob.png"}},
		},
		"attachment": []any{
			map[string]any{
				"type": "Document", "mediaType": "image/png", "url": "https://remote.example/files/a.png", "name": "alt <b>text</b>",
				"blurhash": "UBL_:rOpGG-oBUNG,qRj2so|=eE1w^n4S5NH", "width": 640, "height": 480, "focalPoint": []any{0.5, -2.0},
			},
			map[string]any{"type": "Video", "url": map[string]any{"type": "Link", "href": "https://remote.example/files/b.mp4", "mediaType": "video/mp4"}},
		},
		"anyOf":       []any{map[string]any{"type": "Note", "name": "yes", "replies": map[string]any{"totalItems": 3}}, map[string]any{"type": "Note", "name": "no", "replies": map[string]any{"totalItems": 1}}},
		"endTime":     end.Format(time.RFC3339),
		"votersCount": 3,
	})), appfederation.InboxHandled)

	p := e.post(t, id)
	st, rev := p.Status, p.Revision
	if st.Visibility != content.VisibilityPrivate || st.URL != "https://remote.example/@alice/parts" || st.AuthorAccountID != aliceID {
		t.Errorf("status = %+v, want a followers-only post with its url", st)
	}
	if rev.SpoilerText != "spoiler" || !rev.Sensitive || rev.Language != "en" || len(rev.Bodies) != 2 ||
		rev.Bodies[0].Language != "en" || strings.Contains(rev.Bodies[0].ContentHTML, "script") || rev.Bodies[1].ContentHTML != "<p>やあ</p>" {
		t.Errorf("revision = %+v, want CW, sensitive, and sanitized en and ja bodies", rev)
	}
	carolActor, err := e.remote.FindByURI(t.Context(), carol)
	if err != nil {
		t.Fatalf("the mentioned carol was not resolved: %v", err)
	}
	if want := sortedIDs(bobID, carolActor.Account.ID, local); !reflect.DeepEqual(rev.MentionedAccountIDs, want) {
		t.Errorf("mentions = %v, want bob, carol, and the addressed local account %v", rev.MentionedAccountIDs, want)
	}
	if len(rev.TagIDs) != 1 {
		t.Errorf("tags = %v, want #Go once", rev.TagIDs)
	} else if tag, err := e.h.Statuses.FindTag(t.Context(), rev.TagIDs[0]); err != nil || tag.NameNormalized != "go" {
		t.Errorf("tag = %+v, %v; want go", tag, err)
	}
	if len(rev.EmojiIDs) != 1 {
		t.Fatalf("emoji = %v, want :blob:", rev.EmojiIDs)
	}
	emoji, img, err := e.content.CustomEmoji(t.Context(), rev.EmojiIDs[0])
	if err != nil || emoji.Shortcode != "blob" || emoji.Domain != "remote.example" || emoji.URI != "https://remote.example/emojis/1" {
		t.Errorf("emoji = %+v, %v; want blob of remote.example", emoji, err)
	}
	if m, err := e.h.Media.FindRemoteMedia(t.Context(), img.MediaID); err != nil || m.RemoteURL != "https://remote.example/files/blob.png" || m.OwnerAccountID != aliceID {
		t.Errorf("emoji image = %+v, %v; want the icon as alice's remote media", m, err)
	}
	if len(rev.Attachments) != 2 || rev.Attachments[0].Description != "alt text" {
		t.Fatalf("attachments = %+v, want two, the first with plain alt text", rev.Attachments)
	}
	first, err := e.h.Media.FindRemoteMedia(t.Context(), rev.Attachments[0].MediaID)
	if err != nil || first.RemoteURL != "https://remote.example/files/a.png" || first.DeclaredContentType != "image/png" ||
		first.DeclaredMediaType != content.MediaTypeImage || first.Width != 640 || first.Height != 480 || first.Blurhash == "" ||
		first.Focus == nil || first.Focus.X != 0.5 || first.Focus.Y != -1 || first.Description != "alt text" {
		t.Errorf("first attachment = %+v, %v; want the declared values, the focus clamped", first, err)
	}
	if second, err := e.h.Media.FindRemoteMedia(t.Context(), rev.Attachments[1].MediaID); err != nil || second.DeclaredMediaType != content.MediaTypeVideo {
		t.Errorf("second attachment = %+v, %v; want a video", second, err)
	}
	poll := rev.Poll
	if poll == nil || !poll.Multiple || poll.ClosesAt == nil || !poll.ClosesAt.Equal(end) || len(poll.Options) != 2 || poll.Options[0].Title != "yes" {
		t.Fatalf("poll = %+v, want a multiple-choice poll closing at %v", poll, end)
	}
	tallies, err := e.content.PollTallies(t.Context(), poll.ID)
	three := int64(3)
	if want := []content.PollTally{{PollID: poll.ID, Position: 0, Votes: 3, VotersCount: &three}, {PollID: poll.ID, Position: 1, Votes: 1, VotersCount: &three}}; err != nil || !reflect.DeepEqual(tallies, want) {
		t.Errorf("PollTallies = %+v, %v; want %+v", tallies, err, want)
	}
}

// remoteStatusStoresStringsWithNUL delivers a Create of a poll whose every string has a NUL (\u0000):
// the activity is recorded and handled, and each NUL is stored as U+FFFD (federation.md §4) the
// same way in every dialect.
func remoteStatusStoresStringsWithNUL(t *testing.T, e *remoteStatusEnv) {
	const alice = "https://remote.example/users/alice"
	e.follower(t, alice)
	const n = "\x00"
	id := alice + "/notes/nul" + n
	doc := remoteActivity(t, "Create", id+"/activity", alice, remoteNote(id, alice, remoteNow.Add(-time.Minute), map[string]any{
		"type":       "Question",
		"url":        "https://remote.example/@alice/nul" + n,
		"content":    "<p>a" + n + "b #Go" + n + "</p>",
		"contentMap": map[string]any{"en" + n: "<p>c" + n + "</p>", "ja": "<p>d" + n + "</p>"},
		"summary":    "cw" + n,
		"tag": []any{
			map[string]any{"type": "Hashtag", "name": "#Go" + n},
			map[string]any{"type": "Mention", "href": alice, "name": "@alice" + n},
			map[string]any{"type": "Emoji", "id": "https://remote.example/emojis/nul" + n, "name": ":blob" + n + ":", "icon": map[string]any{"type": "Image", "url": "https://remote.example/files/blob" + n + ".png"}},
		},
		"attachment": []any{map[string]any{
			"type": "Document", "mediaType": "image/png" + n, "url": "https://remote.example/files/a" + n + ".png", "name": "alt" + n, "blurhash": "U" + n,
		}},
		"oneOf":   []any{map[string]any{"type": "Note", "name": "yes" + n}, map[string]any{"type": "Note", "name": "no" + n}},
		"endTime": remoteNow.Add(time.Hour).Format(time.RFC3339),
	}))
	if !strings.Contains(string(doc), `\u0000`) {
		t.Fatalf("the document %s has no \\u0000", doc)
	}
	e.expect(t, doc, appfederation.InboxHandled)

	p := e.post(t, alice+"/notes/nul%EF%BF%BD")
	if path := nulPath(reflect.ValueOf(p), "Post"); path != "" {
		t.Errorf("%s has a NUL", path)
	}
	st, rev := p.Status, p.Revision
	if st.URL != "https://remote.example/@alice/nul%EF%BF%BD" || rev.SpoilerText != "cw\uFFFD" || len(rev.Bodies) != 2 ||
		rev.Bodies[0] != (content.Body{Language: "ja", ContentHTML: "<p>d\uFFFD</p>"}) || rev.Bodies[1] != (content.Body{Language: federation.UndeterminedLang, ContentHTML: "<p>c\uFFFD</p>"}) {
		t.Errorf("post = %+v, want every NUL as U+FFFD", p)
	}
	if rev.Poll == nil || len(rev.Poll.Options) != 2 || rev.Poll.Options[0].Title != "yes\uFFFD" {
		t.Errorf("poll = %+v, want the options with U+FFFD", rev.Poll)
	}
	if len(rev.Attachments) != 1 || rev.Attachments[0].Description != "alt\uFFFD" {
		t.Fatalf("attachments = %+v, want one described alt\uFFFD", rev.Attachments)
	}
	if m, err := e.h.Media.FindRemoteMedia(t.Context(), rev.Attachments[0].MediaID); err != nil ||
		m.RemoteURL != "https://remote.example/files/a%EF%BF%BD.png" || m.Description != "alt\uFFFD" || nulPath(reflect.ValueOf(m), "") != "" {
		t.Errorf("attachment = %+v, %v; want every NUL as U+FFFD", m, err)
	}
	for _, tagID := range rev.TagIDs {
		if tag, err := e.h.Statuses.FindTag(t.Context(), tagID); err != nil || nulPath(reflect.ValueOf(tag), "") != "" {
			t.Errorf("tag = %+v, %v; want no NUL", tag, err)
		}
	}
	for _, emojiID := range rev.EmojiIDs {
		if emoji, _, err := e.content.CustomEmoji(t.Context(), emojiID); err != nil || nulPath(reflect.ValueOf(emoji), "") != "" {
			t.Errorf("emoji = %+v, %v; want no NUL", emoji, err)
		}
	}
	// Processing it again is a no-op, not a failure.
	got, err := e.processor.Process(t.Context(), federation.IRI(alice+"/notes/nul%EF%BF%BD/activity"), federation.TrustSigned)
	if err != nil || got != appfederation.InboxHandled {
		t.Errorf("processing again = %q, %v; want handled", got, err)
	}
}

func sortedIDs(ids ...string) []string {
	slices.Sort(ids)
	return ids
}

// follower stores the remote actor uri, makes a local account follow it, and returns its ID.
func (e *remoteStatusEnv) follower(t *testing.T, uri string) string {
	t.Helper()
	id := e.actor(t, uri)
	e.follow(t, e.user(t, "follower"+fmt.Sprint(len(e.actors.served))), id)
	return id
}

func remoteChecksAuthor(t *testing.T, e *remoteStatusEnv) {
	const alice, bob = "https://remote.example/users/alice", "https://other.example/users/bob"
	e.follower(t, alice)
	e.follower(t, bob)
	published := remoteNow.Add(-time.Hour)
	// A post attributed to someone else, and one from another origin, are not taken.
	forged := alice + "/notes/forged"
	e.expect(t, remoteActivity(t, "Create", forged+"/activity", alice, remoteNote(forged, bob, published, nil)), appfederation.InboxDropped)
	foreign := "https://other.example/users/bob/notes/1"
	e.expect(t, remoteActivity(t, "Create", alice+"/notes/foreign/activity", alice, remoteNote(foreign, alice, published, nil)), appfederation.InboxDropped)
	// Something other than a post is not taken either.
	e.expect(t, remoteActivity(t, "Create", alice+"/collections/1/activity", alice, map[string]any{"id": alice + "/collections/1", "type": "Collection"}), appfederation.InboxDropped)
	for _, uri := range []string{forged, foreign, alice + "/collections/1"} {
		if e.stored(t, uri) {
			t.Errorf("%s was stored", uri)
		}
	}
	// A Create carrying only the id is fetched from the origin when a local account follows the
	// author, and not fetched otherwise.
	const carol = "https://third.example/users/carol"
	e.actor(t, carol)
	unfollowed := carol + "/notes/by-id"
	e.fetcher.serve(t, unfollowed, remoteNote(unfollowed, carol, published, nil))
	e.expect(t, remoteActivity(t, "Create", unfollowed+"/activity", carol, unfollowed), appfederation.InboxDropped)
	if slices.Contains(e.fetcher.fetched, unfollowed) {
		t.Errorf("%s was fetched, though no local account follows its author", unfollowed)
	}
	byID := alice + "/notes/by-id"
	e.fetcher.serve(t, byID, remoteNote(byID, alice, published, map[string]any{"content": "<p>fetched</p>"}))
	e.expect(t, remoteActivity(t, "Create", byID+"/activity", alice, byID), appfederation.InboxHandled)
	if p := e.post(t, byID); p.Revision.Bodies[0].ContentHTML != "<p>fetched</p>" {
		t.Errorf("post created by id = %+v, want the fetched content", p.Revision)
	}
	// Another actor may neither update nor delete alice's post.
	e.expect(t, remoteActivity(t, "Update", bob+"#update", bob, remoteNote(byID, bob, published, map[string]any{"updated": remoteNow.Format(time.RFC3339)})), appfederation.InboxDropped)
	e.expect(t, remoteActivity(t, "Delete", bob+"#delete", bob, byID), appfederation.InboxDropped)
	if p := e.post(t, byID); p.Revision.Seq != 1 {
		t.Errorf("bob revised alice's post: %+v", p.Revision)
	}
}

// remoteTakesAddressingFromTheAuthor checks that a post without addressing takes the addressing
// of the activity that carried it only when its author signed the activity: a Create that
// someone else signed takes the post from its origin and nothing of the activity.
func remoteTakesAddressingFromTheAuthor(t *testing.T, e *remoteStatusEnv) {
	const alice = "https://remote.example/users/alice"
	e.follower(t, alice)
	published := remoteNow.Add(-time.Hour)
	unaddressed := func(id string) map[string]any {
		n := remoteNote(id, alice, published, nil)
		delete(n, "to")
		delete(n, "cc")
		return n
	}
	create := func(id string) []byte {
		b, err := json.Marshal(map[string]any{
			"@context": "https://www.w3.org/ns/activitystreams", "id": id + "/activity", "type": "Create", "actor": alice,
			"object": unaddressed(id), "to": []string{string(federation.PublicIRI)},
		})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	// Signed by its author, the post takes the activity's addressing.
	signed := alice + "/notes/unaddressed-signed"
	e.expect(t, create(signed), appfederation.InboxHandled)
	if p := e.post(t, signed); p.Status.Visibility != content.VisibilityPublic {
		t.Errorf("post of a signed Create = %s, want public from the activity", p.Status.Visibility)
	}

	// Signed by someone else, it does not.
	relayed := alice + "/notes/unaddressed-relayed"
	e.fetcher.serve(t, relayed, unaddressed(relayed))
	uri := e.recordAs(t, create(relayed), federation.TrustRefetch)
	if _, err := e.processor.Process(t.Context(), uri, federation.TrustRefetch); err != nil {
		t.Fatalf("Process(%s): %v", uri, err)
	}
	if e.stored(t, relayed) {
		if p := e.post(t, relayed); p.Status.Visibility != content.VisibilityDirect {
			t.Errorf("post of a relayed Create = %s, want direct: no addressing of its author", p.Status.Visibility)
		}
	}
}

func remoteRevises(t *testing.T, e *remoteStatusEnv) {
	const alice = "https://remote.example/users/alice"
	e.follower(t, alice)
	id := alice + "/notes/1"
	published := remoteNow.Add(-2 * time.Hour)
	poll := func(yes int, updated time.Time, text string) map[string]any {
		f := map[string]any{
			"type": "Question", "content": text, "votersCount": yes,
			"oneOf": []any{map[string]any{"type": "Note", "name": "yes", "replies": map[string]any{"totalItems": yes}}},
		}
		if !updated.IsZero() {
			f["updated"] = updated.Format(time.RFC3339)
		}
		return remoteNote(id, alice, published, f)
	}
	e.expect(t, remoteActivity(t, "Create", id+"/activity", alice, poll(1, time.Time{}, "<p>first</p>")), appfederation.InboxHandled)
	edited := published.Add(time.Hour)
	update := func(n int, note map[string]any) {
		t.Helper()
		e.expect(t, remoteActivity(t, "Update", fmt.Sprintf("%s#update-%d", id, n), alice, note), appfederation.InboxHandled)
	}
	// Newer: a second revision.
	update(1, poll(2, edited, "<p>second</p>"))
	// The same Update again, and an older one: no revision, though the higher counts count.
	update(2, poll(2, edited, "<p>second</p>"))
	update(3, poll(5, published.Add(time.Minute), "<p>stale</p>"))
	// The counts of the current poll without an edit (Mastodon's Update of a Question).
	update(4, poll(7, edited, "<p>second</p>"))
	// Lower counts arriving late are older.
	update(5, poll(6, edited, "<p>second</p>"))

	p := e.post(t, id)
	if p.Revision.Seq != 2 || p.Revision.Bodies[0].ContentHTML != "<p>second</p>" || !p.Revision.RevisedAt.Equal(edited) {
		t.Errorf("current revision = %+v, want the second at %v", p.Revision, edited)
	}
	revs, err := e.content.Revisions(t.Context(), p.Status.ID)
	if err != nil || len(revs) != 2 {
		t.Errorf("Revisions = %d, %v; want 2", len(revs), err)
	}
	tallies, err := e.content.PollTallies(t.Context(), p.Revision.Poll.ID)
	if err != nil || len(tallies) != 1 || tallies[0].Votes != 7 {
		t.Errorf("PollTallies = %+v, %v; want 7 votes, the highest count", tallies, err)
	}
}

func remoteUpdateAsCreate(t *testing.T, e *remoteStatusEnv) {
	const alice = "https://remote.example/users/alice"
	e.follower(t, alice)
	recent, old := alice+"/notes/recent", alice+"/notes/old"
	e.expect(t, remoteActivity(t, "Update", recent+"#update", alice, remoteNote(recent, alice, remoteNow.Add(-time.Hour), map[string]any{
		"updated": remoteNow.Add(-time.Minute).Format(time.RFC3339), "content": "<p>edited</p>",
	})), appfederation.InboxHandled)
	e.expect(t, remoteActivity(t, "Update", old+"#update", alice, remoteNote(old, alice, remoteNow.Add(-48*time.Hour), map[string]any{
		"updated": remoteNow.Add(-time.Minute).Format(time.RFC3339),
	})), appfederation.InboxDropped)
	p := e.post(t, recent)
	first := p.Revision.ID
	if p.Revision.Seq != 1 || p.Revision.Bodies[0].ContentHTML != "<p>edited</p>" || !p.Revision.RevisedAt.Equal(remoteNow.Add(-time.Minute)) {
		t.Errorf("post from its Update = %+v, want its first revision with the edited content", p.Revision)
	}
	if e.stored(t, old) {
		t.Errorf("the Update of a post published two days ago created it")
	}
	// The Create arriving after changes nothing.
	e.expect(t, remoteActivity(t, "Create", recent+"/activity", alice, remoteNote(recent, alice, remoteNow.Add(-time.Hour), nil)), appfederation.InboxHandled)
	if p := e.post(t, recent); p.Revision.ID != first || p.Revision.Bodies[0].ContentHTML != "<p>edited</p>" {
		t.Errorf("post after its late Create = %+v, want it unchanged", p.Revision)
	}
}

func remoteDeletes(t *testing.T, e *remoteStatusEnv) {
	const alice = "https://remote.example/users/alice"
	e.follower(t, alice)
	published := remoteNow.Add(-time.Hour)
	id := alice + "/notes/1"
	create := remoteActivity(t, "Create", id+"/activity", alice, remoteNote(id, alice, published, nil))
	e.expect(t, create, appfederation.InboxHandled)
	st := e.post(t, id).Status
	e.expect(t, remoteActivity(t, "Delete", id+"#delete", alice, map[string]any{"id": id, "type": "Tombstone"}), appfederation.InboxHandled)
	d, err := e.h.Statuses.FindDeletion(t.Context(), st.ID)
	if err != nil || d.Origin != content.DeletedByRemote || d.URI != id {
		t.Errorf("deletion = %+v, %v; want a remote deletion of %s", d, err, id)
	}
	if _, err := e.h.Statuses.FindCurrentRevision(t.Context(), st.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("the deleted post still has a head: %v", err)
	}
	// Again, and a new Create or Update of it: nothing comes back.
	e.expect(t, remoteActivity(t, "Delete", id+"#delete-again", alice, id), appfederation.InboxHandled)
	e.expect(t, remoteActivity(t, "Create", id+"/activity-again", alice, remoteNote(id, alice, published, nil)), appfederation.InboxDropped)
	e.expect(t, remoteActivity(t, "Update", id+"#update", alice, remoteNote(id, alice, published, map[string]any{"updated": remoteNow.Format(time.RFC3339)})), appfederation.InboxDropped)
	if _, err := e.h.Statuses.FindCurrentRevision(t.Context(), st.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("the deleted post came back: %v", err)
	}
	// The Delete of an unknown post leaves a tombstone that stops its Create.
	unknown := alice + "/notes/2"
	e.expect(t, remoteActivity(t, "Delete", unknown+"#delete", alice, unknown), appfederation.InboxTombstoned)
	e.expect(t, remoteActivity(t, "Create", unknown+"/activity", alice, remoteNote(unknown, alice, published, nil)), appfederation.InboxDropped)
	if e.stored(t, unknown) {
		t.Errorf("%s was stored after its Delete", unknown)
	}
}

func remoteFeeds(t *testing.T, e *remoteStatusEnv) {
	const alice = "https://remote.example/users/alice"
	aliceID := e.actor(t, alice)
	reader, mentioned := e.user(t, "reader"), e.user(t, "mentioned")
	e.follow(t, reader, aliceID)
	local, err := e.content.Create(t.Context(), appcontent.CreateInput{
		AuthorAccountID: reader, TextInput: appcontent.TextInput{Text: "question"}, Visibility: content.VisibilityPublic,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := alice + "/notes/1"
	e.expect(t, remoteActivity(t, "Create", id+"/activity", alice, remoteNote(id, alice, remoteNow.Add(-time.Hour), map[string]any{
		"inReplyTo": local.Status.URI,
		"cc":        []string{alice + "/followers", e.localIRI(mentioned)},
		"tag":       []any{map[string]any{"type": "Mention", "href": e.localIRI(mentioned)}, map[string]any{"type": "Hashtag", "name": "#news"}},
	})), appfederation.InboxHandled)
	st := e.post(t, id).Status

	home, err := e.timeline.Home(t.Context(), appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: reader}, apptimeline.Query{Limit: apptimeline.MaxLimit})
	if err != nil || !slices.ContainsFunc(home.Entries, func(en apptimeline.Entry) bool { return en.StatusID == st.ID }) {
		t.Errorf("home of the follower = %+v, %v; want the remote post", home.Entries, err)
	}
	ns, err := e.notifications.List(t.Context(), mentioned, appnotification.Query{Limit: appnotification.MaxPageLimit})
	if err != nil || len(ns) != 1 || ns[0].Type != notification.TypeMention || ns[0].StatusID != st.ID || ns[0].SourceAccountID != aliceID {
		t.Errorf("notifications of the mentioned account = %+v, %v; want a mention by alice", ns, err)
	}
	counters, err := e.h.Statuses.FindCounters(t.Context(), local.Status.ID)
	if err != nil || counters.RepliesCount != 1 {
		t.Errorf("counters of the local post = %+v, %v; want one reply", counters, err)
	}
	if posts, err := e.h.Statuses.FindPostCounter(t.Context(), aliceID); err != nil || posts.StatusesCount != 1 {
		t.Errorf("post counter of alice = %+v, %v; want 1", posts, err)
	}
	// The Delete takes the post out of the timelines.
	e.expect(t, remoteActivity(t, "Delete", id+"#delete", alice, id), appfederation.InboxHandled)
	home, err = e.timeline.Home(t.Context(), appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: reader}, apptimeline.Query{Limit: apptimeline.MaxLimit})
	if err != nil || slices.ContainsFunc(home.Entries, func(en apptimeline.Entry) bool { return en.StatusID == st.ID }) {
		t.Errorf("home after the Delete = %+v, %v; want the post gone", home.Entries, err)
	}
}

// The cases of remote posts in the order and duplicate property test (inboxOrderCases): the
// real handlers of Create, Update, and Delete on the content module, with stand-ins for the
// accounts and a local follower of every author, so that every post is in scope.

// remoteOrderPublished is the published time of the post of the order cases, an hour before the
// clock of inboxProcessEnv; its Updates follow a minute apart.
var remoteOrderPublished = time.Date(2026, 10, 17, 11, 0, 0, 0, time.UTC)

// remoteStatusActivities returns the activities named by steps about alice's note 1: "create",
// "update-N" (updated N minutes after it was published, with the content <p>update-N</p>), and
// "delete".
func remoteStatusActivities(steps ...string) func(t *testing.T) [][]byte {
	return func(t *testing.T) [][]byte {
		id := inboxAlice + "/notes/1"
		var out [][]byte
		for _, step := range steps {
			switch {
			case step == "create":
				out = append(out, remoteActivity(t, "Create", id+"/activity", inboxAlice, remoteNote(id, inboxAlice, remoteOrderPublished, nil)))
			case step == "delete":
				out = append(out, remoteActivity(t, "Delete", id+"#delete", inboxAlice, map[string]any{"id": id, "type": "Tombstone"}))
			case strings.HasPrefix(step, "update-"):
				var n int
				if _, err := fmt.Sscanf(step, "update-%d", &n); err != nil {
					t.Fatal(err)
				}
				out = append(out, remoteActivity(t, "Update", id+"#"+step, inboxAlice, remoteNote(id, inboxAlice, remoteOrderPublished, map[string]any{
					"updated": remoteOrderPublished.Add(time.Duration(n) * time.Minute).Format(time.RFC3339), "content": "<p>" + step + "</p>",
				})))
			default:
				t.Fatalf("unknown step %q", step)
			}
		}
		return out
	}
}

// wireStatusHandlers registers the handlers of remote posts on e, on the content module of its
// database.
func wireStatusHandlers(t *testing.T, e *inboxProcessEnv) {
	t.Helper()
	if e.h.Statuses == nil {
		t.Skip("the harness has no content repository")
	}
	appcontent.RegisterProjectors(e.projectors, e.h.Statuses)
	statuses := appcontent.NewService(appcontent.Deps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Statuses, Sanitizer: remoteSanitizer{},
		Clock: e.clock, IDs: NewIDGenerator(e.clock), Host: contractHost,
	})
	appfederation.NewStatusHandlers(appfederation.StatusHandlersDeps{
		UnitOfWork: e.h.UnitOfWork, Statuses: statuses, Accounts: noStatusAccounts{}, Fetcher: e.fetcher,
		Followers: everyAuthorFollowed{}, Clock: e.clock, Host: contractHost,
	}).Register(e.processor)
}

// noStatusAccounts knows no account: posts mention nobody stored, and authors have no followers
// collection.
type noStatusAccounts struct{}

func (noStatusAccounts) FindByURI(_ context.Context, uri string) (appaccount.Actor, error) {
	return appaccount.Actor{}, fmt.Errorf("%s: %w", uri, appaccount.ErrNotFound)
}

func (noStatusAccounts) RemoteFollowersURL(_ context.Context, accountID string) (string, error) {
	return "", fmt.Errorf("%s: %w", accountID, appaccount.ErrNotFound)
}

// everyAuthorFollowed gives every account one local follower.
type everyAuthorFollowed struct{}

func (everyAuthorFollowed) ActiveFollowerIDs(context.Context, string, string, int) ([]string, string, error) {
	return []string{"local-follower"}, "", nil
}

// inboxStatusRows are the remote posts among some URIs as their final rows show them: the live
// ones with their current content and its time, and the deleted or tombstoned ones.
type inboxStatusRows struct {
	Live []inboxLiveStatus
	Gone []federation.IRI
}

type inboxLiveStatus struct {
	URI       federation.IRI
	HTML      string
	RevisedAt time.Time
}

func remoteStatusRows(uris ...string) func(t *testing.T, e *inboxProcessEnv) any {
	return func(t *testing.T, e *inboxProcessEnv) any {
		ctx := t.Context()
		r := inboxStatusRows{Live: []inboxLiveStatus{}, Gone: []federation.IRI{}}
		for _, u := range uris {
			uri := federation.IRI(u)
			st, err := e.h.Statuses.FindStatusByURI(ctx, u)
			switch {
			case errors.Is(err, appcontent.ErrNotFound):
				if e.tombstoned(t, uri) {
					r.Gone = append(r.Gone, uri)
				}
				continue
			case err != nil:
				t.Fatal(err)
			}
			rev, err := e.h.Statuses.FindCurrentRevision(ctx, st.ID)
			switch {
			case errors.Is(err, appcontent.ErrNotFound):
				r.Gone = append(r.Gone, uri)
			case err != nil:
				t.Fatal(err)
			default:
				r.Live = append(r.Live, inboxLiveStatus{URI: uri, HTML: rev.Bodies[0].ContentHTML, RevisedAt: rev.RevisedAt})
			}
		}
		return r
	}
}
