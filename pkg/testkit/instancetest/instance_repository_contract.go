// SPDX-License-Identifier: AGPL-3.0-only

package instancetest

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/pkg/testkit"
)

// InstanceRepositoryContract verifies the instance repository of a database adapter by its
// methods alone, the port contract of ADR-0153 §1: the revisions of the settings and the texts
// with their current values, histories newest first, and what a rebuild reads and deletes; the
// rules with their revisions, retirements, and heads by position; the rules of the link card
// crawler; the theme, the brand images, the announcements with their states, pages, and reads,
// and the service closure, each with its head; the secrets and domains of the instance,
// recorded once; and the TLS accounts, orders, and certificates with the current certificate of
// a host. The use cases on top of it are InstanceFlows, which internal/flowtest/instance
// runs once on SQLite.
func InstanceRepositoryContract(t *testing.T, newHarness InstanceFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *instanceRepo)
	}{
		{"KeepsSettingRevisions", instanceRepoSettings},
		{"KeepsTextRevisions", instanceRepoTexts},
		{"KeepsRulesByPosition", instanceRepoRules},
		{"KeepsLinkCardDomainRules", instanceRepoLinkCards},
		{"KeepsTheTheme", instanceRepoTheme},
		{"KeepsTheBrandImages", instanceRepoBrands},
		{"KeepsAnnouncements", instanceRepoAnnouncements},
		{"KeepsTheServiceClosure", instanceRepoClosure},
		{"RecordsSecretsAndDomainsOnce", instanceRepoSecretsAndDomains},
		{"RegistersOneTLSAccountPerDirectory", instanceRepoTLSAccounts},
		{"KeepsTLSCertificatesAndOrders", instanceRepoTLSCertificates},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := testkit.NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			tc.run(t, &instanceRepo{h: h, i: h.Instance, clock: clock, ids: testkit.NewIDGenerator(clock)})
		})
	}
}

type instanceRepo struct {
	h     InstanceHarness
	i     appinstance.Repository
	clock *testkit.Clock
	ids   *testkit.IDGenerator
}

func (r *instanceRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// now returns the time and moves the clock a second on.
func (r *instanceRepo) now() time.Time {
	at := r.clock.Now()
	r.clock.Advance(time.Second)
	return at
}

func wantNotFound(t *testing.T, what string, err error) {
	t.Helper()
	if !errors.Is(err, appinstance.ErrNotFound) {
		t.Errorf("%s = %v, want ErrNotFound", what, err)
	}
}

func instanceRepoSettings(t *testing.T, r *instanceRepo) {
	ctx := t.Context()
	set := func(key instance.SettingKey, value string) instance.SettingRevision {
		rev := instance.SettingRevision{ID: r.ids.NewID(), Key: key, Value: value, RevisedByUserID: testkit.NamedID("admin"), RevisedAt: r.now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return errors.Join(r.i.InsertSettingRevision(ctx, tx, rev), r.i.UpsertCurrentSetting(ctx, tx, instance.CurrentSetting{Key: key, RevisionID: rev.ID, Value: value}))
		})
		return rev
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.i.CurrentSetting(ctx, tx, instance.SettingRegistrationMode)
		wantNotFound(t, "CurrentSetting before a revision", err)
		return nil
	})
	first := set(instance.SettingRegistrationMode, "open")
	locale := set(instance.SettingDefaultLocale, "ja")
	second := set(instance.SettingRegistrationMode, "approval")

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.i.CurrentSetting(ctx, tx, instance.SettingRegistrationMode)
		if want := (instance.CurrentSetting{Key: instance.SettingRegistrationMode, RevisionID: second.ID, Value: "approval"}); err != nil || got != want {
			t.Errorf("CurrentSetting = %+v, %v; want %+v", got, err, want)
		}
		return err
	})
	want := []instance.CurrentSetting{
		{Key: instance.SettingDefaultLocale, RevisionID: locale.ID, Value: "ja"},
		{Key: instance.SettingRegistrationMode, RevisionID: second.ID, Value: "approval"},
	}
	if got, err := r.i.ListCurrentSettings(ctx); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ListCurrentSettings = %+v, %v; want %+v by key", got, err, want)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.i.ListCurrentSettingsTx(ctx, tx)
		if err == nil && !reflect.DeepEqual(got, want) {
			t.Errorf("ListCurrentSettingsTx = %+v, want %+v", got, want)
		}
		return err
	})
	if got, err := r.i.ListSettingHistory(ctx, instance.SettingRegistrationMode, 10); err != nil || !reflect.DeepEqual(got, []instance.SettingRevision{second, first}) {
		t.Errorf("ListSettingHistory = %+v, %v; want both, newest first", got, err)
	}
	if got, err := r.i.ListSettingHistory(ctx, instance.SettingRegistrationMode, 1); err != nil || !reflect.DeepEqual(got, []instance.SettingRevision{second}) {
		t.Errorf("ListSettingHistory(limit 1) = %+v, %v; want the newest", got, err)
	}
	for value, want := range map[string]bool{"open": true, "approval": true, "closed": false} {
		if got, err := r.i.IsSettingValueUsed(ctx, instance.SettingRegistrationMode, value); err != nil || got != want {
			t.Errorf("IsSettingValueUsed(%s) = %t, %v; want %t", value, got, err, want)
		}
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		revs, err := r.i.ListSettingRevisions(ctx, tx)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(revs, []instance.SettingRevision{first, locale, second}) {
			t.Errorf("ListSettingRevisions = %+v; want every revision, oldest first", revs)
		}
		return r.i.DeleteAllCurrentSettings(ctx, tx)
	})
	if got, err := r.i.ListCurrentSettings(ctx); err != nil || len(got) != 0 {
		t.Errorf("ListCurrentSettings after DeleteAllCurrentSettings = %+v, %v; want none", got, err)
	}
}

