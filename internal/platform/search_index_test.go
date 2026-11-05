// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/search/fake"
	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// searchEnv is a server whose search backend is the fake, whose jobs run when a test drains them.
type searchEnv struct {
	*accountsEnv
	index *fake.Index
}

func newSearchEnv(t *testing.T) *searchEnv {
	t.Helper()
	index := fake.New()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), SearchIndex: index})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return &searchEnv{accountsEnv: &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}, index: index}
}

func (e *searchEnv) drain() {
	e.t.Helper()
	if _, err := e.app.Runner.Drain(e.t.Context()); err != nil {
		e.t.Fatalf("Drain: %v", err)
	}
}

// found returns the status IDs the index finds for word, after the jobs ran.
func (e *searchEnv) found(word string) []string {
	e.t.Helper()
	e.drain()
	page, err := e.index.Search(e.t.Context(), search.Query{Text: word, Limit: 100})
	if err != nil {
		e.t.Fatal(err)
	}
	return page.StatusIDs
}

func (e *searchEnv) wantFound(word string, ids ...string) {
	e.t.Helper()
	got := e.found(word)
	slices.Sort(got)
	want := slices.Sorted(slices.Values(ids))
	if !slices.Equal(got, want) {
		e.t.Errorf("found(%q) = %v, want %v", word, got, want)
	}
}

func (e *searchEnv) setIndexable(u testUser, on bool) {
	e.t.Helper()
	e.want(e.do(http.MethodPatch, "/accounts/update_credentials", u.token, map[string]any{"indexable": on}, nil), http.StatusOK, "update_credentials")
}

func (e *searchEnv) post(u testUser, text, visibility string) string {
	e.t.Helper()
	var st clientapi.Status
	e.want(e.do(http.MethodPost, "/statuses", u.token, map[string]any{"text": text, "visibility": visibility}, &st), http.StatusCreated, "POST /statuses")
	return st.Id
}

func (e *searchEnv) setVisibilities(value string) {
	e.t.Helper()
	if _, err := e.app.Instance.SetSetting(e.t.Context(), appinstance.SetSettingInput{Key: instance.SettingSearchVisibilities, Value: value}); err != nil {
		e.t.Fatalf("SetSetting(%s): %v", value, err)
	}
}

func (e *searchEnv) remoteActor(uri, domain, username string, indexable bool) string {
	e.t.Helper()
	ctx := e.t.Context()
	var a account.Account
	err := e.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		a, err = e.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: uri, Domain: domain, Type: "Person", PreferredUsername: username, Indexable: indexable,
			InboxURL: uri + "/inbox", Document: []byte(`{"id":"` + uri + `"}`),
			Handle: appaccount.HandleCheck{Outcome: account.HandleVerified, Username: username, Domain: domain},
		})
		return err
	})
	if err != nil {
		e.t.Fatalf("UpsertRemoteActor: %v", err)
	}
	return a.ID
}

func (e *searchEnv) remoteStatus(actorURI, n, accountID, domain, html string) string {
	e.t.Helper()
	ctx := e.t.Context()
	var id string
	at := time.Now().UTC()
	err := e.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		p, err := e.app.Content.IngestRemoteStatus(ctx, tx, appcontent.RemoteStatusInput{
			URI: actorURI + "/statuses/" + n, AuthorAccountID: accountID, AuthorDomain: domain,
			Visibility: domcontent.VisibilityPublic, PublishedAt: at, RevisedAt: at,
			Bodies: []appcontent.RemoteBody{{Language: "und", HTML: html}},
		})
		id = p.Status.ID
		return err
	})
	if err != nil {
		e.t.Fatalf("IngestRemoteStatus: %v", err)
	}
	return id
}

// TestSearchIndexFollowsPosts checks the path of a post: it is indexed when it qualifies, follows an
// edit, and goes with a deletion (ADR-0149 §5, §7).
func TestSearchIndexFollowsPosts(t *testing.T) {
	t.Parallel()
	e := newSearchEnv(t)
	alice := e.user("alice")
	e.setIndexable(alice, true)

	id := e.post(alice, "first armadillo", "public")
	e.wantFound("armadillo", id)

	e.want(e.do(http.MethodPut, "/statuses/"+id, alice.token, map[string]any{"text": "second capybara"}, nil), http.StatusOK, "edit")
	e.wantFound("armadillo")
	e.wantFound("capybara", id)

	e.want(e.do(http.MethodDelete, "/statuses/"+id, alice.token, nil, nil), http.StatusNoContent, "delete")
	e.wantFound("capybara")
}

