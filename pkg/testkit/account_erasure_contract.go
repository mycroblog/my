// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/media"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
)

// AccountErasureHarness is one database under test for the erasure of deleted accounts: the
// harness of the retention contract, the modules' erasure repositories on the same database,
// and Rows, which counts the rows of a table whose column equals value.
type AccountErasureHarness struct {
	RetentionHarness
	AccountErasure      appaccount.ErasureRepository
	IdentityErasure     appidentity.ErasureRepository
	ContentErasure      appcontent.ErasureRepository
	GraphErasure        appgraph.ErasureRepository
	TimelineErasure     apptimeline.ErasureRepository
	NotificationErasure appnotification.ErasureRepository
	Rows                func(t *testing.T, table, column, value string) int
}

// AccountErasureFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type AccountErasureFactory func(t *testing.T, opts uow.Options) AccountErasureHarness

// AccountErasureContract verifies the erasure of deleted local accounts (data-model §8) on a
// database adapter, with the real modules: an account deleted longer than the delay ago, without
// a retention hold, loses its personal data (profile, keys, email addresses, credentials,
// sessions, preferences, statuses, favourites, bookmarks, boosts, follows, mutes, domain mutes, uploads,
// notifications, timeline rows) while its row, URI, domain, username, and lifecycle stay; its
// statuses become deletions with origin account_deletion, the other accounts' counters lose it,
// and a status that the moderation records need keeps its rows until they are gone, after which
// the account is recorded as erased once.
func AccountErasureContract(t *testing.T, newHarness AccountErasureFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *erasureEnv)
	}{
		{"ErasesDeletedAccounts", erasureErasesDeletedAccounts},
		{"WaitsForTheDelayAndHolds", erasureWaits},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newErasureEnv(t, newHarness)) })
	}
}

type erasureEnv struct {
	*retentionEnv
	eh    AccountErasureHarness
	blobs *MemoryBlobStore
	media *appcontent.MediaService
}

func newErasureEnv(t *testing.T, newHarness AccountErasureFactory) *erasureEnv {
	t.Helper()
	var eh AccountErasureHarness
	re := newRetentionEnv(t, func(t *testing.T, opts uow.Options) RetentionHarness {
		eh = newHarness(t, opts)
		return eh.RetentionHarness
	})
	blobs := NewMemoryBlobStore(re.clock)
	svc := appcontent.NewMediaService(appcontent.MediaDeps{
		UnitOfWork: eh.UnitOfWork, Repository: eh.Media, Blobs: blobs, Processor: FakeMediaProcessor{},
		Jobs: &recordingJobs{}, Clock: re.clock, IDs: re.ids, Limits: media.Limits{MaxBytes: 1 << 20},
	})
	return &erasureEnv{retentionEnv: re, eh: eh, blobs: blobs, media: svc}
}

// upload uploads and processes an image of the account and returns its media file ID.
func (e *erasureEnv) upload(t *testing.T, accountID string) string {
	t.Helper()
	m, err := e.media.Upload(t.Context(), appcontent.UploadInput{OwnerAccountID: accountID, Content: bytes.NewReader(FixturePNG(4, 3, false))})
	if err != nil {
		t.Fatalf("Upload = %v", err)
	}
	if res := e.media.Process(t.Context(), m.File.ID, ""); !res.IsDone() {
		t.Fatalf("Process = %+v, want Done", res)
	}
	return m.File.ID
}

// blobsOf returns the number of blobs of a media file.
func (e *erasureEnv) blobsOf(mediaID string) int {
	n := 0
	for _, k := range e.blobs.Keys() {
		if strings.Contains(k, mediaID) {
			n++
		}
	}
	return n
}

// stubHolds stands in for the moderation module's retention holds.
type stubHolds []string

// HeldAccountIDs implements appaccount.HeldAccounts.
func (s stubHolds) HeldAccountIDs(context.Context) ([]string, error) { return s, nil }

