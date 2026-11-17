// SPDX-License-Identifier: AGPL-3.0-only

package instancetest

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// instanceThemeCases are the cases of InstanceFlows about the instance theme (ADR-0059 §15): no
// theme until a set is published, a draft kept apart from the set in effect, publications that keep
// their sets and stay readable after a newer one, a reset by an empty set, the restore of the set
// published before, invalid sets that write nothing, and the projections equal to a rebuild.
var instanceThemeCases = []struct {
	name string
	run  func(t *testing.T, e *instanceEnv)
}{
	{"HasNoThemeUntilPublished", instanceHasNoTheme},
	{"KeepsTheDraftApart", instanceKeepsTheThemeDraftApart},
	{"RestoresThePreviousTheme", instanceRestoresThePreviousTheme},
	{"PublishesSetsWithRetiredTokens", instancePublishesSetsWithRetiredTokens},
	{"RejectsInvalidThemes", instanceRejectsInvalidThemes},
	{"ThemeProjectionEqualsRebuild", instanceThemeProjectionEqualsRebuild},
}

func (e *instanceEnv) saveThemeDraft(t *testing.T, tokens ...instance.ThemeTokenValue) instance.ThemeSet {
	t.Helper()
	e.clock.Advance(time.Second)
	set, err := e.service.SaveThemeDraft(t.Context(), appinstance.SaveThemeDraftInput{Tokens: tokens, SavedByUserID: ruleModerator})
	if err != nil {
		t.Fatalf("SaveThemeDraft = %v", err)
	}
	return set
}

func (e *instanceEnv) publishTheme(t *testing.T) instance.ThemeRevision {
	t.Helper()
	e.clock.Advance(time.Second)
	rev, err := e.service.PublishTheme(t.Context(), ruleModerator)
	if err != nil {
		t.Fatalf("PublishTheme = %v", err)
	}
	return rev
}

func (e *instanceEnv) theme(t *testing.T) (instance.ThemeRevision, bool) {
	t.Helper()
	rev, ok, err := e.service.Theme(t.Context())
	if err != nil {
		t.Fatalf("Theme = %v", err)
	}
	return rev, ok
}

func (e *instanceEnv) themeDraft(t *testing.T) (instance.ThemeSet, bool) {
	t.Helper()
	set, ok, err := e.service.ThemeDraft(t.Context())
	if err != nil {
		t.Fatalf("ThemeDraft = %v", err)
	}
	return set, ok
}

