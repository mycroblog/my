// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/instance"
)

// instanceLinkCardDomainCases are the cases of InstanceRepositoryContract about the rules of the
// link card crawler: one rule per domain, listed by domain, removed at once, read by a host and
// its parents, bounded, and written only when valid.
var instanceLinkCardDomainCases = []struct {
	name string
	run  func(t *testing.T, e *instanceEnv)
}{
	{"LinkCardDomainRuleIsOnePerDomain", instanceLinkCardDomainOnePerDomain},
	{"LinkCardDomainRulesCoverSubdomains", instanceLinkCardDomainCoversSubdomains},
	{"RemovesLinkCardDomainRules", instanceLinkCardDomainRemoves},
	{"RejectsInvalidLinkCardDomainRules", instanceLinkCardDomainRejectsInvalid},
	{"BoundsLinkCardDomainRules", instanceLinkCardDomainBounds},
}

func (e *instanceEnv) setLinkCardRule(t *testing.T, domain string, mode instance.LinkCardDomainMode, note string) instance.LinkCardDomainRule {
	t.Helper()
	e.clock.Advance(time.Second)
	r, err := e.service.SetLinkCardDomainRule(t.Context(), instance.LinkCardDomainRule{Domain: domain, Mode: mode, Note: note}, ruleModerator)
	if err != nil {
		t.Fatalf("SetLinkCardDomainRule(%q, %q) = %v", domain, mode, err)
	}
	return r
}

func (e *instanceEnv) linkCardRules(t *testing.T) []instance.LinkCardDomainRule {
	t.Helper()
	rules, err := e.service.LinkCardDomainRules(t.Context())
	if err != nil {
		t.Fatalf("LinkCardDomainRules = %v", err)
	}
	return rules
}

func instanceLinkCardDomainOnePerDomain(t *testing.T, e *instanceEnv) {
	e.setLinkCardRule(t, "b.example", instance.LinkCardNoFetch, "")
	first := e.setLinkCardRule(t, "a.example", instance.LinkCardNoImages, "Heavy images")
	second := e.setLinkCardRule(t, "a.example", instance.LinkCardNoFetch, "Asked us to stop")
	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want it after %v", second.UpdatedAt, first.UpdatedAt)
	}
	rules := e.linkCardRules(t)
	if len(rules) != 2 || rules[0].Domain != "a.example" || rules[1].Domain != "b.example" {
		t.Fatalf("rules = %+v, want a.example and b.example, by domain", rules)
	}
	got := rules[0]
	if got.Mode != instance.LinkCardNoFetch || got.Note != "Asked us to stop" || !got.UpdatedAt.Equal(second.UpdatedAt) {
		t.Errorf("rule = %+v, want the second one, %+v", got, second)
	}
}

func instanceLinkCardDomainCoversSubdomains(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	e.setLinkCardRule(t, "images.example", instance.LinkCardNoImages, "")
	e.setLinkCardRule(t, "stop.example", instance.LinkCardNoFetch, "")
	e.setLinkCardRule(t, "sub.images.example", instance.LinkCardNoFetch, "")
	e.setLinkCardRule(t, "images.stop.example", instance.LinkCardNoImages, "")
	for host, want := range map[string]instance.LinkCardCrawl{
		"images.example":       {NoImages: true},
		"a.images.example":     {NoImages: true},
		"a.sub.images.example": {NoFetch: true, NoImages: true},
		"stop.example":         {NoFetch: true, NoImages: true},
		// A rule on a subdomain does not loosen the rule on its parent.
		"images.stop.example": {NoFetch: true, NoImages: true},
		"other.example":       {},
		"notimages.example":   {},
		"":                    {},
	} {
		got, err := e.service.LinkCardCrawl(ctx, host)
		if err != nil || got != want {
			t.Errorf("LinkCardCrawl(%q) = %+v, %v, want %+v", host, got, err, want)
		}
	}
}

