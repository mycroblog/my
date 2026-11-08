// SPDX-License-Identifier: AGPL-3.0-only

package i18n

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// viewerKey is the context key of the fake viewer: the saved locale of the user that a
// request is authenticated as.
type viewerKey struct{}

func withViewer(ctx context.Context, locale string) context.Context {
	return context.WithValue(ctx, viewerKey{}, locale)
}

func viewerLocale(ctx context.Context) string {
	l, _ := ctx.Value(viewerKey{}).(string)
	return l
}

func TestResolverLocale(t *testing.T) {
	b := load(t)
	tests := []struct {
		name            string
		viewer          string // "" is an anonymous request
		acceptLanguage  string
		instanceDefault string
		want            string
	}{
		{"nothing", "", "", "", "en"},
		{"user setting wins", "ja", "en", "en", "ja"},
		{"user setting wins over the instance default", "en", "", "ja", "en"},
		{"accept-language", "", "it;q=0.9, ja-JP;q=0.8, en;q=0.1", "", "ja"},
		{"accept-language before instance default", "", "en-GB", "ja", "en"},
		{"instance default without accept-language", "", "", "ja", "ja"},
		{"unsupported accept-language uses instance default", "", "it, nl", "ja", "ja"},
		{"unsupported user setting is skipped", "it-CH", "", "ja", "ja"},
		{"unsupported instance default falls back to en", "", "it", "nl", "en"},
		{"malformed values are skipped", "!!", "*, ja_jp", "en", "ja"},
		{"malformed instance default falls back to en", "", "", "!!", "en"},
		{"pseudo-locales are not selectable by default", "en-XA", "", "ja", "en"},
		{"zh resolves to zh-CN", "", "zh", "", "zh-CN"},
		{"zh-SG resolves to zh-CN", "", "zh-SG, ja;q=0.5", "", "zh-CN"},
		{"zh-Hans resolves to zh-CN", "", "zh-Hans", "", "zh-CN"},
		{"zh-Hans with a region resolves to zh-CN", "", "zh-Hans-SG", "", "zh-CN"},
		{"zh-CN user setting", "zh-CN", "ja", "", "zh-CN"},
		{"es", "", "es", "", "es"},
		{"es-419 resolves to es", "", "es-419, en;q=0.5", "", "es"},
		{"es-MX user setting", "es-MX", "ja", "", "es"},
		{"pt-BR", "", "pt-BR", "", "pt-BR"},
		{"pt resolves to pt-BR", "", "pt, en;q=0.5", "", "pt-BR"},
		{"pt-PT resolves to pt-BR", "", "pt-PT", "", "pt-BR"},
		{"pt-PT user setting", "pt-PT", "ja", "", "pt-BR"},
		{"de", "", "de", "", "de"},
		{"de-AT resolves to de", "", "de-AT, en;q=0.5", "", "de"},
		{"de-CH user setting", "de-CH", "ja", "", "de"},
		{"fr", "", "fr", "", "fr"},
		{"fr-CA resolves to fr", "", "fr-CA, en;q=0.5", "", "fr"},
		{"fr-BE user setting", "fr-BE", "ja", "", "fr"},
		{"zh-TW", "", "zh-TW", "", "zh-TW"},
		{"zh-HK resolves to zh-TW", "", "zh-HK, ja;q=0.5", "", "zh-TW"},
		{"zh-MO resolves to zh-TW", "", "zh-MO", "", "zh-TW"},
		{"zh-Hant resolves to zh-TW", "", "zh-Hant", "", "zh-TW"},
		{"zh-Hant with a region resolves to zh-TW", "", "zh-Hant-HK", "", "zh-TW"},
		{"zh-TW user setting", "zh-Hant-TW", "zh-CN", "", "zh-TW"},
		{"Traditional Chinese never resolves to zh-CN", "", "zh-Hant, zh;q=0.5", "", "zh-TW"},
		{"Simplified Chinese never resolves to zh-TW", "", "zh-Hans, zh-TW;q=0.5", "", "zh-CN"},
	}
	for _, tt := range tests {
		r := &Resolver{
			Bundle:          b,
			UserLocale:      viewerLocale,
			InstanceDefault: func(context.Context) string { return tt.instanceDefault },
		}
		ctx := t.Context()
		if tt.viewer != "" {
			ctx = withViewer(ctx, tt.viewer)
		}
		if got := r.Locale(ctx, tt.acceptLanguage); got != tt.want {
			t.Errorf("%s: Locale(viewer %q, %q) with default %q = %q, want %q",
				tt.name, tt.viewer, tt.acceptLanguage, tt.instanceDefault, got, tt.want)
		}
	}

	pseudo := &Resolver{Bundle: load(t, WithPseudoLocales())}
	if got := pseudo.Locale(t.Context(), "ar-XB"); got != "ar-XB" {
		t.Errorf("Locale(ar-XB) with pseudo-locales = %q", got)
	}
	if got := (&Resolver{Bundle: b}).Locale(t.Context(), ""); got != "en" {
		t.Errorf("Locale without hooks = %q, want en", got)
	}
}

