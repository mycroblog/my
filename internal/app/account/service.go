// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// Deps are the ports and settings a Service uses.
type Deps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Clock      kernel.Clock
	IDs        kernel.IDGenerator
	// Sealer encrypts actor private keys for storage.
	Sealer *kernel.Sealer
	// Host is the serving domain that every local actor URI uses (ADR-0018).
	Host string
	// GenerateKey generates actor keys; nil means GenerateKey.
	GenerateKey KeyGenerator
	// RenderProfileText renders the typed note and field values of a local profile to HTML,
	// as the text of a post is rendered (the content module); nil escapes the text.
	RenderProfileText func(ctx context.Context, text string) (string, error)
	// CheckProfileImage fails with an error wrapping ErrUnusableImage unless a media file is
	// an image that the account uploaded and that was processed successfully (the content
	// module), which then makes its variants for prop (account.PropertyAvatar or
	// account.PropertyHeader); nil accepts no image.
	CheckProfileImage func(ctx context.Context, accountID, mediaID string, prop account.ProfileProperty) error
	// Sanitizer cleans the HTML of remote profiles (ADR-0019 §2); remote actors cannot be
	// stored without it.
	Sanitizer kernel.HTMLSanitizer
	// RecordRemoteImage records the avatar or header image of a remote actor at url inside tx
	// and returns its media file ID (the content module); nil keeps remote profiles without
	// images.
	RecordRemoteImage func(ctx context.Context, tx uow.Tx, accountID, url string, prop account.ProfileProperty) (string, error)
	// RecordRemoteEmojis records the custom emoji of a remote profile inside tx, as the emoji of
	// domain with their images owned by the account, and returns their IDs, sorted (the content
	// module, ADR-0051 §3); nil keeps remote profiles without custom emoji.
	RecordRemoteEmojis func(ctx context.Context, tx uow.Tx, accountID, domain string, emojis []RemoteEmojiInput) ([]string, error)
	// LocalEmojis returns the IDs, sorted, of the enabled local custom emoji that the texts of
	// a local profile use as :shortcode: (the content module, ADR-0051 §3); nil records none.
	LocalEmojis func(ctx context.Context, texts ...string) ([]string, error)
}

// ErrUnusableImage reports an avatar or header that is not a processed image of the account.
var ErrUnusableImage = errors.New("account: the media file is not a processed image of the account")

// Service runs the account module's use cases.
type Service struct {
	d Deps
}

// NewService returns a Service on d.
func NewService(d Deps) *Service {
	if d.GenerateKey == nil {
		d.GenerateKey = GenerateKey
	}
	return &Service{d: d}
}

// LocalAccountInput describes a new local account.
type LocalAccountInput struct {
	// UserID is the identity module's user that logs in as the account.
	UserID   string
	Username string
	// DisplayName is the display name of the first profile revision; it may be empty.
	DisplayName string
	// Indexable is the indexable of the first profile revision: the instance setting
	// indexable_default (FR-412, ADR-0057).
	Indexable bool
}

// PreparedLocalAccount is a local account with its IDs, time, and sealed keys fixed, ready to
// be written by WriteLocalAccount. Preparing does the slow and external work (key generation,
// sealing through the KeyManager) outside the transaction, so that a retried transaction
// writes the same values.
type PreparedLocalAccount struct {
	Account account.Account
	Local   account.LocalAccount
	Profile account.ProfileRevision
	keys    []preparedKey
}

type preparedKey struct {
	key    account.Key
	sealed []byte
}

