// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// RemoteMediaInput is a remote media file as a received document declares it.
type RemoteMediaInput struct {
	OwnerAccountID      string
	RemoteURL           string
	DeclaredContentType string
	DeclaredMediaType   content.MediaType
	// Purpose is what the file is recorded for; "" is content.PurposeAttachment.
	Purpose       content.MediaPurpose
	Width, Height int
	Blurhash      string
	Focus         *content.MediaFocus
	Description   string
}

// RecordRemoteMedia records a remote media file inside tx: the media file (origin remote) and
// its remote subtype with the declared values (docs/architecture/federation.md §9). The file
// is not fetched. Recording the same owner and URL again returns the file recorded first.
func (s *MediaService) RecordRemoteMedia(ctx context.Context, tx uow.Tx, in RemoteMediaInput) (content.RemoteMedia, error) {
	existing, err := s.d.Repository.RemoteMediaByURL(ctx, tx, in.OwnerAccountID, in.RemoteURL)
	switch {
	case err == nil:
		return existing, nil
	case !errors.Is(err, ErrNotFound):
		return content.RemoteMedia{}, err
	}
	kind := in.DeclaredMediaType
	if kind == "" {
		kind = content.MediaTypeUnknown
	}
	purpose := in.Purpose
	if !purpose.Valid() {
		purpose = content.PurposeAttachment
	}
	m := content.RemoteMedia{
		MediaID: s.d.IDs.NewID(), OwnerAccountID: in.OwnerAccountID, RemoteURL: in.RemoteURL,
		DeclaredContentType: in.DeclaredContentType, DeclaredMediaType: kind, Purpose: purpose,
		Width: in.Width, Height: in.Height, Blurhash: in.Blurhash, Description: in.Description,
	}
	if in.Focus != nil {
		m.Focus = &content.MediaFocus{MediaID: m.MediaID, X: in.Focus.X, Y: in.Focus.Y}
	}
	file := content.MediaFile{ID: m.MediaID, OwnerAccountID: in.OwnerAccountID, Origin: content.MediaOriginRemote, CreatedAt: s.d.Clock.Now()}
	if err := s.d.Repository.InsertMediaFile(ctx, tx, file); err != nil {
		return content.RemoteMedia{}, err
	}
	inserted, err := s.d.Repository.InsertRemoteMedia(ctx, tx, m)
	if err != nil {
		return content.RemoteMedia{}, err
	}
	if !inserted {
		// A concurrent transaction recorded the same URL after the lookup; the retry finds it.
		return content.RemoteMedia{}, fmt.Errorf("remote media %s: %w", in.RemoteURL, uow.ErrRetryable)
	}
	return m, nil
}

// ProxyPathPrefix is the URL path under which the media proxy serves remote media files:
// ProxyPathPrefix + "{media_id}/{variant}?sig=...".
const ProxyPathPrefix = "/media/proxy/"

// ErrProxyUpstream reports a remote server that did not answer the media proxy's fetch with
// the file: a network error, a timeout, or a 5xx status. A later request may succeed.
var ErrProxyUpstream = errors.New("content: the remote server did not return the media file")

// ErrUnsupportedRemoteMedia reports a remote media file whose content is not in a format that
// the media proxy serves for it. The errors wrapping it wrap ErrNotFound too.
var ErrUnsupportedRemoteMedia = errors.New("content: the remote media file is not in an allowed format")

// AccountDomains tells the domain of an account. The account module implements it.
type AccountDomains interface {
	// AccountDomain returns the domain of an account ("" for a local one).
	AccountDomain(ctx context.Context, accountID string) (string, error)
}

// MediaFetchPolicy is the port through which the media proxy reads the domain restrictions on
// fetching media (the media_fetch restriction of a reject_media domain action, data-model §5.2).
// *moderation.FederationPolicy implements it.
type MediaFetchPolicy interface {
	// RejectsMedia reports whether the media of domain, or of a parent domain, are not fetched.
	RejectsMedia(ctx context.Context, domain string) (bool, error)
	// RejectsMediaTx answers as RejectsMedia does, read in tx with what tx has written.
	RejectsMediaTx(ctx context.Context, tx uow.Tx, domain string) (bool, error)
}

