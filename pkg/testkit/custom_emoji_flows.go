// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// CustomEmojiHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the content module's repositories of media files, statuses, and the custom
// emoji that the administrators manage.
type CustomEmojiHarness struct {
	UnitOfWork uow.UnitOfWork
	Media      appcontent.MediaRepository
	Statuses   appcontent.Repository
	Emojis     appcontent.CustomEmojiRepository
}

// CustomEmojiFactory returns a harness on a new, empty database whose UnitOfWork uses opts. The
// factory registers the cleanup of everything it creates with t.Cleanup.
type CustomEmojiFactory func(t *testing.T, opts uow.Options) CustomEmojiHarness

// CustomEmojiFlows verifies the custom emoji use cases (ADR-0051) on a database adapter, the
// flows of ADR-0153 §1, which internal/flowtest/content runs once on SQLite;
// CustomEmojiRepositoryContract checks the repositories' methods on every adapter. The flows cover
// local emoji registered with a processed upload
// of the administrator and refused when the shortcode, an alias, the category, or the image
// breaks a rule; settings and aliases as current values, with the disabled and hidden emoji left
// out of the picker, the views, the Emoji tags, and the emoji of local texts; new images as new
// revisions; local emoji deleted with their rows while their images become unattached uploads;
// remote emoji that can only be enabled and disabled, whose image changes only with its URL;
// remote emoji copied into local ones through the media proxy, and copies refused without a
// trace when the image cannot be fetched or does not qualify;
// the administrators' list with its filters
// and pages; every operation audited by an event in its transaction; and the requests of the
// holders of create_custom_emojis (custom_emoji_request_flows.go, ADR-0070).
func CustomEmojiFlows(t *testing.T, newHarness CustomEmojiFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *emojiEnv)
	}{
		{"RegistersLocalEmoji", emojiRegistersLocal},
		{"RefusesEmojiThatBreakARule", emojiRefuses},
		{"KeepsSettingsAndAliasesAsCurrentValues", emojiKeepsSettings},
		{"ReplacesImagesWithNewRevisions", emojiReplacesImages},
		{"DeletesLocalEmoji", emojiDeletesLocal},
		{"OnlyEnablesAndDisablesRemoteEmoji", emojiLimitsRemote},
		{"ReplacesRemoteImagesByURL", emojiReplacesRemoteImagesByURL},
		{"ListsAndFiltersEmoji", emojiLists},
		{"ReadsAnEmojiInATransaction", emojiReadsInTransaction},
		{"CopiesRemoteEmoji", emojiCopiesRemote},
		{"RefusesCopiesWithoutATrace", emojiRefusesCopies},
		{"HidesRequestsUntilApproved", requestHidesUntilApproved},
		{"RejectsAndWithdrawsRequests", requestRejectsAndWithdraws},
		{"ReservesTheShortcodesOfPendingRequests", requestReservesShortcodes},
		{"LimitsCreatorsToTheirOwnEmoji", requestLimitsCreatorsToTheirOwn},
		{"LapsesImageRequestsWithTheirEmoji", requestLapsesWithItsEmoji},
		{"RebuildsPendingRequests", requestRebuildsPendingRequests},
		{"RequestLapsesWithItsRequester", requestLapsesWithItsRequester},
		{"AppliesTheLimitOfTheCreators", requestAppliesTheLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newEmojiEnv(t, newHarness)) })
	}
}

const (
	emojiAdmin     = "0192f0c1-0000-7000-8000-0000000ad111"
	emojiAdminUser = "0192f0c1-0000-7000-8000-0000000ad112"
	emojiOther     = "0192f0c1-0000-7000-8000-0000000ad113"
	emojiSenderA   = "0192f0c1-0000-7000-8000-0000000ad114"
	emojiSenderB   = "0192f0c1-0000-7000-8000-0000000ad115"
)

type emojiEnv struct {
	h          CustomEmojiHarness
	projectors *uow.Projectors
	clock      *Clock
	ids        *IDGenerator
	blobs      *MemoryBlobStore
	net        *emojiNet
	media      *appcontent.MediaService
	service    *appcontent.CustomEmojiService

	mu     sync.Mutex
	events []appcontent.CustomEmojiAdministered
}