func instanceHasNoTheme(t *testing.T, e *instanceEnv) {
	if _, ok := e.theme(t); ok {
		t.Error("a new instance has a theme")
	}
	if _, ok := e.themeDraft(t); ok {
		t.Error("a new instance has a draft")
	}
	if _, err := e.service.ThemeRevision(t.Context(), "0199f0e0-0000-7000-8000-000000000000"); !errors.Is(err, appinstance.ErrNotFound) {
		t.Errorf("ThemeRevision(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := e.service.PublishTheme(t.Context(), ruleModerator); !errors.Is(err, appinstance.ErrNotFound) {
		t.Errorf("PublishTheme without a draft = %v, want ErrNotFound", err)
	}
	if _, err := e.service.RestoreTheme(t.Context(), ruleModerator); !errors.Is(err, appinstance.ErrNotFound) {
		t.Errorf("RestoreTheme without a publication = %v, want ErrNotFound", err)
	}
}

func instanceKeepsTheThemeDraftApart(t *testing.T, e *instanceEnv) {
	draft := e.saveThemeDraft(t,
		instance.ThemeTokenValue{Token: "radius-control", Value: "2px"},
		instance.ThemeTokenValue{Token: "color-surface", Light: "#FFFDF5", Dark: "#0a0a0a"},
	)
	wantSet := instance.ThemeSet{
		ID: draft.ID, SavedByUserID: ruleModerator, SavedAt: draft.SavedAt,
		Tokens: []instance.ThemeTokenValue{
			{Token: "color-surface", Light: "#fffdf5", Dark: "#0a0a0a"},
			{Token: "radius-control", Value: "2px"},
		},
	}
	if got, ok := e.themeDraft(t); !ok || !reflect.DeepEqual(got, wantSet) || !reflect.DeepEqual(draft, wantSet) {
		t.Errorf("ThemeDraft = %+v, %t; SaveThemeDraft = %+v\nwant %+v", got, ok, draft, wantSet)
	}
	if _, ok := e.theme(t); ok {
		t.Error("a saved draft is in effect before it is published")
	}

	first := e.publishTheme(t)
	if got, ok := e.theme(t); !ok || got.ID != first.ID || got.SetID != draft.ID || !reflect.DeepEqual(got.Set, wantSet) || got.PublishedByUserID != ruleModerator {
		t.Errorf("Theme after publishing = %+v, %t", got, ok)
	}

	// A new draft leaves the theme in effect as it is until it is published.
	second := e.saveThemeDraft(t, instance.ThemeTokenValue{Token: "radius-overlay", Value: "0.3rem"})
	if got, _ := e.theme(t); got.ID != first.ID {
		t.Errorf("Theme after saving a draft = %+v, want revision %s", got, first.ID)
	}
	published := e.publishTheme(t)
	if got, _ := e.theme(t); got.ID != published.ID || got.SetID != second.ID || len(got.Set.Tokens) != 1 {
		t.Errorf("Theme after the second publication = %+v", got)
	}
	// An earlier revision keeps its set: its stylesheet URL never changes meaning.
	if old, err := e.service.ThemeRevision(t.Context(), first.ID); err != nil || !reflect.DeepEqual(old.Set, wantSet) {
		t.Errorf("ThemeRevision(first) = %+v, %v", old, err)
	}

	// An empty set, published, resets the theme.
	e.saveThemeDraft(t)
	if reset := e.publishTheme(t); !reset.Empty() {
		t.Errorf("reset revision = %+v", reset)
	}
	if _, ok := e.theme(t); ok {
		t.Error("the theme is still in effect after a reset")
	}
}

func instanceRestoresThePreviousTheme(t *testing.T, e *instanceEnv) {
	a := e.saveThemeDraft(t, instance.ThemeTokenValue{Token: "radius-control", Value: "2px"})
	e.publishTheme(t)
	if _, err := e.service.RestoreTheme(t.Context(), ruleModerator); !errors.Is(err, appinstance.ErrNotFound) {
		t.Errorf("RestoreTheme with one set published = %v, want ErrNotFound", err)
	}
	b := e.saveThemeDraft(t, instance.ThemeTokenValue{Token: "radius-control", Value: "3px"})
	e.publishTheme(t)
	// Publishing the same set again does not make it the previous one.
	pb := e.publishTheme(t)

	e.clock.Advance(time.Second)
	restored, err := e.service.RestoreTheme(t.Context(), ruleModerator)
	if err != nil || restored.SetID != a.ID || restored.ID <= pb.ID {
		t.Fatalf("RestoreTheme = %+v, %v; want set %s under a new revision", restored, err, a.ID)
	}
	if got, _ := e.theme(t); got.ID != restored.ID || got.Set.Tokens[0].Value != "2px" {
		t.Errorf("Theme after restoring = %+v", got)
	}
	if draft, _ := e.themeDraft(t); draft.ID != b.ID {
		t.Errorf("the draft after restoring = %s, want %s", draft.ID, b.ID)
	}
	// Restoring again goes back to the set in effect before the first restore.
	e.clock.Advance(time.Second)
	if again, err := e.service.RestoreTheme(t.Context(), ruleModerator); err != nil || again.SetID != b.ID {
		t.Errorf("RestoreTheme again = %+v, %v; want set %s", again, err, b.ID)
	}
}

// instancePublishesSetsWithRetiredTokens publishes and restores a set stored before an upgrade
// retired some of its overrides (font-sans and font-mono, an override the font check now refuses):
// they are left out of the theme, never carried over, and never fail the publication.
func instancePublishesSetsWithRetiredTokens(t *testing.T, e *instanceEnv) {
	old := instance.ThemeSet{
		ID: e.ids.NewID(), SavedByUserID: ruleModerator, SavedAt: e.clock.Now(),
		Tokens: []instance.ThemeTokenValue{{Token: "radius-control", Value: "2px"}},
		Stale: []instance.StaleOverride{
			{ThemeTokenValue: instance.ThemeTokenValue{Token: "font-sans", Value: `"Noto Sans JP", sans-serif`}, Reason: instance.ThemeUnknownToken},
			{ThemeTokenValue: instance.ThemeTokenValue{Token: "font-mono", Value: `"Fira Code", monospace`}, Reason: instance.ThemeUnknownToken},
			{ThemeTokenValue: instance.ThemeTokenValue{Token: "font-sans-override", Value: "M PLUS 1p"}, Reason: instance.ThemeInvalidValue}, //nolint:gosec // a design token, not a credential
		},
	}
	if err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return errors.Join(e.h.Instance.InsertThemeSet(t.Context(), tx, old), e.h.Instance.UpsertThemeDraftHead(t.Context(), tx, old.ID))
	}); err != nil {
		t.Fatal(err)
	}
	wantTokens := []instance.ThemeTokenValue{{Token: "radius-control", Value: "2px"}}
	if rev := e.publishTheme(t); rev.SetID != old.ID || !reflect.DeepEqual(rev.Set.Tokens, wantTokens) {
		t.Errorf("PublishTheme(old draft) = %+v, want set %s with %+v", rev, old.ID, wantTokens)
	}
	e.saveThemeDraft(t, instance.ThemeTokenValue{Token: "radius-control", Value: "3px"})
	e.publishTheme(t)
	e.clock.Advance(time.Second)
	restored, err := e.service.RestoreTheme(t.Context(), ruleModerator)
	if err != nil || restored.SetID != old.ID || !reflect.DeepEqual(restored.Set.Tokens, wantTokens) {
		t.Fatalf("RestoreTheme = %+v, %v; want set %s with %+v", restored, err, old.ID, wantTokens)
	}
	if css := string(restored.Stylesheet()); strings.Contains(css, "font") {
		t.Errorf("the restored stylesheet carries a retired override:\n%s", css)
	}
}

