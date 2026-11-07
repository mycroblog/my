// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

// PostingHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the identity, account, graph, instance, and status repositories on it.
type PostingHarness struct {
	identitytest.AccountHarness
	Graph    appgraph.Repository
	Instance appinstance.Repository
	Statuses appcontent.Repository
	Media    appcontent.MediaRepository
}

// PostingFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type PostingFactory func(t *testing.T, opts uow.Options) PostingHarness

// PostingFlows verifies the content module's posting use cases on a database adapter, the flows
// of ADR-0153 §1, which internal/flowtest/content runs once on SQLite; PostingContract checks the
// repository methods they rely on that StatusRepositoryContract does not, on every adapter:
// posting with mentions, hashtags, and links rendered to HTML; mentions of remote accounts, the
// unknown ones resolved before the transaction and within bounds; returning the same status for a
// repeated Idempotency-Key, also under concurrency; the max_status_length setting; rejecting
// invalid input; capping the visibility and forbidding posts and replies through the
// RestrictionPolicy; replies to statuses the
// author may see; attaching only the author's own processed uploads; the default post language; editing into new revisions that move the head;
// deleting into a tombstone that removes the status from the projections; the deletion and
// the sensitivity that moderation actions on statuses record; and the emitted domain events.
func PostingFlows(t *testing.T, newHarness PostingFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *postingEnv)
	}{
		{"PostsAStatus", postingPosts},
		{"RendersProfileText", postingRendersProfileText},
		{"MentionsRemoteAccounts", postingMentionsRemote},
		{"ReturnsTheSameStatusForAnIdempotencyKey", postingIdempotent},
		{"AgreesOnConcurrentIdempotencyKeys", postingIdempotentConcurrent},
		{"EnforcesMaxStatusLength", postingMaxLength},
		{"RejectsInvalidInput", postingRejectsInvalid},
		{"AppliesRestrictions", postingRestrictions},
		{"RepliesToVisibleStatuses", postingReplies},
		{"UsesTheDefaultLanguage", postingDefaultLanguage},
		{"EditsIntoNewRevisions", postingEdits},
		{"AttachesOwnProcessedMedia", postingAttachesMedia},
		{"RequiresMediaDescriptions", postingRequiresDescriptions},
		{"DeletesIntoATombstone", postingDeletes},
		{"CarriesOutModerationOnStatuses", postingModeration},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newPostingEnv(t, newHarness)) })
	}
}

type postingEnv struct {
	*identitytest.AccountEnv
	repo         appcontent.Repository
	media        appcontent.MediaRepository
	graph        *appgraph.Service
	instance     *appinstance.Service
	content      *appcontent.Service
	restrictions *fakeRestrictions
	languages    map[string]string // default post language by user ID
	events       *testkit.EventLog
	remote       *fakeRemoteMentions
}

// trackingUnitOfWork is a UnitOfWork that knows whether one of its transactions is running.
type trackingUnitOfWork struct {
	uow.UnitOfWork
	inTx *atomic.Bool
}

func (u trackingUnitOfWork) Tx(ctx context.Context, fn func(tx uow.Tx) error) error {
	return u.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		u.inTx.Store(true)
		defer u.inTx.Store(false)
		return fn(tx)
	})
}

// fakeRemoteMentions is a content.RemoteMentions whose resolvable handles the test serves: a
// resolution stores the actor, as fetching it would. It records every resolution and whether
// one ran inside a transaction of the content module or without a deadline.
type fakeRemoteMentions struct {
	accounts *appaccount.Service // with a sanitizer, for storing remote actors
	inTx     *atomic.Bool
	mu       sync.Mutex
	actors   map[string]appaccount.RemoteActorInput // by lowercase user@domain
	calls    []string
	inTxRuns int
	unbound  int
}

func (f *fakeRemoteMentions) ResolveHandle(ctx context.Context, username, domain string) (appaccount.Actor, error) {
	f.mu.Lock()
	h := username + "@" + domain
	f.calls = append(f.calls, h)
	if f.inTx.Load() {
		f.inTxRuns++
	}
	if _, ok := ctx.Deadline(); !ok {
		f.unbound++
	}
	in, ok := f.actors[strings.ToLower(h)]
	f.mu.Unlock()
	if !ok {
		return appaccount.Actor{}, errors.New("fake: no actor holds " + h)
	}
	return f.accounts.SaveRemoteActor(ctx, in)
}

func (f *fakeRemoteMentions) RemoteProfileURL(ctx context.Context, accountID string) (string, error) {
	return f.accounts.RemoteProfileURL(ctx, accountID)
}

// serve makes username@domain resolvable to a remote actor whose profile page is profileURL.
func (f *fakeRemoteMentions) serve(username, domain, profileURL string) appaccount.RemoteActorInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	in := appaccount.RemoteActorInput{
		URI: "https://" + domain + "/users/" + username, Domain: domain, Type: "Person", PreferredUsername: username,
		ProfileURL: profileURL, InboxURL: "https://" + domain + "/users/" + username + "/inbox",
		Document: []byte(`{"id":"https://` + domain + `/users/` + username + `"}`),
		Handle:   appaccount.HandleCheck{Outcome: account.HandleVerified, Username: username, Domain: domain},
	}
	f.actors[strings.ToLower(username+"@"+domain)] = in
	return in
}

func (f *fakeRemoteMentions) takeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.calls
	f.calls = nil
	return out
}

// fakeRestrictions is a RestrictionPolicy whose restrictions the test sets per account.
type fakeRestrictions struct {
	mu   sync.Mutex
	byID map[string]appcontent.Restrictions
}

func (f *fakeRestrictions) Restrictions(_ context.Context, accountID string) (appcontent.Restrictions, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byID[accountID], nil
}

func (f *fakeRestrictions) set(accountID string, r appcontent.Restrictions) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[accountID] = r
}