func instanceRepoTexts(t *testing.T, r *instanceRepo) {
	ctx := t.Context()
	set := func(key instance.TextKey, body string) instance.TextRevision {
		rev := instance.TextRevision{ID: r.ids.NewID(), Key: key, Body: body, RevisedByUserID: testkit.NamedID("admin"), RevisedAt: r.now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return errors.Join(r.i.InsertTextRevision(ctx, tx, rev), r.i.UpsertCurrentText(ctx, tx, instance.CurrentText{Key: key, RevisionID: rev.ID}))
		})
		return rev
	}
	_, err := r.i.FindCurrentText(ctx, instance.TextTerms)
	wantNotFound(t, "FindCurrentText before a revision", err)
	terms := set(instance.TextTerms, "Be nice.")
	name := set(instance.TextName, "Example")
	cleared := set(instance.TextTerms, "")
	if got, err := r.i.FindCurrentText(ctx, instance.TextTerms); err != nil ||
		got != (instance.Text{Key: instance.TextTerms, Body: "", RevisionID: cleared.ID, RevisedAt: cleared.RevisedAt}) {
		t.Errorf("FindCurrentText = %+v, %v; want the cleared text", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.i.CurrentText(ctx, tx, instance.TextName)
		if err == nil && got != (instance.CurrentText{Key: instance.TextName, RevisionID: name.ID}) {
			t.Errorf("CurrentText = %+v, want the revision %s", got, name.ID)
		}
		return err
	})
	all, err := r.i.ListAllCurrentTexts(ctx)
	want := []instance.Text{
		{Key: instance.TextName, Body: "Example", RevisionID: name.ID, RevisedAt: name.RevisedAt},
		{Key: instance.TextTerms, Body: "", RevisionID: cleared.ID, RevisedAt: cleared.RevisedAt},
	}
	if err != nil || !reflect.DeepEqual(all, want) {
		t.Errorf("ListAllCurrentTexts = %+v, %v; want %+v by key", all, err, want)
	}
	if got, err := r.i.ListTextHistory(ctx, instance.TextTerms, 10); err != nil || !reflect.DeepEqual(got, []instance.TextRevision{cleared, terms}) {
		t.Errorf("ListTextHistory = %+v, %v; want both, newest first", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		heads, err := r.i.ListTextRevisionHeads(ctx, tx)
		if err != nil {
			return err
		}
		wantHeads := []instance.CurrentText{{Key: instance.TextTerms, RevisionID: terms.ID}, {Key: instance.TextName, RevisionID: name.ID}, {Key: instance.TextTerms, RevisionID: cleared.ID}}
		if !reflect.DeepEqual(heads, wantHeads) {
			t.Errorf("ListTextRevisionHeads = %+v, want %+v oldest first", heads, wantHeads)
		}
		return r.i.DeleteAllCurrentTexts(ctx, tx)
	})
	if all, err := r.i.ListAllCurrentTexts(ctx); err != nil || len(all) != 0 {
		t.Errorf("ListAllCurrentTexts after DeleteAllCurrentTexts = %+v, %v; want none", all, err)
	}
}

