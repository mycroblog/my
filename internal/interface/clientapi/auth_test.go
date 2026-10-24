// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/pkg/testkit"
)

const aliceAccountID = "01890a5d-ac96-774b-bcce-b302099a8057"

// fakeAuthenticator knows a fixed set of tokens and records the tokens it is asked about.
type fakeAuthenticator struct {
	mu     sync.Mutex
	tokens map[string]appidentity.Bearer
	errs   map[string]error
	asked  []string
}

func (f *fakeAuthenticator) AuthenticateBearer(_ context.Context, token string) (appidentity.Bearer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, token)
	if err, ok := f.errs[token]; ok {
		return appidentity.Bearer{}, err
	}
	b, ok := f.tokens[token]
	if !ok {
		return appidentity.Bearer{}, appidentity.ErrInvalidAccessToken
	}
	return b, nil
}

type fakeAccounts map[string]appaccount.Actor

func (f fakeAccounts) FindByID(_ context.Context, id string) (appaccount.Actor, error) {
	a, ok := f[id]
	if !ok {
		return appaccount.Actor{}, appaccount.ErrNotFound
	}
	return a, nil
}

func (f fakeAccounts) FindByHandle(_ context.Context, username, domain string) (appaccount.Actor, error) {
	for _, a := range f {
		if a.Local != nil && domain == account.LocalDomain && a.Local.Username == username {
			return a, nil
		}
		if a.Handle != nil && a.Handle.Domain == domain && a.Handle.Username == username {
			return a, nil
		}
	}
	return appaccount.Actor{}, appaccount.ErrNotFound
}

func (f fakeAccounts) FindByURI(_ context.Context, uri string) (appaccount.Actor, error) {
	for _, a := range f {
		if a.Account.URI == uri {
			return a, nil
		}
	}
	return appaccount.Actor{}, appaccount.ErrNotFound
}

// fakeGraph answers the reads of the account endpoints: nobody blocks anybody, and the
// counts are fixed. The other methods are not implemented.
type fakeGraph struct{ clientapi.Graph }

func (fakeGraph) IsBlocking(context.Context, string, string) (bool, error) { return false, nil }

func (fakeGraph) Counters(_ context.Context, id string) (graph.Counters, error) {
	return graph.Counters{AccountID: id, FollowersCount: 2, FollowingCount: 1}, nil
}

// authEnv is a Server with fake tokens, accounts, and a rate limiter on a stopped clock.
type authEnv struct {
	h     http.Handler
	auth  *fakeAuthenticator
	clock *testkit.Clock
	spec  *apitest.Spec
}

var createdAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func bearer(scope identity.Scope, accountID string) appidentity.Bearer {
	return appidentity.Bearer{TokenID: testkit.NamedID("tok-" + string(scope)), FamilyID: testkit.NamedID("fam-" + string(scope)), ClientID: "client-1", ClientPublicID: "public-client-1", UserID: userOf(accountID), AccountID: accountID, Scope: scope, FirstParty: true}
}

// thirdParty returns b as the token of a client that is not this server's own.
func thirdParty(b appidentity.Bearer) appidentity.Bearer {
	b.FirstParty = false
	return b
}

