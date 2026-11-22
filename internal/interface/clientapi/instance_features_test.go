// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/app/search"
	appsupporters "github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// featureSupporters is a supporter program that is on or off; nothing else of it is used.
type featureSupporters struct {
	clientapi.Supporters
	on bool
}

func (f featureSupporters) Enabled(context.Context) (bool, error) { return f.on, nil }

func (f featureSupporters) Program(context.Context) (appsupporters.Program, error) {
	return appsupporters.Program{Enabled: f.on}, nil
}

// featurePush is a Web Push sender with the given VAPID public key, "" for none.
type featurePush struct {
	clientapi.WebPush
	key string
}

func (f featurePush) ServerKey() string { return f.key }

// featureTranslations is a translation service with the given providers enabled.
type featureTranslations struct {
	clientapi.Translations
	providers []translation.Provider
}

func (f featureTranslations) EnabledProviders(context.Context) ([]translation.Provider, error) {
	return f.providers, nil
}

func (f featureTranslations) Overview(context.Context) (translation.Overview, error) {
	return translation.Overview{}, nil
}

var featureProvider = translation.Provider{ID: "deepl", Name: "DeepL", URL: "https://www.deepl.com/"}

// featureOptions turns on the condition of each identifier of Instance.features.
var featureOptions = map[string]func(*clientapi.Options){
	"voice_rooms":      func(o *clientapi.Options) { o.Voice = &fakeVoiceRooms{} },
	"full_text_search": func(o *clientapi.Options) { o.FullTextSearch = fakeFullTextSearch{search.VisibilityPublic} },
	"supporters":       func(o *clientapi.Options) { o.Supporters = featureSupporters{on: true} },
	"tips":             func(o *clientapi.Options) { o.Tips = &fakeTips{} },
	"web_push":         func(o *clientapi.Options) { o.Push = featurePush{key: "BPublicKey"} },
	"translation": func(o *clientapi.Options) {
		o.Translations = featureTranslations{providers: []translation.Provider{featureProvider}}
	},
}

// getInstanceFeatures answers GET /instance on a server built with the tweaks, and returns the
// Instance and the raw JSON of its features.
func getInstanceFeatures(t *testing.T, tweaks ...func(*clientapi.Options)) (clientapi.Instance, json.RawMessage) {
	t.Helper()
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Instance = &fakeInstance{}
		for _, tweak := range tweaks {
			tweak(o)
		}
	})
	res := e.send(t, http.MethodGet, "/instance", "", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /instance = %d", res.StatusCode)
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var got clientapi.Instance
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	features, ok := top["features"]
	if !ok {
		t.Fatalf("GET /instance has no features: %s", raw)
	}
	return got, features
}

// inRegistryOrder returns the identifiers of want in the order of the registry.
func inRegistryOrder(want ...string) []string {
	out := []string{}
	for _, f := range clientapi.Features() {
		if slices.Contains(want, f.ID) {
			out = append(out, f.ID)
		}
	}
	return out
}

// TestInstanceFeaturesEmpty checks that a default instance lists no feature as an empty array,
// never null (#3024).
func TestInstanceFeaturesEmpty(t *testing.T) {
	got, raw := getInstanceFeatures(t)
	if string(raw) != "[]" || got.Features == nil || len(got.Features) != 0 {
		t.Errorf("features = %s, want []", raw)
	}
}