func newPostingEnv(t *testing.T, newHarness PostingFactory) *postingEnv {
	t.Helper()
	var (
		ph       PostingHarness
		captured uow.Options
	)
	ae := identitytest.NewAccountEnv(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		captured = opts
		ph = newHarness(t, opts)
		return ph.AccountHarness
	})
	appgraph.RegisterProjectors(captured.Projectors, ph.Graph)
	appinstance.RegisterProjectors(captured.Projectors, ph.Instance)
	appcontent.RegisterProjectors(captured.Projectors, ph.Statuses)
	log := &testkit.EventLog{}
	for _, typ := range []string{appcontent.EventStatusCreated, appcontent.EventStatusRevised, appcontent.EventStatusDeleted} {
		captured.Projectors.Register(typ, func(_ context.Context, _ uow.Tx, e uow.Event) error {
			log.Mu.Lock()
			defer log.Mu.Unlock()
			log.Events = append(log.Events, e)
			return nil
		})
	}
	e := &postingEnv{
		AccountEnv:   ae,
		repo:         ph.Statuses,
		media:        ph.Media,
		restrictions: &fakeRestrictions{byID: map[string]appcontent.Restrictions{}},
		languages:    map[string]string{},
		events:       log,
		graph: appgraph.NewService(appgraph.Deps{
			UnitOfWork: ph.UnitOfWork, Repository: ph.Graph, Accounts: ae.Accounts, Clock: ae.Clock, IDs: ae.IDs, Host: testkit.ContractHost,
		}),
		instance: appinstance.NewService(appinstance.Deps{
			UnitOfWork: ph.UnitOfWork, Repository: ph.Instance, Clock: ae.Clock, IDs: ae.IDs,
		}),
	}
	inTx := &atomic.Bool{}
	e.remote = &fakeRemoteMentions{
		accounts: appaccount.NewService(appaccount.Deps{
			UnitOfWork: ph.UnitOfWork, Repository: ph.Accounts, Clock: ae.Clock, IDs: ae.IDs,
			Sealer: kernel.NewSealer(testkit.NewKeyManager("posting")), Host: testkit.ContractHost, Sanitizer: testkit.RemoteSanitizer{},
		}),
		inTx: inTx, actors: map[string]appaccount.RemoteActorInput{},
	}
	e.content = appcontent.NewService(appcontent.Deps{
		UnitOfWork:              trackingUnitOfWork{UnitOfWork: ph.UnitOfWork, inTx: inTx},
		Repository:              ph.Statuses,
		Accounts:                ae.Accounts,
		Relationships:           e.graph,
		Restrictions:            e.restrictions,
		Media:                   ph.Media,
		RemoteMentions:          e.remote,
		MaxStatusLength:         e.instance.MaxStatusLength,
		RequireMediaDescription: e.instance.RequireMediaDescription,
		DefaultLanguage:         func(_ context.Context, userID string) (string, error) { return e.languages[userID], nil },
		Clock:                   ae.Clock,
		IDs:                     ae.IDs,
		Host:                    testkit.ContractHost,
		AccountDomain:           "example.org",
	})
	return e
}

func (e *postingEnv) user(t *testing.T, username string) string {
	t.Helper()
	return e.Register(t, username).AccountID
}

func (e *postingEnv) post(t *testing.T, in appcontent.CreateInput) appcontent.Post {
	t.Helper()
	e.Clock.Advance(time.Second)
	p, err := e.content.Create(t.Context(), in)
	if err != nil {
		t.Fatalf("Create(%+v) = %v", in, err)
	}
	return p
}

func (e *postingEnv) postText(t *testing.T, author, text string, v content.Visibility) appcontent.Post {
	t.Helper()
	return e.post(t, appcontent.CreateInput{AuthorAccountID: author, TextInput: appcontent.TextInput{Text: text}, Visibility: v})
}

func (e *postingEnv) setMaxLength(t *testing.T, n string) {
	t.Helper()
	if _, err := e.instance.SetSetting(t.Context(), appinstance.SetSettingInput{Key: instance.SettingMaxStatusLength, Value: n}); err != nil {
		t.Fatalf("SetSetting(max_status_length, %s) = %v", n, err)
	}
}

// head returns the head of a status.
func (e *postingEnv) head(t *testing.T, statusID string) content.Head {
	t.Helper()
	var h content.Head
	err := e.H.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		var err error
		h, err = e.repo.Head(t.Context(), tx, statusID)
		return err
	})
	if err != nil {
		t.Fatalf("Head(%s) = %v", statusID, err)
	}
	return h
}

// mentionsOf returns the accounts that status_mention_index holds for a status.
func (e *postingEnv) mentionsOf(t *testing.T, statusID string) []string {
	t.Helper()
	all, err := e.repo.ListMentions(t.Context())
	if err != nil {
		t.Fatalf("ListMentions = %v", err)
	}
	var out []string
	for _, m := range all {
		if m.StatusID == statusID {
			out = append(out, m.AccountID)
		}
	}
	return out
}

// EventTypes returns the types of events, for messages.
func EventTypes(events []uow.Event) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.EventType())
	}
	return out
}

