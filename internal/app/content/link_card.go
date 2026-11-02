// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"mime"
	"net/http"
	"slices"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// Jobs, schedule, and bounds of the link cards (ADR-0062).
const (
	// JobFetchLinkCard fetches the card of the URL that a status links; payload
	// fetchLinkCardPayload.
	JobFetchLinkCard = "content.fetch_link_card"
	// JobCleanupLinkCards deletes the cards that no status refers to and the images that no card
	// uses (FR-803); no payload.
	JobCleanupLinkCards = "content.cleanup_link_cards"
	// LinkCardCleanupInterval is the period of JobCleanupLinkCards.
	LinkCardCleanupInterval = 24 * time.Hour
	// LinkCardFailureTTL is how long a failed fetch of a URL is remembered, so that the URL is
	// not fetched again in the meantime.
	LinkCardFailureTTL = 24 * time.Hour
	// LinkCardFailureNamespace is the KeyValueStore namespace of that memory, keyed by the
	// SHA-256 of the URL.
	LinkCardFailureNamespace = "content.link_card_failure"
	// LinkCardLockTTL bounds the lock that keeps two jobs from fetching one URL at once.
	LinkCardLockTTL = 60 * time.Second
	// LinkCardLockNamespace is the KeyValueStore namespace of that lock, keyed as the memory.
	LinkCardLockNamespace = "content.link_card_lock"
	// MaxRemoteLinkCardDelay is the most a remote status's fetch is delayed, so that the
	// servers that receive one status do not all fetch its URL at once.
	MaxRemoteLinkCardDelay = 60 * time.Second
	// LinkCardCreatorTimeout bounds resolving and verifying the fediverse:creator of a page.
	LinkCardCreatorTimeout = 10 * time.Second
	// linkCardCleanupBatch is the number of rows one step of the cleanup reads.
	linkCardCleanupBatch = 200
)

// LinkCardRepository is the persistence port of the link cards (link_cards, link_card_media,
// status_link_cards; data-model §4.4). The database adapters implement it and pass
// testkit.LinkCardRepositoryContract. Methods that take a uow.Tx run inside that transaction;
// lookups that find nothing return an error wrapping ErrNotFound.
type LinkCardRepository interface {
	// UpsertStatusLinkCard sets the URL whose card a status shows.
	UpsertStatusLinkCard(ctx context.Context, tx uow.Tx, l content.StatusLinkCard) error
	// DeleteStatusLinkCard removes the card of a status; a status without one is not an error.
	DeleteStatusLinkCard(ctx context.Context, tx uow.Tx, statusID string) error
	// StatusLinkCardURL returns the URL whose card a status shows.
	StatusLinkCardURL(ctx context.Context, statusID string) (string, error)
	// FindLinkCard returns the card of a normalized URL.
	FindLinkCard(ctx context.Context, url string) (content.LinkCard, error)
	// FindLinkCardTx reads as FindLinkCard does, in tx.
	FindLinkCardTx(ctx context.Context, tx uow.Tx, url string) (content.LinkCard, error)
	// UpsertLinkCard records the card of its URL, replacing the one fetched before.
	UpsertLinkCard(ctx context.Context, tx uow.Tx, c content.LinkCard) error
	// ListUnreferencedLinkCards returns up to limit cards fetched before fetchedBefore that no
	// status refers to, ordered by URL.
	ListUnreferencedLinkCards(ctx context.Context, fetchedBefore time.Time, limit int) ([]content.LinkCard, error)
	// DeleteUnreferencedLinkCard deletes the card of url if it was fetched at fetchedAt and no
	// status refers to it, and reports whether it deleted it.
	DeleteUnreferencedLinkCard(ctx context.Context, tx uow.Tx, url string, fetchedAt time.Time) (bool, error)
	// InsertLinkCardMedia inserts an image of link cards with its media file (origin remote,
	// no owner). It reports false, and inserts nothing, when an image with the same URL exists.
	InsertLinkCardMedia(ctx context.Context, tx uow.Tx, m content.LinkCardMedia, createdAt time.Time) (bool, error)
	// LinkCardMediaByURL returns the image of link cards with a remote URL.
	LinkCardMediaByURL(ctx context.Context, tx uow.Tx, remoteURL string) (content.LinkCardMedia, error)
	// FindLinkCardMedia returns the image of link cards with a media ID.
	FindLinkCardMedia(ctx context.Context, mediaID string) (content.LinkCardMedia, error)
	// ListUnreferencedLinkCardMedia returns the IDs of up to limit images that no card uses,
	// ordered by ID.
	ListUnreferencedLinkCardMedia(ctx context.Context, limit int) ([]string, error)
	// DeleteUnreferencedLinkCardMedia deletes an image with its media file if no card uses it,
	// and reports whether it deleted it.
	DeleteUnreferencedLinkCardMedia(ctx context.Context, tx uow.Tx, mediaID string) (bool, error)
}

