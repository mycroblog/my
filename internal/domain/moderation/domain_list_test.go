// SPDX-License-Identifier: AGPL-3.0-only

package moderation_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/domain/moderation"
)

func TestParseDomainList(t *testing.T) {
	t.Parallel()
	csv := "#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate\n" +
		"spam.example,suspend,false,false,Spam,false\nloud.example,silence,true,false,,true\nodd.example,block,,,,\n"
	rows, err := moderation.ParseDomainList(moderation.DomainListMastodonCSV, []byte(csv))
	if err != nil || len(rows) != 3 {
		t.Fatalf("ParseDomainList(csv) = %+v, %v; want 3 rows", rows, err)
	}
	if rows[1].Severity != moderation.DomainBlockSilence || !rows[1].RejectMedia || !rows[1].Obfuscate || rows[2].Severity != "block" || !rows[2].Invalid {
		t.Errorf("ParseDomainList(csv) = %+v; want silence with reject_media and obfuscate, and the unknown severity kept", rows)
	}

	rows, err = moderation.ParseDomainList(moderation.DomainListPlainText, []byte("spam.example\n\nmore.example\n"))
	if err != nil || len(rows) != 2 || rows[0].Severity != moderation.DomainBlockSuspend || rows[1].Domain != "more.example" {
		t.Errorf("ParseDomainList(plain) = %+v, %v; want two suspends", rows, err)
	}

	body := `[{"domain":"spam.example","digest":"x","severity":"suspend","comment":"Spam"},` +
		`{"domain":"hid***.example","digest":"y","severity":"silence"},{"domain":"loud.example","severity":"silence"},` +
		`{"domain":"odd.example","severity":"noop"}]`
	rows, err = moderation.ParseDomainList(moderation.DomainListMastodonJSON, []byte(body))
	if err != nil || len(rows) != 3 {
		t.Fatalf("ParseDomainList(json) = %+v, %v; want 3 rows without the obfuscated one", rows, err)
	}
	if rows[0].PublicComment != "Spam" || rows[1].Severity != moderation.DomainBlockSilence || rows[2].Severity != moderation.DomainBlockNoop {
		t.Errorf("ParseDomainList(json) = %+v", rows)
	}

	for _, c := range []struct {
		format moderation.DomainListFormat
		body   string
	}{
		{moderation.DomainListMastodonJSON, `{"error":"not found"}`},
		{moderation.DomainListMastodonCSV, "#domain\n\"broken\n"},
	} {
		if _, err := moderation.ParseDomainList(c.format, []byte(c.body)); !errors.Is(err, moderation.ErrMalformedDomainList) {
			t.Errorf("ParseDomainList(%s, %q) = %v; want ErrMalformedDomainList", c.format, c.body, err)
		}
	}
	var many strings.Builder
	for i := range moderation.MaxDomainBlockImport + 1 {
		fmt.Fprintf(&many, "d%d.example\n", i)
	}
	if _, err := moderation.ParseDomainList(moderation.DomainListPlainText, []byte(many.String())); !errors.Is(err, moderation.ErrTooManyDomainBlocks) {
		t.Errorf("ParseDomainList(%d rows) = %v; want ErrTooManyDomainBlocks", moderation.MaxDomainBlockImport+1, err)
	}
}

func TestDomainListSettingsKinds(t *testing.T) {
	t.Parallel()
	st := moderation.DefaultDomainListSettings("List")
	entry := func(sev moderation.DomainBlockSeverity, media, reports bool) moderation.DomainListEntry {
		return moderation.DomainListEntry{Domain: "x.example", Severity: sev, RejectMedia: media, RejectReports: reports}
	}
	cases := []struct {
		st   func(*moderation.DomainListSettings)
		e    moderation.DomainListEntry
		want []moderation.DomainActionKind
	}{
		{nil, entry(moderation.DomainBlockSuspend, true, true), []moderation.DomainActionKind{moderation.DomainSuspend, moderation.DomainRejectMedia, moderation.DomainRejectReports}},
		{nil, entry(moderation.DomainBlockSilence, false, false), []moderation.DomainActionKind{moderation.DomainLimit}},
		{nil, entry(moderation.DomainBlockNoop, true, false), []moderation.DomainActionKind{moderation.DomainRejectMedia}},
		{nil, entry("block", false, false), []moderation.DomainActionKind{moderation.DomainSuspend}},
		{func(s *moderation.DomainListSettings) { s.SuspendAs = moderation.DomainListAsLimit }, entry(moderation.DomainBlockSuspend, false, false), []moderation.DomainActionKind{moderation.DomainLimit}},
		{func(s *moderation.DomainListSettings) { s.SilenceAs = moderation.DomainListAsIgnore }, entry(moderation.DomainBlockSilence, false, false), nil},
		{func(s *moderation.DomainListSettings) { s.ApplyRejectMedia = false }, entry(moderation.DomainBlockNoop, true, false), nil},
	}
	for i, c := range cases {
		s := st
		if c.st != nil {
			c.st(&s)
		}
		if got := s.Kinds(c.e); !slices.Equal(got, c.want) {
			t.Errorf("case %d: Kinds(%+v) = %v, want %v", i, c.e, got, c.want)
		}
	}
}

