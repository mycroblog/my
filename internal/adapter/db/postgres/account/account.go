// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/accountgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// Repository implements account.Repository on PostgreSQL.
type Repository struct {
	db *moduleDB
}

var _ appaccount.Repository = (*Repository)(nil)

// NewRepository returns the account module's repository on d.
func NewRepository(d *postgres.DB) *Repository {
	return &Repository{db: &moduleDB{d}}
}

// InsertAccount implements account.Repository.
func (r *Repository) InsertAccount(ctx context.Context, tx uow.Tx, a account.Account) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAccount(ctx, accountgen.InsertAccountParams{
		ID: a.ID, Uri: a.URI, Domain: a.Domain, ActorType: string(a.ActorType), CreatedAt: a.CreatedAt,
	})
	if postgres.IsUniqueViolation(err, "accounts_instance_actor") {
		return fmt.Errorf("insert account %s: %w", a.ID, appaccount.ErrInstanceActorExists)
	}
	if postgres.IsUniqueViolation(err, "accounts_uri") {
		// A concurrent transaction stored the same remote actor; a new attempt finds it.
		return uow.MarkRetryable(postgres.Wrap(err, "insert account"))
	}
	return postgres.Wrap(err, "insert account")
}

// InsertLocalAccount implements account.Repository.
func (r *Repository) InsertLocalAccount(ctx context.Context, tx uow.Tx, la account.LocalAccount) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertLocalAccount(ctx, accountgen.InsertLocalAccountParams{
		AccountID: la.AccountID, UserID: la.UserID, Username: la.Username, UsernameNormalized: la.UsernameNormalized,
	})
	if postgres.IsUniqueViolation(err, "local_accounts_username_normalized") {
		return fmt.Errorf("insert local account %q: %w", la.Username, appaccount.ErrUsernameTaken)
	}
	return postgres.Wrap(err, "insert local account")
}

// InsertProfileRevision implements account.Repository.
func (r *Repository) InsertProfileRevision(ctx context.Context, tx uow.Tx, rev account.ProfileRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	seq, err := postgres.ToInt32(rev.Seq, "profile revision seq")
	if err != nil {
		return err
	}
	err = q.InsertProfileRevision(ctx, accountgen.InsertProfileRevisionParams{
		ID:            rev.ID,
		AccountID:     rev.AccountID,
		Seq:           seq,
		DisplayName:   rev.DisplayName,
		NoteHtml:      rev.NoteHTML,
		Locked:        rev.Locked,
		Bot:           rev.Bot,
		Discoverable:  rev.Discoverable,
		Indexable:     rev.Indexable,
		AiOptOut:      rev.AIOptOut,
		AvatarMediaID: postgres.ToNullString(rev.AvatarMediaID),
		HeaderMediaID: postgres.ToNullString(rev.HeaderMediaID),
		RevisedAt:     rev.RevisedAt,
	})
	if postgres.IsUniqueViolation(err, "account_profile_revisions_account_id_seq") {
		// A concurrent revision took this seq after the command read the head at READ COMMITTED.
		// A new attempt reads the new head and takes the next seq.
		return uow.MarkRetryable(postgres.Wrap(err, "insert profile revision"))
	}
	if err != nil {
		return postgres.Wrap(err, "insert profile revision")
	}
	for _, f := range rev.Fields {
		pos, err := postgres.ToInt32(f.Position, "profile field position")
		if err != nil {
			return err
		}
		err = q.InsertProfileField(ctx, accountgen.InsertProfileFieldParams{
			RevisionID: rev.ID, Position: pos, Name: f.Name, ValueHtml: f.ValueHTML,
		})
		if err != nil {
			return postgres.Wrap(err, "insert profile field")
		}
	}
	for _, id := range rev.EmojiIDs {
		err := q.InsertProfileRevisionEmoji(ctx, accountgen.InsertProfileRevisionEmojiParams{RevisionID: rev.ID, EmojiID: id})
		if err != nil {
			return postgres.Wrap(err, "insert profile revision emoji")
		}
	}
	if rev.Remote != nil {
		version, err := postgres.ToInt32(rev.Remote.HTMLPolicyVersion, "HTML policy version")
		if err != nil {
			return err
		}
		err = q.InsertRemoteProfileRevision(ctx, accountgen.InsertRemoteProfileRevisionParams{
			RevisionID: rev.ID, SnapshotID: rev.Remote.SnapshotID, HtmlPolicyVersion: version, ProfileDigest: rev.Remote.ProfileDigest,
		})
		if err != nil {
			return postgres.Wrap(err, "insert remote profile revision")
		}
	}
	if rev.Source == nil {
		return nil
	}
	if err := q.InsertProfileRevisionSource(ctx, accountgen.InsertProfileRevisionSourceParams{
		RevisionID: rev.ID, NoteSource: rev.Source.Note,
	}); err != nil {
		return postgres.Wrap(err, "insert profile revision source")
	}
	for i, v := range rev.Source.FieldValues {
		if i >= len(rev.Fields) {
			return fmt.Errorf("profile revision %s: %d field sources for %d fields", rev.ID, len(rev.Source.FieldValues), len(rev.Fields))
		}
		pos, err := postgres.ToInt32(rev.Fields[i].Position, "profile field position")
		if err != nil {
			return err
		}
		err = q.InsertProfileFieldSource(ctx, accountgen.InsertProfileFieldSourceParams{
			RevisionID: rev.ID, Position: pos, ValueSource: v,
		})
		if err != nil {
			return postgres.Wrap(err, "insert profile field source")
		}
	}
	return nil
}