// eraser returns the erasure with the evidence and holds of moderation, batch rows per
// statement.
func (e *erasureEnv) eraser(evidence appcontent.RetainedStatuses, holds appaccount.HeldAccounts, batch int) *appaccount.Eraser {
	u := e.eh.UnitOfWork
	notifications := appnotification.NewEraser(u, e.eh.NotificationErasure)
	return appaccount.NewEraser(appaccount.ErasureDeps{
		UnitOfWork: u, Repository: e.eh.AccountErasure, Holds: holds, Clock: e.clock, Batch: batch,
		Erasers: []appaccount.DataEraser{
			appcontent.NewStatusEraser(appcontent.ErasureDeps{
				UnitOfWork: u, Repository: e.eh.Statuses, Erasure: e.eh.ContentErasure, Notifications: notifications,
				Evidence: evidence, Clock: e.clock,
			}),
			appcontent.NewInteractionEraser(u, e.eh.ContentErasure),
			appgraph.NewEraser(u, e.eh.Graph, e.eh.GraphErasure, e.clock),
			apptimeline.NewEraser(u, e.eh.TimelineErasure),
			notifications,
			appcontent.NewMediaEraser(appcontent.MediaErasureDeps{UnitOfWork: u, Repository: e.eh.Media, Blobs: e.blobs}),
			appidentity.NewEraser(u, e.eh.IdentityErasure),
		},
	})
}

// run runs the erasure to the end of its run and returns how many accounts it erased.
func (e *erasureEnv) run(t *testing.T, er *appaccount.Eraser) int {
	t.Helper()
	total, after := 0, appaccount.ErasureCursor{}
	for range 100 {
		res, err := er.Run(t.Context(), after)
		if err != nil {
			t.Fatalf("Run(%+v) = %v", after, err)
		}
		total += res.Erased
		if !res.More {
			return total
		}
		after = res.Next
	}
	t.Fatal("the erasure did not reach the end")
	return total
}

// deleteAccount schedules the deletion of the account now and runs the deletion job.
func (e *erasureEnv) deleteAccount(t *testing.T, accountID string) {
	t.Helper()
	ctx := t.Context()
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.accounts.ScheduleDeletionByModeration(ctx, tx, accountID, NamedID("action-ban"), e.clock.Now())
	})
	if err != nil {
		t.Fatalf("ScheduleDeletionByModeration = %v", err)
	}
	if n, err := e.accounts.ExecuteDueDeletions(ctx); err != nil || n != 1 {
		t.Fatalf("ExecuteDueDeletions = %d, %v; want 1", n, err)
	}
}

// rows fails unless the table has want rows whose column is value.
func (e *erasureEnv) rows(t *testing.T, table, column, value string, want int) {
	t.Helper()
	if got := e.eh.Rows(t, table, column, value); got != want {
		t.Errorf("%s rows with %s = %s: %d, want %d", table, column, value, got, want)
	}
}

