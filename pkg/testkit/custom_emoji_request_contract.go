// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The cases of CustomEmojiRepositoryContract on the requests of the holders of
// create_custom_emojis (ADR-0070 §2, §3, §4): a request is no emoji until it is approved; the
// approval makes the emoji with the requester as its creator and a copy of the image owned by
// the instance actor; a pending request to create reserves its shortcode; the creators change
// and delete only their own emoji while they may create emoji, and a new image is a request;
// a request for a new image lapses with its emoji; and the limit counts the approved and pending
// requests to create an emoji that the creator submitted in the UTC calendar month, while the
// default limits nobody.

const (
	requestInstance = "0192f0c1-0000-7000-8000-0000000ad121"
	requesterA      = "0192f0c1-0000-7000-8000-0000000ad122"
	requesterAUser  = "0192f0c1-0000-7000-8000-0000000ad123"
	requesterB      = "0192f0c1-0000-7000-8000-0000000ad124"
	requesterBUser  = "0192f0c1-0000-7000-8000-0000000ad125"
)

// fakeEmojiCreators grants create_custom_emojis to the users in allowed.
type fakeEmojiCreators struct {
	mu      sync.Mutex
	allowed map[string]bool
}

func (f *fakeEmojiCreators) MayCreate(_ context.Context, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.allowed[userID], nil
}

func (f *fakeEmojiCreators) AccountOf(_ context.Context, userID string) (string, error) {
	return map[string]string{requesterAUser: requesterA, requesterBUser: requesterB}[userID], nil
}

func (f *fakeEmojiCreators) set(userID string, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.allowed[userID] = ok
}

// fakeEmojiQuota limits every user to limit, when limited.
type fakeEmojiQuota struct {
	limit   int
	limited bool
}

func (q *fakeEmojiQuota) Limit(context.Context, string) (int, bool, error) {
	return q.limit, q.limited, nil
}

type requestEnv struct {
	*emojiEnv
	creators *fakeEmojiCreators
	quota    *fakeEmojiQuota

	rmu      sync.Mutex
	requests []string
}

// newRequestEnv rebuilds the media and custom emoji use cases of e with the ports of the
// requests, and the uploads of the pending requests kept from the cleanup.
func newRequestEnv(t *testing.T, e *emojiEnv) *requestEnv {
	t.Helper()
	r := &requestEnv{emojiEnv: e, creators: &fakeEmojiCreators{allowed: map[string]bool{requesterAUser: true, requesterBUser: true}}, quota: &fakeEmojiQuota{}}
	e.media = appcontent.NewMediaService(appcontent.MediaDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Media, Blobs: e.blobs, Processor: FakeMediaProcessor{},
		Jobs: &recordingJobs{}, Clock: e.clock, IDs: e.ids, Limits: media.Limits{MaxBytes: 1 << 20},
		Referrers: []appcontent.MediaReferrer{appcontent.PendingCustomEmojiRequestMedia(e.h.Emojis)},
	})
	e.service = appcontent.NewCustomEmojiService(appcontent.CustomEmojiDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Emojis, Media: e.media, Clock: e.clock, IDs: e.ids, MaxBytes: 64 << 10,
		Creators: r.creators, Quota: r.quota,
		InstanceAccount: func(context.Context) (string, error) { return requestInstance, nil },
	})
	e.projectors.OnCommit(appcontent.EventCustomEmojiRequestRecorded, func(_ context.Context, ev uow.Event) {
		r.rmu.Lock()
		defer r.rmu.Unlock()
		r.requests = append(r.requests, ev.(appcontent.CustomEmojiRequestRecorded).Action)
	})
	return r
}

// takeRequestEvents returns the actions of the request events since the last call, in order.
func (r *requestEnv) takeRequestEvents() []string {
	r.rmu.Lock()
	defer r.rmu.Unlock()
	out := r.requests
	r.requests = nil
	return out
}

// submit requests a new emoji shortcode as the user of account, with a new image.
func (r *requestEnv) submit(t *testing.T, account, user, shortcode string) (appcontent.CustomEmojiRequestView, error) {
	t.Helper()
	media := r.image(t, account, FixturePNG(4, 4, true))
	r.clock.Advance(time.Second)
	return r.service.SubmitRequest(t.Context(), appcontent.SubmitCustomEmojiRequestInput{
		Kind: content.RequestCreate, Shortcode: shortcode, MediaID: media, RequesterUserID: user, RequesterAccountID: account,
	})
}