// LinkCardDomains reads what bears on link cards by domain (ADR-0062 §7): the domain actions and
// the rules of the crawler. The platform adapts *moderation.FederationPolicy and the rules of the
// instance module, canonicalizing the hosts.
type LinkCardDomains interface {
	// FederationSuspended reports whether a host, or one of its parents, is suspended.
	FederationSuspended(ctx context.Context, host string) (bool, error)
	// RejectsMedia reports whether the media of a host, or of one of its parents, are rejected.
	RejectsMedia(ctx context.Context, host string) (bool, error)
	// FetchStopped reports whether a rule of the crawler on a host, or on one of its parents,
	// stops fetching from it.
	FetchStopped(ctx context.Context, host string) (bool, error)
	// ImagesStopped reports whether a rule of the crawler on a host, or on one of its parents,
	// stops fetching images from it: either mode of a rule does.
	ImagesStopped(ctx context.Context, host string) (bool, error)
}

// LinkCardCreators verifies the fediverse:creator of a page (ADR-0062 §5). The federation
// module's RemoteActors implements it.
type LinkCardCreators interface {
	// VerifyCreator resolves handle ("@user@domain") with WebFinger and returns its account
	// when the actor's attributionDomains hold host or one of its parents; "" with a nil error
	// when the handle cannot be resolved or does not verify. It does network I/O.
	VerifyCreator(ctx context.Context, handle, host string) (string, error)
}

// LinkCardAccounts reads the accounts of verified creators. *appaccount.Service implements it.
type LinkCardAccounts interface {
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
}

// LinkCardsDeps are the ports and settings LinkCards uses.
type LinkCardsDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository LinkCardRepository
	// Statuses reads the heads of statuses in the projectors, and the restrictions of their
	// authors.
	Statuses Repository
	// Outbox enqueues the fetches inside the transactions of the statuses.
	Outbox queue.Outbox
	// HTTP fetches the pages and images through the SSRF guard (ADR-0019 §1).
	HTTP kernel.HTTPClient
	KV   kernel.KeyValueStore
	// Processor checks the images and reads their size and blurhash.
	Processor media.Processor
	// Proxy signs the URLs of the images, which clients load through it.
	Proxy    *MediaProxy
	Domains  LinkCardDomains
	Accounts LinkCardAccounts
	// Creators verifies the fediverse:creator of pages; nil records none.
	Creators LinkCardCreators
	// Enabled returns the link_cards_enabled instance setting; nil means enabled.
	Enabled func(ctx context.Context) (bool, error)
	// Language returns the Accept-Language of the fetches: the instance's default locale.
	Language func(ctx context.Context) (string, error)
	// LocalHosts are the hosts of this instance, whose URLs get no card.
	LocalHosts []string
	// RemoteDelay returns the delay of a remote status's fetch; nil picks one at random up to
	// MaxRemoteLinkCardDelay.
	RemoteDelay func() time.Duration
	Clock       kernel.Clock
	IDs         kernel.IDGenerator
	Logger      *slog.Logger
}

