// SPDX-License-Identifier: AGPL-3.0-only

package instance_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/instance"
)

func TestDefinitionsAreValid(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range instance.Definitions() {
		if seen[string(d.Key)] {
			t.Errorf("duplicate key %s", d.Key)
		}
		seen[string(d.Key)] = true
		if v, err := d.Parse(d.Default); err != nil || v != d.Default {
			t.Errorf("default of %s = %q parses to %q, %v; want itself", d.Key, d.Default, v, err)
		}
	}
	for _, d := range instance.TextDefinitions() {
		if seen[string(d.Key)] {
			t.Errorf("text key %s is also a setting key", d.Key)
		}
		seen[string(d.Key)] = true
	}
}

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		key  instance.SettingKey
		raw  string
		want string // "" means invalid
	}{
		{instance.SettingAuthorizedFetch, "true", "true"},
		{instance.SettingAuthorizedFetch, "False", "false"},
		{instance.SettingAuthorizedFetch, "1", ""},
		{instance.SettingLocalTimelineAccess, "logged_in", "logged_in"},
		{instance.SettingFederatedTimelineAccess, "true", ""},
		{instance.SettingMaxStatusLength, "1", "1"},
		{instance.SettingMaxStatusLength, "+42", "42"},
		{instance.SettingMaxStatusLength, "100000", "100000"},
		{instance.SettingMaxStatusLength, "100001", ""},
		{instance.SettingMaxStatusLength, "-1", ""},
		{instance.SettingMaxStatusLength, "5e2", ""},
		{instance.SettingRegistrationMode, "closed", "closed"},
		{instance.SettingRegistrationMode, "Closed", ""},
		{instance.SettingDefaultLocale, "ja", "ja"},
		{instance.SettingDefaultLocale, "zh-Hant-TW", "zh-Hant-TW"},
		{instance.SettingDefaultLocale, "x", ""},
		{instance.SettingDefaultLocale, "1a", ""},
		{instance.SettingDefaultLocale, "en--US", ""},
		{instance.SettingDefaultLocale, "", ""},
		// The retention periods may not go below their floors (ADR-0038).
		{instance.SettingRetentionSessionOrigins, "90", "90"},
		{instance.SettingRetentionSessionOrigins, "89", ""},
		{instance.SettingRetentionRegistrationOrigins, "89", ""},
		{instance.SettingRetentionReports, "180", "180"},
		{instance.SettingRetentionReports, "179", ""},
		{instance.SettingRetentionModerationActions, "179", ""},
		{instance.SettingRetentionAuditLog, "365", "365"},
		{instance.SettingRetentionAuditLog, "364", ""},
		{instance.SettingRetentionAuditLog, "3650", "3650"},
		{instance.SettingRetentionAuditLog, "3651", ""},
		// The voice chat settings (ADR-0100 §2, §9, §13).
		{instance.SettingVoiceRoomStarters, "everyone", "everyone"},
		{instance.SettingVoiceRoomStarters, "permitted", "permitted"},
		{instance.SettingVoiceRoomStarters, "nobody", ""},
		{instance.SettingVoicePublicSummary, "TRUE", "true"},
		{instance.SettingVoicePublicSummary, "yes", ""},
		{instance.SettingRetentionVoiceParticipants, "30", "30"},
		{instance.SettingRetentionVoiceParticipants, "29", ""},
		{instance.SettingRetentionVoiceParticipants, "3650", "3650"},
		{instance.SettingRetentionVoiceParticipants, "3651", ""},
	} {
		d, err := instance.Lookup(tc.key)
		if err != nil {
			t.Fatal(err)
		}
		got, err := d.Parse(tc.raw)
		switch {
		case tc.want == "" && !errors.Is(err, instance.ErrInvalidValue):
			t.Errorf("Parse(%s, %q) = %q, %v; want ErrInvalidValue", tc.key, tc.raw, got, err)
		case tc.want != "" && (err != nil || got != tc.want):
			t.Errorf("Parse(%s, %q) = %q, %v; want %q", tc.key, tc.raw, got, err, tc.want)
		}
	}
	if _, err := instance.Lookup("nope"); !errors.Is(err, instance.ErrUnknownKey) {
		t.Errorf("Lookup(nope) = %v, want ErrUnknownKey", err)
	}
	if _, err := instance.LookupText("nope"); !errors.Is(err, instance.ErrUnknownKey) {
		t.Errorf("LookupText(nope) = %v, want ErrUnknownKey", err)
	}
}

