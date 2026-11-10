// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeSettings keeps instance settings with the semantics of appinstance.Service.SetSettings:
// every value is checked before any is stored.
type fakeSettings struct {
	stored []instance.CurrentSetting
	sets   [][]appinstance.SetSettingInput
	// accounts, when set, is how many local accounts refuse turning single_user on.
	accounts int
}

func (f *fakeSettings) Settings(context.Context) (instance.Settings, error) {
	return instance.NewSettings(f.stored), nil
}

func (f *fakeSettings) SetSettings(_ context.Context, ins []appinstance.SetSettingInput) ([]instance.SettingRevision, error) {
	revs := make([]instance.SettingRevision, len(ins))
	for i, in := range ins {
		d, err := instance.Lookup(in.Key)
		if err != nil {
			return nil, &appinstance.SettingError{Key: in.Key, Err: err}
		}
		v, err := d.Parse(in.Value)
		if err != nil {
			return nil, &appinstance.SettingError{Key: in.Key, Err: err}
		}
		if in.Key == instance.SettingSingleUser && v == "true" && f.accounts != 0 {
			return nil, &appinstance.SingleUserAccountsError{Count: f.accounts}
		}
		revs[i] = instance.SettingRevision{Key: in.Key, Value: v, RevisedByUserID: in.RevisedByUserID}
	}
	f.sets = append(f.sets, ins)
	for _, r := range revs {
		f.stored = slices.DeleteFunc(f.stored, func(c instance.CurrentSetting) bool { return c.Key == r.Key })
		f.stored = append(f.stored, instance.CurrentSetting{Key: r.Key, Value: r.Value})
	}
	return revs, nil
}