// ProxyDeps are the ports a MediaProxy uses.
type ProxyDeps struct {
	Repository MediaRepository
	HTTP       kernel.HTTPClient
	Accounts   AccountDomains
	Policy     MediaFetchPolicy
	// Settings returns the current remote_media_policy and remote_media_cache_ttl instance
	// settings.
	Settings func(ctx context.Context) (ProxySettings, error)
	// UnitOfWork, Blobs, and Clock keep the copies of the remote media cache; without Blobs the
	// proxy never caches, whatever the policy.
	UnitOfWork uow.UnitOfWork
	Blobs      blob.Store
	Clock      kernel.Clock
	// ProfileImages tells the avatars and headers of profiles, which the avatars policy caches
	// with the custom emoji images; nil caches only the custom emoji images under it.
	ProfileImages MediaReferrer
	// Processor makes the small variant (ProxySmall) of remote images, within Sizes (zero
	// fields take media.DefaultSizes); without it the proxy serves no small variant.
	Processor media.Processor
	Sizes     media.Sizes
	// MaxCacheSize bounds the total size of the cache in bytes; zero means
	// DefaultRemoteMediaCacheMaxSize. The cleanup deletes the least recently accessed copies
	// over it.
	MaxCacheSize int64
	// EmojiMaxBytes bounds the images of remote custom emoji (remote_custom_emoji_max_bytes,
	// ADR-0051 §2); zero means content.DefaultRemoteCustomEmojiMaxBytes.
	EmojiMaxBytes int64
	// Keys are the HMAC-SHA256 keys of the proxy URLs' signatures. The first signs; every one
	// verifies, so that URLs signed before a key rotation keep working while the old key is kept.
	Keys [][]byte
	// KeySource, when Keys is empty, returns the one key that signs and verifies. The proxy
	// calls it on first use, and again after a failure until it succeeds (LoadKeys), so that the
	// key can live in a database whose schema is applied after the start.
	KeySource func(ctx context.Context) ([]byte, error)
	// BaseURL is the origin of the proxy URLs ("https://example.com").
	BaseURL string
	// LinkCardMedia reads the images of link cards (ADR-0062 §3), which the proxy serves as the
	// remote media files; nil serves none.
	LinkCardMedia LinkCardMediaReader
	// Federation says whether the federation mode lets the proxy fetch a remote media file
	// (ADR-0076): not at all when it is off, and only the avatars and headers of the remote
	// followers of local accounts when it is outbound. nil lets every file through.
	Federation MediaFederation
	// Logger logs unexpected errors; nil means slog.Default().
	Logger *slog.Logger
}

// MediaFederation is the port through which the media proxy asks the federation mode
// (ADR-0076) whether it fetches a remote media file; federation.MediaGate implements it.
type MediaFederation interface {
	// ProxiesMedia reports whether a remote media file of the account ownerAccountID ("" for none,
	// as the images of link cards) may be fetched now; profileImage tells an avatar or a header.
	ProxiesMedia(ctx context.Context, ownerAccountID string, profileImage bool) (bool, error)
}

// LinkCardMediaReader reads the images of link cards. LinkCardRepository implements it.
type LinkCardMediaReader interface {
	FindLinkCardMedia(ctx context.Context, mediaID string) (content.LinkCardMedia, error)
}

// MediaProxy relays remote media files from the instance's origin (ADR-0005,
// docs/architecture/federation.md §9), so that clients never contact remote servers: their IP
// addresses stay private and the web client's CSP needs no remote origins. URLs are signed
// with an HMAC of the media ID and the variant, so the proxy relays only the files that the
// instance has recorded, never an arbitrary URL. The file is fetched through the SSRF-guarded
// HTTPClient, its format is detected from its content (ADR-0019 §8: SVG and anything outside
// the allow list is refused), and its size is bounded: 16 MiB for images and 100 MiB for video
// and audio (ADR-0019 §1). The image of a custom emoji is further bounded by
// remote_custom_emoji_max_bytes and limited to PNG, GIF, and WebP (ADR-0051 §2). A CDN in front of the instance caches the responses for
// remote_media_cache_ttl. Under the remote_media_policy none the instance keeps no copy; under
// avatars and all it also keeps one in blob storage for remote_media_cache_ttl
// (remote_media_cache.go).
type MediaProxy struct {
	d ProxyDeps

	keyMu sync.Mutex               // serializes the loads from KeySource
	keys  atomic.Pointer[[][]byte] // nil until loaded
}

