// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/httpsig"
	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

const deactivationPassword = "correct horse battery staple"

// deactivationEnv is a server with a real database and the inline queue, whose clock, email, and
// outgoing requests the test sees, with the users alice, bob, and mod.
type deactivationEnv struct {
	*statusesClient
	clock   *testkit.Clock
	mailer  *testkit.Mailer
	network *relayNet
}

func newDeactivationEnv(t *testing.T) *deactivationEnv {
	t.Helper()
	clock := testkit.NewClock(time.Now().UTC())
	mailer := &testkit.Mailer{}
	network := &relayNet{docs: map[string][]byte{}}
	c := newStatusesClientWith(t, map[string]string{
		"MY_QUEUE": "inline", "MY_MAIL": "smtp", "MY_MAIL_FROM": "noreply@example.test", "MY_SMTP_HOST": "127.0.0.1",
	}, platform.AppOptions{HTTPClient: network, Clock: clock, Mailer: mailer}, "alice", "bob", "mod")
	return &deactivationEnv{statusesClient: c, clock: clock, mailer: mailer, network: network}
}

// deactivate confirms the deactivation of user with the password, and returns the date.
func (e *deactivationEnv) deactivate(user string) time.Time {
	e.t.Helper()
	var d clientapi.Deactivation
	res := e.do(user, http.MethodPost, "/accounts/deactivation", map[string]any{"password": deactivationPassword}, &d)
	if res.StatusCode != http.StatusOK {
		e.t.Fatalf("deactivate %s = %d", user, res.StatusCode)
	}
	return d.DeletionDueAt
}

// userOf returns the user and the account of a username.
func (e *deactivationEnv) userOf(username string) (userID, accountID string) {
	e.t.Helper()
	a, err := e.app.Accounts.FindByHandle(e.t.Context(), username, account.LocalDomain)
	if err != nil || a.Local == nil {
		e.t.Fatalf("FindByHandle(%s) = %+v, %v", username, a, err)
	}
	return a.Local.UserID, a.Account.ID
}

// freshToken signs a user in again through the app's flow, for use after the reactivation.
func (e *deactivationEnv) freshToken(username string) {
	e.t.Helper()
	userID, _ := e.userOf(username)
	e.tokens[username] = issueTokens(e.t, e.app, userID, "").AccessToken
}

// reactivate brings an account back through the status page.
func (e *deactivationEnv) reactivate(username string) {
	e.t.Helper()
	s := e.screens()
	s.signIn(username)
	if rec := s.do(http.MethodPost, authpages.PathReactivate, s.form(authpages.PathSignIn)); rec.Code != http.StatusSeeOther {
		e.t.Fatalf("POST reactivate = %d %s", rec.Code, rec.Body)
	}
}

// deactivationScreens drives the server-rendered screens with the cookies of one browser.
type deactivationScreens struct {
	*screens
	e *deactivationEnv
}

func (e *deactivationEnv) screens() *deactivationScreens {
	return &deactivationScreens{screens: &screens{t: e.t, h: e.h, cookies: map[string]string{}}, e: e}
}

// signIn signs in with the password and expects to reach a page.
func (s *deactivationScreens) signIn(username string) {
	s.t.Helper()
	rec := s.do(http.MethodPost, authpages.PathSignIn, s.form(authpages.PathSignIn, "email", username+"@example.com", "password", deactivationPassword))
	if rec.Code != http.StatusSeeOther {
		s.t.Fatalf("sign in as %s = %d %s", username, rec.Code, rec.Body)
	}
}

// page returns the body of the sign-in page, the status page of an account that cannot be used.
func (s *deactivationScreens) page() string {
	s.t.Helper()
	return s.do(http.MethodGet, authpages.PathSignIn, nil).Body.String()
}