// PrepareLocalAccount validates in and generates everything a new local account needs.
// It writes nothing.
func (s *Service) PrepareLocalAccount(ctx context.Context, in LocalAccountInput) (*PreparedLocalAccount, error) {
	if in.UserID == "" {
		return nil, errors.New("account: a local account needs a user")
	}
	if err := account.ValidateUsername(in.Username); err != nil {
		return nil, err
	}
	now := s.d.Clock.Now()
	a := account.Account{ID: s.d.IDs.NewID(), Domain: account.LocalDomain, ActorType: account.ActorPerson, CreatedAt: now}
	a.URI = account.ActorURI(s.d.Host, a.ID)
	keys, err := s.prepareKeys(ctx, a, now)
	if err != nil {
		return nil, err
	}
	return &PreparedLocalAccount{
		Account: a,
		Local: account.LocalAccount{
			AccountID:          a.ID,
			UserID:             in.UserID,
			Username:           in.Username,
			UsernameNormalized: account.NormalizeUsername(in.Username),
		},
		Profile: account.ProfileRevision{
			ID:          s.d.IDs.NewID(),
			AccountID:   a.ID,
			Seq:         1,
			DisplayName: in.DisplayName,
			Indexable:   in.Indexable,
			RevisedAt:   now,
			Source:      &account.ProfileSource{},
		},
		keys: keys,
	}, nil
}

func (s *Service) prepareKeys(ctx context.Context, a account.Account, now time.Time) ([]preparedKey, error) {
	out := make([]preparedKey, 0, len(LocalKeyAlgorithms))
	for _, alg := range LocalKeyAlgorithms {
		priv, err := s.d.GenerateKey(alg)
		if err != nil {
			return nil, fmt.Errorf("account: generate %s key: %w", alg, err)
		}
		if err := checkAlgorithm(alg, priv); err != nil {
			return nil, err
		}
		pubPEM, fingerprint, err := encodePublicKey(priv.Public())
		if err != nil {
			return nil, fmt.Errorf("account: encode %s public key: %w", alg, err)
		}
		der, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			return nil, fmt.Errorf("account: encode %s private key: %w", alg, err)
		}
		k := account.Key{
			ID:           s.d.IDs.NewID(),
			AccountID:    a.ID,
			KeyID:        account.KeyIRI(a.URI, alg),
			Algorithm:    alg,
			PublicKeyPEM: pubPEM,
			Fingerprint:  fingerprint,
			CreatedAt:    now,
		}
		sealed, err := s.d.Sealer.Seal(ctx, der, PrivateKeyAAD(k.ID))
		if err != nil {
			return nil, fmt.Errorf("account: seal %s private key: %w", alg, err)
		}
		out = append(out, preparedKey{key: k, sealed: sealed})
	}
	return out, nil
}

// WriteLocalAccount writes a prepared local account inside tx and emits its events, so that
// another module's command (such as user registration) can create the account in its own
// transaction. It fails with ErrUsernameTaken when the username is taken.
func (s *Service) WriteLocalAccount(ctx context.Context, tx uow.Tx, p *PreparedLocalAccount) error {
	repo := s.d.Repository
	if err := repo.InsertAccount(ctx, tx, p.Account); err != nil {
		return err
	}
	if err := repo.InsertLocalAccount(ctx, tx, p.Local); err != nil {
		return err
	}
	if err := writeKeys(ctx, tx, repo, p.keys); err != nil {
		return err
	}
	if err := repo.InsertProfileRevision(ctx, tx, p.Profile); err != nil {
		return err
	}
	if err := tx.Emit(ctx, LocalAccountCreated{Account: p.Account, Local: p.Local}); err != nil {
		return err
	}
	return tx.Emit(ctx, ProfileRevised{Revision: p.Profile})
}

func writeKeys(ctx context.Context, tx uow.Tx, repo Repository, keys []preparedKey) error {
	for _, k := range keys {
		if err := repo.InsertKey(ctx, tx, k.key, k.sealed); err != nil {
			return err
		}
	}
	return nil
}

// CreateLocalAccount creates a local account for an existing user, with RSA and Ed25519 keys
// and an initial profile revision.
func (s *Service) CreateLocalAccount(ctx context.Context, in LocalAccountInput) (account.Account, error) {
	p, err := s.PrepareLocalAccount(ctx, in)
	if err != nil {
		return account.Account{}, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.WriteLocalAccount(ctx, tx, p) })
	if err != nil {
		return account.Account{}, err
	}
	return p.Account, nil
}