func newEmojiEnv(t *testing.T, newHarness CustomEmojiFactory) *emojiEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appcontent.RegisterProjectors(projectors, h.Statuses)
	clock := NewClock(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	e := &emojiEnv{
		h: h, projectors: projectors, clock: clock, ids: NewIDGenerator(clock), blobs: NewMemoryBlobStore(clock),
		net: &emojiNet{bodies: map[string][]byte{}, rejected: map[string]bool{}},
	}
	// The events are kept once their transaction commits, as the audit log keeps its entries.
	projectors.OnCommit(appcontent.EventCustomEmojiAdministered, func(_ context.Context, ev uow.Event) {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.events = append(e.events, ev.(appcontent.CustomEmojiAdministered))
	})
	// The media proxy fetches the images of remote emoji from e.net, and keeps no copy.
	proxy, err := appcontent.NewMediaProxy(appcontent.ProxyDeps{
		Repository: h.Media, HTTP: e.net, Accounts: remoteDomains{}, Policy: e.net,
		Settings: func(context.Context) (appcontent.ProxySettings, error) {
			return appcontent.ProxySettings{Policy: appcontent.RemoteMediaCacheNone, CacheTTL: time.Hour}, nil
		},
		UnitOfWork: h.UnitOfWork, Clock: clock, Keys: [][]byte{bytes.Repeat([]byte("k"), 32)}, BaseURL: "https://social.example",
		Processor: FakeMediaProcessor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.media = appcontent.NewMediaService(appcontent.MediaDeps{
		UnitOfWork: h.UnitOfWork, Repository: h.Media, Blobs: e.blobs, Processor: FakeMediaProcessor{},
		Jobs: &recordingJobs{}, Clock: clock, IDs: e.ids, Limits: media.Limits{MaxBytes: 1 << 20}, Proxy: proxy,
	})
	e.service = appcontent.NewCustomEmojiService(appcontent.CustomEmojiDeps{
		UnitOfWork: h.UnitOfWork, Repository: h.Emojis, Media: e.media, Clock: clock, IDs: e.ids, MaxBytes: 64 << 10,
	})
	return e
}

// image uploads data as owner for a custom emoji and processes it.
func (e *emojiEnv) image(t *testing.T, owner string, data []byte) string {
	t.Helper()
	e.clock.Advance(time.Millisecond)
	m, err := e.media.Upload(t.Context(), appcontent.UploadInput{OwnerAccountID: owner, Content: bytes.NewReader(data), Purpose: content.PurposeEmoji})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if res := e.media.Process(t.Context(), m.File.ID, content.PurposeEmoji); !res.IsDone() {
		t.Fatalf("Process = %+v", res)
	}
	return m.File.ID
}

func (e *emojiEnv) create(t *testing.T, shortcode string, opts ...func(*appcontent.CreateCustomEmojiInput)) appcontent.AdminCustomEmoji {
	t.Helper()
	in := appcontent.CreateCustomEmojiInput{
		Shortcode: shortcode, MediaID: e.image(t, emojiAdmin, FixturePNG(4, 3, true)), OwnerAccountID: emojiAdmin,
		VisibleInPicker: true, ByUserID: emojiAdminUser,
	}
	for _, o := range opts {
		o(&in)
	}
	e.clock.Advance(time.Second)
	got, err := e.service.Create(t.Context(), in)
	if err != nil {
		t.Fatalf("Create(%s): %v", shortcode, err)
	}
	return got
}

// takeEvents returns the actions of the events since the last call, in order.
func (e *emojiEnv) takeEvents() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for _, ev := range e.events {
		out = append(out, ev.Action)
	}
	e.events = nil
	return out
}

func emojiReason(err error) content.CustomEmojiReason {
	var ce *content.CustomEmojiError
	if errors.As(err, &ce) {
		return ce.Reason
	}
	return ""
}

// remote records a remote emoji as a received document would.
func (e *emojiEnv) remote(t *testing.T, shortcode, domain string) content.CustomEmoji {
	t.Helper()
	e.clock.Advance(time.Second)
	var stored content.CustomEmoji
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		var err error
		stored, err = e.h.Statuses.EnsureCustomEmoji(t.Context(), tx, content.CustomEmoji{
			ID: e.ids.NewID(), Shortcode: shortcode, Domain: domain, URI: "https://" + domain + "/emojis/" + shortcode, CreatedAt: e.clock.Now(),
		})
		if err != nil {
			return err
		}
		return e.h.Statuses.InsertCustomEmojiImage(t.Context(), tx, content.CustomEmojiImage{
			ID: e.ids.NewID(), EmojiID: stored.ID, MediaID: e.ids.NewID(), RevisedAt: e.clock.Now(),
		})
	})
	if err != nil {
		t.Fatalf("record remote emoji: %v", err)
	}
	return stored
}

