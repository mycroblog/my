// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// CustomEmojiRepositoryContract verifies the custom emoji of a database adapter by the methods of
// its repositories alone, the port contract of ADR-0153 §1: emoji inserted with a shortcode free
// on their domain, their images (the newest is current), settings (the defaults need no row), and
// aliases, found, listed with the filters of the administrators newest first, and deleted; the
// local emoji by shortcode; the emoji lookups of the status repository and the emoji reads of the
// media repository; the requests of the creators (ADR-0070), whose pending ones reserve their
// shortcodes and the images of their emoji, closed once, listed, counted, and rebuilt; and the
// purge of the remote emoji that nothing refers to, with their remote media files
// (EmojiPurgeRepository). The custom emoji use cases on top of it are CustomEmojiFlows, which
// internal/flowtest/content runs once on SQLite.
func CustomEmojiRepositoryContract(t *testing.T, newHarness CustomEmojiFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *emojiRepo)
	}{
		{"InsertsFindsAndDeletesEmoji", emojiRepoRecords},
		{"ListsEmoji", emojiRepoLists},
		{"LooksUpEmojiOfStatusesAndMedia", emojiRepoLookups},
		{"KeepsRequestsAndTheirReservations", emojiRepoRequests},
		{"ListsCountsAndRebuildsRequests", emojiRepoRequestLists},
		{"PurgesUnreferencedRemoteEmoji", emojiRepoPurge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var h CustomEmojiHarness
			se := newStatusEnv(t, func(t *testing.T, opts uow.Options) StatusHarness {
				h = newHarness(t, opts)
				return StatusHarness{UnitOfWork: h.UnitOfWork, Statuses: h.Statuses}
			})
			tc.run(t, &emojiRepo{se: se, h: h, repo: h.Emojis})
		})
	}
}

type emojiRepo struct {
	se   *StatusEnv
	h    CustomEmojiHarness
	repo appcontent.CustomEmojiRepository
}

func (r *emojiRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

func (r *emojiRepo) id() string { return r.se.IDs.NewID() }

// emoji inserts an emoji of domain ("" for a local one) by creator with an image of a new media
// file, a second after the previous one.
func (r *emojiRepo) emoji(t *testing.T, shortcode, domain, creator string) (content.CustomEmoji, content.CustomEmojiImage) {
	t.Helper()
	r.se.Clock.Advance(time.Second)
	e := content.CustomEmoji{ID: r.id(), Shortcode: shortcode, Domain: domain, CreatedByUserID: creator, CreatedAt: r.se.Clock.Now()}
	if domain != "" {
		e.URI = "https://" + domain + "/emojis/" + shortcode
	}
	img := content.CustomEmojiImage{ID: r.id(), EmojiID: e.ID, MediaID: r.id(), RevisedAt: r.se.Clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.InsertCustomEmoji(ctx, tx, e); err != nil {
			return err
		}
		return r.repo.InsertCustomEmojiImage(ctx, tx, img)
	})
	return e, img
}

// remoteMedia stores a remote media file of url and returns its ID.
func (r *emojiRepo) remoteMedia(t *testing.T, url string) string {
	t.Helper()
	m := content.MediaFile{ID: r.id(), OwnerAccountID: r.id(), Origin: content.MediaOriginRemote, CreatedAt: r.se.Clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Media.InsertMediaFile(ctx, tx, m); err != nil {
			return err
		}
		_, err := r.h.Media.InsertRemoteMedia(ctx, tx, content.RemoteMedia{
			MediaID: m.ID, OwnerAccountID: m.OwnerAccountID, RemoteURL: url, DeclaredContentType: "image/png",
			DeclaredMediaType: content.MediaTypeImage, Purpose: content.PurposeEmoji,
		})
		return err
	})
	return m.ID
}