// InsertKey implements account.Repository.
func (r *Repository) InsertKey(ctx context.Context, tx uow.Tx, k account.Key, sealedPrivateKey []byte) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertAccountKey(ctx, accountgen.InsertAccountKeyParams{
		ID: k.ID, AccountID: k.AccountID, KeyID: k.KeyID, Algorithm: string(k.Algorithm),
		PublicKeyPem: k.PublicKeyPEM, Fingerprint: k.Fingerprint, CreatedAt: k.CreatedAt,
	})
	if err != nil {
		if postgres.IsUniqueViolation(err, "account_keys_key_id_fingerprint") {
			// A concurrent transaction stored the same key; a new attempt finds it.
			return uow.MarkRetryable(postgres.Wrap(err, "insert account key"))
		}
		return postgres.Wrap(err, "insert account key")
	}
	if sealedPrivateKey == nil {
		return nil
	}
	err = q.InsertLocalAccountPrivateKey(ctx, accountgen.InsertLocalAccountPrivateKeyParams{
		AccountKeyID: k.ID, PrivateKeyEnc: sealedPrivateKey,
	})
	return postgres.Wrap(err, "insert private key")
}

// InsertLifecycleEvent implements account.Repository.
func (r *Repository) InsertLifecycleEvent(ctx context.Context, tx uow.Tx, e account.LifecycleEvent) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertLifecycleEvent(ctx, accountgen.InsertLifecycleEventParams{
		ID:                 e.ID,
		AccountID:          e.AccountID,
		Kind:               string(e.Kind),
		InitiatedBy:        string(e.InitiatedBy),
		ModerationActionID: postgres.ToNullString(e.ModerationActionID),
		DeletionDueAt:      e.DeletionDueAt,
		OccurredAt:         e.OccurredAt,
	})
	return postgres.Wrap(err, "insert lifecycle event")
}

// UpsertHandle implements account.Repository.
func (r *Repository) UpsertHandle(ctx context.Context, tx uow.Tx, h account.Handle) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.UpsertAccountHandle(ctx, accountgen.UpsertAccountHandleParams(h)), "upsert account handle")
}

// DeleteAllHandles implements account.Repository.
func (r *Repository) DeleteAllHandles(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteAllAccountHandles(ctx), "delete account handles")
}

// ProfileHead implements account.Repository.
func (r *Repository) ProfileHead(ctx context.Context, tx uow.Tx, accountID string) (account.ProfileHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.ProfileHead{}, err
	}
	row, err := q.GetProfileHead(ctx, accountID)
	if err != nil {
		return account.ProfileHead{}, postgres.NotFound(err, appaccount.ErrNotFound, "profile head of "+accountID)
	}
	return account.ProfileHead{AccountID: row.AccountID, RevisionID: row.RevisionID, Seq: int(row.Seq)}, nil
}

// UpsertProfileHead implements account.Repository.
func (r *Repository) UpsertProfileHead(ctx context.Context, tx uow.Tx, h account.ProfileHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertProfileHead(ctx, accountgen.UpsertProfileHeadParams{AccountID: h.AccountID, RevisionID: h.RevisionID})
	return postgres.Wrap(err, "upsert profile head")
}

// DeleteAllProfileHeads implements account.Repository.
func (r *Repository) DeleteAllProfileHeads(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteAllProfileHeads(ctx), "delete profile heads")
}

