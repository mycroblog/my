// SPDX-License-Identifier: AGPL-3.0-only

package instancetest

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
)

// instanceRuleCases are the cases of InstanceFlows about the instance rules
// (FR-705): a rule keeps its ID across revisions, retired rules leave the rules in effect but
// stay readable, moves renumber the rules, invalid input writes nothing, and the
// instance_rule_heads projection equals a rebuild.
var instanceRuleCases = []struct {
	name string
	run  func(t *testing.T, e *instanceEnv)
}{
	{"RuleRevisionsKeepTheRuleID", instanceRuleRevisionsKeepID},
	{"RetiredRulesLeaveTheRulesInEffect", instanceRetiredRules},
	{"MovesRules", instanceMovesRules},
	{"RejectsInvalidRules", instanceRejectsInvalidRules},
	{"BoundsTheRulesInEffect", instanceBoundsRules},
	{"RuleProjectionEqualsRebuild", instanceRuleProjectionEqualsRebuild},
}

const ruleModerator = "0199f0e0-0000-7000-8000-0000000000aa"

func (e *instanceEnv) addRule(t *testing.T, text, hint string) instance.Rule {
	t.Helper()
	e.clock.Advance(time.Second)
	r, err := e.service.AddRule(t.Context(), appinstance.AddRuleInput{Text: text, Hint: hint, CreatedByUserID: ruleModerator})
	if err != nil {
		t.Fatalf("AddRule(%q, %q) = %v", text, hint, err)
	}
	return r
}

func (e *instanceEnv) rules(t *testing.T, includeRetired bool) []instance.Rule {
	t.Helper()
	rules, err := e.service.Rules(t.Context(), includeRetired)
	if err != nil {
		t.Fatalf("Rules(%v) = %v", includeRetired, err)
	}
	return rules
}

// ruleOrder renders rules as "position:text" in order.
func ruleOrder(rules []instance.Rule) []string {
	out := make([]string, len(rules))
	for i, r := range rules {
		out[i] = fmt.Sprintf("%d:%s", r.Revision.Position, r.Revision.Text)
	}
	return out
}

func instanceRuleRevisionsKeepID(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	added := e.addRule(t, "Be kind", "No harassment.")
	if added.Revision.Text != "Be kind" || added.Revision.Hint != "No harassment." {
		t.Errorf("added = %+v, want the text and the hint", added.Revision)
	}
	if added.Revision.Position != 1 || added.Revision.RevisedByUserID != ruleModerator || added.Retired() {
		t.Errorf("added rule = %+v, want position 1 by the moderator, in effect", added)
	}

	e.clock.Advance(time.Second)
	revised, err := e.service.ReviseRule(ctx, appinstance.ReviseRuleInput{RuleID: added.ID, Text: "親切に"})
	if err != nil {
		t.Fatalf("ReviseRule = %v", err)
	}
	if revised.ID != added.ID || revised.Revision.ID == added.Revision.ID || revised.Revision.Position != 1 ||
		revised.Revision.Text != "親切に" || revised.Revision.Hint != "" || revised.Revision.RevisedByUserID != "" {
		t.Errorf("revised = %+v; want the same rule ID with a new revision that replaces the text and the hint", revised)
	}

	e.clock.Advance(time.Second)
	if _, err := e.service.ReviseRule(ctx, appinstance.ReviseRuleInput{RuleID: added.ID, Text: "とても親切に", Hint: "嫌がらせ禁止"}); err != nil {
		t.Fatalf("ReviseRule(hint) = %v", err)
	}
	e.clock.Advance(time.Second)
	kept, err := e.service.ReviseRule(ctx, appinstance.ReviseRuleInput{RuleID: added.ID, Text: "常に親切に", KeepHint: true})
	if err != nil {
		t.Fatalf("ReviseRule(keep hint) = %v", err)
	}
	if kept.Revision.Text != "常に親切に" || kept.Revision.Hint != "嫌がらせ禁止" {
		t.Errorf("ReviseRule(keep hint) = %+v, want the new text with the hint kept", kept.Revision)
	}
	got, err := e.service.Rule(ctx, added.ID)
	if err != nil {
		t.Fatalf("Rule = %v", err)
	}
	if !reflect.DeepEqual(got, kept) || !got.CreatedAt.Equal(added.CreatedAt) {
		t.Errorf("Rule = %+v\nwant  %+v", got, kept)
	}
	if rules := e.rules(t, true); len(rules) != 1 || rules[0].ID != added.ID {
		t.Errorf("Rules = %+v, want the one rule", rules)
	}
}

