// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
)

// timeoutNet is a kernel.HTTPClient whose every request times out waiting for the server.
type timeoutNet struct{}

func (timeoutNet) Do(context.Context, *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	return nil, errors.New("net/http: timeout awaiting response headers")
}

// A server that does not answer for its NodeInfo is the remote server's doing: the job tries
// again, and is not reported as an error once it is buried (queue.RetryRemote). A server whose
// NodeInfo is unusable ends the job, and a failure of this server is a plain retry.
func TestObserveRetriesRemoteFailuresAsRemote(t *testing.T) {
	answering := func(status int) func() kernel.HTTPClient {
		return func() kernel.HTTPClient {
			n := newRedirectNet()
			n.handle("remote.example", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
			return n
		}
	}
	cases := []struct {
		name         string
		http         func() kernel.HTTPClient
		retry, other bool
	}{
		{"Timeout", func() kernel.HTTPClient { return timeoutNet{} }, true, true},
		{"ServerError", answering(http.StatusServiceUnavailable), true, true},
		{"TooManyRequests", answering(http.StatusTooManyRequests), true, true},
		{"NotFound", answering(http.StatusNotFound), false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := tc.http()
			r := &RemoteActors{d: RemoteActorsDeps{Fetcher: testFetcher(t, newRedirectNet(), "blocked.example"), HTTP: h, Logger: slog.New(slog.DiscardHandler)}}
			res := r.handleObserve(t.Context(), queue.Job{Payload: []byte(`{"domain":"remote.example"}`)})
			if res.IsRetry() != tc.retry || res.IsRemote() != tc.other {
				t.Errorf("handleObserve() = retry %v, remote %v (%v); want retry %v, remote %v",
					res.IsRetry(), res.IsRemote(), res.Err(), tc.retry, tc.other)
			}
			if tc.name == "NotFound" && !res.IsDone() {
				t.Errorf("handleObserve() on a 404 = %v, want done: the server has no NodeInfo", res.Err())
			}
		})
	}
}