func instanceRepoRules(t *testing.T, r *instanceRepo) {
	ctx := t.Context()
	rule := func(position int, text string) instance.Rule {
		id, created := r.ids.NewID(), r.now()
		rev := instance.RuleRevision{ID: r.ids.NewID(), RuleID: id, Position: position, Text: text, Hint: "hint", RevisedByUserID: testkit.NamedID("admin"), RevisedAt: created}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return errors.Join(r.i.InsertRule(ctx, tx, id, created), r.i.InsertRuleRevision(ctx, tx, rev),
				r.i.UpsertRuleHead(ctx, tx, instance.RuleHead{RuleID: id, RevisionID: rev.ID, Position: position}))
		})
		return instance.Rule{ID: id, CreatedAt: created, Revision: rev}
	}
	second, first := rule(2, "No spam"), rule(1, "Be nice")
	retired := rule(3, "No ads")
	// A new revision moves a rule.
	moved := instance.RuleRevision{ID: r.ids.NewID(), RuleID: first.ID, Position: 4, Text: "Be kind", RevisedAt: r.now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.i.InsertRuleRevision(ctx, tx, moved), r.i.UpsertRuleHead(ctx, tx, instance.RuleHead{RuleID: first.ID, RevisionID: moved.ID, Position: 4}))
	})
	first.Revision = moved
	retirement := instance.RuleRetirement{RuleID: retired.ID, RetiredByUserID: testkit.NamedID("admin"), RetiredAt: r.now()}
	var ok, again bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		if ok, err = r.i.InsertRuleRetirement(ctx, tx, retirement); err != nil {
			return err
		}
		if again, err = r.i.InsertRuleRetirement(ctx, tx, retirement); err != nil {
			return err
		}
		return r.i.DeleteRuleHead(ctx, tx, retired.ID)
	})
	if !ok || again {
		t.Errorf("InsertRuleRetirement = %t, then %t; want true, then false", ok, again)
	}
	retired.Retirement = &retirement

	if got, err := r.i.FindRule(ctx, first.ID); err != nil || !reflect.DeepEqual(got, first) {
		t.Errorf("FindRule = %+v, %v; want %+v with its latest revision", got, err, first)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.i.Rule(ctx, tx, retired.ID)
		if err == nil && !reflect.DeepEqual(got, retired) {
			t.Errorf("Rule = %+v, want %+v with its retirement", got, retired)
		}
		head, err := r.i.RuleHead(ctx, tx, second.ID)
		if err == nil && head != (instance.RuleHead{RuleID: second.ID, RevisionID: second.Revision.ID, Position: 2}) {
			t.Errorf("RuleHead = %+v", head)
		}
		_, herr := r.i.RuleHead(ctx, tx, retired.ID)
		wantNotFound(t, "RuleHead of a retired rule", herr)
		heads, lerr := r.i.ListRuleHeads(ctx, tx)
		if lerr == nil && (len(heads) != 2 || heads[0].RuleID != second.ID || heads[1].RuleID != first.ID) {
			t.Errorf("ListRuleHeads = %+v; want the rules in effect by position", heads)
		}
		revs, rerr := r.i.ListRuleRevisionHeads(ctx, tx)
		if rerr == nil && len(revs) != 4 {
			t.Errorf("ListRuleRevisionHeads = %+v; want the 4 revisions", revs)
		}
		ids, ierr := r.i.ListRetiredRuleIDs(ctx, tx)
		if ierr == nil && !slices.Equal(ids, []string{retired.ID}) {
			t.Errorf("ListRetiredRuleIDs = %v, want %s", ids, retired.ID)
		}
		return errors.Join(err, lerr, rerr, ierr)
	})
	ids := func(rules []instance.Rule) []string {
		var out []string
		for _, rule := range rules {
			out = append(out, rule.ID)
		}
		return out
	}
	if got, err := r.i.ListRules(ctx, false); err != nil || !slices.Equal(ids(got), []string{second.ID, first.ID}) {
		t.Errorf("ListRules = %v, %v; want the rules in effect by position", ids(got), err)
	}
	if got, err := r.i.ListRules(ctx, true); err != nil || !slices.Equal(ids(got), []string{second.ID, first.ID, retired.ID}) {
		t.Errorf("ListRules(retired too) = %v, %v; want the retired rule last", ids(got), err)
	}
	_, err := r.i.FindRule(ctx, r.ids.NewID())
	wantNotFound(t, "FindRule(unknown)", err)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.i.DeleteAllRuleHeads(ctx, tx) })
	if got, err := r.i.ListRules(ctx, false); err != nil || len(got) != 0 {
		t.Errorf("ListRules after DeleteAllRuleHeads = %v, %v; want none", ids(got), err)
	}
}

