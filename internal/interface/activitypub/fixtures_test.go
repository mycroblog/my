// SPDX-License-Identifier: AGPL-3.0-only

package activitypub_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/interface/activitypub"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata/golden")

const (
	host          = "social.example.org"
	accountDomain = "example.org"

	aliceID    = "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f"
	botID      = "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e70"
	carolID    = "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e71" // deactivated
	daveID     = "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e72" // deleted
	remoteID   = "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e73" // a remote account
	instanceID = "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e74"
	missingID  = "0192f3a4-7b1c-7d2e-9f00-ffffffffffff"
)

var created = time.Date(2026, 10, 16, 12, 0, 0, 123_000_000, time.UTC)

// fakeAccounts is an in-memory activitypub.Accounts with the lookup semantics of
// account.Service.
type fakeAccounts struct {
	mu       sync.Mutex
	actors   map[string]appaccount.Actor
	instance string
	users    int
	err      error // returned by every lookup when set
}

func (f *fakeAccounts) FindByID(_ context.Context, id string) (appaccount.Actor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return appaccount.Actor{}, f.err
	}
	a, ok := f.actors[id]
	if !ok {
		return appaccount.Actor{}, appaccount.ErrNotFound
	}
	return a, nil
}

func (f *fakeAccounts) FindByHandle(ctx context.Context, username, domain string) (appaccount.Actor, error) {
	f.mu.Lock()
	var id string
	for _, a := range f.actors {
		if h := a.Handle; h != nil && h.UsernameNormalized == account.NormalizeUsername(username) && h.Domain == account.NormalizeDomain(domain) {
			id = a.Account.ID
		}
	}
	err := f.err
	f.mu.Unlock()
	if err != nil {
		return appaccount.Actor{}, err
	}
	if id == "" {
		return appaccount.Actor{}, appaccount.ErrNotFound
	}
	return f.FindByID(ctx, id)
}

func (f *fakeAccounts) InstanceActor(ctx context.Context) (appaccount.Actor, error) {
	if f.instance == "" {
		return appaccount.Actor{}, appaccount.ErrNotFound
	}
	return f.FindByID(ctx, f.instance)
}

