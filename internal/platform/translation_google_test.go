// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_translator_google && !no_gcpkms

package platform_test

import (
	"testing"

	translatorgoogle "github.com/mycroblog/my/internal/adapter/translator/google"
	"github.com/mycroblog/my/internal/app/translation"
)

// translator_google_project registers Google Cloud Translation, with its name and link and no logo, on Application
// Default Credentials and without a call; without it, the server has no such provider (ADR-0148 §15).
func TestTranslationProviderGoogleFromConfig(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", writeServiceAccount(t))
	for _, tc := range []struct {
		env  map[string]string
		want []translation.ProviderInfo
	}{
		{map[string]string{}, nil},
		{
			map[string]string{"MY_TRANSLATOR_GOOGLE_PROJECT": "my-project", "MY_TRANSLATOR_GOOGLE_LOCATION": "us-central1"},
			[]translation.ProviderInfo{{ID: translatorgoogle.ProviderID, Name: "Google Translate", URL: "https://translate.google.com"}},
		},
	} {
		app := openApp(t, newDatabase(t, true), tc.env, nil, nil)
		o, err := app.Translation.Overview(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var got []translation.ProviderInfo
		for _, p := range o.Registered {
			if p.ID == translatorgoogle.ProviderID {
				got = append(got, p)
			}
		}
		if len(got) != len(tc.want) || len(got) == 1 && got[0] != tc.want[0] {
			t.Errorf("env %v: registered %+v, want %+v", tc.env, got, tc.want)
		}
	}
}