// EnsureInstanceActor returns the instance actor (the local Application actor that signs the
// instance's own requests), creating it with its keys on first use. Concurrent calls agree on
// one actor.
func (s *Service) EnsureInstanceActor(ctx context.Context) (account.Account, error) {
	a, err := s.d.Repository.FindInstanceActor(ctx)
	if !errors.Is(err, ErrNotFound) {
		return a, err
	}
	now := s.d.Clock.Now()
	a = account.Account{ID: s.d.IDs.NewID(), Domain: account.LocalDomain, ActorType: account.ActorApplication, CreatedAt: now}
	a.URI = account.ActorURI(s.d.Host, a.ID)
	keys, err := s.prepareKeys(ctx, a, now)
	if err != nil {
		return account.Account{}, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertAccount(ctx, tx, a); err != nil {
			return err
		}
		return writeKeys(ctx, tx, s.d.Repository, keys)
	})
	if errors.Is(err, ErrInstanceActorExists) {
		return s.d.Repository.FindInstanceActor(ctx)
	}
	if err != nil {
		return account.Account{}, err
	}
	return a, nil
}

// ProfileInput is the content of a new profile revision.
type ProfileInput struct {
	DisplayName   string
	NoteHTML      string
	Locked        bool
	Bot           bool
	Discoverable  bool
	Indexable     bool
	AIOptOut      bool
	AvatarMediaID string
	HeaderMediaID string
	// Fields are numbered by their order.
	Fields []account.ProfileField
	// Source is what the owner typed, nil for none; its FieldValues match Fields.
	Source *account.ProfileSource
	// EmojiIDs are the custom emoji of the display name, note, and fields, sorted.
	EmojiIDs []string
}

// IsMediaReferenced reports whether any profile revision, current or not, uses a media file as
// its avatar or header. It implements content.MediaReferrer, so that the cleanup of unattached
// uploads keeps profile images.
func (s *Service) IsMediaReferenced(ctx context.Context, mediaID string) (bool, error) {
	return s.d.Repository.IsMediaReferenced(ctx, mediaID)
}

// IsEmojiReferenced reports whether any profile revision, current or not, uses a custom emoji in
// its display name, note, or fields. It implements content.EmojiReferrer, so that the purge of
// remote custom emoji keeps those of profiles.
func (s *Service) IsEmojiReferenced(ctx context.Context, emojiID string) (bool, error) {
	return s.d.Repository.IsEmojiReferenced(ctx, emojiID)
}

// ReviseProfile records a new revision of an account's profile and makes it current.
func (s *Service) ReviseProfile(ctx context.Context, accountID string, in ProfileInput) (account.ProfileRevision, error) {
	return s.reviseProfile(ctx, accountID, in, nil)
}

// errProfileChanged reports that the current profile revision is no longer the one that a
// revision was built on.
var errProfileChanged = errors.New("account: the profile changed concurrently")

// reviseProfile records a new revision like ReviseProfile. With a non-nil baseSeq, it fails with
// errProfileChanged unless the current revision has that seq (0 for none).
func (s *Service) reviseProfile(ctx context.Context, accountID string, in ProfileInput, baseSeq *int) (account.ProfileRevision, error) {
	base := account.ProfileRevision{
		ID:            s.d.IDs.NewID(),
		AccountID:     accountID,
		DisplayName:   in.DisplayName,
		NoteHTML:      in.NoteHTML,
		Locked:        in.Locked,
		Bot:           in.Bot,
		Discoverable:  in.Discoverable,
		Indexable:     in.Indexable,
		AIOptOut:      in.AIOptOut,
		AvatarMediaID: in.AvatarMediaID,
		HeaderMediaID: in.HeaderMediaID,
		RevisedAt:     s.d.Clock.Now(),
		Source:        in.Source,
		EmojiIDs:      in.EmojiIDs,
	}
	if in.Source != nil && len(in.Source.FieldValues) != len(in.Fields) {
		return account.ProfileRevision{}, fmt.Errorf("account: %d field sources for %d fields", len(in.Source.FieldValues), len(in.Fields))
	}
	for i, f := range in.Fields {
		f.Position = i
		base.Fields = append(base.Fields, f)
	}
	var rev account.ProfileRevision
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rev = base
		head, err := s.d.Repository.ProfileHead(ctx, tx, accountID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if baseSeq != nil && head.Seq != *baseSeq {
			return errProfileChanged
		}
		rev.Seq = head.Seq + 1
		if err := s.d.Repository.InsertProfileRevision(ctx, tx, rev); err != nil {
			return err
		}
		return tx.Emit(ctx, ProfileRevised{Revision: rev})
	})
	if err != nil {
		return account.ProfileRevision{}, err
	}
	return rev, nil
}

