// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/accountgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// FindActorParts implements account.Repository: one query (ListActorsByIDs) reads every part of
// the actors of ids, each as the read of one account reads it.
func (r *Repository) FindActorParts(ctx context.Context, ids []string) (appaccount.ActorParts, error) {
	return actorParts(ctx, r.db.reader(), ids)
}

// FindActorPartsTx implements the reads in a transaction.
func (r *Repository) FindActorPartsTx(ctx context.Context, tx uow.Tx, ids []string) (appaccount.ActorParts, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return appaccount.ActorParts{}, err
	}
	return actorParts(ctx, db.reader(), ids)
}

func actorParts(ctx context.Context, q *accountgen.Queries, ids []string) (appaccount.ActorParts, error) {
	out := appaccount.ActorParts{
		Accounts: map[string]account.Account{}, Locals: map[string]account.LocalAccount{}, Handles: map[string]account.Handle{},
		Profiles: map[string]account.ProfileRevision{}, Keys: map[string][]account.Key{}, Lifecycle: map[string]account.LifecycleState{},
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListActorsByIDs(ctx, ids)
	if err != nil {
		return out, postgres.Wrap(err, "list actors")
	}
	for _, row := range rows {
		if err := addActorRow(out, row); err != nil {
			return out, err
		}
	}
	return out, nil
}

// addActorRow puts the parts of an actor of ListActorsByIDs into out.
func addActorRow(out appaccount.ActorParts, row accountgen.ListActorsByIDsRow) error {
	id := row.ID
	out.Accounts[id] = accountFrom(accountgen.Account{ID: id, Uri: row.Uri, Domain: row.Domain, ActorType: row.ActorType, CreatedAt: row.CreatedAt})
	if row.LocalUserID != nil {
		out.Locals[id] = account.LocalAccount{
			AccountID: id, UserID: *row.LocalUserID, Username: *row.LocalUsername, UsernameNormalized: *row.LocalUsernameNormalized,
		}
	}
	if row.HandleUsername != nil {
		out.Handles[id] = account.Handle{
			AccountID: id, Username: *row.HandleUsername, UsernameNormalized: *row.HandleUsernameNormalized, Domain: *row.HandleDomain,
		}
	}
	if row.RevisionID != nil {
		rev, err := profileFrom(accountgen.GetCurrentProfileRow{
			ID: *row.RevisionID, AccountID: id, Seq: *row.Seq, DisplayName: *row.DisplayName, NoteHtml: *row.NoteHtml,
			Locked: *row.Locked, Bot: *row.Bot, Discoverable: *row.Discoverable, Indexable: *row.Indexable, AiOptOut: *row.AiOptOut,
			AvatarMediaID: row.AvatarMediaID, HeaderMediaID: row.HeaderMediaID, RevisedAt: *row.RevisedAt,
			Feed: *row.Feed, FollowsVisibility: *row.FollowsVisibility, NoteSource: row.NoteSource, Fields: row.Fields, EmojiIds: row.EmojiIds,
		})
		if err != nil {
			return err
		}
		out.Profiles[id] = rev
	}
	var keys []keyJSON
	if err := json.Unmarshal([]byte(row.Keys), &keys); err != nil {
		return fmt.Errorf("decode the keys of %s: %w", id, err)
	}
	for _, k := range keys {
		fingerprint, err := hex.DecodeString(k.Fingerprint)
		if err != nil {
			return fmt.Errorf("decode the fingerprint of key %s: %w", k.ID, err)
		}
		out.Keys[id] = append(out.Keys[id], keyFrom(accountgen.AccountKey{
			ID: k.ID, AccountID: id, KeyID: k.KeyID, Algorithm: k.Algorithm, PublicKeyPem: k.PublicKeyPEM, Fingerprint: fingerprint,
			CreatedAt: time.UnixMicro(k.CreatedAt),
		}))
	}
	if row.State != nil {
		out.Lifecycle[id] = lifecycleState(accountgen.AccountLifecycleState{
			AccountID: id, State: *row.State, Since: *row.Since, DeletionDueAt: row.DeletionDueAt,
		})
	}
	return nil
}

// keyJSON is a key of the keys of ListActorsByIDs, created at CreatedAt microseconds since the
// epoch.
type keyJSON struct {
	ID           string `json:"id"`
	KeyID        string `json:"key_id"`
	Algorithm    string `json:"algorithm"`
	PublicKeyPEM string `json:"public_key_pem"`
	Fingerprint  string `json:"fingerprint"`
	CreatedAt    int64  `json:"created_at"`
}

// FindAccounts implements account.Repository.
func (r *Repository) FindAccounts(ctx context.Context, ids []string) (map[string]account.Account, error) {
	out := map[string]account.Account{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListAccountsByIDs(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list accounts")
	}
	for _, row := range rows {
		out[row.ID] = accountFrom(row)
	}
	return out, nil
}

// FindLocalAccountsByUsers implements account.Repository.
func (r *Repository) FindLocalAccountsByUsers(ctx context.Context, userIDs []string) (map[string]account.LocalAccount, error) {
	out := map[string]account.LocalAccount{}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListLocalAccountsByUsers(ctx, userIDs)
	if err != nil {
		return nil, postgres.Wrap(err, "list local accounts of users")
	}
	for _, row := range rows {
		out[row.UserID] = account.LocalAccount(row)
	}
	return out, nil
}

// FindMoves implements account.Repository.
func (r *Repository) FindMoves(ctx context.Context, ids []string) (map[string]account.Move, error) {
	out := map[string]account.Move{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListAccountMovesByIDs(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list account moves")
	}
	for _, row := range rows {
		out[row.ID] = moveFrom(row)
	}
	return out, nil
}

// FindLifecycleStates implements account.Repository.
func (r *Repository) FindLifecycleStates(ctx context.Context, ids []string) (map[string]account.LifecycleState, error) {
	out := map[string]account.LifecycleState{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListLifecycleStatesByIDs(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list lifecycle states")
	}
	for _, row := range rows {
		out[row.AccountID] = lifecycleState(row)
	}
	return out, nil
}

// FindAccountsByURIs implements account.Repository.
func (r *Repository) FindAccountsByURIs(ctx context.Context, uris []string) (map[string]account.Account, error) {
	out := map[string]account.Account{}
	if len(uris) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListAccountsByURIs(ctx, uris)
	if err != nil {
		return nil, postgres.Wrap(err, "list accounts by URI")
	}
	for _, row := range rows {
		out[row.Uri] = accountFrom(row)
	}
	return out, nil
}

// FindHandles implements account.Repository.
func (r *Repository) FindHandles(ctx context.Context, names []appaccount.HandleName) (map[appaccount.HandleName]account.Handle, error) {
	out := map[appaccount.HandleName]account.Handle{}
	if len(names) == 0 {
		return out, nil
	}
	var users, domains []string
	for _, n := range names {
		users, domains = append(users, n.UsernameNormalized), append(domains, n.Domain)
	}
	rows, err := r.db.reader().ListAccountHandlesByNames(ctx, accountgen.ListAccountHandlesByNamesParams{Usernames: users, Domains: domains})
	if err != nil {
		return nil, postgres.Wrap(err, "list account handles")
	}
	for _, row := range rows {
		out[appaccount.HandleName{UsernameNormalized: row.UsernameNormalized, Domain: row.Domain}] = account.Handle(row)
	}
	return out, nil
}