// LinkCards fetches, keeps, and presents the cards of the links in statuses (ADR-0062): a
// projector chooses the URL of each status from its current revision and enqueues the fetch;
// the job reads the head of the page through the SSRF-guarded HTTPClient and records the card,
// shared by every status that links the URL, with its image checked once; the reader presents
// the card with its image through the media proxy.
type LinkCards struct {
	d LinkCardsDeps
}

// NewLinkCards returns LinkCards on d.
func NewLinkCards(d LinkCardsDeps) *LinkCards {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.RemoteDelay == nil {
		d.RemoteDelay = func() time.Duration {
			return rand.N(MaxRemoteLinkCardDelay + time.Second) //nolint:gosec // spreading fetches needs no cryptographic randomness
		}
	}
	return &LinkCards{d: d}
}

// fetchLinkCardPayload is the payload of JobFetchLinkCard.
type fetchLinkCardPayload struct {
	StatusID string `json:"status_id"`
}

// RegisterProjectors registers the projector of status_link_cards (ADR-0062 §1, §6): the URL
// chosen from the current revision of each status that is not deleted. When the card of the URL
// is missing or stale, it enqueues JobFetchLinkCard, delayed for a remote status. Register it
// after RegisterProjectors, which moves the heads.
func (l *LinkCards) RegisterProjectors(r *uow.Projectors) {
	r.Register(EventStatusCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(StatusCreated)
		return l.project(ctx, tx, ev.Status, ev.Revision)
	})
	r.Register(EventStatusRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(StatusRevised)
		head, err := l.d.Statuses.Head(ctx, tx, ev.Revision.StatusID)
		switch {
		case errors.Is(err, ErrNotFound):
			return nil // deleted
		case err != nil:
			return err
		case head.RevisionID != ev.Revision.ID:
			return nil // an older revision that arrived late
		}
		return l.project(ctx, tx, ev.Status, ev.Revision)
	})
	r.Register(EventStatusDeleted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return l.d.Repository.DeleteStatusLinkCard(ctx, tx, e.(StatusDeleted).Status.ID)
	})
}

func (l *LinkCards) project(ctx context.Context, tx uow.Tx, st content.Status, rev content.Revision) error {
	u := LinkCardURL(rev, l.d.LocalHosts)
	if u == "" {
		return l.d.Repository.DeleteStatusLinkCard(ctx, tx, st.ID)
	}
	if err := l.d.Repository.UpsertStatusLinkCard(ctx, tx, content.StatusLinkCard{StatusID: st.ID, URL: u}); err != nil {
		return err
	}
	switch c, err := l.d.Repository.FindLinkCardTx(ctx, tx, u); {
	case err == nil && !c.Stale(l.d.Clock.Now()):
		return nil
	case err != nil && !errors.Is(err, ErrNotFound):
		return err
	}
	payload, err := json.Marshal(fetchLinkCardPayload{StatusID: st.ID})
	if err != nil {
		return err
	}
	job := queue.NewJob{Type: JobFetchLinkCard, Payload: payload, IdempotencyKey: "link_card:" + st.ID}
	if !st.IsLocal {
		job.RunAt = l.d.Clock.Now().Add(l.d.RemoteDelay())
	}
	return l.d.Outbox.EnqueueTx(ctx, tx, job)
}

