// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakePreferences holds the preferences of one user and applies updates without validating
// them, unless err is set.
type fakePreferences struct {
	userID string
	p      appidentity.UserPreferences
	err    error
	// suppression is the suppression of the user's address, if any.
	suppression *identity.EmailSuppression
}

func (f *fakePreferences) PrimaryEmailSuppression(_ context.Context, userID string) (identity.EmailSuppression, bool, error) {
	if userID != f.userID || f.suppression == nil {
		return identity.EmailSuppression{}, false, nil
	}
	return *f.suppression, true, nil
}

func (f *fakePreferences) Preferences(_ context.Context, userID string) (appidentity.UserPreferences, error) {
	if userID != f.userID {
		return appidentity.UserPreferences{}, appidentity.ErrNotFound
	}
	return f.p, nil
}

func (f *fakePreferences) UpdatePreferences(ctx context.Context, userID string, in appidentity.PreferencesUpdate) (appidentity.UserPreferences, error) {
	if f.err != nil {
		return appidentity.UserPreferences{}, f.err
	}
	p, err := f.Preferences(ctx, userID)
	if err != nil {
		return p, err
	}
	if in.Locale != nil {
		p.Locale = *in.Locale
	}
	if in.DefaultVisibility != nil {
		p.DefaultVisibility = *in.DefaultVisibility
	}
	for k, set := range in.DefaultInteractionPolicy {
		p.DefaultInteractionPolicy = p.DefaultInteractionPolicy.With(k, set)
	}
	if in.TimelineLanguages != nil {
		p.TimelineLanguages = *in.TimelineLanguages
	}
	if in.TimelineAutoInsert != nil {
		p.TimelineAutoInsert = *in.TimelineAutoInsert
	}
	if in.LinkCardSize != nil {
		p.LinkCardSize = *in.LinkCardSize
	}
	f.p = p
	return p, nil
}

func newPreferencesEnv(t *testing.T) (*authEnv, *fakePreferences) {
	t.Helper()
	prefs := &fakePreferences{userID: userOf(aliceAccountID), p: appidentity.UserPreferences{
		Preferences: identity.Preferences{
			UserID: userOf(aliceAccountID), Locale: "ja", TimeZone: "Asia/Tokyo", TimeZoneChosen: true,
			LocaleFollowsBrowser: true, TimeZoneFollowsBrowser: true,
			DefaultVisibility: identity.Visibility("unlisted"), LinkCardSize: identity.LinkCardLarge, DefaultInteractionPolicy: identity.DefaultInteractionDefaults, DefaultPostLanguage: "ja",
		},
	}}
	return newAuthEnv(t, func(o *clientapi.Options) { o.Preferences = prefs }), prefs
}