func TestSettingsFallBackToDefaults(t *testing.T) {
	var zero instance.Settings
	if zero.MaxStatusLength() != 500 || zero.RegistrationMode() != instance.RegistrationApproval {
		t.Errorf("zero Settings = %d, %s; want the defaults", zero.MaxStatusLength(), zero.RegistrationMode())
	}
	s := instance.NewSettings([]instance.CurrentSetting{
		{Key: instance.SettingMaxStatusLength, RevisionID: testID(1), Value: "42"},
		{Key: "retired_key", RevisionID: testID(2), Value: "x"},
	})
	if v, stored := s.Value(instance.SettingMaxStatusLength); v != "42" || !stored || s.MaxStatusLength() != 42 {
		t.Errorf("max_status_length = %q, %v", v, stored)
	}
	if v, stored := s.Value("retired_key"); v != "" || stored {
		t.Errorf("unknown key = %q, %v; want ignored", v, stored)
	}
}

func TestRemoteMediaSettings(t *testing.T) {
	var zero instance.Settings
	if zero.RemoteMediaPolicy() != instance.RemoteMediaNone || zero.RemoteMediaCacheTTL() != 24*time.Hour {
		t.Errorf("zero Settings = %s, %v; want none and a day", zero.RemoteMediaPolicy(), zero.RemoteMediaCacheTTL())
	}
	s := instance.NewSettings([]instance.CurrentSetting{
		{Key: instance.SettingRemoteMediaPolicy, RevisionID: testID(1), Value: "all"},
		{Key: instance.SettingRemoteMediaCacheTTL, RevisionID: testID(2), Value: "3600"},
	})
	if s.RemoteMediaPolicy() != instance.RemoteMediaAll || s.RemoteMediaCacheTTL() != time.Hour {
		t.Errorf("Settings = %s, %v; want all and an hour", s.RemoteMediaPolicy(), s.RemoteMediaCacheTTL())
	}
	d, err := instance.Lookup(instance.SettingRemoteMediaCacheTTL)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"59", "31536001", "1h"} {
		if _, err := d.Parse(raw); !errors.Is(err, instance.ErrInvalidValue) {
			t.Errorf("Parse(%q) = %v, want ErrInvalidValue", raw, err)
		}
	}
	p, err := instance.Lookup(instance.SettingRemoteMediaPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Parse("some"); !errors.Is(err, instance.ErrInvalidValue) {
		t.Errorf("Parse(some) = %v, want ErrInvalidValue", err)
	}
}

func TestAuthorizedFetchSetting(t *testing.T) {
	var zero instance.Settings
	if zero.AuthorizedFetch() {
		t.Error("zero Settings have authorized fetch on, want it off by default")
	}
	s := instance.NewSettings([]instance.CurrentSetting{{Key: instance.SettingAuthorizedFetch, RevisionID: testID(1), Value: "true"}})
	if !s.AuthorizedFetch() {
		t.Error("Settings with authorized_fetch true have it off")
	}
}

func TestFederationModeSetting(t *testing.T) {
	var zero instance.Settings
	if got := zero.FederationMode(); got != instance.FederationBoth {
		t.Errorf("zero Settings federate %q, want both by default", got)
	}
	for _, m := range []instance.FederationMode{instance.FederationOff, instance.FederationInbound, instance.FederationOutbound, instance.FederationBoth} {
		d, err := instance.Lookup(instance.SettingFederationMode)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.Parse(string(m)); err != nil {
			t.Errorf("Parse(%s) = %v", m, err)
		}
		s := instance.NewSettings([]instance.CurrentSetting{{Key: instance.SettingFederationMode, RevisionID: testID(1), Value: string(m)}})
		if got := s.FederationMode(); got != m {
			t.Errorf("FederationMode() = %q, want %q", got, m)
		}
	}
	d, _ := instance.Lookup(instance.SettingFederationMode)
	if _, err := d.Parse("sideways"); !errors.Is(err, instance.ErrInvalidValue) {
		t.Errorf("Parse(sideways) = %v, want ErrInvalidValue", err)
	}
}