// newSettingsEnv is newAuthEnv with the tokens of newAdminEnv and the instance settings: alice
// holds every permission and bob none.
func newSettingsEnv(t *testing.T) (*authEnv, *fakeSettings) {
	t.Helper()
	f := &fakeSettings{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.Settings = f
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	return e, f
}

func settingItem(t *testing.T, list clientapi.AdminSettingList, key clientapi.InstanceSettingKey) clientapi.AdminSetting {
	t.Helper()
	i := slices.IndexFunc(list.Items, func(s clientapi.AdminSetting) bool { return s.Key == key })
	if i < 0 {
		t.Fatalf("no setting %s in %+v", key, list.Items)
	}
	return list.Items[i]
}

func TestInstanceSettingOperationsNeedAPermission(t *testing.T) {
	e, f := newSettingsEnv(t)
	for _, op := range []struct{ method, body string }{
		{http.MethodGet, ""},
		{http.MethodPatch, `{"values":{"registration_mode":"open"}}`},
	} {
		res := e.send(t, op.method, "/admin/settings", "bob-admin", op.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
			p.Detail == nil || !strings.Contains(*p.Detail, "manage_settings") {
			t.Errorf("%s by bob = %d %+v, want 403 for manage_settings", op.method, res.StatusCode, p)
		}
	}
	res := e.sendJSON(t, http.MethodPatch, "/admin/settings", "admin-read", `{"values":{"registration_mode":"open"}}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
		t.Errorf("patch with admin:read = %d %+v, want 403 insufficient-scope", res.StatusCode, p)
	}
	if len(f.sets) != 0 {
		t.Error("a refused operation changed a setting")
	}
}

func TestManageInstanceSettings(t *testing.T) {
	e, f := newSettingsEnv(t)
	res := e.admin(t, http.MethodGet, "/admin/settings", "admin-read", nil)
	list := decodeJSON[clientapi.AdminSettingList](t, res)
	if res.StatusCode != http.StatusOK || len(list.Items) != len(instance.Definitions()) {
		t.Fatalf("list = %d with %d items, want every setting", res.StatusCode, len(list.Items))
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	mode := settingItem(t, list, clientapi.InstanceSettingKeyRegistrationMode)
	if mode.Value != "approval" || mode.Default != "approval" || mode.Stored || mode.Kind != clientapi.InstanceSettingKindEnum ||
		mode.Choices == nil || !slices.Equal(*mode.Choices, []string{"open", "approval", "invite", "closed"}) || mode.Minimum != nil {
		t.Errorf("registration_mode = %+v, want the default with its choices", mode)
	}
	ttl := settingItem(t, list, clientapi.InstanceSettingKeyRemoteMediaCacheTtlSeconds)
	if ttl.Kind != clientapi.InstanceSettingKindInt || ttl.Minimum == nil || *ttl.Minimum != 60 || ttl.Maximum == nil || *ttl.Maximum != 31536000 || ttl.Choices != nil {
		t.Errorf("remote_media_cache_ttl_seconds = %+v, want its bounds", ttl)
	}

	res = e.sendJSON(t, http.MethodPatch, "/admin/settings", "admin",
		`{"values":{"authorized_fetch":"TRUE","registration_mode":"invite"}}`)
	list = decodeJSON[clientapi.AdminSettingList](t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("patch = %d", res.StatusCode)
	}
	if got := settingItem(t, list, clientapi.InstanceSettingKeyRegistrationMode); got.Value != "invite" || !got.Stored {
		t.Errorf("registration_mode after the change = %+v, want invite, stored", got)
	}
	if got := settingItem(t, list, clientapi.InstanceSettingKeyAuthorizedFetch); got.Value != "true" {
		t.Errorf("authorized_fetch after the change = %+v, want the canonical true", got)
	}
	// The values are set together, in the order of the definitions, by alice.
	want := []appinstance.SetSettingInput{
		{Key: instance.SettingRegistrationMode, Value: "invite", RevisedByUserID: userOf(aliceAccountID)},
		{Key: instance.SettingAuthorizedFetch, Value: "TRUE", RevisedByUserID: userOf(aliceAccountID)},
	}
	if len(f.sets) != 1 || !slices.Equal(f.sets[0], want) {
		t.Errorf("sets = %+v, want one call of %+v", f.sets, want)
	}
}

func TestInstanceSettingProblems(t *testing.T) {
	e, f := newSettingsEnv(t)
	for _, tc := range []struct{ name, body, detail string }{
		{"unknown key", `{"values":{"registration_mode":"open","motto":"x"}}`, "values.motto"},
		{"invalid value", `{"values":{"registration_mode":"open","max_status_length":"0"}}`, "values.max_status_length"},
	} {
		res := e.sendJSON(t, http.MethodPatch, "/admin/settings", "admin", tc.body)
		p := decodeProblem(t, res)
		if res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, tc.detail) {
			t.Errorf("%s: %d %+v, want 422 naming %q", tc.name, res.StatusCode, p, tc.detail)
		}
	}
	if len(f.stored) != 0 {
		t.Errorf("a refused change was stored: %+v", f.stored)
	}
}

// TestSingleUserNeedsOneAccount checks the 422 of turning single_user on while the server has more
// than one local account (ADR-0152 §1).
func TestSingleUserNeedsOneAccount(t *testing.T) {
	e, f := newSettingsEnv(t)
	f.accounts = 3
	res := e.sendJSON(t, http.MethodPatch, "/admin/settings", "admin", `{"values":{"single_user":"true"}}`)
	p := decodeProblem(t, res)
	if res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeSingleUserRequiresOneAccount {
		t.Fatalf("single_user with 3 accounts = %d %+v, want the 422 single-user-requires-one-account", res.StatusCode, p)
	}
	if p.Detail == nil || *p.Detail != "このサーバーにはローカルのアカウントが 3 個あります。" {
		t.Errorf("detail = %v, want the count", p.Detail)
	}
	if len(f.stored) != 0 {
		t.Errorf("a refused change was stored: %+v", f.stored)
	}
	f.accounts = 0
	if res := e.sendJSON(t, http.MethodPatch, "/admin/settings", "admin", `{"values":{"single_user":"true"}}`); res.StatusCode != http.StatusOK {
		t.Errorf("single_user with one account = %d, want 200", res.StatusCode)
	}
}

// TestMediaSettingsCarryTheirImage checks that a media setting with a processed image carries it in
// the admin list, so that the admin screen can show a preview_image uploaded by anyone, and that a
// media file that is not one, or an empty setting, carries none.
func TestMediaSettingsCarryTheirImage(t *testing.T) {
	f := &fakeSettings{stored: []instance.CurrentSetting{
		{Key: instance.SettingPreviewImage, Value: "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f"},
		{Key: instance.SettingThumbnail, Value: "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e70"},
	}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.Settings = f
		o.MediaImage = func(_ context.Context, id string) (*clientapi.InstanceImage, error) {
			if id != "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f" {
				return nil, nil // still processing
			}
			return &clientapi.InstanceImage{URL: "https://social.example/media/p/large.jpg", Width: 1200, Height: 630, Alt: "ignored"}, nil
		}
	})
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	list := decodeJSON[clientapi.AdminSettingList](t, e.admin(t, http.MethodGet, "/admin/settings", "admin-read", nil))
	img := settingItem(t, list, clientapi.InstanceSettingKeyPreviewImage).Image
	if img == nil || img.Url != "https://social.example/media/p/large.jpg" || img.Width != 1200 || img.Height != 630 {
		t.Errorf("preview_image image = %+v, want the large variant", img)
	}
	if img := settingItem(t, list, clientapi.InstanceSettingKeyThumbnail).Image; img != nil {
		t.Errorf("thumbnail still processing has image %+v, want none", img)
	}
	if img := settingItem(t, list, clientapi.InstanceSettingKeyRegistrationMode).Image; img != nil {
		t.Errorf("an enum setting has image %+v", img)
	}
}