// LifecycleInput describes a lifecycle event to record.
type LifecycleInput struct {
	AccountID          string
	Kind               account.LifecycleKind
	InitiatedBy        account.Initiator
	ModerationActionID string
	// DeletionDueAt is required for account.LifecycleDeletionScheduled and forbidden otherwise.
	DeletionDueAt *time.Time
}

// RecordLifecycleEvent records a lifecycle event of an account and updates its lifecycle state.
func (s *Service) RecordLifecycleEvent(ctx context.Context, in LifecycleInput) (account.LifecycleEvent, error) {
	if (in.Kind == account.LifecycleDeletionScheduled) != (in.DeletionDueAt != nil) {
		return account.LifecycleEvent{}, errors.New("account: a deletion due time is required for, and only for, deletion_scheduled")
	}
	e := account.LifecycleEvent{
		ID:                 s.d.IDs.NewID(),
		AccountID:          in.AccountID,
		Kind:               in.Kind,
		InitiatedBy:        in.InitiatedBy,
		ModerationActionID: in.ModerationActionID,
		OccurredAt:         s.d.Clock.Now(),
	}
	if in.DeletionDueAt != nil {
		due := kernel.Normalize(*in.DeletionDueAt)
		e.DeletionDueAt = &due
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertLifecycleEvent(ctx, tx, e); err != nil {
			return err
		}
		return tx.Emit(ctx, LifecycleEventRecorded{Event: e})
	})
	if err != nil {
		return account.LifecycleEvent{}, err
	}
	return e, nil
}

// ActorURI returns the ActivityPub id of a local account on host (ADR-0018), the URI that every
// local account gets when it is created.
func ActorURI(host, accountID string) string { return account.ActorURI(host, accountID) }

// Actor is an account with what is needed to present it: its handle and current profile
// (when it has them), public keys, and lifecycle state.
type Actor struct {
	Account   account.Account
	Local     *account.LocalAccount // nil unless a local account (the instance actor has none)
	Handle    *account.Handle
	Profile   *account.ProfileRevision
	Keys      []account.Key
	Lifecycle account.LifecycleState
}

// Deleted reports whether a is deleted, locally or on its own server. Deletion is final.
func (a Actor) Deleted() bool { return a.Lifecycle.State == account.StateDeleted }

// IsInstanceActor reports whether a is the instance actor.
func (a Actor) IsInstanceActor() bool {
	return a.Account.IsLocal() && a.Account.ActorType == account.ActorApplication
}

// ActivityStreamsType returns the AS2 type a is published as: its actor type, except that a
// person whose profile is a bot's is a Service (Mastodon's bot flag).
func (a Actor) ActivityStreamsType() string {
	t := a.Account.ActorType
	if t == account.ActorPerson && a.Profile != nil && a.Profile.Bot {
		t = account.ActorService
	}
	return activityStreamsTypes[t]
}