func TestDomainActionsVisibilitySettings(t *testing.T) {
	var zero instance.Settings
	if zero.DomainActionsVisibility() != instance.AudienceNone || zero.DomainActionsRationaleVisibility() != instance.AudienceNone {
		t.Errorf("zero Settings list domain actions to %q and their comments to %q, want none by default",
			zero.DomainActionsVisibility(), zero.DomainActionsRationaleVisibility())
	}
	s := instance.NewSettings([]instance.CurrentSetting{
		{Key: instance.SettingDomainActionsVisibility, RevisionID: testID(1), Value: "all"},
		{Key: instance.SettingDomainActionsRationaleVisibility, RevisionID: testID(2), Value: "users"},
	})
	if s.DomainActionsVisibility() != instance.AudienceAll || s.DomainActionsRationaleVisibility() != instance.AudienceUsers {
		t.Errorf("Settings list domain actions to %q and their comments to %q, want all and users",
			s.DomainActionsVisibility(), s.DomainActionsRationaleVisibility())
	}
	for _, tc := range []struct {
		a                   instance.Audience
		anonymous, loggedIn bool
	}{
		{instance.AudienceNone, false, false},
		{instance.AudienceUsers, false, true},
		{instance.AudienceAll, true, true},
		{"unknown", false, false},
	} {
		if got := tc.a.Allows(false); got != tc.anonymous {
			t.Errorf("%q.Allows(anonymous) = %v, want %v", tc.a, got, tc.anonymous)
		}
		if got := tc.a.Allows(true); got != tc.loggedIn {
			t.Errorf("%q.Allows(logged in) = %v, want %v", tc.a, got, tc.loggedIn)
		}
	}
	def, err := instance.Lookup(instance.SettingDomainActionsVisibility)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := def.Parse("everyone"); !errors.Is(err, instance.ErrInvalidValue) {
		t.Errorf("Parse(everyone) = %v, want ErrInvalidValue", err)
	}
}

// The income is private until the setting publishes it, apart from the costs (ADR-0073 §7).
func TestIncomeVisibility(t *testing.T) {
	var zero instance.Settings
	if got := zero.IncomeVisibility(); got != instance.IncomePrivate {
		t.Errorf("zero Settings IncomeVisibility() = %q, want private", got)
	}
	for _, v := range []instance.IncomeVisibility{instance.IncomeTotals, instance.IncomePlans} {
		s := instance.NewSettings([]instance.CurrentSetting{{Key: instance.SettingIncomeVisibility, RevisionID: testID(1), Value: string(v)}})
		if got := s.IncomeVisibility(); got != v {
			t.Errorf("IncomeVisibility() = %q, want %q", got, v)
		}
		if got := s.FinanceVisibility(); got != instance.FinancePrivate {
			t.Errorf("FinanceVisibility() with income_visibility = %s is %q, want private", v, got)
		}
	}
	def, err := instance.Lookup(instance.SettingIncomeVisibility)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := def.Parse("breakdown"); !errors.Is(err, instance.ErrInvalidValue) {
		t.Errorf("Parse(breakdown) = %v, want ErrInvalidValue", err)
	}
}

// The finance viewers read neither the names nor the memos of the cost items, and the income as
// totals, until the settings say otherwise (ADR-0070 §5).
func TestFinanceViewerSettings(t *testing.T) {
	var zero instance.Settings
	if got, want := zero.FinanceViewer(), (instance.FinanceViewer{Income: instance.FinanceViewerIncomeTotals}); got != want {
		t.Errorf("zero Settings FinanceViewer() = %+v, want %+v", got, want)
	}
	s := instance.NewSettings([]instance.CurrentSetting{
		{Key: instance.SettingFinanceViewerItemNames, RevisionID: testID(1), Value: "true"},
		{Key: instance.SettingFinanceViewerItemMemos, RevisionID: testID(2), Value: "true"},
		{Key: instance.SettingFinanceViewerIncome, RevisionID: testID(3), Value: "plans"},
	})
	if got, want := s.FinanceViewer(), (instance.FinanceViewer{ItemNames: true, ItemMemos: true, Income: instance.FinanceViewerIncomePlans}); got != want {
		t.Errorf("FinanceViewer() = %+v, want %+v", got, want)
	}
	def, err := instance.Lookup(instance.SettingFinanceViewerIncome)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := def.Parse("amounts"); !errors.Is(err, instance.ErrInvalidValue) {
		t.Errorf("Parse(amounts) = %v, want ErrInvalidValue", err)
	}
}