func instanceRepoLinkCards(t *testing.T, r *instanceRepo) {
	ctx := t.Context()
	put := func(domain string, mode instance.LinkCardDomainMode) instance.LinkCardDomainRule {
		rule := instance.LinkCardDomainRule{Domain: domain, Mode: mode, Note: "noisy", UpdatedAt: r.now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.i.UpsertLinkCardDomainRule(ctx, tx, rule) })
		return rule
	}
	put("b.example", instance.LinkCardNoImages)
	a := put("a.example", instance.LinkCardNoFetch)
	b := put("b.example", instance.LinkCardNoFetch)
	c := put("c.example", instance.LinkCardNoImages)
	got, err := r.i.ListLinkCardDomainRules(ctx)
	for i := range got {
		got[i].UpdatedAt = got[i].UpdatedAt.UTC()
	}
	if err != nil || !reflect.DeepEqual(got, []instance.LinkCardDomainRule{a, b, c}) {
		t.Errorf("ListLinkCardDomainRules = %+v, %v; want the rules by domain, b.example changed", got, err)
	}
	for _, q := range []struct {
		domains []string
		want    []instance.LinkCardDomainMode
	}{
		{[]string{"a.example", "c.example", "d.example"}, []instance.LinkCardDomainMode{instance.LinkCardNoFetch, instance.LinkCardNoImages}},
		{[]string{"a.example", "b.example"}, []instance.LinkCardDomainMode{instance.LinkCardNoFetch}},
		{[]string{"d.example"}, nil},
	} {
		got, err := r.i.LinkCardDomainModes(ctx, q.domains)
		if err != nil || !slices.Equal(got, q.want) {
			t.Errorf("LinkCardDomainModes(%v) = %v, %v; want %v", q.domains, got, err, q.want)
		}
	}
	// The modes of each domain, as those of the domain alone.
	of, err := r.i.LinkCardDomainModesOf(ctx, []string{"a.example", "b.example", "c.example", "d.example"})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"a.example", "b.example", "c.example", "d.example"} {
		one, err := r.i.LinkCardDomainModes(ctx, []string{d})
		if err != nil || !slices.Equal(of[d], one) {
			t.Errorf("LinkCardDomainModesOf[%s] = %v; want %v, as LinkCardDomainModes (%v)", d, of[d], one, err)
		}
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		deleted, err := r.i.DeleteLinkCardDomainRule(ctx, tx, "b.example")
		if err != nil {
			return err
		}
		again, err := r.i.DeleteLinkCardDomainRule(ctx, tx, "b.example")
		if err != nil {
			return err
		}
		has, err := r.i.HasLinkCardDomainRule(ctx, tx, "a.example")
		if err != nil {
			return err
		}
		gone, err := r.i.HasLinkCardDomainRule(ctx, tx, "b.example")
		if err != nil {
			return err
		}
		n, err := r.i.CountLinkCardDomainRules(ctx, tx)
		if !deleted || again || !has || gone || n != 2 {
			t.Errorf("DeleteLinkCardDomainRule = %t, then %t; Has(a) %t, Has(b) %t; Count %d; want true, false, true, false, 2", deleted, again, has, gone, n)
		}
		return err
	})
}

// instanceRepoTheme keeps token sets as documents, with the overrides that no longer apply apart,
// their publications, the draft and the published heads, and the publication before another.
func instanceRepoTheme(t *testing.T, r *instanceRepo) {
	ctx := t.Context()
	_, err := r.i.CurrentTheme(ctx)
	wantNotFound(t, "CurrentTheme before a publication", err)
	_, err = r.i.ThemeDraft(ctx)
	wantNotFound(t, "ThemeDraft before a set", err)
	save := func(stale ...instance.StaleOverride) instance.ThemeSet {
		set := instance.ThemeSet{ID: r.ids.NewID(), SavedByUserID: testkit.NamedID("admin"), SavedAt: r.now(), Tokens: []instance.ThemeTokenValue{
			{Token: "color-accent", Light: "#123456", Dark: "#abcdef"}, {Token: "radius-control", Value: "4px"},
		}, Stale: stale}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return errors.Join(r.i.InsertThemeSet(ctx, tx, set), r.i.UpsertThemeDraftHead(ctx, tx, set.ID))
		})
		return set
	}
	publish := func(set instance.ThemeSet, by string) instance.ThemeRevision {
		rev := instance.ThemeRevision{ThemePublication: instance.ThemePublication{ID: r.ids.NewID(), SetID: set.ID, PublishedByUserID: by, PublishedAt: r.now()}, Set: set}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return errors.Join(r.i.InsertThemePublication(ctx, tx, rev.ThemePublication), r.i.UpsertThemeHead(ctx, tx, instance.ThemeHead{RevisionID: rev.ID}))
		})
		return rev
	}
	first := save()
	// A stored override of a retired token stays in the document and comes back apart.
	second := save(instance.StaleOverride{ThemeTokenValue: instance.ThemeTokenValue{Token: "elevation-card", Value: "none"}, Reason: instance.ThemeUnknownToken})
	p1 := publish(first, testkit.NamedID("admin"))
	p2 := publish(second, "")

	if got, err := r.i.CurrentTheme(ctx); err != nil || !reflect.DeepEqual(got, p2) {
		t.Errorf("CurrentTheme = %+v, %v; want %+v", got, err, p2)
	}
	if got, err := r.i.FindThemeRevision(ctx, p1.ID); err != nil || !reflect.DeepEqual(got, p1) {
		t.Errorf("FindThemeRevision = %+v, %v; want %+v", got, err, p1)
	}
	_, err = r.i.FindThemeRevision(ctx, r.ids.NewID())
	wantNotFound(t, "FindThemeRevision(unknown)", err)
	if got, err := r.i.ThemeDraft(ctx); err != nil || !reflect.DeepEqual(got, second) {
		t.Errorf("ThemeDraft = %+v, %v; want %+v", got, err, second)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		// Inserting a set or a publication again leaves the first one.
		again := first
		again.Tokens = nil
		if err := errors.Join(r.i.InsertThemeSet(ctx, tx, again), r.i.InsertThemePublication(ctx, tx, instance.ThemePublication{ID: p1.ID, SetID: second.ID, PublishedAt: r.now()})); err != nil {
			return err
		}
		if got, err := r.i.FindThemeSet(ctx, tx, first.ID); err != nil || !reflect.DeepEqual(got, first) {
			t.Errorf("FindThemeSet after inserting again = %+v, %v; want %+v", got, err, first)
		}
		if got, err := r.i.FindThemePublication(ctx, tx, p1.ID); err != nil || got != p1.ThemePublication {
			t.Errorf("FindThemePublication after inserting again = %+v, %v; want %+v", got, err, p1.ThemePublication)
		}
		if prev, err := r.i.PreviousThemePublication(ctx, tx, p2.ThemePublication); err != nil || prev != p1.ThemePublication {
			t.Errorf("PreviousThemePublication(second) = %+v, %v; want %+v", prev, err, p1.ThemePublication)
		}
		_, err := r.i.PreviousThemePublication(ctx, tx, p1.ThemePublication)
		wantNotFound(t, "PreviousThemePublication(first)", err)
		_, err = r.i.FindThemeSet(ctx, tx, r.ids.NewID())
		wantNotFound(t, "FindThemeSet(unknown)", err)

		head, err := r.i.ThemeHead(ctx, tx)
		if err == nil && head.RevisionID != p2.ID {
			t.Errorf("ThemeHead = %+v, want %s", head, p2.ID)
		}
		latest, lerr := r.i.LatestThemeRevisionID(ctx, tx)
		if lerr == nil && latest != p2.ID {
			t.Errorf("LatestThemeRevisionID = %s, want %s", latest, p2.ID)
		}
		draft, derr := r.i.ThemeDraftHead(ctx, tx)
		if derr == nil && draft != second.ID {
			t.Errorf("ThemeDraftHead = %s, want %s", draft, second.ID)
		}
		latestSet, serr := r.i.LatestThemeSetID(ctx, tx)
		if serr == nil && latestSet != second.ID {
			t.Errorf("LatestThemeSetID = %s, want %s", latestSet, second.ID)
		}
		return errors.Join(err, lerr, derr, serr, r.i.DeleteThemeHead(ctx, tx), r.i.DeleteThemeDraftHead(ctx, tx))
	})
	_, err = r.i.CurrentTheme(ctx)
	wantNotFound(t, "CurrentTheme after DeleteThemeHead", err)
	_, err = r.i.ThemeDraft(ctx)
	wantNotFound(t, "ThemeDraft after DeleteThemeDraftHead", err)
}