// activityStreamsTypes maps actor types to AS2 types, the inverse of ActorTypeOf.
var activityStreamsTypes = map[account.ActorType]string{
	account.ActorPerson:       "Person",
	account.ActorService:      "Service",
	account.ActorApplication:  "Application",
	account.ActorGroup:        "Group",
	account.ActorOrganization: "Organization",
}

// PublishedKey returns the key that a's document publishes as publicKey: its RSA key at
// {actor}#main-key (ADR-0032), false when it has none.
func (a Actor) PublishedKey() (account.Key, bool) {
	for _, k := range a.Keys {
		if k.Algorithm == account.KeyRSASHA256 && k.KeyID == account.KeyIRI(a.Account.URI, account.KeyRSASHA256) {
			return k, true
		}
	}
	return account.Key{}, false
}

// AssertionKey returns the key that a's document publishes in assertionMethod: its Ed25519 key
// at {actor}#ed25519-key, which signs its integrity proofs (FEP-521a, FEP-8b32), false when it
// has none.
func (a Actor) AssertionKey() (account.Key, bool) {
	for _, k := range a.Keys {
		if k.Algorithm == account.KeyEd25519 && k.KeyID == account.KeyIRI(a.Account.URI, account.KeyEd25519) {
			return k, true
		}
	}
	return account.Key{}, false
}

// FindByID returns the actor with an account ID. It fails with ErrNotFound when there is none.
func (s *Service) FindByID(ctx context.Context, id string) (Actor, error) {
	a, err := s.d.Repository.FindAccount(ctx, id)
	if err != nil {
		return Actor{}, err
	}
	return s.actor(ctx, a)
}

// ExistingIDs returns, in ID order, the accounts among ids that exist, in one lookup.
func (s *Service) ExistingIDs(ctx context.Context, ids []string) ([]string, error) {
	return s.d.Repository.ExistingAccountIDs(ctx, ids)
}

// RemoteInbox is where a remote actor receives activities, from the newest snapshot of its
// document.
type RemoteInbox struct {
	InboxURL string
	// SharedInboxURL is "" when the actor declares no shared inbox.
	SharedInboxURL string
}

// RemoteInbox returns the inboxes of a remote actor, for delivery. It fails with ErrNotFound
// for a local account and for a remote one that has never been fetched.
func (s *Service) RemoteInbox(ctx context.Context, accountID string) (RemoteInbox, error) {
	h, err := s.d.Repository.FindRemoteHead(ctx, accountID)
	if err != nil {
		return RemoteInbox{}, err
	}
	return RemoteInbox{InboxURL: h.InboxURL, SharedInboxURL: h.SharedInboxURL}, nil
}

// KnownSharedInboxes returns at most limit distinct shared inboxes that remote actors not
// deleted declare in their newest documents, in order, after the shared inbox after ("" for
// the first batch). A local account's deletion reaches every known server through them
// (data-model §8).
func (s *Service) KnownSharedInboxes(ctx context.Context, after string, limit int) ([]string, error) {
	return s.d.Repository.ListKnownSharedInboxes(ctx, after, limit)
}

// CountKnownSharedInboxes counts the shared inboxes of KnownSharedInboxes.
func (s *Service) CountKnownSharedInboxes(ctx context.Context) (int, error) {
	return s.d.Repository.CountKnownSharedInboxes(ctx)
}

// AccountDomain returns the domain of an account (account.LocalDomain for a local one). It
// fails with ErrNotFound when there is none.
func (s *Service) AccountDomain(ctx context.Context, id string) (string, error) {
	a, err := s.d.Repository.FindAccount(ctx, id)
	if err != nil {
		return "", err
	}
	return a.Domain, nil
}

// FindByURI returns the actor with an ActivityPub id, local or remote. It fails with
// ErrNotFound when there is none.
func (s *Service) FindByURI(ctx context.Context, uri string) (Actor, error) {
	a, err := s.d.Repository.FindAccountByURI(ctx, uri)
	if err != nil {
		return Actor{}, err
	}
	return s.actor(ctx, a)
}

