// SPDX-License-Identifier: AGPL-3.0-only

package moderation_test

import (
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/moderation"
)

func TestCategoryOfHookReason(t *testing.T) {
	t.Parallel()
	for reason, want := range map[string]moderation.Category{
		"spam": moderation.CategorySpam, "malware": moderation.CategorySpam, "illegal": moderation.CategoryLegal,
		"harassment": moderation.CategoryViolation, "sexual": moderation.CategoryViolation, "violence": moderation.CategoryViolation,
		"other": moderation.CategoryOther, "": moderation.CategoryOther, "unknown": moderation.CategoryOther,
	} {
		if got := moderation.CategoryOfHookReason(reason); got != want {
			t.Errorf("CategoryOfHookReason(%q) = %q, want %q", reason, got, want)
		}
		if !want.Valid() {
			t.Errorf("category %q is not valid", want)
		}
	}
}

func TestAReportOfAHookIsLocalAndNotForwarded(t *testing.T) {
	t.Parallel()
	valid := moderation.Report{
		ID: "r", ReporterAccountID: "a", TargetAccountID: "b", Category: moderation.CategorySpam, FiledAt: time.Unix(1, 0),
		HookOrigin: &moderation.ReportHookOrigin{DecisionID: "d", Hook: "inbound.status"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate = %v", err)
	}
	for name, edit := range map[string]func(r *moderation.Report){
		"a URI":        func(r *moderation.Report) { r.URI = "https://remote.example/flag" },
		"forwarded":    func(r *moderation.Report) { r.Forwarded = true },
		"no decision":  func(r *moderation.Report) { r.HookOrigin = &moderation.ReportHookOrigin{Hook: "inbound.status"} },
		"no hook":      func(r *moderation.Report) { r.HookOrigin = &moderation.ReportHookOrigin{DecisionID: "d"} },
		"a voice room": func(r *moderation.Report) { r.VoiceRoom = &moderation.ReportVoiceRoom{RoomID: "x", RevisionID: "y"} },
	} {
		r := valid
		edit(&r)
		if err := r.Validate(); err == nil {
			t.Errorf("%s: Validate took it", name)
		}
	}
}