func emojiRegistersLocal(t *testing.T, e *emojiEnv) {
	got := e.create(t, "blobcat", func(in *appcontent.CreateCustomEmojiInput) {
		in.Category, in.Aliases = "  Cats ", []string{"neko", "blobcat", "kitty"}
	})
	if !got.Emoji.Local() || got.Emoji.Shortcode != "blobcat" || got.Emoji.URI != "" || !got.Emoji.CreatedAt.Equal(e.clock.Now()) {
		t.Errorf("Emoji = %+v", got.Emoji)
	}
	if got.Settings != (content.CustomEmojiSettings{EmojiID: got.Emoji.ID, Enabled: true, VisibleInPicker: true, Category: "Cats"}) {
		t.Errorf("Settings = %+v", got.Settings)
	}
	if !slices.Equal(got.Aliases, []string{"kitty", "neko"}) {
		t.Errorf("Aliases = %q", got.Aliases)
	}
	if got.View == nil || got.View.URL == "" || got.View.StaticURL == "" {
		t.Fatalf("View = %+v", got.View)
	}
	if ev := e.takeEvents(); !slices.Equal(ev, []string{appcontent.CustomEmojiCreated}) {
		t.Errorf("events = %v", ev)
	}
	ctx := t.Context()
	ids, err := e.service.LocalEmojiIDs(ctx, "hi :blobcat: and :unknown:", ":blobcat:")
	if err != nil || !slices.Equal(ids, []string{got.Emoji.ID}) {
		t.Errorf("LocalEmojiIDs = %v, %v", ids, err)
	}
	picker, err := e.service.Picker(ctx)
	if err != nil || len(picker) != 1 || picker[0].Shortcode != "blobcat" || picker[0].Category != "Cats" || picker[0].URL != got.View.URL {
		t.Errorf("Picker = %+v, %v", picker, err)
	}
	views, err := e.media.CustomEmojis(ctx, []string{got.Emoji.ID})
	if err != nil || views[got.Emoji.ID] != *got.View {
		t.Errorf("CustomEmojis = %+v, %v; want %+v", views, err, got.View)
	}
	tags, err := e.media.EmojiTags(ctx, []string{got.Emoji.ID})
	if err != nil || len(tags) != 1 || !tags[0].Local || tags[0].Shortcode != "blobcat" || tags[0].MediaType != "image/png" ||
		tags[0].URL == "" || !tags[0].Updated.Equal(got.Image.RevisedAt) {
		t.Errorf("EmojiTags = %+v, %v", tags, err)
	}
	// The image of an emoji is never an unattached upload, whoever uploaded it.
	uploads, err := e.h.Media.ListOwnerUploads(ctx, emojiAdmin, "", 10)
	if err != nil || len(uploads) != 1 || !uploads[0].Attached {
		t.Errorf("ListOwnerUploads = %+v, %v; want the emoji image as attached", uploads, err)
	}
	e.clock.Advance(appcontent.DefaultUnattachedTTL + time.Hour)
	if n, err := e.media.CleanupUnattached(ctx); err != nil || n != 0 {
		t.Errorf("CleanupUnattached = %d, %v; want the emoji image kept", n, err)
	}
}

