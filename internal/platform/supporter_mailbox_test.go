// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/platform"
)

// TestSupporterMailboxFollowsTransactionalSuppression sends the supporter emails, which are
// transactional (ADR-0086 §3), to an address that has a complaint and stops them after a hard
// bounce (ADR-0086 §5).
func TestSupporterMailboxFollowsTransactionalSuppression(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		kind domainidentity.EmailFailureKind
		want bool
	}{
		{name: "unsuppressed", want: true},
		{name: "complaint", kind: domainidentity.EmailComplaint, want: true},
		{name: "hard bounce", kind: domainidentity.EmailHardBounce, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newAccountsEnv(t)
			ctx := t.Context()
			carol := e.user("carol")
			addressID, ok, err := platform.SupporterMailbox(ctx, e.app, carol.id)
			if err != nil || !ok || addressID == "" {
				t.Fatalf("SupporterMailbox before any failure = %q, %v, %v", addressID, ok, err)
			}
			if tt.kind != "" {
				if err := e.app.Identity.RecordDeliveryFailure(ctx, identity.DeliveryFailure{
					EmailAddressID: addressID, Kind: tt.kind, Source: domainidentity.EmailFailureRejection,
					FeedbackKey: "test:" + tt.name, OccurredAt: time.Now(),
				}); err != nil {
					t.Fatal(err)
				}
			}
			if _, ok, err := platform.SupporterMailbox(ctx, e.app, carol.id); err != nil || ok != tt.want {
				t.Errorf("SupporterMailbox = %v, %v, want %v", ok, err, tt.want)
			}
		})
	}
}