func (r *requestEnv) mustSubmit(t *testing.T, account, user, shortcode string) appcontent.CustomEmojiRequestView {
	t.Helper()
	req, err := r.submit(t, account, user, shortcode)
	if err != nil {
		t.Fatalf("SubmitRequest(%s): %v", shortcode, err)
	}
	return req
}

// approved submits and approves a request of A for shortcode and returns the emoji.
func (r *requestEnv) approved(t *testing.T, shortcode string) appcontent.AdminCustomEmoji {
	t.Helper()
	req := r.mustSubmit(t, requesterA, requesterAUser, shortcode)
	done, err := r.service.ApproveRequest(t.Context(), req.Request.ID, emojiAdminUser)
	if err != nil {
		t.Fatalf("ApproveRequest: %v", err)
	}
	e, err := r.service.Get(t.Context(), done.Outcome.EmojiID)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func conflictReason(err error) appcontent.CustomEmojiRequestConflictReason {
	var c *appcontent.CustomEmojiRequestConflict
	if errors.As(err, &c) {
		return c.Reason
	}
	return ""
}

func requestHidesUntilApproved(t *testing.T, e *emojiEnv) {
	r := newRequestEnv(t, e)
	ctx := t.Context()
	req := r.mustSubmit(t, requesterA, requesterAUser, "blobcat")
	if req.State() != content.RequestPending || req.Request.Kind != content.RequestCreate || req.Request.Shortcode != "blobcat" || req.View == nil {
		t.Fatalf("submitted = %+v", req)
	}
	if ev := r.takeRequestEvents(); !slices.Equal(ev, []string{appcontent.CustomEmojiRequestSubmitted}) {
		t.Errorf("request events = %v", ev)
	}
	// A pending request is no emoji: nothing that shows the custom emoji shows it.
	if picker, err := r.service.Picker(ctx); err != nil || len(picker) != 0 {
		t.Errorf("Picker = %+v, %v; want none", picker, err)
	}
	if ids, err := r.service.LocalEmojiIDs(ctx, ":blobcat:"); err != nil || len(ids) != 0 {
		t.Errorf("LocalEmojiIDs = %v, %v; want none", ids, err)
	}
	if list, err := r.service.List(ctx, appcontent.CustomEmojiQuery{}, 10); err != nil || len(list) != 0 {
		t.Errorf("List = %+v, %v; want none", list, err)
	}
	if views, err := r.media.CustomEmojis(ctx, []string{req.Request.ID}); err != nil || len(views) != 0 {
		t.Errorf("CustomEmojis(request) = %+v, %v; want none", views, err)
	}
	// The upload of a pending request stays past the unattached TTL.
	r.clock.Advance(appcontent.DefaultUnattachedTTL + time.Hour)
	if n, err := r.media.CleanupUnattached(ctx); err != nil || n != 0 {
		t.Errorf("CleanupUnattached = %d, %v; want the pending request's upload kept", n, err)
	}
	if pending, err := r.service.ListRequests(ctx, appcontent.CustomEmojiRequestQuery{State: content.RequestPending}, 10); err != nil ||
		len(pending) != 1 || pending[0].Request.ID != req.Request.ID {
		t.Errorf("pending requests = %+v, %v", pending, err)
	}

	done, err := r.service.ApproveRequest(ctx, req.Request.ID, emojiAdminUser)
	if err != nil {
		t.Fatalf("ApproveRequest: %v", err)
	}
	if done.State() != content.CustomEmojiRequestState(content.RequestApproved) || done.Outcome.EmojiID == "" || done.Outcome.DecidedByUserID != emojiAdminUser {
		t.Fatalf("approved = %+v %+v", done, done.Outcome)
	}
	if ev := r.takeRequestEvents(); !slices.Equal(ev, []string{appcontent.CustomEmojiRequestApproved}) {
		t.Errorf("request events = %v", ev)
	}
	if ev := r.takeEvents(); !slices.Equal(ev, []string{appcontent.CustomEmojiCreated}) {
		t.Errorf("emoji events = %v", ev)
	}
	emoji, err := r.service.Get(ctx, done.Outcome.EmojiID)
	if err != nil {
		t.Fatal(err)
	}
	if emoji.Emoji.Shortcode != "blobcat" || emoji.Emoji.CreatedByUserID != requesterAUser || !emoji.Settings.IsDefault() || len(emoji.Aliases) != 0 {
		t.Errorf("emoji = %+v", emoji)
	}
	// The image is a copy owned by the instance actor, so that it outlives the requester.
	img, err := r.media.Media(ctx, emoji.Image.MediaID)
	if err != nil || img.File.OwnerAccountID != requestInstance || emoji.Image.MediaID == req.Request.MediaID {
		t.Errorf("image = %+v, %v; want a copy of the instance actor", img.File, err)
	}
	if picker, err := r.service.Picker(ctx); err != nil || len(picker) != 1 || picker[0].Shortcode != "blobcat" {
		t.Errorf("Picker after the approval = %+v, %v", picker, err)
	}
	if ids, err := r.service.LocalEmojiIDs(ctx, ":blobcat:"); err != nil || !slices.Equal(ids, []string{emoji.Emoji.ID}) {
		t.Errorf("LocalEmojiIDs after the approval = %v, %v", ids, err)
	}
	if own, err := r.service.ListOwn(ctx, requesterAUser, "", 10); err != nil || len(own) != 1 || own[0].Emoji.ID != emoji.Emoji.ID {
		t.Errorf("ListOwn = %+v, %v", own, err)
	}
	// The requester's upload is unattached again, and goes.
	r.clock.Advance(appcontent.DefaultUnattachedTTL + time.Hour)
	if n, err := r.media.CleanupUnattached(ctx); err != nil || n != 1 {
		t.Errorf("CleanupUnattached = %d, %v; want the requester's upload", n, err)
	}
	if _, err := r.media.Media(ctx, req.Request.MediaID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("the requester's upload is kept: %v", err)
	}
	if _, err := r.service.ApproveRequest(ctx, req.Request.ID, emojiAdminUser); conflictReason(err) != appcontent.RequestConflictNotPending {
		t.Errorf("ApproveRequest again = %v, want not_pending", err)
	}
	if ids, err := r.service.ExistingRequestIDs(ctx, []string{req.Request.ID, "0192f0c1-0000-7000-8000-00000000dead"}); err != nil ||
		!slices.Equal(ids, []string{req.Request.ID}) {
		t.Errorf("ExistingRequestIDs = %v, %v", ids, err)
	}
}

func requestRejectsAndWithdraws(t *testing.T, e *emojiEnv) {
	r := newRequestEnv(t, e)
	ctx := t.Context()
	req := r.mustSubmit(t, requesterA, requesterAUser, "blobcat")
	if _, err := r.service.RejectRequest(ctx, req.Request.ID, strings.Repeat("a", 501), emojiAdminUser); emojiReason(err) != content.EmojiInvalidReason {
		t.Errorf("RejectRequest(too long) = %v", err)
	}
	done, err := r.service.RejectRequest(ctx, req.Request.ID, "  Not this one ", emojiAdminUser)
	if err != nil {
		t.Fatal(err)
	}
	if done.State() != content.CustomEmojiRequestState(content.RequestRejected) || done.Outcome.Reason != "Not this one" || done.Outcome.EmojiID != "" {
		t.Errorf("rejected = %+v %+v", done, done.Outcome)
	}
	if list, _ := r.service.List(ctx, appcontent.CustomEmojiQuery{}, 10); len(list) != 0 {
		t.Errorf("a rejected request made an emoji: %+v", list)
	}
	// The rejection frees the shortcode.
	again := r.mustSubmit(t, requesterA, requesterAUser, "blobcat")
	if _, err := r.service.WithdrawRequest(ctx, again.Request.ID, requesterBUser); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("WithdrawRequest of another user = %v, want ErrNotFound", err)
	}
	withdrawn, err := r.service.WithdrawRequest(ctx, again.Request.ID, requesterAUser)
	if err != nil || withdrawn.State() != content.CustomEmojiRequestState(content.RequestWithdrawn) || withdrawn.Outcome.DecidedByUserID != requesterAUser {
		t.Errorf("WithdrawRequest = %+v, %v", withdrawn, err)
	}
	if _, err := r.service.WithdrawRequest(ctx, again.Request.ID, requesterAUser); conflictReason(err) != appcontent.RequestConflictNotPending {
		t.Errorf("WithdrawRequest again = %v, want not_pending", err)
	}
	if _, err := r.service.RejectRequest(ctx, again.Request.ID, "", emojiAdminUser); conflictReason(err) != appcontent.RequestConflictNotPending {
		t.Errorf("RejectRequest of a withdrawn request = %v, want not_pending", err)
	}
	if ev := r.takeRequestEvents(); !slices.Equal(ev, []string{
		appcontent.CustomEmojiRequestSubmitted, appcontent.CustomEmojiRequestRejected,
		appcontent.CustomEmojiRequestSubmitted, appcontent.CustomEmojiRequestWithdrawn,
	}) {
		t.Errorf("request events = %v", ev)
	}
	mine, err := r.service.ListRequests(ctx, appcontent.CustomEmojiRequestQuery{RequesterUserID: requesterAUser}, 10)
	if err != nil || len(mine) != 2 || mine[0].Request.ID != again.Request.ID {
		t.Errorf("ListRequests(A) = %+v, %v", mine, err)
	}
	if others, err := r.service.ListRequests(ctx, appcontent.CustomEmojiRequestQuery{RequesterUserID: requesterBUser}, 10); err != nil || len(others) != 0 {
		t.Errorf("ListRequests(B) = %+v, %v", others, err)
	}
	rejected, err := r.service.ListRequests(ctx, appcontent.CustomEmojiRequestQuery{State: content.CustomEmojiRequestState(content.RequestRejected)}, 10)
	if err != nil || len(rejected) != 1 || rejected[0].Request.ID != req.Request.ID {
		t.Errorf("ListRequests(rejected) = %+v, %v", rejected, err)
	}
	if got, err := r.service.RequestsByID(ctx, []string{again.Request.ID, req.Request.ID}); err != nil || len(got) != 2 {
		t.Errorf("RequestsByID = %+v, %v", got, err)
	}
}

