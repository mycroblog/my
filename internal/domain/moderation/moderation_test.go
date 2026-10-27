// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMergeKeepsTheLatestEndOfEachRestriction(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { t := now.Add(d); return &t }
	rows := []RestrictionRow{
		{Restriction: RestrictFederation, SourceActionID: testID(1), EffectiveUntil: at(time.Hour)},
		{Restriction: RestrictFederation, SourceActionID: testID(2), EffectiveUntil: at(2 * time.Hour)},
		{Restriction: RestrictProfileView, SourceActionID: testID(1), EffectiveUntil: at(time.Hour)},
		{Restriction: RestrictProfileView, SourceActionID: testID(3)},
		{Restriction: RestrictProfileView, SourceActionID: testID(4), EffectiveUntil: at(3 * time.Hour)},
		{Restriction: RestrictMediaFetch, SourceActionID: testID(5), EffectiveUntil: at(0)},
	}
	got := Merge(rows, now)
	want := []Effect{
		{Restriction: RestrictFederation, Until: at(2 * time.Hour)},
		{Restriction: RestrictProfileView},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Merge = %+v, want %+v", got, want)
	}
	if got := Merge(nil, now); got == nil || len(got) != 0 {
		t.Errorf("Merge(nil) = %#v, want an empty slice", got)
	}
}

func TestDomainAndParents(t *testing.T) {
	if got, want := DomainAndParents("a.b.example"), []string{"a.b.example", "b.example", "example"}; !reflect.DeepEqual(got, want) {
		t.Errorf("DomainAndParents = %v, want %v", got, want)
	}
	if got := DomainAndParents(""); got != nil {
		t.Errorf("DomainAndParents(\"\") = %v, want none", got)
	}
}

func TestActionsAndTheirRestrictions(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	ok := Action{ID: testID(1), TargetAccountID: testID(10), Kind: ActionSuspend, ReasonCategory: CategorySpam, Issuer: IssuerModerator, IssuedAt: now, ExpiresAt: &later}
	if err := ok.Validate(); err != nil {
		t.Fatalf("Validate = %v", err)
	}
	if rows := ok.Rows(); len(rows) != 5 || rows[0].Target != testID(10) || rows[0].SourceActionID != testID(1) || rows[0].EffectiveUntil != &later {
		t.Errorf("Rows = %+v, want the 5 restrictions of a suspension", rows)
	}
	ban := ok
	ban.Kind = ActionBan
	early := now
	past := ok
	past.ExpiresAt = &early
	tooLate := now.Add(MaxDuration + time.Second)
	tooLong := ok
	tooLong.ExpiresAt = &tooLate
	for name, a := range map[string]Action{"expiring ban": ban, "expiry at issue": past, "expiry too late": tooLong} {
		if err := a.Validate(); !errors.Is(err, ErrInvalidAction) {
			t.Errorf("Validate(%s) = %v, want ErrInvalidAction", name, err)
		}
	}
	if err := ban.Validate(); !errors.Is(err, ErrActionNeverExpires) {
		t.Errorf("Validate(expiring ban) = %v, want ErrActionNeverExpires", err)
	}
	if ActionBan.Liftable() || !ActionSuspend.Liftable() || !ActionDeleteStatuses.Liftable() {
		t.Error("only a ban should not be liftable")
	}
	d := DomainAction{ID: testID(20), Domain: "bad.example", Kind: DomainSuspend, IssuedAt: now}
	if err := d.Validate(); err != nil {
		t.Fatalf("Validate = %v", err)
	}
	if got := d.Rows(); len(got) != 2 || got[0].Restriction != RestrictProfileView || got[1].Restriction != RestrictFederation {
		t.Errorf("Rows = %+v, want profile_view and federation", got)
	}
	for _, domain := range []string{"", "Bad.example", "bad.example:443", "bad..example", "https://bad.example", "bad.example/"} {
		if ValidDomain(domain) {
			t.Errorf("ValidDomain(%q) = true", domain)
		}
	}
}