// RegisterJobs registers the fetch and the cleanup of the link cards on reg, and the daily
// schedule of the cleanup on sched.
func (l *LinkCards) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobFetchLinkCard, func(ctx context.Context, j queue.Job) queue.Result {
		var p fetchLinkCardPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.StatusID == "" {
			return queue.Fail(fmt.Errorf("link card payload: %q", j.Payload))
		}
		retry, err := l.Fetch(ctx, p.StatusID)
		switch {
		case err != nil:
			return queue.Retry(0, err)
		case retry:
			return queue.Retry(LinkCardLockTTL, nil)
		}
		return queue.Done()
	})
	reg.Register(JobCleanupLinkCards, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := l.Cleanup(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	sched.Add(queue.Schedule{Name: JobCleanupLinkCards, Every: LinkCardCleanupInterval, Job: queue.NewJob{Type: JobCleanupLinkCards}})
}

func (l *LinkCards) enabled(ctx context.Context) (bool, error) {
	if l.d.Enabled == nil {
		return true, nil
	}
	return l.d.Enabled(ctx)
}

// linkCardKey is the key of a URL in the failure memory and the lock.
func linkCardKey(u string) string {
	sum := sha256.Sum256([]byte(u))
	return hex.EncodeToString(sum[:])
}

// errLinkCardUnusable reports a page that makes no card or cannot be fetched; the URL is
// remembered as failed.
var errLinkCardUnusable = errors.New("content: no link card")

// errLinkCardRefused reports a redirect to a host that nothing is fetched from. It is not
// remembered, so that the URL can be fetched as soon as the rule is gone.
var errLinkCardRefused = errors.New("content: link card host refused")

// fetchRefused reports whether nothing is fetched from a host and its cards are hidden: it is
// suspended, or a rule of the crawler stops fetching from it.
func (l *LinkCards) fetchRefused(ctx context.Context, host string) (bool, error) {
	if suspended, err := l.d.Domains.FederationSuspended(ctx, host); err != nil || suspended {
		return suspended, err
	}
	return l.d.Domains.FetchStopped(ctx, host)
}

// imageRefused reports whether no image is fetched from a host and the cards that touch it have
// no image: its media are rejected, or a rule of the crawler stops fetching images from it.
func (l *LinkCards) imageRefused(ctx context.Context, host string) (bool, error) {
	if rejected, err := l.d.Domains.RejectsMedia(ctx, host); err != nil || rejected {
		return rejected, err
	}
	return l.d.Domains.ImagesStopped(ctx, host)
}

// checkRedirect returns the HTTPRequest.CheckRedirect that ends a fetch at a redirect to a host
// that refused reports, with an error wrapping errLinkCardRefused.
func (l *LinkCards) checkRedirect(refused func(ctx context.Context, host string) (bool, error)) func(ctx context.Context, url string) error {
	return func(ctx context.Context, url string) error {
		switch stop, err := refused(ctx, content.LinkHost(url)); {
		case err != nil:
			return err
		case stop:
			return fmt.Errorf("%w: %s", errLinkCardRefused, url)
		}
		return nil
	}
}

// Fetch fetches the card of the URL that a status links, unless the card is fresh, the URL
// failed within LinkCardFailureTTL, its host is suspended or a rule of the crawler stops fetching
// from it, or link cards are disabled. A fetch skipped for its host writes no failure memory, so
// that the next status that links the URL gets a card as soon as the action or the rule is gone.
// It reports retry when another job is fetching the URL. Failures of the page are remembered and
// are not errors; an error is a failure of the stores.
func (l *LinkCards) Fetch(ctx context.Context, statusID string) (retry bool, err error) {
	if on, err := l.enabled(ctx); err != nil || !on {
		return false, err
	}
	u, err := l.d.Repository.StatusLinkCardURL(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	switch c, err := l.d.Repository.FindLinkCard(ctx, u); {
	case err == nil && !c.Stale(l.d.Clock.Now()):
		return false, nil
	case err != nil && !errors.Is(err, ErrNotFound):
		return false, err
	}
	key := linkCardKey(u)
	switch _, err := l.d.KV.Get(ctx, LinkCardFailureNamespace, key); {
	case err == nil:
		return false, nil
	case !errors.Is(err, kernel.ErrKeyNotFound):
		return false, fmt.Errorf("read link card failure memory: %w", err)
	}
	if refused, err := l.fetchRefused(ctx, content.LinkHost(u)); err != nil || refused {
		return false, err
	}
	n, err := l.d.KV.IncrBy(ctx, LinkCardLockNamespace, key, 1, LinkCardLockTTL)
	if err != nil {
		return false, fmt.Errorf("lock link card: %w", err)
	}
	if n > 1 {
		return true, nil
	}
	defer func() {
		if derr := l.d.KV.Delete(context.WithoutCancel(ctx), LinkCardLockNamespace, key); derr != nil {
			l.d.Logger.WarnContext(ctx, "unlock link card", slog.String("url", u), slog.Any("error", derr))
		}
	}()
	card, err := l.fetchCard(ctx, u)
	if errors.Is(err, errLinkCardRefused) {
		l.d.Logger.DebugContext(ctx, "link card redirected to a refused host", slog.String("url", u), slog.Any("error", err))
		return false, nil
	}
	if errors.Is(err, errLinkCardUnusable) {
		l.d.Logger.DebugContext(ctx, "no link card", slog.String("url", u), slog.Any("error", err))
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if err := l.d.KV.Set(ctx, LinkCardFailureNamespace, key, []byte("1"), LinkCardFailureTTL); err != nil {
			return false, fmt.Errorf("write link card failure memory: %w", err)
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return false, l.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return l.d.Repository.UpsertLinkCard(ctx, tx, card) })
}

// fetchCard fetches the page of u and its image, and returns the card. A page that makes no
// card fails with errLinkCardUnusable.
func (l *LinkCards) fetchCard(ctx context.Context, u string) (content.LinkCard, error) {
	header := http.Header{"Accept": {"text/html, application/xhtml+xml"}}
	if l.d.Language != nil {
		if lang, err := l.d.Language(ctx); err == nil && lang != "" {
			header.Set("Accept-Language", lang)
		}
	}
	resp, err := l.d.HTTP.Do(ctx, &kernel.HTTPRequest{
		URL: u, Purpose: kernel.HTTPPurposeHTML, Header: header, CheckRedirect: l.checkRedirect(l.fetchRefused),
	})
	if errors.Is(err, errLinkCardRefused) {
		return content.LinkCard{}, err
	}
	if err != nil {
		return content.LinkCard{}, fmt.Errorf("%w: fetch %s: %w", errLinkCardUnusable, u, err)
	}
	defer func() { _ = resp.Body.Close() }()
	ct := resp.Header.Get("Content-Type")
	mt, _, _ := mime.ParseMediaType(ct)
	if resp.StatusCode != http.StatusOK || (mt != "text/html" && mt != "application/xhtml+xml") {
		return content.LinkCard{}, fmt.Errorf("%w: %s answered %d %q", errLinkCardUnusable, u, resp.StatusCode, ct)
	}
	page, err := ParseLinkCardPage(resp.Body, ct, resp.URL)
	if err != nil {
		return content.LinkCard{}, fmt.Errorf("%w: %w", errLinkCardUnusable, err)
	}
	card := content.LinkCard{
		URL: u, CardURL: page.URL, Title: page.Title, Description: page.Description, SiteName: page.SiteName,
		AuthorName: page.AuthorName, PublishedAt: page.PublishedAt, Language: page.Language, FetchedAt: l.d.Clock.Now(),
	}
	if page.ImageURL != "" {
		hosts := uniqueHosts(u, resp.URL, card.CardURL, page.ImageURL)
		if card.ImageMediaID, err = l.image(ctx, hosts, page.ImageURL, page.ImageAlt); err != nil {
			return content.LinkCard{}, err
		}
	}
	if page.Creator != "" && l.d.Creators != nil {
		cctx, cancel := context.WithTimeout(ctx, LinkCardCreatorTimeout)
		id, err := l.d.Creators.VerifyCreator(cctx, page.Creator, content.LinkHost(card.CardURL))
		cancel()
		if err != nil {
			l.d.Logger.WarnContext(ctx, "verify link card creator", slog.String("url", u), slog.Any("error", err))
		}
		card.CreatorAccountID = id
	}
	return card, nil
}

// image returns the media file of the image at imageURL of a card that touches hosts: the one
// recorded for the URL before, or the image fetched once, checked, and recorded with its size
// and blurhash. It returns "" when no image is fetched from one of the hosts or the image cannot
// be used, so that the card has no image (ADR-0062 §3, §7).
func (l *LinkCards) image(ctx context.Context, hosts []string, imageURL, alt string) (string, error) {
	for _, host := range hosts {
		if refused, err := l.imageRefused(ctx, host); err != nil || refused {
			return "", err
		}
	}
	var existing content.LinkCardMedia
	err := l.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		existing, err = l.d.Repository.LinkCardMediaByURL(ctx, tx, imageURL)
		return err
	})
	switch {
	case err == nil:
		return existing.MediaID, nil
	case !errors.Is(err, ErrNotFound):
		return "", err
	}
	m, ok := l.probeImage(ctx, imageURL)
	if !ok {
		return "", nil
	}
	m.MediaID, m.Description = l.d.IDs.NewID(), alt
	err = l.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		inserted, err := l.d.Repository.InsertLinkCardMedia(ctx, tx, m, l.d.Clock.Now())
		if err != nil || inserted {
			return err
		}
		// Another job recorded the same URL meanwhile: use its file.
		existing, err = l.d.Repository.LinkCardMediaByURL(ctx, tx, imageURL)
		m.MediaID = existing.MediaID
		return err
	})
	if err != nil {
		return "", err
	}
	return m.MediaID, nil
}

