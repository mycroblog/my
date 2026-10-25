// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InstanceHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the instance repository on it.
type InstanceHarness struct {
	UnitOfWork uow.UnitOfWork
	Instance   appinstance.Repository
}

// InstanceFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type InstanceFactory func(t *testing.T, opts uow.Options) InstanceHarness

// InstanceRepositoryContract verifies the instance repository of a database adapter through the
// use cases that drive it: defaults for unset settings, every change kept as a revision,
// canonical values, rejection of invalid input without writing, texts (one value for every
// locale) with their history, cleared texts, the media files that media settings
// refer to, the property that the synchronous projections equal a rebuild from the
// revisions, the instance rules (instanceRuleCases), the theme (instanceThemeCases), the
// announcements (instanceAnnouncementCases), and the service closure (instanceClosureCases).
func InstanceRepositoryContract(t *testing.T, newHarness InstanceFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *instanceEnv)
	}{
		{"ReadsDefaultsWhenUnset", instanceReadsDefaults},
		{"KeepsSettingRevisions", instanceKeepsSettingRevisions},
		{"StoresCanonicalValues", instanceStoresCanonicalValues},
		{"RejectsInvalidSettings", instanceRejectsInvalidSettings},
		{"SetsSeveralSettingsAtOnce", instanceSetsSeveralSettings},
		{"KeepsTextRevisions", instanceKeepsTextRevisions},
		{"ClearsTexts", instanceClearsTexts},
		{"RejectsInvalidTexts", instanceRejectsInvalidTexts},
		{"IncrementalProjectionEqualsRebuild", instanceProjectionEqualsRebuild},
		{"ReferencesMediaFromSettings", instanceReferencesMedia},
		{"KeepsTheContactAndTheThumbnail", instanceKeepsContactAndThumbnail},
	}
	cases = append(cases, instanceRuleCases...)
	cases = append(cases, instanceThemeCases...)
	cases = append(cases, instanceAnnouncementCases...)
	cases = append(cases, instanceClosureCases...)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newInstanceEnv(t, newHarness)) })
	}
}

type instanceEnv struct {
	h       InstanceHarness
	clock   *Clock
	service *appinstance.Service
}

func newInstanceEnv(t *testing.T, newHarness InstanceFactory) *instanceEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appinstance.RegisterProjectors(projectors, h.Instance)
	clock := NewClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	return &instanceEnv{
		h:     h,
		clock: clock,
		service: appinstance.NewService(appinstance.Deps{
			UnitOfWork: h.UnitOfWork, Repository: h.Instance, Clock: clock, IDs: NewIDGenerator(clock),
		}),
	}
}

func (e *instanceEnv) set(t *testing.T, key instance.SettingKey, value, by string) instance.SettingRevision {
	t.Helper()
	e.clock.Advance(time.Second)
	rev, err := e.service.SetSetting(t.Context(), appinstance.SetSettingInput{Key: key, Value: value, RevisedByUserID: by})
	if err != nil {
		t.Fatalf("SetSetting(%s, %q) = %v", key, value, err)
	}
	return rev
}

func (e *instanceEnv) setText(t *testing.T, key instance.TextKey, body string) instance.TextRevision {
	t.Helper()
	e.clock.Advance(time.Second)
	rev, err := e.service.SetText(t.Context(), appinstance.SetTextInput{Key: key, Body: body})
	if err != nil {
		t.Fatalf("SetText(%s, %q) = %v", key, body, err)
	}
	return rev
}

func (e *instanceEnv) settings(t *testing.T) instance.Settings {
	t.Helper()
	s, err := e.service.Settings(t.Context())
	if err != nil {
		t.Fatalf("Settings = %v", err)
	}
	return s
}