// TestSearchIndexRefusesOtherVisibilities checks that unlisted, private, and direct posts reach the
// backend under no setting, and enqueue no job (ADR-0149 §5).
func TestSearchIndexRefusesOtherVisibilities(t *testing.T) {
	t.Parallel()
	e := newSearchEnv(t)
	alice := e.user("alice")
	e.setIndexable(alice, true)
	e.setVisibilities(instance.SearchVisibilitiesPublicLocal)
	e.drain()

	for _, v := range []string{"unlisted", "private", "direct"} {
		e.post(alice, "secret okapi", v)
		jobs, err := e.app.JobInspector.ListJobs(t.Context(), time.Now().Add(time.Hour), queue.JobFilter{Type: search.JobSyncStatus}, queue.JobPosition{}, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) != 0 {
			t.Errorf("%s: %d search.sync_status jobs, want none", v, len(jobs))
		}
	}
	e.wantFound("okapi")

	// Even a job that names one of them sends nothing.
	var st clientapi.Status
	e.want(e.do(http.MethodPost, "/statuses", alice.token, map[string]any{"text": "secret tapir", "visibility": "private"}, &st), http.StatusCreated, "post")
	e.syncStatus(st.Id)
	e.wantFound("tapir")
}

// syncStatus runs the job of a status now.
func (e *searchEnv) syncStatus(id string) {
	e.t.Helper()
	b, err := json.Marshal(map[string]string{"status_id": id})
	if err != nil {
		e.t.Fatal(err)
	}
	h, _, ok := e.app.Handlers.Lookup(search.JobSyncStatus)
	if !ok {
		e.t.Fatal("no handler of search.sync_status")
	}
	if res := h(e.t.Context(), queue.Job{ID: "manual", Type: search.JobSyncStatus, Payload: b}); res.Err() != nil {
		e.t.Fatalf("sync_status: %v", res.Err())
	}
}

// TestSearchIndexFollowsIndexable checks that turning indexable off removes all of an author's
// documents and that turning it on brings back the older posts that qualify (ADR-0149 §6).
func TestSearchIndexFollowsIndexable(t *testing.T) {
	t.Parallel()
	e := newSearchEnv(t)
	alice := e.user("alice")

	hidden := e.post(alice, "older wombat", "public")
	e.wantFound("wombat")

	e.setIndexable(alice, true)
	e.post(alice, "private wombat", "private")
	e.wantFound("wombat", hidden)

	e.setIndexable(alice, false)
	e.wantFound("wombat")

	// A profile revision that leaves indexable alone enqueues nothing.
	e.drain()
	e.want(e.do(http.MethodPatch, "/accounts/update_credentials", alice.token, map[string]any{"display_name": "Alice"}, nil), http.StatusOK, "update_credentials")
	jobs, err := e.app.JobInspector.ListJobs(t.Context(), time.Now().Add(time.Hour), queue.JobFilter{Type: search.JobSyncAuthor}, queue.JobPosition{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Errorf("%d search.sync_author jobs after a change of the name, want none", len(jobs))
	}
}

// TestSearchIndexFollowsSanctions checks that a sanction removes an author's documents, that its
// lift brings them back, and that so does its expiry, which raises no action or lift
// (ADR-0149 §7).
func TestSearchIndexFollowsSanctions(t *testing.T) {
	t.Parallel()
	e := newSearchEnv(t)
	ctx := t.Context()
	alice, bob := e.user("alice"), e.user("bob")
	e.setIndexable(alice, true)
	e.setIndexable(bob, true)
	id := e.post(alice, "punished quokka", "public")
	other := e.post(bob, "innocent quokka", "public")
	e.wantFound("quokka", id, other)

	// limit puts the ceiling of private on the author, so the effective visibility stops qualifying.
	act, err := e.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: alice.id, Kind: moderation.ActionLimit, ReasonCategory: moderation.CategoryOther,
	})
	if err != nil {
		t.Fatalf("IssueAccountAction: %v", err)
	}
	e.wantFound("quokka", other)
	if err := e.app.Moderation.LiftAccountAction(ctx, act.ID, "", ""); err != nil {
		t.Fatalf("LiftAccountAction: %v", err)
	}
	e.wantFound("quokka", id, other)

	if _, err := e.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: alice.id, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategoryOther, Duration: 50 * time.Millisecond,
	}); err != nil {
		t.Fatalf("IssueAccountAction: %v", err)
	}
	e.wantFound("quokka", other)
	waitPast(time.Now().Add(60 * time.Millisecond))
	// The rows remain until the expiry job runs: nothing has told the index yet.
	e.wantFound("quokka", other)
	if _, err := e.app.Moderation.ExpireAccountRestrictions(ctx); err != nil {
		t.Fatalf("ExpireAccountRestrictions: %v", err)
	}
	e.wantFound("quokka", id, other)
}