// Only the holders of start_voice_rooms open voice chat rooms, rooms show no summary to
// visitors, and their participants are kept 90 days, until the settings say otherwise
// (ADR-0100 §2, §9, §13).
func TestVoiceSettings(t *testing.T) {
	var zero instance.Settings
	if zero.VoiceRoomStarters() != instance.VoiceRoomStartersPermitted || zero.VoicePublicSummary() ||
		zero.Retention(instance.SettingRetentionVoiceParticipants) != 90*24*time.Hour {
		t.Errorf("zero Settings = %s, %v, %v; want permitted, false, 90 days", zero.VoiceRoomStarters(), zero.VoicePublicSummary(),
			zero.Retention(instance.SettingRetentionVoiceParticipants))
	}
	s := instance.NewSettings([]instance.CurrentSetting{
		{Key: instance.SettingVoiceRoomStarters, RevisionID: testID(1), Value: "everyone"},
		{Key: instance.SettingVoicePublicSummary, RevisionID: testID(2), Value: "true"},
		{Key: instance.SettingRetentionVoiceParticipants, RevisionID: testID(3), Value: "30"},
	})
	if s.VoiceRoomStarters() != instance.VoiceRoomStartersEveryone || !s.VoicePublicSummary() ||
		s.Retention(instance.SettingRetentionVoiceParticipants) != 30*24*time.Hour {
		t.Errorf("Settings = %s, %v, %v; want everyone, true, 30 days", s.VoiceRoomStarters(), s.VoicePublicSummary(),
			s.Retention(instance.SettingRetentionVoiceParticipants))
	}
	unexpected := instance.NewSettings([]instance.CurrentSetting{{Key: instance.SettingVoiceRoomStarters, RevisionID: testID(1), Value: "nobody"}})
	if unexpected.VoiceRoomStarters() != instance.VoiceRoomStartersPermitted {
		t.Errorf("an unexpected voice_room_starters = %s, want permitted", unexpected.VoiceRoomStarters())
	}
}

// Only the holders of create_invites create invites of their own, up to 5 usable ones of 10
// uses and 30 days, until the settings say otherwise; the caps keep to their bounds
// (ADR-0108 §1, §2).
func TestMemberInviteSettings(t *testing.T) {
	var zero instance.Settings
	want := instance.MemberInviteCaps{MaxActive: 5, MaxUses: 10, MaxExpiresIn: 30 * 24 * time.Hour}
	if zero.InviteCreators() != instance.InviteCreatorsPermitted || zero.MemberInviteCaps() != want {
		t.Errorf("zero Settings = %s, %+v; want permitted, %+v", zero.InviteCreators(), zero.MemberInviteCaps(), want)
	}
	s := instance.NewSettings([]instance.CurrentSetting{
		{Key: instance.SettingInviteCreators, RevisionID: testID(1), Value: "everyone"},
		{Key: instance.SettingMemberInviteMaxActive, RevisionID: testID(2), Value: "0"},
		{Key: instance.SettingMemberInviteMaxUses, RevisionID: testID(3), Value: "1000"},
		{Key: instance.SettingMemberInviteMaxExpiresIn, RevisionID: testID(4), Value: "3600"},
	})
	want = instance.MemberInviteCaps{MaxActive: 0, MaxUses: 1000, MaxExpiresIn: time.Hour}
	if s.InviteCreators() != instance.InviteCreatorsEveryone || s.MemberInviteCaps() != want {
		t.Errorf("Settings = %s, %+v; want everyone, %+v", s.InviteCreators(), s.MemberInviteCaps(), want)
	}
	for key, bad := range map[instance.SettingKey][]string{
		instance.SettingInviteCreators:           {"nobody", ""},
		instance.SettingMemberInviteMaxActive:    {"-1", "101"},
		instance.SettingMemberInviteMaxUses:      {"0", "1001"},
		instance.SettingMemberInviteMaxExpiresIn: {"3599", "31536001"},
	} {
		def, err := instance.Lookup(key)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range bad {
			if _, err := def.Parse(v); !errors.Is(err, instance.ErrInvalidValue) {
				t.Errorf("%s: Parse(%q) = %v, want ErrInvalidValue", key, v, err)
			}
		}
	}
}