// AnyOnDomain reports whether one of the accounts ids is on domain, for judging what the
// servers of a domain may see (docs/architecture/visibility.md §1).
func (s *Service) AnyOnDomain(ctx context.Context, domain string, ids []string) (bool, error) {
	return s.d.Repository.AnyAccountOnDomain(ctx, domain, ids)
}

// AccountsOnDomain returns inside tx, in ID order, the accounts among ids that are on domain or
// one of its subdomains, for the follows that a domain suspension cuts (FR-717).
func (s *Service) AccountsOnDomain(ctx context.Context, tx uow.Tx, domain string, ids []string) ([]string, error) {
	if domain == account.LocalDomain || len(ids) == 0 {
		return nil, nil
	}
	return s.d.Repository.AccountsOnDomain(ctx, tx, domain, ids)
}

// LocalAccountIDs returns up to limit local account IDs in order, after the ID after ("" for the
// first page), the instance actor included.
func (s *Service) LocalAccountIDs(ctx context.Context, after string, limit int) ([]string, error) {
	return s.d.Repository.ListLocalAccountIDs(ctx, after, limit)
}

// IsLocal reports, inside tx, whether an account exists and is local. Projectors of other
// modules call it in the transaction of the event they project.
func (s *Service) IsLocal(ctx context.Context, tx uow.Tx, id string) (bool, error) {
	a, err := s.d.Repository.Account(ctx, tx, id)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return a.IsLocal(), nil
}

// DomainOf returns, inside tx, the domain of a remote account: account.LocalDomain ("") for a
// local account or one that does not exist.
func (s *Service) DomainOf(ctx context.Context, tx uow.Tx, id string) (string, error) {
	a, err := s.d.Repository.Account(ctx, tx, id)
	if errors.Is(err, ErrNotFound) {
		return account.LocalDomain, nil
	}
	if err != nil {
		return "", err
	}
	return a.Domain, nil
}

// FindByHandle returns the actor with a handle. username compares case-insensitively; domain is
// account.LocalDomain for local accounts. It fails with ErrNotFound when there is none.
func (s *Service) FindByHandle(ctx context.Context, username, domain string) (Actor, error) {
	h, err := s.d.Repository.FindHandle(ctx, account.NormalizeUsername(username), account.NormalizeDomain(domain))
	if err != nil {
		return Actor{}, err
	}
	return s.FindByID(ctx, h.AccountID)
}

// InstanceActor returns the instance actor. It fails with ErrNotFound before EnsureInstanceActor.
func (s *Service) InstanceActor(ctx context.Context) (Actor, error) {
	a, err := s.d.Repository.FindInstanceActor(ctx)
	if err != nil {
		return Actor{}, err
	}
	return s.actor(ctx, a)
}

func (s *Service) actor(ctx context.Context, a account.Account) (Actor, error) {
	return actorOf(ctx, s.d.Repository, a)
}

// actorReads are the reads of an actor's parts: those of the repository outside transactions,
// or those of txReads in a transaction.
type actorReads interface {
	FindLocalAccount(ctx context.Context, accountID string) (account.LocalAccount, error)
	FindHandleOf(ctx context.Context, accountID string) (account.Handle, error)
	FindCurrentProfile(ctx context.Context, accountID string) (account.ProfileRevision, error)
	ListKeys(ctx context.Context, accountID string) ([]account.Key, error)
	FindLifecycleState(ctx context.Context, accountID string) (account.LifecycleState, error)
}

// txReads reads an actor's parts in a transaction.
type txReads struct {
	repo Repository
	tx   uow.Tx
}

func (r txReads) FindLocalAccount(ctx context.Context, accountID string) (account.LocalAccount, error) {
	return r.repo.FindLocalAccountTx(ctx, r.tx, accountID)
}

func (r txReads) FindHandleOf(ctx context.Context, accountID string) (account.Handle, error) {
	return r.repo.FindHandleOfTx(ctx, r.tx, accountID)
}

