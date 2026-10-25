// SPDX-License-Identifier: MIT

package integration_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/platformtest"

	"github.com/mycroblog/my/pkg/mykit"
)

// signIn returns a client of srv for u: the code of the authorization page exchanged at the token endpoint with
// the PKCE verifier of mykit, and the token kept by a RefreshingTokenSource, as an application would keep it.
// cfg's Server and TokenSource are filled in.
func signIn(t *testing.T, srv *platformtest.Server, u platformtest.User, cfg mykit.Config) *mykit.API {
	t.Helper()
	verifier := mykit.NewCodeVerifier()
	code := srv.AuthorizationCode(t, u, "", mykit.CodeChallenge(verifier))
	oauth := mykit.OAuthConfig{Server: srv.URL, ClientID: platformtest.WebClientID, RedirectURI: platformtest.WebClientRedirectURI}
	tok, err := oauth.Exchange(t.Context(), code, verifier)
	if err != nil {
		t.Fatalf("exchange the authorization code of %s: %v", u.Username, err)
	}
	// The web client gets its refresh token as a cookie, not in the answer: the access token outlives the test.
	if tok.AccessToken == "" || tok.Expiry.Before(time.Now().Add(time.Minute)) {
		t.Fatalf("token of %s = %+v, want an access token valid for longer than the test", u.Username, tok)
	}
	cfg.Server = srv.URL
	cfg.TokenSource = mykit.NewRefreshingTokenSource(oauth, tok, func(context.Context, *mykit.Token) error { return nil })
	if cfg.AcceptLanguage == "" {
		cfg.AcceptLanguage = "en"
	}
	api, err := mykit.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return api
}

// post creates a public status with text, and the media of ids.
func post(t *testing.T, api *mykit.API, text string, ids ...string) mykit.Status {
	t.Helper()
	req := mykit.CreateStatusRequest{Text: text, Visibility: mykit.VisibilityPublic}
	if len(ids) > 0 {
		media := make(mykit.StatusMediaInput, 0, len(ids))
		for _, id := range ids {
			media = append(media, mykit.StatusMediaItem{Id: id})
		}
		req.Media = &media
	}
	resp, err := api.CreateStatusWithResponse(t.Context(), &mykit.CreateStatusParams{}, req)
	if err != nil {
		t.Fatalf("post %q: %v", text, err)
	}
	if resp.JSON200 == nil {
		t.Fatalf("post %q = %d %s, want 200 with the status", text, resp.StatusCode(), resp.Body)
	}
	return *resp.JSON200
}

// homeTimeline returns the PageFunc of the home timeline, limit statuses a page, counting the pages it fetches.
func homeTimeline(api *mykit.API, limit int, pages *int) mykit.PageFunc[mykit.Status] {
	return func(ctx context.Context, cursor *mykit.Cursor) ([]mykit.Status, *mykit.Cursor, error) {
		resp, err := api.GetHomeTimelineWithResponse(ctx, &mykit.GetHomeTimelineParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			return nil, nil, err
		}
		if resp.JSON200 == nil {
			return nil, nil, fmt.Errorf("home timeline = %d %s, want 200 with a page", resp.StatusCode(), resp.Body)
		}
		*pages++
		return resp.JSON200.Items, resp.JSON200.NextCursor, nil
	}
}

