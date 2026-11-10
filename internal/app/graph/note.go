// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// SetNote writes accountID's private note on targetID (FR-310), replacing the note it had; an
// empty text deletes the note. Only the account reads the note (Relationship.Note), nothing
// federates, and the note keeps no history. A text that graph.ValidateNoteText refuses fails with
// graph.ErrInvalidNote and a note on the account itself with graph.ErrSelfNote. The account must be
// local and active; the target must exist, may be remote, and must not be deleted.
func (s *Service) SetNote(ctx context.Context, accountID, targetID, text string) (Relationship, error) {
	if text == "" {
		return s.DeleteNote(ctx, accountID, targetID)
	}
	n := graph.Note{AccountID: accountID, TargetAccountID: targetID, Text: text, UpdatedAt: kernel.Normalize(s.d.Clock.Now())}
	if err := n.Validate(); err != nil {
		return Relationship{}, err
	}
	if _, err := s.localActive(ctx, accountID); err != nil {
		return Relationship{}, err
	}
	target, err := s.existing(ctx, targetID)
	if err != nil {
		return Relationship{}, err
	}
	if target.Deleted() {
		// The erasure of the deleted account may have run already; a note written now would stay.
		return Relationship{}, fmt.Errorf("account %s: %w", targetID, ErrAccountUnavailable)
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.UpsertNote(ctx, tx, n)
	})
	if err != nil {
		return Relationship{}, err
	}
	return s.Relationship(ctx, accountID, targetID)
}

// DeleteNote deletes accountID's private note on targetID. Without a note it changes nothing.
func (s *Service) DeleteNote(ctx context.Context, accountID, targetID string) (Relationship, error) {
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := s.d.Repository.DeleteNote(ctx, tx, accountID, targetID)
		return err
	})
	if err != nil {
		return Relationship{}, err
	}
	return s.Relationship(ctx, accountID, targetID)
}

// noteOf returns the text of accountID's note on targetID, "" without one.
func (s *Service) noteOf(ctx context.Context, accountID, targetID string) (string, error) {
	n, err := s.d.Repository.FindNote(ctx, accountID, targetID)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	return n.Text, err
}

// NoteExport is what a local account's data export (FR-008) holds of its private notes: every
// note, oldest first by its last change. Times are RFC 3339 in UTC.
type NoteExport struct {
	AccountID  string         `json:"account_id"`
	ExportedAt time.Time      `json:"exported_at"`
	Notes      []ExportedNote `json:"notes"`
}

// ExportedNote is a note as the export holds it: the account it is on, by ID and by its actor
// URI, which stays meaningful outside this server.
type ExportedNote struct {
	AccountID  string    `json:"account_id"`
	AccountURI string    `json:"account_uri"`
	Note       string    `json:"note"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ExportNotes returns the private notes of accountID for its data export. A note on an account
// that is gone (the erasure removes such notes) is left out.
func (s *Service) ExportNotes(ctx context.Context, accountID string) (NoteExport, error) {
	notes, err := s.d.Repository.ListNotesBy(ctx, accountID)
	if err != nil {
		return NoteExport{}, err
	}
	out := NoteExport{AccountID: accountID, ExportedAt: kernel.Normalize(s.d.Clock.Now()).UTC(), Notes: make([]ExportedNote, 0, len(notes))}
	for _, n := range notes {
		a, err := s.d.Accounts.FindByID(ctx, n.TargetAccountID)
		if errors.Is(err, appaccount.ErrNotFound) {
			continue
		}
		if err != nil {
			return NoteExport{}, err
		}
		out.Notes = append(out.Notes, ExportedNote{
			AccountID: n.TargetAccountID, AccountURI: a.Account.URI, Note: n.Text, UpdatedAt: n.UpdatedAt.UTC(),
		})
	}
	return out, nil
}

// WriteNotes writes the export of ExportNotes to w as JSON, the account_notes.json of the data
// export's archive.
func (s *Service) WriteNotes(ctx context.Context, accountID string, w io.Writer) error {
	x, err := s.ExportNotes(ctx, accountID)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(x)
}