func instanceRetiredRules(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	first := e.addRule(t, "First", "")
	second := e.addRule(t, "Second", "")
	e.clock.Advance(time.Second)
	retired, err := e.service.RetireRule(ctx, first.ID, ruleModerator)
	if err != nil {
		t.Fatalf("RetireRule = %v", err)
	}
	if !retired.Retired() || retired.Retirement.RetiredByUserID != ruleModerator || !retired.Retirement.RetiredAt.Equal(e.clock.Now()) {
		t.Errorf("retired = %+v, want retired by the moderator now", retired.Retirement)
	}
	// Retiring again changes nothing.
	e.clock.Advance(time.Second)
	again, err := e.service.RetireRule(ctx, first.ID, "")
	if err != nil || !reflect.DeepEqual(again, retired) {
		t.Errorf("RetireRule again = %+v, %v; want the first retirement %+v", again, err, retired)
	}

	if got := ruleOrder(e.rules(t, false)); !reflect.DeepEqual(got, []string{"2:Second"}) {
		t.Errorf("rules in effect = %q, want only the second", got)
	}
	if got := e.rules(t, false); len(got) != 1 || got[0].ID != second.ID {
		t.Errorf("Rules = %+v; want only the second", got)
	}
	all := e.rules(t, true)
	if len(all) != 2 || all[0].ID != second.ID || all[1].ID != first.ID || !reflect.DeepEqual(all[1], retired) {
		t.Errorf("Rules(include retired) = %+v; want the second, then the retired first", all)
	}
	// Reports still find a retired rule by its ID.
	if got, err := e.service.Rule(ctx, first.ID); err != nil || !reflect.DeepEqual(got, retired) {
		t.Errorf("Rule(retired) = %+v, %v; want %+v", got, err, retired)
	}
	if _, err := e.service.ReviseRule(ctx, appinstance.ReviseRuleInput{RuleID: first.ID, Text: "x"}); !errors.Is(err, instance.ErrRuleRetired) {
		t.Errorf("ReviseRule(retired) = %v, want ErrRuleRetired", err)
	}
	if _, err := e.service.MoveRule(ctx, appinstance.MoveRuleInput{RuleID: first.ID, Position: 1}); !errors.Is(err, instance.ErrRuleRetired) {
		t.Errorf("MoveRule(retired) = %v, want ErrRuleRetired", err)
	}
	// A new rule goes after the rules in effect.
	if third := e.addRule(t, "Third", ""); third.Revision.Position != 3 {
		t.Errorf("third position = %d, want 3", third.Revision.Position)
	}
}

func instanceMovesRules(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	a := e.addRule(t, "A", "")
	e.addRule(t, "B", "")
	c := e.addRule(t, "C", "See")
	move := func(r instance.Rule, position int, want ...string) {
		t.Helper()
		e.clock.Advance(time.Second)
		moved, err := e.service.MoveRule(ctx, appinstance.MoveRuleInput{RuleID: r.ID, Position: position, RevisedByUserID: ruleModerator})
		if err != nil {
			t.Fatalf("MoveRule(%d) = %v", position, err)
		}
		if moved.ID != r.ID {
			t.Errorf("moved rule = %s, want %s", moved.ID, r.ID)
		}
		if got := ruleOrder(e.rules(t, false)); !reflect.DeepEqual(got, want) {
			t.Errorf("after MoveRule(%d): rules = %q, want %q", position, got, want)
		}
	}
	move(c, 1, "1:C", "2:A", "3:B")
	move(c, 99, "1:A", "2:B", "3:C")
	move(a, 2, "1:B", "2:A", "3:C")
	move(a, 2, "1:B", "2:A", "3:C")
	// Moving keeps the text and the hint.
	got, err := e.service.Rule(ctx, c.ID)
	if err != nil || got.Revision.Text != "C" || got.Revision.Hint != "See" {
		t.Errorf("moved rule = %+v, %v; want its text and hint kept", got.Revision, err)
	}
	// Retiring leaves a gap that the next move closes.
	if _, err := e.service.RetireRule(ctx, a.ID, ""); err != nil {
		t.Fatalf("RetireRule = %v", err)
	}
	move(c, 1, "1:C", "2:B")
}