func instanceReadsDefaults(t *testing.T, e *instanceEnv) {
	s := e.settings(t)
	for _, d := range instance.Definitions() {
		if v, stored := s.Value(d.Key); v != d.Default || stored {
			t.Errorf("Value(%s) = %q, %v; want the default %q, false", d.Key, v, stored, d.Default)
		}
	}
	if s.RegistrationMode() != instance.RegistrationApproval || s.MaxStatusLength() != 500 || s.DefaultLocale() != "en" ||
		s.LocalTimelineAccess() != instance.TimelineEveryone || s.FederatedTimelineAccess() != instance.TimelineEveryone {
		t.Errorf("typed defaults = %v %d %q %v %v", s.RegistrationMode(), s.MaxStatusLength(), s.DefaultLocale(),
			s.LocalTimelineAccess(), s.FederatedTimelineAccess())
	}
	texts, err := e.service.Texts(t.Context())
	if err != nil || len(texts) != 0 {
		t.Errorf("Texts = %+v, %v; want none", texts, err)
	}
	if _, ok, err := e.service.Text(t.Context(), instance.TextName); ok || err != nil {
		t.Errorf("Text(name) = _, %v, %v; want not found", ok, err)
	}
}

func instanceKeepsSettingRevisions(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	first := e.set(t, instance.SettingRegistrationMode, "open", "")
	second := e.set(t, instance.SettingRegistrationMode, "closed", "0199f0e0-0000-7000-8000-000000000001")
	e.set(t, instance.SettingMaxStatusLength, "1000", "")

	s := e.settings(t)
	if v, stored := s.Value(instance.SettingRegistrationMode); v != "closed" || !stored {
		t.Errorf("registration_mode = %q, %v; want the latest value closed", v, stored)
	}
	if s.MaxStatusLength() != 1000 {
		t.Errorf("MaxStatusLength = %d, want 1000", s.MaxStatusLength())
	}
	history, err := e.service.SettingHistory(ctx, instance.SettingRegistrationMode, 10)
	if err != nil {
		t.Fatalf("SettingHistory = %v", err)
	}
	if want := []instance.SettingRevision{second, first}; !reflect.DeepEqual(history, want) {
		t.Errorf("history = %+v\nwant      %+v", history, want)
	}
	if one, err := e.service.SettingHistory(ctx, instance.SettingRegistrationMode, 1); err != nil || len(one) != 1 || one[0].ID != second.ID {
		t.Errorf("SettingHistory(limit 1) = %+v, %v; want the newest revision", one, err)
	}
	if _, err := e.service.SettingHistory(ctx, instance.SettingRegistrationMode, 0); !errors.Is(err, instance.ErrInvalidValue) {
		t.Errorf("SettingHistory(limit 0) = %v, want ErrInvalidValue", err)
	}
}

func instanceStoresCanonicalValues(t *testing.T, e *instanceEnv) {
	for _, tc := range []struct {
		key       instance.SettingKey
		raw, want string
	}{
		{instance.SettingAuthorizedFetch, "TRUE", "true"},
		{instance.SettingLocalTimelineAccess, "off", "off"},
		{instance.SettingFederatedTimelineAccess, "logged_in", "logged_in"},
		{instance.SettingMaxStatusLength, "0750", "750"},
		{instance.SettingDefaultLocale, "zh_hant_tw", "zh-Hant-TW"},
		{instance.SettingRegistrationMode, "invite", "invite"},
	} {
		if rev := e.set(t, tc.key, tc.raw, ""); rev.Value != tc.want {
			t.Errorf("SetSetting(%s, %q) stored %q, want %q", tc.key, tc.raw, rev.Value, tc.want)
		}
		if v, _ := e.settings(t).Value(tc.key); v != tc.want {
			t.Errorf("Value(%s) = %q, want %q", tc.key, v, tc.want)
		}
	}
	s := e.settings(t)
	if !s.AuthorizedFetch() || s.LocalTimelineAccess() != instance.TimelineOff ||
		s.FederatedTimelineAccess() != instance.TimelineLoggedIn || s.MaxStatusLength() != 750 ||
		s.DefaultLocale() != "zh-Hant-TW" || s.RegistrationMode() != instance.RegistrationInvite {
		t.Errorf("typed values = %v %v %v %d %q %v", s.AuthorizedFetch(), s.LocalTimelineAccess(), s.FederatedTimelineAccess(),
			s.MaxStatusLength(), s.DefaultLocale(), s.RegistrationMode())
	}
}