// LifecycleState implements account.Repository.
func (r *Repository) LifecycleState(ctx context.Context, tx uow.Tx, accountID string) (account.LifecycleState, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.LifecycleState{}, err
	}
	row, err := q.GetLifecycleStateForUpdate(ctx, accountID)
	if err != nil {
		return account.LifecycleState{}, postgres.NotFound(err, appaccount.ErrNotFound, "lifecycle state of "+accountID)
	}
	return lifecycleState(row), nil
}

func lifecycleState(row accountgen.AccountLifecycleState) account.LifecycleState {
	return account.LifecycleState{
		AccountID:     row.AccountID,
		State:         account.LifecycleStateName(row.State),
		Since:         postgres.UTC(row.Since),
		DeletionDueAt: postgres.UTCPtr(row.DeletionDueAt),
	}
}

// NewestRemoteDeletion implements account.Repository.
func (r *Repository) NewestRemoteDeletion(ctx context.Context, tx uow.Tx, accountID string) (account.Initiator, error) {
	var by string
	var err error
	if tx == nil {
		by, err = r.db.reader().FindNewestRemoteDeletion(ctx, accountID)
	} else {
		q, qerr := r.db.queries(tx)
		if qerr != nil {
			return "", qerr
		}
		by, err = q.FindNewestRemoteDeletion(ctx, accountID)
	}
	if err != nil {
		return "", postgres.NotFound(err, appaccount.ErrNotFound, "remote deletion of "+accountID)
	}
	return account.Initiator(by), nil
}

// UpsertLifecycleState implements account.Repository.
func (r *Repository) UpsertLifecycleState(ctx context.Context, tx uow.Tx, s account.LifecycleState) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertLifecycleState(ctx, accountgen.UpsertLifecycleStateParams{
		AccountID: s.AccountID, State: string(s.State), Since: s.Since, DeletionDueAt: s.DeletionDueAt,
	})
	return postgres.Wrap(err, "upsert lifecycle state")
}

// DeleteAllLifecycleStates implements account.Repository.
func (r *Repository) DeleteAllLifecycleStates(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteAllLifecycleStates(ctx), "delete lifecycle states")
}

// ListLocalAccounts implements account.Repository.
func (r *Repository) ListLocalAccounts(ctx context.Context, tx uow.Tx) ([]account.LocalAccount, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLocalAccounts(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "list local accounts")
	}
	out := make([]account.LocalAccount, len(rows))
	for i, row := range rows {
		out[i] = account.LocalAccount(row)
	}
	return out, nil
}

// ListProfileRevisionSeqs implements account.Repository.
func (r *Repository) ListProfileRevisionSeqs(ctx context.Context, tx uow.Tx) ([]account.ProfileHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListProfileRevisionSeqs(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "list profile revisions")
	}
	out := make([]account.ProfileHead, len(rows))
	for i, row := range rows {
		out[i] = account.ProfileHead{AccountID: row.AccountID, RevisionID: row.ID, Seq: int(row.Seq)}
	}
	return out, nil
}

// ListLifecycleEvents implements account.Repository.
func (r *Repository) ListLifecycleEvents(ctx context.Context, tx uow.Tx) ([]account.LifecycleEvent, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLifecycleEvents(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "list lifecycle events")
	}
	out := make([]account.LifecycleEvent, len(rows))
	for i, row := range rows {
		out[i] = account.LifecycleEvent{
			ID:                 row.ID,
			AccountID:          row.AccountID,
			Kind:               account.LifecycleKind(row.Kind),
			InitiatedBy:        account.Initiator(row.InitiatedBy),
			ModerationActionID: postgres.FromNullString(row.ModerationActionID),
			DeletionDueAt:      postgres.UTCPtr(row.DeletionDueAt),
			OccurredAt:         postgres.UTC(row.OccurredAt),
		}
	}
	return out, nil
}

// ListHandles implements account.Repository.
func (r *Repository) ListHandles(ctx context.Context) ([]account.Handle, error) {
	rows, err := r.db.reader().ListAccountHandles(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "list account handles")
	}
	out := make([]account.Handle, len(rows))
	for i, row := range rows {
		out[i] = account.Handle(row)
	}
	return out, nil
}

// ListProfileHeads implements account.Repository.
func (r *Repository) ListProfileHeads(ctx context.Context) ([]account.ProfileHead, error) {
	rows, err := r.db.reader().ListProfileHeads(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "list profile heads")
	}
	out := make([]account.ProfileHead, len(rows))
	for i, row := range rows {
		out[i] = account.ProfileHead{AccountID: row.AccountID, RevisionID: row.RevisionID, Seq: int(row.Seq)}
	}
	return out, nil
}