func (r txReads) FindCurrentProfile(ctx context.Context, accountID string) (account.ProfileRevision, error) {
	return r.repo.FindCurrentProfileTx(ctx, r.tx, accountID)
}

func (r txReads) ListKeys(ctx context.Context, accountID string) ([]account.Key, error) {
	return r.repo.ListKeysTx(ctx, r.tx, accountID)
}

func (r txReads) FindLifecycleState(ctx context.Context, accountID string) (account.LifecycleState, error) {
	return r.repo.FindLifecycleStateTx(ctx, r.tx, accountID)
}

// FindByIDTx returns an account with its parts as FindByID does, read in tx with what tx has
// written (the data of the webhook messages, ADR-0060).
func (s *Service) FindByIDTx(ctx context.Context, tx uow.Tx, id string) (Actor, error) {
	a, err := s.d.Repository.FindAccountTx(ctx, tx, id)
	if err != nil {
		return Actor{}, err
	}
	return actorOf(ctx, txReads{repo: s.d.Repository, tx: tx}, a)
}

// LocalAccountOfUserTx returns the local account of a user as LocalAccountOfUser does, read in tx.
func (s *Service) LocalAccountOfUserTx(ctx context.Context, tx uow.Tx, userID string) (account.LocalAccount, error) {
	return s.d.Repository.FindLocalAccountByUserTx(ctx, tx, userID)
}

// LocalAccountIDOfUserTx returns the ID of the local account of a user, read in tx; ErrNotFound
// without one.
func (s *Service) LocalAccountIDOfUserTx(ctx context.Context, tx uow.Tx, userID string) (string, error) {
	la, err := s.d.Repository.FindLocalAccountByUserTx(ctx, tx, userID)
	if err != nil {
		return "", err
	}
	return la.AccountID, nil
}

// RemoteProfileURLsTx returns the profile pages of remote accounts as RemoteProfileURLs does,
// read in tx.
func (s *Service) RemoteProfileURLsTx(ctx context.Context, tx uow.Tx, accountIDs []string) (map[string]string, error) {
	if len(accountIDs) == 0 {
		return map[string]string{}, nil
	}
	return s.d.Repository.ListRemoteProfileURLsTx(ctx, tx, accountIDs)
}

func actorOf(ctx context.Context, repo actorReads, a account.Account) (Actor, error) {
	out := Actor{Account: a, Lifecycle: account.InitialLifecycleState(a.ID)}
	if a.IsLocal() && a.ActorType != account.ActorApplication {
		la, err := repo.FindLocalAccount(ctx, a.ID)
		if err != nil {
			return Actor{}, err
		}
		out.Local = &la
	}
	h, err := repo.FindHandleOf(ctx, a.ID)
	switch {
	case err == nil:
		out.Handle = &h
	case !errors.Is(err, ErrNotFound):
		return Actor{}, err
	}
	p, err := repo.FindCurrentProfile(ctx, a.ID)
	switch {
	case err == nil:
		out.Profile = &p
	case !errors.Is(err, ErrNotFound):
		return Actor{}, err
	}
	if out.Keys, err = repo.ListKeys(ctx, a.ID); err != nil {
		return Actor{}, err
	}
	st, err := repo.FindLifecycleState(ctx, a.ID)
	switch {
	case err == nil:
		out.Lifecycle = st
	case !errors.Is(err, ErrNotFound):
		return Actor{}, err
	}
	return out, nil
}

// UserLifecycle returns the lifecycle state of the local account a user logs in as, for
// sign-in (an account without lifecycle events is active). It fails with ErrNotFound when the
// user has no local account.
func (s *Service) UserLifecycle(ctx context.Context, userID string) (account.LifecycleState, error) {
	la, err := s.d.Repository.FindLocalAccountByUser(ctx, userID)
	if err != nil {
		return account.LifecycleState{}, err
	}
	st, err := s.d.Repository.FindLifecycleState(ctx, la.AccountID)
	if errors.Is(err, ErrNotFound) {
		return account.InitialLifecycleState(la.AccountID), nil
	}
	return st, err
}