func TestActionsOnStatuses(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	base := Action{ID: testID(1), TargetAccountID: testID(10), ReasonCategory: CategorySpam, Issuer: IssuerModerator, IssuedAt: now}
	with := func(kind ActionKind, ids []string, expires *time.Time) Action {
		a := base
		a.Kind, a.StatusIDs, a.ExpiresAt = kind, ids, expires
		return a
	}
	tooMany := make([]string, MaxActionStatuses+1)
	for i := range tooMany {
		tooMany[i] = testID(i + 1)
	}
	for _, kind := range []ActionKind{ActionDeleteStatuses, ActionMarkStatusesSensitive} {
		if !kind.OnStatuses() || len(kind.Restrictions()) != 0 {
			t.Errorf("%s: OnStatuses = %v, restrictions %v", kind, kind.OnStatuses(), kind.Restrictions())
		}
		if err := with(kind, []string{testID(41), testID(42)}, nil).Validate(); err != nil {
			t.Errorf("Validate(%s on 2 statuses) = %v", kind, err)
		}
		for name, c := range map[string]struct {
			a    Action
			want error
		}{
			"no statuses":    {with(kind, nil, nil), ErrInvalidActionStatuses},
			"repeated":       {with(kind, []string{testID(41), testID(41)}, nil), ErrInvalidActionStatuses},
			"empty":          {with(kind, []string{""}, nil), ErrInvalidActionStatuses},
			"too many":       {with(kind, tooMany, nil), ErrInvalidActionStatuses},
			"with an expiry": {with(kind, []string{testID(41)}, &later), ErrActionNeverExpires},
		} {
			if err := c.a.Validate(); !errors.Is(err, c.want) || !errors.Is(err, ErrInvalidAction) {
				t.Errorf("Validate(%s, %s) = %v, want %v", kind, name, err, c.want)
			}
		}
	}
	if err := with(ActionSuspend, []string{testID(41)}, nil).Validate(); !errors.Is(err, ErrInvalidActionStatuses) {
		t.Errorf("Validate(suspend naming a status) = %v, want ErrInvalidActionStatuses", err)
	}
	if ActionSuspend.OnStatuses() || ActionBan.OnStatuses() {
		t.Error("suspend and ban act on accounts, not statuses")
	}
}

func TestTruncateComment(t *testing.T) {
	long := strings.Repeat("é", MaxReportComment+1)
	if got := TruncateComment(long); got != strings.Repeat("é", MaxReportComment) {
		t.Errorf("TruncateComment kept %d bytes", len(got))
	}
	if got := TruncateComment("short"); got != "short" {
		t.Errorf("TruncateComment(short) = %q", got)
	}
}

func TestReportValidate(t *testing.T) {
	ok := Report{ID: testID(30), ReporterAccountID: testID(11), TargetAccountID: testID(12), Category: CategorySpam, Comment: "spam", FiledAt: time.Unix(1, 0)}
	if err := ok.Validate(); err != nil {
		t.Fatalf("Validate = %v", err)
	}
	for _, comment := range []string{"a\x00b", "\xff", strings.Repeat("a", MaxReportComment+1)} {
		r := ok
		r.Comment = comment
		if err := r.Validate(); !errors.Is(err, ErrInvalidReport) {
			t.Errorf("Validate(comment %q...) = %v, want ErrInvalidReport", comment[:1], err)
		}
	}
}

func TestReportCitesRulesOnlyForAViolation(t *testing.T) {
	r := Report{ID: testID(30), ReporterAccountID: testID(11), TargetAccountID: testID(12), Category: CategoryViolation, RuleIDs: []string{testID(50)}, FiledAt: time.Unix(1, 0)}
	if err := r.ValidateLocal(); err != nil {
		t.Fatalf("ValidateLocal(violation with a rule) = %v", err)
	}
	for _, c := range []Category{CategorySpam, CategoryLegal, CategoryOther} {
		r.Category = c
		if err := r.Validate(); !errors.Is(err, ErrInvalidReport) {
			t.Errorf("Validate(%s with a rule) = %v, want ErrInvalidReport", c, err)
		}
	}
}

func TestReportValidateLocal(t *testing.T) {
	ok := Report{ID: testID(30), ReporterAccountID: testID(11), TargetAccountID: testID(12), Category: CategoryOther, FiledAt: time.Unix(1, 0)}
	if err := ok.ValidateLocal(); err != nil {
		t.Fatalf("ValidateLocal = %v", err)
	}
	long := ok
	long.Comment = strings.Repeat("あ", MaxLocalReportComment+1)
	var textErr *TextError
	if err := long.ValidateLocal(); !errors.Is(err, ErrInvalidReport) || !errors.As(err, &textErr) || textErr.Max != MaxLocalReportComment {
		t.Errorf("ValidateLocal(long comment) = %v, want a *TextError of %d", err, MaxLocalReportComment)
	}
	long.Comment = strings.Repeat("あ", MaxLocalReportComment)
	if err := long.ValidateLocal(); err != nil {
		t.Errorf("ValidateLocal(comment of %d) = %v", MaxLocalReportComment, err)
	}
	many := ok
	many.StatusIDs = make([]string, MaxLocalReportStatuses+1)
	var countErr *CountError
	if err := many.ValidateLocal(); !errors.As(err, &countErr) || countErr.Field != "status_ids" {
		t.Errorf("ValidateLocal(too many statuses) = %v, want a *CountError of status_ids", err)
	}
	withURI := ok
	withURI.URI = "https://remote.example/flag"
	if err := withURI.ValidateLocal(); !errors.Is(err, ErrInvalidReport) {
		t.Errorf("ValidateLocal(with a URI) = %v, want ErrInvalidReport", err)
	}
}