func newAuthEnv(t *testing.T, tweak func(*clientapi.Options)) *authEnv {
	t.Helper()
	messages, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	e := &authEnv{
		auth: &fakeAuthenticator{
			tokens: map[string]appidentity.Bearer{
				"read":          bearer("read", aliceAccountID),
				"read-accounts": bearer("read:accounts", aliceAccountID),
				"write":         bearer("write", aliceAccountID),
				"follow-push":   bearer("follow push", aliceAccountID),
				"bob":           bearer("read", "01890a5d-ac96-774b-bcce-b302099a8058"),
				"read-statuses": bearer("read:statuses", aliceAccountID),
				"read-direct":   bearer("read:direct read:statuses", aliceAccountID),
				"third-party":   thirdParty(bearer("admin read write", aliceAccountID)),
			},
			errs: map[string]error{"pending": appidentity.ErrAccountUnavailable},
		},
		clock: testkit.NewClock(time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)),
		spec:  apitest.Load(t),
	}
	o := clientapi.Options{
		Host: "social.example", SoftwareVersion: "1.2.3",
		SourceURL:     "https://git.example/my",
		Authenticator: e.auth,
		Accounts: fakeAccounts{aliceAccountID: {
			Account:   account.Account{ID: aliceAccountID, URI: "https://social.example/ap/actors/" + aliceAccountID, CreatedAt: createdAt},
			Local:     &account.LocalAccount{AccountID: aliceAccountID, UserID: userOf(aliceAccountID), Username: "alice"},
			Profile:   &account.ProfileRevision{DisplayName: "Alice", NoteHTML: "<p>hi</p>", Locked: true},
			Lifecycle: account.InitialLifecycleState(aliceAccountID),
		}},
		Graph:       fakeGraph{},
		RateLimiter: kernel.NewRateLimiter(testkit.NewKeyValueStore(e.clock), e.clock),
		// No statuses exist.
		Statuses: &fakeStatuses{}, StatusReader: &fakeStatuses{}, Interactions: &fakeStatuses{},
		Locales: &i18n.Resolver{
			Bundle: messages,
			UserLocale: func(ctx context.Context) string {
				if v, ok := clientapi.ViewerFrom(ctx); ok && v.AccountID == aliceAccountID {
					return "ja"
				}
				return ""
			},
		},
		RequestID:      func(context.Context) string { return "req-1" },
		Logger:         slog.New(slog.DiscardHandler),
		Timelines:      &fakeTimelines{},
		TimelineReader: &fakeViews{},
	}
	if tweak != nil {
		tweak(&o)
	}
	s := clientapi.New(o)
	mux := http.NewServeMux()
	s.Register(mux)
	e.h = mux
	return e
}

func (e *authEnv) get(t *testing.T, path string, header map[string]string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, clientapi.BasePath+path, nil)
	req.RemoteAddr = "192.0.2.1:1234"
	for k, v := range header {
		req.Header.Set(k, v)
	}
	return e.spec.Serve(t, e.h, req)
}

func decodeProblem(t *testing.T, res *http.Response) clientapi.Problem {
	t.Helper()
	var p clientapi.Problem
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVerifyAccountCredentials(t *testing.T) {
	e := newAuthEnv(t, nil)
	for _, token := range []string{"read", "read-accounts"} {
		res := e.get(t, "/accounts/verify_credentials", map[string]string{"Authorization": "Bearer " + token})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", token, res.StatusCode)
		}
		var got clientapi.Account
		if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		followers, following := 2, 1
		want := clientapi.Account{
			Id: aliceAccountID, Username: "alice", Acct: "alice", DisplayName: "Alice", Note: "<p>hi</p>",
			Fields: []clientapi.ProfileField{}, Emojis: []clientapi.CustomEmoji{}, Badges: []clientapi.AccountBadge{},
			Url: "https://social.example/@alice", Locked: true, CreatedAt: createdAt, FollowersCount: &followers, FollowingCount: &following,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: account = %+v, want %+v", token, got, want)
		}
		if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
			t.Errorf("%s: Cache-Control = %q, want no-store", token, cc)
		}
		if got := res.Header.Get("RateLimit-Policy"); got != `"account";q=1500;w=300` {
			t.Errorf("%s: RateLimit-Policy = %q", token, got)
		}
	}
}