// probeImage fetches an image, checks its format against the allow list of ADR-0019 §8 (SVG
// and anything else is refused) and its size against the limits, and reads its size and
// blurhash. It reports false for an image that cannot be used.
func (l *LinkCards) probeImage(ctx context.Context, imageURL string) (content.LinkCardMedia, bool) {
	fail := func(why string, err error) (content.LinkCardMedia, bool) {
		l.d.Logger.DebugContext(ctx, "link card image not used", slog.String("url", imageURL), slog.String("why", why), slog.Any("error", err))
		return content.LinkCardMedia{}, false
	}
	resp, err := l.d.HTTP.Do(ctx, &kernel.HTTPRequest{
		URL: imageURL, Purpose: kernel.HTTPPurposeImage, Header: http.Header{"Accept": {"image/*"}},
		CheckRedirect: l.checkRedirect(l.imageRefused),
	})
	if err != nil {
		return fail("fetch", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return fail(fmt.Sprintf("status %d", resp.StatusCode), nil)
	}
	data, err := resp.ReadAll()
	if err != nil {
		return fail("read", err)
	}
	contentType, kind := content.SniffRemoteMedia(data[:min(len(data), content.SniffHeadLength)])
	if kind != content.MediaTypeImage || media.Sniff(data) == "" {
		return fail("format "+contentType, nil)
	}
	out, err := l.d.Processor.Process(ctx, bytes.Clone(data), media.Limits{}, media.Sizes{}, media.Request{Purpose: media.PurposeAttachment})
	if err != nil {
		return fail("process", err)
	}
	return content.LinkCardMedia{
		RemoteURL: imageURL, ContentType: contentType, Width: out.Width, Height: out.Height, Blurhash: out.Blurhash,
	}, true
}

// CleanupResult counts what one Cleanup deleted.
type CleanupResult struct {
	Cards, Images int
}

// Cleanup deletes the cards that no status refers to and that were fetched
// content.LinkCardRefetchAfter ago, then the images that no card uses (ADR-0062 §6, FR-803),
// in short transactions.
func (l *LinkCards) Cleanup(ctx context.Context) (CleanupResult, error) {
	var res CleanupResult
	before := l.d.Clock.Now().Add(-content.LinkCardRefetchAfter)
	for {
		cards, err := l.d.Repository.ListUnreferencedLinkCards(ctx, before, linkCardCleanupBatch)
		if err != nil {
			return res, err
		}
		for _, c := range cards {
			var deleted bool
			if err := l.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				var err error
				deleted, err = l.d.Repository.DeleteUnreferencedLinkCard(ctx, tx, c.URL, c.FetchedAt)
				return err
			}); err != nil {
				return res, err
			}
			if deleted {
				res.Cards++
			}
		}
		if len(cards) < linkCardCleanupBatch {
			break
		}
	}
	for {
		ids, err := l.d.Repository.ListUnreferencedLinkCardMedia(ctx, linkCardCleanupBatch)
		if err != nil {
			return res, err
		}
		for _, id := range ids {
			var deleted bool
			if err := l.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				var err error
				deleted, err = l.d.Repository.DeleteUnreferencedLinkCardMedia(ctx, tx, id)
				return err
			}); err != nil {
				return res, err
			}
			if deleted {
				res.Images++
			}
		}
		if len(ids) < linkCardCleanupBatch {
			break
		}
	}
	return res, nil
}

