// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// logRepo serves one delivery and drops the attempts.
type logRepo struct {
	Repository
	dc DeliveryContext
}

func (r *logRepo) FindDeliveryContext(context.Context, string) (DeliveryContext, error) {
	return r.dc, nil
}

func (r *logRepo) InsertDeliveryAttempt(context.Context, uow.Tx, federation.DeliveryAttempt) error {
	return nil
}

type logTx struct{}

func (logTx) Emit(context.Context, uow.Event) error { return nil }

type logUoW struct{ uow.UnitOfWork }

func (logUoW) Tx(_ context.Context, fn func(uow.Tx) error) error { return fn(logTx{}) }

type logKeys struct{ key SigningKey }

func (k logKeys) SigningKey(context.Context, string) (SigningKey, error) { return k.key, nil }

type logIDs struct{ kernel.IDGenerator }

func (logIDs) NewID() string { return "attempt-1" }

// The log line of a delivery run carries the server's domain, the activity type, how the attempt
// ended, and the status code, and none of the sender, the activity URI, or the inbox URL.
func TestDeliverAddsItsAttributesToTheRunLog(t *testing.T) {
	const (
		inbox    = "https://remote.example/users/secret-inbox-name/inbox"
		activity = "https://social.example/activities/secret-activity"
		sender   = "sender-account-secret"
	)
	cases := []struct {
		name      string
		status    int
		suspended bool
		outcome   string
		code      bool
	}{
		{"success", http.StatusAccepted, false, "succeeded", true},
		{"permanent failure", http.StatusGone, false, "permanent_failure", true},
		{"skip", 0, true, "skipped", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := newRedirectNet()
			n.handle("remote.example", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status) })
			repo := &logRepo{dc: DeliveryContext{
				Delivery: federation.Delivery{ID: "d1", Domain: "remote.example", InboxURL: inbox},
				Activity: federation.OutboundActivity{
					ID: "o1", ActivityURI: activity, Type: federation.ActivityCreate, ObjectKind: federation.OutboundStatus, ActorAccountID: sender,
				},
				Document: []byte(`{}`), Health: federation.InstanceAvailable,
			}}
			var domains DomainPolicy
			if tc.suspended {
				domains = suspendedDomains{"remote.example"}
			}
			d := NewDeliverer(DeliverDeps{
				UnitOfWork: logUoW{}, Repository: repo,
				Client:  NewSignedHTTPClient(n, nopSigner{}, emptyKV{}, fixedClock{}),
				Keys:    logKeys{testSigningKey(t)},
				Domains: domains, Clock: fixedClock{}, IDs: logIDs{},
			})
			ctx, run := queue.WithRunAttrs(t.Context())
			if res := d.Deliver(ctx, "d1"); !res.IsDone() {
				t.Fatalf("Deliver() = %+v, want done", res)
			}
			got := map[string]string{}
			for _, a := range run.Attrs() {
				got[a.Key] = a.Value.String()
				for _, secret := range []string{inbox, activity, sender, "secret"} {
					if strings.Contains(a.Value.String(), secret) || strings.Contains(a.Key, secret) {
						t.Errorf("attribute %s=%s shows %q", a.Key, a.Value, secret)
					}
				}
			}
			want := map[string]string{
				LogDeliveryDomain: "remote.example", LabelActivityType: string(federation.ActivityCreate),
				LogDeliveryOutcome: tc.outcome,
			}
			if tc.code {
				want[LogStatusCode] = slog.IntValue(tc.status).String()
			}
			if len(got) != len(want) {
				t.Errorf("attributes = %v, want %v", got, want)
			}
			for k, v := range want {
				if got[k] != v {
					t.Errorf("%s = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}