// TestAuthenticationProblems checks the answers to requests that may not run an operation:
// problem documents with stable types, localized, with the Bearer challenge of RFC 6750 §3.
func TestAuthenticationProblems(t *testing.T) {
	e := newAuthEnv(t, nil)
	tests := []struct {
		name          string
		path          string
		authorization []string
		wantStatus    int
		wantType      string
		wantChallenge string
		wantLanguage  string
		wantDetail    string
	}{
		{name: "no token", path: "/accounts/verify_credentials",
			wantStatus: 401, wantType: clientapi.ProblemTypeUnauthenticated, wantChallenge: "Bearer", wantLanguage: "en"},
		{name: "unknown token", path: "/accounts/verify_credentials", authorization: []string{"Bearer nope"},
			wantStatus: 401, wantType: clientapi.ProblemTypeInvalidToken, wantChallenge: `Bearer error="invalid_token"`, wantLanguage: "en"},
		{name: "lower-case scheme still checks the token", path: "/accounts/verify_credentials", authorization: []string{"bearer nope"},
			wantStatus: 401, wantType: clientapi.ProblemTypeInvalidToken, wantChallenge: `Bearer error="invalid_token"`, wantLanguage: "en"},
		{name: "basic credentials", path: "/accounts/verify_credentials", authorization: []string{"Basic YWxpY2U6c2VjcmV0"},
			wantStatus: 401, wantType: clientapi.ProblemTypeInvalidToken, wantChallenge: `Bearer error="invalid_token"`, wantLanguage: "en"},
		{name: "empty bearer", path: "/accounts/verify_credentials", authorization: []string{"Bearer "},
			wantStatus: 401, wantType: clientapi.ProblemTypeInvalidToken, wantChallenge: `Bearer error="invalid_token"`, wantLanguage: "en"},
		{name: "two authorization headers", path: "/accounts/verify_credentials", authorization: []string{"Bearer read", "Bearer read"},
			wantStatus: 401, wantType: clientapi.ProblemTypeInvalidToken, wantChallenge: `Bearer error="invalid_token"`, wantLanguage: "en"},
		{name: "invalid token on an anonymous operation", path: "/instance", authorization: []string{"Bearer nope"},
			wantStatus: 401, wantType: clientapi.ProblemTypeInvalidToken, wantChallenge: `Bearer error="invalid_token"`, wantLanguage: "en"},
		{name: "account unavailable", path: "/accounts/verify_credentials", authorization: []string{"Bearer pending"},
			wantStatus: 403, wantType: clientapi.ProblemTypeAccountUnavailable, wantLanguage: "en"},
		// The user's locale (ja) wins over Accept-Language (en).
		{name: "insufficient scope", path: "/accounts/verify_credentials", authorization: []string{"Bearer write"},
			wantStatus: 403, wantType: clientapi.ProblemTypeInsufficientScope,
			wantChallenge: `Bearer error="insufficient_scope", scope="read:accounts"`, wantLanguage: "ja",
			wantDetail: "アクセストークンにスコープ read:accounts が必要です。"},
		{name: "follow does not grant read:accounts", path: "/accounts/verify_credentials", authorization: []string{"Bearer follow-push"},
			wantStatus: 403, wantType: clientapi.ProblemTypeInsufficientScope,
			wantChallenge: `Bearer error="insufficient_scope", scope="read:accounts"`, wantLanguage: "ja"},
		// ADR-0090 §4: the account security operations take only a first-party client's token,
		// and a third-party client's token never holds the first-party-only scopes.
		{name: "password status for a third-party client", path: "/accounts/password", authorization: []string{"Bearer third-party"},
			wantStatus: 403, wantType: clientapi.ProblemTypeFirstPartyOnly, wantLanguage: "ja"},
		{name: "two-factor status for a third-party client", path: "/two_factor", authorization: []string{"Bearer third-party"},
			wantStatus: 403, wantType: clientapi.ProblemTypeFirstPartyOnly, wantLanguage: "ja"},
		{name: "passkeys for a third-party client", path: "/passkeys", authorization: []string{"Bearer third-party"},
			wantStatus: 403, wantType: clientapi.ProblemTypeFirstPartyOnly, wantLanguage: "ja"},
		{name: "OAuth apps for a third-party client", path: "/oauth_apps", authorization: []string{"Bearer third-party"},
			wantStatus: 403, wantType: clientapi.ProblemTypeFirstPartyOnly, wantLanguage: "ja"},
		{name: "admin for a third-party client", path: "/admin/accounts", authorization: []string{"Bearer third-party"},
			wantStatus: 403, wantType: clientapi.ProblemTypeInsufficientScope,
			wantChallenge: `Bearer error="insufficient_scope", scope="admin:read"`, wantLanguage: "ja"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, clientapi.BasePath+tt.path, nil)
			req.Header.Set("Accept-Language", "en")
			for _, a := range tt.authorization {
				req.Header.Add("Authorization", a)
			}
			res := e.spec.Serve(t, e.h, req)
			if res.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}
			if got := res.Header.Get("WWW-Authenticate"); got != tt.wantChallenge {
				t.Errorf("WWW-Authenticate = %q, want %q", got, tt.wantChallenge)
			}
			if got := res.Header.Get("Content-Language"); got != tt.wantLanguage {
				t.Errorf("Content-Language = %q, want %q", got, tt.wantLanguage)
			}
			if got := res.Header.Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			p := decodeProblem(t, res)
			if p.Type != tt.wantType || p.Status != tt.wantStatus || p.Title == "" {
				t.Errorf("problem = %+v, want type %s", p, tt.wantType)
			}
			if tt.wantDetail != "" && (p.Detail == nil || *p.Detail != tt.wantDetail) {
				t.Errorf("detail = %v, want %q", p.Detail, tt.wantDetail)
			}
		})
	}
}