// ListLifecycleStates implements account.Repository.
func (r *Repository) ListLifecycleStates(ctx context.Context) ([]account.LifecycleState, error) {
	rows, err := r.db.reader().ListLifecycleStates(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "list lifecycle states")
	}
	out := make([]account.LifecycleState, len(rows))
	for i, row := range rows {
		out[i] = lifecycleState(row)
	}
	return out, nil
}

func accountFrom(row accountgen.Account) account.Account {
	return account.Account{
		ID: row.ID, URI: row.Uri, Domain: row.Domain, ActorType: account.ActorType(row.ActorType), CreatedAt: postgres.UTC(row.CreatedAt),
	}
}

// Account implements account.Repository.
func (r *Repository) Account(ctx context.Context, tx uow.Tx, id string) (account.Account, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.Account{}, err
	}
	row, err := q.GetAccount(ctx, id)
	if err != nil {
		return account.Account{}, postgres.NotFound(err, appaccount.ErrNotFound, "account "+id)
	}
	return accountFrom(row), nil
}

// FindAccount implements account.Repository.
func (r *Repository) FindAccount(ctx context.Context, id string) (account.Account, error) {
	row, err := r.db.reader().GetAccount(ctx, id)
	if err != nil {
		return account.Account{}, postgres.NotFound(err, appaccount.ErrNotFound, "account "+id)
	}
	return accountFrom(row), nil
}

// FindAccountByURI implements account.Repository.
func (r *Repository) FindAccountByURI(ctx context.Context, uri string) (account.Account, error) {
	row, err := r.db.reader().GetAccountByURI(ctx, uri)
	if err != nil {
		return account.Account{}, postgres.NotFound(err, appaccount.ErrNotFound, "account "+uri)
	}
	return accountFrom(row), nil
}

// AnyAccountOnDomain implements account.Repository.
func (r *Repository) AnyAccountOnDomain(ctx context.Context, domain string, ids []string) (bool, error) {
	if len(ids) == 0 {
		return false, nil
	}
	if ids == nil {
		ids = []string{}
	}
	found, err := r.db.reader().AnyAccountOnDomain(ctx, accountgen.AnyAccountOnDomainParams{Domain: domain, Ids: ids})
	if err != nil {
		return false, postgres.Wrap(err, "find accounts on "+domain)
	}
	return found, nil
}

// ExistingAccountIDs implements account.Repository.
func (r *Repository) ExistingAccountIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out, err := r.db.reader().ListExistingAccountIDs(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list existing accounts")
	}
	return out, nil
}

// AccountsOnDomain implements account.Repository.
func (r *Repository) AccountsOnDomain(ctx context.Context, tx uow.Tx, domain string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	out, err := q.ListAccountsOnDomain(ctx, accountgen.ListAccountsOnDomainParams{Domain: domain, Ids: ids})
	if err != nil {
		return nil, postgres.Wrap(err, "list accounts on "+domain)
	}
	return out, nil
}

// ListAccountIDsOnDomain implements account.Repository.
func (r *Repository) ListAccountIDsOnDomain(ctx context.Context, domain, after string, limit int) ([]string, error) {
	pageLimit, err := postgres.ToInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	out, err := r.db.reader().ListAccountIDsOnDomainAfter(ctx, accountgen.ListAccountIDsOnDomainAfterParams{Domain: domain, After: postgres.ToNullString(after), PageLimit: pageLimit})
	if err != nil {
		return nil, postgres.Wrap(err, "list accounts on "+domain)
	}
	return out, nil
}

// ListLocalAccountIDs implements account.Repository.
func (r *Repository) ListLocalAccountIDs(ctx context.Context, after string, limit int) ([]string, error) {
	pageLimit, err := postgres.ToInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	out, err := r.db.reader().ListLocalAccountIDsAfter(ctx, accountgen.ListLocalAccountIDsAfterParams{After: postgres.ToNullString(after), PageLimit: pageLimit})
	if err != nil {
		return nil, postgres.Wrap(err, "list local account ids")
	}
	return out, nil
}

// FindInstanceActor implements account.Repository.
func (r *Repository) FindInstanceActor(ctx context.Context) (account.Account, error) {
	row, err := r.db.reader().GetInstanceActor(ctx)
	if err != nil {
		return account.Account{}, postgres.NotFound(err, appaccount.ErrNotFound, "instance actor")
	}
	return accountFrom(row), nil
}