func emojiRefuses(t *testing.T, e *emojiEnv) {
	e.create(t, "taken")
	e.takeEvents()
	png := e.image(t, emojiAdmin, FixturePNG(4, 3, true))
	jpeg := e.image(t, emojiAdmin, FixtureJPEG(4, 3, 1))
	large := e.image(t, emojiAdmin, append(FixturePNG(4, 3, true), make([]byte, 64<<10)...))
	others := e.image(t, emojiOther, FixturePNG(4, 3, true))
	e.clock.Advance(time.Millisecond)
	pending, err := e.media.Upload(t.Context(), appcontent.UploadInput{OwnerAccountID: emojiAdmin, Content: bytes.NewReader(FixturePNG(4, 3, true))})
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		in   appcontent.CreateCustomEmojiInput
		want content.CustomEmojiReason
	}{
		"short shortcode":   {appcontent.CreateCustomEmojiInput{Shortcode: "a", MediaID: png}, content.EmojiInvalidShortcode},
		"hyphen":            {appcontent.CreateCustomEmojiInput{Shortcode: "blob-cat", MediaID: png}, content.EmojiInvalidShortcode},
		"taken":             {appcontent.CreateCustomEmojiInput{Shortcode: "taken", MediaID: png}, content.EmojiShortcodeTaken},
		"bad alias":         {appcontent.CreateCustomEmojiInput{Shortcode: "ok", MediaID: png, Aliases: []string{"ね"}}, content.EmojiInvalidAlias},
		"bad category":      {appcontent.CreateCustomEmojiInput{Shortcode: "ok", MediaID: png, Category: "a\nb"}, content.EmojiInvalidCategory},
		"jpeg":              {appcontent.CreateCustomEmojiInput{Shortcode: "ok", MediaID: jpeg}, content.EmojiUnsupportedFormat},
		"over the limit":    {appcontent.CreateCustomEmojiInput{Shortcode: "ok", MediaID: large}, content.EmojiTooLarge},
		"another's upload":  {appcontent.CreateCustomEmojiInput{Shortcode: "ok", MediaID: others}, content.EmojiMediaNotFound},
		"unknown upload":    {appcontent.CreateCustomEmojiInput{Shortcode: "ok", MediaID: e.ids.NewID()}, content.EmojiMediaNotFound},
		"processing upload": {appcontent.CreateCustomEmojiInput{Shortcode: "ok", MediaID: pending.File.ID}, content.EmojiMediaNotReady},
	} {
		tc.in.OwnerAccountID = emojiAdmin
		_, err := e.service.Create(t.Context(), tc.in)
		if got := emojiReason(err); got != tc.want || !errors.Is(err, content.ErrInvalidCustomEmoji) {
			t.Errorf("%s: Create = %v, want %s", name, err, tc.want)
		}
	}
	list, err := e.service.List(t.Context(), appcontent.CustomEmojiQuery{}, 10)
	if err != nil || len(list) != 1 {
		t.Errorf("refused emoji were stored: %d, %v", len(list), err)
	}
	if ev := e.takeEvents(); len(ev) != 0 {
		t.Errorf("refused emoji emitted %v", ev)
	}
}