func instanceRejectsInvalidRules(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	for _, tc := range []struct {
		text, hint string
		field      string
		max        int
	}{
		{"", "", "text", 0},
		{"two\nlines", "", "text", 0},
		{"   ", "", "text", 0},
		{"nul\x00", "", "text", 0},
		{"\xff", "", "text", 0},
		{strings.Repeat("a", instance.MaxRuleTextLength+1), "", "text", instance.MaxRuleTextLength},
		{"x", strings.Repeat("a", instance.MaxRuleHintLength+1), "hint", instance.MaxRuleHintLength},
		{"x", "nul\x00", "hint", 0},
	} {
		_, err := e.service.AddRule(ctx, appinstance.AddRuleInput{Text: tc.text, Hint: tc.hint})
		var te *instance.RuleTextError
		if !errors.Is(err, instance.ErrInvalidValue) || !errors.As(err, &te) || te.Field != tc.field || te.Max != tc.max {
			t.Errorf("AddRule(%q, %q) = %v, want a RuleTextError{%q, %d}", tc.text, tc.hint, err, tc.field, tc.max)
		}
	}
	if rules := e.rules(t, true); len(rules) != 0 {
		t.Errorf("Rules = %+v, want nothing written", rules)
	}
	// The longest text is accepted, counted in characters, and a hint may span lines.
	r := e.addRule(t, strings.Repeat("あ", instance.MaxRuleTextLength), "two\nlines")

	const unknown = "0199f0e0-0000-7000-8000-00000000dead"
	for name, err := range map[string]error{
		"Rule": func() error { _, err := e.service.Rule(ctx, unknown); return err }(),
		"ReviseRule": func() error {
			_, err := e.service.ReviseRule(ctx, appinstance.ReviseRuleInput{RuleID: unknown, Text: "x"})
			return err
		}(),
		"MoveRule": func() error {
			_, err := e.service.MoveRule(ctx, appinstance.MoveRuleInput{RuleID: unknown, Position: 1})
			return err
		}(),
		"RetireRule": func() error { _, err := e.service.RetireRule(ctx, unknown, ""); return err }(),
	} {
		if !errors.Is(err, appinstance.ErrNotFound) {
			t.Errorf("%s(unknown) = %v, want ErrNotFound", name, err)
		}
	}
	if _, err := e.service.MoveRule(ctx, appinstance.MoveRuleInput{RuleID: r.ID, Position: 0}); !errors.Is(err, instance.ErrInvalidValue) {
		t.Errorf("MoveRule(0) = %v, want ErrInvalidValue", err)
	}
	if _, err := e.service.ReviseRule(ctx, appinstance.ReviseRuleInput{RuleID: r.ID}); !errors.Is(err, instance.ErrInvalidValue) {
		t.Errorf("ReviseRule(no text) = %v, want ErrInvalidValue", err)
	}
	if got, err := e.service.Rule(ctx, r.ID); err != nil || got.Revision.ID != r.Revision.ID {
		t.Errorf("Rule = %+v, %v; want the rule unchanged", got, err)
	}
}

func instanceBoundsRules(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	var first instance.Rule
	for i := range instance.MaxRules {
		r := e.addRule(t, fmt.Sprintf("rule %d", i), "")
		if i == 0 {
			first = r
		}
	}
	if _, err := e.service.AddRule(ctx, appinstance.AddRuleInput{Text: "one more"}); !errors.Is(err, instance.ErrTooManyRules) {
		t.Errorf("AddRule beyond %d = %v, want ErrTooManyRules", instance.MaxRules, err)
	}
	// Retired rules do not count.
	if _, err := e.service.RetireRule(ctx, first.ID, ""); err != nil {
		t.Fatalf("RetireRule = %v", err)
	}
	e.addRule(t, "one more", "")
}

func instanceRuleProjectionEqualsRebuild(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	rng := rand.New(rand.NewPCG(331, 20261017)) //nolint:gosec // deterministic test input, not a secret
	var ids []string
	for round := range 3 {
		for step := range 30 {
			e.clock.Advance(time.Second)
			text := fmt.Sprintf("rule %d.%d", round, step)
			var err error
			switch op := rng.IntN(5); {
			case op == 0 || len(ids) == 0:
				var r instance.Rule
				r, err = e.service.AddRule(ctx, appinstance.AddRuleInput{Text: text})
				ids = append(ids, r.ID)
			case op == 1:
				_, err = e.service.ReviseRule(ctx, appinstance.ReviseRuleInput{RuleID: ids[rng.IntN(len(ids))], Text: text, Hint: "hint"})
			case op == 2:
				_, err = e.service.MoveRule(ctx, appinstance.MoveRuleInput{RuleID: ids[rng.IntN(len(ids))], Position: 1 + rng.IntN(len(ids)+1)})
			case op == 3 && rng.IntN(3) == 0:
				_, err = e.service.RetireRule(ctx, ids[rng.IntN(len(ids))], "")
			default:
				_, err = e.service.ReviseRule(ctx, appinstance.ReviseRuleInput{RuleID: ids[rng.IntN(len(ids))], Text: "ルール", KeepHint: true})
			}
			if err != nil && !errors.Is(err, instance.ErrRuleRetired) {
				t.Fatalf("round %d step %d: %v", round, step, err)
			}
		}
		incremental := e.rules(t, false)
		if len(incremental) == 0 {
			t.Fatalf("round %d: no rules in effect", round)
		}
		if err := appinstance.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Instance); err != nil {
			t.Fatalf("RebuildProjections = %v", err)
		}
		if rebuilt := e.rules(t, false); !reflect.DeepEqual(incremental, rebuilt) {
			t.Fatalf("round %d: incremental rules differ from a rebuild:\nincremental %+v\nrebuilt     %+v", round, incremental, rebuilt)
		}
	}
}