func emojiRepoRecords(t *testing.T, r *emojiRepo) {
	ctx := t.Context()
	admin := r.id()
	blob, firstImage := r.emoji(t, "blob", "", admin)
	if err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return r.repo.InsertCustomEmoji(ctx, tx, content.CustomEmoji{ID: r.id(), Shortcode: "blob", CreatedAt: r.se.Clock.Now()})
	}); !errors.Is(err, appcontent.ErrShortcodeTaken) {
		t.Errorf("InsertCustomEmoji(a taken local shortcode) = %v, want ErrShortcodeTaken", err)
	}
	remote, _ := r.emoji(t, "blob", "remote.example", "") // another domain may have the shortcode

	// A newer image is current; settings and aliases are current values.
	r.se.Clock.Advance(time.Second)
	image := content.CustomEmojiImage{ID: r.id(), EmojiID: blob.ID, MediaID: r.id(), RevisedAt: r.se.Clock.Now()}
	settings := content.CustomEmojiSettings{EmojiID: blob.ID, Enabled: false, VisibleInPicker: true, Category: "Blobs"}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.repo.InsertCustomEmojiImage(ctx, tx, image),
			r.repo.SetCustomEmojiSettings(ctx, tx, settings),
			r.repo.SetCustomEmojiAliases(ctx, tx, blob.ID, []string{"old"}),
			r.repo.SetCustomEmojiAliases(ctx, tx, blob.ID, []string{"blobby", "round"}),
		)
	})
	want := content.CustomEmojiRecord{
		CurrentCustomEmoji: content.CurrentCustomEmoji{Emoji: blob, Image: image, Settings: settings},
		Aliases:            []string{"blobby", "round"},
	}
	got, err := r.repo.FindCustomEmojiRecord(ctx, blob.ID)
	slices.Sort(got.Aliases)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("FindCustomEmojiRecord = %+v, %v; want %+v (not the first image %s)", got, err, want, firstImage.ID)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.repo.FindCustomEmojiRecordTx(ctx, tx, remote.ID)
		if err != nil || got.Emoji != remote || got.Settings != content.DefaultCustomEmojiSettings(remote.ID) || len(got.Aliases) != 0 {
			t.Errorf("FindCustomEmojiRecordTx = %+v, %v; want the remote emoji with the default settings", got, err)
		}
		// The default settings delete the row: the emoji reads as default again.
		return r.repo.SetCustomEmojiSettings(ctx, tx, content.DefaultCustomEmojiSettings(blob.ID))
	})
	if got, err := r.repo.FindCustomEmojiRecord(ctx, blob.ID); err != nil || got.Settings != content.DefaultCustomEmojiSettings(blob.ID) {
		t.Errorf("FindCustomEmojiRecord after the default settings = %+v, %v", got.Settings, err)
	}
	if _, err := r.repo.FindCustomEmojiRecord(ctx, r.id()); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindCustomEmojiRecord(unknown) = %v, want ErrNotFound", err)
	}
	if got, err := r.repo.ExistingCustomEmojiIDs(ctx, []string{remote.ID, r.id(), blob.ID}); err != nil || !slices.Equal(got, []string{blob.ID, remote.ID}) {
		t.Errorf("ExistingCustomEmojiIDs = %v, %v; want %v", got, err, []string{blob.ID, remote.ID})
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.DeleteCustomEmoji(ctx, tx, blob.ID) })
	if _, err := r.repo.FindCustomEmojiRecord(ctx, blob.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindCustomEmojiRecord after DeleteCustomEmoji = %v, want ErrNotFound", err)
	}
	if got, err := r.repo.ExistingCustomEmojiIDs(ctx, []string{blob.ID, remote.ID}); err != nil || !slices.Equal(got, []string{remote.ID}) {
		t.Errorf("ExistingCustomEmojiIDs after DeleteCustomEmoji = %v, %v", got, err)
	}
	// The shortcode is free again.
	r.emoji(t, "blob", "", admin)
}

