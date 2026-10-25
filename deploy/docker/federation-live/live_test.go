// SPDX-License-Identifier: AGPL-3.0-only

//go:build federationlive

// Package federationlive holds the scenarios of the live interoperability test (federation.md §12):
// black-box checks, over HTTPS through the proxy of compose.yaml, that this server federates with
// Mastodon, GoToSocial, and Misskey. scripts/federation-live.sh starts the stack and runs them; without the
// FEDERATION_LIVE_* variables that it sets, every test skips.
package federationlive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// The host names and users of compose.yaml and scripts/federation-live.sh.
const (
	localHost    = "my.test"
	localUser    = "alice"
	mastodonHost = "mastodon.test"
	mastodonUser = "bob"
	gtsHost      = "gts.test"
	gtsUser      = "carol"
	misskeyHost  = "misskey.test"
	misskeyUser  = "grace"

	// The users who never follow alice: the mentioned non-followers of TestVisibilityAudiences.
	mastodonStranger = "dave"
	gtsStranger      = "erin"
	misskeyStranger  = "heidi"
	// mastodonSigner is the user of TestSignatureProfile, whose draft-cavage requests the proxy
	// refuses in both directions (Caddyfile).
	mastodonSigner = "eve"

	// deliveryTimeout bounds the wait for an asynchronous delivery (queues, retries) to land.
	deliveryTimeout = 90 * time.Second
	// quietPeriod is how long a delivery that must not happen is waited for.
	quietPeriod = 15 * time.Second
)

// env is the stack that scripts/federation-live.sh started.
type env struct {
	http  *http.Client
	db    string
	local *localClient
	peers []*peerClient
}

var (
	setupOnce sync.Once
	setupEnv  *env
	setupErr  error
)

// stack returns the running stack, or skips the test when the variables of
// scripts/federation-live.sh are not set.
func stack(t *testing.T) *env {
	t.Helper()
	if os.Getenv("FEDERATION_LIVE_ADDR") == "" {
		t.Skip("FEDERATION_LIVE_ADDR is not set; run scripts/federation-live.sh")
	}
	setupOnce.Do(func() { setupEnv, setupErr = setup() })
	if setupErr != nil {
		t.Fatalf("setting up the clients: %v", setupErr)
	}
	return setupEnv
}

