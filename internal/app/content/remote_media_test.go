// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/app/uow"
	domain "github.com/mycroblog/my/internal/domain/content"
)

// proxyRepo is a MediaRepository that holds only remote media files.
type proxyRepo struct {
	content.MediaRepository
	files map[string]domain.RemoteMedia
}

func (r proxyRepo) FindRemoteMedia(_ context.Context, id string) (domain.RemoteMedia, error) {
	if m, ok := r.files[id]; ok {
		return m, nil
	}
	return domain.RemoteMedia{}, fmt.Errorf("remote media %s: %w", id, content.ErrNotFound)
}

type domains map[string]string

func (d domains) AccountDomain(_ context.Context, id string) (string, error) {
	if v, ok := d[id]; ok {
		return v, nil
	}
	return "", errors.New("no such account")
}

type rejected map[string]bool

func (r rejected) RejectsMedia(_ context.Context, domain string) (bool, error) { return r[domain], nil }

func (r rejected) RejectsMediaTx(ctx context.Context, _ uow.Tx, domain string) (bool, error) {
	return r.RejectsMedia(ctx, domain)
}

// fakeHTTP answers every request with the response of its URL, or with err. It enforces the
// purpose's body limit as the real HTTPClient does.
type fakeHTTP struct {
	responses map[string]fakeResponse
	requests  []*kernel.HTTPRequest
}

type fakeResponse struct {
	status int
	header http.Header
	body   []byte
	err    error
}

func (f *fakeHTTP) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	f.requests = append(f.requests, req)
	r, ok := f.responses[req.URL]
	if !ok {
		return nil, fmt.Errorf("unexpected request %s", req.URL)
	}
	if r.err != nil {
		return nil, r.err
	}
	limits, _ := kernel.DefaultHTTPLimits(req.Purpose)
	if cl := r.header.Get("Content-Length"); cl != "" {
		if n, _ := strconv.ParseInt(cl, 10, 64); n > limits.MaxBodyBytes {
			return nil, kernel.ErrResponseTooLarge
		}
	}
	h := r.header
	if h == nil {
		h = http.Header{}
	}
	status := r.status
	if status == 0 {
		status = http.StatusOK
	}
	body := io.NopCloser(io.MultiReader(io.LimitReader(bytes.NewReader(r.body), limits.MaxBodyBytes), tooLargeAfter(r.body, limits.MaxBodyBytes)))
	return &kernel.HTTPResponse{StatusCode: status, Header: h, URL: req.URL, Body: body}, nil
}

// tooLargeAfter fails with ErrResponseTooLarge when body is longer than limit.
func tooLargeAfter(body []byte, limit int64) io.Reader {
	if int64(len(body)) > limit {
		return iotest{err: kernel.ErrResponseTooLarge}
	}
	return bytes.NewReader(nil)
}

type iotest struct{ err error }

func (r iotest) Read([]byte) (int, error) { return 0, r.err }

const (
	ownerID  = "0192f0c1-0000-7000-8000-0000000000a1"
	pngID    = "0192f0c1-0000-7000-8000-000000000001"
	remoteMP = "0192f0c1-0000-7000-8000-000000000002"
)

var pngData = append([]byte("\x89PNG\r\n\x1A\n\x00\x00\x00\x0DIHDR"), bytes.Repeat([]byte{0}, 100)...)

type proxyEnv struct {
	proxy *content.MediaProxy
	repo  proxyRepo
	http  *fakeHTTP
	rej   rejected
}

func newProxyEnv(t *testing.T, keys ...[]byte) *proxyEnv {
	t.Helper()
	if len(keys) == 0 {
		keys = [][]byte{bytes.Repeat([]byte("k"), 32)}
	}
	e := &proxyEnv{
		repo: proxyRepo{files: map[string]domain.RemoteMedia{}},
		http: &fakeHTTP{responses: map[string]fakeResponse{}},
		rej:  rejected{},
	}
	p, err := content.NewMediaProxy(content.ProxyDeps{
		Repository: e.repo, HTTP: e.http, Accounts: domains{ownerID: "remote.example"}, Policy: e.rej,
		Settings: func(context.Context) (content.ProxySettings, error) {
			return content.ProxySettings{Policy: content.RemoteMediaCacheAll, CacheTTL: time.Hour}, nil
		},
		Keys: keys, BaseURL: "https://social.example/",
	})
	if err != nil {
		t.Fatal(err)
	}
	e.proxy = p
	return e
}