func TestSupersedes(t *testing.T) {
	rev := instance.SettingRevision{ID: testID(2)}
	for _, tc := range []struct {
		cur  string
		want bool
	}{{"", true}, {testID(1), true}, {testID(2), false}, {testID(3), false}} {
		if got := rev.Supersedes(instance.CurrentSetting{RevisionID: tc.cur}); got != tc.want {
			t.Errorf("revision 2 supersedes %q = %v, want %v", tc.cur, got, tc.want)
		}
		if got := (instance.TextRevision{ID: testID(2)}).Supersedes(instance.CurrentText{RevisionID: tc.cur}); got != tc.want {
			t.Errorf("text revision 2 supersedes %q = %v, want %v", tc.cur, got, tc.want)
		}
	}
}

func TestTextValidate(t *testing.T) {
	name, _ := instance.LookupText(instance.TextName)
	desc, _ := instance.LookupText(instance.TextDescription)
	for _, tc := range []struct {
		d    instance.TextDefinition
		body string
		ok   bool
	}{
		{name, "", true},
		{name, "Example", true},
		{name, "a\nb", false},
		{desc, "a\nb", true},
		{name, " \t　", false},
		{name, " Example ", true},
		{desc, "   ", true},
		{name, "\xff", false},
		{desc, "a\x00b", false},
	} {
		if err := tc.d.Validate(tc.body); (err == nil) != tc.ok {
			t.Errorf("Validate(%s, %q) = %v, want ok %v", tc.d.Key, tc.body, err, tc.ok)
		}
	}
}