// TestSelfDeactivationThroughTheAPI drives the whole of the feature through the client API and the
// server-rendered screens (ADR-0145): the confirmation, the date and the one email, the end of
// the sessions and tokens, what others no longer see, what the reactivation brings back, the
// 24-hour gap, and the deletion at the date.
func TestSelfDeactivationThroughTheAPI(t *testing.T) {
	t.Parallel()
	e := newDeactivationEnv(t)
	ctx := t.Context()
	_, aliceID := e.userOf("alice")
	post := e.post("alice", map[string]any{"text": "before leaving", "visibility": "public"})
	e.do("bob", http.MethodPost, "/statuses/"+post.Id+"/boost", nil, nil)
	if res := e.do("bob", http.MethodPost, "/accounts/"+aliceID+"/follow", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("follow = %d", res.StatusCode)
	}

	// The preview tells the date and refuses nothing; a third-party app reaches neither.
	var preview clientapi.DeactivationPreview
	e.do("alice", http.MethodGet, "/accounts/deactivation", nil, &preview)
	if preview.Refusal != nil || !preview.DeletionDueAt.Equal(kernel.Normalize(e.clock.Now()).Add(account.DeactivationPeriod)) {
		t.Errorf("preview = %+v, want the date 30 days from now", preview)
	}
	app := e.appToken("alice", "read write")
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		var body any
		if method == http.MethodPost {
			body = map[string]any{"password": deactivationPassword}
		}
		if res := e.do(app, method, "/accounts/deactivation", body, nil); res.StatusCode != http.StatusForbidden {
			t.Errorf("%s /accounts/deactivation with a third-party token = %d, want 403", method, res.StatusCode)
		}
	}

	// It takes a confirmation, and a wrong one changes nothing.
	if res := e.do("alice", http.MethodPost, "/accounts/deactivation", map[string]any{}, nil); res.StatusCode == http.StatusOK {
		t.Error("a deactivation without a confirmation succeeded")
	}
	if p := e.problemOf("alice", http.MethodPost, "/accounts/deactivation", map[string]any{"password": "wrong"}, http.StatusUnprocessableEntity); p != clientapi.ProblemTypeWrongPassword {
		t.Errorf("wrong password: %s", p)
	}
	if p := e.problemOf("alice", http.MethodPost, "/accounts/deactivation", map[string]any{"two_factor_code": "000000"}, http.StatusConflict); p != clientapi.ProblemTypeTwoFactorNotEnabled {
		t.Errorf("a code without a second factor: %s", p)
	}

	due := e.deactivate("alice")
	if !due.Equal(kernel.Normalize(e.clock.Now()).Add(account.DeactivationPeriod)) {
		t.Errorf("date = %v, want 30 days from now", due)
	}

	// One email states the date.
	sent := e.mailer.Sent()
	var deactivationMails int
	for _, m := range sent {
		if strings.Contains(m.Text, deletionDate(due)) {
			deactivationMails++
		}
	}
	if deactivationMails != 1 {
		t.Errorf("%d emails state the date %s among %d; want 1", deactivationMails, deletionDate(due), len(sent))
	}

	// Every token has ended.
	if res := e.do("alice", http.MethodGet, "/accounts/verify_credentials", nil, nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("verify_credentials after the deactivation = %d, want 401", res.StatusCode)
	}

	// Others do not see the profile or the posts; the boost goes with them.
	if res := e.do("bob", http.MethodGet, "/accounts/"+aliceID, nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("the profile = %d, want 404", res.StatusCode)
	}
	if res := e.do("bob", http.MethodGet, "/statuses/"+post.Id, nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("the post = %d, want 404", res.StatusCode)
	}
	wantHome := func(what string, want bool) {
		t.Helper()
		var page clientapi.StatusPage
		e.do("bob", http.MethodGet, "/timelines/home", nil, &page)
		found := false
		for _, st := range page.Items {
			if st.Id == post.Id || (st.Boost != nil && st.Boost.Id == post.Id) {
				found = true
			}
		}
		if found != want {
			t.Errorf("%s: the post in bob's home = %v, want %v", what, found, want)
		}
	}
	wantHome("while deactivated", false)
	var found []clientapi.Account
	e.do("bob", http.MethodGet, "/accounts/search?q=alice", nil, &found)
	for _, a := range found {
		if a.Id == aliceID {
			t.Error("the search finds the deactivated account")
		}
	}

	// New relationships are refused, and a mention stays text.
	if res := e.do("mod", http.MethodPost, "/accounts/"+aliceID+"/follow", nil, nil); res.StatusCode == http.StatusOK {
		t.Error("a local follow of a deactivated account succeeded")
	}
	mention := e.post("bob", map[string]any{"text": "hello @alice", "visibility": "public"})
	if len(mention.Mentions) != 0 {
		t.Errorf("the mentions of a deactivated account = %+v, want none", mention.Mentions)
	}
	dm := e.post("bob", map[string]any{"text": "@alice psst", "visibility": "direct"})
	if len(dm.Mentions) != 0 {
		t.Errorf("the recipients of a direct post = %+v, want none", dm.Mentions)
	}

	// The moderators see that it is deactivated and can list it.
	var admin clientapi.AdminAccount
	e.do("mod", http.MethodGet, "/admin/accounts/"+aliceID, nil, &admin)
	if admin.State != clientapi.AdminAccountStateDeactivated {
		t.Errorf("state for the moderators = %s, want deactivated", admin.State)
	}
	var listed clientapi.AdminAccountPage
	e.do("mod", http.MethodGet, "/admin/accounts?state=deactivated", nil, &listed)
	if len(listed.Items) != 1 || listed.Items[0].Id != aliceID {
		t.Errorf("deactivated accounts = %+v, want alice", listed.Items)
	}

	// A sign-in reaches the status page, with the date, and no token; the app is closed.
	s := e.screens()
	s.signIn("alice")
	body := s.page()
	for _, want := range []string{deletionDate(due), authpages.PathReactivate, authpages.PathDeleteDeactivated} {
		if !strings.Contains(body, want) {
			t.Errorf("the status page has no %q:\n%s", want, body)
		}
	}

	// Reactivating brings everything back, with the old token still ended.
	oldToken := e.tokens["alice"]
	if rec := s.do(http.MethodPost, authpages.PathReactivate, s.form(authpages.PathSignIn)); rec.Code != http.StatusSeeOther {
		t.Fatalf("POST reactivate = %d %s", rec.Code, rec.Body)
	}
	if body := s.page(); strings.Contains(body, authpages.PathReactivate) {
		t.Errorf("the page after the reactivation still offers it:\n%s", body)
	}
	if res := e.do("alice", http.MethodGet, "/accounts/verify_credentials", nil, nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("the old token after the reactivation = %d, want 401", res.StatusCode)
	}
	e.freshToken("alice")
	if oldToken == e.tokens["alice"] {
		t.Fatal("the same token")
	}
	var me clientapi.CredentialAccount
	if res := e.do("alice", http.MethodGet, "/accounts/verify_credentials", nil, &me); res.StatusCode != http.StatusOK {
		t.Fatalf("verify_credentials after the reactivation = %d", res.StatusCode)
	}
	if me.FollowersCount == nil || *me.FollowersCount != 1 {
		t.Errorf("followers after the reactivation = %v, want the one who stayed", me.FollowersCount)
	}
	if res := e.do("bob", http.MethodGet, "/statuses/"+post.Id, nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("the post after the reactivation = %d, want 200", res.StatusCode)
	}
	wantHome("after the reactivation", true)
	e.do("bob", http.MethodGet, "/accounts/search?q=alice", nil, &found)
	if len(found) == 0 {
		t.Error("the search does not find the account after the reactivation")
	}
	var notifications clientapi.NotificationPage
	e.do("alice", http.MethodGet, "/notifications", nil, &notifications)
	for _, n := range notifications.Items {
		if n.Status != nil && (n.Status.Id == mention.Id || n.Status.Id == dm.Id) {
			t.Errorf("a notification of what was sent while deactivated: %+v", n)
		}
	}
	// What was addressed to it does not arrive.
	if res := e.do("alice", http.MethodGet, "/statuses/"+dm.Id, nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("the direct post after the reactivation = %d, want 404", res.StatusCode)
	}

	// The gap: 24 hours before the next deactivation, told in the preview too.
	if p := e.problemOf("alice", http.MethodPost, "/accounts/deactivation", map[string]any{"password": deactivationPassword}, http.StatusConflict); p != clientapi.ProblemTypeDeactivationRefused {
		t.Errorf("deactivate within the gap: %s", p)
	}
	e.do("alice", http.MethodGet, "/accounts/deactivation", nil, &preview)
	if preview.Refusal == nil || *preview.Refusal != clientapi.DeactivationPreviewRefusalRecentlyReactivated || preview.AvailableAt == nil {
		t.Errorf("preview within the gap = %+v", preview)
	}
	e.clock.Advance(account.DeactivationGap)
	e.freshToken("alice")
	e.deactivate("alice")

	// The date: past it, no sign-in and no reactivation, before the job and after it.
	e.clock.Advance(account.DeactivationPeriod)
	s = e.screens()
	rec := s.do(http.MethodPost, authpages.PathSignIn, s.form(authpages.PathSignIn, "email", "alice@example.com", "password", deactivationPassword))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("sign in past the date = %d, want 422", rec.Code)
	}
	mails := len(e.mailer.Sent())
	for _, want := range []int{1, 0} {
		if n, err := e.app.Accounts.ExecuteDueDeletions(ctx); err != nil || n != want {
			t.Fatalf("ExecuteDueDeletions = %d, %v; want %d", n, err, want)
		}
	}
	if n := len(e.mailer.Sent()); n != mails {
		t.Errorf("the deletion sent %d emails, want none", n-mails)
	}
	a, err := e.app.Accounts.FindByID(ctx, aliceID)
	if err != nil || a.Lifecycle.State != account.StateDeleted {
		t.Errorf("the account after the job = %+v, %v; want deleted", a.Lifecycle, err)
	}
}

