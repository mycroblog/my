// SPDX-License-Identifier: AGPL-3.0-only

package supporterstest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
)

// supporterRepoTipChanges records the tips enabled with their amounts and disabled.
func supporterRepoTipChanges(t *testing.T, r *supporterRepo) {
	ctx := t.Context()
	if _, err := r.s.LastTipChange(ctx, nil); !errors.Is(err, supporters.ErrNotFound) {
		t.Fatalf("LastTipChange of none = %v, want ErrNotFound", err)
	}
	on := supporters.TipChange{ID: r.ids.NewID(), Enabled: true, Currency: "JPY", Amounts: []int64{300, 500, 1000}, ChangedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.InsertTipChange(ctx, tx, on) })
	got, err := r.s.LastTipChange(ctx, nil)
	if err != nil || got.ID != on.ID || !got.Enabled || got.Currency != "JPY" || !slices.Equal(got.Amounts, on.Amounts) || !got.ChangedAt.Equal(on.ChangedAt) {
		t.Fatalf("LastTipChange = %+v, %v, want %+v", got, err, on)
	}
	r.clock.Advance(time.Second)
	off := supporters.TipChange{ID: r.ids.NewID(), Currency: "JPY", ChangedByUserID: testkit.NamedID("admin"), ChangedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.InsertTipChange(ctx, tx, off) })
	if got, err := r.s.LastTipChange(ctx, nil); err != nil || got.Enabled || len(got.Amounts) != 0 || got.ChangedByUserID != off.ChangedByUserID {
		t.Errorf("LastTipChange after disabling = %+v, %v", got, err)
	}
}

// supporterRepoTips records a tip, its checkout, its payment, and its outcomes once each, keeps
// one open tip per user, and erases a user's tips.
func supporterRepoTips(t *testing.T, r *supporterRepo) {
	ctx := t.Context()
	now := r.clock.Now()
	user := testkit.NamedID("tipper")
	tip := supporters.Tip{ID: r.ids.NewID(), UserID: user, AmountMinor: 500, Currency: "JPY", Provider: supporterRepoProvider, CreatedAt: now}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.InsertTip(ctx, tx, tip, now.Add(time.Hour)) })
	second := tip
	second.ID = r.ids.NewID()
	if err := r.txErr(t, func(ctx context.Context, tx uow.Tx) error { return r.s.InsertTip(ctx, tx, second, now.Add(time.Hour)) }); !errors.Is(err, supporters.ErrOpenTip) {
		t.Errorf("InsertTip of a user with an open tip = %v, want ErrOpenTip", err)
	}
	if id, err := r.s.OpenTipID(ctx, nil, user); err != nil || id != tip.ID {
		t.Errorf("OpenTipID = %s, %v, want %s", id, err, tip.ID)
	}
	co := supporters.TipCheckout{TipID: tip.ID, Provider: supporterRepoProvider, Ref: "cs_tip_1", ExpiresAt: now.Add(30 * time.Minute), CreatedAt: now}
	for range 2 {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.InsertTipCheckout(ctx, tx, co) })
	}
	if due, err := r.s.DueOpenTips(ctx, now.Add(31*time.Minute), 10); err != nil || !slices.Equal(due, []string{tip.ID}) {
		t.Errorf("DueOpenTips after the checkout's end = %v, %v, want the tip", due, err)
	}
	if due, err := r.s.DueOpenTips(ctx, now.Add(29*time.Minute), 10); err != nil || len(due) != 0 {
		t.Errorf("DueOpenTips before the checkout's end = %v, %v, want none", due, err)
	}
	if refs, err := r.s.OpenTipCheckouts(ctx, supporterRepoProvider, now); err != nil || !slices.Equal(refs, []supporters.CheckoutRef{"cs_tip_1"}) {
		t.Errorf("OpenTipCheckouts = %v, %v", refs, err)
	}
	if id, err := r.s.FindTipIDByCheckout(ctx, nil, supporterRepoProvider, "cs_tip_1"); err != nil || id != tip.ID {
		t.Errorf("FindTipIDByCheckout = %s, %v", id, err)
	}
	pay := supporters.TipPayment{TipID: tip.ID, Provider: supporterRepoProvider, Ref: "pi_tip_1", AmountMinor: 500, Currency: "JPY", PaidAt: now}
	for i := range 2 {
		var inserted bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			var err error
			inserted, err = r.s.InsertTipPayment(ctx, tx, pay)
			return err
		})
		if inserted != (i == 0) {
			t.Errorf("InsertTipPayment #%d inserted %v", i+1, inserted)
		}
	}
	if _, err := r.s.OpenTipID(ctx, nil, user); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("OpenTipID after the payment = %v, want ErrNotFound", err)
	}
	if id, err := r.s.FindTipIDByPayment(ctx, nil, supporterRepoProvider, "pi_tip_1"); err != nil || id != tip.ID {
		t.Errorf("FindTipIDByPayment = %s, %v", id, err)
	}
	for i := range 2 {
		var inserted bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			var err error
			inserted, err = r.s.InsertTipOutcome(ctx, tx, supporters.TipOutcome{ID: r.ids.NewID(), TipID: tip.ID, Kind: supporters.TipRefunded, OccurredAt: now})
			return err
		})
		if inserted != (i == 0) {
			t.Errorf("InsertTipOutcome #%d inserted %v", i+1, inserted)
		}
	}
	rec, err := r.s.FindTip(ctx, nil, tip.ID)
	if err != nil || rec.ID != tip.ID || rec.UserID != user || rec.AmountMinor != 500 || !rec.CreatedAt.Equal(now) ||
		rec.Checkout == nil || rec.Checkout.Ref != "cs_tip_1" || !rec.Checkout.ExpiresAt.Equal(co.ExpiresAt) ||
		rec.Payment == nil || rec.Payment.Ref != "pi_tip_1" || rec.State() != supporters.TipStatePaid || !rec.Has(supporters.TipRefunded) {
		t.Errorf("FindTip = %+v, %v", rec, err)
	}
	// The user's next tip expires: the open tip goes with it.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.InsertTip(ctx, tx, second, now.Add(time.Hour)) })
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.s.InsertTipOutcome(ctx, tx, supporters.TipOutcome{ID: r.ids.NewID(), TipID: second.ID, Kind: supporters.TipExpired, OccurredAt: now})
		return err
	})
	if _, err := r.s.OpenTipID(ctx, nil, user); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("OpenTipID after the expiry = %v, want ErrNotFound", err)
	}
	if rec, err := r.s.FindTip(ctx, nil, second.ID); err != nil || rec.State() != supporters.TipStateExpired || rec.Checkout != nil {
		t.Errorf("FindTip(expired) = %+v, %v", rec, err)
	}
	users, err := r.s.TipUsers(ctx, []string{tip.ID, second.ID, testkit.NamedID("unknown")})
	if err != nil || len(users) != 2 {
		t.Errorf("TipUsers = %v, %v, want two", users, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.s.EraseUser(ctx, tx, user) })
	for _, id := range []string{tip.ID, second.ID} {
		if _, err := r.s.FindTip(ctx, nil, id); !errors.Is(err, supporters.ErrNotFound) {
			t.Errorf("FindTip(%s) after the erasure = %v, want ErrNotFound", id, err)
		}
	}
	if _, err := r.s.FindTipIDByPayment(ctx, nil, supporterRepoProvider, "pi_tip_1"); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("FindTipIDByPayment after the erasure = %v, want ErrNotFound", err)
	}
}