func emojiRepoLists(t *testing.T, r *emojiRepo) {
	ctx := t.Context()
	alice, bob := r.id(), r.id()
	cat, _ := r.emoji(t, "cat", "", alice)
	bigCat, _ := r.emoji(t, "BigCat", "", bob)
	dog, _ := r.emoji(t, "dog", "", alice)
	remoteCat, _ := r.emoji(t, "cat", "remote.example", "")
	otherCat, _ := r.emoji(t, "cat", "other.example", "")
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.SetCustomEmojiSettings(ctx, tx, content.CustomEmojiSettings{EmojiID: dog.ID, VisibleInPicker: true})
	})
	ids := func(rs []content.CustomEmojiRecord) []string {
		out := make([]string, len(rs))
		for i, rec := range rs {
			out[i] = rec.Emoji.ID
		}
		return out
	}
	for _, tc := range []struct {
		name string
		q    appcontent.CustomEmojiQuery
		want []content.CustomEmoji
	}{
		{"every emoji", appcontent.CustomEmojiQuery{}, []content.CustomEmoji{otherCat, remoteCat, dog, bigCat, cat}},
		{"local", appcontent.CustomEmojiQuery{Locality: appcontent.LocalEmoji}, []content.CustomEmoji{dog, bigCat, cat}},
		{"remote", appcontent.CustomEmojiQuery{Locality: appcontent.RemoteEmoji}, []content.CustomEmoji{otherCat, remoteCat}},
		{"one domain", appcontent.CustomEmojiQuery{Domain: "remote.example"}, []content.CustomEmoji{remoteCat}},
		// The caller lowers the part of the shortcode; it matches any case.
		{"shortcode", appcontent.CustomEmojiQuery{Locality: appcontent.LocalEmoji, Shortcode: "cat"}, []content.CustomEmoji{bigCat, cat}},
		{"creator", appcontent.CustomEmojiQuery{CreatedBy: alice}, []content.CustomEmoji{dog, cat}},
		{"before", appcontent.CustomEmojiQuery{Before: dog.ID}, []content.CustomEmoji{bigCat, cat}},
	} {
		got, err := r.repo.ListCustomEmojiRecords(ctx, tc.q, 10)
		want := make([]string, len(tc.want))
		for i, e := range tc.want {
			want[i] = e.ID
		}
		if err != nil || !slices.Equal(ids(got), want) {
			t.Errorf("ListCustomEmojiRecords(%s) = %v, %v; want %v", tc.name, ids(got), err, want)
		}
	}
	if got, err := r.repo.ListCustomEmojiRecords(ctx, appcontent.CustomEmojiQuery{}, 2); err != nil || !slices.Equal(ids(got), []string{otherCat.ID, remoteCat.ID}) {
		t.Errorf("ListCustomEmojiRecords(limit 2) = %v, %v", ids(got), err)
	}

	// The local emoji by shortcode, disabled ones too.
	all, err := r.repo.ListLocalCustomEmojis(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var shortcodes []string
	for _, rec := range all {
		shortcodes = append(shortcodes, rec.Emoji.Shortcode)
		if rec.Emoji.ID == dog.ID && rec.Settings.Enabled {
			t.Errorf("ListLocalCustomEmojis(dog) = %+v, want it disabled", rec)
		}
	}
	if want := []string{"BigCat", "cat", "dog"}; !slices.Equal(shortcodes, want) {
		t.Errorf("ListLocalCustomEmojis(nil) = %v, %v; want %v", shortcodes, err, want)
	}
	if got, err := r.repo.ListLocalCustomEmojis(ctx, []string{"dog", "cat", "missing"}); err != nil || !slices.Equal(ids(got), []string{cat.ID, dog.ID}) {
		t.Errorf("ListLocalCustomEmojis(some) = %v, %v; want cat and dog", ids(got), err)
	}
	if got, err := r.repo.ListLocalCustomEmojis(ctx, []string{}); err != nil || len(got) != 0 {
		t.Errorf("ListLocalCustomEmojis(none) = %v, %v; want none", ids(got), err)
	}
}