// NewMediaProxy returns a MediaProxy on d. It needs at least one key or a KeySource.
func NewMediaProxy(d ProxyDeps) (*MediaProxy, error) {
	if len(d.Keys) == 0 && d.KeySource == nil {
		return nil, errors.New("content: the media proxy needs a signing key")
	}
	if err := checkProxyKeys(d.Keys); err != nil {
		return nil, err
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.MaxCacheSize <= 0 {
		d.MaxCacheSize = DefaultRemoteMediaCacheMaxSize
	}
	if d.EmojiMaxBytes <= 0 {
		d.EmojiMaxBytes = content.DefaultRemoteCustomEmojiMaxBytes
	}
	d.BaseURL = strings.TrimSuffix(d.BaseURL, "/")
	d.Sizes = d.Sizes.WithDefaults()
	p := &MediaProxy{d: d}
	if len(d.Keys) > 0 {
		p.keys.Store(&d.Keys)
	}
	return p, nil
}

func checkProxyKeys(keys [][]byte) error {
	for _, k := range keys {
		if len(k) < sha256.Size {
			return errors.New("content: a media proxy key must be at least 32 bytes")
		}
	}
	return nil
}

// LoadKeys loads the signing key from ProxyDeps.KeySource unless it is loaded. The server
// calls it at the start, so that the first request does not wait for it.
func (p *MediaProxy) LoadKeys(ctx context.Context) error {
	_, err := p.signingKeys(ctx)
	return err
}

// signingKeys returns the keys, loading them on first use.
func (p *MediaProxy) signingKeys(ctx context.Context) ([][]byte, error) {
	if k := p.keys.Load(); k != nil {
		return *k, nil
	}
	p.keyMu.Lock()
	defer p.keyMu.Unlock()
	if k := p.keys.Load(); k != nil {
		return *k, nil
	}
	key, err := p.d.KeySource(ctx)
	if err != nil {
		return nil, fmt.Errorf("content: load the media proxy key: %w", err)
	}
	keys := [][]byte{key}
	if err := checkProxyKeys(keys); err != nil {
		return nil, err
	}
	p.keys.Store(&keys)
	return keys, nil
}

// ProxyVariant names what the media proxy serves of a remote media file.
type ProxyVariant string

// Proxy variants.
const (
	// ProxyFull is the remote file itself.
	ProxyFull ProxyVariant = "full"
	// ProxySmall is the small still variant of a remote image within the boxes of the purpose
	// it was recorded for (ADR-0054 §7): of an animation, its first frame. Video and audio
	// have none.
	ProxySmall ProxyVariant = "small"
)

// proxyVariants are the variants the proxy serves.
var proxyVariants = map[ProxyVariant]bool{ProxyFull: true, ProxySmall: true}

// URL returns the signed proxy URL of a remote media file's variant.
// Without a key (KeySource fails), the URL has an empty signature, which the proxy refuses.
func (p *MediaProxy) URL(mediaID string, variant ProxyVariant) string {
	sig := ""
	if keys, err := p.signingKeys(context.Background()); err != nil {
		p.d.Logger.Error("media proxy URL without a signature", slog.Any("error", err))
	} else {
		sig = p.sign(keys[0], mediaID, string(variant))
	}
	return p.d.BaseURL + ProxyPathPrefix + url.PathEscape(mediaID) + "/" + url.PathEscape(string(variant)) + "?sig=" + sig
}

func (p *MediaProxy) sign(key []byte, mediaID, variant string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(kernel.AAD("media-proxy", mediaID, variant))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// verify reports whether sig is the signature of mediaID and variant under one of the keys.
func (p *MediaProxy) verify(ctx context.Context, mediaID, variant, sig string) bool {
	keys, err := p.signingKeys(ctx)
	if err != nil {
		p.d.Logger.ErrorContext(ctx, "media proxy request not verified", slog.Any("error", err))
		return false
	}
	ok := false
	for _, k := range keys {
		// Compare under every key, so that the time does not tell which key matched.
		if hmac.Equal([]byte(p.sign(k, mediaID, variant)), []byte(sig)) {
			ok = true
		}
	}
	return ok
}

// ProxiedMedia is a remote media file on its way to the client.
type ProxiedMedia struct {
	// ContentType is the media type detected from the content.
	ContentType string
	// Length is the length of Body, or -1 when the remote server did not tell it.
	Length int64
	// MaxAge is how long the response may be cached.
	MaxAge time.Duration
	// Body reads the file. A read past the size limit fails with an error wrapping
	// kernel.ErrResponseTooLarge; the caller then aborts the response. The caller closes it.
	Body io.ReadCloser
}

// Open fetches the variant of a remote media file for a proxy request. It fails with an error
// wrapping ErrNotFound when the signature does not match, when the file is not a recorded
// remote media file, when its domain's media are rejected, when the HTTPClient refuses its
// destination, when the remote server answers with a 4xx status, and when the content is not
// in an allowed format or is larger than the limit; it fails with ErrProxyUpstream when the
// remote server cannot be reached or answers with a 5xx status.
func (p *MediaProxy) Open(ctx context.Context, mediaID, variant, sig string) (*ProxiedMedia, error) {
	notFound := func(why string) error {
		return fmt.Errorf("proxy media %s/%s: %s: %w", mediaID, variant, why, ErrNotFound)
	}
	if !p.verify(ctx, mediaID, variant, sig) {
		return nil, notFound("bad signature")
	}
	if !proxyVariants[ProxyVariant(variant)] || ProxyVariant(variant) == ProxySmall && p.d.Processor == nil {
		return nil, notFound("unknown variant")
	}
	return p.open(ctx, mediaID, ProxyVariant(variant))
}

// open fetches the variant of a recorded remote media file as Open does, without a signature to
// check: the server's own reads of a remote file (the copy of a remote custom emoji) go through
// the same domain restrictions, limits, and cache as the clients' requests.
func (p *MediaProxy) open(ctx context.Context, mediaID string, variant ProxyVariant) (*ProxiedMedia, error) {
	notFound := func(why string) error {
		return fmt.Errorf("proxy media %s/%s: %s: %w", mediaID, variant, why, ErrNotFound)
	}
	m, err := p.find(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(m.RemoteURL)
	if err != nil || u.Hostname() == "" {
		return nil, notFound("malformed remote URL")
	}
	var owner string
	if m.OwnerAccountID != "" {
		if owner, err = p.d.Accounts.AccountDomain(ctx, m.OwnerAccountID); err != nil {
			return nil, fmt.Errorf("proxy media %s: owner: %w", mediaID, err)
		}
	}
	switch rejected, err := p.rejects(ctx, nil, owner, u.Hostname()); {
	case err != nil:
		return nil, err
	case rejected != "":
		return nil, notFound("media of " + rejected + " are rejected")
	}
	if p.d.Federation != nil {
		profile := m.Purpose == content.PurposeAvatar || m.Purpose == content.PurposeHeader
		switch ok, err := p.d.Federation.ProxiesMedia(ctx, m.OwnerAccountID, profile); {
		case err != nil:
			return nil, err
		case !ok:
			return nil, notFound("the federation mode does not fetch it")
		}
	}
	settings, err := p.d.Settings(ctx)
	if err != nil {
		return nil, err
	}
	cache, err := p.cacheable(ctx, m, settings.Policy)
	if err != nil {
		return nil, err
	}
	if variant == ProxySmall {
		return p.openSmall(ctx, m, settings.CacheTTL, cache)
	}
	if cache {
		return p.openCached(ctx, m, settings.CacheTTL)
	}
	return p.relay(ctx, m, settings.CacheTTL)
}

// find returns the remote media file mediaID: a file that a remote document declared, or the
// image of link cards, which has no owner and is served as an attachment's image.
func (p *MediaProxy) find(ctx context.Context, mediaID string) (content.RemoteMedia, error) {
	m, err := p.d.Repository.FindRemoteMedia(ctx, mediaID)
	if !errors.Is(err, ErrNotFound) || p.d.LinkCardMedia == nil {
		return m, err
	}
	c, err := p.d.LinkCardMedia.FindLinkCardMedia(ctx, mediaID)
	if err != nil {
		return content.RemoteMedia{}, err
	}
	return content.RemoteMedia{
		MediaID: c.MediaID, RemoteURL: c.RemoteURL, DeclaredContentType: c.ContentType,
		DeclaredMediaType: content.MediaTypeImage, Purpose: content.PurposeAttachment,
		Width: c.Width, Height: c.Height, Blurhash: c.Blurhash, Description: c.Description,
	}, nil
}

// smallCacheURL is the key of the small variant of m in the remote media cache: its URL with a
// fragment that names every input of the variant besides the file, so that a copy made from
// other inputs is never served. The fragment holds the variant, the purpose, the box of the
// purpose's small variant (which the configured sizes can change), and, when that box crops,
// the declared focal point that the crop centres on ("#small-avatar-96x96c-f0.5,-0.25"). An
// image without a declared focus is cropped around its centre, the same as a focus of 0,0.
func smallCacheURL(m content.RemoteMedia, sizes media.Sizes) string {
	b := sizes.For(media.Purpose(m.Purpose)).Small
	var sb strings.Builder
	sb.WriteString(m.RemoteURL + "#" + string(ProxySmall) + "-" + string(m.Purpose) + "-")
	if b.MaxPixels > 0 {
		sb.WriteString(strconv.FormatInt(b.MaxPixels, 10) + "px")
	} else {
		sb.WriteString(strconv.Itoa(b.Width) + "x" + strconv.Itoa(b.Height))
	}
	if b.MaxPixels <= 0 && b.Crop {
		var x, y float64
		if m.Focus != nil {
			x, y = m.Focus.X, m.Focus.Y
		}
		sb.WriteString("c-f" + strconv.FormatFloat(x, 'g', -1, 64) + "," + strconv.FormatFloat(y, 'g', -1, 64))
	}
	return sb.String()
}

// openSmall serves the small still variant of a remote image (ADR-0054 §7). It makes it from the
// file, which it fetches or reads from its cached copy as the full variant would, on every
// request; under a policy that caches m, it keeps the variant in the cache too.
func (p *MediaProxy) openSmall(ctx context.Context, m content.RemoteMedia, ttl time.Duration, cache bool) (*ProxiedMedia, error) {
	notFound := func(why string) error {
		return fmt.Errorf("proxy media %s/%s: %s: %w", m.MediaID, ProxySmall, why, ErrNotFound)
	}
	switch m.DeclaredMediaType {
	case content.MediaTypeVideo, content.MediaTypeGIFV, content.MediaTypeAudio:
		return nil, notFound("not an image")
	}
	key := smallCacheURL(m, p.d.Sizes)
	if cache {
		if pm, err := p.openCopy(ctx, key, ttl); pm != nil || err != nil {
			return pm, err
		}
	}
	var (
		src *ProxiedMedia
		err error
	)
	if cache {
		src, err = p.openCached(ctx, m, ttl)
	} else {
		src, err = p.relay(ctx, m, ttl)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = src.Body.Close() }()
	if !strings.HasPrefix(src.ContentType, "image/") {
		return nil, notFound("not an image")
	}
	data, err := io.ReadAll(src.Body)
	if err != nil {
		return nil, readError(m, err)
	}
	req := media.Request{Purpose: media.Purpose(m.Purpose)}
	if m.Focus != nil {
		req.FocusX, req.FocusY = m.Focus.X, m.Focus.Y
	}
	v, err := p.d.Processor.Preview(ctx, data, media.Limits{}, p.d.Sizes, req)
	if media.Rejected(err) {
		return nil, notFound(err.Error())
	}
	if err != nil {
		return nil, fmt.Errorf("proxy media %s/%s: %w", m.MediaID, ProxySmall, err)
	}
	if cache {
		p.keepSmall(ctx, m, key, v, ttl)
	}
	return &ProxiedMedia{ContentType: v.ContentType, Length: int64(len(v.Data)), MaxAge: ttl, Body: io.NopCloser(bytes.NewReader(v.Data))}, nil
}

// keepSmall keeps the small variant v of m in the remote media cache under key. A failure is
// logged: the variant is served anyway and made again next time.
func (p *MediaProxy) keepSmall(ctx context.Context, m content.RemoteMedia, key string, v media.Variant, ttl time.Duration) {
	if _, err := p.keepCopy(ctx, key, bytes.NewReader(v.Data), v.ContentType, ttl); err != nil {
		p.d.Logger.WarnContext(ctx, "small variant of remote media not cached", slog.String("media_id", m.MediaID), slog.Any("error", err))
	}
}

// rejects returns the first of domains whose media are rejected (the media_fetch restriction of
// a reject_media domain action), or "" when none is; "" domains are skipped. seen, when not nil,
// keeps the answers by domain across calls, so that a page of media asks once per domain.
func (p *MediaProxy) rejects(ctx context.Context, seen map[string]bool, domains ...string) (string, error) {
	return rejectsAny(ctx, p.d.Policy.RejectsMedia, seen, domains...)
}

// rejectsAny is MediaProxy.rejects with the check of one domain.
func rejectsAny(ctx context.Context, rejectsMedia func(ctx context.Context, domain string) (bool, error), seen map[string]bool, domains ...string) (string, error) {
	for _, domain := range domains {
		domain = strings.ToLower(domain)
		if domain == "" {
			continue
		}
		rejected, ok := seen[domain]
		if !ok {
			var err error
			if rejected, err = rejectsMedia(ctx, domain); err != nil {
				return "", err
			}
			if seen != nil {
				seen[domain] = rejected
			}
		}
		if rejected {
			return domain, nil
		}
	}
	return "", nil
}

// relay fetches a remote media file and relays it without keeping a copy.
func (p *MediaProxy) relay(ctx context.Context, m content.RemoteMedia, maxAge time.Duration) (*ProxiedMedia, error) {
	f, err := p.fetch(ctx, m)
	if err != nil {
		return nil, err
	}
	if f.length < 0 && f.kind == content.MediaTypeImage {
		// An image without a length is read whole (at most 16 MiB), so that the response has
		// one and a file over the limit is refused before anything is sent.
		data, err := io.ReadAll(f.body)
		_ = f.body.Close()
		if err != nil {
			return nil, readError(m, err)
		}
		return &ProxiedMedia{ContentType: f.contentType, Length: int64(len(data)), MaxAge: maxAge, Body: io.NopCloser(bytes.NewReader(data))}, nil
	}
	return &ProxiedMedia{ContentType: f.contentType, Length: f.length, MaxAge: maxAge, Body: f.body}, nil
}

// readError classifies an error reading the body of a remote media file: a file over the size
// limit is not found, and anything else is an upstream failure.
func readError(m content.RemoteMedia, err error) error {
	if errors.Is(err, kernel.ErrResponseTooLarge) {
		return fmt.Errorf("proxy media %s: %w: %w", m.MediaID, ErrNotFound, err)
	}
	return fmt.Errorf("proxy media %s: read: %w: %w", m.MediaID, ErrProxyUpstream, err)
}

// fetchedMedia is the response of a remote media file in an allowed format.
type fetchedMedia struct {
	contentType string
	kind        content.MediaType
	// length is the declared length of the body, or -1.
	length int64
	// body reads the whole file and fails past the size limit of its kind; the caller closes
	// it.
	body *limitedBody
}

// fetch requests a remote media file and checks its status, format, and declared length.
func (p *MediaProxy) fetch(ctx context.Context, m content.RemoteMedia) (*fetchedMedia, error) {
	purpose := kernel.HTTPPurposeVideo
	if m.DeclaredMediaType == content.MediaTypeImage {
		purpose = kernel.HTTPPurposeImage
	}
	resp, err := p.d.HTTP.Do(ctx, &kernel.HTTPRequest{
		URL: m.RemoteURL, Purpose: purpose,
		Header: http.Header{"Accept": {"image/*, video/*, audio/*;q=0.9, */*;q=0.1"}},
	})
	switch {
	case errors.Is(err, kernel.ErrForbiddenDestination), errors.Is(err, kernel.ErrResponseTooLarge),
		errors.Is(err, kernel.ErrInvalidHTTPRequest), errors.Is(err, kernel.ErrTooManyRedirects):
		return nil, fmt.Errorf("proxy media %s: %w: %w", m.MediaID, ErrNotFound, err)
	case err != nil:
		return nil, fmt.Errorf("proxy media %s: %w: %w", m.MediaID, ErrProxyUpstream, err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = resp.Body.Close()
		}
	}()
	switch {
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("proxy media %s: status %d: %w", m.MediaID, resp.StatusCode, ErrProxyUpstream)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("proxy media %s: status %d: %w", m.MediaID, resp.StatusCode, ErrNotFound)
	}

	head := make([]byte, content.SniffHeadLength)
	n, err := io.ReadFull(resp.Body, head)
	switch {
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
	case errors.Is(err, kernel.ErrResponseTooLarge):
		return nil, fmt.Errorf("proxy media %s: %w: %w", m.MediaID, ErrNotFound, err)
	case err != nil:
		return nil, fmt.Errorf("proxy media %s: read: %w: %w", m.MediaID, ErrProxyUpstream, err)
	}
	head = head[:n]
	contentType, kind := content.SniffRemoteMedia(head)
	if contentType == "" || isEmojiImage(m) && !content.CustomEmojiFormat(contentType) {
		return nil, fmt.Errorf("proxy media %s: %w: %w", m.MediaID, ErrNotFound, ErrUnsupportedRemoteMedia)
	}
	limitPurpose := kernel.HTTPPurposeVideo
	if kind == content.MediaTypeImage {
		limitPurpose = kernel.HTTPPurposeImage
	}
	limits, _ := kernel.DefaultHTTPLimits(limitPurpose)
	if isEmojiImage(m) {
		limits.MaxBodyBytes = min(limits.MaxBodyBytes, p.d.EmojiMaxBytes)
	}

	length := int64(-1)
	if cl := resp.Header.Get("Content-Length"); cl != "" && resp.Header.Get("Content-Encoding") == "" {
		if v, err := strconv.ParseInt(cl, 10, 64); err == nil && v >= int64(n) {
			length = v
		}
	}
	if length > limits.MaxBodyBytes {
		return nil, fmt.Errorf("proxy media %s: %d bytes: %w: %w", m.MediaID, length, ErrNotFound, kernel.ErrResponseTooLarge)
	}
	ok = true
	return &fetchedMedia{
		contentType: contentType, kind: kind, length: length,
		body: &limitedBody{r: io.MultiReader(bytes.NewReader(head), resp.Body), c: resp.Body, left: limits.MaxBodyBytes},
	}, nil
}

// isEmojiImage reports whether m is the image of a custom emoji (recorded for an Emoji tag),
// which the proxy serves only in the formats and within the size of ADR-0051 §2.
func isEmojiImage(m content.RemoteMedia) bool { return m.Purpose == content.PurposeEmoji }

// checkEmojiCopy refuses the copy pm of the image of a custom emoji that the limits of
// ADR-0051 §2 no longer allow: one kept before remote_custom_emoji_max_bytes was lowered. It
// closes pm when it refuses it.
func (p *MediaProxy) checkEmojiCopy(m content.RemoteMedia, pm *ProxiedMedia) error {
	if !isEmojiImage(m) || content.CustomEmojiFormat(pm.ContentType) && pm.Length <= p.d.EmojiMaxBytes {
		return nil
	}
	_ = pm.Body.Close()
	why := ErrUnsupportedRemoteMedia
	if content.CustomEmojiFormat(pm.ContentType) {
		why = kernel.ErrResponseTooLarge
	}
	return fmt.Errorf("proxy media %s: a copy of %d bytes of %s: %w: %w", m.MediaID, pm.Length, pm.ContentType, ErrNotFound, why)
}

// limitedBody reads at most left bytes and fails with kernel.ErrResponseTooLarge past them.
type limitedBody struct {
	r    io.Reader
	c    io.Closer
	left int64
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.left <= 0 {
		// Probe for one more byte: a body of exactly the limit is not too large.
		var one [1]byte
		n, err := b.r.Read(one[:])
		if n > 0 {
			return 0, fmt.Errorf("media proxy: %w", kernel.ErrResponseTooLarge)
		}
		return 0, err
	}
	if int64(len(p)) > b.left {
		p = p[:b.left]
	}
	n, err := b.r.Read(p)
	b.left -= int64(n)
	return n, err
}

func (b *limitedBody) Close() error { return b.c.Close() }