// TestCookiesNeverAuthenticate checks that the client API reads credentials only from the
// Authorization header (ADR-0019 §4): a request with the server's cookies, even one holding a
// valid access token, is anonymous and the cookies are never looked at.
func TestCookiesNeverAuthenticate(t *testing.T) {
	e := newAuthEnv(t, nil)
	req := httptest.NewRequest(http.MethodGet, clientapi.BasePath+"/accounts/verify_credentials", nil)
	req.Header.Set("Cookie", `__Host-my_rt=read; __Host-my_session=read; access_token=read; Authorization="Bearer read"`)
	res := e.spec.Serve(t, e.h, req)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
	if p := decodeProblem(t, res); p.Type != clientapi.ProblemTypeUnauthenticated {
		t.Errorf("type = %s, want %s", p.Type, clientapi.ProblemTypeUnauthenticated)
	}
	if len(e.auth.asked) != 0 {
		t.Errorf("the authenticator was asked about %q", e.auth.asked)
	}
	// The access token in the query string does not authenticate either.
	res = e.get(t, "/accounts/verify_credentials?access_token=read", nil)
	if res.StatusCode != http.StatusUnauthorized || len(e.auth.asked) != 0 {
		t.Errorf("access_token query = %d (asked %q), want 401 without a lookup", res.StatusCode, e.auth.asked)
	}
}