func emojiRepoLookups(t *testing.T, r *emojiRepo) {
	ctx := t.Context()
	local, localImage := r.emoji(t, "local", "", r.id())
	statuses := r.h.Statuses
	// EnsureCustomEmoji stores a remote emoji once per shortcode and domain.
	remote := content.CustomEmoji{ID: r.id(), Shortcode: "far", Domain: "remote.example", URI: "https://remote.example/emojis/far", CreatedAt: r.se.Clock.Now()}
	url := "https://remote.example/files/far.png"
	mediaID := r.remoteMedia(t, url)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := statuses.EnsureCustomEmoji(ctx, tx, remote)
		if err != nil || got != remote {
			t.Errorf("EnsureCustomEmoji = %+v, %v; want %+v", got, err, remote)
		}
		again := remote
		again.ID = r.id()
		if got, err := statuses.EnsureCustomEmoji(ctx, tx, again); err != nil || got != remote {
			t.Errorf("EnsureCustomEmoji(again) = %+v, %v; want the stored %+v", got, err, remote)
		}
		if _, err := statuses.CurrentCustomEmojiImageURL(ctx, tx, remote.ID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("CurrentCustomEmojiImageURL(no image) = %v, want ErrNotFound", err)
		}
		return statuses.InsertCustomEmojiImage(ctx, tx, content.CustomEmojiImage{ID: r.id(), EmojiID: remote.ID, MediaID: mediaID, RevisedAt: r.se.Clock.Now()})
	})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := statuses.CurrentCustomEmojiImageURL(ctx, tx, remote.ID); err != nil || got != url {
			t.Errorf("CurrentCustomEmojiImageURL = %q, %v; want %q", got, err, url)
		}
		if got, err := statuses.CurrentCustomEmojiImageURL(ctx, tx, local.ID); err != nil || got != "" {
			t.Errorf("CurrentCustomEmojiImageURL(an upload) = %q, %v; want \"\"", got, err)
		}
		if got, err := statuses.FindCustomEmojiByShortcodeTx(ctx, tx, "far", "remote.example"); err != nil || got != remote {
			t.Errorf("FindCustomEmojiByShortcodeTx = %+v, %v", got, err)
		}
		return nil
	})
	if got, err := statuses.FindCustomEmoji(ctx, local.ID); err != nil || got != local {
		t.Errorf("FindCustomEmoji = %+v, %v; want %+v", got, err, local)
	}
	if got, err := statuses.FindCustomEmojis(ctx, []string{local.ID, "01890a5d-0000-7000-8000-000000000000"}); err != nil || len(got) != 1 || got[local.ID] != local {
		t.Errorf("FindCustomEmojis = %+v, %v; want %+v alone, as FindCustomEmoji", got, err, local)
	}
	if got, err := statuses.FindCustomEmojiByShortcode(ctx, "local", ""); err != nil || got != local {
		t.Errorf("FindCustomEmojiByShortcode = %+v, %v; want %+v", got, err, local)
	}
	if _, err := statuses.FindCustomEmojiByShortcode(ctx, "local", "remote.example"); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindCustomEmojiByShortcode(another domain) = %v, want ErrNotFound", err)
	}
	if got, err := statuses.FindCustomEmojiImage(ctx, local.ID); err != nil || got != localImage {
		t.Errorf("FindCustomEmojiImage = %+v, %v; want %+v", got, err, localImage)
	}

	// The media repository reads the emoji of a page of statuses, and whether a file is an image of one.
	media := r.h.Media
	for _, tc := range []struct {
		id   string
		want bool
	}{{localImage.MediaID, true}, {mediaID, true}, {r.id(), false}} {
		if got, err := media.IsCustomEmojiImage(ctx, tc.id); err != nil || got != tc.want {
			t.Errorf("IsCustomEmojiImage(%s) = %v, %v; want %v", tc.id, got, err, tc.want)
		}
	}
	bare := content.CustomEmoji{ID: r.id(), Shortcode: "bare", CreatedAt: r.se.Clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.InsertCustomEmoji(ctx, tx, bare) })
	ids := []string{remote.ID, bare.ID, r.id(), local.ID}
	check := func(name string, got []content.CurrentCustomEmoji, err error) {
		t.Helper()
		if err != nil || len(got) != 2 || got[0].Emoji.ID != local.ID || got[0].Image != localImage || got[1].Emoji.ID != remote.ID ||
			got[1].Image.MediaID != mediaID || got[0].Settings != content.DefaultCustomEmojiSettings(local.ID) {
			t.Errorf("%s = %+v, %v; want local and remote with their images, in ID order", name, got, err)
		}
	}
	got, err := media.ListCurrentCustomEmojis(ctx, ids)
	check("ListCurrentCustomEmojis", got, err)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := media.ListCurrentCustomEmojisTx(ctx, tx, ids)
		check("ListCurrentCustomEmojisTx", got, err)
		return nil
	})
}