func instanceLinkCardDomainRemoves(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	e.setLinkCardRule(t, "a.example", instance.LinkCardNoFetch, "")
	if err := e.service.RemoveLinkCardDomainRule(ctx, "a.example", ruleModerator); err != nil {
		t.Fatal(err)
	}
	if rules := e.linkCardRules(t); len(rules) != 0 {
		t.Errorf("rules = %+v, want none", rules)
	}
	if got, err := e.service.LinkCardCrawl(ctx, "a.example"); err != nil || got != (instance.LinkCardCrawl{}) {
		t.Errorf("LinkCardCrawl = %+v, %v, want nothing stopped", got, err)
	}
	if err := e.service.RemoveLinkCardDomainRule(ctx, "a.example", ruleModerator); err != nil {
		t.Errorf("removing a rule that is gone = %v", err)
	}
}

func instanceLinkCardDomainRejectsInvalid(t *testing.T, e *instanceEnv) {
	for _, tc := range []struct {
		name  string
		rule  instance.LinkCardDomainRule
		field string
		max   int
	}{
		{"empty domain", instance.LinkCardDomainRule{Mode: instance.LinkCardNoFetch}, "domain", 0},
		{"URL", instance.LinkCardDomainRule{Domain: "https://a.example/", Mode: instance.LinkCardNoFetch}, "domain", 0},
		{"upper case", instance.LinkCardDomainRule{Domain: "A.example", Mode: instance.LinkCardNoFetch}, "domain", 0},
		{"mode", instance.LinkCardDomainRule{Domain: "a.example", Mode: "off"}, "mode", 0},
		{"long note", instance.LinkCardDomainRule{
			Domain: "a.example", Mode: instance.LinkCardNoFetch, Note: strings.Repeat("あ", instance.MaxLinkCardDomainNote+1),
		}, "note", instance.MaxLinkCardDomainNote},
		{"NUL", instance.LinkCardDomainRule{Domain: "a.example", Mode: instance.LinkCardNoFetch, Note: "a\x00"}, "note", 0},
	} {
		_, err := e.service.SetLinkCardDomainRule(t.Context(), tc.rule, ruleModerator)
		var invalid *instance.LinkCardDomainError
		if !errors.As(err, &invalid) || invalid.Field != tc.field || invalid.Max != tc.max || !errors.Is(err, instance.ErrInvalidValue) {
			t.Errorf("%s: err = %v, want a LinkCardDomainError on %s", tc.name, err, tc.field)
		}
	}
	if rules := e.linkCardRules(t); len(rules) != 0 {
		t.Errorf("rules = %+v, want none written", rules)
	}
}

func instanceLinkCardDomainBounds(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	for i := range instance.MaxLinkCardDomainRules {
		if _, err := e.service.SetLinkCardDomainRule(ctx, instance.LinkCardDomainRule{
			Domain: fmt.Sprintf("d%d.example", i), Mode: instance.LinkCardNoImages,
		}, ruleModerator); err != nil {
			t.Fatalf("rule %d: %v", i, err)
		}
	}
	if _, err := e.service.SetLinkCardDomainRule(ctx, instance.LinkCardDomainRule{
		Domain: "one-more.example", Mode: instance.LinkCardNoFetch,
	}, ruleModerator); !errors.Is(err, instance.ErrTooManyLinkCardDomainRules) {
		t.Errorf("a new rule beyond the limit = %v, want ErrTooManyLinkCardDomainRules", err)
	}
	// Changing a rule at the limit adds none.
	got, err := e.service.SetLinkCardDomainRule(ctx, instance.LinkCardDomainRule{Domain: "d0.example", Mode: instance.LinkCardNoFetch}, ruleModerator)
	want := instance.LinkCardDomainRule{Domain: "d0.example", Mode: instance.LinkCardNoFetch, UpdatedAt: got.UpdatedAt}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("changing a rule at the limit = %+v, %v", got, err)
	}
}
