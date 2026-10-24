// SPDX-License-Identifier: AGPL-3.0-only

package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	domain "github.com/mycroblog/my/internal/domain/identity"
)

// clientEventsRepository records the client events and the revocations of tokens of a client.
// The methods it does not override are not called.
type clientEventsRepository struct {
	identity.Repository
	client      domain.OAuthClient
	disablement []domain.OAuthClientDisablement
	deletions   []domain.OAuthClientDeletion
	revocations []domain.TokenRevocationReason
}

func (r *clientEventsRepository) FindOAuthClientByID(_ context.Context, id string) (domain.OAuthClient, error) {
	if id != r.client.ID {
		return domain.OAuthClient{}, identity.ErrNotFound
	}
	return r.client, nil
}

func (r *clientEventsRepository) InsertOAuthClientDisablement(_ context.Context, _ uow.Tx, d domain.OAuthClientDisablement) (bool, error) {
	r.disablement = append(r.disablement, d)
	return true, nil
}

func (r *clientEventsRepository) InsertOAuthClientDeletion(_ context.Context, _ uow.Tx, d domain.OAuthClientDeletion) (bool, error) {
	r.deletions = append(r.deletions, d)
	return true, nil
}

func (r *clientEventsRepository) RevokeTokensOfClient(_ context.Context, _ uow.Tx, clientID string, reason domain.TokenRevocationReason, _ time.Time) error {
	if clientID == r.client.ID {
		r.revocations = append(r.revocations, reason)
	}
	return nil
}

// directUnitOfWork runs a transaction without a database and keeps the events it emits.
type directUnitOfWork struct {
	uow.UnitOfWork
	tx *recordingTx
}

type recordingTx struct{ events []uow.Event }

func (t *recordingTx) Emit(_ context.Context, e uow.Event) error {
	t.events = append(t.events, e)
	return nil
}

type fixedClock time.Time

func (c fixedClock) Now() time.Time { return time.Time(c) }

type fixedIDs struct{}

func (fixedIDs) NewID() string            { return "0190a000-0000-7000-8000-000000000001" }
func (fixedIDs) NewIDAt(time.Time) string { return "0190a000-0000-7000-8000-000000000001" }

func (u directUnitOfWork) Tx(_ context.Context, fn func(tx uow.Tx) error) error { return fn(u.tx) }

func TestDisablingAndDeletingAClientRevokeItsTokensWithTheirReasons(t *testing.T) {
	clock := fixedClock(time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC))
	repo := &clientEventsRepository{client: domain.OAuthClient{
		ID: "0190a000-0000-7000-8000-0000000000aa", ClientID: "app", Registration: domain.ClientRegisteredDynamically,
	}}
	tx := &recordingTx{}
	s := identity.NewService(identity.Deps{
		UnitOfWork: directUnitOfWork{tx: tx}, Repository: repo, Clock: clock, IDs: fixedIDs{},
	})
	admin := "0190a000-0000-7000-8000-0000000000ad"
	if err := s.DisableOAuthClient(t.Context(), repo.client.ID, admin); err != nil {
		t.Fatalf("DisableOAuthClient = %v", err)
	}
	if err := s.DeleteOAuthClient(t.Context(), repo.client.ID, domain.ClientDeletedUnused, ""); err != nil {
		t.Fatalf("DeleteOAuthClient = %v", err)
	}
	want := []domain.TokenRevocationReason{domain.TokenRevokedClientDisabled, domain.TokenRevokedClientDeleted}
	if len(repo.revocations) != 2 || repo.revocations[0] != want[0] || repo.revocations[1] != want[1] {
		t.Errorf("revocation reasons = %v, want %v", repo.revocations, want)
	}
	if len(repo.disablement) != 1 || repo.disablement[0].ClientID != repo.client.ID || repo.disablement[0].DisabledByUserID != admin {
		t.Errorf("disablements = %+v, want one of the client by the admin", repo.disablement)
	}
	if len(tx.events) != 1 {
		t.Fatalf("emitted events = %v, want the audit entry of the disablement only", tx.events)
	}
	audit, ok := tx.events[0].(appmoderation.AuditRecorded)
	if !ok || audit.Entry.Action != appmoderation.AuditOAuthClientDisabled || audit.Entry.TargetKind != appmoderation.AuditTargetOAuthClient ||
		audit.Entry.TargetID != repo.client.ID || audit.Entry.ActorUserID != admin || audit.Entry.SourceEventID != repo.disablement[0].ID {
		t.Errorf("audit event = %+v, want the admin's disablement of the client", tx.events[0])
	}
	if len(repo.deletions) != 1 || repo.deletions[0].Reason != domain.ClientDeletedUnused || repo.deletions[0].DeletedByUserID != "" {
		t.Errorf("deletions = %+v, want one for an unused client by the clean-up", repo.deletions)
	}
}
