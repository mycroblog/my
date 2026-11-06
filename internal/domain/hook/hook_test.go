// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSpecsFollowTheDecision(t *testing.T) {
	media, _ := SpecOf(MediaCheck)
	if !media.Accepts(Reject) || media.Accepts(Drop) {
		t.Error("media.check takes reject and no drop")
	}
	for _, name := range Names() {
		s, ok := SpecOf(name)
		if !ok {
			t.Fatalf("no spec of %s", name)
		}
		if !s.Accepts(Allow) {
			t.Errorf("%s does not take allow", name)
		}
		if s.OnFailure[0] != PassOnFailure {
			t.Errorf("%s does not pass by default", name)
		}
	}
	local, _ := SpecOf(LocalStatus)
	if local.AcceptsFailure(RetryOnFailure) {
		t.Error("local.status can retry")
	}
	signup, _ := SpecOf(SignupCheck)
	if signup.AcceptsFailure(RetryOnFailure) || !signup.AcceptsFailure(ApproveOnFailure) {
		t.Error("signup.check chooses between pass and require_approval")
	}
}

func TestLeavesState(t *testing.T) {
	for _, o := range []Outcome{MarkSensitive, Flag, RequireApproval} {
		if !o.LeavesState() {
			t.Errorf("%s leaves no state", o)
		}
	}
	for _, o := range []Outcome{Allow, Drop, Reject, FailedTimeout, FailedBreakerOpen} {
		if o.LeavesState() {
			t.Errorf("%s leaves state", o)
		}
	}
}

func TestCleanNote(t *testing.T) {
	long := strings.Repeat("あ", MaxNote+10)
	if got := CleanNote(long); utf8.RuneCountInString(got) != MaxNote {
		t.Errorf("CleanNote kept %d characters", utf8.RuneCountInString(got))
	}
	if got := CleanNote(" a\nb\x00\x07c\xff "); got != "a bc" {
		t.Errorf("CleanNote = %q", got)
	}
}

func TestDecisionValidate(t *testing.T) {
	at := time.Unix(1, 0)
	good := Decision{ID: "i", Hook: InboundStatus, SubjectKey: "k", Mode: ModeObserve, Outcome: Drop, DecidedAt: at}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := map[string]func(*Decision){
		"no id":         func(d *Decision) { d.ID = "" },
		"no key":        func(d *Decision) { d.SubjectKey = "" },
		"unknown hook":  func(d *Decision) { d.Hook = "x" },
		"off":           func(d *Decision) { d.Mode = ModeOff },
		"wrong verdict": func(d *Decision) { d.Outcome = Reject },
		"reason":        func(d *Decision) { d.Reason = "x" },
		"note":          func(d *Decision) { d.Note = strings.Repeat("a", MaxNote+1) },
		"latency":       func(d *Decision) { d.Latency = -1 },
	}
	for name, change := range bad {
		d := good
		change(&d)
		if err := d.Validate(); !errors.Is(err, ErrInvalidDecision) {
			t.Errorf("%s: %v", name, err)
		}
	}
	good.Outcome = FailedTimeout
	if err := good.Validate(); err != nil {
		t.Errorf("a failure is refused: %v", err)
	}
}

func TestEffect(t *testing.T) {
	over := &Override{DecisionID: "i", At: time.Unix(1, 0)}
	cases := []struct {
		mode      Mode
		outcome   Outcome
		override  *Override
		onFailure OnFailure
		want      Outcome
	}{
		{ModeObserve, Drop, nil, PassOnFailure, Allow},
		{ModeEnforce, Drop, nil, PassOnFailure, Drop},
		{ModeEnforce, Drop, over, PassOnFailure, Allow},
		{ModeEnforce, FailedTimeout, nil, PassOnFailure, Allow},
		{ModeEnforce, FailedError, nil, ApproveOnFailure, RequireApproval},
		{ModeObserve, FailedError, nil, ApproveOnFailure, Allow},
	}
	for _, c := range cases {
		r := Record{Decision: Decision{Mode: c.mode, Outcome: c.outcome}, Override: c.override}
		if got := r.Effect(c.onFailure); got != c.want {
			t.Errorf("%s %s override=%v: %s, want %s", c.mode, c.outcome, c.override != nil, got, c.want)
		}
	}
}