// TestSearchIndexFollowsDomainRestrictions checks that a restriction of a domain reaches the authors
// of the domain and its subdomains, and that its expiry brings them back (ADR-0149 §7).
func TestSearchIndexFollowsDomainRestrictions(t *testing.T) {
	t.Parallel()
	e := newSearchEnv(t)
	ctx := t.Context()
	const carolURI, daveURI, erinURI = "https://remote.example/users/carol", "https://sub.remote.example/users/dave", "https://other.example/users/erin"
	carol := e.remoteActor(carolURI, "remote.example", "carol", true)
	dave := e.remoteActor(daveURI, "sub.remote.example", "dave", true)
	erin := e.remoteActor(erinURI, "other.example", "erin", true)
	shy := e.remoteActor("https://remote.example/users/shy", "remote.example", "shy", false)

	carolPost := e.remoteStatus(carolURI, "1", carol, "remote.example", "<p>hello</p><p>narwhal</p>")
	davePost := e.remoteStatus(daveURI, "1", dave, "sub.remote.example", "<p>narwhal</p>")
	erinPost := e.remoteStatus(erinURI, "1", erin, "other.example", "<p>narwhal</p>")
	e.remoteStatus("https://remote.example/users/shy", "1", shy, "remote.example", "<p>narwhal</p>")
	e.wantFound("narwhal", carolPost, davePost, erinPost)
	// The tags are out of the text, and the words of two paragraphs stay apart.
	e.wantFound("hellonarwhal")
	e.wantFound("p")

	if _, err := e.app.Moderation.IssueDomainAction(ctx, appmoderation.DomainActionInput{
		Domain: "remote.example", Kind: moderation.DomainLimit, Duration: 200 * time.Millisecond,
	}); err != nil {
		t.Fatalf("IssueDomainAction: %v", err)
	}
	e.wantFound("narwhal", erinPost)
	waitPast(time.Now().Add(250 * time.Millisecond))
	if _, err := e.app.Moderation.ExpireDomainRestrictions(ctx); err != nil {
		t.Fatalf("ExpireDomainRestrictions: %v", err)
	}
	e.wantFound("narwhal", carolPost, davePost, erinPost)

	// A remote actor that turns indexable off goes too, through the same revision event.
	if err := e.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := e.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: carolURI, Domain: "remote.example", Type: "Person", PreferredUsername: "carol", Indexable: false,
			InboxURL: carolURI + "/inbox", Document: []byte(`{"id":"` + carolURI + `"}`),
			Handle: appaccount.HandleCheck{Outcome: account.HandleVerified, Username: "carol", Domain: "remote.example"},
		})
		return err
	}); err != nil {
		t.Fatalf("UpsertRemoteActor: %v", err)
	}
	e.wantFound("narwhal", davePost, erinPost)
}