func requestReservesShortcodes(t *testing.T, e *emojiEnv) {
	r := newRequestEnv(t, e)
	ctx := t.Context()
	pending := r.mustSubmit(t, requesterA, requesterAUser, "blobcat")
	if _, err := r.submit(t, requesterB, requesterBUser, "blobcat"); emojiReason(err) != content.EmojiShortcodeTaken {
		t.Errorf("a second request for the shortcode = %v, want shortcode_taken", err)
	}
	// An administrator cannot take the shortcode of a pending request either.
	in := appcontent.CreateCustomEmojiInput{
		Shortcode: "blobcat", MediaID: r.image(t, emojiAdmin, FixturePNG(4, 3, true)), OwnerAccountID: emojiAdmin, VisibleInPicker: true,
	}
	if _, err := r.service.Create(ctx, in); emojiReason(err) != content.EmojiShortcodeTaken {
		t.Errorf("Create of a reserved shortcode = %v, want shortcode_taken", err)
	}
	r.create(t, "blobfox")
	if _, err := r.submit(t, requesterA, requesterAUser, "blobfox"); emojiReason(err) != content.EmojiShortcodeTaken {
		t.Errorf("a request for a local emoji's shortcode = %v, want shortcode_taken", err)
	}
	if _, err := r.submit(t, requesterA, requesterAUser, "x"); emojiReason(err) != content.EmojiInvalidShortcode {
		t.Errorf("a request for an invalid shortcode = %v", err)
	}
	if _, err := r.service.WithdrawRequest(ctx, pending.Request.ID, requesterAUser); err != nil {
		t.Fatal(err)
	}
	if _, err := r.service.Create(ctx, in); err != nil {
		t.Errorf("Create after the withdrawal = %v", err)
	}
	// A request of a user who no longer may create emoji cannot be approved.
	late := r.mustSubmit(t, requesterB, requesterBUser, "blobdog")
	r.creators.set(requesterBUser, false)
	if _, err := r.service.ApproveRequest(ctx, late.Request.ID, emojiAdminUser); conflictReason(err) != appcontent.RequestConflictRequesterCannotCreate {
		t.Errorf("ApproveRequest of a requester who cannot create = %v, want requester_cannot_create", err)
	}
	if got, _ := r.service.GetRequest(ctx, late.Request.ID); got.State() != content.RequestPending {
		t.Errorf("the refused approval changed the request: %+v", got)
	}
	if _, err := r.submit(t, requesterB, requesterBUser, "blobowl"); !errors.Is(err, appcontent.ErrNotCustomEmojiCreator) {
		t.Errorf("a request of a user who cannot create = %v, want ErrNotCustomEmojiCreator", err)
	}
}