// deletionDate is the date as a user without a time zone reads it.
func deletionDate(t time.Time) string {
	return identity.DeletionDateText(t, "")
}

// problemOf sends a request, checks its status, and returns the type of its problem.
func (e *deactivationEnv) problemOf(user, method, path string, body any, status int) string {
	e.t.Helper()
	res := e.do(user, method, path, body, nil)
	if res.StatusCode != status {
		e.t.Fatalf("%s %s = %d, want %d", method, path, res.StatusCode, status)
	}
	var p struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		e.t.Fatal(err)
	}
	return p.Type
}

// TestSelfDeactivationStatusPageDeletes checks the deletion button of the status page.
func TestSelfDeactivationStatusPageDeletes(t *testing.T) {
	t.Parallel()
	e := newDeactivationEnv(t)
	_, aliceID := e.userOf("alice")
	e.deactivate("alice")
	s := e.screens()
	s.signIn("alice")
	rec := s.do(http.MethodPost, authpages.PathDeleteDeactivated, s.form(authpages.PathSignIn, "password", "wrong"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("delete with a wrong password = %d, want 422", rec.Code)
	}
	rec = s.do(http.MethodPost, authpages.PathDeleteDeactivated, s.form(authpages.PathSignIn, "password", deactivationPassword))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body)
	}
	a, err := e.app.Accounts.FindByID(t.Context(), aliceID)
	if err != nil || a.Lifecycle.State != account.StateDeleted {
		t.Errorf("the account = %+v, %v; want deleted", a.Lifecycle, err)
	}
}