// request inserts a request of kind by requester: to create an emoji with shortcode, or to replace
// the image of emoji.
func (r *emojiRepo) request(ctx context.Context, kind content.CustomEmojiRequestKind, requester, shortcode, emojiID string) (content.CustomEmojiRequest, error) {
	r.se.Clock.Advance(time.Second)
	req := content.CustomEmojiRequest{
		ID: r.id(), Kind: kind, RequesterUserID: requester, EmojiID: emojiID, Shortcode: shortcode, MediaID: r.id(), CreatedAt: r.se.Clock.Now(),
	}
	err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.repo.InsertCustomEmojiRequest(ctx, tx, req) })
	return req, err
}

func (r *emojiRepo) mustRequest(t *testing.T, kind content.CustomEmojiRequestKind, requester, shortcode, emojiID string) content.CustomEmojiRequest {
	t.Helper()
	req, err := r.request(t.Context(), kind, requester, shortcode, emojiID)
	if err != nil {
		t.Fatalf("InsertCustomEmojiRequest = %v", err)
	}
	return req
}

func (r *emojiRepo) close(t *testing.T, req content.CustomEmojiRequest, kind content.CustomEmojiRequestOutcomeKind, emojiID string) content.CustomEmojiRequestOutcome {
	t.Helper()
	r.se.Clock.Advance(time.Second)
	o := content.CustomEmojiRequestOutcome{ID: r.id(), RequestID: req.ID, Outcome: kind, EmojiID: emojiID, DecidedAt: r.se.Clock.Now()}
	if kind != content.RequestLapsed {
		o.DecidedByUserID = r.id()
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.CloseCustomEmojiRequest(ctx, tx, o) })
	return o
}