func emojiKeepsSettings(t *testing.T, e *emojiEnv) {
	ctx := t.Context()
	cat := e.create(t, "blobcat")
	e.takeEvents()
	off, on, hidden := false, true, false
	got, err := e.service.Update(ctx, cat.Emoji.ID, appcontent.CustomEmojiUpdate{Enabled: &off, ByUserID: emojiAdminUser})
	if err != nil || got.Settings.Enabled {
		t.Fatalf("disable = %+v, %v", got.Settings, err)
	}
	if ev := e.takeEvents(); !slices.Equal(ev, []string{appcontent.CustomEmojiDisabled}) {
		t.Errorf("disable events = %v", ev)
	}
	if ids, _ := e.service.LocalEmojiIDs(ctx, ":blobcat:"); len(ids) != 0 {
		t.Errorf("a disabled emoji is used by texts: %v", ids)
	}
	if views, _ := e.media.CustomEmojis(ctx, []string{cat.Emoji.ID}); len(views) != 0 {
		t.Errorf("a disabled emoji is shown: %v", views)
	}
	if tags, _ := e.media.EmojiTags(ctx, []string{cat.Emoji.ID}); len(tags) != 0 {
		t.Errorf("a disabled emoji is federated: %v", tags)
	}
	if p, _ := e.service.Picker(ctx); len(p) != 0 {
		t.Errorf("the picker offers a disabled emoji: %v", p)
	}
	// Disabling again changes nothing and records nothing.
	if _, err := e.service.Update(ctx, cat.Emoji.ID, appcontent.CustomEmojiUpdate{Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	if ev := e.takeEvents(); len(ev) != 0 {
		t.Errorf("a change of nothing emitted %v", ev)
	}
	category, aliases := "Cats", []string{"neko"}
	got, err = e.service.Update(ctx, cat.Emoji.ID, appcontent.CustomEmojiUpdate{
		Enabled: &on, VisibleInPicker: &hidden, Category: &category, Aliases: &aliases, ByUserID: emojiAdminUser,
	})
	if err != nil || !got.Settings.Enabled || got.Settings.VisibleInPicker || got.Settings.Category != "Cats" || !slices.Equal(got.Aliases, aliases) {
		t.Fatalf("update = %+v %q, %v", got.Settings, got.Aliases, err)
	}
	if ev := e.takeEvents(); !slices.Equal(ev, []string{appcontent.CustomEmojiEnabled, appcontent.CustomEmojiUpdated}) {
		t.Errorf("update events = %v", ev)
	}
	// A hidden emoji is not offered, but texts use it.
	if p, _ := e.service.Picker(ctx); len(p) != 0 {
		t.Errorf("the picker offers a hidden emoji: %v", p)
	}
	if ids, _ := e.service.LocalEmojiIDs(ctx, ":blobcat:"); !slices.Equal(ids, []string{cat.Emoji.ID}) {
		t.Errorf("a hidden emoji is not used by texts: %v", ids)
	}
	// Back to the defaults, and a read sees them.
	empty, none, shown := "", []string{}, true
	if _, err := e.service.Update(ctx, cat.Emoji.ID, appcontent.CustomEmojiUpdate{VisibleInPicker: &shown, Category: &empty, Aliases: &none}); err != nil {
		t.Fatal(err)
	}
	got, err = e.service.Get(ctx, cat.Emoji.ID)
	if err != nil || got.Settings != content.DefaultCustomEmojiSettings(cat.Emoji.ID) || len(got.Aliases) != 0 {
		t.Errorf("Get = %+v %q, %v; want the defaults", got.Settings, got.Aliases, err)
	}
	bad := []string{"bad-alias"}
	if _, err := e.service.Update(ctx, cat.Emoji.ID, appcontent.CustomEmojiUpdate{Aliases: &bad}); emojiReason(err) != content.EmojiInvalidAlias {
		t.Errorf("bad alias = %v", err)
	}
	if _, err := e.service.Update(ctx, e.ids.NewID(), appcontent.CustomEmojiUpdate{Enabled: &on}); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Update(unknown) = %v, want ErrNotFound", err)
	}
}

func emojiReplacesImages(t *testing.T, e *emojiEnv) {
	ctx := t.Context()
	cat := e.create(t, "blobcat")
	e.takeEvents()
	gif := e.image(t, emojiAdmin, FixtureGIF(4, 3, 2))
	e.clock.Advance(time.Minute)
	got, err := e.service.ReplaceImage(ctx, cat.Emoji.ID, emojiAdmin, gif, emojiAdminUser)
	if err != nil {
		t.Fatal(err)
	}
	if got.Image.MediaID != gif || !got.Image.RevisedAt.Equal(e.clock.Now()) || got.Image.ID == cat.Image.ID {
		t.Errorf("Image = %+v", got.Image)
	}
	if got.View == nil || got.View.URL == got.View.StaticURL {
		t.Errorf("an animated emoji has no moving image: %+v", got.View)
	}
	tags, err := e.media.EmojiTags(ctx, []string{cat.Emoji.ID})
	if err != nil || len(tags) != 1 || tags[0].MediaType != "image/gif" || !tags[0].Updated.Equal(e.clock.Now()) {
		t.Errorf("EmojiTags = %+v, %v; want the moving large variant, updated now", tags, err)
	}
	if ev := e.takeEvents(); !slices.Equal(ev, []string{appcontent.CustomEmojiImageReplaced}) {
		t.Errorf("events = %v", ev)
	}
	if _, err := e.service.ReplaceImage(ctx, cat.Emoji.ID, emojiAdmin, e.image(t, emojiAdmin, FixtureJPEG(4, 3, 1)), ""); emojiReason(err) != content.EmojiUnsupportedFormat {
		t.Errorf("ReplaceImage(jpeg) = %v", err)
	}
	// The previous image stays an image of the emoji, so the cleanup keeps it too.
	e.clock.Advance(appcontent.DefaultUnattachedTTL + time.Hour)
	if n, err := e.media.CleanupUnattached(ctx); err != nil || n != 1 {
		t.Errorf("CleanupUnattached = %d, %v; want only the refused JPEG deleted", n, err)
	}
}

func emojiDeletesLocal(t *testing.T, e *emojiEnv) {
	ctx := t.Context()
	cat := e.create(t, "blobcat")
	e.takeEvents()
	if ids, err := e.service.ExistingIDs(ctx, []string{cat.Emoji.ID}); err != nil || !slices.Equal(ids, []string{cat.Emoji.ID}) {
		t.Errorf("ExistingIDs = %v, %v; want the emoji", ids, err)
	}
	if err := e.service.Delete(ctx, cat.Emoji.ID, emojiAdminUser); err != nil {
		t.Fatal(err)
	}
	if ids, err := e.service.ExistingIDs(ctx, []string{cat.Emoji.ID}); err != nil || len(ids) != 0 {
		t.Errorf("ExistingIDs after the deletion = %v, %v; want none", ids, err)
	}
	if ev := e.takeEvents(); !slices.Equal(ev, []string{appcontent.CustomEmojiDeleted}) {
		t.Errorf("events = %v", ev)
	}
	if _, err := e.service.Get(ctx, cat.Emoji.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Get(deleted) = %v, want ErrNotFound", err)
	}
	if ids, _ := e.service.LocalEmojiIDs(ctx, ":blobcat:"); len(ids) != 0 {
		t.Errorf("a deleted emoji is used: %v", ids)
	}
	if views, _ := e.media.CustomEmojis(ctx, []string{cat.Emoji.ID}); len(views) != 0 {
		t.Errorf("a deleted emoji is shown: %v", views)
	}
	if err := e.service.Delete(ctx, cat.Emoji.ID, ""); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Delete(deleted) = %v, want ErrNotFound", err)
	}
	// The shortcode is free again, for another emoji.
	again := e.create(t, "blobcat")
	if again.Emoji.ID == cat.Emoji.ID {
		t.Error("the new emoji has the deleted one's ID")
	}
	// The image of the deleted emoji is an unattached upload, which the cleanup deletes.
	e.clock.Advance(appcontent.DefaultUnattachedTTL + time.Hour)
	if n, err := e.media.CleanupUnattached(ctx); err != nil || n != 1 {
		t.Errorf("CleanupUnattached = %d, %v; want the deleted emoji's image", n, err)
	}
	if _, err := e.media.Media(ctx, cat.Image.MediaID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("the deleted emoji's image is kept: %v", err)
	}
}