func setup() (*env, error) {
	pem, err := os.ReadFile(os.Getenv("FEDERATION_LIVE_CA")) //nolint:gosec // G703: the path of the CA that scripts/federation-live.sh generated
	if err != nil {
		return nil, fmt.Errorf("reading FEDERATION_LIVE_CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("FEDERATION_LIVE_CA holds no certificate")
	}
	proxy := os.Getenv("FEDERATION_LIVE_ADDR")
	var dialer net.Dialer
	transport := &http.Transport{
		// Every host of the stack is behind the proxy; the TLS handshake still names the host.
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			switch host {
			case localHost, mastodonHost, gtsHost, misskeyHost:
				return dialer.DialContext(ctx, network, proxy)
			}
			return nil, fmt.Errorf("%s is not a host of the stack", host)
		},
		TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
	}
	e := &env{
		http: &http.Client{Transport: transport, Timeout: 60 * time.Second},
		// The role and database of postgres-init.sh, on the port that compose.yaml publishes.
		db: "postgres://my:my@127.0.0.1:" + os.Getenv("FEDERATION_LIVE_DB_PORT") + "/my?sslmode=disable",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if e.local, err = signInLocal(ctx, transport, os.Getenv("FEDERATION_LIVE_MY_PASSWORD")); err != nil {
		return nil, fmt.Errorf("signing in to %s: %w", localHost, err)
	}
	gts := map[string]string{}
	for _, user := range []string{gtsUser, gtsStranger} {
		token, err := signInGoToSocial(ctx, transport, user, os.Getenv("FEDERATION_LIVE_GTS_PASSWORD"))
		if err != nil {
			return nil, fmt.Errorf("signing in to %s as %s: %w", gtsHost, user, err)
		}
		gts[user] = token
	}
	mastodon := func(user, token string) *peerClient {
		return &peerClient{name: "mastodon", host: mastodonHost, user: user,
			peerAPI: &mastodonAPI{host: mastodonHost, token: token, http: e.http}}
	}
	gotosocial := func(user string) *peerClient {
		return &peerClient{name: "gotosocial", host: gtsHost, user: user,
			peerAPI: &mastodonAPI{host: gtsHost, token: gts[user], http: e.http, cachedHome: true}}
	}
	misskey := func(user, token string) *peerClient {
		return &peerClient{name: "misskey", host: misskeyHost, user: user,
			peerAPI: &misskeyAPI{host: misskeyHost, token: token, http: e.http}, noEdits: misskeyNoEdits,
			favouriteReaction: misskeyReaction}
	}
	m := mastodon(mastodonUser, os.Getenv("FEDERATION_LIVE_MASTODON_TOKEN"))
	m.stranger = mastodon(mastodonStranger, os.Getenv("FEDERATION_LIVE_MASTODON_TOKEN_DAVE"))
	m.signer = mastodon(mastodonSigner, os.Getenv("FEDERATION_LIVE_MASTODON_TOKEN_EVE"))
	g := gotosocial(gtsUser)
	g.stranger = gotosocial(gtsStranger)
	k := misskey(misskeyUser, os.Getenv("FEDERATION_LIVE_MISSKEY_TOKEN"))
	k.stranger = misskey(misskeyStranger, os.Getenv("FEDERATION_LIVE_MISSKEY_TOKEN_HEIDI"))
	e.peers = []*peerClient{m, g, k}
	// GoToSocial creates accounts that approve their followers by hand; the scenarios that need an
	// approval lock the account themselves (TestFollowApproval).
	for _, p := range []*peerClient{g, g.stranger} {
		if err := p.setLocked(ctx, false); err != nil {
			return nil, fmt.Errorf("unlocking %s: %w", p.handle(), err)
		}
	}
	return e, nil
}

// eventually calls check until it reports done or the timeout passes, then fails with its last
// description of the state.
func eventually(t *testing.T, timeout time.Duration, what string, check func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		done, state := check()
		if done {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: not after %s; last state: %s", what, timeout, state)
		}
		time.Sleep(2 * time.Second)
	}
}

// unique returns text that no earlier run posted.
func unique(prefix string) string {
	return fmt.Sprintf("%s %d", prefix, time.Now().UnixNano())
}

// --- HTTP -------------------------------------------------------------------------------------

// apiError is a response other than the expected status.
type apiError struct {
	method, url string
	status      int
	body        string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s %s = %d: %s", e.method, e.url, e.status, e.body)
}

// call sends a request with a bearer token and decodes a 2xx JSON response into out (if not nil).
func call(ctx context.Context, c *http.Client, token, method, rawURL string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, r)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return &apiError{method: method, url: rawURL, status: resp.StatusCode, body: string(b)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("%s %s: decoding %q: %w", method, rawURL, b, err)
	}
	return nil
}

// browser is an HTTP client with cookies that does not follow redirects, for sign-in forms.
type browser struct {
	c *http.Client
}

