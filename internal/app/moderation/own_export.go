// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// OwnRecordExport is what a local account's data export (FR-008) holds of its moderation
// records (ADR-0058): the actions on it as it sees them, with its appeals and their decisions,
// and the reports it filed with whether they are open. It has no private note, issuer, report
// about the account, or reporter. Times are RFC 3339 in UTC.
type OwnRecordExport struct {
	AccountID  string    `json:"account_id"`
	ExportedAt time.Time `json:"exported_at"`
	// Actions are the actions on the account, newest first.
	Actions []OwnExportedAction `json:"actions"`
	// ReportsFiled are the reports that the account filed, newest first.
	ReportsFiled []ExportedFiledReport `json:"reports_filed"`
}

// OwnExportedAction is an action on the account as the account sees it.
type OwnExportedAction struct {
	ID             string             `json:"id"`
	Kind           string             `json:"kind"`
	ReasonCategory string             `json:"reason_category"`
	PublicReason   string             `json:"public_reason"`
	StatusIDs      []string           `json:"status_ids"`
	IssuedAt       time.Time          `json:"issued_at"`
	ExpiresAt      *time.Time         `json:"expires_at"`
	LiftedAt       *time.Time         `json:"lifted_at"`
	Appeal         *OwnExportedAppeal `json:"appeal"`
}

// OwnExportedAppeal is the account's appeal of an action with the decision on it.
type OwnExportedAppeal struct {
	ID             string     `json:"id"`
	Text           string     `json:"text"`
	State          string     `json:"state"`
	SubmittedAt    time.Time  `json:"submitted_at"`
	DecidedAt      *time.Time `json:"decided_at"`
	DecisionReason string     `json:"decision_reason"`
}

// ExportOwnRecords returns the moderation records of a local account that its data export holds.
func (s *Service) ExportOwnRecords(ctx context.Context, accountID string) (OwnRecordExport, error) {
	out := OwnRecordExport{
		AccountID: accountID, ExportedAt: kernel.Normalize(s.d.Clock.Now()).UTC(), Actions: []OwnExportedAction{},
	}
	before := ""
	for {
		page, err := s.OwnActions(ctx, accountID, OwnActionFilter{Before: before}, exportPage)
		if err != nil {
			return OwnRecordExport{}, err
		}
		for _, v := range page {
			out.Actions = append(out.Actions, ownExportedAction(v))
		}
		if len(page) < exportPage {
			break
		}
		before = page[len(page)-1].ID
	}
	filed, err := s.exportReportsFiled(ctx, accountID)
	if err != nil {
		return OwnRecordExport{}, err
	}
	out.ReportsFiled = filed
	return out, nil
}

// WriteOwnRecords writes the export of ExportOwnRecords to w as JSON.
func (s *Service) WriteOwnRecords(ctx context.Context, accountID string, w io.Writer) error {
	x, err := s.ExportOwnRecords(ctx, accountID)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(x)
}

func ownExportedAction(v TargetActionView) OwnExportedAction {
	a := OwnExportedAction{
		ID: v.ID, Kind: string(v.Kind), ReasonCategory: string(v.ReasonCategory), PublicReason: v.PublicReason,
		StatusIDs: nonNil(v.StatusIDs), IssuedAt: v.IssuedAt.UTC(), ExpiresAt: utcPtr(v.ExpiresAt), LiftedAt: utcPtr(v.LiftedAt),
	}
	if ap := v.Appeal; ap != nil {
		a.Appeal = &OwnExportedAppeal{
			ID: ap.ID, Text: ap.Text, State: string(ap.State), SubmittedAt: ap.SubmittedAt.UTC(),
			DecidedAt: utcPtr(ap.DecidedAt), DecisionReason: ap.DecisionReason,
		}
	}
	return a
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