func requestLimitsCreatorsToTheirOwn(t *testing.T, e *emojiEnv) {
	r := newRequestEnv(t, e)
	ctx := t.Context()
	cat := r.approved(t, "blobcat")
	r.takeEvents()
	adminEmoji := r.create(t, "blobfox")
	r.takeEvents()
	category := "Cats"
	// Another creator, or an emoji that an administrator made, is out of reach.
	for _, tc := range []struct{ id, user string }{{cat.Emoji.ID, requesterBUser}, {adminEmoji.Emoji.ID, requesterAUser}} {
		if _, err := r.service.UpdateOwn(ctx, tc.id, tc.user, appcontent.OwnCustomEmojiUpdate{Category: &category}); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("UpdateOwn(%s by %s) = %v, want ErrNotFound", tc.id, tc.user, err)
		}
		if err := r.service.DeleteOwn(ctx, tc.id, tc.user); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("DeleteOwn(%s by %s) = %v, want ErrNotFound", tc.id, tc.user, err)
		}
		_, err := r.service.SubmitRequest(ctx, appcontent.SubmitCustomEmojiRequestInput{
			Kind: content.RequestImage, EmojiID: tc.id, MediaID: r.image(t, requesterB, FixturePNG(4, 4, true)),
			RequesterUserID: tc.user, RequesterAccountID: requesterB,
		})
		if !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("an image request for %s by %s = %v, want ErrNotFound", tc.id, tc.user, err)
		}
	}
	got, err := r.service.UpdateOwn(ctx, cat.Emoji.ID, requesterAUser, appcontent.OwnCustomEmojiUpdate{Category: &category, Aliases: &[]string{"neko"}})
	if err != nil || got.Settings.Category != "Cats" || !slices.Equal(got.Aliases, []string{"neko"}) {
		t.Errorf("UpdateOwn = %+v, %v", got, err)
	}
	if ev := r.takeEvents(); !slices.Equal(ev, []string{appcontent.CustomEmojiUpdated}) {
		t.Errorf("events = %v", ev)
	}
	// A new image is a request, at most one pending per emoji.
	image := func() (appcontent.CustomEmojiRequestView, error) {
		return r.service.SubmitRequest(ctx, appcontent.SubmitCustomEmojiRequestInput{
			Kind: content.RequestImage, EmojiID: cat.Emoji.ID, MediaID: r.image(t, requesterA, FixturePNG(8, 8, true)),
			RequesterUserID: requesterAUser, RequesterAccountID: requesterA,
		})
	}
	req, err := image()
	if err != nil || req.Request.Kind != content.RequestImage || req.Request.EmojiID != cat.Emoji.ID || req.Request.Shortcode != "blobcat" {
		t.Fatalf("image request = %+v, %v", req, err)
	}
	if current, _ := r.service.Get(ctx, cat.Emoji.ID); current.Image.ID != cat.Image.ID {
		t.Errorf("a pending image request changed the image: %+v", current.Image)
	}
	if _, err := image(); emojiReason(err) != content.EmojiRequestPending {
		t.Errorf("a second image request = %v, want request_pending", err)
	}
	if _, err := r.service.ApproveRequest(ctx, req.Request.ID, emojiAdminUser); err != nil {
		t.Fatal(err)
	}
	if current, _ := r.service.Get(ctx, cat.Emoji.ID); current.Image.ID == cat.Image.ID {
		t.Error("the approved image request kept the old image")
	}
	if ev := r.takeEvents(); !slices.Equal(ev, []string{appcontent.CustomEmojiImageReplaced}) {
		t.Errorf("events = %v", ev)
	}
	// Once the creator may no longer create emoji, the emoji stays and they cannot change it.
	r.creators.set(requesterAUser, false)
	if _, err := r.service.UpdateOwn(ctx, cat.Emoji.ID, requesterAUser, appcontent.OwnCustomEmojiUpdate{Category: &category}); !errors.Is(err, appcontent.ErrNotCustomEmojiCreator) {
		t.Errorf("UpdateOwn without the permission = %v, want ErrNotCustomEmojiCreator", err)
	}
	if err := r.service.DeleteOwn(ctx, cat.Emoji.ID, requesterAUser); !errors.Is(err, appcontent.ErrNotCustomEmojiCreator) {
		t.Errorf("DeleteOwn without the permission = %v, want ErrNotCustomEmojiCreator", err)
	}
	if _, err := image(); !errors.Is(err, appcontent.ErrNotCustomEmojiCreator) {
		t.Errorf("an image request without the permission = %v, want ErrNotCustomEmojiCreator", err)
	}
	if _, err := r.service.Get(ctx, cat.Emoji.ID); err != nil {
		t.Errorf("the emoji went with the permission: %v", err)
	}
	// With the permission again, the creator deletes it.
	r.creators.set(requesterAUser, true)
	if err := r.service.DeleteOwn(ctx, cat.Emoji.ID, requesterAUser); err != nil {
		t.Errorf("DeleteOwn = %v", err)
	}
	if ev := r.takeEvents(); !slices.Equal(ev, []string{appcontent.CustomEmojiDeleted}) {
		t.Errorf("events = %v", ev)
	}
}