// postingRendersProfileText checks that profile text renders through the rules of posts.
func postingRendersProfileText(t *testing.T, e *postingEnv) {
	e.user(t, "Bob")
	html, err := e.content.RenderProfileText(t.Context(), "hi @bob <b>\n\n#Go https://example.com")
	if err != nil {
		t.Fatalf("RenderProfileText = %v", err)
	}
	for _, want := range []string{
		`<a href="https://` + testkit.ContractHost + `/@Bob" class="u-url mention"`, "&lt;b&gt;", "</p><p>",
		`class="mention hashtag"`, `<a href="https://example.com"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("RenderProfileText = %s, want it to contain %s", html, want)
		}
	}
}

// postingMentionsRemote checks mentions of remote accounts: a stored one links to its profile
// page without a resolution, an unknown one is resolved outside the transaction, one that does
// not resolve stays text, and at most MaxResolvedMentions handles of a text are resolved.
func postingMentionsRemote(t *testing.T, e *postingEnv) {
	ctx := t.Context()
	alice := e.user(t, "alice")
	carol, err := e.remote.accounts.SaveRemoteActor(ctx, e.remote.serve("Carol", "remote.example", "https://remote.example/@Carol"))
	if err != nil {
		t.Fatalf("SaveRemoteActor = %v", err)
	}
	e.remote.serve("dave", "other.example", "")
	e.remote.takeCalls()

	p := e.postText(t, alice, "hi @carol@Remote.Example, @dave@other.example and @nobody@nowhere.example", content.VisibilityPublic)
	if calls := e.remote.takeCalls(); !reflect.DeepEqual(calls, []string{"dave@other.example", "nobody@nowhere.example"}) {
		t.Errorf("resolved %v, want the unknown handles only", calls)
	}
	dave, err := e.Accounts.FindByHandle(ctx, "dave", "other.example")
	if err != nil {
		t.Fatalf("FindByHandle(dave@other.example) = %v", err)
	}
	want := []string{carol.Account.ID, dave.Account.ID}
	slices.Sort(want)
	if !reflect.DeepEqual(p.Revision.MentionedAccountIDs, want) {
		t.Errorf("mentions = %v, want carol and dave %v", p.Revision.MentionedAccountIDs, want)
	}
	if m := e.mentionsOf(t, p.Status.ID); !reflect.DeepEqual(m, want) {
		t.Errorf("mention index = %v, want %v", m, want)
	}
	html := p.Revision.Bodies[0].ContentHTML
	for _, s := range []string{
		`<span class="h-card"><a href="https://remote.example/@Carol" class="u-url mention" rel="nofollow noopener noreferrer" target="_blank">@<span>Carol</span></a></span>`,
		// No url: the link is the actor's URI.
		`<a href="https://other.example/users/dave" class="u-url mention"`,
		"@nobody@nowhere.example",
	} {
		if !strings.Contains(html, s) {
			t.Errorf("HTML %s, want it to contain %s", html, s)
		}
	}
	e.remote.mu.Lock()
	inTx, unbound := e.remote.inTxRuns, e.remote.unbound
	e.remote.mu.Unlock()
	if inTx != 0 || unbound != 0 {
		t.Errorf("%d resolutions ran inside a transaction and %d without a deadline, want none", inTx, unbound)
	}

	// The unknown handles of one text are bounded.
	var text []string
	for i := range appcontent.MaxResolvedMentions + 2 {
		text = append(text, "@u"+strconv.Itoa(i)+"@far.example")
	}
	p = e.postText(t, alice, strings.Join(text, " "), content.VisibilityPublic)
	if calls := e.remote.takeCalls(); len(calls) != appcontent.MaxResolvedMentions {
		t.Errorf("resolved %v, want the first %d handles", calls, appcontent.MaxResolvedMentions)
	}
	if len(p.Revision.MentionedAccountIDs) != 0 {
		t.Errorf("mentions = %v, want none", p.Revision.MentionedAccountIDs)
	}

	// A remote account that is not active stays text.
	err = e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.remote.accounts.MarkRemoteDeleted(ctx, tx, carol.Account.ID) })
	if err != nil {
		t.Fatalf("MarkRemoteDeleted = %v", err)
	}
	p = e.postText(t, alice, "bye @carol@remote.example", content.VisibilityPublic)
	if calls := e.remote.takeCalls(); len(calls) != 0 {
		t.Errorf("resolved %v, want no resolution of a stored handle", calls)
	}
	if len(p.Revision.MentionedAccountIDs) != 0 || strings.Contains(p.Revision.Bodies[0].ContentHTML, "<a") {
		t.Errorf("mention of a deleted account = %v %s, want text", p.Revision.MentionedAccountIDs, p.Revision.Bodies[0].ContentHTML)
	}
}

func postingPosts(t *testing.T, e *postingEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice"), e.user(t, "Bob")
	e.events.Take()
	p := e.post(t, appcontent.CreateInput{
		AuthorAccountID: alice,
		TextInput: appcontent.TextInput{
			Text:        "hi @bob and @bob@example.org, @carol and @dave@remote.example\n#Go #ＧＯ <b>https://example.com</b>",
			SpoilerText: "cw", Sensitive: true, Language: "ja_jp",
		},
		Visibility:     content.VisibilityUnlisted,
		ClientID:       testkit.NamedID("client-1"),
		IdempotencyKey: "key-1",
	})
	st, rev := p.Status, p.Revision
	// A local status's page is derived on read, not stored (ADR-0018).
	if st.URI != content.StatusURI(testkit.ContractHost, st.ID) || st.URL != "" {
		t.Errorf("status URIs = %q, %q", st.URI, st.URL)
	}
	if st.AuthorAccountID != alice || !st.IsLocal || st.Visibility != content.VisibilityUnlisted || st.InReplyToURI != "" ||
		st.ConversationURI != st.URI || !st.PublishedAt.Equal(e.Clock.Now()) {
		t.Errorf("status = %+v", st)
	}
	if rev.Seq != 1 || rev.SpoilerText != "cw" || !rev.Sensitive || rev.Language != "ja-JP" || !rev.RevisedAt.Equal(st.PublishedAt) {
		t.Errorf("revision = %+v", rev)
	}
	if rev.Source == nil || rev.Source.Format != content.SourceTextPlain || !strings.HasPrefix(rev.Source.Text, "hi @bob") {
		t.Errorf("source = %+v, want the text as typed", rev.Source)
	}
	if !reflect.DeepEqual(rev.MentionedAccountIDs, []string{bob}) {
		t.Errorf("mentions = %v, want bob once", rev.MentionedAccountIDs)
	}
	bobLink := `<a href="https://` + testkit.ContractHost + `/@Bob" class="u-url mention"`
	if len(rev.Bodies) != 1 || rev.Bodies[0].Language != "ja-JP" {
		t.Fatalf("bodies = %+v, want one in ja-JP", rev.Bodies)
	}
	html := rev.Bodies[0].ContentHTML
	for _, want := range []string{
		bobLink, "@<span>Bob</span>", "@carol", "@dave@remote.example", "&lt;b&gt;",
		`href="https://` + testkit.ContractHost + `/tags/go" class="mention hashtag"`, `<a href="https://example.com"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML %s does not contain %s", html, want)
		}
	}
	if strings.Count(html, bobLink) != 2 {
		t.Errorf("HTML %s should link both mentions of bob", html)
	}
	if len(rev.TagIDs) != 1 {
		t.Fatalf("tags = %v, want one for #Go and #ＧＯ", rev.TagIDs)
	}
	tag, err := e.repo.FindTagByName(ctx, "go")
	if err != nil || tag.ID != rev.TagIDs[0] || tag.Name != "Go" {
		t.Errorf("tag = %+v, %v; want #Go", tag, err)
	}

	got, err := e.content.Find(ctx, st.ID)
	if err != nil || got.Status != st || got.Revision.ID != rev.ID || got.Revision.Bodies[0].ContentHTML != html {
		t.Errorf("Find = %+v, %v; want the posted status", got, err)
	}
	ls, err := e.repo.FindLocalStatus(ctx, st.ID)
	if err != nil || ls != (content.LocalStatus{StatusID: st.ID, AuthorAccountID: alice, ClientID: testkit.NamedID("client-1"), IdempotencyKey: "key-1"}) {
		t.Errorf("local status = %+v, %v", ls, err)
	}
	if h := e.head(t, st.ID); h.RevisionID != rev.ID || h.Edited() {
		t.Errorf("head = %+v, want revision 1", h)
	}
	if m := e.mentionsOf(t, st.ID); !reflect.DeepEqual(m, []string{bob}) {
		t.Errorf("mention index = %v, want bob", m)
	}
	if c, err := e.repo.FindPostCounter(ctx, alice); err != nil || c.StatusesCount != 1 {
		t.Errorf("post counter = %+v, %v; want 1", c, err)
	}
	events := e.events.Take()
	if len(events) != 1 {
		t.Fatalf("events = %v, want one", EventTypes(events))
	}
	if ev, ok := events[0].(appcontent.StatusCreated); !ok || ev.Status != st || ev.Revision.ID != rev.ID || !reflect.DeepEqual(ev.Revision.TagIDs, rev.TagIDs) {
		t.Errorf("event = %+v, want StatusCreated of the status", events[0])
	}

	// A later post reuses the stored tag.
	p2 := e.postText(t, bob, "#go again", content.VisibilityPublic)
	if !reflect.DeepEqual(p2.Revision.TagIDs, rev.TagIDs) {
		t.Errorf("tags of the second post = %v, want %v", p2.Revision.TagIDs, rev.TagIDs)
	}
}

