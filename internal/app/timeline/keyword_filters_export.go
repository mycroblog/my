// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// KeywordFilterExport is what a local account's data export (FR-008) holds of its keyword
// filters (ADR-0157 §8): every filter, the expired ones too, oldest first, in the terms of the
// client API's KeywordFilter. Times are RFC 3339 in UTC.
type KeywordFilterExport struct {
	AccountID  string                  `json:"account_id"`
	ExportedAt time.Time               `json:"exported_at"`
	Filters    []ExportedKeywordFilter `json:"filters"`
}

// ExportedKeywordFilter is a keyword filter as the export holds it.
type ExportedKeywordFilter struct {
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	Action   string            `json:"action"`
	Contexts []string          `json:"contexts"`
	Keywords []ExportedKeyword `json:"keywords"`
	// ExpiresAt is nil for a filter that never expires.
	ExpiresAt *time.Time `json:"expires_at"`
	Expired   bool       `json:"expired"`
	CreatedAt time.Time  `json:"created_at"`
}

// ExportedKeyword is a word of an exported keyword filter, as the owner typed it.
type ExportedKeyword struct {
	Keyword   string `json:"keyword"`
	WholeWord bool   `json:"whole_word"`
}

// ExportKeywordFilters returns the keyword filters of accountID for its data export.
func (s *Service) ExportKeywordFilters(ctx context.Context, accountID string) (KeywordFilterExport, error) {
	fs, err := s.KeywordFilters(ctx, accountID)
	if err != nil {
		return KeywordFilterExport{}, err
	}
	out := KeywordFilterExport{
		AccountID: accountID, ExportedAt: kernel.Normalize(s.d.Clock.Now()).UTC(), Filters: make([]ExportedKeywordFilter, 0, len(fs)),
	}
	for _, f := range fs {
		x := ExportedKeywordFilter{
			ID: f.ID, Title: f.Title, Action: string(f.Action), Contexts: make([]string, 0, len(f.Contexts)),
			Keywords: make([]ExportedKeyword, 0, len(f.Words)), Expired: f.Expired, CreatedAt: f.CreatedAt.UTC(),
		}
		for _, c := range f.Contexts {
			x.Contexts = append(x.Contexts, string(c))
		}
		for _, w := range f.Words {
			x.Keywords = append(x.Keywords, ExportedKeyword{Keyword: w.Word, WholeWord: w.WholeWord})
		}
		if !f.ExpiresAt.IsZero() {
			at := f.ExpiresAt.UTC()
			x.ExpiresAt = &at
		}
		out.Filters = append(out.Filters, x)
	}
	return out, nil
}

// WriteKeywordFilters writes the export of ExportKeywordFilters to w as JSON, the
// keyword_filters.json of the data export's archive.
func (s *Service) WriteKeywordFilters(ctx context.Context, accountID string, w io.Writer) error {
	x, err := s.ExportKeywordFilters(ctx, accountID)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(x)
}
