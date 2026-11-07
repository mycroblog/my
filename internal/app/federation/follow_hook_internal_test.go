// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/uow"
)

// unconfiguredGate has no plugin, and fails the test when Decide reaches it.
type unconfiguredGate struct{ t *testing.T }

func (unconfiguredGate) Configured(apphook.Name) bool { return false }

func (unconfiguredGate) Active(context.Context, apphook.Name) (bool, error) { return false, nil }

func (g unconfiguredGate) Decide(context.Context, apphook.Call) (apphook.Result, error) {
	g.t.Error("Decide called on a hook with no plugin")
	return apphook.Result{}, nil
}

func (g unconfiguredGate) Recorded(context.Context, apphook.Name, string) (apphook.Result, error) {
	g.t.Error("Recorded called on a hook with no plugin")
	return apphook.Result{}, nil
}

func (unconfiguredGate) LinkStatus(context.Context, uow.Tx, string, string) error { return nil }

func (unconfiguredGate) Applied(context.Context, uow.Tx, apphook.Result) error { return nil }

// TestReviewWithoutAPlugin checks that a server with no inbound.follow plugin asks nothing.
func TestReviewWithoutAPlugin(t *testing.T) {
	t.Parallel()
	for name, gate := range map[string]apphook.Gate{"no gate": nil, "no plugin": unconfiguredGate{t}} {
		if _, err := (followHandlers{d: FollowHandlerDeps{Hooks: gate}}).review(t.Context(), &InboxActivity{}, "followee"); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestActorReviewWithoutAPlugin checks that a server with no inbound.actor plugin reads and asks
// nothing before it stores an actor, and leaves the input as it is.
func TestActorReviewWithoutAPlugin(t *testing.T) {
	t.Parallel()
	for name, gate := range map[string]apphook.Gate{"no gate": nil, "no plugin": unconfiguredGate{t}} {
		r := &RemoteActors{d: RemoteActorsDeps{Review: ActorReview{Gate: gate}}}
		in := appaccount.RemoteActorInput{URI: "https://remote.example/users/bob"}
		v, err := r.reviewActor(t.Context(), &in)
		if err != nil || v.Effect != apphook.Allow || v.file || in.KeepProfile || in.AccountID != "" {
			t.Errorf("%s: %+v, %v, input %+v", name, v, err, in)
		}
	}
}