func TestViewerOnAnonymousOperation(t *testing.T) {
	e := newAuthEnv(t, nil)
	res := e.get(t, "/instance", map[string]string{"Authorization": "Bearer write"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("RateLimit-Policy"); !strings.HasPrefix(got, `"account";`) {
		t.Errorf("RateLimit-Policy = %q, want the account policy", got)
	}
}

func TestRateLimits(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.AnonymousLimit = kernel.Limit{Name: "test.ip", Max: 2, Window: time.Minute}
		o.AccountLimit = kernel.Limit{Name: "test.account", Max: 3, Window: time.Minute}
	})
	send := func(remote, token string) *http.Response {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, clientapi.BasePath+"/instance", nil)
		req.RemoteAddr = remote
		req.Header.Set("Accept-Language", "ja")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return e.spec.Serve(t, e.h, req)
	}

	// Anonymous requests share the limit of their IP; IPv6 addresses share it by /64.
	res := send("[2001:db8::1]:1000", "")
	if res.StatusCode != http.StatusOK || res.Header.Get("RateLimit-Policy") != `"ip";q=2;w=60` || res.Header.Get("RateLimit") != `"ip";r=1;t=60` {
		t.Fatalf("first anonymous request = %d, RateLimit-Policy %q, RateLimit %q", res.StatusCode, res.Header.Get("RateLimit-Policy"), res.Header.Get("RateLimit"))
	}
	// An invalid token counts against the IP, like an anonymous request.
	if res := send("[2001:db8::2]:1000", "nope"); res.StatusCode != http.StatusUnauthorized || res.Header.Get("RateLimit") != `"ip";r=0;t=60` {
		t.Fatalf("invalid token = %d, RateLimit %q", res.StatusCode, res.Header.Get("RateLimit"))
	}
	res = send("[2001:db8::3]:1000", "")
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("third request from the /64 = %d, want 429", res.StatusCode)
	}
	if got := res.Header.Get("Retry-After"); got == "" || got == "0" {
		t.Errorf("Retry-After = %q", got)
	}
	if got := res.Header.Get("RateLimit"); !strings.HasPrefix(got, `"ip";r=0;t=`) {
		t.Errorf("RateLimit = %q", got)
	}
	if p := decodeProblem(t, res); p.Status != http.StatusTooManyRequests || p.Detail == nil || !strings.Contains(*p.Detail, "秒後") {
		t.Errorf("problem = %+v (detail %v), want a localized 429", p, p.Detail)
	}
	// Another network and authenticated requests have their own limits.
	if res := send("192.0.2.1:1000", ""); res.StatusCode != http.StatusOK {
		t.Errorf("request from another network = %d, want 200", res.StatusCode)
	}
	for i := range 3 {
		if res := send("[2001:db8::4]:1000", "read"); res.StatusCode != http.StatusOK {
			t.Fatalf("authenticated request %d = %d, want 200", i+1, res.StatusCode)
		}
	}
	res = send("[2001:db8::4]:1000", "read")
	if res.StatusCode != http.StatusTooManyRequests || !strings.HasPrefix(res.Header.Get("RateLimit"), `"account";r=0;`) {
		t.Fatalf("fourth authenticated request = %d, RateLimit %q, want 429 on the account policy", res.StatusCode, res.Header.Get("RateLimit"))
	}
	// The account limit is per account, wherever its requests come from.
	if res := send("192.0.2.9:1000", "write"); res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("same account from another IP = %d, want 429", res.StatusCode)
	}
	if res := send("[2001:db8::4]:1000", "bob"); res.StatusCode != http.StatusOK {
		t.Errorf("another account = %d, want 200", res.StatusCode)
	}

	// The window slides: after it, requests fit again.
	e.clock.Advance(2 * time.Minute)
	if res := send("[2001:db8::3]:1000", ""); res.StatusCode != http.StatusOK {
		t.Errorf("after the window = %d, want 200", res.StatusCode)
	}
}

// TestEveryOperationIsGuarded sends an anonymous request and a request whose token has no
// scope to every operation of the spec: those that need a token answer 401 and 403 before
// their handler runs (security.gen.go matches the spec and covers every operation).
func TestEveryOperationIsGuarded(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) { o.RateLimiter = nil })
	e.auth.tokens["none"] = bearer("", aliceAccountID)
	doc := e.spec.Doc()
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			reqs := doc.Security
			if op.Security != nil {
				reqs = *op.Security
			}
			anonymous := len(reqs) == 0 || slices.ContainsFunc(reqs, func(r openapi3.SecurityRequirement) bool { return len(r) == 0 })
			var scopes []string
			for _, r := range reqs {
				scopes = append(scopes, r["oauth2"]...)
			}
			url := clientapi.BasePath + strings.NewReplacer("{", "", "}", "").Replace(path)
			for token, want := range map[string]int{"": http.StatusUnauthorized, "none": http.StatusForbidden} {
				req := httptest.NewRequest(method, url, nil)
				if token != "" {
					req.Header.Set("Authorization", "Bearer "+token)
				}
				rec := httptest.NewRecorder()
				e.h.ServeHTTP(rec, req)
				needs := (token == "" && !anonymous) || (token != "" && len(scopes) > 0)
				if got := rec.Code; needs && got != want {
					t.Errorf("%s %s with token %q = %d, want %d", method, path, token, got, want)
				} else if !needs && (got == http.StatusUnauthorized || got == http.StatusForbidden) {
					t.Errorf("%s %s with token %q = %d, want it served", method, path, token, got)
				}
			}
		}
	}
}

