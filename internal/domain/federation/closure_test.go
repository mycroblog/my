// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"testing"

	"github.com/mycroblog/my/internal/domain/federation"
)

func TestClosureSends(t *testing.T) {
	for _, tc := range []struct {
		typ  federation.ActivityType
		kind federation.OutboundObjectKind
		want bool
	}{
		{federation.ActivityDelete, federation.OutboundAccount, true},
		{federation.ActivityUpdate, federation.OutboundAccount, false},
		{federation.ActivityDelete, federation.OutboundStatus, false},
		{federation.ActivityMove, federation.OutboundMove, false},
		{federation.ActivityCreate, federation.OutboundStatus, false},
	} {
		a := federation.OutboundActivity{Type: tc.typ, ObjectKind: tc.kind}
		if got := federation.ClosureSends(a); got != tc.want {
			t.Errorf("ClosureSends(%s of a %s) = %v, want %v", tc.typ, tc.kind, got, tc.want)
		}
		if got, want := federation.RecordsFanOutEnd(a), tc.want || tc.kind == federation.OutboundMove; got != want {
			t.Errorf("RecordsFanOutEnd(%s of a %s) = %v, want %v", tc.typ, tc.kind, got, want)
		}
	}
}

func TestClosureDeliveryStateOf(t *testing.T) {
	for _, tc := range []struct {
		outcome   federation.DeliveryOutcome
		exhausted bool
		want      federation.ClosureDeliveryState
	}{
		{"", false, federation.ClosureDeliveryPending},
		{federation.DeliveryRetryableFailure, false, federation.ClosureDeliveryPending},
		{federation.DeliveryRetryableFailure, true, federation.ClosureDeliveryDead},
		{federation.DeliverySucceeded, true, federation.ClosureDeliverySucceeded},
		{federation.DeliveryPermanentFailure, false, federation.ClosureDeliveryPermanentFailure},
		{federation.DeliverySkipped, false, federation.ClosureDeliverySkipped},
	} {
		if got := federation.ClosureDeliveryStateOf(tc.outcome, tc.exhausted); got != tc.want {
			t.Errorf("ClosureDeliveryStateOf(%q, %v) = %s, want %s", tc.outcome, tc.exhausted, got, tc.want)
		}
	}
	p := federation.ClosureProgress{Deliveries: map[federation.ClosureDeliveryState]int{federation.ClosureDeliveryDead: 2}}
	if !p.Ended() {
		t.Error("a progress with only dead deliveries has not ended")
	}
	p.FanOuts = 1
	if p.Ended() {
		t.Error("a progress with a fan-out left has ended")
	}
}