func instanceRepoBrands(t *testing.T, r *instanceRepo) {
	ctx := t.Context()
	revise := func(kind instance.BrandKind, action instance.BrandAction) instance.BrandRevision {
		rev := instance.BrandRevision{ID: r.ids.NewID(), Kind: kind, Action: action, RevisedByUserID: testkit.NamedID("admin"), RevisedAt: r.now()}
		if action == instance.BrandSet {
			if kind == instance.BrandIcon {
				rev.Background = "#ffffff"
			}
			rev.Files = []instance.BrandFile{{Rendition: instance.RenditionSVG, Key: "brand/" + rev.ID + ".svg", ContentType: "image/svg+xml", Width: 64, Height: 64, Size: 512, SHA256: strings.Repeat("ab", 32)}}
		}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return errors.Join(r.i.InsertBrandRevision(ctx, tx, rev), r.i.UpsertBrandHead(ctx, tx, instance.BrandHead{Kind: kind, RevisionID: rev.ID}))
		})
		return rev
	}
	icon := revise(instance.BrandIcon, instance.BrandSet)
	logo := revise(instance.BrandLogo, instance.BrandSet)
	if got, err := r.i.CurrentBrands(ctx); err != nil || got.Icon == nil || !reflect.DeepEqual(*got.Icon, icon) || got.Logo == nil || got.LogoDark != nil {
		t.Errorf("CurrentBrands = %+v, %v; want the icon and the logo", got, err)
	}
	if got, err := r.i.FindBrandRevision(ctx, logo.ID); err != nil || !reflect.DeepEqual(got, logo) {
		t.Errorf("FindBrandRevision = %+v, %v; want %+v with its files", got, err, logo)
	}
	_, err := r.i.FindBrandRevision(ctx, r.ids.NewID())
	wantNotFound(t, "FindBrandRevision(unknown)", err)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		head, err := r.i.BrandHead(ctx, tx, instance.BrandLogo)
		if err == nil && head != (instance.BrandHead{Kind: instance.BrandLogo, RevisionID: logo.ID}) {
			t.Errorf("BrandHead = %+v", head)
		}
		_, derr := r.i.BrandHead(ctx, tx, instance.BrandLogoDark)
		wantNotFound(t, "BrandHead of a kind never set", derr)
		return errors.Join(err, r.i.DeleteBrandHead(ctx, tx, instance.BrandLogo))
	})
	cleared := revise(instance.BrandIcon, instance.BrandClear)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		heads, err := r.i.ListBrandRevisionHeads(ctx, tx)
		if err == nil && (len(heads) != 3 || heads[0].ID != icon.ID || heads[1].ID != logo.ID || heads[2].ID != cleared.ID ||
			heads[2].Kind != instance.BrandIcon || heads[2].Action != instance.BrandClear) {
			t.Errorf("ListBrandRevisionHeads = %+v; want the three revisions oldest first", heads)
		}
		return errors.Join(err, r.i.DeleteAllBrandHeads(ctx, tx))
	})
	if got, err := r.i.CurrentBrands(ctx); err != nil || got.Icon != nil || got.Logo != nil || got.LogoDark != nil {
		t.Errorf("CurrentBrands after DeleteAllBrandHeads = %+v, %v; want none", got, err)
	}
}