// TestInstanceFeatures checks that GET /instance lists each feature exactly while its condition
// holds, in the order of the registry (#3024).
func TestInstanceFeatures(t *testing.T) {
	all := make([]func(*clientapi.Options), 0, len(featureOptions))
	allIDs := make([]string, 0, len(featureOptions))
	for _, f := range clientapi.Features() {
		on, ok := featureOptions[f.ID]
		if !ok {
			t.Errorf("feature %q: no condition to turn it on in this test", f.ID)
			continue
		}
		all, allIDs = append(all, on), append(allIDs, f.ID)
	}
	for _, tc := range []struct {
		name   string
		tweaks []func(*clientapi.Options)
		want   []string
	}{
		{"none", nil, nil},
		{"voice on", []func(*clientapi.Options){featureOptions["voice_rooms"]}, []string{"voice_rooms"}},
		{"full-text search configured", []func(*clientapi.Options){featureOptions["full_text_search"]}, []string{"full_text_search"}},
		{"supporters enabled", []func(*clientapi.Options){featureOptions["supporters"]}, []string{"supporters"}},
		{"supporters disabled", []func(*clientapi.Options){func(o *clientapi.Options) { o.Supporters = featureSupporters{on: false} }}, nil},
		{"tips enabled", []func(*clientapi.Options){featureOptions["tips"]}, []string{"tips"}},
		{"tips disabled", []func(*clientapi.Options){func(o *clientapi.Options) { o.Tips = &fakeTips{disabled: true} }}, nil},
		{"web push with a key", []func(*clientapi.Options){featureOptions["web_push"]}, []string{"web_push"}},
		{"web push without a key", []func(*clientapi.Options){func(o *clientapi.Options) { o.Push = featurePush{} }}, nil},
		{"a translation provider", []func(*clientapi.Options){featureOptions["translation"]}, []string{"translation"}},
		{"no translation provider", []func(*clientapi.Options){func(o *clientapi.Options) {
			o.Translations = featureTranslations{providers: []translation.Provider{}}
		}}, nil},
		{"everything on", all, allIDs},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, raw := getInstanceFeatures(t, tc.tweaks...)
			if want := inRegistryOrder(tc.want...); got.Features == nil || !slices.Equal(got.Features, want) {
				t.Errorf("features = %s, want %q", raw, want)
			}
		})
	}
}

// TestInstanceFeaturesAgreeWithTheirFields checks that each identifier is in features exactly
// when the dedicated field of the same answer says that its feature is on (#3024).
func TestInstanceFeaturesAgreeWithTheirFields(t *testing.T) {
	fields := map[string]func(clientapi.Instance) bool{
		"voice_rooms":      func(i clientapi.Instance) bool { return i.VoiceEnabled },
		"full_text_search": func(i clientapi.Instance) bool { return i.FullTextSearch.Available },
		"supporters":       func(i clientapi.Instance) bool { return i.SupportersEnabled },
		"tips":             func(i clientapi.Instance) bool { return i.TipsEnabled },
		"web_push":         func(i clientapi.Instance) bool { return i.PushServerKey != nil && *i.PushServerKey != "" },
		"translation":      func(i clientapi.Instance) bool { return len(i.Translation.Providers) > 0 },
	}
	for _, f := range clientapi.Features() {
		if fields[f.ID] == nil {
			t.Errorf("feature %q: no dedicated field to compare it with in this test", f.ID)
		}
	}
	answers := map[string][]func(*clientapi.Options){"nothing on": nil}
	for id, on := range featureOptions {
		answers[id+" on"] = []func(*clientapi.Options){on}
	}
	everything := []func(*clientapi.Options){}
	for _, on := range featureOptions {
		everything = append(everything, on)
	}
	answers["everything on"] = everything
	for name, tweaks := range answers {
		t.Run(name, func(t *testing.T) {
			got, raw := getInstanceFeatures(t, tweaks...)
			for id, field := range fields {
				if listed, on := slices.Contains(got.Features, id), field(got); listed != on {
					t.Errorf("%q listed %v, but its field says %v (features %s)", id, listed, on, raw)
				}
			}
		})
	}
}

// TestFeaturesReturnsACopy checks that a caller's change to the result of Features does not
// reach the registry.
func TestFeaturesReturnsACopy(t *testing.T) {
	before := clientapi.Features()
	if len(before) < 2 {
		t.Fatalf("Features() has %d entries, want at least 2", len(before))
	}
	ids := make([]string, len(before))
	for i, f := range before {
		ids[i] = f.ID
	}
	before[0].ID, before[0].Meaning = "changed", "changed"
	_ = append(before[:1], before[2:]...)

	after := clientapi.Features()
	if len(after) != len(ids) {
		t.Fatalf("Features() has %d entries after a caller's change, want %d", len(after), len(ids))
	}
	for i, f := range after {
		if f.ID != ids[i] || f.Meaning == "changed" {
			t.Errorf("Features()[%d] = %q %q after a caller's change, want %q", i, f.ID, f.Meaning, ids[i])
		}
	}
	got, _ := getInstanceFeatures(t, featureOptions[ids[0]])
	if !slices.Equal(got.Features, []string{ids[0]}) {
		t.Errorf("features = %q after a caller's change, want %q", got.Features, ids[0])
	}
}
