// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/pkg/testkit"
)

func newServer(t *testing.T) (*clientapi.Server, http.Handler) {
	t.Helper()
	return newServerWithLocales(t, func(b *i18n.Bundle) *i18n.Resolver { return &i18n.Resolver{Bundle: b} })
}

func newServerWithLocales(t *testing.T, locales func(*i18n.Bundle) *i18n.Resolver) (*clientapi.Server, http.Handler) {
	t.Helper()
	messages, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	s := clientapi.New(clientapi.Options{
		Host: "social.example", SoftwareVersion: "1.2.3",
		SourceURL: "https://git.example/my",
		Locales:   locales(messages),
		RequestID: func(context.Context) string { return "req-1" },
		Logger:    slog.New(slog.DiscardHandler),
	})
	mux := http.NewServeMux()
	s.Register(mux)
	return s, mux
}

func TestGetInstance(t *testing.T) {
	_, h := newServer(t)
	spec := apitest.Load(t)

	res := spec.Serve(t, h, httptest.NewRequest(http.MethodGet, clientapi.BasePath+"/instance", nil))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var got clientapi.Instance
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	// Without stored settings and texts, the defaults apply and the instance is named after its host.
	want := clientapi.Instance{
		Domain: "social.example", Name: "social.example", Version: "1.2.3", SourceUrl: "https://git.example/my",
		Registrations: clientapi.InstanceRegistrations{Mode: clientapi.InstanceRegistrationsModeApproval},
		Limits:        clientapi.InstanceLimits{MaxPostCharacters: 500},
		Timelines:     clientapi.InstanceTimelines{Local: clientapi.TimelineAccessEveryone, Federated: clientapi.TimelineAccessEveryone},
		Rules:         []clientapi.InstanceRule{},
		// Unset, the instance federates both ways (ADR-0076).
		Federation: clientapi.InstanceFederation{Mode: clientapi.InstanceFederationModeBoth},
		// Unset, the instance denies search engines and AI (ADR-0057).
		SearchIndexing: clientapi.SearchIndexingDeny, AiUsage: clientapi.AIUsageDeny,
		// Unset, the costs are private (ADR-0067 §8).
		FinanceVisibility:       clientapi.FinanceVisibilityPrivate,
		IncomeVisibility:        clientapi.IncomeVisibilityPrivate,
		SupporterListVisibility: clientapi.SupporterListVisibilityOff,
		VoiceRoomStarters:       clientapi.InstanceVoiceRoomStartersPermitted,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("instance = %+v, want %+v", got, want)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("Cache-Control = %q, want public, max-age=60", cc)
	}
	if v := res.Header.Values("Vary"); !slices.Equal(v, []string{"Authorization", "Accept-Language"}) {
		t.Errorf("Vary = %q, want Authorization and Accept-Language", v)
	}
}

// fakeInstance holds settings and texts by key.
type fakeInstance struct {
	settings []instance.CurrentSetting
	texts    map[instance.TextKey]string
	err      error
}

func (f *fakeInstance) Settings(context.Context) (instance.Settings, error) {
	return instance.NewSettings(f.settings), f.err
}

func (f *fakeInstance) Text(_ context.Context, key instance.TextKey) (instance.Text, bool, error) {
	body := f.texts[key]
	return instance.Text{Key: key, Body: body}, body != "", nil
}

func TestGetInstanceFromSettings(t *testing.T) {
	fake := &fakeInstance{
		settings: []instance.CurrentSetting{
			{Key: instance.SettingMaxStatusLength, RevisionID: testkit.NamedID("revision-1"), Value: "1200"},
			{Key: instance.SettingRegistrationMode, RevisionID: testkit.NamedID("revision-2"), Value: "invite"},
			{Key: instance.SettingFederatedTimelineAccess, RevisionID: testkit.NamedID("revision-3"), Value: "logged_in"},
			{Key: instance.SettingFederationMode, RevisionID: testkit.NamedID("revision-4"), Value: "inbound"},
		},
		texts: map[instance.TextKey]string{
			instance.TextName:             "例のソーシャル",
			instance.TextShortDescription: "A small instance.",
		},
	}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Instance, o.SendsEmail = fake, true })
	for _, tt := range []struct {
		name, acceptLanguage, token, wantName, wantCache string
	}{
		// The texts are the same in every locale.
		{"anonymous en", "en", "", "例のソーシャル", "public, max-age=60"},
		{"anonymous ja", "ja-JP,ja;q=0.9", "", "例のソーシャル", "public, max-age=60"},
		{"anonymous fr", "fr", "", "例のソーシャル", "public, max-age=60"},
		// alice's locale decides the locale of the answer, so it is private.
		{"user locale", "en", "read", "例のソーシャル", "private, max-age=60"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			header := map[string]string{"Accept-Language": tt.acceptLanguage}
			if tt.token != "" {
				header["Authorization"] = "Bearer " + tt.token
			}
			res := e.get(t, "/instance", header)
			if res.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", res.StatusCode)
			}
			var got clientapi.Instance
			if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got.Name != tt.wantName || got.ShortDescription == nil || *got.ShortDescription != "A small instance." ||
				got.Description != nil || got.Limits.MaxPostCharacters != 1200 || got.Registrations.Mode != clientapi.InstanceRegistrationsModeInvite ||
				got.Timelines != (clientapi.InstanceTimelines{Local: clientapi.TimelineAccessEveryone, Federated: clientapi.TimelineAccessLoggedIn}) || !got.SendsEmail ||
				got.Federation.Mode != clientapi.InstanceFederationModeInbound {
				t.Errorf("instance = %+v, want name %q, the stored settings and email", got, tt.wantName)
			}
			if cc := res.Header.Get("Cache-Control"); cc != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", cc, tt.wantCache)
			}
		})
	}

	fake.err = errors.New("database down")
	if res := e.get(t, "/instance", nil); res.StatusCode != http.StatusInternalServerError {
		t.Errorf("failing settings = %d, want 500", res.StatusCode)
	}
}