// postActorInbox POSTs activity to the personal inbox of accountID, signed by signer, and expects 202.
func postActorInbox(t *testing.T, e *deactivationEnv, accountID string, signer remoteActor, activity map[string]any) {
	t.Helper()
	if _, ok := activity["@context"]; !ok {
		activity["@context"] = "https://www.w3.org/ns/activitystreams"
	}
	body, err := json.Marshal(activity)
	if err != nil {
		t.Fatal(err)
	}
	const host = "example.test"
	path := "/ap/actors/" + accountID + "/inbox"
	req := &kernel.HTTPRequest{
		Method: http.MethodPost, URL: "https://" + host + path, Body: body,
		Header: http.Header{"Date": {e.clock.Now().UTC().Format(http.TimeFormat)}, "Host": {host}},
	}
	err = httpsig.New().Sign(req, appfederation.SigningKey{KeyID: signer.id + "#main-key", PrivateKey: signer.key}, appfederation.SignOptions{
		Scheme: appfederation.SchemeCavage, Algorithm: appfederation.AlgRSAPKCS1SHA256,
		Components: []string{"(request-target)", "host", "date", "digest"},
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	hr := httptest.NewRequest(http.MethodPost, "https://"+host+path, bytes.NewReader(body))
	for k, v := range req.Header {
		if k != "Host" {
			hr.Header[k] = v
		}
	}
	hr.Header.Set("Content-Type", "application/activity+json")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, hr)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST %v = %d %s, want 202", activity["id"], rec.Code, rec.Body)
	}
}

// typesPosted returns the activities of a type that posts holds for an inbox.
func typesPosted(posts map[string][]map[string]any, inbox, typ string) []map[string]any {
	var out []map[string]any
	for _, doc := range posts[inbox] {
		if doc["type"] == typ {
			out = append(out, doc)
		}
	}
	return out
}