func (e *proxyEnv) add(id, remoteURL string, kind domain.MediaType, resp fakeResponse) {
	e.repo.files[id] = domain.RemoteMedia{MediaID: id, OwnerAccountID: ownerID, RemoteURL: remoteURL, DeclaredMediaType: kind}
	e.http.responses[remoteURL] = resp
}

// open opens the proxy URL that the proxy signs for id.
func (e *proxyEnv) open(t *testing.T, id string) (*content.ProxiedMedia, []byte, error) {
	t.Helper()
	u, err := url.Parse(e.proxy.URL(id, content.ProxyFull))
	if err != nil {
		t.Fatal(err)
	}
	m, err := e.proxy.Open(t.Context(), id, "full", u.Query().Get("sig"))
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = m.Body.Close() }()
	data, err := io.ReadAll(m.Body)
	return m, data, err
}

func TestMediaProxyRelaysWithTheDetectedType(t *testing.T) {
	e := newProxyEnv(t)
	e.add(pngID, "https://cdn.remote.example/a.svg", domain.MediaTypeImage, fakeResponse{
		header: http.Header{"Content-Type": {"image/svg+xml"}, "Content-Length": {strconv.Itoa(len(pngData))}}, body: pngData,
	})
	got := e.proxy.URL(pngID, content.ProxyFull)
	if !strings.HasPrefix(got, "https://social.example/media/proxy/"+pngID+"/full?sig=") {
		t.Errorf("URL = %s", got)
	}
	m, data, err := e.open(t, pngID)
	if err != nil {
		t.Fatal(err)
	}
	if m.ContentType != "image/png" || m.Length != int64(len(pngData)) || m.MaxAge != time.Hour || !bytes.Equal(data, pngData) {
		t.Errorf("Open = %s, %d, %v, %d bytes", m.ContentType, m.Length, m.MaxAge, len(data))
	}
	req := e.http.requests[0]
	if req.Purpose != kernel.HTTPPurposeImage || req.Method != "" || req.Body != nil {
		t.Errorf("request = %+v, want a GET for an image", req)
	}
}

func TestMediaProxyRefusesBadSignatures(t *testing.T) {
	e := newProxyEnv(t)
	e.add(pngID, "https://remote.example/a.png", domain.MediaTypeImage, fakeResponse{body: pngData})
	u, _ := url.Parse(e.proxy.URL(pngID, content.ProxyFull))
	sig := u.Query().Get("sig")
	other := newProxyEnv(t, bytes.Repeat([]byte("o"), 32))
	ou, _ := url.Parse(other.proxy.URL(pngID, content.ProxyFull))
	tampered := []byte(sig)
	tampered[0] ^= 1

	for _, tc := range []struct{ name, id, variant, sig string }{
		{"no signature", pngID, "full", ""},
		{"tampered signature", pngID, "full", string(tampered)},
		{"signature of another media file", remoteMP, "full", sig},
		{"signature of another variant", pngID, "thumbnail", sig},
		{"signature under another key", pngID, "full", ou.Query().Get("sig")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.proxy.Open(t.Context(), tc.id, tc.variant, tc.sig); !errors.Is(err, content.ErrNotFound) {
				t.Errorf("Open = %v, want ErrNotFound", err)
			}
		})
	}
	if len(e.http.requests) != 0 {
		t.Errorf("fetched %d times for bad signatures", len(e.http.requests))
	}
}

func TestMediaProxyVerifiesWithPreviousKeys(t *testing.T) {
	old := bytes.Repeat([]byte("o"), 32)
	before := newProxyEnv(t, old)
	u, _ := url.Parse(before.proxy.URL(pngID, content.ProxyFull))

	e := newProxyEnv(t, bytes.Repeat([]byte("n"), 32), old)
	e.add(pngID, "https://remote.example/a.png", domain.MediaTypeImage, fakeResponse{body: pngData})
	if _, err := e.proxy.Open(t.Context(), pngID, "full", u.Query().Get("sig")); err != nil {
		t.Errorf("Open with a URL signed under the previous key = %v", err)
	}
	if e.proxy.URL(pngID, content.ProxyFull) == before.proxy.URL(pngID, content.ProxyFull) {
		t.Error("the previous key still signs")
	}
}