func instanceRejectsInvalidSettings(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	for _, tc := range []struct {
		key  instance.SettingKey
		raw  string
		want error
	}{
		{"no_such_key", "1", instance.ErrUnknownKey},
		{instance.SettingRegistrationMode, "everyone", instance.ErrInvalidValue},
		{instance.SettingMaxStatusLength, "0", instance.ErrInvalidValue},
		{instance.SettingMaxStatusLength, "100001", instance.ErrInvalidValue},
		{instance.SettingMaxStatusLength, "lots", instance.ErrInvalidValue},
		{instance.SettingLocalTimelineAccess, "true", instance.ErrInvalidValue},
		{instance.SettingAuthorizedFetch, "yes", instance.ErrInvalidValue},
		{instance.SettingDefaultLocale, "", instance.ErrInvalidValue},
		{instance.SettingDefaultLocale, "not a locale", instance.ErrInvalidValue},
	} {
		_, err := e.service.SetSetting(ctx, appinstance.SetSettingInput{Key: tc.key, Value: tc.raw})
		if !errors.Is(err, tc.want) {
			t.Errorf("SetSetting(%s, %q) = %v, want %v", tc.key, tc.raw, err, tc.want)
		}
	}
	cur, err := e.h.Instance.ListCurrentSettings(ctx)
	if err != nil || len(cur) != 0 {
		t.Errorf("ListCurrentSettings = %+v, %v; want nothing written", cur, err)
	}
	if _, err := e.service.SettingHistory(ctx, "no_such_key", 10); !errors.Is(err, instance.ErrUnknownKey) {
		t.Errorf("SettingHistory(unknown) = %v, want ErrUnknownKey", err)
	}
}

// instanceSetsSeveralSettings checks SetSettings: one rejected value writes nothing and names
// its key; otherwise every value becomes current, canonical, and kept as a revision by the user.
func instanceSetsSeveralSettings(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	_, err := e.service.SetSettings(ctx, []appinstance.SetSettingInput{
		{Key: instance.SettingRegistrationMode, Value: "invite"},
		{Key: instance.SettingLocalTimelineAccess, Value: "maybe"},
	})
	var se *appinstance.SettingError
	if !errors.As(err, &se) || se.Key != instance.SettingLocalTimelineAccess || !errors.Is(err, instance.ErrInvalidValue) {
		t.Errorf("SetSettings(invalid local_timeline_access) = %v, want a SettingError of that key wrapping ErrInvalidValue", err)
	}
	if cur, err := e.h.Instance.ListCurrentSettings(ctx); err != nil || len(cur) != 0 {
		t.Errorf("ListCurrentSettings = %+v, %v; want nothing written", cur, err)
	}

	e.clock.Advance(time.Second)
	revs, err := e.service.SetSettings(ctx, []appinstance.SetSettingInput{
		{Key: instance.SettingRegistrationMode, Value: "invite", RevisedByUserID: NamedID("admin")},
		{Key: instance.SettingDefaultLocale, Value: "ja_jp", RevisedByUserID: NamedID("admin")},
		{Key: instance.SettingFederatedTimelineAccess, Value: "off", RevisedByUserID: NamedID("admin")},
	})
	if err != nil || len(revs) != 3 {
		t.Fatalf("SetSettings = %+v, %v; want 3 revisions", revs, err)
	}
	s := e.settings(t)
	if s.RegistrationMode() != instance.RegistrationInvite || s.DefaultLocale() != "ja-JP" || s.FederatedTimelineAccess() != instance.TimelineOff {
		t.Errorf("settings = %v %q %v; want invite, ja-JP, off", s.RegistrationMode(), s.DefaultLocale(), s.FederatedTimelineAccess())
	}
	history, err := e.service.SettingHistory(ctx, instance.SettingDefaultLocale, 10)
	if err != nil || !reflect.DeepEqual(history, revs[1:2]) || history[0].RevisedByUserID != NamedID("admin") {
		t.Errorf("SettingHistory(default_locale) = %+v, %v; want [%+v]", history, err, revs[1])
	}
}