// TestSearchIndexFollowsTheSetting checks that narrowing search_visibilities removes the documents
// that no longer qualify, that widening it indexes only new posts, and that a change is audited
// (ADR-0149 §5). local posts need a setting that includes them.
func TestSearchIndexFollowsTheSetting(t *testing.T) {
	t.Parallel()
	e := newSearchEnv(t)
	ctx := t.Context()
	alice := e.user("alice")
	e.setIndexable(alice, true)

	if got, err := e.app.Search.Visibilities(ctx); err != nil || !slices.Equal(got, []search.Visibility{search.VisibilityPublic}) {
		t.Errorf("Visibilities() = %v, %v, want the default of public", got, err)
	}
	publicPost := e.post(alice, "open ibex", "public")
	localOld := e.post(alice, "local ibex", "local")
	e.wantFound("ibex", publicPost)

	// Widening leaves the older local post out until a rebuild.
	e.setVisibilities(instance.SearchVisibilitiesPublicLocal)
	if got, err := e.app.Search.Visibilities(ctx); err != nil || !slices.Equal(got, []search.Visibility{search.VisibilityPublic, search.VisibilityLocal}) {
		t.Errorf("Visibilities() = %v, %v, want both", got, err)
	}
	e.wantFound("ibex", publicPost)
	localNew := e.post(alice, "newer ibex", "local")
	e.wantFound("ibex", publicPost, localNew)

	// Narrowing removes what it leaves out.
	e.setVisibilities(instance.SearchVisibilitiesLocal)
	e.wantFound("ibex", localNew)
	e.syncStatus(localOld)
	e.wantFound("ibex", localNew, localOld)
	e.syncStatus(publicPost)
	e.wantFound("ibex", localNew, localOld)

	if _, err := e.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingSearchVisibilities, Value: ""}); err == nil {
		t.Error("SetSetting accepted an empty search_visibilities")
	}
	if _, err := e.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingSearchVisibilities, Value: "unlisted"}); err == nil {
		t.Error("SetSetting accepted search_visibilities = unlisted")
	}

	entries, err := e.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(entries, func(a moderation.AuditEntry) bool {
		return a.Action == appmoderation.AuditSettingChanged && a.TargetID == string(instance.SettingSearchVisibilities)
	}) {
		t.Errorf("no audit entry of the change of search_visibilities in %+v", entries)
	}
}

// TestSearchIndexFollowsTheLifecycle checks that deactivating and deleting an account removes its
// documents without waiting for the erasure, under a retention hold too, that a reactivation brings
// them back, and that the erasure itself only enqueues a job (ADR-0149 §11).
func TestSearchIndexFollowsTheLifecycle(t *testing.T) {
	t.Parallel()
	e := newSearchEnv(t)
	ctx := t.Context()
	alice := e.user("alice")
	e.setIndexable(alice, true)
	id := e.post(alice, "active pangolin", "public")
	e.wantFound("pangolin", id)

	if err := e.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := e.app.Accounts.DeactivateTx(ctx, tx, alice.id, "")
		return err
	}); err != nil {
		t.Fatalf("DeactivateTx: %v", err)
	}
	e.wantFound("pangolin")
	if err := e.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.app.Accounts.ReactivateTx(ctx, tx, alice.id) }); err != nil {
		t.Fatalf("ReactivateTx: %v", err)
	}
	e.wantFound("pangolin", id)

	if _, err := e.app.Moderation.PlaceRetentionHold(ctx, alice.id, "legal", ""); err != nil {
		t.Fatalf("PlaceRetentionHold: %v", err)
	}
	if err := e.app.Accounts.DeleteBySelf(ctx, alice.id); err != nil {
		t.Fatalf("DeleteBySelf: %v", err)
	}
	e.wantFound("pangolin")
}

// TestSearchEraserOnlyEnqueues checks the DataEraser of the module: a job, and no call of the engine.
func TestSearchEraserOnlyEnqueues(t *testing.T) {
	t.Parallel()
	e := newSearchEnv(t)
	step, err := e.app.Search.EraseAccountData(t.Context(), appaccount.ErasureTarget{AccountID: "00000000-0000-7000-8000-000000000001"}, "", 100)
	if err != nil || !step.Done {
		t.Fatalf("EraseAccountData = %+v, %v", step, err)
	}
	jobs, err := e.app.JobInspector.ListJobs(t.Context(), time.Now().Add(time.Hour), queue.JobFilter{Type: search.JobSyncAuthor}, queue.JobPosition{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Errorf("%d search.sync_author jobs, want 1", len(jobs))
	}
}