func TestDomainListSettingsValidate(t *testing.T) {
	t.Parallel()
	if err := moderation.DefaultDomainListSettings("List").Validate(); err != nil {
		t.Errorf("Validate(default) = %v", err)
	}
	for field, change := range map[string]func(*moderation.DomainListSettings){
		"title":      func(s *moderation.DomainListSettings) { s.Title = " " },
		"mode":       func(s *moderation.DomainListSettings) { s.Mode = "auto" },
		"suspend_as": func(s *moderation.DomainListSettings) { s.SuspendAs = "silence" },
		"silence_as": func(s *moderation.DomainListSettings) { s.SilenceAs = moderation.DomainListAsSuspend },
	} {
		s := moderation.DefaultDomainListSettings("List")
		change(&s)
		var fe *moderation.DomainListFieldError
		if err := s.Validate(); !errors.Is(err, moderation.ErrInvalidDomainList) || !errors.As(err, &fe) || fe.Field != field {
			t.Errorf("Validate() with a bad %s = %v", field, err)
		}
	}
	for _, u := range []string{"http://lists.example/b.csv", "https://user@lists.example/b.csv", "lists.example/b.csv", "https:///b.csv"} {
		if err := moderation.CheckDomainListURL(u); !errors.Is(err, moderation.ErrInvalidDomainList) {
			t.Errorf("CheckDomainListURL(%q) = %v; want ErrInvalidDomainList", u, err)
		}
	}
	if err := moderation.CheckDomainListURL("https://lists.example/blocks.csv"); err != nil {
		t.Errorf("CheckDomainListURL(https) = %v", err)
	}
}