func postingIdempotent(t *testing.T, e *postingEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	in := appcontent.CreateInput{AuthorAccountID: alice, TextInput: appcontent.TextInput{Text: "once"}, Visibility: content.VisibilityPublic, IdempotencyKey: "k"}
	first := e.post(t, in)
	e.events.Take()
	in.Text = "a retry that differs"
	again := e.post(t, in)
	if again.Status != first.Status || again.Revision.ID != first.Revision.ID {
		t.Errorf("retry = %+v, want the first post %+v", again.Status, first.Status)
	}
	if ev := e.events.Take(); len(ev) != 0 {
		t.Errorf("retry emitted %v, want nothing", EventTypes(ev))
	}
	if c, _ := e.repo.FindPostCounter(ctx, alice); c.StatusesCount != 1 {
		t.Errorf("statuses = %d after a retry, want 1", c.StatusesCount)
	}
	// The key is per author, and posts without a key are never merged.
	in.AuthorAccountID = bob
	if other := e.post(t, in); other.Status.ID == first.Status.ID {
		t.Error("another author's post with the same key returned the first post")
	}
	a := e.postText(t, alice, "no key", content.VisibilityPublic)
	b := e.postText(t, alice, "no key", content.VisibilityPublic)
	if a.Status.ID == b.Status.ID {
		t.Error("two posts without a key are the same post")
	}
	// A retry after the deletion does not post again.
	if _, err := e.content.Delete(ctx, alice, first.Status.ID); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	in.AuthorAccountID = alice
	if _, err := e.content.Create(ctx, in); !errors.Is(err, appcontent.ErrAlreadyDeleted) {
		t.Errorf("retry after deletion = %v, want ErrAlreadyDeleted", err)
	}
}

func postingIdempotentConcurrent(t *testing.T, e *postingEnv) {
	alice := e.user(t, "alice")
	const n = 8
	ids := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			p, err := e.content.Create(t.Context(), appcontent.CreateInput{
				AuthorAccountID: alice, TextInput: appcontent.TextInput{Text: "race"}, Visibility: content.VisibilityPublic, IdempotencyKey: "same",
			})
			ids[i], errs[i] = p.Status.ID, err
		})
	}
	wg.Wait()
	for i := range n {
		if errs[i] != nil || ids[i] != ids[0] {
			t.Errorf("request %d = %s, %v; want %s", i, ids[i], errs[i], ids[0])
		}
	}
	if c, _ := e.repo.FindPostCounter(t.Context(), alice); c.StatusesCount != 1 {
		t.Errorf("statuses = %d, want 1", c.StatusesCount)
	}
}

func postingMaxLength(t *testing.T, e *postingEnv) {
	ctx := t.Context()
	alice := e.user(t, "alice")
	create := func(text, cw string) error {
		_, err := e.content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: alice, TextInput: appcontent.TextInput{Text: text, SpoilerText: cw}, Visibility: content.VisibilityPublic,
		})
		return err
	}
	if err := create(strings.Repeat("あ", 500), ""); err != nil {
		t.Errorf("500 characters with the default limit = %v", err)
	}
	if err := create(strings.Repeat("a", 501), ""); !errors.Is(err, appcontent.ErrTextTooLong) {
		t.Errorf("501 characters = %v, want ErrTextTooLong", err)
	}
	e.setMaxLength(t, "10")
	if err := create("0123456789", ""); err != nil {
		t.Errorf("10 characters with limit 10 = %v", err)
	}
	if err := create("01234", "567890"); !errors.Is(err, appcontent.ErrTextTooLong) {
		t.Errorf("text and content warning of 11 characters = %v, want ErrTextTooLong", err)
	}
	p := e.postText(t, alice, "short", content.VisibilityPublic)
	if _, err := e.content.Edit(ctx, appcontent.EditInput{AccountID: alice, StatusID: p.Status.ID, TextInput: appcontent.TextInput{Text: "much too long"}}); !errors.Is(err, appcontent.ErrTextTooLong) {
		t.Errorf("Edit over the limit = %v, want ErrTextTooLong", err)
	}
}

