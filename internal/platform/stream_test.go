// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/v2/pstest"

	"github.com/mycroblog/my/internal/adapter/httpsig"
	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/uow"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit/valkeytest"
)

// TestStreamsAcrossProcesses runs two Apps on one database and one broker, as two processes: a
// web process with an open stream and a worker. The post that the worker's process creates and
// the notification that its processing of an inbound Follow creates reach the stream of the web
// process (ADR-0050 §5), on the in-memory Bus and on stream = postgres (pgmem in pull requests).
func TestStreamsAcrossProcesses(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		env  map[string]string
		bus  func() (web, worker stream.Bus)
	}{
		{name: "memory", bus: func() (stream.Bus, stream.Bus) {
			b := stream.NewMemoryBroker()
			return b.Bus(), b.Bus()
		}},
		{name: "postgres", env: map[string]string{"MY_STREAM": "postgres"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := map[string]string{"MY_DB": "postgres", "MY_DATABASE_URL": newPostgresDatabase(t), "MY_WEB_ORIGIN": webOrigin}
			for k, v := range tt.env {
				env[k] = v
			}
			var webBus, workerBus stream.Bus
			if tt.bus != nil {
				webBus, workerBus = tt.bus()
			}
			web := newRoleApp(t, env, "web", webBus)
			worker := newRoleApp(t, env, "worker", workerBus)
			checkStreamsAcrossProcesses(t, web, worker)
		})
	}
}

