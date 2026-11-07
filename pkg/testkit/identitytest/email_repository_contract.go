// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The cases of AccountRepositoryContract on the email suppressions and the service closure emails
// of the identity repository.

// deliveryFailure records a delivery failure of an address and reports whether it did.
func (r *identityRepo) deliveryFailure(t *testing.T, addressID string, kind identity.EmailFailureKind, key string) bool {
	t.Helper()
	r.clock.Advance(time.Second)
	f := identity.EmailDeliveryFailure{ID: r.ids.NewID(), EmailAddressID: addressID, Kind: kind, Source: identity.EmailFailureSES, FeedbackKey: key, EnhancedStatus: "5.1.1", OccurredAt: r.clock.Now()}
	var ok bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		if ok, err = r.h.Identity.InsertEmailDeliveryFailure(ctx, tx, f); err != nil {
			return err
		}
		return r.h.Identity.RefreshEmailSuppression(ctx, tx, addressID)
	})
	return ok
}

func emailRepoSuppressions(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice := r.user(t)
	e := r.email(t, alice, "alice@example.com")
	if id, err := r.h.Identity.FindPrimaryEmailAddressID(ctx, "alice@example.com"); err != nil || id != e.ID {
		t.Errorf("FindPrimaryEmailAddressID = %q, %v; want %q", id, err, e.ID)
	}
	if _, err := r.h.Identity.FindPrimaryEmailAddressID(ctx, "nobody@example.com"); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindPrimaryEmailAddressID(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Identity.FindEmailSuppression(ctx, e.ID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindEmailSuppression before a failure = %v, want ErrNotFound", err)
	}
	if !r.deliveryFailure(t, e.ID, identity.EmailComplaint, "feedback-1") || r.deliveryFailure(t, e.ID, identity.EmailComplaint, "feedback-1") {
		t.Error("InsertEmailDeliveryFailure: want a feedback key recorded once")
	}
	complaintAt := r.clock.Now().Add(-time.Second)
	if s, err := r.h.Identity.FindEmailSuppression(ctx, e.ID); err != nil || s.Kind != identity.EmailComplaint || !s.Since.Equal(complaintAt) {
		t.Errorf("FindEmailSuppression after a complaint = %+v, %v; want a complaint since %v", s, err, complaintAt)
	}
	// A hard bounce outranks a complaint.
	r.deliveryFailure(t, e.ID, identity.EmailHardBounce, "feedback-2")
	if s, err := r.h.Identity.FindEmailSuppression(ctx, e.ID); err != nil || s.Kind != identity.EmailHardBounce {
		t.Errorf("FindEmailSuppression after a hard bounce = %+v, %v; want a hard bounce", s, err)
	}
	// A lift ends the suppression, and only a later failure brings it back.
	r.clock.Advance(time.Second)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.InsertEmailSuppressionLift(ctx, tx, identity.EmailSuppressionLift{ID: r.ids.NewID(), EmailAddressID: e.ID, Reason: identity.EmailLiftModerator, LiftedAt: r.clock.Now()}); err != nil {
			return err
		}
		return r.h.Identity.RefreshEmailSuppression(ctx, tx, e.ID)
	})
	if _, err := r.h.Identity.FindEmailSuppression(ctx, e.ID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindEmailSuppression after the lift = %v, want ErrNotFound", err)
	}
	r.deliveryFailure(t, e.ID, identity.EmailComplaint, "feedback-3")
	if s, err := r.h.Identity.FindEmailSuppression(ctx, e.ID); err != nil || s.Kind != identity.EmailComplaint {
		t.Errorf("FindEmailSuppression after a complaint past the lift = %+v, %v; want a complaint", s, err)
	}
}

func emailRepoClosureEmails(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	var verified []string
	for i := range 4 {
		u := r.user(t)
		e := r.email(t, u, "user"+string(rune('a'+i))+"@example.com")
		if i != 2 {
			r.tx(t, func(ctx context.Context, tx uow.Tx) error {
				return r.h.Identity.InsertEmailVerification(ctx, tx, identity.EmailVerification{EmailAddressID: e.ID, VerifiedAt: r.clock.Now()})
			})
			verified = append(verified, u)
		}
	}
	r.user(t) // without an address
	var got []string
	after := ""
	for range 3 {
		page, err := r.h.Identity.ListVerifiedEmailUsers(ctx, after, 2)
		if err != nil || len(page) > 2 {
			t.Fatalf("ListVerifiedEmailUsers = %v, %v; want at most 2", page, err)
		}
		for _, id := range page {
			got = append(got, id)
			after = id
		}
	}
	if !slices.Equal(got, verified) {
		t.Errorf("ListVerifiedEmailUsers pages = %v, want %v", got, verified)
	}
	event := r.ids.NewID()
	if queued, err := r.h.Identity.ServiceClosureEmailQueued(ctx, event, identity.ClosureEmailAnnounced); err != nil || queued {
		t.Errorf("ServiceClosureEmailQueued before any = %t, %v; want false", queued, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for i, want := range []bool{true, false} {
			ok, err := r.h.Identity.InsertServiceClosureEmail(ctx, tx, identity.ServiceClosureEmailRecord{
				ID: r.ids.NewID(), ClosureEventID: event, Email: identity.ClosureEmailAnnounced, UserID: verified[0], QueuedAt: r.clock.Now(),
			})
			if err != nil {
				return err
			}
			if ok != want {
				t.Errorf("InsertServiceClosureEmail #%d = %t, want %t", i+1, ok, want)
			}
		}
		return nil
	})
	if queued, err := r.h.Identity.ServiceClosureEmailQueued(ctx, event, identity.ClosureEmailAnnounced); err != nil || !queued {
		t.Errorf("ServiceClosureEmailQueued = %t, %v; want true", queued, err)
	}
	if queued, err := r.h.Identity.ServiceClosureEmailQueued(ctx, event, identity.ClosureEmailWithdrawn); err != nil || queued {
		t.Errorf("ServiceClosureEmailQueued(another email) = %t, %v; want false", queued, err)
	}
}
