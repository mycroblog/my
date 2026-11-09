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
	"github.com/mycroblog/my/pkg/testkit"
)

// LinkCardHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the content module's link card and media repositories on it.
type LinkCardHarness struct {
	UnitOfWork uow.UnitOfWork
	Cards      appcontent.LinkCardRepository
	Media      appcontent.MediaRepository
}

// LinkCardFactory returns a harness on a new, empty database whose UnitOfWork uses opts. The
// factory registers the cleanup of everything it creates with t.Cleanup.
type LinkCardFactory func(t *testing.T, opts uow.Options) LinkCardHarness

// LinkCardRepositoryContract verifies the link card repository of a database adapter
// (ADR-0062): the URL whose card each status shows, set, replaced, and removed; the cards
// upserted by URL with their optional values, read in and outside transactions; the images
// recorded once per URL with a media file without an owner; and the cleanup of the cards that
// no status refers to and of the images that no card uses.
func LinkCardRepositoryContract(t *testing.T, newHarness LinkCardFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, h LinkCardHarness)
	}{
		{"KeepsTheURLOfEachStatus", linkCardStatusURLs},
		{"UpsertsCardsByURL", linkCardUpserts},
		{"RecordsImagesOncePerURL", linkCardImages},
		{"CleansUpUnreferencedCardsAndImages", linkCardCleanup},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t, newHarness(t, uow.Options{}))
		})
	}
}

func linkCardTx(t *testing.T, h LinkCardHarness, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	ctx := t.Context()
	if err := h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return fn(ctx, tx) }); err != nil {
		t.Fatal(err)
	}
}

func linkCardStatusURLs(t *testing.T, h LinkCardHarness) {
	ctx := t.Context()
	if _, err := h.Cards.StatusLinkCardURL(ctx, testkit.NamedID("s1")); !errors.Is(err, appcontent.ErrNotFound) {
		t.Fatalf("StatusLinkCardURL of a status without a card: %v, want ErrNotFound", err)
	}
	linkCardTx(t, h, func(ctx context.Context, tx uow.Tx) error {
		if err := h.Cards.UpsertStatusLinkCard(ctx, tx, content.StatusLinkCard{StatusID: testkit.NamedID("s1"), URL: "https://a.example/"}); err != nil {
			return err
		}
		return h.Cards.UpsertStatusLinkCard(ctx, tx, content.StatusLinkCard{StatusID: testkit.NamedID("s1"), URL: "https://b.example/"})
	})
	if u, err := h.Cards.StatusLinkCardURL(ctx, testkit.NamedID("s1")); err != nil || u != "https://b.example/" {
		t.Errorf("StatusLinkCardURL = %q, %v, want the replaced URL", u, err)
	}
	if got, err := h.Cards.StatusLinkCardURLs(ctx, []string{testkit.NamedID("s1"), testkit.NamedID("none")}); err != nil || len(got) != 1 || got[testkit.NamedID("s1")] != "https://b.example/" {
		t.Errorf("StatusLinkCardURLs = %v, %v; want the URL of s1 alone, as StatusLinkCardURL", got, err)
	}
	linkCardTx(t, h, func(ctx context.Context, tx uow.Tx) error {
		if err := h.Cards.DeleteStatusLinkCard(ctx, tx, testkit.NamedID("s1")); err != nil {
			return err
		}
		return h.Cards.DeleteStatusLinkCard(ctx, tx, testkit.NamedID("s1")) // deleting nothing is not an error
	})
	if _, err := h.Cards.StatusLinkCardURL(ctx, testkit.NamedID("s1")); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("StatusLinkCardURL after the deletion: %v, want ErrNotFound", err)
	}
}