// TestResolverReadsInstanceDefaultPerRequest checks that a changed default applies to the
// next request, and that the default is not read when an earlier step decides.
func TestResolverReadsInstanceDefaultPerRequest(t *testing.T) {
	current, reads := "en", 0
	r := &Resolver{Bundle: load(t), InstanceDefault: func(context.Context) string { reads++; return current }}

	if got := r.Locale(t.Context(), ""); got != "en" {
		t.Fatalf("Locale = %q, want en", got)
	}
	current = "ja"
	if got := r.RequestLocalizer(httptest.NewRequest(http.MethodGet, "/", nil)).Locale(); got != "ja" {
		t.Errorf("Locale after changing the default = %q, want ja", got)
	}
	reads = 0
	if got := r.Locale(t.Context(), "en"); got != "en" || reads != 0 {
		t.Errorf("Locale(en) = %q with %d reads of the default, want en with none", got, reads)
	}
}

// TestResolverRequestedLocale checks that a Localizer keeps the preference that chose its
// locale, region and script included.
func TestResolverRequestedLocale(t *testing.T) {
	b := load(t)
	tests := []struct {
		name            string
		viewer          string
		acceptLanguage  string
		instanceDefault string
		wantLocale      string
		wantRequested   string
	}{
		{"region of a catalog", "", "ja-JP", "", "ja", "ja-JP"},
		{"unknown region of a catalog", "", "ja-XX", "", "ja", "ja-XX"},
		{"case and separators are canonical", "", "ja_jp", "", "ja", "ja-JP"},
		{"language without region", "", "ja", "", "ja", "ja"},
		{"first matching tag decides", "", "it-CH, en-GB;q=0.5", "", "en", "en-GB"},
		{"user setting with region", "ja-JP", "en", "", "ja", "ja-JP"},
		{"instance default with region", "", "it", "ja-JP", "ja", "ja-JP"},
		{"nothing matches", "", "it-CH", "nl", "en", "en"},
		{"Simplified Chinese with a script", "", "zh-Hans-CN", "", "zh-CN", "zh-Hans-CN"},
		{"Traditional Chinese with a script", "", "zh-Hant-TW", "", "zh-TW", "zh-Hant-TW"},
		{"Spanish with a region", "", "es-ES", "", "es", "es-ES"},
		{"European Portuguese", "", "pt-PT", "", "pt-BR", "pt-PT"},
		{"German with a region", "", "de-DE", "", "de", "de-DE"},
		{"Canadian French", "", "fr-CA", "", "fr", "fr-CA"},
	}
	for _, tt := range tests {
		r := &Resolver{
			Bundle:          b,
			UserLocale:      viewerLocale,
			InstanceDefault: func(context.Context) string { return tt.instanceDefault },
		}
		ctx := t.Context()
		if tt.viewer != "" {
			ctx = withViewer(ctx, tt.viewer)
		}
		l := r.Localizer(ctx, tt.acceptLanguage)
		if l.Locale() != tt.wantLocale || l.RequestedLocale() != tt.wantRequested {
			t.Errorf("%s: Localizer(viewer %q, %q) with default %q = %q requested %q, want %q requested %q",
				tt.name, tt.viewer, tt.acceptLanguage, tt.instanceDefault,
				l.Locale(), l.RequestedLocale(), tt.wantLocale, tt.wantRequested)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en")
	if got := (&Resolver{Bundle: b}).UserLocalizer(req, "ja-JP").RequestedLocale(); got != "ja-JP" {
		t.Errorf("UserLocalizer(user ja-JP).RequestedLocale() = %q, want ja-JP", got)
	}
	if got := b.Localizer("ja").RequestedLocale(); got != "ja" {
		t.Errorf("Bundle.Localizer(ja).RequestedLocale() = %q, want ja", got)
	}
}

func TestResolverUserLocalizer(t *testing.T) {
	r := &Resolver{Bundle: load(t), InstanceDefault: func(context.Context) string { return "ja" }}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en")
	for user, want := range map[string]string{"ja": "ja", "": "en", "zh-Hant-TW": "zh-TW", "es-AR": "es", "pt": "pt-BR", "de-AT": "de", "fr-CA": "fr", "it": "en", "!!": "en"} {
		if got := r.UserLocalizer(req, user).Locale(); got != want {
			t.Errorf("UserLocalizer(Accept-Language en, user %q) = %q, want %q", user, got, want)
		}
	}
}

func TestResolverAcceptedLocale(t *testing.T) {
	r := &Resolver{Bundle: load(t), InstanceDefault: func(context.Context) string { return "ja" }}
	tests := []struct {
		acceptLanguage string
		want           string // "" means none matches
	}{
		{"ja", "ja"},
		{"ja-JP,en;q=0.5", "ja"},
		{"en-US", "en"},
		{"it;q=0.9, ja;q=0.8, en;q=0.1", "ja"},
		{"it, nl", ""},
		{"", ""},
		{"*", ""},
	}
	for _, tt := range tests {
		got, ok := r.AcceptedLocale(tt.acceptLanguage)
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("AcceptedLocale(%q) = %q, %v, want %q", tt.acceptLanguage, got, ok, tt.want)
		}
	}

	// A pseudo-locale is never a saved locale, even where it is selectable; the next tag decides.
	pseudo := &Resolver{Bundle: load(t, WithPseudoLocales())}
	for header, want := range map[string]string{"en-XA, ja;q=0.5": "ja", "ar-XB": ""} {
		if got, _ := pseudo.AcceptedLocale(header); got != want {
			t.Errorf("AcceptedLocale(%q) with pseudo-locales = %q, want %q", header, got, want)
		}
	}
}
