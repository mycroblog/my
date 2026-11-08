// SPDX-License-Identifier: AGPL-3.0-only

package i18n

import (
	"slices"
	"testing"
)

func envOf(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestEnvLocales(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"unset", nil, nil},
		{"LANG with codeset", map[string]string{"LANG": "ja_JP.UTF-8"}, []string{"ja-JP"}},
		{"LC_ALL wins", map[string]string{"LC_ALL": "en_US.UTF-8", "LC_MESSAGES": "ja_JP", "LANG": "ja_JP"}, []string{"en-US"}},
		{"empty LC_ALL is skipped", map[string]string{"LC_ALL": "", "LC_MESSAGES": "ja_JP@euro", "LANG": "en_US"}, []string{"ja-JP"}},
		{"C", map[string]string{"LANG": "C.UTF-8"}, nil},
		{"POSIX ignores LANGUAGE", map[string]string{"LANG": "POSIX", "LANGUAGE": "ja"}, nil},
		{"LANGUAGE first", map[string]string{"LANG": "en_US.UTF-8", "LANGUAGE": "it:ja_JP::x y"}, []string{"it", "ja-JP", "en-US"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EnvLocales(envOf(tt.env)); !slices.Equal(got, tt.want) {
				t.Errorf("EnvLocales() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBundleEnvLocale(t *testing.T) {
	b, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		env  map[string]string
		want string
	}{
		{nil, SourceLocale},
		{map[string]string{"LANG": "ja_JP.UTF-8"}, "ja"},
		{map[string]string{"LANG": "it_IT.UTF-8"}, SourceLocale},
		{map[string]string{"LANG": "en_US.UTF-8", "LANGUAGE": "it:ja"}, "ja"},
	}
	for _, tt := range tests {
		if got := b.EnvLocale(envOf(tt.env)); got != tt.want {
			t.Errorf("EnvLocale(%v) = %q, want %q", tt.env, got, tt.want)
		}
	}
}