// LinkCardView is the card of a status as clients show it (ADR-0062 §8).
type LinkCardView struct {
	Card content.LinkCard
	// Image is the image through the media proxy, nil for none.
	Image *LinkCardImage
	// Author is the verified creator's account, nil for none.
	Author *appaccount.Actor
}

// LinkCardImage is the image of a card: the media proxy's small variant.
type LinkCardImage struct {
	URL           string
	Width, Height int
	Blurhash      string
	Description   string
}

// View returns the card that rev, the current revision of st, shows, or nil for none: link
// cards are disabled, the revision has attachments or a poll, the card is not fetched yet, or
// nothing is fetched from the card's host. The image is left out when no image is fetched from
// the card's or the image's host. These apply when reading, so a change of a domain action or a
// rule of the crawler shows on the cards already kept at once, and is undone when it goes.
func (l *LinkCards) View(ctx context.Context, st content.Status, rev content.Revision) (*LinkCardView, error) {
	if len(rev.Attachments) > 0 || rev.Poll != nil {
		return nil, nil
	}
	if on, err := l.enabled(ctx); err != nil || !on {
		return nil, err
	}
	u, err := l.d.Repository.StatusLinkCardURL(ctx, st.ID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c, err := l.d.Repository.FindLinkCard(ctx, u)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, host := range uniqueHosts(c.URL, c.CardURL) {
		if refused, err := l.fetchRefused(ctx, host); err != nil || refused {
			return nil, err
		}
	}
	out := &LinkCardView{Card: c}
	if c.ImageMediaID != "" && l.d.Proxy != nil {
		img, err := l.cardImage(ctx, c)
		if err != nil {
			return nil, err
		}
		out.Image = img
	}
	if c.CreatorAccountID != "" && l.d.Accounts != nil {
		a, err := l.d.Accounts.FindByID(ctx, c.CreatorAccountID)
		switch {
		case err == nil:
			out.Author = &a
		case !errors.Is(err, appaccount.ErrNotFound):
			return nil, err
		}
	}
	return out, nil
}

// cardImage returns the image of c through the media proxy, nil when it is missing or no image
// is fetched from one of its hosts.
func (l *LinkCards) cardImage(ctx context.Context, c content.LinkCard) (*LinkCardImage, error) {
	m, err := l.d.Repository.FindLinkCardMedia(ctx, c.ImageMediaID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, host := range uniqueHosts(c.URL, c.CardURL, m.RemoteURL) {
		if refused, err := l.imageRefused(ctx, host); err != nil || refused {
			return nil, err
		}
	}
	return &LinkCardImage{
		URL: l.d.Proxy.URL(m.MediaID, ProxySmall), Width: m.Width, Height: m.Height, Blurhash: m.Blurhash,
		Description: m.Description,
	}, nil
}

// uniqueHosts returns the distinct hosts of urls.
func uniqueHosts(urls ...string) []string {
	var out []string
	for _, u := range urls {
		if h := content.LinkHost(u); h != "" && !slices.Contains(out, h) {
			out = append(out, h)
		}
	}
	return out
}
