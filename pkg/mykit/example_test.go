// SPDX-License-Identifier: MIT

package mykit_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/mycroblog/my/pkg/mykit"
)

// fakeServer answers like the server would, so that the examples below run without one.
func fakeServer(h http.HandlerFunc) *httptest.Server { return httptest.NewServer(h) }

func writeJSON(w http.ResponseWriter, status int, contentType, body string) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// A client posts a status. The library adds an Idempotency-Key that it keeps for every retry, so a post that is
// sent again after a 429 or a 503 is applied once.
func ExampleNew() {
	srv := fakeServer(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.Method, r.URL.Path, "Idempotency-Key set:", r.Header.Get("Idempotency-Key") != "")
		writeJSON(w, http.StatusOK, "application/json", `{"id":"01890a5d-ac96-774b-bcce-b302099a8057"}`)
	})
	defer srv.Close()

	api, err := mykit.New(mykit.Config{
		Server:         srv.URL, // the origin, such as "https://example.com"
		TokenSource:    mykit.StaticToken("access token"),
		AcceptLanguage: "en",
		UserAgent:      "mybot/1.0",
	})
	if err != nil {
		panic(err)
	}
	resp, err := api.CreateStatusWithResponse(context.Background(), &mykit.CreateStatusParams{}, mykit.CreateStatusRequest{
		Text:       "Hello, fediverse!",
		Visibility: mykit.VisibilityPublic,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("posted", resp.JSON200.Id)
	// Output:
	// POST /api/v1/statuses Idempotency-Key set: true
	// posted 01890a5d-ac96-774b-bcce-b302099a8057
}

// An answer with a status of 400 or more is an error that errors.As turns into a *mykit.Problem. Branch on its
// Type, never on its Title or Detail, which the server localizes.
func ExampleProblem() {
	srv := fakeServer(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusForbidden, "application/problem+json",
			`{"type":"/api/v1/problems/insufficient-scope","title":"Insufficient scope","status":403}`)
	})
	defer srv.Close()

	api, err := mykit.New(mykit.Config{Server: srv.URL, TokenSource: mykit.StaticToken("access token")})
	if err != nil {
		panic(err)
	}
	// The admin operations need a first-party-only scope, which a third-party token never has.
	_, err = api.ListAdminAccountsWithResponse(context.Background(), &mykit.ListAdminAccountsParams{})
	var problem *mykit.Problem
	var network *mykit.NetworkError
	switch {
	case errors.As(err, &problem):
		fmt.Println(problem.Status, problem.Type)
	case errors.As(err, &network):
		fmt.Println("no answer:", network.Err)
	}
	// Output: 403 /api/v1/problems/insufficient-scope
}

// All follows next_cursor page by page, fetching the next page only when the loop needs it.
func ExampleAll() {
	srv := fakeServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") == "" {
			writeJSON(w, http.StatusOK, "application/json", `{"items":[{"id":"1"},{"id":"2"}],"next_cursor":"page-2"}`)
			return
		}
		writeJSON(w, http.StatusOK, "application/json", `{"items":[{"id":"3"}]}`)
	})
	defer srv.Close()

	api, err := mykit.New(mykit.Config{Server: srv.URL, TokenSource: mykit.StaticToken("access token")})
	if err != nil {
		panic(err)
	}
	fetch := func(ctx context.Context, cursor *mykit.Cursor) ([]mykit.Status, *mykit.Cursor, error) {
		resp, err := api.GetHomeTimelineWithResponse(ctx, &mykit.GetHomeTimelineParams{Cursor: cursor})
		if err != nil {
			return nil, nil, err
		}
		if resp.JSON200 == nil {
			return nil, nil, fmt.Errorf("unexpected status %d", resp.StatusCode())
		}
		return resp.JSON200.Items, resp.JSON200.NextCursor, nil
	}
	for status, err := range mykit.All(context.Background(), fetch) {
		if err != nil {
			panic(err)
		}
		fmt.Println(status.Id)
	}
	// Output:
	// 1
	// 2
	// 3
}