// TestSelfDeactivationFederates drives what the other servers see and send through the inboxes
// (ADR-0145 §6, §7): the Update of the actor marked suspended and the Update with the original
// document, GET of the actor, WebFinger, and the post, the Reject of a new Follow, what stays of the
// follows, what is dropped, what is counted, and the Delete at the date.
func TestSelfDeactivationFederates(t *testing.T) {
	t.Parallel()
	e := newDeactivationEnv(t)
	ctx := t.Context()
	_, aliceID := e.userOf("alice")
	_, bobID := e.userOf("bob")
	aliceURI := "https://example.test/ap/actors/" + aliceID
	const carolInbox, daveInbox, frankInbox = "https://remote.example/users/carol/inbox", "https://remote.example/users/dave/inbox", "https://remote.example/users/frank/inbox"
	carol := newRemoteActor(t, e.network, "https://remote.example/users/carol", "Person", carolInbox)
	dave := newRemoteActor(t, e.network, "https://remote.example/users/dave", "Person", daveInbox)
	frank := newRemoteActor(t, e.network, "https://remote.example/users/frank", "Person", frankInbox)

	// carol and frank follow alice before she leaves; bob follows dave, so that dave's posts come in.
	follow := func(who remoteActor, n string) {
		t.Helper()
		postActorInbox(t, e, aliceID, who, map[string]any{"id": who.id + "/follows/" + n, "type": "Follow", "actor": who.id, "object": aliceURI})
	}
	follow(carol, "1")
	follow(frank, "1")
	daveAccount, err := e.app.RemoteActors.Resolve(ctx, dave.id)
	if err != nil {
		t.Fatalf("Resolve(dave) = %v", err)
	}
	e.network.taken()
	e.do("bob", http.MethodPost, "/accounts/"+daveAccount.Account.ID+"/follow", nil, nil)
	sentFollow := typesPosted(e.network.taken(), daveInbox, "Follow")
	if len(sentFollow) != 1 {
		t.Fatalf("Follows to dave = %+v, want 1", sentFollow)
	}
	postInbox(t, e.statusesClient, dave, map[string]any{"id": dave.id + "/accepts/1", "type": "Accept", "actor": dave.id, "object": sentFollow[0]["id"]})
	post := e.post("alice", map[string]any{"text": "public post", "visibility": "public"})
	e.network.taken()

	actor := func() (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, aliceURI, nil)
		req.Header.Set("Accept", "application/activity+json")
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		var doc map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &doc)
		return rec.Code, doc
	}

	due := e.deactivate("alice")
	posts := e.network.taken()
	updates := typesPosted(posts, carolInbox, "Update")
	if len(updates) != 1 {
		t.Fatalf("Updates to carol after the deactivation = %+v, want 1", posts)
	}
	object, _ := updates[0]["object"].(map[string]any)
	if object["suspended"] != true || object["name"] != "" || object["id"] != aliceURI {
		t.Errorf("the Update = %+v, want the emptied actor marked suspended", object)
	}
	if code, doc := actor(); code != http.StatusOK || doc["suspended"] != true || doc["publicKey"] == nil || doc["inbox"] == nil {
		t.Errorf("GET of the actor = %d %v, want the suspended document with its key and inbox", code, doc)
	}
	finger := httptest.NewRequestWithContext(ctx, http.MethodGet, "https://example.test/.well-known/webfinger?resource="+url.QueryEscape("acct:alice@example.test"), nil)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, finger)
	if rec.Code != http.StatusOK {
		t.Errorf("WebFinger = %d, want 200", rec.Code)
	}
	objectReq := httptest.NewRequestWithContext(ctx, http.MethodGet, post.Uri, nil)
	objectReq.Header.Set("Accept", "application/activity+json")
	rec = httptest.NewRecorder()
	e.h.ServeHTTP(rec, objectReq)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET of the post = %d, want 404, not 410", rec.Code)
	}

	// A new Follow gets a Reject and is not recorded.
	follow(dave, "1")
	posts = e.network.taken()
	if rejects := typesPosted(posts, daveInbox, "Reject"); len(rejects) != 1 {
		t.Errorf("the answer to a new Follow = %+v, want a Reject", posts)
	}
	// A Follow sent again by a follower changes nothing and gets no answer.
	follow(carol, "2")
	posts = e.network.taken()
	if got := posts[carolInbox]; len(got) != 0 {
		t.Errorf("the answer to a Follow sent again = %+v, want none", got)
	}
	// An Undo of a follow goes through.
	postActorInbox(t, e, aliceID, frank, map[string]any{
		"id": frank.id + "/undo/1", "type": "Undo", "actor": frank.id,
		"object": map[string]any{"id": frank.id + "/follows/1", "type": "Follow", "actor": frank.id, "object": aliceURI},
	})

	// A remote Create drops the mention of the deactivated account, not the post.
	public := "https://www.w3.org/ns/activitystreams#Public"
	create := func(id string, to []string, tag []map[string]any, signer remoteActor) {
		t.Helper()
		postInbox(t, e.statusesClient, signer, map[string]any{
			"id": id + "/activity", "type": "Create", "actor": signer.id, "to": to,
			"object": map[string]any{
				"id": id, "type": "Note", "attributedTo": signer.id, "content": "<p>hello</p>", "to": to, "tag": tag,
				"published": e.clock.Now().UTC().Format(time.RFC3339),
			},
		})
	}
	mentionPost := dave.id + "/posts/mention"
	create(mentionPost, []string{public, aliceURI}, []map[string]any{{"type": "Mention", "href": aliceURI, "name": "@alice"}}, dave)
	stored, err := e.app.Content.FindStatusByURI(ctx, mentionPost)
	if err != nil {
		t.Fatalf("the post that also names a deactivated account was not stored: %v", err)
	}
	var seen clientapi.Status
	e.do("bob", http.MethodGet, "/statuses/"+stored.ID, nil, &seen)
	if len(seen.Mentions) != 0 {
		t.Errorf("the mentions of the remote post = %+v, want none", seen.Mentions)
	}
	// A post addressed only to it is not stored, even from an author that a local account follows.
	dmURI := dave.id + "/posts/dm"
	create(dmURI, []string{aliceURI}, []map[string]any{{"type": "Mention", "href": aliceURI, "name": "@alice"}}, dave)
	if _, err := e.app.Content.FindStatusByURI(ctx, dmURI); err == nil {
		t.Error("a direct post to a deactivated account alone was stored")
	}
	// A post addressed to it and to another keeps the other.
	sharedURI := dave.id + "/posts/shared"
	bobURI := "https://example.test/ap/actors/" + bobID
	create(sharedURI, []string{aliceURI, bobURI}, []map[string]any{
		{"type": "Mention", "href": aliceURI, "name": "@alice"}, {"type": "Mention", "href": bobURI, "name": "@bob"},
	}, dave)
	shared, err := e.app.Content.FindStatusByURI(ctx, sharedURI)
	if err != nil {
		t.Fatalf("a direct post to another local account as well was not stored: %v", err)
	}

	// A favourite counts and notifies nobody.
	postInbox(t, e.statusesClient, carol, map[string]any{"id": carol.id + "/likes/1", "type": "Like", "actor": carol.id, "object": post.Uri, "to": []string{aliceURI}})
	e.network.taken()

	// The reactivation sends the original document, and everything else is as it was.
	e.reactivate("alice")
	e.freshToken("alice")
	posts = e.network.taken()
	updates = typesPosted(posts, carolInbox, "Update")
	if len(updates) != 1 {
		t.Fatalf("Updates to carol after the reactivation = %+v, want 1", posts)
	}
	object, _ = updates[0]["object"].(map[string]any)
	if object["suspended"] != nil || object["preferredUsername"] != "alice" || object["publicKey"] == nil {
		t.Errorf("the Update = %+v, want the original document", object)
	}
	if _, doc := actor(); doc["suspended"] != nil {
		t.Errorf("GET of the actor after the reactivation = %v, want the original", doc)
	}
	var followers clientapi.AccountPage
	e.do("alice", http.MethodGet, "/accounts/"+aliceID+"/followers", nil, &followers)
	if len(followers.Accounts) != 1 || followers.Accounts[0].Uri != carol.id {
		t.Errorf("followers after the reactivation = %+v, want carol alone: frank undid, dave was refused", followers.Accounts)
	}
	var back clientapi.Status
	e.do("alice", http.MethodGet, "/statuses/"+post.Id, nil, &back)
	if back.FavouritesCount != 1 {
		t.Errorf("favourites of the post = %d, want the one received while deactivated", back.FavouritesCount)
	}
	var notifications clientapi.NotificationPage
	e.do("alice", http.MethodGet, "/notifications", nil, &notifications)
	for _, n := range notifications.Items {
		if n.CreatedAt.After(due.Add(-account.DeactivationPeriod)) {
			t.Errorf("a notification created while deactivated: %+v", n)
		}
	}
	if res := e.do("bob", http.MethodGet, "/statuses/"+shared.ID, nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("the direct post to bob = %d, want 200", res.StatusCode)
	}
	if res := e.do("alice", http.MethodGet, "/statuses/"+shared.ID, nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("the direct post, as alice after the reactivation, = %d, want 404", res.StatusCode)
	}

	// The Delete at the date.
	e.clock.Advance(account.DeactivationGap)
	e.freshToken("alice")
	e.deactivate("alice")
	e.network.taken()
	e.clock.Advance(account.DeactivationPeriod)
	if n, err := e.app.Accounts.ExecuteDueDeletions(ctx); err != nil || n != 1 {
		t.Fatalf("ExecuteDueDeletions = %d, %v", n, err)
	}
	posts = e.network.taken()
	deletes := typesPosted(posts, carolInbox, "Delete")
	if len(deletes) != 1 || deletes[0]["object"] != aliceURI {
		t.Errorf("Deletes to carol = %+v, want one of the actor", posts)
	}
}

