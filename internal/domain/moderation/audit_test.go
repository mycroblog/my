// SPDX-License-Identifier: AGPL-3.0-only

package moderation_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/moderation"
)

func validAuditEntry() moderation.AuditEntry {
	return moderation.AuditEntry{
		ID: "01890a5d-ac96-774b-bcce-b302099a8057", ActorKind: moderation.AuditActorUser, ActorUserID: testID(60),
		Action: moderation.AuditDomainActionIssued, TargetKind: moderation.AuditTargetDomain, TargetID: "bad.example",
		TargetLabel: "bad.example", SourceEventID: "01890a5d-ac96-774b-bcce-b302099a8058",
		OccurredAt: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC),
	}
}

func TestAuditEntryValidate(t *testing.T) {
	if err := validAuditEntry().Validate(); err != nil {
		t.Fatalf("valid entry: %v", err)
	}
	cli := validAuditEntry()
	cli.ActorKind, cli.ActorUserID, cli.SourceEventID = moderation.AuditActorCLI, "", ""
	if err := cli.Validate(); err != nil {
		t.Errorf("CLI entry without a source: %v", err)
	}
	for name, edit := range map[string]func(*moderation.AuditEntry){
		"no ID":                func(e *moderation.AuditEntry) { e.ID = "" },
		"no target":            func(e *moderation.AuditEntry) { e.TargetID = "" },
		"no time":              func(e *moderation.AuditEntry) { e.OccurredAt = time.Time{} },
		"unknown actor":        func(e *moderation.AuditEntry) { e.ActorKind = "robot" },
		"user without user ID": func(e *moderation.AuditEntry) { e.ActorUserID = "" },
		"CLI with a user ID":   func(e *moderation.AuditEntry) { e.ActorKind = moderation.AuditActorCLI },
		"unknown action":       func(e *moderation.AuditEntry) { e.Action = "domain_burned" },
		"wrong target kind":    func(e *moderation.AuditEntry) { e.TargetKind = moderation.AuditTargetReport },
		"long label": func(e *moderation.AuditEntry) {
			e.TargetLabel = strings.Repeat("あ", moderation.MaxAuditTargetLabel+1)
		},
		"NUL":           func(e *moderation.AuditEntry) { e.TargetLabel = "a\x00b" },
		"invalid UTF-8": func(e *moderation.AuditEntry) { e.TargetID = "\xff" },
	} {
		e := validAuditEntry()
		edit(&e)
		if err := e.Validate(); !errors.Is(err, moderation.ErrInvalidAuditEntry) {
			t.Errorf("%s: Validate = %v, want ErrInvalidAuditEntry", name, err)
		}
	}
}

func TestAuditActionsHaveTargets(t *testing.T) {
	seen := map[moderation.AuditAction]bool{}
	for _, a := range moderation.AuditActions() {
		target, ok := a.Target()
		if !ok || !target.Valid() || !a.Valid() || seen[a] {
			t.Errorf("action %q: target %q, %v (seen %v)", a, target, ok, seen[a])
		}
		seen[a] = true
	}
	if _, ok := moderation.AuditAction("nothing").Target(); ok {
		t.Error("an unknown action has a target")
	}
	if len(moderation.AuditTargetKinds()) != 24 {
		t.Errorf("target kinds = %v, want the 24 of data-model §4.3", moderation.AuditTargetKinds())
	}
}

func TestAuditActor(t *testing.T) {
	if k, u := moderation.AuditActor(""); k != moderation.AuditActorCLI || u != "" {
		t.Errorf(`AuditActor("") = %q, %q; want the CLI`, k, u)
	}
	if k, u := moderation.AuditActor(testID(61)); k != moderation.AuditActorUser || u != testID(61) {
		t.Errorf(`AuditActor(testID(61)) = %q, %q; want the user`, k, u)
	}
}

func TestTruncateAuditLabel(t *testing.T) {
	long := strings.Repeat("あ", moderation.MaxAuditTargetLabel+3)
	if got := moderation.TruncateAuditLabel(long); got != strings.Repeat("あ", moderation.MaxAuditTargetLabel) {
		t.Errorf("TruncateAuditLabel kept %d characters", len([]rune(got)))
	}
	if got := moderation.TruncateAuditLabel("@alice"); got != "@alice" {
		t.Errorf("TruncateAuditLabel(@alice) = %q", got)
	}
}