func TestMediaProxyRefuses(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	bigImage := append(append([]byte{}, pngData...), make([]byte, 16<<20)...)
	for _, tc := range []struct {
		name string
		kind domain.MediaType
		resp fakeResponse
		want error
	}{
		{"svg", domain.MediaTypeImage, fakeResponse{header: http.Header{"Content-Type": {"image/png"}}, body: svg}, content.ErrNotFound},
		{"html", domain.MediaTypeUnknown, fakeResponse{body: []byte("<!doctype html><script>alert(1)</script>")}, content.ErrNotFound},
		{"empty", domain.MediaTypeImage, fakeResponse{}, content.ErrNotFound},
		{"image over 16 MiB by Content-Length", domain.MediaTypeImage, fakeResponse{
			header: http.Header{"Content-Length": {strconv.Itoa(len(bigImage))}}, body: bigImage,
		}, content.ErrNotFound},
		{"image over 16 MiB without a length", domain.MediaTypeImage, fakeResponse{body: bigImage}, content.ErrNotFound},
		{"image over 16 MiB declared as video", domain.MediaTypeVideo, fakeResponse{
			header: http.Header{"Content-Length": {strconv.Itoa(len(bigImage))}}, body: bigImage,
		}, content.ErrNotFound},
		{"image over 16 MiB declared as video without a length", domain.MediaTypeVideo, fakeResponse{body: bigImage}, content.ErrNotFound},
		{"refused destination", domain.MediaTypeImage, fakeResponse{err: fmt.Errorf("dial: %w", kernel.ErrForbiddenDestination)}, content.ErrNotFound},
		{"too many redirects", domain.MediaTypeImage, fakeResponse{err: kernel.ErrTooManyRedirects}, content.ErrNotFound},
		{"not found upstream", domain.MediaTypeImage, fakeResponse{status: http.StatusNotFound, body: pngData}, content.ErrNotFound},
		{"gone upstream", domain.MediaTypeImage, fakeResponse{status: http.StatusGone}, content.ErrNotFound},
		{"server error upstream", domain.MediaTypeImage, fakeResponse{status: http.StatusServiceUnavailable}, content.ErrProxyUpstream},
		{"network error", domain.MediaTypeImage, fakeResponse{err: errors.New("connection reset")}, content.ErrProxyUpstream},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newProxyEnv(t)
			e.add(pngID, "https://remote.example/file", tc.kind, tc.resp)
			_, _, err := e.open(t, pngID)
			if !errors.Is(err, tc.want) {
				t.Errorf("Open = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestMediaProxyStreamsVideoUpToTheLimit(t *testing.T) {
	mp4 := append([]byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00isomiso2mp41"), make([]byte, 1000)...)
	e := newProxyEnv(t)
	e.add(remoteMP, "https://remote.example/v.mp4", domain.MediaTypeGIFV, fakeResponse{body: mp4})
	m, data, err := e.open(t, remoteMP)
	if err != nil {
		t.Fatal(err)
	}
	if m.ContentType != "video/mp4" || m.Length != -1 || !bytes.Equal(data, mp4) {
		t.Errorf("Open = %s, %d, %d bytes", m.ContentType, m.Length, len(data))
	}
	if e.http.requests[0].Purpose != kernel.HTTPPurposeVideo {
		t.Errorf("purpose = %v, want video", e.http.requests[0].Purpose)
	}

	big := append(append([]byte{}, mp4...), make([]byte, 100<<20)...)
	e = newProxyEnv(t)
	e.add(remoteMP, "https://remote.example/v.mp4", domain.MediaTypeVideo, fakeResponse{body: big})
	if _, _, err := e.open(t, remoteMP); !errors.Is(err, kernel.ErrResponseTooLarge) {
		t.Errorf("reading a video over 100 MiB = %v, want ErrResponseTooLarge", err)
	}
}

func TestMediaProxyAppliesMediaFetchRestrictions(t *testing.T) {
	for _, domainName := range []string{"remote.example", "cdn.example"} {
		t.Run(domainName, func(t *testing.T) {
			e := newProxyEnv(t)
			e.add(pngID, "https://CDN.example/a.png", domain.MediaTypeImage, fakeResponse{body: pngData})
			e.rej[domainName] = true
			if _, _, err := e.open(t, pngID); !errors.Is(err, content.ErrNotFound) {
				t.Errorf("Open = %v, want ErrNotFound", err)
			}
			if len(e.http.requests) != 0 {
				t.Error("fetched the media of a rejected domain")
			}
		})
	}
}

func TestMediaProxyRefusesUnknownFiles(t *testing.T) {
	e := newProxyEnv(t)
	if _, _, err := e.open(t, pngID); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("Open(unknown) = %v, want ErrNotFound", err)
	}
	u, _ := url.Parse(e.proxy.URL(pngID, content.ProxyVariant("thumbnail")))
	e.add(pngID, "https://remote.example/a.png", domain.MediaTypeImage, fakeResponse{body: pngData})
	if _, err := e.proxy.Open(t.Context(), pngID, "thumbnail", u.Query().Get("sig")); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("Open(thumbnail) = %v, want ErrNotFound", err)
	}
}

func TestNewMediaProxyNeedsAKey(t *testing.T) {
	for _, keys := range [][][]byte{nil, {[]byte("short")}} {
		if _, err := content.NewMediaProxy(content.ProxyDeps{Keys: keys}); err == nil {
			t.Errorf("NewMediaProxy(%q) succeeded", keys)
		}
	}
}

func TestMediaProxyLoadsItsKeyOnFirstUse(t *testing.T) {
	t.Parallel()
	key := bytes.Repeat([]byte("k"), 32)
	calls, fail := 0, true
	lazy, err := content.NewMediaProxy(content.ProxyDeps{
		KeySource: func(context.Context) ([]byte, error) {
			calls++
			if fail {
				return nil, errors.New("no schema yet")
			}
			return key, nil
		},
		BaseURL: "https://social.example/", Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := lazy.LoadKeys(t.Context()); err == nil {
		t.Fatal("LoadKeys succeeded while the key source fails")
	}
	if u := lazy.URL(pngID, content.ProxyFull); !strings.HasSuffix(u, "?sig=") {
		t.Errorf("URL without a key = %q, want an empty signature", u)
	}
	fail = false
	want := newProxyEnv(t, key).proxy.URL(pngID, content.ProxyFull)
	if got := lazy.URL(pngID, content.ProxyFull); got != want {
		t.Errorf("URL after the key loads = %q, want %q", got, want)
	}
	_ = lazy.LoadKeys(t.Context())
	_ = lazy.URL(pngID, content.ProxySmall)
	if calls != 3 {
		t.Errorf("key source calls = %d, want 3 (none after it succeeded)", calls)
	}
}

// openSmall opens the small variant of a remote media file through proxy.
func openSmall(t *testing.T, proxy *content.MediaProxy, id string) (*content.ProxiedMedia, []byte, error) {
	t.Helper()
	u, err := url.Parse(proxy.URL(id, content.ProxySmall))
	if err != nil {
		t.Fatal(err)
	}
	m, err := proxy.Open(t.Context(), id, string(content.ProxySmall), u.Query().Get("sig"))
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = m.Body.Close() }()
	data, err := io.ReadAll(m.Body)
	return m, data, err
}

func TestMediaProxySmallVariant(t *testing.T) {
	e := newProxyEnv(t)
	e.add(pngID, "https://remote.example/a.png", domain.MediaTypeImage, fakeResponse{body: pngData})
	mp4 := append([]byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00isomiso2mp41"), make([]byte, 1000)...)
	e.add(remoteMP, "https://remote.example/v.mp4", domain.MediaTypeUnknown, fakeResponse{body: mp4})
	const videoID = "0192f0c1-0000-7000-8000-000000000003"
	e.add(videoID, "https://remote.example/w.mp4", domain.MediaTypeVideo, fakeResponse{body: mp4})

	// Without a processor, there is no small variant.
	if _, _, err := openSmall(t, e.proxy, pngID); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("small without a processor = %v, want ErrNotFound", err)
	}

	proxy, err := content.NewMediaProxy(content.ProxyDeps{
		Repository: e.repo, HTTP: e.http, Accounts: domains{ownerID: "remote.example"}, Policy: e.rej,
		Settings: func(context.Context) (content.ProxySettings, error) {
			return content.ProxySettings{Policy: content.RemoteMediaCacheNone, CacheTTL: time.Hour}, nil
		},
		Keys: [][]byte{bytes.Repeat([]byte("k"), 32)}, BaseURL: "https://social.example/", Processor: stubPreview{},
	})
	if err != nil {
		t.Fatal(err)
	}
	m, data, err := openSmall(t, proxy, pngID)
	want, _ := stubPreview{}.Preview(t.Context(), pngData, media.Limits{}, media.Sizes{}, media.Request{})
	if err != nil || m.ContentType != want.ContentType || !bytes.Equal(data, want.Data) || m.MaxAge != time.Hour {
		t.Errorf("small = %+v, %q, %v; want the preview", m, data, err)
	}
	// Content that is not an image, and a declared video, have no small variant; the video is
	// not even fetched.
	before := len(e.http.requests)
	for _, id := range []string{remoteMP, videoID} {
		if _, _, err := openSmall(t, proxy, id); !errors.Is(err, content.ErrNotFound) {
			t.Errorf("small of %s = %v, want ErrNotFound", id, err)
		}
	}
	if n := len(e.http.requests) - before; n != 1 {
		t.Errorf("%d fetches, want 1 (the video declared as one is not fetched)", n)
	}
}

// stubPreview is a media.Processor whose preview is the source behind a marker.
type stubPreview struct{ media.Processor }

func (stubPreview) Preview(_ context.Context, data []byte, _ media.Limits, _ media.Sizes, _ media.Request) (media.Variant, error) {
	return media.Variant{Data: append([]byte("small:"), data[:8]...), ContentType: "image/webp", Extension: "webp", Width: 1, Height: 1}, nil
}

func TestMediaProxyLimitsCustomEmojiImages(t *testing.T) {
	jpeg := append([]byte("\xFF\xD8\xFF\xE0"), make([]byte, 100)...)
	gif := append([]byte("GIF89a"), make([]byte, 100)...)
	webp := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 100)...)
	big := append(append([]byte{}, pngData...), make([]byte, 1<<20)...)
	for _, tc := range []struct {
		name     string
		purpose  domain.MediaPurpose
		maxBytes int64
		resp     fakeResponse
		want     error
	}{
		{"png", domain.PurposeEmoji, 0, fakeResponse{body: pngData}, nil},
		{"gif", domain.PurposeEmoji, 0, fakeResponse{body: gif}, nil},
		{"webp", domain.PurposeEmoji, 0, fakeResponse{body: webp}, nil},
		{"jpeg", domain.PurposeEmoji, 0, fakeResponse{body: jpeg}, content.ErrNotFound},
		{"jpeg of an attachment", domain.PurposeAttachment, 0, fakeResponse{body: jpeg}, nil},
		{"over 1 MiB by Content-Length", domain.PurposeEmoji, 0, fakeResponse{
			header: http.Header{"Content-Length": {strconv.Itoa(len(big))}}, body: big,
		}, content.ErrNotFound},
		{"over 1 MiB without a length", domain.PurposeEmoji, 0, fakeResponse{body: big}, content.ErrNotFound},
		{"over 1 MiB of an attachment", domain.PurposeAttachment, 0, fakeResponse{body: big}, nil},
		{"exactly the limit", domain.PurposeEmoji, int64(len(pngData)), fakeResponse{body: pngData}, nil},
		{"over the configured limit", domain.PurposeEmoji, int64(len(pngData)) - 1, fakeResponse{body: pngData}, content.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newProxyEnv(t)
			if tc.maxBytes > 0 {
				p, err := content.NewMediaProxy(content.ProxyDeps{
					Repository: e.repo, HTTP: e.http, Accounts: domains{ownerID: "remote.example"}, Policy: e.rej,
					Settings: func(context.Context) (content.ProxySettings, error) {
						return content.ProxySettings{Policy: content.RemoteMediaCacheNone, CacheTTL: time.Hour}, nil
					},
					Keys: [][]byte{bytes.Repeat([]byte("k"), 32)}, BaseURL: "https://social.example/", EmojiMaxBytes: tc.maxBytes,
				})
				if err != nil {
					t.Fatal(err)
				}
				e.proxy = p
			}
			e.add(pngID, "https://remote.example/emoji", domain.MediaTypeImage, tc.resp)
			m := e.repo.files[pngID]
			m.Purpose = tc.purpose
			e.repo.files[pngID] = m
			_, data, err := e.open(t, pngID)
			switch {
			case tc.want == nil && (err != nil || !bytes.Equal(data, tc.resp.body)):
				t.Errorf("Open = %d bytes, %v; want the file", len(data), err)
			case tc.want != nil && !errors.Is(err, tc.want):
				t.Errorf("Open = %v, want %v", err, tc.want)
			}
		})
	}
}