func instanceRepoAnnouncements(t *testing.T, r *instanceRepo) {
	ctx := t.Context()
	now := r.clock.Now()
	announce := func(title string, state instance.AnnouncementState, starts, ends time.Time) instance.Announcement {
		id, created := r.ids.NewID(), r.now()
		rev := instance.AnnouncementRevision{ID: r.ids.NewID(), AnnouncementID: id, Title: title, Body: "Body of " + title, StartsAt: starts, EndsAt: ends, RevisedByUserID: testkit.NamedID("admin"), RevisedAt: created}
		change := instance.AnnouncementStateChange{ID: r.ids.NewID(), AnnouncementID: id, State: state, ChangedByUserID: testkit.NamedID("admin"), ChangedAt: created}
		head := instance.AnnouncementHead{AnnouncementID: id, RevisionID: rev.ID, State: state}
		if state != instance.AnnouncementDraft {
			head.PublishedAt = created
		}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			if err := errors.Join(r.i.InsertAnnouncement(ctx, tx, id, created), r.i.InsertAnnouncementRevision(ctx, tx, rev)); err != nil {
				return err
			}
			// A draft has no state change of its own.
			if state != instance.AnnouncementDraft {
				if err := r.i.InsertAnnouncementStateChange(ctx, tx, change); err != nil {
					return err
				}
			}
			return r.i.UpsertAnnouncementHead(ctx, tx, head)
		})
		return instance.Announcement{ID: id, CreatedAt: created, Revision: rev, State: state, PublishedAt: head.PublishedAt}
	}
	hour := time.Hour
	draft := announce("Draft", instance.AnnouncementDraft, time.Time{}, time.Time{})
	showing := announce("Showing", instance.AnnouncementPublished, time.Time{}, now.Add(hour))
	scheduled := announce("Scheduled", instance.AnnouncementPublished, now.Add(hour), now.Add(2*hour))
	ended := announce("Ended", instance.AnnouncementPublished, now.Add(-2*hour), now.Add(-hour))
	withdrawn := announce("Withdrawn", instance.AnnouncementWithdrawn, time.Time{}, time.Time{})

	if got, err := r.i.FindAnnouncement(ctx, showing.ID); err != nil || !reflect.DeepEqual(got, showing) {
		t.Errorf("FindAnnouncement = %+v, %v; want %+v", got, err, showing)
	}
	_, err := r.i.FindAnnouncement(ctx, r.ids.NewID())
	wantNotFound(t, "FindAnnouncement(unknown)", err)
	if got, err := r.i.FindAnnouncements(ctx, []string{showing.ID, withdrawn.ID, r.ids.NewID()}); err != nil ||
		!reflect.DeepEqual(got, map[string]instance.Announcement{showing.ID: showing, withdrawn.ID: withdrawn}) {
		t.Errorf("FindAnnouncements = %+v, %v; want the two that exist", got, err)
	}
	ids := func(as []instance.Announcement) []string {
		var out []string
		for _, a := range as {
			out = append(out, a.ID)
		}
		return out
	}
	if got, err := r.i.ListAnnouncements(ctx, false); err != nil || !slices.Equal(ids(got), []string{withdrawn.ID, ended.ID, scheduled.ID, showing.ID, draft.ID}) {
		t.Errorf("ListAnnouncements = %v, %v; want every one, newest first", ids(got), err)
	}
	if got, err := r.i.ListAnnouncements(ctx, true); err != nil || !slices.Equal(ids(got), []string{ended.ID, scheduled.ID, showing.ID}) {
		t.Errorf("ListAnnouncements(published) = %v, %v; want the published ones, newest first", ids(got), err)
	}
	for _, q := range []struct {
		filter appinstance.AnnouncementFilter
		before string
		limit  int
		want   []string
	}{
		{appinstance.AnnouncementFilterAll, "", 2, []string{withdrawn.ID, ended.ID}},
		{appinstance.AnnouncementFilterAll, ended.ID, 2, []string{scheduled.ID, showing.ID}},
		{appinstance.AnnouncementFilterDraft, "", 10, []string{draft.ID}},
		{appinstance.AnnouncementFilterWithdrawn, "", 10, []string{withdrawn.ID}},
		{appinstance.AnnouncementFilterScheduled, "", 10, []string{scheduled.ID}},
		{appinstance.AnnouncementFilterShowing, "", 10, []string{showing.ID}},
		{appinstance.AnnouncementFilterEnded, "", 10, []string{ended.ID}},
	} {
		got, err := r.i.ListAnnouncementPage(ctx, appinstance.AnnouncementQuery{Filter: q.filter, Now: now, Before: q.before}, q.limit)
		if err != nil || !slices.Equal(ids(got), q.want) {
			t.Errorf("ListAnnouncementPage(%q, before %s) = %v, %v; want %v", q.filter, q.before, ids(got), err, q.want)
		}
	}

	user := r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.i.InsertAnnouncementRead(ctx, tx, user, showing.ID, now), r.i.InsertAnnouncementRead(ctx, tx, user, showing.ID, now.Add(hour)),
			r.i.InsertAnnouncementRead(ctx, tx, r.ids.NewID(), ended.ID, now))
	})
	if got, err := r.i.ListReadAnnouncementIDs(ctx, user, []string{showing.ID, ended.ID}); err != nil || !slices.Equal(got, []string{showing.ID}) {
		t.Errorf("ListReadAnnouncementIDs = %v, %v; want the one the user read", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.i.DeleteAnnouncementReadsOfUser(ctx, tx, user) })
	if got, err := r.i.ListReadAnnouncementIDs(ctx, user, []string{showing.ID}); err != nil || len(got) != 0 {
		t.Errorf("ListReadAnnouncementIDs after the delete = %v, %v; want none", got, err)
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.i.Announcement(ctx, tx, draft.ID)
		if err == nil && !reflect.DeepEqual(got, draft) {
			t.Errorf("Announcement = %+v, want %+v", got, draft)
		}
		head, herr := r.i.AnnouncementHead(ctx, tx, showing.ID)
		if herr == nil && head != (instance.AnnouncementHead{AnnouncementID: showing.ID, RevisionID: showing.Revision.ID, State: instance.AnnouncementPublished, PublishedAt: showing.PublishedAt}) {
			t.Errorf("AnnouncementHead = %+v", head)
		}
		revs, rerr := r.i.ListAnnouncementRevisionIDs(ctx, tx)
		if rerr == nil && (len(revs) != 5 || revs[0] != (instance.AnnouncementHead{AnnouncementID: draft.ID, RevisionID: draft.Revision.ID})) {
			t.Errorf("ListAnnouncementRevisionIDs = %+v; want the five revisions oldest first", revs)
		}
		changes, cerr := r.i.ListAnnouncementStateChanges(ctx, tx)
		if cerr == nil && (len(changes) != 4 || changes[3].AnnouncementID != withdrawn.ID || changes[3].State != instance.AnnouncementWithdrawn) {
			t.Errorf("ListAnnouncementStateChanges = %+v; want the four changes oldest first", changes)
		}
		return errors.Join(err, herr, rerr, cerr, r.i.DeleteAllAnnouncementHeads(ctx, tx))
	})
	if got, err := r.i.ListAnnouncements(ctx, false); err != nil || len(got) != 0 {
		t.Errorf("ListAnnouncements after DeleteAllAnnouncementHeads = %v, %v; want none", ids(got), err)
	}
}