func instanceKeepsTextRevisions(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	first := e.setText(t, instance.TextDescription, "最初の説明")
	second := e.setText(t, instance.TextDescription, "Second description\nwith a line break")
	e.setText(t, instance.TextName, "Example Social")

	history, err := e.service.TextHistory(ctx, instance.TextDescription, 10)
	if err != nil {
		t.Fatalf("TextHistory = %v", err)
	}
	if want := []instance.TextRevision{second, first}; !reflect.DeepEqual(history, want) {
		t.Errorf("history = %+v\nwant      %+v", history, want)
	}
	if _, err := e.service.TextHistory(ctx, "motto", 10); !errors.Is(err, instance.ErrUnknownKey) {
		t.Errorf("TextHistory(unknown) = %v, want ErrUnknownKey", err)
	}
	texts, err := e.service.Texts(ctx)
	if err != nil {
		t.Fatalf("Texts = %v", err)
	}
	var got []string
	for _, x := range texts {
		got = append(got, fmt.Sprintf("%s=%s", x.Key, x.Body))
	}
	want := []string{"description=Second description\nwith a line break", "name=Example Social"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Texts = %q, want %q", got, want)
	}
	if texts[0].RevisionID != second.ID || !texts[0].RevisedAt.Equal(second.RevisedAt) {
		t.Errorf("current description = %+v, want revision %+v", texts[0], second)
	}
	if x, ok, err := e.service.Text(ctx, instance.TextDescription); err != nil || !ok || x.Body != second.Body {
		t.Errorf("Text(description) = %+v, %v, %v; want %q", x, ok, err, second.Body)
	}
}

// instanceClearsTexts checks that an empty body clears a text, which is then not found and
// not listed, and that a later body sets it again.
func instanceClearsTexts(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	e.setText(t, instance.TextName, "Example")
	e.setText(t, instance.TextName, "")
	if x, ok, err := e.service.Text(ctx, instance.TextName); ok || err != nil {
		t.Errorf("Text(cleared name) = %+v, %v, %v; want not found", x, ok, err)
	}
	if texts, err := e.service.Texts(ctx); err != nil || len(texts) != 0 {
		t.Errorf("Texts = %+v, %v; want none", texts, err)
	}
	e.setText(t, instance.TextName, "例")
	if x, ok, err := e.service.Text(ctx, instance.TextName); !ok || err != nil || x.Body != "例" {
		t.Errorf("Text(name) = %+v, %v, %v; want 例", x, ok, err)
	}
}

func instanceRejectsInvalidTexts(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	for _, tc := range []struct {
		in   appinstance.SetTextInput
		want error
	}{
		{appinstance.SetTextInput{Key: "motto", Body: "x"}, instance.ErrUnknownKey},
		{appinstance.SetTextInput{Key: instance.TextName, Body: "two\nlines"}, instance.ErrInvalidValue},
		{appinstance.SetTextInput{Key: instance.TextName, Body: "   "}, instance.ErrInvalidValue},
		{appinstance.SetTextInput{Key: instance.TextName, Body: strings.Repeat("a", 101)}, instance.ErrInvalidValue},
		{appinstance.SetTextInput{Key: instance.TextName, Body: "\xff"}, instance.ErrInvalidValue},
	} {
		if _, err := e.service.SetText(ctx, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("SetText(%+v) = %v, want %v", tc.in, err, tc.want)
		}
	}
	all, err := e.h.Instance.ListAllCurrentTexts(ctx)
	if err != nil || len(all) != 0 {
		t.Errorf("ListAllCurrentTexts = %+v, %v; want nothing written", all, err)
	}
	// The longest allowed name is accepted, counted in characters.
	e.setText(t, instance.TextName, strings.Repeat("あ", 100))
}