// newRoleApp builds the App of role on env, with bus as its streams' Bus when not nil.
func newRoleApp(t *testing.T, env map[string]string, role string, bus stream.Bus) *platform.App {
	t.Helper()
	full := map[string]string{"MY_ROLE": role}
	for k, v := range env {
		full[k] = v
	}
	app, err := platform.NewApp(t.Context(), loadConfig(t, full), platform.AppOptions{Logger: slog.New(slog.DiscardHandler), StreamBus: bus})
	if err != nil {
		t.Fatalf("NewApp(%s): %v", role, err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if _, ok := app.Streams.(*stream.Relay); !ok {
		t.Fatalf("the streams of the %s process are a %T, want a *stream.Relay", role, app.Streams)
	}
	return app
}

func checkStreamsAcrossProcesses(t *testing.T, web, worker *platform.App) {
	ctx := t.Context()
	e := &timelineAPI{app: web, h: web.Handler(), spec: apitest.Load(t)}
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	e.follow(t, alice, bob)

	const actor = "https://remote.example/users/carol"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	err = web.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := web.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: actor, Domain: "remote.example", Type: "Person", PreferredUsername: "carol",
			InboxURL: actor + "/inbox", Keys: []appaccount.RemoteKeyInput{{KeyID: actor + "#main-key", PublicKey: &key.PublicKey}},
			Document: []byte(`{"id":"` + actor + `"}`),
		})
		return err
	})
	if err != nil {
		t.Fatalf("UpsertRemoteActor: %v", err)
	}

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	streamCtx, disconnect := context.WithCancel(ctx)
	defer disconnect()
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, srv.URL+clientapi.BasePath+"/streaming", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+alice.token)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /streaming: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /streaming = %d", res.StatusCode)
	}
	events := readEvents(bufio.NewReader(res.Body))
	next := func(name, schema string, v any) {
		t.Helper()
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("the stream ended before a %s event", name)
			}
			if ev.name != name {
				t.Fatalf("event %q (%s), want %q", ev.name, ev.data, name)
			}
			e.spec.ValidateSchema(t, schema, ev.data)
			if err := json.Unmarshal(ev.data, v); err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("no %s event within 10 seconds", name)
		}
	}
	// The stream of the federated timeline (ADR-0127) shares the Bus.
	federated := openTimelineStream(streamCtx, t, e, srv, alice, "/streaming/timelines/federated")
	// The subscriptions open once the web process listens on the Bus.
	deadline := time.Now().Add(15 * time.Second)
	for web.Streams.Len() < 2 {
		if time.Now().After(deadline) {
			t.Fatal("the stream did not subscribe")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// bob posts in the worker's process.
	created, err := worker.Content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: bob.accountID, TextInput: appcontent.TextInput{Text: "hello from the other process", Language: "en"},
		Visibility: domcontent.VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	p := created.Status.ID
	var st clientapi.Status
	next("update", "Status", &st)
	if st.Id != p || st.Account.Id != bob.accountID {
		t.Errorf("update = %s by %s, want bob's post %s", st.Id, st.Account.Id, p)
	}
	if got := nextUpdate(t, e, "the federated timeline", federated, 10*time.Second); got != p {
		t.Errorf("federated update = %s, want bob's post %s", got, p)
	}

	// carol's Follow arrives at the web process's inbox, and the worker processes it.
	follow := []byte(`{"@context":"https://www.w3.org/ns/activitystreams","id":"` + actor + `#follow-1","type":"Follow","actor":"` + actor +
		`","object":"https://` + web.Config.Host + `/ap/actors/` + alice.accountID + `"}`)
	postSigned(t, srv.URL, web.Config.Host, "/ap/inbox", follow, actor+"#main-key", key)
	if _, err := worker.Runner.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	var n clientapi.Notification
	next("notification", "Notification", &n)
	if n.Type != "follow" || n.Account == nil || n.Account.Uri != actor {
		t.Errorf("notification = %+v (account %+v), want carol's follow", n, n.Account)
	}
}

// postSigned POSTs body to path on base, signed with key as keyID for host (draft-cavage, as
// Mastodon signs), and fails unless the inbox accepts it.
func postSigned(t *testing.T, base, host, path string, body []byte, keyID string, key *rsa.PrivateKey) {
	t.Helper()
	req := &kernel.HTTPRequest{
		Method: http.MethodPost, URL: "https://" + host + path, Body: body,
		Header: http.Header{"Date": {time.Now().UTC().Format(http.TimeFormat)}, "Host": {host}},
	}
	err := httpsig.New().Sign(req, appfederation.SigningKey{KeyID: keyID, PrivateKey: key}, appfederation.SignOptions{
		Scheme: appfederation.SchemeCavage, Algorithm: appfederation.AlgRSAPKCS1SHA256,
		Components: []string{"(request-target)", "host", "date", "digest"},
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	hr, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range req.Header {
		if k != "Host" {
			hr.Header[k] = v
		}
	}
	hr.Header.Set("Content-Type", "application/activity+json")
	resp, err := http.DefaultTransport.RoundTrip(hr)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST %s = %d, want 202", path, resp.StatusCode)
	}
}

// TestServerStartsOnEachStreamBroker starts the server with stream set to each broker that
// needs no account: it is ready and its streams run on a Relay. stream = sns reaches AWS only
// once a stream opens, so it starts without it and registers the sweep of its queues.
func TestServerStartsOnEachStreamBroker(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		env   func(t *testing.T) map[string]string
		sweep bool
	}{
		{name: "postgres", env: func(t *testing.T) map[string]string {
			return map[string]string{"MY_DB": "postgres", "MY_DATABASE_URL": newPostgresDatabase(t), "MY_STREAM": "postgres", "MY_ROLE": "web"}
		}},
		{name: "valkey", env: func(t *testing.T) map[string]string {
			v := valkeytest.New(t)
			return map[string]string{
				"MY_DATABASE_URL": newDatabase(t, true), "MY_STREAM": "valkey", "MY_VALKEY_URL": v.URL, "MY_VALKEY_KEY_PREFIX": v.KeyPrefix,
			}
		}},
		{name: "sns", sweep: true, env: func(t *testing.T) map[string]string {
			return map[string]string{
				"MY_DATABASE_URL": newDatabase(t, true), "MY_STREAM": "sns",
				"MY_STREAM_SNS_TOPIC_ARN": "arn:aws:sns:us-east-1:123456789012:my-stream", "MY_STREAM_SQS_QUEUE_PREFIX": "my-stream",
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := start(t, loadConfig(t, tt.env(t)), platform.ServeOptions{}, nil)
			if resp, _ := s.do(t, http.MethodGet, platform.PathReadiness, nil); resp.StatusCode != http.StatusOK {
				t.Fatalf("readiness = %d, want 200", resp.StatusCode)
			}
			if _, ok := s.app.Streams.(*stream.Relay); !ok {
				t.Errorf("streams = %T, want a *stream.Relay", s.app.Streams)
			}
			if _, _, ok := s.app.Handlers.Lookup(stream.JobSweep); ok != tt.sweep {
				t.Errorf("handler of %s registered = %v, want %v", stream.JobSweep, ok, tt.sweep)
			}
			if strings.Contains(s.logs.String(), "stream = hub") {
				t.Errorf("a broker across processes warned about stream = hub:\n%s", s.logs.String())
			}
		})
	}
}

// With stream = hub, a process whose role is not all warns at start that its streams miss the
// changes of the other processes; one with role = all does not.
func TestHubWarnsOutsideRoleAll(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		role string
		warn bool
	}{{"all", false}, {"web", true}, {"worker", true}} {
		t.Run(tt.role, func(t *testing.T) {
			t.Parallel()
			s := start(t, loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_ROLE": tt.role}), platform.ServeOptions{}, nil)
			if _, ok := s.app.Streams.(*stream.Hub); !ok {
				t.Errorf("streams = %T, want a *stream.Hub", s.app.Streams)
			}
			if got := strings.Contains(s.logs.String(), "stream = hub delivers to the streams of this process only"); got != tt.warn {
				t.Errorf("warned = %v, want %v; logs:\n%s", got, tt.warn, s.logs.String())
			}
		})
	}
}

// TestStreamsAcrossProcessesOnPubSub runs the processes of TestStreamsAcrossProcesses on
// stream = pubsub, with the in-process Pub/Sub server pstest at PUBSUB_EMULATOR_HOST, and checks
// that both register the sweep of the subscriptions. It sets the environment, so it does not run
// in parallel.
func TestStreamsAcrossProcessesOnPubSub(t *testing.T) {
	srv := pstest.NewServer()
	t.Cleanup(func() { _ = srv.Close() })
	t.Setenv("PUBSUB_EMULATOR_HOST", srv.Addr)
	const topic = "projects/my-test/topics/my-stream"
	if _, err := srv.GServer.CreateTopic(t.Context(), &pubsubpb.Topic{Name: topic}); err != nil {
		t.Fatalf("CreateTopic: %v", err)
	}
	env := map[string]string{
		"MY_DB": "postgres", "MY_DATABASE_URL": newPostgresDatabase(t), "MY_WEB_ORIGIN": webOrigin,
		"MY_STREAM": "pubsub", "MY_STREAM_PUBSUB_TOPIC": topic, "MY_STREAM_PUBSUB_SUBSCRIPTION_PREFIX": "my-stream",
	}
	web := newRoleApp(t, env, "web", nil)
	worker := newRoleApp(t, env, "worker", nil)
	for _, a := range []*platform.App{web, worker} {
		if _, _, ok := a.Handlers.Lookup(stream.JobSweep); !ok {
			t.Errorf("no handler of %s is registered", stream.JobSweep)
		}
	}
	checkStreamsAcrossProcesses(t, web, worker)
}