func requestLapsesWithItsEmoji(t *testing.T, e *emojiEnv) {
	r := newRequestEnv(t, e)
	ctx := t.Context()
	cat := r.approved(t, "blobcat")
	req, err := r.service.SubmitRequest(ctx, appcontent.SubmitCustomEmojiRequestInput{
		Kind: content.RequestImage, EmojiID: cat.Emoji.ID, MediaID: r.image(t, requesterA, FixturePNG(8, 8, true)),
		RequesterUserID: requesterAUser, RequesterAccountID: requesterA,
	})
	if err != nil {
		t.Fatal(err)
	}
	r.takeRequestEvents()
	if err := r.service.Delete(ctx, cat.Emoji.ID, emojiAdminUser); err != nil {
		t.Fatal(err)
	}
	got, err := r.service.GetRequest(ctx, req.Request.ID)
	if err != nil || got.State() != content.CustomEmojiRequestState(content.RequestLapsed) || got.Outcome.DecidedByUserID != "" {
		t.Errorf("the image request of a deleted emoji = %+v, %v; want lapsed", got, err)
	}
	if ev := r.takeRequestEvents(); len(ev) != 0 {
		t.Errorf("a lapse recorded %v, want nothing", ev)
	}
	if _, err := r.service.ApproveRequest(ctx, req.Request.ID, emojiAdminUser); conflictReason(err) != appcontent.RequestConflictNotPending {
		t.Errorf("ApproveRequest of a lapsed request = %v, want not_pending", err)
	}
}