// FindLocalAccount implements account.Repository.
func (r *Repository) FindLocalAccount(ctx context.Context, accountID string) (account.LocalAccount, error) {
	row, err := r.db.reader().GetLocalAccount(ctx, accountID)
	if err != nil {
		return account.LocalAccount{}, postgres.NotFound(err, appaccount.ErrNotFound, "local account "+accountID)
	}
	return account.LocalAccount(row), nil
}

// FindLocalAccountByUser implements account.Repository.
func (r *Repository) FindLocalAccountByUser(ctx context.Context, userID string) (account.LocalAccount, error) {
	row, err := r.db.reader().GetLocalAccountByUserID(ctx, userID)
	if err != nil {
		return account.LocalAccount{}, postgres.NotFound(err, appaccount.ErrNotFound, "local account of user "+userID)
	}
	return account.LocalAccount(row), nil
}

// FindHandle implements account.Repository.
func (r *Repository) FindHandle(ctx context.Context, usernameNormalized, domain string) (account.Handle, error) {
	row, err := r.db.reader().GetAccountHandleByName(ctx, accountgen.GetAccountHandleByNameParams{
		UsernameNormalized: usernameNormalized, Domain: domain,
	})
	if err != nil {
		return account.Handle{}, postgres.NotFound(err, appaccount.ErrNotFound, fmt.Sprintf("handle %s@%s", usernameNormalized, domain))
	}
	return account.Handle(row), nil
}

// FindHandleOf implements account.Repository.
func (r *Repository) FindHandleOf(ctx context.Context, accountID string) (account.Handle, error) {
	row, err := r.db.reader().GetAccountHandle(ctx, accountID)
	if err != nil {
		return account.Handle{}, postgres.NotFound(err, appaccount.ErrNotFound, "handle of "+accountID)
	}
	return account.Handle(row), nil
}

// FindCurrentProfile implements account.Repository.
func (r *Repository) FindCurrentProfile(ctx context.Context, accountID string) (account.ProfileRevision, error) {
	return currentProfile(ctx, r.db.reader(), accountID)
}

// CurrentProfile implements account.Repository.
func (r *Repository) CurrentProfile(ctx context.Context, tx uow.Tx, accountID string) (account.ProfileRevision, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return account.ProfileRevision{}, err
	}
	return currentProfile(ctx, q, accountID)
}

func currentProfile(ctx context.Context, q *accountgen.Queries, accountID string) (account.ProfileRevision, error) {
	head, err := q.GetProfileHead(ctx, accountID)
	if err != nil {
		return account.ProfileRevision{}, postgres.NotFound(err, appaccount.ErrNotFound, "profile head of "+accountID)
	}
	row, err := q.GetProfileRevision(ctx, head.RevisionID)
	if err != nil {
		return account.ProfileRevision{}, postgres.NotFound(err, appaccount.ErrNotFound, "profile revision "+head.RevisionID)
	}
	fields, err := q.ListProfileFields(ctx, row.ID)
	if err != nil {
		return account.ProfileRevision{}, postgres.Wrap(err, "list profile fields")
	}
	emojis, err := q.ListProfileRevisionEmojis(ctx, row.ID)
	if err != nil {
		return account.ProfileRevision{}, postgres.Wrap(err, "list profile revision emoji")
	}
	rev := profileParts(profileRow(row), fields, emojis)
	note, err := q.GetProfileRevisionSource(ctx, row.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return rev, nil
	}
	if err != nil {
		return account.ProfileRevision{}, postgres.Wrap(err, "get profile revision source")
	}
	sources, err := q.ListProfileFieldSources(ctx, row.ID)
	if err != nil {
		return account.ProfileRevision{}, postgres.Wrap(err, "list profile field sources")
	}
	byPosition := make(map[int]string, len(sources))
	for _, s := range sources {
		byPosition[int(s.Position)] = s.ValueSource
	}
	return profileSource(rev, note, byPosition), nil
}

// profileRow returns the revision of row, without its fields, custom emoji, and source.
func profileRow(row accountgen.AccountProfileRevision) account.ProfileRevision {
	return account.ProfileRevision{
		ID:            row.ID,
		AccountID:     row.AccountID,
		Seq:           int(row.Seq),
		DisplayName:   row.DisplayName,
		NoteHTML:      row.NoteHtml,
		Locked:        row.Locked,
		Bot:           row.Bot,
		Discoverable:  row.Discoverable,
		Indexable:     row.Indexable,
		AIOptOut:      row.AiOptOut,
		AvatarMediaID: postgres.FromNullString(row.AvatarMediaID),
		HeaderMediaID: postgres.FromNullString(row.HeaderMediaID),
		RevisedAt:     postgres.UTC(row.RevisedAt),
	}
}