func postingRejectsInvalid(t *testing.T, e *postingEnv) {
	ctx := t.Context()
	alice := e.user(t, "alice")
	deactivated := e.user(t, "gone")
	if _, err := e.Accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: deactivated, Kind: account.LifecycleDeactivated, InitiatedBy: account.InitiatedBySelf,
	}); err != nil {
		t.Fatalf("RecordLifecycleEvent = %v", err)
	}
	ok := appcontent.CreateInput{AuthorAccountID: alice, TextInput: appcontent.TextInput{Text: "ok"}, Visibility: content.VisibilityPublic}
	for _, tc := range []struct {
		name   string
		modify func(*appcontent.CreateInput)
		want   error
	}{
		{"empty text", func(in *appcontent.CreateInput) { in.Text = "" }, appcontent.ErrEmptyText},
		{"blank text", func(in *appcontent.CreateInput) { in.Text = " \n\t" }, appcontent.ErrEmptyText},
		{"NUL", func(in *appcontent.CreateInput) { in.Text = "a\x00b" }, appcontent.ErrInvalidText},
		{"invalid UTF-8", func(in *appcontent.CreateInput) { in.Text = "a\xffb" }, appcontent.ErrInvalidText},
		{"NUL in content warning", func(in *appcontent.CreateInput) { in.SpoilerText = "\x00" }, appcontent.ErrInvalidText},
		{"NUL in the idempotency key", func(in *appcontent.CreateInput) { in.IdempotencyKey = "k\x00" }, content.ErrInvalidIdempotencyKey},
		{"invalid UTF-8 in the idempotency key", func(in *appcontent.CreateInput) { in.IdempotencyKey = "k\xff" }, content.ErrInvalidIdempotencyKey},
		{"long idempotency key", func(in *appcontent.CreateInput) {
			in.IdempotencyKey = strings.Repeat("k", content.MaxIdempotencyKeyLength+1)
		}, content.ErrInvalidIdempotencyKey},
		{"visibility", func(in *appcontent.CreateInput) { in.Visibility = "everyone" }, content.ErrInvalidVisibility},
		{"language", func(in *appcontent.CreateInput) { in.Language = "not a tag" }, appcontent.ErrInvalidLanguage},
		{"unknown author", func(in *appcontent.CreateInput) { in.AuthorAccountID = testkit.NamedID("missing") }, appcontent.ErrAccountUnavailable},
		{"deactivated author", func(in *appcontent.CreateInput) { in.AuthorAccountID = deactivated }, appcontent.ErrAccountUnavailable},
		{"unknown reply target", func(in *appcontent.CreateInput) { in.InReplyToID = testkit.NamedID("missing") }, appcontent.ErrReplyTargetNotFound},
	} {
		in := ok
		tc.modify(&in)
		if _, err := e.content.Create(ctx, in); !errors.Is(err, tc.want) {
			t.Errorf("%s: Create = %v, want %v", tc.name, err, tc.want)
		}
	}
	if c, err := e.repo.FindPostCounter(ctx, alice); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("post counter = %+v, %v; want no statuses", c, err)
	}
}

func postingRestrictions(t *testing.T, e *postingEnv) {
	ctx := t.Context()
	alice := e.user(t, "alice")
	if l, err := e.content.PostingLimits(ctx, alice); err != nil || l.VisibilityCeiling != content.VisibilityPublic || !slices.Equal(l.Visibilities, content.Visibilities()) {
		t.Errorf("PostingLimits without restrictions = %+v, %v; want every visibility", l, err)
	}
	e.restrictions.set(alice, appcontent.Restrictions{Author: content.AuthorRestrictions{VisibilityCeilingPrivate: true}})
	if l, err := e.content.PostingLimits(ctx, alice); err != nil || l.VisibilityCeiling != content.VisibilityPrivate ||
		!slices.Equal(l.Visibilities, []content.Visibility{content.VisibilityPrivate, content.VisibilityDirect}) {
		t.Errorf("PostingLimits under the ceiling = %+v, %v; want private and direct", l, err)
	}
	for _, tc := range []struct{ requested, want content.Visibility }{
		{content.VisibilityPublic, content.VisibilityPrivate},
		{content.VisibilityLocal, content.VisibilityPrivate},
		{content.VisibilityUnlisted, content.VisibilityPrivate},
		{content.VisibilityPrivate, content.VisibilityPrivate},
		{content.VisibilityDirect, content.VisibilityDirect},
	} {
		if p := e.postText(t, alice, "silenced", tc.requested); p.Status.Visibility != tc.want {
			t.Errorf("visibility %s under the ceiling = %s, want %s", tc.requested, p.Status.Visibility, tc.want)
		}
	}
	p := e.postText(t, alice, "before", content.VisibilityPublic)
	e.restrictions.set(alice, appcontent.Restrictions{Post: true})
	if _, err := e.content.Create(ctx, appcontent.CreateInput{AuthorAccountID: alice, TextInput: appcontent.TextInput{Text: "x"}, Visibility: content.VisibilityPublic}); !errors.Is(err, appcontent.ErrPostingRestricted) {
		t.Errorf("Create under the post restriction = %v, want ErrPostingRestricted", err)
	}
	if _, err := e.content.Edit(ctx, appcontent.EditInput{AccountID: alice, StatusID: p.Status.ID, TextInput: appcontent.TextInput{Text: "x"}}); !errors.Is(err, appcontent.ErrPostingRestricted) {
		t.Errorf("Edit under the post restriction = %v, want ErrPostingRestricted", err)
	}
	if _, err := e.content.Delete(ctx, alice, p.Status.ID); err != nil {
		t.Errorf("Delete under the post restriction = %v, want allowed", err)
	}

	// The interact restriction forbids replies, not posts.
	root := e.postText(t, e.user(t, "bob"), "root", content.VisibilityPublic)
	e.restrictions.set(alice, appcontent.Restrictions{Interact: true})
	if _, err := e.content.Create(ctx, appcontent.CreateInput{AuthorAccountID: alice, TextInput: appcontent.TextInput{Text: "re"}, Visibility: content.VisibilityPublic, InReplyToID: root.Status.ID}); !errors.Is(err, appcontent.ErrInteractionRestricted) {
		t.Errorf("reply under the interact restriction = %v, want ErrInteractionRestricted", err)
	}
	e.postText(t, alice, "still posts", content.VisibilityPublic)
}