func instanceRepoClosure(t *testing.T, r *instanceRepo) {
	ctx := t.Context()
	_, err := r.i.CurrentClosure(ctx)
	wantNotFound(t, "CurrentClosure before an announcement", err)
	gate := func(name string, want instance.Closure, ok bool, mode string) {
		t.Helper()
		if got, gok, gmode, err := r.i.FederationGate(ctx); err != nil || gok != ok || gmode != mode || (ok && got != want) {
			t.Errorf("FederationGate %s = %+v, %v, %q, %v; want %+v, %v, %q", name, got, gok, gmode, err, want, ok, mode)
		}
	}
	gate("before an announcement or a mode", instance.Closure{}, false, "")
	rev := instance.SettingRevision{ID: r.ids.NewID(), Key: instance.SettingFederationMode, Value: "outbound", RevisedByUserID: testkit.NamedID("admin"), RevisedAt: r.now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.i.InsertSettingRevision(ctx, tx, rev), r.i.UpsertCurrentSetting(ctx, tx, instance.CurrentSetting{Key: rev.Key, RevisionID: rev.ID, Value: rev.Value}))
	})
	gate("with a mode alone", instance.Closure{}, false, "outbound")
	announced := instance.ClosureEvent{ID: r.ids.NewID(), Kind: instance.ClosureEventAnnounced, ByUserID: testkit.NamedID("admin"), OccurredAt: r.now(),
		Schedule: instance.ClosureSchedule{ClosesAt: r.clock.Now().Add(30 * 24 * time.Hour), ExportUntil: r.clock.Now().Add(60 * 24 * time.Hour), Message: "Thank you"}}
	closed := instance.ClosureEvent{ID: r.ids.NewID(), Kind: instance.ClosureEventClosed, OccurredAt: r.now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.i.InsertClosureEvent(ctx, tx, announced),
			r.i.UpsertClosureHead(ctx, tx, instance.ClosureHead{State: instance.ClosureAnnounced, EventID: announced.ID, AnnouncementID: announced.ID}))
	})
	want := instance.Closure{State: instance.ClosureAnnounced, AnnouncementID: announced.ID, Schedule: announced.Schedule, AnnouncedAt: announced.OccurredAt, ChangedAt: announced.OccurredAt}
	if got, err := r.i.CurrentClosure(ctx); err != nil || got != want {
		t.Errorf("CurrentClosure = %+v, %v; want %+v", got, err, want)
	}
	gate("with a closure and a mode", want, true, "outbound")
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.i.InsertClosureEvent(ctx, tx, closed),
			r.i.UpsertClosureHead(ctx, tx, instance.ClosureHead{State: instance.ClosureClosed, EventID: closed.ID, AnnouncementID: announced.ID}))
	})
	want.State, want.ChangedAt = instance.ClosureClosed, closed.OccurredAt
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.i.CurrentClosureTx(ctx, tx)
		if err == nil && got != want {
			t.Errorf("CurrentClosureTx = %+v, want %+v", got, want)
		}
		head, herr := r.i.ClosureHead(ctx, tx)
		if herr == nil && head != (instance.ClosureHead{State: instance.ClosureClosed, EventID: closed.ID, AnnouncementID: announced.ID}) {
			t.Errorf("ClosureHead = %+v", head)
		}
		events, eerr := r.i.ListClosureEvents(ctx, tx)
		if eerr == nil && (len(events) != 2 || !sameClosureEvent(events[0], announced) || !sameClosureEvent(events[1], closed)) {
			t.Errorf("ListClosureEvents = %+v; want both, oldest first", events)
		}
		return errors.Join(err, herr, eerr)
	})
	if got, err := r.i.FindClosureEvent(ctx, announced.ID); err != nil || !sameClosureEvent(got, announced) {
		t.Errorf("FindClosureEvent = %+v, %v; want %+v with its schedule", got, err, announced)
	}
	_, err = r.i.FindClosureEvent(ctx, r.ids.NewID())
	wantNotFound(t, "FindClosureEvent(unknown)", err)
	if got, err := r.i.FindClosureEvents(ctx, []string{announced.ID, closed.ID, r.ids.NewID()}); err != nil || len(got) != 2 ||
		!sameClosureEvent(got[announced.ID], announced) || !sameClosureEvent(got[closed.ID], closed) {
		t.Errorf("FindClosureEvents = %+v, %v; want the two that exist", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.i.DeleteClosureHead(ctx, tx) })
	_, err = r.i.CurrentClosure(ctx)
	wantNotFound(t, "CurrentClosure after DeleteClosureHead", err)
}