// profileParts returns rev with the fields and the custom emoji of its revision.
func profileParts(rev account.ProfileRevision, fields []accountgen.AccountProfileField, emojis []string) account.ProfileRevision {
	for _, f := range fields {
		rev.Fields = append(rev.Fields, account.ProfileField{Position: int(f.Position), Name: f.Name, ValueHTML: f.ValueHtml})
	}
	if len(emojis) > 0 {
		rev.EmojiIDs = emojis
	}
	return rev
}

// profileSource returns rev, a local account's revision, with its source: the note and the source
// of each field by its position ("" for a field without one).
func profileSource(rev account.ProfileRevision, note string, byPosition map[int]string) account.ProfileRevision {
	rev.Source = &account.ProfileSource{Note: note, FieldValues: make([]string, len(rev.Fields))}
	for i, f := range rev.Fields {
		rev.Source.FieldValues[i] = byPosition[f.Position]
	}
	return rev
}

// FindLifecycleState implements account.Repository.
func (r *Repository) FindLifecycleState(ctx context.Context, accountID string) (account.LifecycleState, error) {
	row, err := r.db.reader().GetLifecycleState(ctx, accountID)
	if err != nil {
		return account.LifecycleState{}, postgres.NotFound(err, appaccount.ErrNotFound, "lifecycle state of "+accountID)
	}
	return lifecycleState(row), nil
}

// ListKeys implements account.Repository.
func (r *Repository) ListKeys(ctx context.Context, accountID string) ([]account.Key, error) {
	rows, err := r.db.reader().ListAccountKeys(ctx, accountID)
	if err != nil {
		return nil, postgres.Wrap(err, "list account keys")
	}
	out := make([]account.Key, len(rows))
	for i, row := range rows {
		out[i] = keyFrom(row)
	}
	return out, nil
}

// ListKeysWithSealed implements account.Repository.
func (r *Repository) ListKeysWithSealed(ctx context.Context, accountID string) ([]appaccount.KeyWithSealed, error) {
	rows, err := r.db.reader().ListAccountKeysWithPrivate(ctx, accountID)
	if err != nil {
		return nil, postgres.Wrap(err, "list account keys")
	}
	out := make([]appaccount.KeyWithSealed, len(rows))
	for i, row := range rows {
		out[i] = appaccount.KeyWithSealed{
			Key: keyFrom(accountgen.AccountKey{
				ID: row.ID, AccountID: row.AccountID, KeyID: row.KeyID, Algorithm: row.Algorithm,
				PublicKeyPem: row.PublicKeyPem, Fingerprint: row.Fingerprint, CreatedAt: row.CreatedAt,
			}),
			Sealed: row.PrivateKeyEnc,
		}
	}
	return out, nil
}

// FindSealedPrivateKey implements account.Repository.
func (r *Repository) FindSealedPrivateKey(ctx context.Context, accountKeyID string) ([]byte, error) {
	sealed, err := r.db.reader().GetLocalAccountPrivateKey(ctx, accountKeyID)
	if err != nil {
		return nil, postgres.NotFound(err, appaccount.ErrNotFound, "private key of "+accountKeyID)
	}
	return sealed, nil
}

// IsMediaReferenced implements account.Repository.
func (r *Repository) IsMediaReferenced(ctx context.Context, mediaID string) (bool, error) {
	ok, err := r.db.reader().IsMediaReferencedByProfiles(ctx, postgres.ToNullString(mediaID))
	return ok, postgres.Wrap(err, "look up profile media")
}

// IsEmojiReferenced implements account.Repository.
func (r *Repository) IsEmojiReferenced(ctx context.Context, emojiID string) (bool, error) {
	ok, err := r.db.reader().IsEmojiReferencedByProfiles(ctx, emojiID)
	return ok, postgres.Wrap(err, "look up profile emoji")
}

// CountLocalAccounts implements account.Repository.
func (r *Repository) CountLocalAccounts(ctx context.Context) (int, error) {
	n, err := r.db.reader().CountLocalAccounts(ctx)
	if err != nil {
		return 0, postgres.Wrap(err, "count local accounts")
	}
	return int(n), nil
}
