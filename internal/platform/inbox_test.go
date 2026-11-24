// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/httpsig"
	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestInbox checks that the composition root serves the inboxes on the real HTTP signature
// adapter and database: a POST signed with the key of a stored remote actor is accepted, also
// when it is sent again, and the requests that the synchronous part refuses get their status.
func TestInbox(t *testing.T) {
	t.Parallel()
	in := startInbox(t)
	aliceID, follow, post, meter := in.aliceID, in.follow, in.post, in.meter

	tests := []struct {
		name, path, contentType string
		body, sent              []byte
		status                  int
	}{
		{"shared inbox", "/ap/inbox", "application/activity+json", follow, nil, http.StatusAccepted},
		{"the same activity again", "/ap/inbox", "application/activity+json", follow, nil, http.StatusAccepted},
		{"actor inbox", "/ap/actors/" + aliceID + "/inbox", `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`, follow, nil, http.StatusAccepted},
		{"unknown actor inbox", "/ap/actors/0192f3b0-0000-7000-8000-0000000000ff/inbox", "application/activity+json", follow, nil, http.StatusNotFound},
		{"unsupported content type", "/ap/inbox", "text/plain", follow, nil, http.StatusUnsupportedMediaType},
		{"body over 1 MiB", "/ap/inbox", "application/activity+json", follow, bytes.Repeat([]byte(" "), appfederation.InboxMaxBodyBytes+1), http.StatusRequestEntityTooLarge},
		{"body changed after signing", "/ap/inbox", "application/activity+json", follow, bytes.Replace(follow, []byte("follow-1"), []byte("follow-2"), 1), http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := post(t, tt.path, tt.contentType, tt.body, tt.sent)
			if resp.StatusCode != tt.status {
				t.Errorf("POST %s = %d, want %d", tt.path, resp.StatusCode, tt.status)
			}
			if tt.status == http.StatusUnauthorized && resp.Header.Get("Accept-Signature") == "" {
				t.Errorf("401 without Accept-Signature: %v", resp.Header)
			}
		})
	}

	// my.inbox.activities counts what the inbox did with each POST it read (ADR-0055 §3); the
	// refusals of the HTTP handler (404, 415, 413) never reach it.
	for labels, want := range map[[2]string]float64{
		{"Follow", appfederation.InboxOutcomeAccepted}:  1,
		{"Follow", appfederation.InboxOutcomeDuplicate}: 2,
		{"other", appfederation.InboxOutcomeRejected}:   1,
	} {
		match := map[string]string{appfederation.LabelActivityType: labels[0], appfederation.LabelOutcome: labels[1]}
		if got := meter.Sum(appfederation.InboxActivities.Name, match); got != want {
			t.Errorf("%s%v = %v, want %v", appfederation.InboxActivities.Name, match, got, want)
		}
	}
	// A refusal also carries its reason: the body changed after signing fails the signature.
	rejected := map[string]string{
		appfederation.LabelOutcome: appfederation.InboxOutcomeRejected, appfederation.LabelInboxRejection: appfederation.InboxRejectionSignature,
	}
	if got := meter.Sum(appfederation.InboxActivities.Name, rejected); got != 1 {
		t.Errorf("%s%v = %v, want 1", appfederation.InboxActivities.Name, rejected, got)
	}
	if err := meter.Err(); err != nil {
		t.Error(err)
	}
}

// TestInboxAccessLogLine checks that the access log line of a POST to an inbox carries the
// labels of my.inbox.activities under the same names and values, the reason only on a refusal,
// so that log-based metrics count what the inbox did (ADR-0055 §5); the line of another request
// carries none of them.
func TestInboxAccessLogLine(t *testing.T) {
	t.Parallel()
	in := startInbox(t)
	changed := bytes.Replace(in.follow, []byte("follow-1"), []byte("follow-2"), 1)
	for _, p := range []struct {
		sent   []byte
		status int
	}{
		{nil, http.StatusAccepted},
		{nil, http.StatusAccepted},
		{changed, http.StatusUnauthorized},
	} {
		if resp := in.post(t, "/ap/inbox", "application/activity+json", in.follow, p.sent); resp.StatusCode != p.status {
			t.Fatalf("POST /ap/inbox = %d, want %d", resp.StatusCode, p.status)
		}
	}
	if resp, _ := in.s.do(t, http.MethodGet, "/api/v1/instance", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/instance = %d", resp.StatusCode)
	}

	labels := []string{appfederation.LabelActivityType, appfederation.LabelOutcome, appfederation.LabelInboxRejection}
	var inbox, other []map[string]any
	testkit.Eventually(t, 10*time.Second, func() bool {
		inbox, other = nil, nil
		for line := range strings.Lines(in.s.logs.String()) {
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Fatalf("line %q: %v", line, err)
			}
			switch {
			case m["msg"] != "http request":
			case m["url.path"] == "/ap/inbox":
				inbox = append(inbox, m)
			case m["url.path"] == "/api/v1/instance":
				other = append(other, m)
			}
		}
		return len(inbox) == 3 && len(other) == 1
	})
	if len(inbox) != 3 || len(other) != 1 {
		t.Fatalf("%d access log lines of the inbox and %d of the instance, want 3 and 1:\n%s", len(inbox), len(other), in.s.logs.String())
	}

	// The lines may be written in another order than the requests: compare them as a set.
	got := map[[3]string]int{}
	for _, m := range inbox {
		var k [3]string
		for i, label := range labels {
			if v, ok := m[label]; ok {
				k[i], _ = v.(string)
				if k[i] == "" {
					k[i] = "<not a string>"
				}
			}
		}
		got[k]++
	}
	want := map[[3]string]int{
		{"Follow", appfederation.InboxOutcomeAccepted, ""}:                                   1,
		{"Follow", appfederation.InboxOutcomeDuplicate, ""}:                                  1,
		{"other", appfederation.InboxOutcomeRejected, appfederation.InboxRejectionSignature}: 1,
	}
	if !maps.Equal(got, want) {
		t.Errorf("(activity.type, my.outcome, my.inbox.rejection) of the inbox lines = %v, want %v:\n%s", got, want, in.s.logs.String())
	}
	for _, label := range labels {
		if v, ok := other[0][label]; ok {
			t.Errorf("the line of GET /api/v1/instance has %s = %v", label, v)
		}
	}
}