func TestGetPreferences(t *testing.T) {
	e, _ := newPreferencesEnv(t)
	res := e.get(t, "/preferences", map[string]string{"Authorization": "Bearer read-accounts"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var got clientapi.Preferences
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	want := clientapi.Preferences{
		Locale: "ja", TimeZone: "Asia/Tokyo", TimeZoneChosen: true, LocaleFollowsBrowser: true, TimeZoneFollowsBrowser: true, DefaultVisibility: clientapi.VisibilityUnlisted, DefaultPostLanguage: "ja",
		DefaultInteractionPolicy: clientapi.InteractionPolicy{
			Reply: clientapi.InteractionRule{Automatic: clientapi.InteractionAudienceEveryone}, Announce: clientapi.InteractionRule{Automatic: clientapi.InteractionAudienceEveryone},
			Quote: clientapi.InteractionRule{Automatic: clientapi.InteractionAudienceEveryone},
		},
		LinkCardSize: clientapi.LinkCardSizeLarge, TimelineLanguages: []string{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("preferences = %+v, want %+v", got, want)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if res := e.get(t, "/preferences", map[string]string{"Authorization": "Bearer write"}); res.StatusCode != http.StatusForbidden {
		t.Errorf("write token = %d, want 403", res.StatusCode)
	}
}

func TestPreferencesShowAnUndeliverableAddress(t *testing.T) {
	e, prefs := newPreferencesEnv(t)
	since := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	prefs.suppression = &identity.EmailSuppression{Kind: identity.EmailHardBounce, FailureID: "f", Since: since}
	for _, res := range []*http.Response{
		e.get(t, "/preferences", map[string]string{"Authorization": "Bearer read-accounts"}),
		e.sendJSON(t, http.MethodPatch, "/preferences", "write", `{}`),
	} {
		var got clientapi.Preferences
		if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusOK || got.EmailSuppression == nil ||
			got.EmailSuppression.Kind != clientapi.EmailSuppressionKindHardBounce || !got.EmailSuppression.StateChangedAt.Equal(since) {
			t.Errorf("%s = %d %+v, want the hard bounce", res.Request.Method, res.StatusCode, got.EmailSuppression)
		}
	}
}

func TestUpdatePreferences(t *testing.T) {
	e, prefs := newPreferencesEnv(t)
	res := e.sendJSON(t, http.MethodPatch, "/preferences", "write", `{"default_visibility":"local","default_interaction_policy":{"quote":{"automatic":"followers","manual":"everyone"}},"timeline_languages":["en","ja"],"timeline_auto_insert":true,"link_card_size":"small"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var got clientapi.Preferences
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.LinkCardSize != clientapi.LinkCardSizeSmall || prefs.p.LinkCardSize != identity.LinkCardSmall {
		t.Errorf("link card size = %q, stored %q, want small", got.LinkCardSize, prefs.p.LinkCardSize)
	}
	if got.DefaultVisibility != clientapi.VisibilityLocal || !reflect.DeepEqual(got.TimelineLanguages, []string{"en", "ja"}) || got.Locale != "ja" ||
		!got.TimelineAutoInsert || !prefs.p.TimelineAutoInsert {
		t.Errorf("preferences = %+v, want the change applied to the rest", got)
	}
	quote := identity.InteractionSetting{Automatic: identity.AudienceFollowers, Manual: identity.AudienceEveryone}
	if prefs.p.DefaultVisibility != "local" || prefs.p.DefaultInteractionPolicy.Quote != quote || prefs.p.DefaultInteractionPolicy.Reply != identity.DefaultInteractionDefaults.Reply ||
		got.DefaultInteractionPolicy.Quote.Automatic != clientapi.InteractionAudienceFollowers || got.DefaultInteractionPolicy.Quote.Manual == nil {
		t.Errorf("stored visibility = %q, interaction policy = %+v (answered %+v)", prefs.p.DefaultVisibility, prefs.p.DefaultInteractionPolicy, got.DefaultInteractionPolicy)
	}
	if res := e.sendJSON(t, http.MethodPatch, "/preferences", "read", `{}`); res.StatusCode != http.StatusForbidden {
		t.Errorf("read token = %d, want 403", res.StatusCode)
	}

	for _, tt := range []struct {
		err        error
		wantDetail string
	}{
		{&appidentity.PreferenceError{Preference: "time_zone", Err: identity.ErrInvalidTimeZone}, "time_zone の値が正しくありません。"},
		{&appidentity.PreferenceError{Preference: "timeline_languages", Err: fmt.Errorf("%w: at most 64", identity.ErrTooManyLanguages)},
			"timeline_languages は 64 個までにしてください。"},
	} {
		prefs.err = tt.err
		res := e.sendJSON(t, http.MethodPatch, "/preferences", "write", `{"time_zone":"Mars/Olympus"}`)
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("%v: status = %d, want 422", tt.err, res.StatusCode)
		}
		if p := decodeProblem(t, res); p.Detail == nil || *p.Detail != tt.wantDetail {
			t.Errorf("%v: problem = %+v (detail %v), want %q", tt.err, p, p.Detail, tt.wantDetail)
		}
	}
}
