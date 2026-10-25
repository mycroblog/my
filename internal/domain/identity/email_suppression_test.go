// SPDX-License-Identifier: AGPL-3.0-only

package identity_test

import (
	"testing"

	"github.com/mycroblog/my/internal/domain/identity"
)

func TestEmailSuppressionStops(t *testing.T) {
	for _, tt := range []struct {
		kind identity.EmailFailureKind
		want [3]bool // verification, transactional, notification
	}{
		{identity.EmailHardBounce, [3]bool{false, true, true}},
		{identity.EmailComplaint, [3]bool{false, false, true}},
		{"", [3]bool{false, false, false}},
	} {
		s := identity.EmailSuppression{Kind: tt.kind}
		got := [3]bool{s.Stops(identity.MailVerification), s.Stops(identity.MailTransactional), s.Stops(identity.MailNotification)}
		if got != tt.want {
			t.Errorf("%q stops %v, want %v", tt.kind, got, tt.want)
		}
	}
}

func TestEnhancedStatusCode(t *testing.T) {
	for code, want := range map[string]string{
		"5.1.1": "5.1.1", "4.2.2": "4.2.2", "5.7.100": "5.7.100",
		"": "", "550": "", "5.1": "", "6.1.1": "", "5.1.1 user unknown": "", "5.1.1234": "",
	} {
		if got := identity.EnhancedStatusCode(code); got != want {
			t.Errorf("EnhancedStatusCode(%q) = %q, want %q", code, got, want)
		}
	}
}