// CountLocalUsers returns the number of local accounts of users that are not deleted, for
// NodeInfo's usage statistics. The instance actor is not counted.
func (s *Service) CountLocalUsers(ctx context.Context) (int, error) {
	return s.d.Repository.CountLocalAccounts(ctx)
}

// PrivateKey opens the sealed private key of a local actor's key.
func (s *Service) PrivateKey(ctx context.Context, k account.Key) (crypto.Signer, error) {
	sealed, err := s.d.Repository.FindSealedPrivateKey(ctx, k.ID)
	if err != nil {
		return nil, err
	}
	der, err := s.d.Sealer.Open(ctx, sealed, PrivateKeyAAD(k.ID))
	if err != nil {
		return nil, fmt.Errorf("account: open private key %s: %w", k.KeyID, err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("account: parse private key %s: %w", k.KeyID, err)
	}
	signer, ok := parsed.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("account: private key %s is a %T", k.KeyID, parsed)
	}
	if err := checkAlgorithm(k.Algorithm, signer); err != nil {
		return nil, err
	}
	return signer, nil
}

// SigningKey returns the key ID and the opened private key that a local actor signs outbound
// requests with: its newest RSA key ({actor}#main-key), which every implementation verifies
// (ADR-0006, ADR-0032). It fails with ErrNotFound when the account does not exist or has no
// RSA key, as a remote account has none.
func (s *Service) SigningKey(ctx context.Context, accountID string) (keyID string, key crypto.Signer, err error) {
	keys, err := s.d.Repository.ListKeys(ctx, accountID)
	if err != nil {
		return "", nil, err
	}
	var newest *account.Key
	for i := range keys {
		k := &keys[i]
		if k.Algorithm == account.KeyRSASHA256 && (newest == nil || k.CreatedAt.After(newest.CreatedAt)) {
			newest = k
		}
	}
	if newest == nil {
		return "", nil, fmt.Errorf("account: signing key of %s: %w", accountID, ErrNotFound)
	}
	signer, err := s.PrivateKey(ctx, *newest)
	if err != nil {
		return "", nil, err
	}
	return newest.KeyID, signer, nil
}

// ProofKey returns the key ID and the opened private key that a local actor signs the integrity
// proofs of its activities with (FEP-8b32): its newest Ed25519 key ({actor}#ed25519-key). It
// fails with ErrNotFound when the account does not exist or has no Ed25519 key.
func (s *Service) ProofKey(ctx context.Context, accountID string) (keyID string, key ed25519.PrivateKey, err error) {
	keys, err := s.d.Repository.ListKeys(ctx, accountID)
	if err != nil {
		return "", nil, err
	}
	var newest *account.Key
	for i := range keys {
		k := &keys[i]
		if k.Algorithm == account.KeyEd25519 && (newest == nil || k.CreatedAt.After(newest.CreatedAt)) {
			newest = k
		}
	}
	if newest == nil {
		return "", nil, fmt.Errorf("account: proof key of %s: %w", accountID, ErrNotFound)
	}
	signer, err := s.PrivateKey(ctx, *newest)
	if err != nil {
		return "", nil, err
	}
	priv, ok := signer.(ed25519.PrivateKey)
	if !ok {
		return "", nil, fmt.Errorf("account: proof key %s is a %T", newest.KeyID, signer)
	}
	return newest.KeyID, priv, nil
}

// ValidateUsername checks a new local username (account.ValidateUsername), so that other
// modules can reject one before doing expensive work.
func ValidateUsername(username string) error { return account.ValidateUsername(username) }

// LocalAccountOfUser returns the local account a user logs in as. It fails with ErrNotFound
// when the user has none.
func (s *Service) LocalAccountOfUser(ctx context.Context, userID string) (account.LocalAccount, error) {
	return s.d.Repository.FindLocalAccountByUser(ctx, userID)
}
