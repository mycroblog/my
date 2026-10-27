// SPDX-License-Identifier: AGPL-3.0-only

package i18n

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func load(t *testing.T, opts ...Option) *Bundle {
	t.Helper()
	b, err := Load(opts...)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAccessors(t *testing.T) {
	b := load(t, WithPseudoLocales())
	tests := []struct {
		locale string
		got    func(*Localizer) string
		want   string
	}{
		{"en", func(l *Localizer) string { return l.ServerConfigErrors(1) }, "Found 1 configuration error."},
		{"en", func(l *Localizer) string { return l.ServerConfigErrors(3) }, "Found 3 configuration errors."},
		{"ja", func(l *Localizer) string { return l.ServerConfigErrors(1) }, "設定エラーが1件あります。"},
		{"en", func(l *Localizer) string { return l.ServerConfigMissing("MY_DB") }, "The required setting MY_DB is not set."},
		{"ja", func(l *Localizer) string { return l.ServerErrorsMethodNotAllowed("PUT") }, "このリソースでは PUT メソッドを使用できません。"},
		{"en-XA", func(l *Localizer) string { return l.ServerErrorsNotFound() }, "[Ŧħḗḗ řḗḗɋŭŭḗḗşŧḗḗḓ řḗḗşǿǿŭŭřƈḗḗ ẇȧȧş ƞǿǿŧ ƒǿǿŭŭƞḓ.]"},
		{"en-XA", func(l *Localizer) string { return l.ServerConfigErrors(1) }, "[Ƒǿǿŭŭƞḓ 1 ƈǿǿƞƒīɠŭŭřȧȧŧīǿǿƞ ḗḗřřǿǿř.]"},
		{"ar-XB", func(l *Localizer) string { return l.ServerConfigInvalid("MY_DB") }, "\u202e⊥ɥǝ sǝʇʇıuƃ MY_DB ɥɐs ɐu ıuʌɐʅıp ʌɐʅnǝ.\u202c"},
		// Locales without a catalog fall back to en.
		{"fr", func(l *Localizer) string { return l.ServerErrorsNotFound() }, "The requested resource was not found."},
		{"ja-JP", func(l *Localizer) string { return l.ServerErrorsNotFound() }, "指定されたリソースが見つかりません。"},
	}
	for _, tt := range tests {
		if got := tt.got(b.Localizer(tt.locale)); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.locale, got, tt.want)
		}
	}
}

func TestLocales(t *testing.T) {
	if got, want := load(t).Locales(), []string{"en", "ja"}; !slices.Equal(got, want) {
		t.Errorf("Locales() = %v, want %v", got, want)
	}
	if got, want := load(t, WithPseudoLocales()).Locales(), []string{"ar-XB", "en", "en-XA", "ja"}; !slices.Equal(got, want) {
		t.Errorf("Locales() with pseudo-locales = %v, want %v", got, want)
	}
}

