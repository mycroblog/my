// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/accountgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/account"
)

// FindActorParts implements account.Repository: one query per part, as the reads of one account
// build it (currentProfile, keyFrom, lifecycleState).
func (r *Repository) FindActorParts(ctx context.Context, ids []string) (appaccount.ActorParts, error) {
	out := appaccount.ActorParts{
		Accounts: map[string]account.Account{}, Locals: map[string]account.LocalAccount{}, Handles: map[string]account.Handle{},
		Profiles: map[string]account.ProfileRevision{}, Keys: map[string][]account.Key{}, Lifecycle: map[string]account.LifecycleState{},
	}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return out, err
	}
	q := r.db.reader()
	accounts, err := q.ListAccountsByIDs(ctx, list)
	if err != nil {
		return out, sqlite.Wrap(err, "list accounts")
	}
	for _, row := range accounts {
		out.Accounts[row.ID] = accountFrom(row)
	}
	locals, err := q.ListLocalAccountsByIDs(ctx, list)
	if err != nil {
		return out, sqlite.Wrap(err, "list local accounts")
	}
	for _, row := range locals {
		out.Locals[row.AccountID] = account.LocalAccount(row)
	}
	handles, err := q.ListAccountHandlesByIDs(ctx, list)
	if err != nil {
		return out, sqlite.Wrap(err, "list account handles")
	}
	for _, row := range handles {
		out.Handles[row.AccountID] = account.Handle(row)
	}
	if err := r.currentProfiles(ctx, q, list, out.Profiles); err != nil {
		return out, err
	}
	keys, err := q.ListAccountKeysOfAccounts(ctx, list)
	if err != nil {
		return out, sqlite.Wrap(err, "list account keys")
	}
	for _, row := range keys {
		out.Keys[row.AccountID] = append(out.Keys[row.AccountID], keyFrom(row))
	}
	states, err := q.ListLifecycleStatesByIDs(ctx, list)
	if err != nil {
		return out, sqlite.Wrap(err, "list lifecycle states")
	}
	for _, row := range states {
		out.Lifecycle[row.AccountID] = lifecycleState(row)
	}
	return out, nil
}

// currentProfiles puts the current profile revision of each account of ids into out, as
// currentProfile reads one.
func (r *Repository) currentProfiles(ctx context.Context, q *accountgen.Queries, ids string, out map[string]account.ProfileRevision) error {
	rows, err := q.ListCurrentProfileRevisions(ctx, ids)
	if err != nil {
		return sqlite.Wrap(err, "list current profile revisions")
	}
	if len(rows) == 0 {
		return nil
	}
	revIDs := make([]string, len(rows))
	for i, row := range rows {
		revIDs[i] = row.ID
	}
	revs, err := sqlite.JSONList(revIDs)
	if err != nil {
		return err
	}
	fields, err := q.ListProfileFieldsOfRevisions(ctx, revs)
	if err != nil {
		return sqlite.Wrap(err, "list profile fields")
	}
	fieldsOf := map[string][]accountgen.AccountProfileField{}
	for _, f := range fields {
		fieldsOf[f.RevisionID] = append(fieldsOf[f.RevisionID], f)
	}
	emojis, err := q.ListProfileRevisionEmojisOfRevisions(ctx, revs)
	if err != nil {
		return sqlite.Wrap(err, "list profile revision emoji")
	}
	emojisOf := map[string][]string{}
	for _, e := range emojis {
		emojisOf[e.RevisionID] = append(emojisOf[e.RevisionID], e.EmojiID)
	}
	notes, err := q.ListProfileRevisionSourcesOfRevisions(ctx, revs)
	if err != nil {
		return sqlite.Wrap(err, "list profile revision sources")
	}
	noteOf := make(map[string]string, len(notes))
	for _, n := range notes {
		noteOf[n.RevisionID] = n.NoteSource
	}
	sources, err := q.ListProfileFieldSourcesOfRevisions(ctx, revs)
	if err != nil {
		return sqlite.Wrap(err, "list profile field sources")
	}
	sourcesOf := map[string]map[int]string{}
	for _, s := range sources {
		if sourcesOf[s.RevisionID] == nil {
			sourcesOf[s.RevisionID] = map[int]string{}
		}
		sourcesOf[s.RevisionID][int(s.Position)] = s.ValueSource
	}
	for _, row := range rows {
		rev := profileParts(profileRow(row), fieldsOf[row.ID], emojisOf[row.ID])
		if note, ok := noteOf[row.ID]; ok {
			rev = profileSource(rev, note, sourcesOf[row.ID])
		}
		out[row.AccountID] = rev
	}
	return nil
}

// FindAccounts implements account.Repository.
func (r *Repository) FindAccounts(ctx context.Context, ids []string) (map[string]account.Account, error) {
	out := map[string]account.Account{}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListAccountsByIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list accounts")
	}
	for _, row := range rows {
		out[row.ID] = accountFrom(row)
	}
	return out, nil
}