func postingReplies(t *testing.T, e *postingEnv) {
	ctx := t.Context()
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	root := e.postText(t, alice, "root", content.VisibilityPublic)
	reply := e.post(t, appcontent.CreateInput{AuthorAccountID: bob, TextInput: appcontent.TextInput{Text: "reply"}, Visibility: content.VisibilityPublic, InReplyToID: root.Status.ID})
	if reply.Status.InReplyToURI != root.Status.URI || reply.Status.ConversationURI != root.Status.URI {
		t.Errorf("reply = %+v, want in reply to and in the conversation of %s", reply.Status, root.Status.URI)
	}
	nested := e.post(t, appcontent.CreateInput{AuthorAccountID: carol, TextInput: appcontent.TextInput{Text: "nested"}, Visibility: content.VisibilityPublic, InReplyToID: reply.Status.ID})
	if nested.Status.InReplyToURI != reply.Status.URI || nested.Status.ConversationURI != root.Status.URI {
		t.Errorf("nested reply = %+v, want the root's conversation", nested.Status)
	}
	if c, err := e.repo.FindCounters(ctx, root.Status.ID); err != nil || c.RepliesCount != 1 {
		t.Errorf("root counters = %+v, %v; want 1 reply", c, err)
	}

	replyTo := func(author, target string) error {
		_, err := e.content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: author, TextInput: appcontent.TextInput{Text: "re"}, Visibility: content.VisibilityPrivate, InReplyToID: target,
		})
		return err
	}
	private := e.postText(t, alice, "followers only", content.VisibilityPrivate)
	if err := replyTo(bob, private.Status.ID); !errors.Is(err, appcontent.ErrReplyTargetNotFound) {
		t.Errorf("reply to a hidden private status = %v, want ErrReplyTargetNotFound", err)
	}
	if err := replyTo(alice, private.Status.ID); err != nil {
		t.Errorf("reply to one's own private status = %v", err)
	}
	if _, err := e.graph.Follow(ctx, bob, alice); err != nil {
		t.Fatalf("Follow = %v", err)
	}
	if err := replyTo(bob, private.Status.ID); err != nil {
		t.Errorf("follower's reply to a private status = %v", err)
	}
	direct := e.postText(t, alice, "@carol secret", content.VisibilityDirect)
	if err := replyTo(bob, direct.Status.ID); !errors.Is(err, appcontent.ErrReplyTargetNotFound) {
		t.Errorf("reply to a direct status without the mention = %v, want ErrReplyTargetNotFound", err)
	}
	if err := replyTo(carol, direct.Status.ID); err != nil {
		t.Errorf("mentioned account's reply to a direct status = %v", err)
	}
	if _, err := e.graph.Block(ctx, alice, carol); err != nil {
		t.Fatalf("Block = %v", err)
	}
	if err := replyTo(carol, root.Status.ID); !errors.Is(err, appcontent.ErrReplyTargetNotFound) {
		t.Errorf("reply to a public status of a blocking author = %v, want ErrReplyTargetNotFound", err)
	}
	if _, err := e.content.Delete(ctx, alice, root.Status.ID); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	if err := replyTo(bob, root.Status.ID); !errors.Is(err, appcontent.ErrReplyTargetNotFound) {
		t.Errorf("reply to a deleted status = %v, want ErrReplyTargetNotFound", err)
	}
}

func postingDefaultLanguage(t *testing.T, e *postingEnv) {
	reg := e.Register(t, "alice")
	if p := e.postText(t, reg.AccountID, "no preference", content.VisibilityPublic); p.Revision.Language != appcontent.UndeterminedLanguage {
		t.Errorf("language without a default = %q, want und", p.Revision.Language)
	}
	e.languages[reg.User.ID] = "ja"
	if p := e.postText(t, reg.AccountID, "default", content.VisibilityPublic); p.Revision.Language != "ja" || p.Revision.Bodies[0].Language != "ja" {
		t.Errorf("language = %q, want the default ja", p.Revision.Language)
	}
	p := e.post(t, appcontent.CreateInput{AuthorAccountID: reg.AccountID, TextInput: appcontent.TextInput{Text: "explicit", Language: "en"}, Visibility: content.VisibilityPublic})
	if p.Revision.Language != "en" {
		t.Errorf("language = %q, want the requested en", p.Revision.Language)
	}
}