func instanceProjectionEqualsRebuild(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	rng := rand.New(rand.NewPCG(98, 20261016)) //nolint:gosec // deterministic test input, not a secret
	defs := instance.Definitions()
	texts := instance.TextDefinitions()
	locales := []string{"en", "ja", "ja-JP", "zh-Hant", "fr"}
	values := func(d instance.Definition) string {
		switch d.Kind {
		case instance.KindBool:
			return fmt.Sprint(rng.IntN(2) == 0)
		case instance.KindInt:
			return fmt.Sprint(d.Min + rng.IntN(min(1000, d.Max-d.Min+1)))
		case instance.KindEnum:
			return d.Choices[rng.IntN(len(d.Choices))]
		case instance.KindMedia:
			return fmt.Sprintf("0199f0e0-0000-7000-8000-%012x", rng.IntN(3))
		case instance.KindAccount:
			return []string{"", "admin", "Moderator_1"}[rng.IntN(3)]
		case instance.KindEmail:
			return []string{"", "admin@example.com"}[rng.IntN(2)]
		case instance.KindURI:
			return fmt.Sprintf("mailto:security%d@social.example", rng.IntN(3))
		default:
			return locales[rng.IntN(len(locales))]
		}
	}
	for round := range 3 {
		for step := range 40 {
			if rng.IntN(2) == 0 {
				d := defs[rng.IntN(len(defs))]
				e.set(t, d.Key, values(d), "")
			} else {
				body := ""
				if rng.IntN(4) > 0 {
					body = fmt.Sprintf("text %d.%d", round, step)
				}
				e.setText(t, texts[rng.IntN(len(texts))].Key, body)
			}
		}
		incremental := snapshotInstanceProjections(t, e.h.Instance)
		if len(incremental.settings) == 0 || len(incremental.texts) == 0 {
			t.Fatalf("round %d: projections are empty: %+v", round, incremental)
		}
		if err := appinstance.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Instance); err != nil {
			t.Fatalf("RebuildProjections = %v", err)
		}
		rebuilt := snapshotInstanceProjections(t, e.h.Instance)
		if !reflect.DeepEqual(incremental, rebuilt) {
			t.Fatalf("round %d: incremental projections differ from a rebuild:\nincremental %+v\nrebuilt     %+v", round, incremental, rebuilt)
		}
	}
}

type instanceProjections struct {
	settings []instance.CurrentSetting
	texts    []instance.Text
}

func snapshotInstanceProjections(t *testing.T, repo appinstance.Repository) instanceProjections {
	t.Helper()
	settings, err := repo.ListCurrentSettings(t.Context())
	if err != nil {
		t.Fatalf("ListCurrentSettings = %v", err)
	}
	texts, err := repo.ListAllCurrentTexts(t.Context())
	if err != nil {
		t.Fatalf("ListAllCurrentTexts = %v", err)
	}
	return instanceProjections{settings: settings, texts: texts}
}