func TestOverridable(t *testing.T) {
	enforce := func(h Name, o Outcome) Record {
		return Record{Decision: Decision{Hook: h, Mode: ModeEnforce, Outcome: o}}
	}
	if !enforce(InboundStatus, Drop).Overridable() || !enforce(SignupCheck, Reject).Overridable() {
		t.Error("a stopped subject cannot be overridden")
	}
	if enforce(LocalStatus, Reject).Overridable() {
		t.Error("a reject of local.status goes through its appeal")
	}
	if enforce(InboundStatus, Allow).Overridable() || enforce(InboundStatus, FailedTimeout).Overridable() {
		t.Error("nothing to cancel")
	}
	r := enforce(InboundStatus, Drop)
	r.Override = &Override{}
	if r.Overridable() {
		t.Error("cancelled twice")
	}
	r = enforce(InboundStatus, Drop)
	r.Decision.Mode = ModeObserve
	if r.Overridable() {
		t.Error("observe had no effect to cancel")
	}
}

func TestRetentionPolicyKeeps(t *testing.T) {
	now := time.Unix(1000, 0)
	p := RetentionPolicy{Held: map[string]bool{"held": true}, AppealsOpenSince: now.Add(-time.Hour)}
	cases := []struct {
		name string
		e    Expired
		want bool
	}{
		{"plain", Expired{Hook: InboundStatus, Mode: ModeEnforce, Outcome: Drop, DecidedAt: now.Add(-48 * time.Hour)}, false},
		{"held", Expired{Hook: InboundStatus, AccountID: "held", Mode: ModeObserve, Outcome: Allow}, true},
		{"projection", Expired{Hook: InboundStatus, Mode: ModeEnforce, Outcome: MarkSensitive, StatusIDs: []string{"s"}}, true},
		{"projection gone", Expired{Hook: InboundStatus, Mode: ModeEnforce, Outcome: MarkSensitive}, false},
		{"cancelled mark", Expired{Hook: InboundStatus, Mode: ModeEnforce, Outcome: MarkSensitive, StatusIDs: []string{"s"}, Overridden: true}, false},
		{"observed mark", Expired{Hook: InboundStatus, Mode: ModeObserve, Outcome: MarkSensitive, StatusIDs: []string{"s"}}, false},
		{"appeal open", Expired{Hook: LocalStatus, SubjectKey: "s", Mode: ModeEnforce, Outcome: Reject, DecidedAt: now}, true},
		{"edit reject", Expired{Hook: LocalStatus, SubjectKey: EditSubjectKey("s", "r"), Mode: ModeEnforce, Outcome: Reject, DecidedAt: now}, false},
		{"appeal closed", Expired{Hook: LocalStatus, Mode: ModeEnforce, Outcome: Reject, DecidedAt: now.Add(-2 * time.Hour)}, false},
		{"under review", Expired{Hook: LocalStatus, Mode: ModeEnforce, Outcome: Reject, DecidedAt: now.Add(-2 * time.Hour), AppealPending: true}, true},
		{"overturned", Expired{Hook: LocalStatus, Mode: ModeEnforce, Outcome: Reject, DecidedAt: now, Overridden: true}, false},
		{"signup reject", Expired{Hook: SignupCheck, Mode: ModeEnforce, Outcome: Reject, DecidedAt: now}, false},
	}
	for _, c := range cases {
		if got := p.Keeps(c.e); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCheckAppealable(t *testing.T) {
	at := time.Unix(1000, 0)
	window := time.Hour
	rec := Record{Decision: Decision{ID: "d", Hook: LocalStatus, Mode: ModeEnforce, Outcome: Reject, DecidedAt: at}}
	if err := rec.CheckAppealable(at.Add(window), window); err != nil {
		t.Errorf("at the deadline: %v", err)
	}
	if err := rec.CheckAppealable(at.Add(window+time.Millisecond), window); !errors.Is(err, ErrAppealWindowClosed) {
		t.Errorf("after the deadline: %v", err)
	}
	for name, change := range map[string]func(*Record){
		"observe":   func(r *Record) { r.Decision.Mode = ModeObserve },
		"flag":      func(r *Record) { r.Decision.Outcome = Flag },
		"signup":    func(r *Record) { r.Decision.Hook = SignupCheck },
		"cancelled": func(r *Record) { r.Override = &Override{} },
	} {
		r := rec
		change(&r)
		if err := r.CheckAppealable(at, window); !errors.Is(err, ErrNotAppealable) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestAppealValidate(t *testing.T) {
	good := Appeal{ID: "a", DecisionID: "d", AccountID: "x", StatusText: "hello", SubmittedAt: time.Unix(1, 0)}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Appeal){
		"blank":        func(a *Appeal) { a.StatusText = " \n" },
		"nul":          func(a *Appeal) { a.Comment = "a\x00" },
		"long text":    func(a *Appeal) { a.StatusText = strings.Repeat("a", MaxAppealStatusText+1) },
		"long comment": func(a *Appeal) { a.Comment = strings.Repeat("a", MaxAppealComment+1) },
	} {
		a := good
		change(&a)
		var textErr *AppealTextError
		if err := a.Validate(); !errors.As(err, &textErr) || !errors.Is(err, ErrInvalidAppeal) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