func emojiLimitsRemote(t *testing.T, e *emojiEnv) {
	ctx := t.Context()
	remote := e.remote(t, "blobfox", "remote.example")
	off, on, hidden, category := false, true, false, "x"
	got, err := e.service.Update(ctx, remote.ID, appcontent.CustomEmojiUpdate{Enabled: &off})
	if err != nil || got.Settings.Enabled || got.Emoji.Local() {
		t.Fatalf("disable remote = %+v, %v", got, err)
	}
	if ev := e.takeEvents(); !slices.Equal(ev, []string{appcontent.CustomEmojiDisabled}) {
		t.Errorf("events = %v", ev)
	}
	if views, _ := e.media.CustomEmojis(ctx, []string{remote.ID}); len(views) != 0 {
		t.Errorf("a disabled remote emoji is shown: %v", views)
	}
	for name, u := range map[string]appcontent.CustomEmojiUpdate{
		"picker": {VisibleInPicker: &hidden}, "category": {Category: &category}, "aliases": {Aliases: &[]string{"fox"}},
	} {
		if _, err := e.service.Update(ctx, remote.ID, u); emojiReason(err) != content.EmojiRemote {
			t.Errorf("%s of a remote emoji = %v", name, err)
		}
	}
	if err := e.service.Delete(ctx, remote.ID, ""); emojiReason(err) != content.EmojiRemote {
		t.Errorf("Delete(remote) = %v", err)
	}
	if _, err := e.service.ReplaceImage(ctx, remote.ID, emojiAdmin, e.image(t, emojiAdmin, FixturePNG(4, 3, true)), ""); emojiReason(err) != content.EmojiRemote {
		t.Errorf("ReplaceImage(remote) = %v", err)
	}
	if _, err := e.service.Update(ctx, remote.ID, appcontent.CustomEmojiUpdate{Enabled: &on}); err != nil {
		t.Fatal(err)
	}
	// A remote emoji with a local one's shortcode is another emoji, never used by local texts.
	e.create(t, "blobfox")
	ids, err := e.service.LocalEmojiIDs(ctx, ":blobfox:")
	if err != nil || len(ids) != 1 || ids[0] == remote.ID {
		t.Errorf("LocalEmojiIDs = %v, %v", ids, err)
	}
}