func (e *erasureEnv) postAs(t *testing.T, authorID, text string) content.Status {
	t.Helper()
	p, err := e.content.Create(t.Context(), appcontent.CreateInput{
		AuthorAccountID: authorID, TextInput: appcontent.TextInput{Text: text}, Visibility: content.VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("Create(%q) = %v", text, err)
	}
	return p.Status
}

func erasureErasesDeletedAccounts(t *testing.T, e *erasureEnv) {
	ctx := t.Context()
	alice := e.sessionUser(t)
	aliceID, userID := alice.AccountID, alice.User.ID
	e.signIn(t, appidentity.SignInInput{})
	e.enableTOTP(t, alice)
	e.registerPasskey(t, userID, "phone")
	bob := e.user(t, "bob")
	carol := e.actor(t, "https://remote.example/users/carol")

	plain := e.postAs(t, aliceID, "plain")
	reported := e.postAs(t, aliceID, "reported")
	gone := e.postAs(t, aliceID, "deleted by alice")
	if _, err := e.content.Delete(ctx, aliceID, gone.ID); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	bobs := e.postAs(t, bob, "bob's")
	aliceUpload, bobUpload := e.upload(t, aliceID), e.upload(t, bob)
	// Bob interacts with alice's statuses, alice with bob's.
	if _, err := e.interactions.Favourite(ctx, bob, plain.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.interactions.Bookmark(ctx, bob, plain.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.interactions.Boost(ctx, bob, plain.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.interactions.Favourite(ctx, aliceID, bobs.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.interactions.Boost(ctx, aliceID, bobs.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.interactions.Bookmark(ctx, aliceID, bobs.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.interactions.Pin(ctx, aliceID, plain.ID); err != nil {
		t.Fatal(err)
	}
	// Carol reacts to alice's status, alice to bob's (as the reaction command of a local account
	// records it: the row and its event).
	if _, err := e.interactions.RecordRemoteReaction(ctx, appcontent.RemoteReactionInput{
		URI: "https://remote.example/users/carol/reactions/1", AccountID: carol, StatusID: plain.ID,
		Emoji: appcontent.RemoteReactionEmoji{Unicode: "👍"},
	}); err != nil {
		t.Fatal(err)
	}
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		r := content.Reaction{
			ID: e.ids.NewID(), AccountID: aliceID, StatusID: bobs.ID, Emoji: content.ReactionEmoji{Unicode: "🎉"},
			URI: "https://" + contractHost + "/ap/activities/alice-reaction", CreatedAt: e.clock.Now(),
		}
		if _, err := e.rh.Interactions.InsertReaction(ctx, tx, r); err != nil {
			return err
		}
		return tx.Emit(ctx, appcontent.Reacted{Reaction: r, Status: appcontent.RefOf(bobs)})
	})
	if err != nil {
		t.Fatal(err)
	}
	e.rows(t, "status_reaction_counts", "status_id", bobs.ID, 1)
	if _, err := e.graph.Follow(ctx, bob, aliceID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.graph.Follow(ctx, aliceID, bob); err != nil {
		t.Fatal(err)
	}
	e.follow(t, aliceID, carol)
	if _, err := e.graph.Mute(ctx, aliceID, bob, appgraph.MuteInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.graph.MuteDomain(ctx, aliceID, "muted.example"); err != nil {
		t.Fatal(err)
	}
	e.rows(t, "feed_domain_mutes", "viewer_account_id", aliceID, 1)
	e.rows(t, "notification_domain_exclusions", "viewer_account_id", aliceID, 1)
	if c, _ := e.graph.Counters(ctx, bob); c.FollowersCount != 1 || c.FollowingCount != 1 {
		t.Fatalf("bob's counters before = %+v", c)
	}
	e.rows(t, "notifications", "recipient_account_id", aliceID, 4) // bob's follow, favourite, boost; carol's reaction
	e.rows(t, "notifications", "source_account_id", aliceID, 4)    // alice's follow, favourite, boost, reaction
	carolActor, err := e.accounts.FindByID(ctx, carol)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.accounts.AddAlias(ctx, aliceID, carolActor); err != nil {
		t.Fatal(err)
	}
	e.rows(t, "account_aliases", "account_id", aliceID, 1)

	e.deleteAccount(t, aliceID)
	e.clock.Advance(appaccount.ErasureDelay)
	evidence := stubEvidence{statuses: map[string]bool{reported.ID: true}}
	// Batches of one row exercise the continuation of every eraser.
	if n := e.run(t, e.eraser(evidence, nil, 1)); n != 0 {
		t.Errorf("erased %d accounts while evidence stays, want 0", n)
	}

	// The account itself stays, without a profile or keys.
	actor, err := e.accounts.FindByID(ctx, aliceID)
	if err != nil {
		t.Fatalf("FindByID = %v", err)
	}
	if actor.Profile != nil || len(actor.Keys) != 0 || actor.Local == nil || actor.Local.Username != "alice" ||
		actor.Lifecycle.State != account.StateDeleted || actor.Account.URI == "" {
		t.Errorf("erased actor = %+v, want the account, its username, and its lifecycle only", actor)
	}
	for _, table := range []string{"account_profile_revisions", "account_profile_heads", "account_keys", "account_aliases", "follow_counters",
		"account_post_counters", "local_post_counters", "mutes", "account_domain_mutes", "favourites", "reactions", "bookmarks", "status_pins",
		"notification_read_markers"} {
		e.rows(t, table, "account_id", aliceID, 0)
	}
	for _, table := range []string{"user_email_addresses", "user_primary_emails", "password_credentials", "sessions",
		"user_preferences", "user_role_badge_displays", "user_time_zones", "user_registration_states", "registration_applications", "totp_enrollments", "passkey_credentials"} {
		e.rows(t, table, "user_id", userID, 0)
	}
	e.rows(t, "local_accounts", "account_id", aliceID, 1)
	e.rows(t, "accounts", "id", aliceID, 1)
	e.rows(t, "users", "id", userID, 1)
	e.rows(t, "account_lifecycle_events", "account_id", aliceID, 2)
	e.rows(t, "follows", "follower_account_id", aliceID, 0)
	e.rows(t, "follows", "followee_account_id", aliceID, 0)
	e.rows(t, "follow_edges", "follower_account_id", aliceID, 0)
	e.rows(t, "follow_edges", "followee_account_id", aliceID, 0)
	e.rows(t, "feed_follow_edges", "follower_account_id", aliceID, 0)
	e.rows(t, "feed_follow_edges", "followee_account_id", aliceID, 0)
	e.rows(t, "mutes", "target_account_id", aliceID, 0)
	e.rows(t, "feed_exclusions", "viewer_account_id", aliceID, 0)
	e.rows(t, "feed_domain_mutes", "viewer_account_id", aliceID, 0)
	e.rows(t, "notification_domain_exclusions", "viewer_account_id", aliceID, 0)
	e.rows(t, "boosts", "booster_account_id", aliceID, 0)
	e.rows(t, "boost_edges", "booster_account_id", aliceID, 0)
	e.rows(t, "feed_entries", "author_account_id", aliceID, 0)
	e.rows(t, "notifications", "recipient_account_id", aliceID, 0)
	e.rows(t, "notifications", "source_account_id", aliceID, 0)
	e.rows(t, "notifications", "status_id", plain.ID, 0)
	e.rows(t, "local_statuses", "author_account_id", aliceID, 1) // the reported one
	// Alice's uploads go with their blobs; bob's stay.
	e.rows(t, "media_files", "owner_account_id", aliceID, 0)
	for _, table := range []string{"uploaded_media", "media_processing_results"} {
		e.rows(t, table, "media_id", aliceUpload, 0)
	}
	if n := e.blobsOf(aliceUpload); n != 0 {
		t.Errorf("alice's upload kept %d blobs", n)
	}
	e.rows(t, "media_files", "owner_account_id", bob, 1)
	if n := e.blobsOf(bobUpload); n != 3 {
		t.Errorf("bob's upload has %d blobs, want its 3 variants", n)
	}

	// The statuses are deletions; the reported one keeps its rows for now.
	for _, st := range []content.Status{plain, gone} {
		if e.stored(t, st.URI) {
			t.Errorf("status %q is still stored", st.URI)
		}
		e.rows(t, "status_revisions", "status_id", st.ID, 0)
		e.rows(t, "boosts", "status_id", st.ID, 0)
		e.rows(t, "favourites", "status_id", st.ID, 0)
		e.rows(t, "reactions", "status_id", st.ID, 0)
		e.rows(t, "status_reaction_counts", "status_id", st.ID, 0)
		e.rows(t, "bookmarks", "status_id", st.ID, 0)
	}
	if d, err := e.eh.Statuses.FindDeletion(ctx, plain.ID); err != nil || d.Origin != content.DeletedByAccountDeletion {
		t.Errorf("FindDeletion(plain) = %+v, %v; want origin account_deletion", d, err)
	}
	if d, err := e.eh.Statuses.FindDeletion(ctx, gone.ID); err != nil || d.Origin != content.DeletedByAuthor {
		t.Errorf("FindDeletion(gone) = %+v, %v; want the author's deletion", d, err)
	}
	if !e.stored(t, reported.URI) {
		t.Errorf("reported status is gone, want its rows kept")
	}
	if d, err := e.eh.Statuses.FindDeletion(ctx, reported.ID); err != nil || d.Origin != content.DeletedByAccountDeletion {
		t.Errorf("FindDeletion(reported) = %+v, %v; want origin account_deletion", d, err)
	}

	// Bob loses alice's follows and interactions.
	if c, _ := e.graph.Counters(ctx, bob); c.FollowersCount != 0 || c.FollowingCount != 0 {
		t.Errorf("bob's follow counters = %+v, want none", c)
	}
	if c, _ := e.interactions.Counters(ctx, bobs.ID); c.FavouritesCount != 0 || c.BoostsCount != 0 {
		t.Errorf("bob's status counters = %+v, want none", c)
	}
	e.rows(t, "status_reaction_counts", "status_id", bobs.ID, 0)
	if erased, err := e.eh.AccountErasure.AccountErased(ctx, aliceID); err != nil || erased {
		t.Errorf("AccountErased while evidence stays = %v, %v; want false", erased, err)
	}

	// Once the records are gone, so is the reported status, and the account is erased once.
	if n := e.run(t, e.eraser(stubEvidence{}, nil, 0)); n != 1 {
		t.Errorf("erased %d accounts, want 1", n)
	}
	if e.stored(t, reported.URI) {
		t.Errorf("reported status is still stored")
	}
	e.rows(t, "local_statuses", "author_account_id", aliceID, 0)
	if erased, err := e.eh.AccountErasure.AccountErased(ctx, aliceID); err != nil || !erased {
		t.Errorf("AccountErased = %v, %v; want true", erased, err)
	}
	if n := e.run(t, e.eraser(stubEvidence{}, nil, 0)); n != 0 {
		t.Errorf("erased %d accounts again, want 0", n)
	}
	// Bob is untouched.
	if _, err := e.accounts.FindByID(ctx, bob); err != nil {
		t.Errorf("FindByID(bob) = %v", err)
	}
	if !e.stored(t, bobs.URI) {
		t.Errorf("bob's status is gone")
	}
}

func erasureWaits(t *testing.T, e *erasureEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	e.postAs(t, alice, "alice's")
	e.postAs(t, bob, "bob's")
	e.deleteAccount(t, alice)
	profile := func(id string) bool {
		t.Helper()
		a, err := e.accounts.FindByID(ctx, id)
		if err != nil {
			t.Fatalf("FindByID = %v", err)
		}
		return a.Profile != nil
	}
	// Within the delay, the Delete of the actor still needs the account's keys and follows.
	e.clock.Advance(appaccount.ErasureDelay - time.Minute)
	if n := e.run(t, e.eraser(nil, nil, 0)); n != 0 || !profile(alice) {
		t.Errorf("erased %d accounts within the delay, want 0", n)
	}
	e.clock.Advance(time.Minute)
	// A retention hold stops the erasure.
	if n := e.run(t, e.eraser(nil, stubHolds{alice}, 0)); n != 0 || !profile(alice) {
		t.Errorf("erased %d held accounts, want 0", n)
	}
	if n := e.run(t, e.eraser(nil, nil, 0)); n != 1 || profile(alice) {
		t.Errorf("erased %d accounts, want 1", n)
	}
	// An account that is not deleted is never erased.
	if !profile(bob) {
		t.Error("bob's profile is gone")
	}
}