// TestRegisterRefusesUnguardedOperations checks that Register panics when an operation has no
// security metadata, instead of serving it without authorization.
func TestRegisterRefusesUnguardedOperations(t *testing.T) {
	s := clientapi.New(clientapi.Options{})
	r := &patternRouter{}
	s.Register(r)
	want := clientapi.GuardedPatterns()
	// The bundled OpenAPI document is static and public: it is not an operation of the spec.
	r.patterns = slices.DeleteFunc(r.patterns, func(p string) bool { return p == "GET "+clientapi.BasePath+"/openapi.yaml" })
	slices.Sort(r.patterns)
	if !slices.Equal(r.patterns, want) {
		t.Errorf("registered %q, want the patterns of security.gen.go %q", r.patterns, want)
	}
	defer func() {
		if recover() == nil {
			t.Error("guarding an unknown pattern did not panic")
		}
	}()
	clientapi.Guard(s, "GET /api/v1/unknown", http.NotFoundHandler())
}

type patternRouter struct{ patterns []string }

func (r *patternRouter) Handle(pattern string, _ http.Handler) {
	r.patterns = append(r.patterns, pattern)
}

// TestSpecScopesAreTheServers checks that the oauth2 scheme of the spec declares exactly the
// scopes that the authorization server grants.
func TestSpecScopesAreTheServers(t *testing.T) {
	scheme := apitest.Load(t).Doc().Components.SecuritySchemes["oauth2"].Value
	var declared []string
	for s := range scheme.Flows.AuthorizationCode.Scopes {
		declared = append(declared, s)
	}
	slices.Sort(declared)
	if known := identity.KnownScopes(); !slices.Equal(declared, known) {
		t.Errorf("spec scopes = %q, want identity.KnownScopes %q", declared, known)
	}
}

func TestLimitProblem(t *testing.T) {
	messages, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	clock := testkit.NewClock(time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
	s := clientapi.New(clientapi.Options{
		RateLimiter: kernel.NewRateLimiter(testkit.NewKeyValueStore(clock), clock),
		Locales:     &i18n.Resolver{Bundle: messages},
		Logger:      slog.New(slog.DiscardHandler),
	})
	lim := kernel.Limit{Name: "test.post", Max: 1, Window: time.Hour}
	if _, _, limited := s.LimitProblem(t.Context(), lim, aliceAccountID); limited {
		t.Fatal("the first event is limited")
	}
	p, h, limited := s.LimitProblem(t.Context(), lim, aliceAccountID)
	if !limited || p.Status != http.StatusTooManyRequests || h.RetryAfter == nil || *h.RetryAfter < 1 || h.ContentLanguage == nil ||
		h.CacheControl == nil || *h.CacheControl != "no-store" {
		t.Errorf("second event = %+v %+v %v, want a 429 problem with Retry-After", p, h, limited)
	}
	if _, _, limited := clientapi.New(clientapi.Options{}).LimitProblem(t.Context(), lim, aliceAccountID); limited {
		t.Error("a Server without a RateLimiter limits")
	}
}

// TestChallengeNamesTheResourceMetadata checks that a 401 names the protected resource metadata
// (RFC 9728 §5.1) when the server has one, and that the challenge of a 403 does not.
func TestChallengeNamesTheResourceMetadata(t *testing.T) {
	const metadata = "https://social.example/.well-known/oauth-protected-resource"
	e := newAuthEnv(t, func(o *clientapi.Options) { o.ResourceMetadataURL = metadata })
	tests := []struct {
		name, authorization, want string
	}{
		{"no credential", "", `Bearer resource_metadata="` + metadata + `"`},
		{"invalid token", "Bearer nope", `Bearer error="invalid_token", resource_metadata="` + metadata + `"`},
		{"insufficient scope", "Bearer write", `Bearer error="insufficient_scope", scope="read:accounts"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, clientapi.BasePath+"/accounts/verify_credentials", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			res := e.spec.Serve(t, e.h, req)
			if got := res.Header.Get("WWW-Authenticate"); got != tt.want {
				t.Errorf("WWW-Authenticate = %q, want %q", got, tt.want)
			}
		})
	}
}