func (f *fakeAccounts) CountLocalUsers(context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.users, f.err
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := fs.ReadFile(os.DirFS("testdata"), name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// localActor returns an active local user with RSA and Ed25519 keys.
func localActor(t *testing.T, id, username string, profile account.ProfileRevision) appaccount.Actor {
	t.Helper()
	uri := account.ActorURI(host, id)
	profile.AccountID = id
	return appaccount.Actor{
		Account: account.Account{ID: id, URI: uri, Domain: account.LocalDomain, ActorType: account.ActorPerson, CreatedAt: created},
		Local:   &account.LocalAccount{AccountID: id, Username: username, UsernameNormalized: account.NormalizeUsername(username)},
		Handle:  &account.Handle{AccountID: id, Username: username, UsernameNormalized: account.NormalizeUsername(username), Domain: account.LocalDomain},
		Profile: &profile,
		Keys: []account.Key{
			{ID: id + "-rsa", AccountID: id, KeyID: account.KeyIRI(uri, account.KeyRSASHA256), Algorithm: account.KeyRSASHA256, PublicKeyPEM: readFile(t, "rsa-public.pem"), CreatedAt: created},
			{ID: id + "-ed", AccountID: id, KeyID: account.KeyIRI(uri, account.KeyEd25519), Algorithm: account.KeyEd25519, PublicKeyPEM: readFile(t, "ed25519-public.pem"), CreatedAt: created},
		},
		Lifecycle: account.InitialLifecycleState(id),
	}
}

func newAccounts(t *testing.T) *fakeAccounts {
	t.Helper()
	alice := localActor(t, aliceID, "alice", account.ProfileRevision{
		DisplayName: "Alice", NoteHTML: "<p>Hello</p>", Discoverable: true, AvatarMediaID: aliceAvatarID, HeaderMediaID: aliceHeaderID,
		Fields: []account.ProfileField{{Position: 0, Name: "Website", ValueHTML: `<a href="https://example.org" rel="me">example.org</a>`}},
	})
	bot := localActor(t, botID, "Botty", account.ProfileRevision{DisplayName: "Botty", Bot: true, Locked: true, Indexable: true})
	carol := localActor(t, carolID, "carol", account.ProfileRevision{DisplayName: "Carol"})
	carol.Lifecycle = account.LifecycleState{AccountID: carolID, State: account.StateDeactivated, Since: created}
	dave := localActor(t, daveID, "dave", account.ProfileRevision{DisplayName: "Dave"})
	dave.Lifecycle = account.LifecycleState{AccountID: daveID, State: account.StateDeleted, Since: created}
	remote := appaccount.Actor{
		Account:   account.Account{ID: remoteID, URI: "https://remote.example/users/erin", Domain: "remote.example", ActorType: account.ActorPerson, CreatedAt: created},
		Handle:    &account.Handle{AccountID: remoteID, Username: "erin", UsernameNormalized: "erin", Domain: "remote.example"},
		Lifecycle: account.InitialLifecycleState(remoteID),
	}
	instance := localActor(t, instanceID, "", account.ProfileRevision{})
	instance.Account.ActorType = account.ActorApplication
	instance.Local, instance.Handle, instance.Profile = nil, nil, nil

	f := &fakeAccounts{actors: map[string]appaccount.Actor{}, instance: instanceID, users: 4}
	for _, a := range []appaccount.Actor{alice, bot, carol, dave, remote, instance} {
		f.actors[a.Account.ID] = a
	}
	return f
}

// Media files of alice's avatar, an animation, and header.
const (
	aliceAvatarID = "01890a5d-ac96-774b-bcce-b302099a0a01"
	aliceHeaderID = "01890a5d-ac96-774b-bcce-b302099a0a02"
)

// profileImages serves alice's avatar and header as the media use cases would.
func profileImages(_ context.Context, refs []appcontent.ImageRef) (map[appcontent.ImageRef]appcontent.Image, error) {
	variant := func(name, contentType string) appcontent.ImageVariant {
		return appcontent.ImageVariant{URL: "https://files." + host + "/media/" + name, ContentType: contentType, Width: 400, Height: 400}
	}
	out := map[appcontent.ImageRef]appcontent.Image{}
	for _, ref := range refs {
		switch {
		case ref.AccountID != aliceID:
		case ref.MediaID == aliceAvatarID:
			out[ref] = appcontent.Image{
				Still:    appcontent.ImageVariants{Small: variant("a/small.webp", "image/webp"), Medium: variant("a/medium.webp", "image/webp"), Large: variant("a/large.png", "image/png")},
				Animated: &appcontent.ImageVariants{Small: variant("a/small-animated.webp", "image/webp"), Medium: variant("a/medium-animated.webp", "image/webp"), Large: variant("a/large-animated.gif", "image/gif")},
			}
		case ref.MediaID == aliceHeaderID:
			out[ref] = appcontent.Image{Still: appcontent.ImageVariants{Small: variant("h/small.webp", "image/webp"), Medium: variant("h/medium.webp", "image/webp"), Large: variant("h/large.jpg", "image/jpeg")}}
		}
	}
	return out, nil
}

// problem is the stand-in for the platform's problem writer: one fixed body per status.
func problem(w http.ResponseWriter, _ *http.Request, status int) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"type":"about:blank","status":` + strconv.Itoa(status) + "}\n"))
}

type server struct {
	mux      *http.ServeMux
	accounts *fakeAccounts
	objects  *fakeObjects
	graph    *fakeGraph
	inbox    *fakeInbox
	// suspended are the accounts that moderation suspends.
	suspended fakeSuspensions
}

func newServer(t *testing.T, accountDomain string) *server {
	t.Helper()
	s := &server{mux: http.NewServeMux(), accounts: newAccounts(t), objects: newObjects(), graph: &fakeGraph{}, inbox: &fakeInbox{}, suspended: fakeSuspensions{}}
	activitypub.New(activitypub.Options{
		Host: host, AccountDomain: accountDomain, SoftwareVersion: "0.1.0", SourceURL: "https://github.com/mycroblog/my",
		InstanceName:      func(context.Context) string { return "Example Social" },
		OpenRegistrations: func(context.Context) (bool, error) { return true, nil },
		LocalPosts:        func(context.Context) (int64, error) { return 42, nil },
		Metadata: func(context.Context) (activitypub.NodeMetadata, error) {
			return activitypub.NodeMetadata{
				Description: "A server for examples.", ContactActorURI: account.ActorURI(host, aliceID), ContactName: "Alice",
				ContactEmail: "admin@example.com", ThumbnailURL: "https://" + host + "/media/thumbnail.webp", ThumbnailAlt: "The beach",
				Closure: activitypub.NodeClosure{
					State: "announced", ClosesAt: time.Date(2027, 1, 31, 15, 0, 0, 0, time.UTC), ExportUntil: time.Date(2027, 3, 2, 15, 0, 0, 0, time.UTC),
				},
			}, nil
		},
		Accounts: s.accounts, Objects: s.objects, Graph: s.graph, Inbox: s.inbox, Problem: problem,
		Suspensions: s.suspended, Images: profileImages,
	}).Register(s.mux)
	return s
}

// get serves one GET request with an optional Accept header.
func (s *server) get(t *testing.T, target, accept string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+host+target, nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	return serve(s, req)
}

// serve serves one request.
func serve(s *server, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

// Accept headers that implementations send when they fetch an actor.
const (
	acceptMastodon   = `application/activity+json, application/ld+json; profile="https://www.w3.org/ns/activitystreams", text/html;q=0.1`
	acceptGoToSocial = `application/activity+json`
	acceptMisskey    = `application/activity+json, application/ld+json; profile="https://www.w3.org/ns/activitystreams"`
	acceptBrowser    = `text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8`
)

// checkGolden compares a JSON or XML body with testdata/golden/name, rewriting it with -update.
// JSON is compared after indenting, so the golden files stay readable.
func checkGolden(t *testing.T, name string, body []byte) {
	t.Helper()
	got := body
	if strings.HasSuffix(name, ".json") {
		var buf bytes.Buffer
		if err := json.Indent(&buf, body, "", "  "); err != nil {
			t.Fatalf("%s: invalid JSON %q: %v", name, body, err)
		}
		buf.WriteByte('\n')
		got = buf.Bytes()
	}
	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := fs.ReadFile(os.DirFS("testdata"), "golden/"+name)
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s is missing; run go test -update", path)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the golden file (run go test -update to accept):\n got: %s\nwant: %s", name, got, want)
	}
}

// decode decodes a JSON body into a generic document.
func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return doc
}