func linkCardUpserts(t *testing.T, h LinkCardHarness) {
	ctx := t.Context()
	at := time.Date(2026, 10, 1, 12, 0, 0, 123e6, time.UTC)
	published := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	full := content.LinkCard{
		URL: "https://news.example/a", CardURL: "https://news.example/a?ref=og", Title: "Title", Description: "Desc",
		SiteName: "News", AuthorName: "Ann", PublishedAt: &published, ImageMediaID: "0192f0c1-0000-7000-8000-000000000001",
		CreatorAccountID: "0192f0c1-0000-7000-8000-000000000002", Language: "en", FetchedAt: at,
	}
	if _, err := h.Cards.FindLinkCard(ctx, full.URL); !errors.Is(err, appcontent.ErrNotFound) {
		t.Fatalf("FindLinkCard of an unknown URL: %v, want ErrNotFound", err)
	}
	linkCardTx(t, h, func(ctx context.Context, tx uow.Tx) error {
		if err := h.Cards.UpsertLinkCard(ctx, tx, full); err != nil {
			return err
		}
		got, err := h.Cards.FindLinkCardTx(ctx, tx, full.URL)
		if err != nil || !reflect.DeepEqual(got, full) {
			t.Errorf("FindLinkCardTx in the transaction = %+v, %v", got, err)
		}
		return nil
	})
	if got, err := h.Cards.FindLinkCard(ctx, full.URL); err != nil || !reflect.DeepEqual(got, full) {
		t.Errorf("FindLinkCard = %+v, %v\nwant %+v", got, err, full)
	}
	if got, err := h.Cards.FindLinkCards(ctx, []string{full.URL, "https://none.example/"}); err != nil || len(got) != 1 || !reflect.DeepEqual(got[full.URL], full) {
		t.Errorf("FindLinkCards = %+v, %v; want the card alone, as FindLinkCard", got, err)
	}
	bare := content.LinkCard{URL: full.URL, CardURL: full.URL, Title: "New", SiteName: "news.example", FetchedAt: at.Add(time.Hour)}
	linkCardTx(t, h, func(ctx context.Context, tx uow.Tx) error { return h.Cards.UpsertLinkCard(ctx, tx, bare) })
	if got, err := h.Cards.FindLinkCard(ctx, full.URL); err != nil || !reflect.DeepEqual(got, bare) {
		t.Errorf("FindLinkCard after the refetch = %+v, %v\nwant %+v", got, err, bare)
	}
}

func linkCardImages(t *testing.T, h LinkCardHarness) {
	ctx := t.Context()
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	m := content.LinkCardMedia{
		MediaID: "0192f0c1-0000-7000-8000-00000000000a", RemoteURL: "https://news.example/a.png", ContentType: "image/png",
		Width: 640, Height: 480, Blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj", Description: "Alt",
	}
	linkCardTx(t, h, func(ctx context.Context, tx uow.Tx) error {
		if ok, err := h.Cards.InsertLinkCardMedia(ctx, tx, m, at); err != nil || !ok {
			t.Errorf("InsertLinkCardMedia = %v, %v, want inserted", ok, err)
		}
		other := m
		other.MediaID = "0192f0c1-0000-7000-8000-00000000000b"
		if ok, err := h.Cards.InsertLinkCardMedia(ctx, tx, other, at); err != nil || ok {
			t.Errorf("InsertLinkCardMedia of the same URL = %v, %v, want not inserted", ok, err)
		}
		if got, err := h.Cards.LinkCardMediaByURL(ctx, tx, m.RemoteURL); err != nil || got != m {
			t.Errorf("LinkCardMediaByURL = %+v, %v", got, err)
		}
		if _, err := h.Cards.LinkCardMediaByURL(ctx, tx, "https://news.example/none.png"); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("LinkCardMediaByURL of an unknown URL: %v, want ErrNotFound", err)
		}
		return nil
	})
	if got, err := h.Cards.FindLinkCardMedia(ctx, m.MediaID); err != nil || got != m {
		t.Errorf("FindLinkCardMedia = %+v, %v", got, err)
	}
	if got, err := h.Cards.FindLinkCardMedias(ctx, []string{m.MediaID, testkit.NamedID("none")}); err != nil || len(got) != 1 || got[m.MediaID] != m {
		t.Errorf("FindLinkCardMedias = %+v, %v; want the media alone, as FindLinkCardMedia", got, err)
	}
	f, err := h.Media.FindMediaFile(ctx, m.MediaID)
	if err != nil || f.Origin != content.MediaOriginRemote || f.OwnerAccountID != "" || !f.CreatedAt.Equal(at) {
		t.Errorf("media file = %+v, %v, want a remote file without an owner", f, err)
	}
	if _, err := h.Media.FindRemoteMedia(ctx, m.MediaID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindRemoteMedia of an image of link cards: %v, want ErrNotFound", err)
	}
}