func postingEdits(t *testing.T, e *postingEnv) {
	ctx := t.Context()
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	p := e.postText(t, alice, "hello @bob #one", content.VisibilityPublic)
	e.events.Take()

	e.Clock.Advance(time.Minute)
	edited, err := e.content.Edit(ctx, appcontent.EditInput{
		AccountID: alice, StatusID: p.Status.ID,
		TextInput: appcontent.TextInput{Text: "hello @carol #two", SpoilerText: "cw", Sensitive: true, Language: "en"},
	})
	if err != nil {
		t.Fatalf("Edit = %v", err)
	}
	rev := edited.Revision
	if edited.Status != p.Status || rev.Seq != 2 || rev.ID == p.Revision.ID || rev.SpoilerText != "cw" || !rev.Sensitive ||
		rev.Language != "en" || !rev.RevisedAt.Equal(e.Clock.Now()) {
		t.Errorf("Edit = %+v", edited)
	}
	if !reflect.DeepEqual(rev.MentionedAccountIDs, []string{carol}) || !strings.Contains(rev.Bodies[0].ContentHTML, "#<span>two</span>") {
		t.Errorf("edited revision = %+v, want carol and #two", rev)
	}
	if h := e.head(t, p.Status.ID); h.RevisionID != rev.ID || h.Seq != 2 || !h.Edited() || !h.LastRevisedAt.Equal(rev.RevisedAt) {
		t.Errorf("head = %+v, want revision 2", h)
	}
	if m := e.mentionsOf(t, p.Status.ID); !reflect.DeepEqual(m, []string{carol}) {
		t.Errorf("mention index = %v, want carol instead of bob", m)
	}
	if got, err := e.content.Find(ctx, p.Status.ID); err != nil || got.Revision.ID != rev.ID {
		t.Errorf("Find = %+v, %v; want the new revision", got.Revision, err)
	}
	history, err := e.content.Revisions(ctx, p.Status.ID)
	if err != nil || len(history) != 2 || history[0].ID != p.Revision.ID || history[1].ID != rev.ID {
		t.Errorf("Revisions = %+v, %v; want both, oldest first", history, err)
	}
	events := e.events.Take()
	if len(events) != 1 {
		t.Fatalf("events = %v, want one", EventTypes(events))
	}
	if ev, ok := events[0].(appcontent.StatusRevised); !ok || ev.Status != p.Status || ev.Revision.ID != rev.ID {
		t.Errorf("event = %+v, want StatusRevised of the status", events[0])
	}

	if _, err := e.content.Edit(ctx, appcontent.EditInput{AccountID: bob, StatusID: p.Status.ID, TextInput: appcontent.TextInput{Text: "mine"}}); !errors.Is(err, appcontent.ErrNotAuthor) {
		t.Errorf("Edit by another account = %v, want ErrNotAuthor", err)
	}
	if _, err := e.content.Edit(ctx, appcontent.EditInput{AccountID: alice, StatusID: testkit.NamedID("missing"), TextInput: appcontent.TextInput{Text: "x"}}); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Edit of a missing status = %v, want ErrNotFound", err)
	}
	if _, err := e.content.Delete(ctx, alice, p.Status.ID); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	if _, err := e.content.Edit(ctx, appcontent.EditInput{AccountID: alice, StatusID: p.Status.ID, TextInput: appcontent.TextInput{Text: "x"}}); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Edit of a deleted status = %v, want ErrNotFound", err)
	}
}

func postingDeletes(t *testing.T, e *postingEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	p := e.postText(t, alice, "bye @bob", content.VisibilityPublic)
	keep := e.postText(t, alice, "stays", content.VisibilityPublic)
	e.events.Take()

	if _, err := e.content.Delete(ctx, bob, p.Status.ID); !errors.Is(err, appcontent.ErrNotAuthor) {
		t.Errorf("Delete by another account = %v, want ErrNotAuthor", err)
	}
	e.Clock.Advance(time.Minute)
	d, err := e.content.Delete(ctx, alice, p.Status.ID)
	if err != nil {
		t.Fatalf("Delete = %v", err)
	}
	want := content.Deletion{StatusID: p.Status.ID, URI: p.Status.URI, AuthorAccountID: alice, Origin: content.DeletedByAuthor, DeletedAt: e.Clock.Now()}
	if d != want {
		t.Errorf("Delete = %+v, want %+v", d, want)
	}
	if got, err := e.repo.FindDeletionByURI(ctx, p.Status.URI); err != nil || got.StatusID != p.Status.ID {
		t.Errorf("tombstone = %+v, %v", got, err)
	}
	if _, err := e.content.Find(ctx, p.Status.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Find after deletion = %v, want ErrNotFound", err)
	}
	if _, _, err := e.content.LiveRevision(ctx, keep.Status.ID, keep.Revision.ID); err != nil {
		t.Errorf("LiveRevision of a live status = %v", err)
	}
	if _, _, err := e.content.LiveRevision(ctx, keep.Status.ID, p.Revision.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("LiveRevision with another status's revision = %v, want ErrNotFound", err)
	}
	if _, _, err := e.content.LiveRevision(ctx, p.Status.ID, p.Revision.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("LiveRevision after deletion = %v, want ErrNotFound", err)
	}
	if st, err := e.content.FindStatus(ctx, p.Status.ID); err != nil || st.URI != p.Status.URI {
		t.Errorf("FindStatus after deletion = %+v, %v; want the status", st, err)
	}
	if m := e.mentionsOf(t, p.Status.ID); len(m) != 0 {
		t.Errorf("mention index = %v after deletion, want none", m)
	}
	if c, err := e.repo.FindPostCounter(ctx, alice); err != nil || c.StatusesCount != 1 || !c.LastStatusAt.Equal(keep.Status.PublishedAt) {
		t.Errorf("post counter = %+v, %v; want the remaining status", c, err)
	}
	events := e.events.Take()
	if len(events) != 1 {
		t.Fatalf("events = %v, want one", EventTypes(events))
	}
	if ev, ok := events[0].(appcontent.StatusDeleted); !ok || ev.Status != p.Status || ev.Deletion != d {
		t.Errorf("event = %+v, want StatusDeleted of the status", events[0])
	}
	if _, err := e.content.Delete(ctx, alice, p.Status.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Delete again = %v, want ErrNotFound", err)
	}
	if ev := e.events.Take(); len(ev) != 0 {
		t.Errorf("a second deletion emitted %v", EventTypes(ev))
	}
}

// upload stores a media file of owner with a processing result of outcome, or none when
// outcome is "".
func (e *postingEnv) upload(t *testing.T, owner string, outcome content.MediaOutcome) string {
	t.Helper()
	ctx := t.Context()
	e.Clock.Advance(time.Second)
	id := e.IDs.NewID()
	err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		err := e.media.InsertMediaFile(ctx, tx, content.MediaFile{ID: id, OwnerAccountID: owner, Origin: content.MediaOriginUpload, CreatedAt: e.Clock.Now()})
		if err != nil || outcome == "" {
			return err
		}
		r := content.MediaProcessingResult{ID: e.IDs.NewID(), MediaID: id, Purpose: content.PurposeAttachment, Outcome: outcome, MediaType: content.MediaTypeUnknown, ProcessedAt: e.Clock.Now()}
		if outcome == content.MediaReady {
			r.MediaType, r.Width, r.Height = content.MediaTypeImage, 4, 3
		}
		return e.media.InsertProcessingResult(ctx, tx, r)
	})
	if err != nil {
		t.Fatalf("upload = %v", err)
	}
	return id
}