func emojiRepoRequests(t *testing.T, r *emojiRepo) {
	ctx := t.Context()
	creator := r.id()
	existing, _ := r.emoji(t, "taken", "", creator)
	create := r.mustRequest(t, content.RequestCreate, creator, "wave", "")
	if _, err := r.request(ctx, content.RequestCreate, r.id(), "wave", ""); !errors.Is(err, appcontent.ErrShortcodeTaken) {
		t.Errorf("InsertCustomEmojiRequest(a reserved shortcode) = %v, want ErrShortcodeTaken", err)
	}
	if _, err := r.request(ctx, content.RequestCreate, creator, "taken", ""); !errors.Is(err, appcontent.ErrShortcodeTaken) {
		t.Errorf("InsertCustomEmojiRequest(a local emoji's shortcode) = %v, want ErrShortcodeTaken", err)
	}
	if err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return r.repo.InsertCustomEmoji(ctx, tx, content.CustomEmoji{ID: r.id(), Shortcode: "wave", CreatedAt: r.se.Clock.Now()})
	}); !errors.Is(err, appcontent.ErrShortcodeTaken) {
		t.Errorf("InsertCustomEmoji(a reserved shortcode) = %v, want ErrShortcodeTaken", err)
	}
	image := r.mustRequest(t, content.RequestImage, creator, existing.Shortcode, existing.ID)
	if _, err := r.request(ctx, content.RequestImage, creator, existing.Shortcode, existing.ID); !errors.Is(err, appcontent.ErrCustomEmojiRequestPending) {
		t.Errorf("InsertCustomEmojiRequest(a second image request) = %v, want ErrCustomEmojiRequestPending", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.repo.PendingImageRequestOf(ctx, tx, existing.ID); err != nil || got != image.ID {
			t.Errorf("PendingImageRequestOf = %q, %v; want %q", got, err, image.ID)
		}
		got, err := r.repo.ListPendingCustomEmojiRequestsOfUser(ctx, tx, creator)
		slices.Sort(got)
		if err != nil || !slices.Equal(got, []string{create.ID, image.ID}) {
			t.Errorf("ListPendingCustomEmojiRequestsOfUser = %v, %v; want both", got, err)
		}
		return nil
	})
	if got, err := r.repo.FindCustomEmojiRequest(ctx, create.ID); err != nil || got.Request != create || got.Outcome != nil || got.State() != content.RequestPending {
		t.Errorf("FindCustomEmojiRequest(pending) = %+v, %v; want %+v without an outcome", got, err, create)
	}
	if _, err := r.repo.FindCustomEmojiRequest(ctx, r.id()); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindCustomEmojiRequest(unknown) = %v, want ErrNotFound", err)
	}
	for _, tc := range []struct {
		id   string
		want bool
	}{{create.MediaID, true}, {image.MediaID, true}, {r.id(), false}} {
		if got, err := r.repo.IsMediaOfPendingCustomEmojiRequest(ctx, tc.id); err != nil || got != tc.want {
			t.Errorf("IsMediaOfPendingCustomEmojiRequest(%s) = %v, %v; want %v", tc.id, got, err, tc.want)
		}
	}

	// Closing a request records its outcome once and frees what it reserved.
	approved := r.close(t, create, content.RequestApproved, r.id())
	o := approved
	o.ID = r.id()
	if err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.repo.CloseCustomEmojiRequest(ctx, tx, o) }); !errors.Is(err, appcontent.ErrCustomEmojiRequestNotPending) {
		t.Errorf("CloseCustomEmojiRequest(again) = %v, want ErrCustomEmojiRequestNotPending", err)
	}
	if got, err := r.repo.FindCustomEmojiRequest(ctx, create.ID); err != nil || got.Outcome == nil || *got.Outcome != approved {
		t.Errorf("FindCustomEmojiRequest(approved) = %+v, %v; want outcome %+v", got, err, approved)
	}
	if got, err := r.repo.IsMediaOfPendingCustomEmojiRequest(ctx, create.MediaID); err != nil || got {
		t.Errorf("IsMediaOfPendingCustomEmojiRequest(a closed request's media) = %v, %v; want false", got, err)
	}
	r.close(t, image, content.RequestWithdrawn, "")
	r.mustRequest(t, content.RequestCreate, r.id(), "wave", "") // the shortcode is free again
	again := r.mustRequest(t, content.RequestImage, creator, existing.Shortcode, existing.ID)

	// An erased requester's pending requests lapse, and blanking it keeps the requests for the
	// audit log.
	r.close(t, again, content.RequestLapsed, "")
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.BlankCustomEmojiRequester(ctx, tx, creator) })
	for _, id := range []string{create.ID, again.ID} {
		if got, err := r.repo.FindCustomEmojiRequest(ctx, id); err != nil || got.Request.RequesterUserID != "" || got.Outcome == nil {
			t.Errorf("FindCustomEmojiRequest after BlankCustomEmojiRequester = %+v, %v; want it closed, without a requester", got, err)
		}
	}
	if got, err := r.repo.ListCustomEmojiRequests(ctx, appcontent.CustomEmojiRequestQuery{RequesterUserID: creator}, 10); err != nil || len(got) != 0 {
		t.Errorf("ListCustomEmojiRequests(the blanked requester) = %d, %v; want none", len(got), err)
	}
}

