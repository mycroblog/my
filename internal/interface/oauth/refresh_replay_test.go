// SPDX-License-Identifier: AGPL-3.0-only

package oauth_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/interface/oauth"
)

// replayingIdentity answers every refresh with the pair of an earlier rotation, as the service
// does for the refresh token that rotation replaced (ADR-0090 §18).
type replayingIdentity struct {
	*fakeIdentity
	pair appidentity.IssuedTokens
}

func (r replayingIdentity) RefreshAccessToken(_ context.Context, in appidentity.RefreshRequest) (appidentity.IssuedTokens, error) {
	r.refreshes = append(r.refreshes, in)
	return r.pair, nil
}

func (r replayingIdentity) RefreshWebClient(_ context.Context, rt string) (appidentity.IssuedTokens, error) {
	r.web = append(r.web, rt)
	return r.pair, nil
}

// replayed is the successor pair of a rotation, replayed 40 minutes after it was issued.
func replayed() appidentity.IssuedTokens {
	p := issued("my_rt_successor")
	p.AccessToken = "my_at_successor"
	p.ReplayedAt = p.Token.IssuedAt.Add(40 * time.Minute)
	return p
}

func newReplayingServer(t *testing.T) http.Handler {
	t.Helper()
	h, err := oauth.New(oauth.Options{
		Identity:  replayingIdentity{fakeIdentity: &fakeIdentity{}, pair: replayed()},
		WebOrigin: "https://web.example", Origin: "https://social.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	m := mux{http.NewServeMux()}
	h.Register(m)
	return m
}

// The web client that lost a refresh response gets the successor's cookie again, and an access
// token that expires when the successor's does.
func TestRefreshEndpointSetsTheReplayedSuccessorsCookie(t *testing.T) {
	h := newReplayingServer(t)
	w := post(h, oauth.PathRefresh, nil, map[string]string{"Cookie": oauth.CookieRefreshToken + "=my_rt_previous", oauth.HeaderRefresh: "1"})
	if w.Code != http.StatusOK {
		t.Fatalf("refresh = %d %s", w.Code, w.Body.String())
	}
	body := decode(t, w)
	if body["access_token"] != "my_at_successor" || body["expires_in"] != float64(20*60) || body["refresh_token"] != nil {
		t.Errorf("refresh body = %v, want the successor's access token expiring in 1200 s", body)
	}
	if c := refreshCookie(w); c == nil || c.Value != "my_rt_successor" || c.MaxAge < 0 || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie = %+v, want the successor's refresh token", c)
	}
}

// An app that lost a refresh response gets the successor pair again in the body.
func TestTokenEndpointReturnsTheReplayedSuccessorPair(t *testing.T) {
	h := newReplayingServer(t)
	form := url.Values{"grant_type": {"refresh_token"}, "client_id": {"app"}, "refresh_token": {"my_rt_previous"}}
	w := post(h, oauth.PathToken, form, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("token = %d %s", w.Code, w.Body.String())
	}
	body := decode(t, w)
	if body["access_token"] != "my_at_successor" || body["refresh_token"] != "my_rt_successor" || body["expires_in"] != float64(20*60) {
		t.Errorf("token body = %v, want the successor pair expiring in 1200 s", body)
	}
	if refreshCookie(w) != nil {
		t.Error("an app's replayed pair set the web client's cookie")
	}
}