// The authorization code flow with PKCE: send the user to the authorization endpoint with the challenge, then
// exchange the code that the redirect brings, together with the verifier, for the tokens.
func ExampleOAuthConfig_Exchange() {
	srv := fakeServer(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fmt.Println(r.URL.Path, r.PostForm.Get("grant_type"), r.PostForm.Get("code_verifier") != "")
		writeJSON(w, http.StatusOK, "application/json",
			`{"access_token":"at","token_type":"Bearer","expires_in":3600,"refresh_token":"rt","scope":"read write"}`)
	})
	defer srv.Close()

	cfg := mykit.OAuthConfig{
		Server:      srv.URL,
		ClientID:    "https://app.example/client.json", // a Client ID Metadata Document URL, or a registered client_id
		RedirectURI: "http://127.0.0.1:8123/callback",
	}
	verifier := mykit.NewCodeVerifier()
	authorize := url.Values{
		"response_type":         {"code"},
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {cfg.RedirectURI},
		"scope":                 {"read write"},
		"code_challenge":        {mykit.CodeChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	authorizeURL := cfg.Server + "/oauth/authorize?" + authorize.Encode()
	_ = authorizeURL // open this URL in the user's browser; the redirect brings the code

	tok, err := cfg.Exchange(context.Background(), "the code from the redirect", verifier)
	if err != nil {
		panic(err)
	}
	fmt.Println(tok.Scope, tok.RefreshToken)
	// Output:
	// /oauth/token authorization_code true
	// read write rt
}

// The challenge of the verifier in RFC 7636 appendix B.
func ExampleCodeChallenge() {
	fmt.Println(mykit.CodeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"))
	// Output: E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM
}

// A RefreshingTokenSource renews the access token when it has expired or the server refuses it. The server
// rotates the refresh token on every use, so onTokens must save the new tokens: a lost one ends the session.
func ExampleNewRefreshingTokenSource() {
	srv := fakeServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == mykit.TokenPath {
			writeJSON(w, http.StatusOK, "application/json",
				`{"access_token":"new access","token_type":"Bearer","expires_in":3600,"refresh_token":"new refresh"}`)
			return
		}
		fmt.Println("Authorization:", r.Header.Get("Authorization"))
		writeJSON(w, http.StatusOK, "application/json", `{"items":[]}`)
	})
	defer srv.Close()

	cfg := mykit.OAuthConfig{Server: srv.URL, ClientID: "client"}
	saved := &mykit.Token{RefreshToken: "old refresh"} // read from your own storage; the access token is gone
	tokens := mykit.NewRefreshingTokenSource(cfg, saved, func(_ context.Context, t *mykit.Token) error {
		fmt.Println("save", t.RefreshToken) // write t to your own storage
		return nil
	})
	api, err := mykit.New(mykit.Config{Server: srv.URL, TokenSource: tokens})
	if err != nil {
		panic(err)
	}
	if _, err := api.GetHomeTimelineWithResponse(context.Background(), &mykit.GetHomeTimelineParams{}); err != nil {
		panic(err)
	}
	// Output:
	// save new refresh
	// Authorization: Bearer new access
}

// UploadMedia streams a file from an io.Reader. The answer is usually still processing: poll GetMediaWithResponse
// until it is ready.
func ExampleAPI_UploadMedia() {
	srv := fakeServer(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.Method, r.URL.Path)
		writeJSON(w, http.StatusAccepted, "application/json", `{"id":"01890a5d-ac96-774b-bcce-b302099a8057"}`)
	})
	defer srv.Close()

	api, err := mykit.New(mykit.Config{Server: srv.URL, TokenSource: mykit.StaticToken("access token")})
	if err != nil {
		panic(err)
	}
	media, err := api.UploadMedia(context.Background(), mykit.MediaUploadInput{
		File:        mykit.MediaFile{Name: "photo.jpg", ContentType: "image/jpeg", Body: strings.NewReader("image bytes")},
		Description: "A photo of a cat",
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("uploaded", media.Id)
	// Output:
	// POST /api/v1/media
	// uploaded 01890a5d-ac96-774b-bcce-b302099a8057
}

// Stream keeps the event stream open until the context ends, and tells every reconnection, after which the
// events missed in between must be read from the timelines and the notifications.
func ExampleAPI_Stream() {
	api, err := mykit.New(mykit.Config{Server: "https://example.com", TokenSource: mykit.StaticToken("access token")})
	if err != nil {
		log.Fatal(err)
	}
	err = api.Stream(context.Background(), mykit.StreamOptions{
		OnOpen: func(reconnected bool) {
			if reconnected {
				fmt.Println("reconnected: read the home timeline and the notifications again")
			}
		},
		OnEvent: func(e mykit.StreamEvent) {
			switch e := e.(type) {
			case *mykit.UpdateEvent:
				fmt.Println("new post", e.Status.Id)
			case *mykit.NotificationEvent:
				fmt.Println("notification", e.Notification.Type)
			case *mykit.AnnouncementEvent, *mykit.AnnouncementRemovedEvent:
				fmt.Println("the announcements changed")
			case *mykit.UnknownEvent:
				// A newer server sent an event this library does not know.
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