// inboxServer is a served App with a local user, alice, and a stored remote actor, bob, whose key
// signs what post sends.
type inboxServer struct {
	s       *server
	meter   *testkit.Meter
	aliceID string
	// follow is bob's Follow of alice.
	follow []byte
	// post signs body for path with bob's key (draft-cavage, as Mastodon sends it) and sends it,
	// with the body replaced by sent when it is not nil.
	post func(t *testing.T, path, contentType string, body, sent []byte) *http.Response
}

func startInbox(t *testing.T) *inboxServer {
	t.Helper()
	const actor = "https://remote.example/users/bob"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	var aliceID string
	meter := testkit.NewMeter()
	s := startApp(t, cfg, platform.AppOptions{Meter: meter}, platform.ServeOptions{}, func(a *platform.App) {
		ctx := t.Context()
		if _, _, ok := a.Handlers.Lookup(appfederation.JobInbox); !ok {
			t.Errorf("no handler of %s is registered", appfederation.JobInbox)
		}
		reg, err := a.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
			Username: "alice", Password: "correct horse battery staple", DisplayName: "Alice",
		})
		if err != nil {
			t.Fatalf("RegisterLocalUser: %v", err)
		}
		aliceID = reg.AccountID
		err = a.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			_, err := a.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
				URI: actor, Domain: "remote.example", Type: "Person", PreferredUsername: "bob",
				InboxURL: actor + "/inbox", Keys: []appaccount.RemoteKeyInput{{KeyID: actor + "#main-key", PublicKey: &key.PublicKey}},
				Document: []byte(`{"id":"` + actor + `"}`),
			})
			return err
		})
		if err != nil {
			t.Fatalf("UpsertRemoteActor: %v", err)
		}
	})
	follow := []byte(`{"@context":"https://www.w3.org/ns/activitystreams","id":"` + actor + `#follow-1","type":"Follow","actor":"` + actor + `","object":"https://example.test/ap/actors/` + aliceID + `"}`)

	post := func(t *testing.T, path, contentType string, body, sent []byte) *http.Response {
		t.Helper()
		req := &kernel.HTTPRequest{
			Method: http.MethodPost, URL: "https://" + cfg.Host + path, Body: body,
			Header: http.Header{"Date": {time.Now().UTC().Format(http.TimeFormat)}, "Host": {cfg.Host}},
		}
		err := httpsig.New().Sign(req, appfederation.SigningKey{KeyID: actor + "#main-key", PrivateKey: key}, appfederation.SignOptions{
			Scheme: appfederation.SchemeCavage, Algorithm: appfederation.AlgRSAPKCS1SHA256,
			Components: []string{"(request-target)", "host", "date", "digest"},
		})
		if err != nil {
			t.Fatalf("Sign: %v", err)
		}
		if sent == nil {
			sent = body
		}
		hr, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.url+path, bytes.NewReader(sent))
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range req.Header {
			if k != "Host" {
				hr.Header[k] = v
			}
		}
		hr.Header.Set("Content-Type", contentType)
		resp, err := http.DefaultTransport.RoundTrip(hr)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp
	}
	return &inboxServer{s: s, meter: meter, aliceID: aliceID, follow: follow, post: post}
}

// TestInboxHandlers builds the composition root, so that a handler registered twice (a panic in
// NewApp) fails the test, and checks that every activity type with a handler is routed.
func TestInboxHandlers(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	for _, typ := range []federation.ActivityType{
		federation.ActivityFollow, federation.ActivityAccept, federation.ActivityReject,
		federation.ActivityCreate, federation.ActivityUpdate, federation.ActivityDelete,
	} {
		if !app.InboxProcessor.Handles(typ) {
			t.Errorf("no inbox handler of %s", typ)
		}
	}
	if !app.InboxProcessor.Handles(federation.ActivityUndo, federation.ActivityFollow, federation.ActivityAccept) {
		t.Error("no Undo handler of Follow and Accept")
	}
}