// sameClosureEvent compares two closure events, their times at the same instant.
func sameClosureEvent(a, b instance.ClosureEvent) bool {
	return a.ID == b.ID && a.Kind == b.Kind && a.ByUserID == b.ByUserID && a.OccurredAt.Equal(b.OccurredAt) &&
		a.Schedule.ClosesAt.Equal(b.Schedule.ClosesAt) && a.Schedule.ExportUntil.Equal(b.Schedule.ExportUntil) && a.Schedule.Message == b.Schedule.Message
}

func instanceRepoSecretsAndDomains(t *testing.T, r *instanceRepo) {
	ctx := t.Context()
	_, err := r.i.FindSecret(ctx, instance.SecretMediaProxyKey)
	wantNotFound(t, "FindSecret before an insert", err)
	first := instance.Secret{ID: r.ids.NewID(), Name: instance.SecretMediaProxyKey, Sealed: []byte("sealed-1"), CreatedAt: r.now()}
	second := instance.Secret{ID: r.ids.NewID(), Name: instance.SecretMediaProxyKey, Sealed: []byte("sealed-2"), CreatedAt: r.now()}
	var ok, again bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		if ok, err = r.i.InsertSecret(ctx, tx, first); err != nil {
			return err
		}
		again, err = r.i.InsertSecret(ctx, tx, second)
		return err
	})
	if got, err := r.i.FindSecret(ctx, instance.SecretMediaProxyKey); !ok || again || err != nil || !reflect.DeepEqual(got, first) {
		t.Errorf("InsertSecret = %t, then %t; FindSecret = %+v, %v; want the first secret only", ok, again, got, err)
	}

	_, err = r.i.FindDomains(ctx)
	wantNotFound(t, "FindDomains before an insert", err)
	d := instance.Domains{Host: "social.example", AccountDomain: "example", RecordedAt: r.now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		if ok, err = r.i.InsertDomains(ctx, tx, d); err != nil {
			return err
		}
		again, err = r.i.InsertDomains(ctx, tx, instance.Domains{Host: "other.example", AccountDomain: "other.example", RecordedAt: r.now()})
		return err
	})
	if got, err := r.i.FindDomains(ctx); !ok || again || err != nil || got != d {
		t.Errorf("InsertDomains = %t, then %t; FindDomains = %+v, %v; want the first domains only", ok, again, got, err)
	}
}