func requestAppliesTheLimit(t *testing.T, e *emojiEnv) {
	r := newRequestEnv(t, e)
	ctx := t.Context()
	november := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	// By default nobody is limited: the requests are counted but limit nothing.
	for _, code := range []string{"blob_a", "blob_b", "blob_c"} {
		r.mustSubmit(t, requesterA, requesterAUser, code)
	}
	if u, err := r.service.Usage(ctx, requesterAUser); err != nil || u != (appcontent.CustomEmojiUsage{Used: 3, Pending: 3, ResetsAt: november}) {
		t.Errorf("Usage without a limit = %+v, %v", u, err)
	}
	d := r.approved(t, "blob_d")
	r.quota.limit, r.quota.limited = 5, true
	if u, err := r.service.Usage(ctx, requesterAUser); err != nil || u != (appcontent.CustomEmojiUsage{Used: 4, Pending: 3, Limit: 5, Limited: true, ResetsAt: november}) {
		t.Errorf("Usage = %+v, %v", u, err)
	}
	last := r.mustSubmit(t, requesterA, requesterAUser, "blob_e")
	if _, err := r.submit(t, requesterA, requesterAUser, "blob_f"); emojiReason(err) != content.EmojiLimitReached {
		t.Errorf("a request past the limit = %v, want limit_reached", err)
	}
	// Another user has their own count.
	r.mustSubmit(t, requesterB, requesterBUser, "blob_g")
	// A rejection and a withdrawal free a place.
	if _, err := r.service.RejectRequest(ctx, last.Request.ID, "", emojiAdminUser); err != nil {
		t.Fatal(err)
	}
	f := r.mustSubmit(t, requesterA, requesterAUser, "blob_f")
	if _, err := r.service.WithdrawRequest(ctx, f.Request.ID, requesterAUser); err != nil {
		t.Fatal(err)
	}
	r.mustSubmit(t, requesterA, requesterAUser, "blob_h")
	// Deleting an approved emoji frees nothing this month.
	if err := r.service.DeleteOwn(ctx, d.Emoji.ID, requesterAUser); err != nil {
		t.Fatal(err)
	}
	if u, err := r.service.Usage(ctx, requesterAUser); err != nil || u.Used != 5 || u.Pending != 4 {
		t.Errorf("Usage after a deletion = %+v, %v; want 5 used, 4 pending", u, err)
	}
	if _, err := r.submit(t, requesterA, requesterAUser, "blob_i"); emojiReason(err) != content.EmojiLimitReached {
		t.Errorf("a request after a deletion = %v, want limit_reached", err)
	}
	// A lower limit keeps the emoji and refuses new requests only.
	a, err := r.service.ListRequests(ctx, appcontent.CustomEmojiRequestQuery{RequesterUserID: requesterAUser, State: content.RequestPending}, 10)
	if err != nil || len(a) != 4 {
		t.Fatalf("pending requests of A = %d, %v; want 4", len(a), err)
	}
	if _, err := r.service.ApproveRequest(ctx, a[0].Request.ID, emojiAdminUser); err != nil {
		t.Fatal(err)
	}
	r.quota.limit = 1
	if _, err := r.submit(t, requesterA, requesterAUser, "blob_i"); emojiReason(err) != content.EmojiLimitReached {
		t.Errorf("a request past a lowered limit = %v, want limit_reached", err)
	}
	if own, err := r.service.ListOwn(ctx, requesterAUser, "", 10); err != nil || len(own) != 1 {
		t.Errorf("ListOwn after the lower limit = %d, %v; want the emoji kept", len(own), err)
	}
	// The count resets at 00:00 UTC on the first day of the next month. A request submitted in
	// October and approved in November counts in October, not in November.
	r.clock.Set(november.Add(-time.Second))
	if u, err := r.service.Usage(ctx, requesterAUser); err != nil || u.Used != 5 || u.ResetsAt != november {
		t.Errorf("Usage at the end of October = %+v, %v; want 5 used", u, err)
	}
	r.clock.Set(november)
	if _, err := r.service.ApproveRequest(ctx, a[1].Request.ID, emojiAdminUser); err != nil {
		t.Fatal(err)
	}
	december := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	if u, err := r.service.Usage(ctx, requesterAUser); err != nil || u != (appcontent.CustomEmojiUsage{Limit: 1, Limited: true, ResetsAt: december}) {
		t.Errorf("Usage in November = %+v, %v; want nothing used", u, err)
	}
	r.quota.limit = 2
	r.mustSubmit(t, requesterA, requesterAUser, "blob_i")
	r.approved(t, "blob_j")
	if _, err := r.submit(t, requesterA, requesterAUser, "blob_k"); emojiReason(err) != content.EmojiLimitReached {
		t.Errorf("a request past the limit in November = %v, want limit_reached", err)
	}
	if u, err := r.service.Usage(ctx, requesterAUser); err != nil || u != (appcontent.CustomEmojiUsage{Used: 2, Pending: 1, Limit: 2, Limited: true, ResetsAt: december}) {
		t.Errorf("Usage in November = %+v, %v", u, err)
	}
}