// TestSelfDeactivationAndModeration checks how a deactivation meets moderation (ADR-0145 §1, §2, §4,
// §7): the actor stays hidden, so a change of either sends nothing, and a reactivation leaves an
// action in effect; a report still reaches the moderators; a ban takes over.
func TestSelfDeactivationAndModeration(t *testing.T) {
	t.Parallel()
	e := newDeactivationEnv(t)
	ctx := t.Context()
	aliceUser, aliceID := e.userOf("alice")
	const carolInbox = "https://remote.example/users/carol/inbox"
	carol := newRemoteActor(t, e.network, "https://remote.example/users/carol", "Person", carolInbox)
	aliceURI := "https://example.test/ap/actors/" + aliceID
	postActorInbox(t, e, aliceID, carol, map[string]any{"id": carol.id + "/follows/1", "type": "Follow", "actor": carol.id, "object": aliceURI})
	post := e.post("alice", map[string]any{"text": "reported", "visibility": "public"})
	res := e.do("bob", http.MethodPost, "/reports", map[string]any{
		"account_id": aliceID, "status_ids": []string{post.Id}, "category": "other", "comment": "look at this",
	}, nil)
	if res.StatusCode >= 300 {
		t.Fatalf("POST /reports = %d", res.StatusCode)
	}
	e.deactivate("alice")
	e.network.taken()
	wantUpdates := func(what string, want int) {
		t.Helper()
		if got := typesPosted(e.network.taken(), carolInbox, "Update"); len(got) != want {
			t.Errorf("%s: %d Updates to carol, want %d", what, len(got), want)
		}
	}

	// The report still reaches the moderators, who can act on the account.
	var reports clientapi.AdminReportPage
	e.do("mod", http.MethodGet, "/admin/reports", nil, &reports)
	if len(reports.Items) != 1 {
		t.Fatalf("reports for the moderators = %+v, want 1", reports.Items)
	}
	if res := e.do("mod", http.MethodGet, "/admin/reports/"+reports.Items[0].Id, nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("the report of a deactivated account = %d, want 200", res.StatusCode)
	}

	// A suspension and its lift while deactivated send nothing, and the posts stay hidden.
	var a clientapi.AdminAccountAction
	if res := e.do("mod", http.MethodPost, "/admin/accounts/"+aliceID+"/actions", map[string]any{"kind": "suspend"}, &a); res.StatusCode != http.StatusOK {
		t.Fatalf("suspend = %d", res.StatusCode)
	}
	wantUpdates("the suspension while deactivated", 0)
	if res := e.do("mod", http.MethodPost, "/admin/account_actions/"+a.Id+"/lift", map[string]any{"reason": "appealed"}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("lift = %d", res.StatusCode)
	}
	wantUpdates("the lift while deactivated", 0)
	if res := e.do("bob", http.MethodGet, "/timelines/public", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("public timeline = %d", res.StatusCode)
	}
	var page clientapi.StatusPage
	e.do("bob", http.MethodGet, "/timelines/public", nil, &page)
	for _, st := range page.Items {
		if st.Id == post.Id {
			t.Error("a lift brought the deactivated author's post back into the timeline")
		}
	}

	// A suspension in effect stays after the reactivation, which sends nothing either; the lift
	// shows the account again.
	if res := e.do("mod", http.MethodPost, "/admin/accounts/"+aliceID+"/actions", map[string]any{"kind": "suspend"}, &a); res.StatusCode != http.StatusOK {
		t.Fatalf("suspend = %d", res.StatusCode)
	}
	wantUpdates("the second suspension", 0)
	s := e.screens()
	s.signIn("alice")
	if body := s.page(); !strings.Contains(body, authpages.PathReactivate) || !strings.Contains(body, "moderation-actions") {
		t.Errorf("the page of a suspended, deactivated account:\n%s", body)
	}
	if rec := s.do(http.MethodPost, authpages.PathReactivate, s.form(authpages.PathSignIn)); rec.Code != http.StatusSeeOther {
		t.Fatalf("reactivate while suspended = %d", rec.Code)
	}
	wantUpdates("the reactivation while suspended", 0)
	got, err := e.app.Identity.UserAccess(ctx, aliceUser)
	if err != nil || got.Deactivated || !got.LoginRestricted || got.Allowed() {
		t.Errorf("access after the reactivation while suspended = %+v, %v", got, err)
	}
	e.do("bob", http.MethodGet, "/timelines/public", nil, &page)
	for _, st := range page.Items {
		if st.Id == post.Id {
			t.Error("the reactivation brought a suspended author's post back into the timeline")
		}
	}
	if res := e.do("mod", http.MethodPost, "/admin/account_actions/"+a.Id+"/lift", map[string]any{"reason": "appealed"}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("lift = %d", res.StatusCode)
	}
	wantUpdates("the lift after the reactivation", 1)
	e.do("bob", http.MethodGet, "/timelines/public", nil, &page)
	var listed bool
	for _, st := range page.Items {
		listed = listed || st.Id == post.Id
	}
	if !listed {
		t.Error("the post is not back after the lift")
	}

	// A ban while deactivated takes over: no reactivation, the ban's date.
	e.clock.Advance(account.DeactivationGap)
	e.freshToken("alice")
	e.freshToken("mod")
	e.deactivate("alice")
	e.network.taken()
	if res := e.do("mod", http.MethodPost, "/admin/accounts/"+aliceID+"/actions", map[string]any{"kind": "ban"}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("ban = %d", res.StatusCode)
	}
	if err := e.app.Identity.ReactivateOwnAccount(ctx, aliceUser); !errorsIsNotDeactivated(err) {
		t.Errorf("ReactivateOwnAccount of a banned account = %v, want ErrNotDeactivated", err)
	}
	s = e.screens()
	s.signIn("alice")
	if body := s.page(); strings.Contains(body, authpages.PathReactivate) {
		t.Errorf("the page of a banned account offers the reactivation:\n%s", body)
	}
}