func emojiRepoRequestLists(t *testing.T, r *emojiRepo) {
	ctx := t.Context()
	alice, bob := r.id(), r.id()
	since := r.se.Clock.Now()
	a1 := r.mustRequest(t, content.RequestCreate, alice, "a1", "")
	a2 := r.mustRequest(t, content.RequestCreate, alice, "a2", "")
	a3 := r.mustRequest(t, content.RequestCreate, alice, "a3", "")
	a4 := r.mustRequest(t, content.RequestCreate, alice, "a4", "")
	b1 := r.mustRequest(t, content.RequestCreate, bob, "b1", "")
	until := r.se.Clock.Now().Add(time.Second)
	r.close(t, a1, content.RequestApproved, r.id())
	r.close(t, a2, content.RequestRejected, "")
	r.se.Clock.Advance(time.Hour)
	r.mustRequest(t, content.RequestCreate, alice, "late", "") // outside the period

	approved, pending, err := r.repo.CountCustomEmojiUsage(ctx, alice, since, until)
	if err != nil || approved != 1 || pending != 2 {
		t.Errorf("CountCustomEmojiUsage = %d approved, %d pending, %v; want 1 and 2", approved, pending, err)
	}
	ids := func(rs []content.CustomEmojiRequestRecord) []string {
		out := make([]string, len(rs))
		for i, rec := range rs {
			out[i] = rec.Request.ID
		}
		return out
	}
	for _, tc := range []struct {
		name string
		q    appcontent.CustomEmojiRequestQuery
		want []string
	}{
		{"bob's", appcontent.CustomEmojiRequestQuery{RequesterUserID: bob}, []string{b1.ID}},
		{"pending of alice before the late one", appcontent.CustomEmojiRequestQuery{RequesterUserID: alice, State: content.RequestPending, Before: b1.ID}, []string{a4.ID, a3.ID}},
		{"rejected", appcontent.CustomEmojiRequestQuery{State: content.CustomEmojiRequestState(content.RequestRejected)}, []string{a2.ID}},
	} {
		if got, err := r.repo.ListCustomEmojiRequests(ctx, tc.q, 10); err != nil || !slices.Equal(ids(got), tc.want) {
			t.Errorf("ListCustomEmojiRequests(%s) = %v, %v; want %v", tc.name, ids(got), err, tc.want)
		}
	}
	if got, err := r.repo.ListCustomEmojiRequests(ctx, appcontent.CustomEmojiRequestQuery{}, 2); err != nil || len(got) != 2 || got[1].Request != b1 {
		t.Errorf("ListCustomEmojiRequests(limit 2) = %v, %v; want the late one and b1, newest first", ids(got), err)
	}
	if got, err := r.repo.ListCustomEmojiRequestsByID(ctx, []string{b1.ID, r.id(), a1.ID}); err != nil || !slices.Equal(ids(got), []string{a1.ID, b1.ID}) || got[0].Outcome == nil {
		t.Errorf("ListCustomEmojiRequestsByID = %v, %v; want a1 with its outcome and b1", ids(got), err)
	}

	// The rebuild of the pending requests keeps the reservations of the pending ones only.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.RebuildCustomEmojiPendingRequests(ctx, tx) })
	if _, err := r.request(ctx, content.RequestCreate, bob, "a3", ""); !errors.Is(err, appcontent.ErrShortcodeTaken) {
		t.Errorf("InsertCustomEmojiRequest(a pending shortcode after the rebuild) = %v, want ErrShortcodeTaken", err)
	}
	if _, err := r.request(ctx, content.RequestCreate, bob, "a2", ""); err != nil {
		t.Errorf("InsertCustomEmojiRequest(a rejected request's shortcode after the rebuild) = %v", err)
	}
}