func linkCardCleanup(t *testing.T, h LinkCardHarness) {
	ctx := t.Context()
	old := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	cutoff := old.Add(content.LinkCardRefetchAfter)
	card := func(u string, at time.Time, image string) content.LinkCard {
		return content.LinkCard{URL: u, CardURL: u, Title: "T", SiteName: "s", ImageMediaID: image, FetchedAt: at}
	}
	img := func(id, u string) content.LinkCardMedia {
		return content.LinkCardMedia{MediaID: id, RemoteURL: u, ContentType: "image/png", Width: 1, Height: 1, Blurhash: "x"}
	}
	const used, orphan = "0192f0c1-0000-7000-8000-000000000011", "0192f0c1-0000-7000-8000-000000000012"
	linkCardTx(t, h, func(ctx context.Context, tx uow.Tx) error {
		for _, c := range []content.LinkCard{
			card("https://a.example/", old, used),  // referenced: kept
			card("https://b.example/", old, ""),    // unreferenced and old: deleted
			card("https://c.example/", cutoff, ""), // unreferenced but fresh: kept
		} {
			if err := h.Cards.UpsertLinkCard(ctx, tx, c); err != nil {
				return err
			}
		}
		for _, m := range []content.LinkCardMedia{img(used, "https://a.example/1.png"), img(orphan, "https://a.example/2.png")} {
			if _, err := h.Cards.InsertLinkCardMedia(ctx, tx, m, old); err != nil {
				return err
			}
		}
		return h.Cards.UpsertStatusLinkCard(ctx, tx, content.StatusLinkCard{StatusID: testkit.NamedID("s1"), URL: "https://a.example/"})
	})
	cards, err := h.Cards.ListUnreferencedLinkCards(ctx, cutoff, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].URL != "https://b.example/" {
		t.Fatalf("ListUnreferencedLinkCards = %+v, want the old unreferenced card", cards)
	}
	linkCardTx(t, h, func(ctx context.Context, tx uow.Tx) error {
		// A card fetched again since it was listed is kept.
		if ok, err := h.Cards.DeleteUnreferencedLinkCard(ctx, tx, "https://b.example/", old.Add(time.Second)); err != nil || ok {
			t.Errorf("DeleteUnreferencedLinkCard with another fetch time = %v, %v", ok, err)
		}
		// A card that a status refers to is kept.
		if ok, err := h.Cards.DeleteUnreferencedLinkCard(ctx, tx, "https://a.example/", old); err != nil || ok {
			t.Errorf("DeleteUnreferencedLinkCard of a referenced card = %v, %v", ok, err)
		}
		if ok, err := h.Cards.DeleteUnreferencedLinkCard(ctx, tx, "https://b.example/", old); err != nil || !ok {
			t.Errorf("DeleteUnreferencedLinkCard = %v, %v", ok, err)
		}
		return nil
	})
	if _, err := h.Cards.FindLinkCard(ctx, "https://b.example/"); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("deleted card: %v", err)
	}
	ids, err := h.Cards.ListUnreferencedLinkCardMedia(ctx, 10)
	if err != nil || !slices.Equal(ids, []string{orphan}) {
		t.Fatalf("ListUnreferencedLinkCardMedia = %v, %v, want [%s]", ids, err, orphan)
	}
	linkCardTx(t, h, func(ctx context.Context, tx uow.Tx) error {
		if ok, err := h.Cards.DeleteUnreferencedLinkCardMedia(ctx, tx, used); err != nil || ok {
			t.Errorf("DeleteUnreferencedLinkCardMedia of a used image = %v, %v", ok, err)
		}
		if ok, err := h.Cards.DeleteUnreferencedLinkCardMedia(ctx, tx, orphan); err != nil || !ok {
			t.Errorf("DeleteUnreferencedLinkCardMedia = %v, %v", ok, err)
		}
		return nil
	})
	if _, err := h.Cards.FindLinkCardMedia(ctx, orphan); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("deleted image: %v", err)
	}
	if _, err := h.Media.FindMediaFile(ctx, orphan); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("media file of the deleted image: %v", err)
	}
	if _, err := h.Media.FindMediaFile(ctx, used); err != nil {
		t.Errorf("media file of the used image: %v", err)
	}
	// DeleteLinkCard deletes a card that a status refers to, and frees its image for the cleanup.
	linkCardTx(t, h, func(ctx context.Context, tx uow.Tx) error {
		if err := h.Cards.DeleteLinkCard(ctx, tx, "https://a.example/"); err != nil {
			t.Errorf("DeleteLinkCard: %v", err)
		}
		if err := h.Cards.DeleteLinkCard(ctx, tx, "https://none.example/"); err != nil {
			t.Errorf("DeleteLinkCard of a URL without a card: %v", err)
		}
		return nil
	})
	if _, err := h.Cards.FindLinkCard(ctx, "https://a.example/"); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("card after DeleteLinkCard: %v", err)
	}
	ids, err = h.Cards.ListUnreferencedLinkCardMedia(ctx, 10)
	if err != nil || !slices.Equal(ids, []string{used}) {
		t.Errorf("ListUnreferencedLinkCardMedia after DeleteLinkCard = %v, %v, want [%s]", ids, err, used)
	}
}