func errorsIsNotDeactivated(err error) bool {
	return err != nil && strings.Contains(err.Error(), appaccount.ErrNotDeactivated.Error())
}

// TestSelfDeactivationLeavesRooms checks the voice rooms (ADR-0145 §5): a deactivation ends the
// participation of a participant, and the room of a host, as a suspension does.
func TestSelfDeactivationLeavesRooms(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline",
		"MY_VOICE": "p2p", "MY_VOICE_STUN_URLS": "stun:stun.example.test:3478",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	alice, bob, carol := e.user("alice"), e.user("bob"), e.user("carol")
	var opened clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, "/voice/rooms", alice.token, map[string]any{"name": "Talk"}, &opened), http.StatusCreated, "open")
	room := "/voice/rooms/" + opened.Room.Id
	e.want(e.do(http.MethodPost, room+"/participation", bob.token, nil, nil), http.StatusOK, "join by bob")
	e.want(e.do(http.MethodPost, room+"/participation", carol.token, nil, nil), http.StatusOK, "join by carol")

	confirm := map[string]any{"password": deactivationPassword}
	e.want(e.do(http.MethodPost, "/accounts/deactivation", bob.token, confirm, nil), http.StatusOK, "deactivate bob")
	var got clientapi.VoiceRoom
	e.want(e.do(http.MethodGet, room, carol.token, nil, &got), http.StatusOK, "the room after bob left")
	if len(got.Participants) != 2 {
		t.Errorf("participants = %+v, want alice and carol", got.Participants)
	}
	e.want(e.do(http.MethodPost, "/accounts/deactivation", alice.token, confirm, nil), http.StatusOK, "deactivate the host")
	// Nobody else may host, so the room ends.
	e.want(e.do(http.MethodGet, room, carol.token, nil, nil), http.StatusNotFound, "the room after the host left")
}