func instanceReferencesMedia(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	const (
		first  = "0199f0e0-0000-7000-8000-00000000f001"
		second = "0199f0e0-0000-7000-8000-00000000f002"
		other  = "0199f0e0-0000-7000-8000-00000000f003"
	)
	if p, err := e.service.PreviewImage(ctx); err != nil || p != (appinstance.Image{}) {
		t.Errorf("PreviewImage unset = %+v, %v; want none", p, err)
	}
	e.set(t, instance.SettingPreviewImage, strings.ToUpper(first), "")
	e.set(t, instance.SettingPreviewImage, second, "")
	e.setText(t, instance.TextPreviewImageAlt, "A mascot")
	if p, err := e.service.PreviewImage(ctx); err != nil || p.MediaID != second || p.Alt != "A mascot" {
		t.Errorf("PreviewImage = %+v, %v; want %s with its alt text", p, err, second)
	}
	// Every revision keeps its file, so that the setting can go back to an earlier one.
	for id, want := range map[string]bool{first: true, second: true, other: false} {
		if got, err := e.service.IsMediaReferenced(ctx, id); err != nil || got != want {
			t.Errorf("IsMediaReferenced(%s) = %v, %v; want %v", id, got, err, want)
		}
	}
	e.set(t, instance.SettingPreviewImage, "", "")
	if p, err := e.service.PreviewImage(ctx); err != nil || p != (appinstance.Image{}) {
		t.Errorf("PreviewImage cleared = %+v, %v; want none", p, err)
	}
	// "" is the cleared value, never a media file.
	if got, err := e.service.IsMediaReferenced(ctx, ""); err != nil || got {
		t.Errorf("IsMediaReferenced(\"\") = %v, %v; want false", got, err)
	}
}

// contactAccounts is the LocalAccounts of instanceKeepsContactAndThumbnail: the local accounts
// are the usernames it holds.
type contactAccounts []string

func (c contactAccounts) CheckLocalAccount(_ context.Context, username string) error {
	if !slices.Contains(c, username) {
		return fmt.Errorf("no local account %s", username)
	}
	return nil
}

func instanceKeepsContactAndThumbnail(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	const thumbnail = "0199f0e0-0000-7000-8000-00000000f004"
	service := appinstance.NewService(appinstance.Deps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Instance, Clock: e.clock, IDs: NewIDGenerator(e.clock),
		Accounts: contactAccounts{"admin"},
	})
	if _, err := service.SetSettings(ctx, []appinstance.SetSettingInput{
		{Key: instance.SettingContactAccount, Value: "Admin"},
		{Key: instance.SettingContactEmail, Value: "admin@example.com"},
		{Key: instance.SettingThumbnail, Value: thumbnail},
	}); err != nil {
		t.Fatalf("SetSettings = %v", err)
	}
	settings := e.settings(t)
	if settings.ContactAccount() != "admin" || settings.ContactEmail() != "admin@example.com" || settings.ThumbnailMediaID() != thumbnail {
		t.Errorf("contact %q %q, thumbnail %q", settings.ContactAccount(), settings.ContactEmail(), settings.ThumbnailMediaID())
	}
	// An account that is not local is refused, and nothing is changed.
	var se *appinstance.SettingError
	if _, err := service.SetSettings(ctx, []appinstance.SetSettingInput{
		{Key: instance.SettingContactEmail, Value: "other@example.com"},
		{Key: instance.SettingContactAccount, Value: "nobody"},
	}); !errors.As(err, &se) || se.Key != instance.SettingContactAccount || !errors.Is(err, instance.ErrInvalidValue) {
		t.Errorf("SetSettings(unknown account) = %v, want a SettingError of contact_account", err)
	}
	if got := e.settings(t).ContactEmail(); got != "admin@example.com" {
		t.Errorf("contact_email after a refused change = %q", got)
	}
	e.setText(t, instance.TextThumbnailAlt, "The beach")
	if img, err := service.Thumbnail(ctx); err != nil || img.MediaID != thumbnail || img.Alt != "The beach" {
		t.Errorf("Thumbnail = %+v, %v", img, err)
	}
	if used, err := service.IsMediaReferenced(ctx, thumbnail); err != nil || !used {
		t.Errorf("IsMediaReferenced(thumbnail) = %v, %v; want true", used, err)
	}
	e.set(t, instance.SettingThumbnail, "", "")
	if img, err := service.Thumbnail(ctx); err != nil || img != (appinstance.Image{}) {
		t.Errorf("Thumbnail cleared = %+v, %v; want none", img, err)
	}
}