func emojiRepoPurge(t *testing.T, r *emojiRepo) {
	ctx := t.Context()
	purge, ok := r.repo.(appcontent.EmojiPurgeRepository)
	if !ok {
		t.Fatalf("the custom emoji repository %T does not implement content.EmojiPurgeRepository", r.repo)
	}
	remoteEmoji := func(shortcode string, url string) (content.CustomEmoji, string) {
		t.Helper()
		e, _ := r.emoji(t, shortcode, "remote.example", "")
		mediaID := r.remoteMedia(t, url)
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.repo.InsertCustomEmojiImage(ctx, tx, content.CustomEmojiImage{ID: r.id(), EmojiID: e.ID, MediaID: mediaID, RevisedAt: r.se.Clock.Now()})
		})
		return e, mediaID
	}
	r.emoji(t, "local", "", r.id())
	unused, unusedMedia := remoteEmoji("unused", "https://remote.example/files/unused.png")
	inPost, _ := remoteEmoji("inpost", "https://remote.example/files/inpost.png")
	disabled, _ := remoteEmoji("disabled", "https://remote.example/files/disabled.png")
	second, _ := remoteEmoji("second", "https://remote.example/files/second.png")
	s := r.se.NewStatus(r.id(), content.VisibilityPublic)
	rev := r.se.FirstRevision(s)
	rev.EmojiIDs = []string{inPost.ID}
	if err := r.se.Create(t, s, nil, rev); err != nil {
		t.Fatal(err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.SetCustomEmojiSettings(ctx, tx, content.CustomEmojiSettings{EmojiID: disabled.ID, VisibleInPicker: true})
	})

	var listed []string
	after := ""
	for range 3 {
		page, err := purge.ListUnreferencedRemoteEmojis(ctx, after, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range page {
			listed, after = append(listed, id), id
		}
	}
	if want := []string{unused.ID, second.ID}; !slices.Equal(listed, want) {
		t.Errorf("ListUnreferencedRemoteEmojis pages = %v, want %v", listed, want)
	}
	mediaIDs, err := purge.CustomEmojiImageMediaIDs(ctx, unused.ID)
	if err != nil || len(mediaIDs) != 2 || !slices.Contains(mediaIDs, unusedMedia) {
		t.Errorf("CustomEmojiImageMediaIDs = %v, %v; want both images' files", mediaIDs, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, tc := range []struct {
			id   string
			want bool
		}{{unused.ID, true}, {unused.ID, false}, {inPost.ID, false}, {disabled.ID, false}} {
			ids, err := purge.CustomEmojiImageMediaIDs(ctx, tc.id)
			if err != nil {
				return err
			}
			if tc.id == unused.ID {
				ids = mediaIDs
			}
			if got, err := purge.PurgeRemoteEmoji(ctx, tx, tc.id, ids); err != nil || got != tc.want {
				t.Errorf("PurgeRemoteEmoji(%s) = %v, %v; want %v", tc.id, got, err, tc.want)
			}
		}
		return nil
	})
	if _, err := r.repo.FindCustomEmojiRecord(ctx, unused.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindCustomEmojiRecord of a purged emoji = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Media.FindMediaFile(ctx, unusedMedia); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindMediaFile of a purged emoji's remote image = %v, want ErrNotFound", err)
	}
	if _, err := r.repo.FindCustomEmojiRecord(ctx, inPost.ID); err != nil {
		t.Errorf("FindCustomEmojiRecord of a referenced emoji = %v", err)
	}
}