func emojiLists(t *testing.T, e *emojiEnv) {
	ctx := t.Context()
	a := e.create(t, "Blob_A")
	b := e.create(t, "other")
	r1 := e.remote(t, "blob_remote", "one.example")
	r2 := e.remote(t, "cat", "two.example")
	ids := func(list []appcontent.AdminCustomEmoji) []string {
		var out []string
		for _, x := range list {
			out = append(out, x.Emoji.ID)
		}
		return out
	}
	for name, tc := range map[string]struct {
		q    appcontent.CustomEmojiQuery
		want []string
	}{
		"all":       {appcontent.CustomEmojiQuery{}, []string{r2.ID, r1.ID, b.Emoji.ID, a.Emoji.ID}},
		"local":     {appcontent.CustomEmojiQuery{Locality: appcontent.LocalEmoji}, []string{b.Emoji.ID, a.Emoji.ID}},
		"remote":    {appcontent.CustomEmojiQuery{Locality: appcontent.RemoteEmoji}, []string{r2.ID, r1.ID}},
		"domain":    {appcontent.CustomEmojiQuery{Domain: "one.example"}, []string{r1.ID}},
		"shortcode": {appcontent.CustomEmojiQuery{Shortcode: "BLOB"}, []string{r1.ID, a.Emoji.ID}},
		"page":      {appcontent.CustomEmojiQuery{Before: r1.ID}, []string{b.Emoji.ID, a.Emoji.ID}},
	} {
		got, err := e.service.List(ctx, tc.q, 10)
		if err != nil || !slices.Equal(ids(got), tc.want) {
			t.Errorf("%s: List = %v, %v; want %v", name, ids(got), err, tc.want)
		}
	}
	got, err := e.service.List(ctx, appcontent.CustomEmojiQuery{}, 1)
	if err != nil || !slices.Equal(ids(got), []string{r2.ID}) {
		t.Errorf("List(limit 1) = %v, %v", ids(got), err)
	}
	remote, err := e.service.Get(ctx, r1.ID)
	if err != nil || remote.Emoji.Domain != "one.example" || remote.Emoji.URI == "" || !remote.Settings.Enabled {
		t.Errorf("Get(remote) = %+v, %v", remote, err)
	}
}

func emojiReplacesRemoteImagesByURL(t *testing.T, e *emojiEnv) {
	ctx := t.Context()
	statuses := appcontent.NewService(appcontent.Deps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Statuses, RemoteMedia: e.media, Clock: e.clock, IDs: e.ids,
	})
	// receive records :blobfox: of remote.example with the image at url, as a document of owner.
	receive := func(owner, url string) content.CustomEmojiImage {
		t.Helper()
		e.clock.Advance(time.Second)
		var ids []string
		err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			ids, err = statuses.RecordRemoteEmojis(ctx, tx, owner, "remote.example", []appcontent.RemoteEmojiInput{
				{Shortcode: "blobfox", URI: "https://remote.example/emojis/blobfox", ImageURL: url},
			})
			return err
		})
		if err != nil || len(ids) != 1 {
			t.Fatalf("RecordRemoteEmojis(%s) = %v, %v", url, ids, err)
		}
		_, img, err := statuses.CustomEmoji(ctx, ids[0])
		if err != nil {
			t.Fatalf("CustomEmoji: %v", err)
		}
		return img
	}
	const v1, v2 = "https://remote.example/files/blobfox.png", "https://remote.example/files/blobfox-2.png"
	first := receive(emojiSenderA, v1)
	// The same image in another document, or in a document of another account, is no new image.
	for _, owner := range []string{emojiSenderA, emojiSenderB} {
		if got := receive(owner, v1); got != first {
			t.Errorf("the same image from %s = %+v, want the first image %+v", owner, got, first)
		}
	}
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := e.h.Media.RemoteMediaByURL(ctx, tx, emojiSenderB, v1)
		return err
	})
	if !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("RemoteMediaByURL(B, v1) = %v; want no media file of the second sender", err)
	}
	// A new URL is a new image, from whichever account.
	second := receive(emojiSenderB, v2)
	if second.ID == first.ID || second.MediaID == first.MediaID || !second.RevisedAt.Equal(e.clock.Now()) {
		t.Errorf("a new image = %+v, want a new revision after %+v", second, first)
	}
	if got := receive(emojiSenderA, v2); got != second {
		t.Errorf("the new image again = %+v, want %+v", got, second)
	}
	// Going back to the first URL is a change too.
	if back := receive(emojiSenderA, v1); back.ID == second.ID || back.MediaID != first.MediaID {
		t.Errorf("the first image again = %+v, want a new revision of media %s", back, first.MediaID)
	}
}