func TestParseMedia(t *testing.T) {
	d, err := instance.Lookup(instance.SettingPreviewImage)
	if err != nil {
		t.Fatal(err)
	}
	for raw, want := range map[string]string{
		"":                                     "",
		"0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f": "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f",
		"0192F3A4-7B1C-7D2E-9F00-1A2B3C4D5E6F": "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f",
	} {
		if got, err := d.Parse(raw); err != nil || got != want {
			t.Errorf("Parse(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{
		"x", "0192f3a47b1c7d2e9f001a2b3c4d5e6f", "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6g", "0192f3a4_7b1c-7d2e-9f00-1a2b3c4d5e6f",
	} {
		if got, err := d.Parse(raw); !errors.Is(err, instance.ErrInvalidValue) {
			t.Errorf("Parse(%q) = %q, %v; want ErrInvalidValue", raw, got, err)
		}
	}
	if keys := instance.MediaKeys(); !slices.Equal(keys, []instance.SettingKey{instance.SettingPreviewImage, instance.SettingThumbnail}) {
		t.Errorf("MediaKeys() = %v", keys)
	}
	var zero instance.Settings
	if zero.PreviewImageMediaID() != "" {
		t.Errorf("default preview image = %q", zero.PreviewImageMediaID())
	}
}

func TestParseContact(t *testing.T) {
	account, err := instance.Lookup(instance.SettingContactAccount)
	if err != nil {
		t.Fatal(err)
	}
	email, err := instance.Lookup(instance.SettingContactEmail)
	if err != nil {
		t.Fatal(err)
	}
	featured, err := instance.Lookup(instance.SettingFeaturedAccounts)
	if err != nil {
		t.Fatal(err)
	}
	many := strings.Repeat("a ", instance.MaxSettingAccounts)
	var names []string
	for i := range instance.MaxSettingAccounts + 1 {
		names = append(names, fmt.Sprintf("u%d", i))
	}
	tooMany := strings.Join(names, " ")
	for _, tc := range []struct {
		d        instance.Definition
		raw      string
		want     string
		accepted bool
	}{
		{account, "", "", true},
		{account, "Admin_1", "admin_1", true},
		{account, "admin@example.com", "", false},
		{account, "@admin", "", false},
		{account, strings.Repeat("a", 31), "", false},
		{featured, "", "", true},
		{featured, " Alice, @bob\ncarol alice ", "alice bob carol", true},
		{featured, "alice bob@example.com", "", false},
		{featured, many, "a", true},
		{featured, tooMany, "", false},
		{email, "", "", true},
		{email, "admin@example.com", "admin@example.com", true},
		{email, "Admin <admin@example.com>", "", false},
		{email, " admin@example.com", "", false},
		{email, "admin", "", false},
		{email, strings.Repeat("a", 310) + "@example.com", "", false},
	} {
		got, err := tc.d.Parse(tc.raw)
		if tc.accepted && (err != nil || got != tc.want) {
			t.Errorf("Parse(%s, %q) = %q, %v; want %q", tc.d.Key, tc.raw, got, err, tc.want)
		}
		if !tc.accepted && !errors.Is(err, instance.ErrInvalidValue) {
			t.Errorf("Parse(%s, %q) = %q, %v; want ErrInvalidValue", tc.d.Key, tc.raw, got, err)
		}
	}
	settings := instance.NewSettings([]instance.CurrentSetting{
		{Key: instance.SettingContactAccount, Value: "admin"},
		{Key: instance.SettingContactEmail, Value: "admin@example.com"},
		{Key: instance.SettingThumbnail, Value: "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f"},
	})
	if settings.ContactAccount() != "admin" || settings.ContactEmail() != "admin@example.com" || settings.ThumbnailMediaID() != "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f" {
		t.Errorf("contact = %q, %q, thumbnail %q", settings.ContactAccount(), settings.ContactEmail(), settings.ThumbnailMediaID())
	}
	var zero instance.Settings
	if zero.ContactAccount() != "" || zero.ContactEmail() != "" || zero.ThumbnailMediaID() != "" {
		t.Error("the contact and the thumbnail are not empty by default")
	}
}

func TestParseSecurityContact(t *testing.T) {
	d, err := instance.Lookup(instance.SettingSecurityContact)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"", "mailto:security@social.example", "https://social.example/security"} {
		if got, err := d.Parse(raw); err != nil || got != raw {
			t.Errorf("Parse(%q) = %q, %v; want itself", raw, got, err)
		}
	}
	for _, raw := range []string{
		"security@social.example", "http://social.example/security", "https:///security", "mailto:nobody",
		"https://user@social.example/", "https://social.example/a b", "javascript:alert(1)", "https://social.example/\n",
		"https://social.example/" + strings.Repeat("a", instance.MaxURILength),
	} {
		if got, err := d.Parse(raw); !errors.Is(err, instance.ErrInvalidValue) {
			t.Errorf("Parse(%q) = %q, %v; want ErrInvalidValue", raw, got, err)
		}
	}
}

// TestCrawlerPolicy checks ADR-0057: unset settings deny, and an owner only narrows the
// instance's policy.
func TestCrawlerPolicy(t *testing.T) {
	var zero instance.Settings
	if p := zero.CrawlerPolicy(); p != instance.DenyAll || p.IndexAllowed() || !p.DeniesTraining() || !p.DeniesAIUse() || zero.IndexableDefault() {
		t.Errorf("unset policy = %+v, indexable default %v; want everything denied", p, zero.IndexableDefault())
	}
	if (instance.CrawlerPolicy{}).IndexAllowed() || !(instance.CrawlerPolicy{}).DeniesAIUse() {
		t.Errorf("the zero CrawlerPolicy allows something")
	}
	allow := instance.CrawlerPolicy{Search: instance.SearchAllow, AI: instance.AIAllow}
	training := instance.CrawlerPolicy{Search: instance.SearchAllow, AI: instance.AIDenyTraining}
	for _, tc := range []struct {
		policy              instance.CrawlerPolicy
		indexable, aiOptOut bool
		want                instance.CrawlerPolicy
	}{
		{allow, true, false, allow},
		{allow, false, false, instance.CrawlerPolicy{Search: instance.SearchDeny, AI: instance.AIAllow}},
		{allow, true, true, instance.CrawlerPolicy{Search: instance.SearchAllow, AI: instance.AIDeny}},
		{training, true, false, training},
		// An owner cannot allow what the instance denies.
		{instance.DenyAll, true, false, instance.DenyAll},
	} {
		if got := tc.policy.ForOwner(tc.indexable, tc.aiOptOut); got != tc.want {
			t.Errorf("%+v.ForOwner(%v, %v) = %+v, want %+v", tc.policy, tc.indexable, tc.aiOptOut, got, tc.want)
		}
	}
	if !training.DeniesTraining() || training.DeniesAIUse() || allow.DeniesTraining() || allow.DeniesAIUse() {
		t.Errorf("deny_training or allow states the wrong reservations")
	}
	s := instance.NewSettings([]instance.CurrentSetting{
		{Key: instance.SettingSearchIndexing, Value: "allow"}, {Key: instance.SettingAIUsage, Value: "deny_training"},
		{Key: instance.SettingIndexableDefault, Value: "true"},
	})
	if s.CrawlerPolicy() != training || !s.IndexableDefault() {
		t.Errorf("stored policy = %+v, indexable default %v", s.CrawlerPolicy(), s.IndexableDefault())
	}
}