func instanceRejectsInvalidThemes(t *testing.T, e *instanceEnv) {
	_, err := e.service.SaveThemeDraft(t.Context(), appinstance.SaveThemeDraftInput{
		Tokens: []instance.ThemeTokenValue{{Token: "color-text", Light: "#ffffff", Dark: "#ffffff"}},
	})
	var te *instance.ThemeError
	if !errors.As(err, &te) || len(te.Problems) == 0 {
		t.Fatalf("SaveThemeDraft(low contrast) = %v, want a *ThemeError", err)
	}
	if _, ok := e.themeDraft(t); ok {
		t.Error("a rejected set was saved")
	}
}

func instanceThemeProjectionEqualsRebuild(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	var last instance.ThemeRevision
	var draft instance.ThemeSet
	for i := range 5 {
		draft = e.saveThemeDraft(t, instance.ThemeTokenValue{Token: "radius-control", Value: []string{"1px", "2px", "3px", "4px", "5px"}[i]})
		if i < 4 {
			last = e.publishTheme(t)
		}
	}
	if err := appinstance.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Instance); err != nil {
		t.Fatal(err)
	}
	if got, ok := e.theme(t); !ok || got.ID != last.ID {
		t.Errorf("theme after rebuild = %+v, want revision %s", got, last.ID)
	}
	if got, ok := e.themeDraft(t); !ok || got.ID != draft.ID {
		t.Errorf("draft after rebuild = %+v, want set %s", got, draft.ID)
	}
}
