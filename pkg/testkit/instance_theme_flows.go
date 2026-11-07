// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"reflect"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
)

// instanceThemeCases are the cases of InstanceFlows about the instance theme
// (ADR-0059): no theme until one is saved, revisions that keep their tokens and CSS and stay
// readable after a newer one, a reset by an empty revision, invalid themes that write nothing, and
// the instance_theme projection equal to a rebuild.
var instanceThemeCases = []struct {
	name string
	run  func(t *testing.T, e *instanceEnv)
}{
	{"HasNoThemeUntilSaved", instanceHasNoTheme},
	{"KeepsThemeRevisions", instanceKeepsThemeRevisions},
	{"RejectsInvalidThemes", instanceRejectsInvalidThemes},
	{"ThemeProjectionEqualsRebuild", instanceThemeProjectionEqualsRebuild},
}

func (e *instanceEnv) setTheme(t *testing.T, css string, tokens ...instance.ThemeTokenValue) instance.ThemeRevision {
	t.Helper()
	e.clock.Advance(time.Second)
	rev, err := e.service.SetTheme(t.Context(), appinstance.SetThemeInput{Tokens: tokens, CustomCSS: css, RevisedByUserID: ruleModerator})
	if err != nil {
		t.Fatalf("SetTheme = %v", err)
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

func instanceHasNoTheme(t *testing.T, e *instanceEnv) {
	if _, ok := e.theme(t); ok {
		t.Error("a new instance has a theme")
	}
	if _, err := e.service.ThemeRevision(t.Context(), "0199f0e0-0000-7000-8000-000000000000"); !errors.Is(err, appinstance.ErrNotFound) {
		t.Errorf("ThemeRevision(unknown) = %v, want ErrNotFound", err)
	}
}

func instanceKeepsThemeRevisions(t *testing.T, e *instanceEnv) {
	first := e.setTheme(t, ".a { color: red }",
		instance.ThemeTokenValue{Token: "radius-md", Value: "2px"},
		instance.ThemeTokenValue{Token: "color-surface", Light: "#FFFDF5", Dark: "#0a0a0a"},
	)
	got, ok := e.theme(t)
	if !ok {
		t.Fatal("no theme after saving one")
	}
	want := instance.ThemeRevision{
		ID: first.ID, CustomCSS: ".a { color: red }", RevisedByUserID: ruleModerator, RevisedAt: first.RevisedAt,
		Tokens: []instance.ThemeTokenValue{
			{Token: "color-surface", Light: "#fffdf5", Dark: "#0a0a0a"},
			{Token: "radius-md", Value: "2px"},
		},
	}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(first, want) {
		t.Errorf("Theme = %+v\nSetTheme = %+v\nwant %+v", got, first, want)
	}

	second := e.setTheme(t, "", instance.ThemeTokenValue{Token: "spacing", Value: "0.3rem"})
	if got, _ := e.theme(t); got.ID != second.ID || got.CustomCSS != "" || len(got.Tokens) != 1 {
		t.Errorf("Theme after the second save = %+v", got)
	}
	// An earlier revision keeps its content: its stylesheet URL never changes meaning.
	old, err := e.service.ThemeRevision(t.Context(), first.ID)
	if err != nil || !reflect.DeepEqual(old, want) {
		t.Errorf("ThemeRevision(first) = %+v, %v", old, err)
	}

	reset := e.setTheme(t, "")
	if !reset.Empty() {
		t.Errorf("reset revision = %+v", reset)
	}
	if _, ok := e.theme(t); ok {
		t.Error("the theme is still in effect after a reset")
	}
}

func instanceRejectsInvalidThemes(t *testing.T, e *instanceEnv) {
	_, err := e.service.SetTheme(t.Context(), appinstance.SetThemeInput{
		Tokens: []instance.ThemeTokenValue{{Token: "color-fg", Light: "#ffffff", Dark: "#ffffff"}},
	})
	var te *instance.ThemeError
	if !errors.As(err, &te) || len(te.Problems) == 0 {
		t.Fatalf("SetTheme(low contrast) = %v, want a *ThemeError", err)
	}
	if _, ok := e.theme(t); ok {
		t.Error("a rejected theme was saved")
	}
}

func instanceThemeProjectionEqualsRebuild(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	var last instance.ThemeRevision
	for i := range 5 {
		last = e.setTheme(t, "", instance.ThemeTokenValue{Token: "radius-md", Value: []string{"1px", "2px", "3px", "4px", "5px"}[i]})
	}
	if err := appinstance.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Instance); err != nil {
		t.Fatal(err)
	}
	got, ok := e.theme(t)
	if !ok || got.ID != last.ID {
		t.Errorf("theme after rebuild = %+v, want revision %s", got, last.ID)
	}
}