func TestTextsOfActionsAndLifts(t *testing.T) {
	now := time.Unix(1, 0)
	action := Action{ID: testID(1), TargetAccountID: testID(12), Kind: ActionLimit, ReasonCategory: CategorySpam, Issuer: IssuerModerator, IssuedAt: now}
	domain := DomainAction{ID: testID(20), Domain: "bad.example", Kind: DomainSuspend, IssuedAt: now}
	lift := Lift{ActionID: testID(1), LiftedAt: now}
	long := strings.Repeat("あ", MaxActionText+1)
	for _, tc := range []struct {
		field    string
		validate func(text string) error
	}{
		{"public_reason", func(s string) error { a := action; a.PublicReason = s; return a.Validate() }},
		{"private_note", func(s string) error { a := action; a.PrivateNote = s; return a.Validate() }},
		{"public_comment", func(s string) error { a := domain; a.PublicComment = s; return a.Validate() }},
		{"private_note", func(s string) error { a := domain; a.PrivateNote = s; return a.Validate() }},
		{"reason", func(s string) error { l := lift; l.Reason = s; return l.Validate() }},
	} {
		if err := tc.validate(long[len("あ"):]); err != nil {
			t.Errorf("%s of %d characters: %v", tc.field, MaxActionText, err)
		}
		for _, bad := range []struct {
			text string
			max  int
		}{{"a\x00b", 0}, {"\xff", 0}, {long, MaxActionText}} {
			err := tc.validate(bad.text)
			var te *TextError
			if !errors.Is(err, ErrInvalidAction) || !errors.As(err, &te) || te.Field != tc.field || te.Max != bad.max {
				t.Errorf("%s %q...: %v, want ErrInvalidAction and TextError{%s, %d}", tc.field, bad.text[:1], err, tc.field, bad.max)
			}
		}
	}
	if err := (Lift{LiftedAt: now}).Validate(); !errors.Is(err, ErrInvalidAction) {
		t.Errorf("a lift without its action: %v", err)
	}
	if ValidDomain("bad\x00.example") || ValidDomain("bad\xff.example") {
		t.Error("ValidDomain accepts NUL or invalid UTF-8")
	}
}

func TestDomainActionState(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	soon, past := now.Add(time.Hour), now.Add(-time.Hour)
	for _, tc := range []struct {
		r    DomainActionRecord
		want ActionState
	}{
		{DomainActionRecord{}, ActionActive},
		{DomainActionRecord{Action: DomainAction{ExpiresAt: &soon}}, ActionActive},
		{DomainActionRecord{Action: DomainAction{ExpiresAt: &now}}, ActionExpired},
		{DomainActionRecord{Action: DomainAction{ExpiresAt: &past}, Lift: &DomainLift{}}, ActionLifted},
		{DomainActionRecord{Lift: &DomainLift{}}, ActionLifted},
	} {
		if got := tc.r.State(now); got != tc.want {
			t.Errorf("State(%+v) = %s, want %s", tc.r, got, tc.want)
		}
	}
	if !OutcomeActioned.Valid() || !OutcomeDismissed.Valid() || ReportOutcome("ignored").Valid() {
		t.Error("ReportOutcome.Valid accepts the wrong outcomes")
	}
}

func TestDurationsAreBounded(t *testing.T) {
	for _, tc := range []struct {
		seconds int64
		want    bool
	}{{0, false}, {1, true}, {MaxDurationSeconds, true}, {MaxDurationSeconds + 1, false}, {1 << 62, false}, {-1, false}} {
		if got := ValidDurationSeconds(tc.seconds); got != tc.want {
			t.Errorf("ValidDurationSeconds(%d) = %v, want %v", tc.seconds, got, tc.want)
		}
	}
	for _, tc := range []struct {
		d    time.Duration
		want bool
	}{{0, true}, {MaxDuration, true}, {MaxDuration + 1, false}, {-time.Second, false}} {
		if got := ValidDuration(tc.d); got != tc.want {
			t.Errorf("ValidDuration(%s) = %v, want %v", tc.d, got, tc.want)
		}
	}
}