func newBrowser(transport http.RoundTripper) (*browser, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &browser{c: &http.Client{
		Transport: transport, Jar: jar, Timeout: 60 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

// do sends a GET (form == nil) or a form POST and returns the status, the Location, and the body.
func (b *browser) do(ctx context.Context, method, rawURL string, form url.Values) (int, string, string, error) {
	var r io.Reader
	if form != nil {
		r = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, r)
	if err != nil {
		return 0, "", "", err
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	resp, err := b.c.Do(req)
	if err != nil {
		return 0, "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", "", err
	}
	location := resp.Header.Get("Location")
	if location != "" {
		if u, err := resp.Request.URL.Parse(location); err == nil {
			location = u.String()
		}
	}
	return resp.StatusCode, location, string(body), nil
}

// --- Database of this server ------------------------------------------------------------------

// receivedActivity is a row of received_activities (data-model §4.8).
type receivedActivity struct {
	URI, Type, SignatureScheme, Trust string
}

// receivedActivities returns what this server recorded from the actor, oldest first.
func (e *env) receivedActivities(t *testing.T, actorURI string) []receivedActivity {
	t.Helper()
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, e.db)
	if err != nil {
		t.Fatalf("connecting to the database of %s: %v", localHost, err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	rows, err := conn.Query(ctx, `
		SELECT activity_uri, type, signature_scheme, trust FROM received_activities
		WHERE actor_uri = $1 ORDER BY received_at, activity_uri`, actorURI)
	if err != nil {
		t.Fatalf("reading received_activities: %v", err)
	}
	var out []receivedActivity
	for rows.Next() {
		var a receivedActivity
		if err := rows.Scan(&a.URI, &a.Type, &a.SignatureScheme, &a.Trust); err != nil {
			t.Fatalf("reading received_activities: %v", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading received_activities: %v", err)
	}
	return out
}

// deliveryAttempt is a row of delivery_attempts with the inbox of its delivery (data-model §4.8).
type deliveryAttempt struct {
	Inbox, Outcome, SignatureScheme string
	HTTPStatus                      int
}

// deliveryAttempts returns every attempt this server made to deliver to the inbox, oldest first.
func (e *env) deliveryAttempts(t *testing.T, inbox string) []deliveryAttempt {
	t.Helper()
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, e.db)
	if err != nil {
		t.Fatalf("connecting to the database of %s: %v", localHost, err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	rows, err := conn.Query(ctx, `
		SELECT d.inbox_url, a.outcome, COALESCE(a.signature_scheme, ''), COALESCE(a.http_status, 0)
		FROM delivery_attempts a JOIN deliveries d ON d.id = a.delivery_id
		WHERE d.inbox_url = $1 ORDER BY a.attempted_at, a.id`, inbox)
	if err != nil {
		t.Fatalf("reading delivery_attempts: %v", err)
	}
	var out []deliveryAttempt
	for rows.Next() {
		var a deliveryAttempt
		if err := rows.Scan(&a.Inbox, &a.Outcome, &a.SignatureScheme, &a.HTTPStatus); err != nil {
			t.Fatalf("reading delivery_attempts: %v", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading delivery_attempts: %v", err)
	}
	return out
}

// --- This server (its own client API) ---------------------------------------------------------

type localClient struct {
	http  *http.Client
	token string
}

var csrfToken = regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)

// signInLocal runs the web client's authorization code flow with PKCE (oauth_test.go in
// internal/platform does the same) as alice and returns a client with her access token.
func signInLocal(ctx context.Context, transport http.RoundTripper, password string) (*localClient, error) {
	return signInLocalAs(ctx, transport, localUser, password)
}

// signInLocalAs is signInLocal as user.
func signInLocalAs(ctx context.Context, transport http.RoundTripper, user, password string) (*localClient, error) {
	b, err := newBrowser(transport)
	if err != nil {
		return nil, err
	}
	base := "https://" + localHost
	redirect := base + "/login/callback"
	verifier := strings.Repeat("v", 43)
	authorize := base + "/oauth/authorize?" + url.Values{
		"response_type": {"code"}, "client_id": {"my-web"}, "redirect_uri": {redirect},
		"state": {"live"}, "code_challenge": {s256(verifier)}, "code_challenge_method": {"S256"},
	}.Encode()

	status, location, _, err := b.do(ctx, http.MethodGet, authorize, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusSeeOther {
		return nil, fmt.Errorf("authorize without a session = %d", status)
	}
	_, _, page, err := b.do(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}
	m := csrfToken.FindStringSubmatch(page)
	if m == nil {
		return nil, errors.New("the sign-in screen has no csrf_token")
	}
	returnTo, err := url.Parse(location)
	if err != nil {
		return nil, err
	}
	status, _, page, err = b.do(ctx, http.MethodPost, base+"/auth/sign_in", url.Values{
		"csrf_token": {m[1]}, "email": {user + "@" + localHost}, "password": {password},
		"return_to": {returnTo.Query().Get("return_to")},
	})
	if err != nil {
		return nil, err
	}
	if status != http.StatusSeeOther {
		return nil, fmt.Errorf("sign-in = %d: %s", status, page)
	}
	status, location, _, err = b.do(ctx, http.MethodGet, authorize, nil)
	if err != nil {
		return nil, err
	}
	back, err := url.Parse(location)
	if err != nil || status != http.StatusSeeOther || back.Query().Get("code") == "" {
		return nil, fmt.Errorf("authorize after the sign-in = %d → %q", status, location)
	}
	var tokens struct {
		AccessToken string `json:"access_token"`
	}
	status, _, page, err = b.do(ctx, http.MethodPost, base+"/oauth/token", url.Values{
		"grant_type": {"authorization_code"}, "client_id": {"my-web"}, "code": {back.Query().Get("code")},
		"redirect_uri": {redirect}, "code_verifier": {verifier},
	})
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(page), &tokens); err != nil || status != http.StatusOK || tokens.AccessToken == "" {
		return nil, fmt.Errorf("token exchange = %d: %s", status, page)
	}
	return &localClient{http: &http.Client{Transport: transport, Timeout: 60 * time.Second}, token: tokens.AccessToken}, nil
}

// s256 is the PKCE S256 challenge of a verifier (RFC 7636 §4.2).
func s256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// account is the part of an account that the scenarios read, in this server's client API and in
// the Mastodon client API (Misskey's users are converted to it, misskey_test.go).
type account struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Acct     string `json:"acct"`
	URI      string `json:"uri"`
	URL      string `json:"url"`
}

// relationship is the part of a relationship that the scenarios read (both APIs).
type relationship struct {
	ID         string `json:"id"`
	Following  bool   `json:"following"`
	Requested  bool   `json:"requested"`
	FollowedBy bool   `json:"followed_by"`
}

// status is the part of a status that the scenarios read (both APIs; the boost count is
// `boosts_count` here and `reblogs_count` in the Mastodon client API).
type status struct {
	ID              string `json:"id"`
	URI             string `json:"uri"`
	Content         string `json:"content"`
	Visibility      string `json:"visibility"`
	InReplyToID     string `json:"in_reply_to_id"`
	BoostsCount     int    `json:"boosts_count"`
	ReblogsCount    int    `json:"reblogs_count"`
	FavouritesCount int    `json:"favourites_count"`
	// Reactions are the emoji reactions, counted per emoji (this server's client API and
	// Misskey's).
	Reactions []statusReaction `json:"reactions"`
}

// statusReaction is the number of the reactions to a status with one emoji (this server's
// client API).
type statusReaction struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// reactionCount returns the number of the reactions to s with the emoji.
func (s status) reactionCount(emoji string) int {
	for _, r := range s.Reactions {
		if r.Name == emoji {
			return r.Count
		}
	}
	return 0
}

// thread is a status context (both APIs).
type thread struct {
	Ancestors   []status `json:"ancestors"`
	Descendants []status `json:"descendants"`
}

// notification is the part of a notification that the scenarios read (both APIs).
type notification struct {
	Type    string  `json:"type"`
	Account account `json:"account"`
	Status  *status `json:"status"`
	// Reaction is the emoji of a notification of type "reaction" (this server's client API only).
	Reaction *struct {
		Name string `json:"name"`
	} `json:"reaction"`
	// AccountMove is the move of an "account_moved" notification (this server's client API only).
	AccountMove *struct {
		Target  account `json:"target"`
		Outcome string  `json:"outcome"`
	} `json:"account_move"`
}

// isNotFound reports whether err is a 404 response.
func isNotFound(err error) bool {
	var ae *apiError
	return errors.As(err, &ae) && ae.status == http.StatusNotFound
}

// findStatus returns the status of the list with the URI.
func findStatus(items []status, uri string) (status, bool) {
	for _, s := range items {
		if s.URI == uri {
			return s, true
		}
	}
	return status{}, false
}

func (l *localClient) call(ctx context.Context, method, path string, body, out any) error {
	return call(ctx, l.http, l.token, method, "https://"+localHost+"/api/v1"+path, body, out)
}

// resolve looks up a handle, fetching it from its server if needed (FR-305).
func (l *localClient) resolve(ctx context.Context, handle string) (account, error) {
	var a account
	err := l.call(ctx, http.MethodGet, "/accounts/lookup?"+url.Values{"acct": {handle}, "resolve": {"true"}}.Encode(), nil, &a)
	return a, err
}

func (l *localClient) follow(ctx context.Context, id string) (relationship, error) {
	var r relationship
	err := l.call(ctx, http.MethodPost, "/accounts/"+url.PathEscape(id)+"/follow", nil, &r)
	return r, err
}

func (l *localClient) relationship(ctx context.Context, id string) (relationship, error) {
	var rs []relationship
	if err := l.call(ctx, http.MethodGet, "/accounts/relationships?"+url.Values{"id": {id}}.Encode(), nil, &rs); err != nil {
		return relationship{}, err
	}
	if len(rs) != 1 {
		return relationship{}, fmt.Errorf("%d relationships with %s", len(rs), id)
	}
	return rs[0], nil
}

func (l *localClient) unfollow(ctx context.Context, id string) error {
	return l.call(ctx, http.MethodPost, "/accounts/"+url.PathEscape(id)+"/unfollow", nil, nil)
}

// setLocked turns the approval of follow requests on or off (update_credentials).
func (l *localClient) setLocked(ctx context.Context, locked bool) error {
	return l.call(ctx, http.MethodPatch, "/accounts/update_credentials", map[string]any{"locked": locked}, nil)
}

// followRequests returns the accounts whose follow requests wait for alice's approval.
func (l *localClient) followRequests(ctx context.Context) ([]account, error) {
	var page struct {
		Accounts []account `json:"accounts"`
	}
	err := l.call(ctx, http.MethodGet, "/follow_requests", nil, &page)
	return page.Accounts, err
}

func (l *localClient) authorize(ctx context.Context, id string) error {
	return l.call(ctx, http.MethodPost, "/follow_requests/"+url.PathEscape(id)+"/authorize", nil, nil)
}

func (l *localClient) post(ctx context.Context, text, visibility string) (status, error) {
	return l.reply(ctx, text, visibility, "")
}

// reply posts a status; inReplyTo is the ID of the status it replies to, or empty.
func (l *localClient) reply(ctx context.Context, text, visibility, inReplyTo string) (status, error) {
	body := map[string]any{"text": text, "visibility": visibility}
	if inReplyTo != "" {
		body["in_reply_to_id"] = inReplyTo
	}
	var s status
	err := l.call(ctx, http.MethodPost, "/statuses", body, &s)
	return s, err
}

func (l *localClient) status(ctx context.Context, id string) (status, error) {
	var s status
	err := l.call(ctx, http.MethodGet, "/statuses/"+url.PathEscape(id), nil, &s)
	return s, err
}

func (l *localClient) context(ctx context.Context, id string) (thread, error) {
	var th thread
	err := l.call(ctx, http.MethodGet, "/statuses/"+url.PathEscape(id)+"/context", nil, &th)
	return th, err
}

func (l *localClient) edit(ctx context.Context, id, text string) (status, error) {
	var s status
	err := l.call(ctx, http.MethodPut, "/statuses/"+url.PathEscape(id), map[string]any{"text": text}, &s)
	return s, err
}

func (l *localClient) delete(ctx context.Context, id string) error {
	return l.call(ctx, http.MethodDelete, "/statuses/"+url.PathEscape(id), nil, nil)
}

// interact boosts or favourites a status (action "boost" / "favourite"), or undoes it.
func (l *localClient) interact(ctx context.Context, id, action string, undo bool) error {
	method := http.MethodPost
	if undo {
		method = http.MethodDelete
	}
	return l.call(ctx, method, "/statuses/"+url.PathEscape(id)+"/"+action, nil, nil)
}

// react sets the user's emoji reaction to a status, or takes it back when emoji is "".
func (l *localClient) react(ctx context.Context, id, emoji string) error {
	path := "/statuses/" + url.PathEscape(id) + "/reaction"
	if emoji == "" {
		return l.call(ctx, http.MethodDelete, path, nil, nil)
	}
	return l.call(ctx, http.MethodPut, path, map[string]any{"emoji": emoji}, nil)
}

func (l *localClient) home(ctx context.Context) ([]status, error) {
	var page struct {
		Items []status `json:"items"`
	}
	err := l.call(ctx, http.MethodGet, "/timelines/home", nil, &page)
	return page.Items, err
}

func (l *localClient) notifications(ctx context.Context) ([]notification, error) {
	var page struct {
		Items []notification `json:"items"`
	}
	err := l.call(ctx, http.MethodGet, "/notifications", nil, &page)
	return page.Items, err
}

// --- Peers ------------------------------------------------------------------------------------

// peerClient is a user of a peer server, driven through that server's own client API (peerAPI).
type peerClient struct {
	name, host, user string
	peerAPI
	// stranger is another user of the peer who never follows alice (TestVisibilityAudiences).
	stranger *peerClient
	// signer is the user of TestSignatureProfile (Mastodon only).
	signer *peerClient
	// noEdits, when set, says why the peer neither edits its posts nor applies the edits it
	// receives (TestEdit).
	noEdits string
	// favouriteReaction, when set, is the emoji of the peer's favourite: the peer sends it as a
	// Like with the emoji (_misskey_reaction), which this server records as an emoji reaction,
	// not as a favourite (ADR-0052, TestFavourite).
	favouriteReaction string
}

// handle is the peer user's handle.
func (p *peerClient) handle() string { return p.user + "@" + p.host }

func (p *peerClient) post(ctx context.Context, text, visibility string) (status, error) {
	return p.reply(ctx, text, visibility, "")
}

// peerAPI is what the scenarios do as a peer user, in the terms of the Mastodon client API: IDs are
// the peer's own, and statuses, accounts, and visibilities are converted to the types above.
type peerAPI interface {
	// resolve finds a handle, making the peer fetch it through WebFinger if needed.
	resolve(ctx context.Context, handle string) (account, error)
	// resolveStatus finds a status by its URI, making the peer fetch it if needed; it fails when
	// the user cannot see the status.
	resolveStatus(ctx context.Context, uri string) (status, error)
	follow(ctx context.Context, id string) (relationship, error)
	unfollow(ctx context.Context, id string) error
	relationship(ctx context.Context, id string) (relationship, error)
	// setLocked turns the approval of follow requests on or off.
	setLocked(ctx context.Context, locked bool) error
	// followRequests returns the accounts whose follow requests wait for the user's approval.
	followRequests(ctx context.Context) ([]account, error)
	authorize(ctx context.Context, id string) error
	// reply posts a status; inReplyTo is the peer's ID of the status it replies to, or empty.
	reply(ctx context.Context, text, visibility, inReplyTo string) (status, error)
	status(ctx context.Context, id string) (status, error)
	context(ctx context.Context, id string) (thread, error)
	edit(ctx context.Context, id, text string) (status, error)
	delete(ctx context.Context, id string) error
	// interact boosts or favourites a status (action "boost" / "favourite"), or undoes it.
	interact(ctx context.Context, id, action string, undo bool) error
	home(ctx context.Context) ([]status, error)
	// notifications returns the user's notifications, including the filtered ones.
	notifications(ctx context.Context) ([]notification, error)
	// statusesOf returns the statuses of an account, by the peer's ID of it (TestFederationMode).
	statusesOf(ctx context.Context, id string) ([]status, error)
	// followers returns the accounts that follow the user (TestFederationMode).
	followers(ctx context.Context) ([]account, error)
	// refetchActor makes the peer fetch a remote actor again, by its URI or the peer's ID of it
	// (TestFederationMode).
	refetchActor(ctx context.Context, uri, id string) error
	// gone reads whether the peer shows an account, by the peer's ID, as gone: suspended, deleted,
	// or missing (TestFederationMode).
	gone(ctx context.Context, id string) (bool, string, error)
}

// --- Mastodon and GoToSocial (the Mastodon client API) ----------------------------------------

type mastodonAPI struct {
	host, token string
	http        *http.Client
	// cachedHome is set for GoToSocial, whose first page of the home timeline comes from a cache
	// that does not always take statuses that arrive after it was filled.
	cachedHome bool
}

func (p *mastodonAPI) call(ctx context.Context, method, path string, body, out any) error {
	return call(ctx, p.http, p.token, method, "https://"+p.host+path, body, out)
}

// resolve searches a handle with resolve=true, which makes the peer fetch it through WebFinger.
func (p *mastodonAPI) resolve(ctx context.Context, handle string) (account, error) {
	var res struct {
		Accounts []account `json:"accounts"`
	}
	q := url.Values{"q": {"@" + handle}, "resolve": {"true"}, "type": {"accounts"}, "limit": {"1"}}
	if err := p.call(ctx, http.MethodGet, "/api/v2/search?"+q.Encode(), nil, &res); err != nil {
		return account{}, err
	}
	if len(res.Accounts) == 0 {
		return account{}, fmt.Errorf("%s found no account for %s", p.host, handle)
	}
	return res.Accounts[0], nil
}

// resolveStatus searches a status URI with resolve=true, which makes the peer fetch it from this
// server with a signed GET.
func (p *mastodonAPI) resolveStatus(ctx context.Context, uri string) (status, error) {
	var res struct {
		Statuses []status `json:"statuses"`
	}
	q := url.Values{"q": {uri}, "resolve": {"true"}, "type": {"statuses"}, "limit": {"1"}}
	if err := p.call(ctx, http.MethodGet, "/api/v2/search?"+q.Encode(), nil, &res); err != nil {
		return status{}, err
	}
	if len(res.Statuses) == 0 {
		return status{}, fmt.Errorf("%s found no status for %s", p.host, uri)
	}
	return res.Statuses[0], nil
}

func (p *mastodonAPI) follow(ctx context.Context, id string) (relationship, error) {
	var r relationship
	err := p.call(ctx, http.MethodPost, "/api/v1/accounts/"+url.PathEscape(id)+"/follow", nil, &r)
	return r, err
}

func (p *mastodonAPI) unfollow(ctx context.Context, id string) error {
	return p.call(ctx, http.MethodPost, "/api/v1/accounts/"+url.PathEscape(id)+"/unfollow", nil, nil)
}

func (p *mastodonAPI) relationship(ctx context.Context, id string) (relationship, error) {
	var rs []relationship
	if err := p.call(ctx, http.MethodGet, "/api/v1/accounts/relationships?"+url.Values{"id[]": {id}}.Encode(), nil, &rs); err != nil {
		return relationship{}, err
	}
	if len(rs) != 1 {
		return relationship{}, fmt.Errorf("%d relationships with %s", len(rs), id)
	}
	return rs[0], nil
}

// setLocked turns the approval of follow requests on or off.
func (p *mastodonAPI) setLocked(ctx context.Context, locked bool) error {
	return p.call(ctx, http.MethodPatch, "/api/v1/accounts/update_credentials", map[string]any{"locked": locked}, nil)
}

// followRequests returns the accounts whose follow requests wait for the peer user's approval.
func (p *mastodonAPI) followRequests(ctx context.Context) ([]account, error) {
	var as []account
	err := p.call(ctx, http.MethodGet, "/api/v1/follow_requests", nil, &as)
	return as, err
}

func (p *mastodonAPI) authorize(ctx context.Context, id string) error {
	return p.call(ctx, http.MethodPost, "/api/v1/follow_requests/"+url.PathEscape(id)+"/authorize", nil, nil)
}

func (p *mastodonAPI) reply(ctx context.Context, text, visibility, inReplyTo string) (status, error) {
	body := map[string]any{"status": text, "visibility": visibility}
	if inReplyTo != "" {
		body["in_reply_to_id"] = inReplyTo
	}
	var s status
	err := p.call(ctx, http.MethodPost, "/api/v1/statuses", body, &s)
	return s, err
}

func (p *mastodonAPI) status(ctx context.Context, id string) (status, error) {
	var s status
	err := p.call(ctx, http.MethodGet, "/api/v1/statuses/"+url.PathEscape(id), nil, &s)
	return s, err
}

func (p *mastodonAPI) context(ctx context.Context, id string) (thread, error) {
	var th thread
	err := p.call(ctx, http.MethodGet, "/api/v1/statuses/"+url.PathEscape(id)+"/context", nil, &th)
	return th, err
}

func (p *mastodonAPI) edit(ctx context.Context, id, text string) (status, error) {
	var s status
	err := p.call(ctx, http.MethodPut, "/api/v1/statuses/"+url.PathEscape(id), map[string]any{"status": text}, &s)
	return s, err
}

func (p *mastodonAPI) delete(ctx context.Context, id string) error {
	return p.call(ctx, http.MethodDelete, "/api/v1/statuses/"+url.PathEscape(id), nil, nil)
}

// interact boosts or favourites a status (action "boost" / "favourite"), or undoes it.
func (p *mastodonAPI) interact(ctx context.Context, id, action string, undo bool) error {
	verb := map[string]string{"boost": "reblog", "favourite": "favourite"}[action]
	if undo {
		verb = "un" + verb
	}
	return p.call(ctx, http.MethodPost, "/api/v1/statuses/"+url.PathEscape(id)+"/"+verb, nil, nil)
}

func (p *mastodonAPI) home(ctx context.Context) ([]status, error) {
	path := "/api/v1/timelines/home?limit=40"
	if p.cachedHome {
		// A max_id makes GoToSocial read the timeline from its database instead of its cache.
		path += "&max_id=" + strings.Repeat("Z", 26)
	}
	var ss []status
	err := p.call(ctx, http.MethodGet, path, nil, &ss)
	return ss, err
}

// notifications returns the peer user's notifications, including those that Mastodon's
// notification policy filtered (include_filtered, Mastodon 4.3 and later; GoToSocial ignores it).
func (p *mastodonAPI) notifications(ctx context.Context) ([]notification, error) {
	var ns []notification
	err := p.call(ctx, http.MethodGet, "/api/v1/notifications?limit=40&include_filtered=true", nil, &ns)
	return ns, err
}

// signInGoToSocial gets an access token for a user through GoToSocial's OAuth screens: register
// an application, sign in, approve, and exchange the out-of-band code.
func signInGoToSocial(ctx context.Context, transport http.RoundTripper, user, password string) (string, error) {
	b, err := newBrowser(transport)
	if err != nil {
		return "", err
	}
	base := "https://" + gtsHost
	const redirect = "urn:ietf:wg:oauth:2.0:oob"
	const scopes = "read write follow"
	var app struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := call(ctx, b.c, "", http.MethodPost, base+"/api/v1/apps",
		map[string]string{"client_name": "federation-live", "redirect_uris": redirect, "scopes": scopes}, &app); err != nil {
		return "", err
	}
	authorize := base + "/oauth/authorize?" + url.Values{
		"client_id": {app.ClientID}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {scopes},
	}.Encode()
	if _, _, _, err := b.do(ctx, http.MethodGet, authorize, nil); err != nil {
		return "", err
	}
	status, _, page, err := b.do(ctx, http.MethodPost, base+"/auth/sign_in", url.Values{
		"username": {user + "@" + gtsHost}, "password": {password},
	})
	if err != nil {
		return "", err
	}
	if status/100 != 3 {
		return "", fmt.Errorf("sign-in = %d: %s", status, page)
	}
	if status, _, page, err = b.do(ctx, http.MethodGet, authorize, nil); err != nil {
		return "", err
	} else if status != http.StatusOK {
		return "", fmt.Errorf("authorize after the sign-in = %d: %s", status, page)
	}
	status, location, page, err := b.do(ctx, http.MethodPost, authorize, url.Values{})
	if err != nil {
		return "", err
	}
	back, err := url.Parse(location)
	if err != nil || status/100 != 3 || back.Query().Get("code") == "" {
		return "", fmt.Errorf("approval = %d → %q: %s", status, location, page)
	}
	var tokens struct {
		AccessToken string `json:"access_token"`
	}
	if err := call(ctx, b.c, "", http.MethodPost, base+"/oauth/token", map[string]string{
		"grant_type": "authorization_code", "client_id": app.ClientID, "client_secret": app.ClientSecret,
		"redirect_uri": redirect, "code": back.Query().Get("code"),
	}, &tokens); err != nil {
		return "", err
	}
	if tokens.AccessToken == "" {
		return "", errors.New("the token response has no access_token")
	}
	return tokens.AccessToken, nil
}
