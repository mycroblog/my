// SPDX-License-Identifier: AGPL-3.0-only

package timelinetest

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// timelineKeywordFilterExport checks the keyword filters of the data export (ADR-0157 §8,
// FR-008): every filter of the owner, the expired ones too, oldest first, with its action,
// contexts, expiry, and words as typed with their whole-word flag; nothing of other accounts.
func timelineKeywordFilterExport(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	alice, bob := e.account(true), e.account(true)
	if x, err := e.svc.ExportKeywordFilters(ctx, alice); err != nil || x.Filters == nil || len(x.Filters) != 0 {
		t.Fatalf("ExportKeywordFilters(no filters) = %+v, %v; want an empty list", x, err)
	}
	spoilers := filterInput("Spoilers", timeline.FilterWarn, "Spoiler")
	spoilers.Words = append(spoilers.Words, apptimeline.KeywordInput{Word: "ネタバレ", WholeWord: true})
	first, err := e.svc.CreateKeywordFilter(ctx, alice, spoilers)
	if err != nil {
		t.Fatal(err)
	}
	e.Clock.Advance(time.Second)
	expiring := apptimeline.KeywordFilterInput{
		Title: "Election", Action: timeline.FilterHide, Contexts: []timeline.FilterContext{timeline.FilterContextNotifications},
		Words: []apptimeline.KeywordInput{{Word: "vote"}}, ExpiresAt: e.Clock.Now().Add(time.Hour),
	}
	second, err := e.svc.CreateKeywordFilter(ctx, alice, expiring)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CreateKeywordFilter(ctx, bob, filterInput("Bob's", timeline.FilterWarn, "x")); err != nil {
		t.Fatal(err)
	}
	e.Clock.Advance(2 * time.Hour) // the second filter has expired, and stays in the export

	x, err := e.svc.ExportKeywordFilters(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	expiresAt := expiring.ExpiresAt.UTC()
	want := []apptimeline.ExportedKeywordFilter{
		{
			ID: first.ID, Title: "Spoilers", Action: "warn", Contexts: []string{"home", "public"},
			Keywords:  []apptimeline.ExportedKeyword{{Keyword: "Spoiler"}, {Keyword: "ネタバレ", WholeWord: true}},
			CreatedAt: first.CreatedAt.UTC(),
		},
		{
			ID: second.ID, Title: "Election", Action: "hide", Contexts: []string{"notifications"},
			Keywords: []apptimeline.ExportedKeyword{{Keyword: "vote"}}, ExpiresAt: &expiresAt, Expired: true,
			CreatedAt: second.CreatedAt.UTC(),
		},
	}
	if x.AccountID != alice || !x.ExportedAt.Equal(e.Clock.Now()) || !reflect.DeepEqual(x.Filters, want) {
		t.Errorf("ExportKeywordFilters = %+v\nwant filters %+v", x, want)
	}

	// The archive's file is that export as JSON.
	var b bytes.Buffer
	if err := e.svc.WriteKeywordFilters(ctx, alice, &b); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Filters []struct {
			Title     string  `json:"title"`
			ExpiresAt *string `json:"expires_at"`
			Keywords  []struct {
				Keyword   string `json:"keyword"`
				WholeWord bool   `json:"whole_word"`
			} `json:"keywords"`
		} `json:"filters"`
	}
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil || len(doc.Filters) != 2 || doc.Filters[0].ExpiresAt != nil ||
		doc.Filters[1].ExpiresAt == nil || !doc.Filters[0].Keywords[1].WholeWord || doc.Filters[0].Keywords[1].Keyword != "ネタバレ" {
		t.Errorf("keyword_filters.json = %s, %v", b.Bytes(), err)
	}
}
