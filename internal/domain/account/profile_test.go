// SPDX-License-Identifier: AGPL-3.0-only

package account_test

import (
	"testing"

	"github.com/mycroblog/my/internal/domain/account"
)

func TestFeedEnabled(t *testing.T) {
	tests := []struct {
		feed      account.FeedSetting
		indexable bool
		enabled   bool
		follows   bool
	}{
		{"", true, true, true},
		{"", false, false, true},
		{account.FeedFollowIndexable, true, true, true},
		{account.FeedFollowIndexable, false, false, true},
		{account.FeedEnabled, false, true, false},
		{account.FeedDisabled, true, false, false},
		{"unknown", true, true, true},
	}
	for _, tt := range tests {
		r := account.ProfileRevision{Feed: tt.feed, Indexable: tt.indexable}
		if r.FeedEnabled() != tt.enabled || r.FeedFollowsIndexable() != tt.follows {
			t.Errorf("feed %q, indexable %v: FeedEnabled = %v, FeedFollowsIndexable = %v; want %v, %v",
				tt.feed, tt.indexable, r.FeedEnabled(), r.FeedFollowsIndexable(), tt.enabled, tt.follows)
		}
	}
	if account.FeedSettingOf(true) != account.FeedEnabled || account.FeedSettingOf(false) != account.FeedDisabled {
		t.Error("FeedSettingOf does not make the explicit choice")
	}
	// The zero value and follow_indexable are the same content, so a remote profile read again is
	// no new revision.
	if !(account.ProfileRevision{}).SameContent(account.ProfileRevision{Feed: account.FeedFollowIndexable}) {
		t.Error("the zero feed setting differs from follow_indexable")
	}
	if (account.ProfileRevision{}).SameContent(account.ProfileRevision{Feed: account.FeedDisabled}) {
		t.Error("a feed choice is not part of the content")
	}
}
