// SPDX-License-Identifier: AGPL-3.0-only

package activitypub_test

import (
	"context"
	"fmt"
	"sync"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/graph"
)

// Statuses of the fake objects.
const (
	publicStatusID   = "0192f3b0-2c3d-7e4f-8a00-112233445501"
	followersOnlyID  = "0192f3b0-2c3d-7e4f-8a00-112233445502" // visible to followerDomain only
	deletedStatusID  = "0192f3b0-2c3d-7e4f-8a00-112233445503"
	missingStatusID  = "0192f3b0-2c3d-7e4f-8a00-1122334455ff"
	followerDomain   = "follower.example"
	strangerDomain   = "stranger.example"
	statusPagePrefix = "https://" + host + "/@alice/status/"
	outboxNext       = "0192f3b0-2c3d-7e4f-8a00-112233445600"
	announceID       = "0192f3b0-2c3d-7e4f-8a00-112233445700"
)

// fakeObjects is an activitypub.Objects over fixed statuses. A request's requester is the domain
// in its Signature header (no header: anonymous).
type fakeObjects struct {
	mu sync.Mutex
	// requests are the signed requests that Authenticate saw.
	requests []*appfederation.SignedRequest
	outbox   []federation.ActivityDocument
	featured []federation.NoteDocument
	// authorizedFetch is the authorized fetch setting.
	authorizedFetch bool
	err             error
}

func newObjects() *fakeObjects {
	return &fakeObjects{}
}

func (f *fakeObjects) AuthorizedFetch(context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.authorizedFetch, f.err
}

func (f *fakeObjects) Authenticate(_ context.Context, r *appfederation.SignedRequest) (appfederation.Requester, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r)
	return appfederation.Requester{Domain: r.Header.Get("Signature")}, f.err
}

func (f *fakeObjects) find(req appfederation.Requester, id, suffix string) (appfederation.Object, error) {
	if f.err != nil {
		return appfederation.Object{}, f.err
	}
	uri := "https://" + host + "/ap/objects/" + id + suffix
	doc := func(typ string) []byte {
		return fmt.Appendf(nil, `{"@context":"https://www.w3.org/ns/activitystreams","id":%q,"type":%q}`, uri, typ)
	}
	typ := "Note"
	if suffix != "" {
		typ = "Create"
	}
	switch {
	case id == publicStatusID:
		return appfederation.Object{Document: doc(typ), Public: true, URL: statusPagePrefix + id}, nil
	case id == followersOnlyID && req.Domain == followerDomain:
		return appfederation.Object{Document: doc(typ), URL: statusPagePrefix + id}, nil
	case id == deletedStatusID:
		return appfederation.Object{Document: doc("Tombstone"), Gone: true, Public: true, URL: statusPagePrefix + id}, nil
	}
	return appfederation.Object{}, fmt.Errorf("status %s: %w", id, appfederation.ErrNotFound)
}

func (f *fakeObjects) Status(_ context.Context, req appfederation.Requester, id string) (appfederation.Object, error) {
	return f.find(req, id, "")
}

func (f *fakeObjects) StatusActivity(_ context.Context, req appfederation.Requester, id string) (appfederation.Object, error) {
	return f.find(req, id, "/activity")
}

func (f *fakeObjects) QuoteAuthorization(context.Context, string) (appfederation.Object, error) {
	return appfederation.Object{}, appfederation.ErrNotFound
}

// Announce finds the one boost announceID.
func (f *fakeObjects) Announce(_ context.Context, _ appfederation.Requester, id string) (appfederation.Object, error) {
	if f.err != nil {
		return appfederation.Object{}, f.err
	}
	if id != announceID {
		return appfederation.Object{}, fmt.Errorf("activity %s: %w", id, appfederation.ErrNotFound)
	}
	return appfederation.Object{
		Public:   true,
		Document: fmt.Appendf(nil, `{"@context":"https://www.w3.org/ns/activitystreams","id":"https://%s/ap/activities/%s","type":"Announce"}`, host, id),
	}, nil
}

// PageStatusURI finds the statuses that an anonymous visitor may see and that are not deleted.
func (f *fakeObjects) PageStatusURI(_ context.Context, id string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if id != publicStatusID {
		return "", fmt.Errorf("status %s: %w", id, appfederation.ErrNotFound)
	}
	return "https://" + host + "/ap/objects/" + id, nil
}

func (f *fakeObjects) OutboxTotal(context.Context, string) (int64, error) {
	return int64(len(f.outbox)), f.err
}

// Outbox returns the fake outbox on the first page, with the cursor outboxNext, and nothing
// after it.
func (f *fakeObjects) Outbox(_ context.Context, _ string, before string) (appfederation.OutboxPage, error) {
	if before != "" {
		return appfederation.OutboxPage{Items: []federation.ActivityDocument{}}, f.err
	}
	return appfederation.OutboxPage{Items: f.outbox, Next: outboxNext}, f.err
}

func (f *fakeObjects) Featured(context.Context, string) ([]federation.NoteDocument, error) {
	return f.featured, f.err
}

// fakeGraph is an activitypub.Graph over a list of edges.
type fakeGraph struct {
	edges []graph.Edge // newest first
}

func (g *fakeGraph) Counters(_ context.Context, accountID string) (graph.Counters, error) {
	c := graph.Counters{AccountID: accountID}
	for _, e := range g.edges {
		if e.FolloweeAccountID == accountID {
			c.FollowersCount++
		}
		if e.FollowerAccountID == accountID {
			c.FollowingCount++
		}
	}
	return c, nil
}

func (g *fakeGraph) list(match func(graph.Edge) bool, p appgraph.Page) []graph.Edge {
	var out []graph.Edge
	started := p.Before == ""
	for _, e := range g.edges {
		if !started {
			started = e.FollowID == p.Before
			continue
		}
		if match(e) && len(out) < p.Limit {
			out = append(out, e)
		}
	}
	return out
}

func (g *fakeGraph) Followers(_ context.Context, accountID string, p appgraph.Page) ([]graph.Edge, error) {
	return g.list(func(e graph.Edge) bool { return e.FolloweeAccountID == accountID }, p), nil
}

func (g *fakeGraph) Following(_ context.Context, accountID string, p appgraph.Page) ([]graph.Edge, error) {
	return g.list(func(e graph.Edge) bool { return e.FollowerAccountID == accountID }, p), nil
}
