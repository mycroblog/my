// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// batchAuthenticator is a fakeAuthenticator that also checks many tokens at once, counting the
// batch checks.
type batchAuthenticator struct {
	*fakeAuthenticator
	mu      sync.Mutex
	batches [][]string
}

func (b *batchAuthenticator) ValidBearers(ctx context.Context, tokens []string) (map[string]bool, error) {
	b.mu.Lock()
	b.batches = append(b.batches, tokens)
	b.mu.Unlock()
	out := map[string]bool{}
	for _, t := range tokens {
		_, err := b.AuthenticateBearer(ctx, t)
		out[t] = err == nil
	}
	return out, nil
}

func (b *batchAuthenticator) RecordBearerUse(context.Context, string) {}

// TestStreamingHeartbeatChecksTheTokensTogether opens three streams on an Authenticator with
// batch checks: every heartbeat checks the three tokens with one batch, none alone, and a token
// revoked ends its stream alone within a heartbeat.
func TestStreamingHeartbeatChecksTheTokensTogether(t *testing.T) {
	hub := stream.NewHub(stream.HubOptions{})
	var batch *batchAuthenticator
	e := newAuthEnv(t, func(o *clientapi.Options) {
		batch = &batchAuthenticator{fakeAuthenticator: o.Authenticator.(*fakeAuthenticator)}
		o.Authenticator, o.Streams, o.StreamHeartbeat = batch, hub, 20*time.Millisecond
	})
	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	_, _, read := openStream(t.Context(), t, srv, "read")
	_, _, bob := openStream(t.Context(), t, srv, "bob")
	_, _, write := openStream(t.Context(), t, srv, "third-party")
	if line := readLine(t, read); line != ": connected" {
		t.Fatalf("first line = %q", line)
	}
	for range 3 {
		if line := readLine(t, read); line != ": heartbeat" {
			t.Fatalf("line = %q, want a heartbeat", line)
		}
	}
	e.auth.mu.Lock()
	opened := len(e.auth.asked)
	e.auth.mu.Unlock()
	batch.mu.Lock()
	batches := len(batch.batches)
	full := 0
	for _, b := range batch.batches {
		if len(b) == 3 {
			full++
		}
	}
	batch.mu.Unlock()
	if batches == 0 || full == 0 {
		t.Errorf("%d batch checks, %d of the three tokens; want the heartbeats to check them together", batches, full)
	}
	// The single checks are those of the requests and of the batches' fake, one per token per batch.
	if want := 3 + 3*full + (batches-full)*2; opened > want+3 {
		t.Errorf("%d single checks, want about %d: none of the heartbeats checks a token alone", opened, want)
	}

	// bob's token is revoked: his stream ends, the others go on.
	e.auth.mu.Lock()
	e.auth.errs["bob"] = appidentity.ErrInvalidAccessToken
	e.auth.mu.Unlock()
	waitEnd(t, bob)
	if line := readLine(t, write); line != ": connected" && line != ": heartbeat" {
		t.Errorf("write's stream: %q, want it open", line)
	}
}