// TestGetInstanceContactAndThumbnail checks that GET /instance presents the contact settings
// (the local account and the email address) and the thumbnail with its alt text, and leaves out
// a contact account that does not exist.
func TestGetInstanceContactAndThumbnail(t *testing.T) {
	fake := &fakeInstance{settings: []instance.CurrentSetting{
		{Key: instance.SettingContactAccount, RevisionID: testkit.NamedID("revision-1"), Value: "alice"},
		{Key: instance.SettingContactEmail, RevisionID: testkit.NamedID("revision-2"), Value: "admin@social.example"},
	}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Instance = fake
		o.InstanceThumbnail = func(context.Context) (*clientapi.InstanceImage, error) {
			return &clientapi.InstanceImage{URL: "https://social.example/media/beach.webp", Width: 1600, Height: 900, Alt: "The beach"}, nil
		}
	})
	get := func(t *testing.T) clientapi.Instance {
		t.Helper()
		res := e.get(t, "/instance", map[string]string{"Accept-Language": "ja"})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
		var got clientapi.Instance
		if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	got := get(t)
	if got.Contact == nil || got.Contact.Email == nil || *got.Contact.Email != "admin@social.example" ||
		got.Contact.Account == nil || got.Contact.Account.Id != aliceAccountID || got.Contact.Account.Username != "alice" {
		t.Errorf("contact = %+v, want alice and admin@social.example", got.Contact)
	}
	alt := "The beach"
	if want := (clientapi.InstanceThumbnail{Url: "https://social.example/media/beach.webp", Width: 1600, Height: 900, Alt: &alt}); got.Thumbnail == nil || !reflect.DeepEqual(*got.Thumbnail, want) {
		t.Errorf("thumbnail = %+v, want %+v", got.Thumbnail, want)
	}

	fake.settings = []instance.CurrentSetting{{Key: instance.SettingContactAccount, RevisionID: testkit.NamedID("revision-3"), Value: "nobody"}}
	if got := get(t); got.Contact != nil {
		t.Errorf("contact of an unknown account = %+v, want none", got.Contact)
	}
}

// TestGetInstanceTextsAreTheSameInEveryLocale checks that the administrators' texts and rules,
// one value each, reach readers of every locale as they are.
func TestGetInstanceTextsAreTheSameInEveryLocale(t *testing.T) {
	fake := &fakeInstance{texts: map[instance.TextKey]string{instance.TextName: "例のソーシャル"}}
	hint := "No harassment."
	rules := &fakeRules{rules: []instance.Rule{{
		ID:       "01890a5d-ac96-774b-bcce-b302099a6001",
		Revision: instance.RuleRevision{Position: 1, Text: "Be kind", Hint: hint},
	}}}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Instance, o.Rules = fake, rules })
	for _, acceptLanguage := range []string{"ja-JP", "ja", "en-GB", "fr-CA", "ar"} {
		res := e.get(t, "/instance", map[string]string{"Accept-Language": acceptLanguage})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("Accept-Language %s: status = %d, want 200", acceptLanguage, res.StatusCode)
		}
		var got clientapi.Instance
		if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		want := []clientapi.InstanceRule{{Id: rules.rules[0].ID, Text: "Be kind", Hint: &hint}}
		if got.Name != "例のソーシャル" || !reflect.DeepEqual(got.Rules, want) {
			t.Errorf("Accept-Language %s: name %q, rules %+v, want the same name and rules in every locale", acceptLanguage, got.Name, got.Rules)
		}
	}
}