func TestFallbackChain(t *testing.T) {
	tests := map[string][]string{
		"en":         {"en"},
		"ja-JP":      {"ja-JP", "ja", "en"},
		"zh-Hant-TW": {"zh-Hant-TW", "zh-Hant", "en"},
		"zh-TW":      {"zh-TW", "zh-Hant", "en"},
		"zh-Hans-CN": {"zh-Hans-CN", "zh-Hans", "zh", "en"},
		"en-XA":      {"en-XA", "en"},
		"ar-XB":      {"ar-XB", "ar", "en"},
		"":           {"en"},
	}
	for in, want := range tests {
		if got := FallbackChain(in); !slices.Equal(got, want) {
			t.Errorf("FallbackChain(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCanonicalize(t *testing.T) {
	tests := map[string]string{
		"ja":                  "ja",
		"JA_jp":               "ja-JP",
		"zh-hant-tw":          "zh-Hant-TW",
		"es-419":              "es-419",
		"en-xa":               "en-XA",
		"de-ch-1996":          "de-CH-1996",
		"ja-JP-u-CA-Japanese": "ja-JP-u-ca-japanese",
		"en-x-AB":             "en-x-ab",
		"":                    "",
		"*":                   "",
		"en--US":              "",
		"ja;q=1":              "",
		"123":                 "",
		"en-u":                "",
		"en-a":                "",
		"en-US-US":            "",
		"iw":                  "he",
		"und-JP":              "",
		"und":                 "und",
		"x-foo":               "",
	}
	for in, want := range tests {
		if got := Canonicalize(in); got != want {
			t.Errorf("Canonicalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseAcceptLanguage(t *testing.T) {
	tests := map[string][]string{
		"":                                   nil,
		"ja":                                 {"ja"},
		"fr-CH, fr;q=0.9, en;q=0.8, *;q=0.5": {"fr-CH", "fr", "en"},
		"en;q=0.5, ja":                       {"ja", "en"},
		"de;q=0, ja;q=bogus, en":             {"en"},
		"es;q=0.8, pt;q=0.8, it;q=0.9":       {"it", "es", "pt"},
	}
	for in, want := range tests {
		if got := ParseAcceptLanguage(in); !slices.Equal(got, want) {
			t.Errorf("ParseAcceptLanguage(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestDirection(t *testing.T) {
	tests := map[string]string{"en": "ltr", "ja": "ltr", "ar": "rtl", "he-IL": "rtl", "ar-XB": "rtl", "en-XA": "ltr", "az-Arab": "rtl", "ku-Latn": "ltr"}
	for in, want := range tests {
		if got := Direction(in); got != want {
			t.Errorf("Direction(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewBundleRejectsBrokenCatalogs(t *testing.T) {
	en := `{"a": {"defaultMessage": "{n, plural, one {#} other {#}}"}}`
	tests := map[string]fstest.MapFS{
		"syntax error":      {"en/x/a.json": {Data: []byte(`{"a": {"defaultMessage": "{n, plural, one {#}"}}`)}},
		"plural category":   {"en/x/a.json": {Data: []byte(en)}, "ja/x/a.json": {Data: []byte(en)}},
		"invalid json":      {"en/x/a.json": {Data: []byte(`{`)}},
		"non-canonical dir": {"en/x/a.json": {Data: []byte(en)}, "ja_JP/x/a.json": {Data: []byte(en)}},
		"no source locale":  {"ja/x/a.json": {Data: []byte(`{}`)}},
		"no namespace dir":  {"en/x.json": {Data: []byte(en)}},
	}
	for name, fsys := range tests {
		if _, err := NewBundle(fsys); err == nil {
			t.Errorf("%s: NewBundle succeeded", name)
		}
	}
}

func TestFormatFallsBack(t *testing.T) {
	fsys := fstest.MapFS{
		"en/x/only_en.json": {Data: []byte(`{"only_en": {"defaultMessage": "en {v}"}}`)},
		"en/x/both.json":    {Data: []byte(`{"both": {"defaultMessage": "en both"}}`)},
		"ja/x/both.json":    {Data: []byte(`{"both": {"defaultMessage": "ja {missing}"}}`)},
	}
	b, err := NewBundle(fsys)
	if err != nil {
		t.Fatal(err)
	}
	l := b.Localizer("ja")
	for id, want := range map[string]string{
		"x:only_en": "en 1", // absent in ja
		"x:both":    "en both",
		"x:unknown": "x:unknown",
	} {
		if got := l.format(id, map[string]any{"v": 1}); got != want {
			t.Errorf("format(%q) = %q, want %q", id, got, want)
		}
	}
}

func ExampleResolver_Locale() {
	b, err := Load()
	if err != nil {
		panic(err)
	}
	r := &Resolver{Bundle: b, InstanceDefault: func(context.Context) string { return "en" }}
	for _, header := range []string{"ja-JP,ja;q=0.9,en;q=0.8", "fr-CA, de;q=0.5"} {
		l := r.Localizer(context.Background(), header)
		fmt.Println(l.Locale(), l.Direction(), strings.TrimSpace(l.ServerConfigErrors(2)))
	}
	// Output:
	// ja ltr 設定エラーが2件あります。
	// en ltr Found 2 configuration errors.
}