func TestDiffDomainList(t *testing.T) {
	t.Parallel()
	auto := moderation.DefaultDomainListSettings("List")
	auto.Mode = moderation.DomainListAutomatic
	entry := func(domain string, sev moderation.DomainBlockSeverity) moderation.DomainListEntry {
		return moderation.DomainListEntry{SubscriptionID: "s", Domain: domain, Severity: sev, SinceFetchID: "f1"}
	}
	own := func(id, domain string, k moderation.DomainActionKind) moderation.DomainAction {
		return moderation.DomainAction{ID: id, Domain: domain, Kind: k}
	}
	keys := func(cs []moderation.DomainListChange) []string {
		out := make([]string, len(cs))
		for i, c := range cs {
			out[i] = c.Key() + "/" + string(c.Held)
		}
		return out
	}

	t.Run("IssuesWhatNoActionCoversAndLiftsOnlyItsOwn", func(t *testing.T) {
		t.Parallel()
		got := moderation.DiffDomainList(moderation.DomainListPlan{
			Settings: auto,
			Entries: []moderation.DomainListEntry{
				entry("new.example", moderation.DomainBlockSuspend), entry("sub.human.example", moderation.DomainBlockSuspend),
				entry("kept.example", moderation.DomainBlockSuspend), entry("changed.example", moderation.DomainBlockSilence),
				entry("odd.example", "block"),
			},
			Own: []moderation.DomainAction{
				own("a1", "kept.example", moderation.DomainSuspend), own("a2", "gone.example", moderation.DomainSuspend),
				own("a3", "changed.example", moderation.DomainSuspend),
			},
			Covered: map[string][]moderation.DomainActionKind{
				"human.example": {moderation.DomainSuspend}, "kept.example": {moderation.DomainSuspend},
				"gone.example": {moderation.DomainSuspend}, "changed.example": {moderation.DomainSuspend},
			},
		})
		want := []string{
			"lift:suspend:changed.example/", "lift:suspend:gone.example/",
			"issue:limit:changed.example/", "issue:suspend:new.example/", "issue:suspend:odd.example/unknown_severity",
		}
		if !slices.Equal(keys(got), want) {
			t.Errorf("DiffDomainList = %v, want %v", keys(got), want)
		}
	})

	t.Run("HoldsEverythingInTheDraftMode", func(t *testing.T) {
		t.Parallel()
		got := moderation.DiffDomainList(moderation.DomainListPlan{
			Settings: moderation.DefaultDomainListSettings("List"),
			Entries:  []moderation.DomainListEntry{entry("new.example", moderation.DomainBlockSuspend), entry("odd.example", "block")},
			Own:      []moderation.DomainAction{own("a2", "gone.example", moderation.DomainSuspend)},
		})
		want := []string{"lift:suspend:gone.example/draft_mode", "issue:suspend:new.example/draft_mode", "issue:suspend:odd.example/unknown_severity"}
		if !slices.Equal(keys(got), want) {
			t.Errorf("DiffDomainList(draft) = %v, want %v", keys(got), want)
		}
	})

	t.Run("HoldsAMassLift", func(t *testing.T) {
		t.Parallel()
		var owns []moderation.DomainAction
		for i := range moderation.MassLiftMin + 1 {
			owns = append(owns, own(fmt.Sprint("a", i), fmt.Sprintf("d%02d.example", i), moderation.DomainSuspend))
		}
		got := moderation.DiffDomainList(moderation.DomainListPlan{Settings: auto, Own: owns})
		if len(got) != len(owns) || slices.ContainsFunc(got, func(c moderation.DomainListChange) bool { return c.Held != moderation.DomainListHoldMassLift }) {
			t.Errorf("DiffDomainList(empty list over %d actions) = %v; want every lift held", len(owns), keys(got))
		}
		// Ten lifts are not too many.
		got = moderation.DiffDomainList(moderation.DomainListPlan{Settings: auto, Own: owns[:moderation.MassLiftMin]})
		if slices.ContainsFunc(got, func(c moderation.DomainListChange) bool { return c.Held != "" }) {
			t.Errorf("DiffDomainList(%d lifts) = %v; want none held", moderation.MassLiftMin, keys(got))
		}
		// Eleven lifts out of thirty are not more than half.
		listed := make([]moderation.DomainListEntry, 0, 19)
		more := slices.Clone(owns)
		for i := range 19 {
			d := fmt.Sprintf("k%02d.example", i)
			listed = append(listed, entry(d, moderation.DomainBlockSuspend))
			more = append(more, own(fmt.Sprint("k", i), d, moderation.DomainSuspend))
		}
		got = moderation.DiffDomainList(moderation.DomainListPlan{Settings: auto, Entries: listed, Own: more})
		if len(got) != len(owns) || slices.ContainsFunc(got, func(c moderation.DomainListChange) bool { return c.Held != "" }) {
			t.Errorf("DiffDomainList(11 of 30) = %v; want 11 lifts, none held", keys(got))
		}
	})

	t.Run("LeavesOutTheRejected", func(t *testing.T) {
		t.Parallel()
		plan := moderation.DomainListPlan{
			Settings: auto,
			Entries:  []moderation.DomainListEntry{entry("new.example", moderation.DomainBlockSuspend)},
			Own:      []moderation.DomainAction{own("a2", "gone.example", moderation.DomainSuspend)},
			Rejections: []moderation.DomainListRejection{
				{Change: moderation.DomainListIssue, Domain: "new.example", Kind: moderation.DomainSuspend, EntrySinceFetchID: "f1"},
				{Change: moderation.DomainListLift, Domain: "gone.example", Kind: moderation.DomainSuspend, ActionID: "a2"},
			},
		}
		if got := moderation.DiffDomainList(plan); len(got) != 0 {
			t.Errorf("DiffDomainList(rejected) = %v; want none", keys(got))
		}
		// A changed entry and a new action come back.
		plan.Entries[0].SinceFetchID = "f2"
		plan.Own[0].ID = "a9"
		if got := moderation.DiffDomainList(plan); len(got) != 2 {
			t.Errorf("DiffDomainList(changed since the rejections) = %v; want both again", keys(got))
		}
	})
}