// TestProblemsAreLocalized checks the problem documents that the generated code writes for
// unparsable requests and failed handlers: they conform to the spec and are localized from
// Accept-Language.
func TestProblemsAreLocalized(t *testing.T) {
	s, _ := newServer(t)
	spec := apitest.Load(t)
	tests := []struct {
		name, acceptLanguage, wantLanguage string
		write                              func(w http.ResponseWriter, r *http.Request)
		wantStatus                         int
		wantTitle, wantDetail              string
	}{
		{
			name: "invalid parameter in en", acceptLanguage: "en-US,en;q=0.9", wantLanguage: "en",
			write: func(w http.ResponseWriter, r *http.Request) {
				clientapi.RequestErrorHandler(s)(w, r, &clientapi.InvalidParamFormatError{ParamName: "limit"})
			},
			wantStatus: http.StatusBadRequest,
			wantTitle:  "The request is invalid.", wantDetail: "The parameter limit has an invalid value.",
		},
		{
			name: "invalid parameter in ja", acceptLanguage: "ja", wantLanguage: "ja",
			write: func(w http.ResponseWriter, r *http.Request) {
				clientapi.RequestErrorHandler(s)(w, r, &clientapi.RequiredParamError{ParamName: "cursor"})
			},
			wantStatus: http.StatusBadRequest,
			wantTitle:  "リクエストが正しくありません。", wantDetail: "パラメーター cursor の値が正しくありません。",
		},
		{
			name: "handler error in ja", acceptLanguage: "ja-JP", wantLanguage: "ja",
			write: func(w http.ResponseWriter, r *http.Request) {
				clientapi.ResponseErrorHandler(s)(w, r, context.DeadlineExceeded)
			},
			wantStatus: http.StatusInternalServerError,
			wantTitle:  "予期しないエラーが発生しました。しばらくしてからもう一度試してください。",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, clientapi.BasePath+"/instance", nil)
			req.Header.Set("Accept-Language", tt.acceptLanguage)
			res := spec.Serve(t, http.HandlerFunc(tt.write), req)
			if res.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}
			if got := res.Header.Get("Content-Language"); got != tt.wantLanguage {
				t.Errorf("Content-Language = %q, want %q", got, tt.wantLanguage)
			}
			var p clientapi.Problem
			if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
				t.Fatal(err)
			}
			var detail string
			if p.Detail != nil {
				detail = *p.Detail
			}
			if p.Type != clientapi.ProblemTypeBlank || p.Status != tt.wantStatus || p.Title != tt.wantTitle || detail != tt.wantDetail {
				t.Errorf("problem = %+v (detail %q), want title %q and detail %q", p, detail, tt.wantTitle, tt.wantDetail)
			}
			if p.RequestId == nil || *p.RequestId != "req-1" {
				t.Errorf("request_id = %v, want req-1", p.RequestId)
			}
		})
	}
}

// viewerKey is the context key of a fake authenticated viewer: its saved locale.
type viewerKey struct{}

// TestProblemLocaleOrder checks that problem documents follow ADR-0009 §2: the
// authenticated user's locale, then Accept-Language, then the instance default.
func TestProblemLocaleOrder(t *testing.T) {
	instanceDefault := "ja"
	s, _ := newServerWithLocales(t, func(b *i18n.Bundle) *i18n.Resolver {
		return &i18n.Resolver{
			Bundle:          b,
			UserLocale:      func(ctx context.Context) string { l, _ := ctx.Value(viewerKey{}).(string); return l },
			InstanceDefault: func(context.Context) string { return instanceDefault },
		}
	})
	tests := []struct {
		name, viewer, acceptLanguage, instanceDefault, want string
	}{
		{name: "anonymous without accept-language", instanceDefault: "ja", want: "ja"},
		{name: "default changed", instanceDefault: "en", want: "en"},
		{name: "accept-language before the default", acceptLanguage: "en", instanceDefault: "ja", want: "en"},
		{name: "user before accept-language", viewer: "ja", acceptLanguage: "en", instanceDefault: "en", want: "ja"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instanceDefault = tt.instanceDefault
			req := httptest.NewRequest(http.MethodGet, clientapi.BasePath+"/instance", nil)
			if tt.acceptLanguage != "" {
				req.Header.Set("Accept-Language", tt.acceptLanguage)
			}
			if tt.viewer != "" {
				req = req.WithContext(context.WithValue(req.Context(), viewerKey{}, tt.viewer))
			}
			w := httptest.NewRecorder()
			clientapi.RequestErrorHandler(s)(w, req, &clientapi.RequiredParamError{ParamName: "cursor"})
			if got := w.Header().Get("Content-Language"); got != tt.want {
				t.Errorf("Content-Language = %q, want %q", got, tt.want)
			}
		})
	}
}