func postingAttachesMedia(t *testing.T, e *postingEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	first, second := e.upload(t, alice, content.MediaReady), e.upload(t, alice, content.MediaReady)
	p := e.post(t, appcontent.CreateInput{
		AuthorAccountID: alice, Visibility: content.VisibilityPublic,
		TextInput: appcontent.TextInput{Text: "pictures", Media: []appcontent.MediaInput{{MediaID: second, Description: "a cat"}, {MediaID: first}}},
	})
	want := []content.Attachment{{Position: 0, MediaID: second, Description: "a cat"}, {Position: 1, MediaID: first}}
	if !reflect.DeepEqual(p.Revision.Attachments, want) {
		t.Errorf("attachments = %+v, want %+v", p.Revision.Attachments, want)
	}
	if got, err := e.content.Find(ctx, p.Status.ID); err != nil || !reflect.DeepEqual(got.Revision.Attachments, want) {
		t.Errorf("stored attachments = %+v, %v; want %+v", got.Revision.Attachments, err, want)
	}

	five := make([]appcontent.MediaInput, appcontent.MaxAttachments+1)
	for i := range five {
		five[i].MediaID = e.upload(t, alice, content.MediaReady)
	}
	for name, tc := range map[string]struct {
		media []appcontent.MediaInput
		want  error
	}{
		"another account's file": {[]appcontent.MediaInput{{MediaID: e.upload(t, bob, content.MediaReady)}}, appcontent.ErrMediaNotFound},
		"a missing file":         {[]appcontent.MediaInput{{MediaID: e.IDs.NewID()}}, appcontent.ErrMediaNotFound},
		"a file in processing":   {[]appcontent.MediaInput{{MediaID: e.upload(t, alice, "")}}, appcontent.ErrMediaNotReady},
		"a failed file":          {[]appcontent.MediaInput{{MediaID: e.upload(t, alice, content.MediaFailed)}}, appcontent.ErrMediaNotReady},
		"too many files":         {five, appcontent.ErrTooManyMedia},
		"a file twice":           {[]appcontent.MediaInput{{MediaID: first}, {MediaID: first}}, appcontent.ErrTooManyMedia},
		"a long description":     {[]appcontent.MediaInput{{MediaID: first, Description: strings.Repeat("a", appcontent.MaxMediaDescription+1)}}, appcontent.ErrInvalidMediaDescription},
	} {
		_, err := e.content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: alice, Visibility: content.VisibilityPublic, TextInput: appcontent.TextInput{Text: "x", Media: tc.media},
		})
		if !errors.Is(err, tc.want) {
			t.Errorf("Create with %s = %v, want %v", name, err, tc.want)
		}
		_, err = e.content.Edit(ctx, appcontent.EditInput{AccountID: alice, StatusID: p.Status.ID, TextInput: appcontent.TextInput{Text: "x", Media: tc.media}})
		if !errors.Is(err, tc.want) {
			t.Errorf("Edit with %s = %v, want %v", name, err, tc.want)
		}
	}

	edited, err := e.content.Edit(ctx, appcontent.EditInput{
		AccountID: alice, StatusID: p.Status.ID, TextInput: appcontent.TextInput{Text: "one picture", Media: []appcontent.MediaInput{{MediaID: first, Description: "a dog"}}},
	})
	if err != nil || !reflect.DeepEqual(edited.Revision.Attachments, []content.Attachment{{Position: 0, MediaID: first, Description: "a dog"}}) {
		t.Errorf("Edit = %+v, %v; want the one attachment", edited.Revision.Attachments, err)
	}
}

// postingRequiresDescriptions checks the require_media_description setting (FR-105): while it is
// on, a post or an edit with a media file without an alt text, or with one of only spaces, fails,
// and one whose every file has an alt text succeeds; turning it off accepts files without one.
func postingRequiresDescriptions(t *testing.T, e *postingEnv) {
	ctx := t.Context()
	alice := e.user(t, "alice")
	first, second := e.upload(t, alice, content.MediaReady), e.upload(t, alice, content.MediaReady)
	p := e.post(t, appcontent.CreateInput{
		AuthorAccountID: alice, Visibility: content.VisibilityPublic,
		TextInput: appcontent.TextInput{Text: "before", Media: []appcontent.MediaInput{{MediaID: first}}},
	})
	set := func(v string) {
		t.Helper()
		if _, err := e.instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingRequireMediaDescription, Value: v}); err != nil {
			t.Fatalf("SetSetting(require_media_description, %s) = %v", v, err)
		}
	}
	set("true")
	for name, media := range map[string][]appcontent.MediaInput{
		"no description":          {{MediaID: first}},
		"a blank description":     {{MediaID: first, Description: " \n\t"}},
		"one file of two without": {{MediaID: first, Description: "a cat"}, {MediaID: second}},
	} {
		_, err := e.content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: alice, Visibility: content.VisibilityPublic, TextInput: appcontent.TextInput{Text: "x", Media: media},
		})
		if !errors.Is(err, appcontent.ErrMissingMediaDescription) {
			t.Errorf("Create with %s = %v, want ErrMissingMediaDescription", name, err)
		}
		_, err = e.content.Edit(ctx, appcontent.EditInput{AccountID: alice, StatusID: p.Status.ID, TextInput: appcontent.TextInput{Text: "x", Media: media}})
		if !errors.Is(err, appcontent.ErrMissingMediaDescription) {
			t.Errorf("Edit with %s = %v, want ErrMissingMediaDescription", name, err)
		}
	}
	described := []appcontent.MediaInput{{MediaID: first, Description: "a cat"}, {MediaID: second, Description: "a dog"}}
	if _, err := e.content.Edit(ctx, appcontent.EditInput{AccountID: alice, StatusID: p.Status.ID, TextInput: appcontent.TextInput{Text: "described", Media: described}}); err != nil {
		t.Errorf("Edit with descriptions = %v", err)
	}
	e.post(t, appcontent.CreateInput{AuthorAccountID: alice, Visibility: content.VisibilityPublic, TextInput: appcontent.TextInput{Text: "described", Media: described}})
	e.postText(t, alice, "no media", content.VisibilityPublic)

	set("false")
	e.post(t, appcontent.CreateInput{
		AuthorAccountID: alice, Visibility: content.VisibilityPublic,
		TextInput: appcontent.TextInput{Text: "after", Media: []appcontent.MediaInput{{MediaID: second}}},
	})
}