// TestPostAndPageHomeTimeline posts statuses and reads them back from the home timeline, following next_cursor
// page by page with mykit.All.
func TestPostAndPageHomeTimeline(t *testing.T) {
	t.Parallel()
	srv := platformtest.Start(t, platformtest.Options{})
	alice, bob := srv.User(t, "alice"), srv.User(t, "bob")
	aliceAPI, bobAPI := signIn(t, srv, alice, mykit.Config{}), signIn(t, srv, bob, mykit.Config{})

	follow, err := aliceAPI.FollowAccountWithResponse(t.Context(), bob.AccountID, mykit.FollowAccountRequest{})
	if err != nil {
		t.Fatalf("alice follows bob: %v", err)
	}
	if follow.StatusCode() != http.StatusOK {
		t.Fatalf("alice follows bob = %d %s, want 200", follow.StatusCode(), follow.Body)
	}
	var want []string
	for i := range 5 {
		s := post(t, bobAPI, fmt.Sprintf("post %d", i))
		if s.Account.Id != bob.AccountID {
			t.Errorf("post %d: account = %s, want bob's %s", i, s.Account.Id, bob.AccountID)
		}
		want = append(want, s.Id)
	}
	slices.Reverse(want) // newest first

	pages := 0
	var got []string
	for s, err := range mykit.All(t.Context(), homeTimeline(aliceAPI, 2, &pages)) {
		if err != nil {
			t.Fatalf("page the home timeline: %v", err)
		}
		got = append(got, s.Id)
	}
	if !slices.Equal(got, want) {
		t.Errorf("home timeline = %v, want bob's posts newest first %v", got, want)
	}
	// Pages of 2, 2, and 1: the last one has no next_cursor, or an empty page follows.
	if pages < 3 || pages > 4 {
		t.Errorf("pages fetched = %d, want 3 (or 4 with an empty last page)", pages)
	}

	// The loop stops when the caller does, without fetching further pages.
	pages = 0
	for _, err := range mykit.All(t.Context(), homeTimeline(aliceAPI, 2, &pages)) {
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	if pages != 1 {
		t.Errorf("pages fetched for the first status = %d, want 1", pages)
	}
}

// TestStream opens the event stream and receives a post of a followed account as an update.
func TestStream(t *testing.T) {
	t.Parallel()
	srv := platformtest.Start(t, platformtest.Options{})
	alice, bob := srv.User(t, "alice"), srv.User(t, "bob")
	aliceAPI, bobAPI := signIn(t, srv, alice, mykit.Config{}), signIn(t, srv, bob, mykit.Config{})
	if _, err := aliceAPI.FollowAccountWithResponse(t.Context(), bob.AccountID, mykit.FollowAccountRequest{}); err != nil {
		t.Fatalf("alice follows bob: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	opened := make(chan bool, 4)
	updates := make(chan mykit.Status, 16)
	var streamErrs []error
	var mu sync.Mutex
	done := make(chan error, 1)
	go func() {
		done <- aliceAPI.Stream(ctx, mykit.StreamOptions{
			OnOpen: func(reconnected bool) { opened <- reconnected },
			OnEvent: func(e mykit.StreamEvent) {
				if u, ok := e.(*mykit.UpdateEvent); ok {
					updates <- u.Status
				}
			},
			OnError: func(err error) { mu.Lock(); streamErrs = append(streamErrs, err); mu.Unlock() },
		})
	}()
	select {
	case reconnected := <-opened:
		if reconnected {
			t.Error("first OnOpen: reconnected = true, want false")
		}
	case err := <-done:
		t.Fatalf("Stream returned before it opened: %v", err)
	case <-ctx.Done():
		t.Fatal("the stream did not open")
	}

	s := post(t, bobAPI, "hello, stream")
	select {
	case got := <-updates:
		if got.Id != s.Id || got.Account.Id != bob.AccountID {
			t.Errorf("update = status %s by %s, want bob's post %s", got.Id, got.Account.Id, s.Id)
		}
	case <-ctx.Done():
		t.Fatal("no update arrived for bob's post")
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Stream after the context ended = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Stream did not return after the context ended")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(streamErrs) > 0 {
		t.Errorf("stream errors = %v, want none", streamErrs)
	}
}

// pngImage returns a small PNG image.
func pngImage(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for x := range 16 {
		for y := range 16 {
			img.Set(x, y, color.RGBA{R: uint8(x * 16), G: uint8(y * 16), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestUploadMedia uploads an image with mykit.UploadMedia, waits for the server to process it, and attaches it to
// a post.
func TestUploadMedia(t *testing.T) {
	t.Parallel()
	srv := platformtest.Start(t, platformtest.Options{})
	alice := srv.User(t, "alice")
	api := signIn(t, srv, alice, mykit.Config{})

	media, err := api.UploadMedia(t.Context(), mykit.MediaUploadInput{
		File:        mykit.MediaFile{Name: "square.png", ContentType: "image/png", Body: bytes.NewReader(pngImage(t))},
		Description: "A square of colors",
	})
	if err != nil {
		t.Fatalf("UploadMedia: %v", err)
	}
	if media.Id == "" || media.Type != mykit.MediaAttachmentTypeImage {
		t.Fatalf("uploaded = %+v, want an image with an ID", media)
	}

	deadline := time.Now().Add(30 * time.Second)
	for media.State == mykit.MediaAttachmentStateProcessing {
		if time.Now().After(deadline) {
			t.Fatalf("media %s still processing after 30 seconds", media.Id)
		}
		time.Sleep(100 * time.Millisecond)
		resp, err := api.GetMediaWithResponse(t.Context(), media.Id)
		if err != nil {
			t.Fatalf("GetMedia: %v", err)
		}
		if resp.JSON200 == nil {
			t.Fatalf("GetMedia = %d %s, want 200", resp.StatusCode(), resp.Body)
		}
		media = resp.JSON200
	}
	if media.State != mykit.MediaAttachmentStateReady {
		t.Fatalf("media state = %s, want ready", media.State)
	}
	if d := media.Description; d == nil || *d != "A square of colors" {
		t.Errorf("media description = %v, want the upload's", d)
	}

	s := post(t, api, "a picture", media.Id)
	if len(s.MediaAttachments) != 1 || s.MediaAttachments[0].Id != media.Id {
		t.Errorf("post media = %+v, want the uploaded %s", s.MediaAttachments, media.Id)
	}
}

// statusRecorder records the status and the Retry-After of every answer, under the client's retries.
type statusRecorder struct {
	mu      sync.Mutex
	answers []answer
}

type answer struct {
	status     int
	retryAfter string
}

func (r *statusRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err == nil {
		r.mu.Lock()
		r.answers = append(r.answers, answer{resp.StatusCode, resp.Header.Get("Retry-After")})
		r.mu.Unlock()
	}
	return resp, err
}

func (r *statusRecorder) take() []answer {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.answers
	r.answers = nil
	return a
}

// TestRetryAfterTooManyRequests reaches the per-account rate limit, which the server answers with 429 and
// Retry-After, and checks that the client waits and sends the request again until it is answered.
func TestRetryAfterTooManyRequests(t *testing.T) {
	t.Parallel()
	// A few requests in a short window reach the limit, and the Retry-After is a second or two.
	srv := platformtest.Start(t, platformtest.Options{
		AccountLimit: kernel.Limit{Name: "api.account", Max: 5, Window: 2 * time.Second},
	})
	alice := srv.User(t, "alice")
	rec := &statusRecorder{}
	api := signIn(t, srv, alice, mykit.Config{HTTPClient: &http.Client{Transport: rec}})

	for i := range 40 {
		resp, err := api.VerifyAccountCredentialsWithResponse(t.Context())
		if err != nil {
			t.Fatalf("request %d: %v, want the client to retry until it is answered", i, err)
		}
		if resp.JSON200 == nil || resp.JSON200.Id != alice.AccountID {
			t.Fatalf("request %d = %d %s, want alice's account", i, resp.StatusCode(), resp.Body)
		}
		answers := rec.take()
		if len(answers) < 2 {
			continue
		}
		// The client sent the request again after each 429, waiting as Retry-After asked, until the 200.
		for _, a := range answers[:len(answers)-1] {
			if a.status != http.StatusTooManyRequests || a.retryAfter == "" {
				t.Errorf("request %d: answers = %+v, want 429s with Retry-After before the last", i, answers)
			}
		}
		if last := answers[len(answers)-1]; last.status != http.StatusOK {
			t.Errorf("request %d: last answer = %+v, want 200", i, last)
		}
		return
	}
	t.Fatal("40 requests never reached the rate limit")
}
