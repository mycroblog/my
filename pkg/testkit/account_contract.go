// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
)

// AccountHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the identity and account repositories on it.
type AccountHarness struct {
	UnitOfWork uow.UnitOfWork
	Identity   appidentity.Repository
	Accounts   appaccount.Repository
}

// AccountFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type AccountFactory func(t *testing.T, opts uow.Options) AccountHarness

// AccountRepositoryContract verifies the identity and account repositories of a database
// adapter through the use cases that drive them: registering a local user with sealed RSA and
// Ed25519 keys, rejecting a taken username, creating the instance actor exactly once, lookups by
// handle and ID, profile revisions, lifecycle events, the deletion of an account at its owner's
// request, the data exports with their archives, expiry, and erasure, the CSV imports carried out by their jobs, the built-in roles, creating an admin with
// an email address and a role grant while rejecting a taken username or email, signing in and out
// with browser sessions (new tokens on every sign-in, expiry, activity, the same error for an
// unknown address, a wrong password, and an inactive account, rate-limited failures, rehashing weaker passwords,
// the origin of each sign-in and its purge after the configured retention, except for held
// users), two-factor authentication with TOTP (enrolling from a sealed setup token, a second
// sign-in step that takes a code once or a recovery code once, rate-limited wrong codes,
// regenerating recovery codes, and disabling), passkeys (registering after the password with the
// earlier ones excluded, listing, removing, a sign-in challenge that works once, the code of TOTP
// after a passkey, rate-limited failures, and the limits on registrations), signing in without a
// password (confirming operations with a passkey once, removing the password while a passkey
// stays, keeping the last passkey while there is no password, and a reset link that sets one
// again), resetting a
// forgotten password with an emailed one-time link and changing it with the current one (the
// answer that does not tell whether an address has an account, links that expire, work once,
// and stop working when the password changes, limited requests, and the end of the other
// sessions and grants while passkeys and TOTP stay), the OAuth 2.0
// authorization server (registering the web client, exact redirect URIs, PKCE S256 only,
// one-time codes, refresh token rotation that revokes the whole family on reuse, revocation,
// disabled and deleted clients refused with their token families revoked, the clients users
// register with their revisions, rotated secrets stored as digests, the limit per user, tokens
// issued to their owner, and clients that stop working with their owner),
// and the property that the account module's synchronous projections equal a rebuild from their events.
func AccountRepositoryContract(t *testing.T, newHarness AccountFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *accountEnv)
	}{
		{"RegistersALocalUser", accountRegisters},
		{"RejectsATakenUsername", accountRejectsTakenUsername},
		{"RejectsInvalidInput", accountRejectsInvalidInput},
		{"CreatesTheInstanceActorOnce", accountCreatesInstanceActorOnce},
		{"ReportsMissingAccounts", accountReportsMissing},
		{"RevisesProfiles", accountRevisesProfiles},
		{"UpdatesLocalProfiles", accountUpdatesLocalProfiles},
		{"RecordsLifecycleEvents", accountRecordsLifecycle},
		{"ExecutesScheduledDeletions", accountExecutesDeletions},
		{"DeletesAccountsAtTheOwnersRequest", accountDeletesAtTheOwnersRequest},
		{"BuildsAndExpiresDataExports", exportsBuildAndExpireArchives},
		{"ErasesDataExportsWithTheAccount", exportsAreErasedWithTheAccount},
		{"CarriesOutCSVImports", importsCarryOutCSVFiles},
		{"ErasesCSVImportsWithTheAccount", importsAreErasedWithTheAccount},
		{"CountsLocalUsers", accountCountsLocalUsers},
		{"ReadsRemoteInboxes", accountReadsRemoteInboxes},
		{"FindsAccountsByURIAndDomain", accountFindsByURIAndDomain},
		{"EnsuresBuiltinRoles", accountEnsuresBuiltinRoles},
		{"CreatesAnAdmin", accountCreatesAdmin},
		{"RejectsATakenAdminUsernameOrEmail", accountRejectsTakenAdmin},
		{"SignsInAndOut", sessionSignsInAndOut},
		{"ReplacesTheSessionOnSignIn", sessionReplacedOnSignIn},
		{"ExpiresSessions", sessionExpires},
		{"RejectsWrongCredentialsAlike", sessionRejectsWrongCredentials},
		{"RateLimitsFailedSignIns", sessionRateLimitsFailures},
		{"RehashesWeakerPasswordsOnSignIn", sessionRehashes},
		{"RejectsSignInToInactiveAccounts", sessionRejectsInactiveAccounts},
		{"RecordsSignInOrigins", originRecordsSignIns},
		{"PurgesSessionOriginsAfterTheRetention", originPurgesSessionOrigins},
		{"OriginPurgeFollowsTheRetentionPolicy", originPurgeFollowsThePolicy},
		{"EndsTheLoginsOfModeratedAccounts", sessionModerationEndsLogins},
		{"SignsInBannedAccountsUntilTheirDeletion", sessionBannedSignsInUntilDeletion},
		{"EnrollsTOTPAndSignsInWithACode", totpEnrollsAndSignsIn},
		{"TOTPRecoveryCodesWorkOnce", totpRecoveryCodesWorkOnce},
		{"RateLimitsWrongTOTPCodes", totpRateLimitsWrongCodes},
		{"RegeneratesRecoveryCodesAndDisablesTOTP", totpRegeneratesAndDisables},
		{"RegistersListsAndRemovesPasskeys", passkeyRegistersListsAndRemoves},
		{"LimitsPasskeyRegistrations", passkeyLimitsRegistrations},
		{"SignsInWithAPasskey", passkeySignsIn},
		{"RejectsRemovedPasskeysAndInactiveAccounts", passkeyRejectsRemovedAndInactive},
		{"RateLimitsFailedPasskeySignIns", passkeyRateLimitsFailedSignIns},
		{"ConfirmsOperationsWithAPasskey", passwordlessConfirmsWithAPasskey},
		{"RemovesThePasswordOfPasskeyUsers", passwordlessRemovesThePassword},
		{"ResetsPasswordsWithOneTimeLinks", passwordResetWorksOnce},
		{"LimitsPasswordResetRequests", passwordResetLimitsRequests},
		{"SendsPasswordResetEmailsFromJobs", passwordResetEmailJobs},
		{"SendsVerificationEmailsFromJobs", verificationEmailJobs},
		{"StopsEmailsAfterAHardBounce", emailHardBounceStopsMail},
		{"StopsNotificationEmailsAfterAComplaint", emailComplaintStopsNotifications},
		{"EmailSuppressionsEqualARebuild", emailSuppressionEqualsRebuild},
		{"TurnsOffNotificationEmailsWithUnsubscribeLinks", emailUnsubscribeTurnsOffNotifications},
		{"ChangesPasswordsKeepingTheCurrentGrant", passwordChangeKeepsTheCurrentGrant},
		{"RegistersTheWebClient", oauthRegistersTheWebClient},
		{"ValidatesAuthorizationRequests", oauthValidatesAuthorizationRequests},
		{"ExchangesCodesWithPKCE", oauthExchangesCodesWithPKCE},
		{"AuthorizationCodesWorkOnce", oauthCodesWorkOnce},
		{"RotatesRefreshTokensAndRevokesReusedFamilies", oauthRotatesRefreshTokens},
		{"ExpiresRefreshTokens", oauthRefreshTokensExpire},
		{"RefreshesThirdPartyTokens", oauthRefreshesThirdPartyTokens},
		{"RevokesTokens", oauthRevokesTokens},
		{"DisablesAndDeletesClients", oauthDisablesAndDeletesClients},
		{"RegistersAndChangesUsersOAuthApps", oauthAppsRegisterAndChange},
		{"IssuesTokensOfOAuthAppsToTheirOwner", oauthAppsIssueTokensToTheirOwner},
		{"LimitsOAuthAppRegistrations", oauthAppsLimitRegistrations},
		{"StopsOAuthAppsWithTheirOwner", oauthAppsStopWithTheirOwner},
		{"RegistersDynamicClients", oauthRegistersDynamicClients},
		{"DeletesUnusedDynamicClients", oauthDeletesUnusedDynamicClients},
		{"UsesClientMetadataDocuments", oauthUsesClientMetadataDocuments},
		{"EndsTheBrowserSessionOnWebSignOut", oauthWebSignOutEndsTheBrowserSession},
		{"IncrementalProjectionEqualsRebuild", accountProjectionEqualsRebuild},
		{"ReadsAnAccountInATransaction", accountReadsInTransaction},
		{"ReadsARegistrationInATransaction", registrationReadsInTransaction},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newAccountEnv(t, newHarness)) })
	}
}

// accountEnv wires the use cases onto one harness with deterministic time and IDs.
type accountEnv struct {
	h          AccountHarness
	projectors *uow.Projectors
	clock      *Clock
	ids        *IDGenerator
	accounts   *appaccount.Service
	identity   *appidentity.Service
	hasher     *appidentity.PasswordHasher
	kv         *KeyValueStore
	limiter    *kernel.RateLimiter
	sealer     *kernel.Sealer
	// mailJobs runs the email jobs of the identity service that newIdentityWith made last.
	mailJobs *InlineJobs
}

const contractHost = "social.example"

// cheapArgon2 keeps the contract fast; the parameters do not affect what is stored.
var cheapArgon2 = appidentity.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

// newAccountEnv returns an env on a new harness; register adds projectors after the account
// module's.
func newAccountEnv(t *testing.T, newHarness AccountFactory, register ...func(*uow.Projectors)) *accountEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	return newAccountEnvOn(h, projectors, register...)
}

// newAccountEnvOn wires the use cases onto a harness whose UnitOfWork runs projectors;
// register adds projectors after the account module's.
func newAccountEnvOn(h AccountHarness, projectors *uow.Projectors, register ...func(*uow.Projectors)) *accountEnv {
	appaccount.RegisterProjectors(projectors, h.Accounts)
	for _, r := range register {
		r(projectors)
	}
	clock := NewClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	ids := NewIDGenerator(clock)
	accounts := appaccount.NewService(appaccount.Deps{
		UnitOfWork:  h.UnitOfWork,
		Repository:  h.Accounts,
		Clock:       clock,
		IDs:         ids,
		Sealer:      kernel.NewSealer(NewKeyManager("contract")),
		Host:        contractHost,
		GenerateKey: cachedKey,
		RenderProfileText: func(_ context.Context, text string) (string, error) {
			return "<p>rendered " + text + "</p>", nil
		},
		CheckProfileImage: func(_ context.Context, _, mediaID string, _ account.ProfileProperty) error {
			if mediaID != contractProfileImage {
				return appaccount.ErrUnusableImage
			}
			return nil
		},
	})
	kv := NewKeyValueStore(clock)
	e := &accountEnv{
		h:          h,
		projectors: projectors,
		clock:      clock,
		ids:        ids,
		accounts:   accounts,
		hasher:     appidentity.NewPasswordHasher(cheapArgon2, 0),
		kv:         kv,
		limiter:    kernel.NewRateLimiter(kv, clock),
		sealer:     kernel.NewSealer(NewKeyManager("contract")),
	}
	e.identity = e.newIdentity(e.hasher)
	return e
}

// newIdentity returns an identity service on the env's harness that hashes with hasher.
func (e *accountEnv) newIdentity(hasher *appidentity.PasswordHasher) *appidentity.Service {
	return e.newIdentityWith(func(d *appidentity.Deps) { d.Passwords = hasher })
}

// newIdentityWith returns an identity service on the env's harness with the deps that change
// sets. The jobs of its emails run right after the commits that enqueue them.
func (e *accountEnv) newIdentityWith(change func(*appidentity.Deps)) *appidentity.Service {
	d := appidentity.Deps{
		UnitOfWork:  e.h.UnitOfWork,
		Repository:  e.h.Identity,
		Clock:       e.clock,
		IDs:         e.ids,
		Passwords:   e.hasher,
		Accounts:    e.accounts,
		RateLimiter: e.limiter,
		Sealer:      e.sealer,
		KV:          e.kv,
		Passkeys:    FakePasskeys{},
		WebClient:   contractWebClient,
		BaseURL:     "https://" + contractHost,
	}
	outbox, jobs := inlineOutbox(e.projectors)
	d.Outbox, e.mailJobs = outbox, jobs
	change(&d)
	s := appidentity.NewService(d)
	// The emails of the use cases are sent right after their commit.
	s.RegisterMailJobs(jobs.Registry)
	return s
}

// cachedKey generates one key per algorithm per process: RSA generation is slow, and the
// contract does not need distinct keys.
func cachedKey(alg account.KeyAlgorithm) (crypto.Signer, error) {
	keyCache.once.Do(func() {
		keyCache.keys = map[account.KeyAlgorithm]crypto.Signer{}
		for _, a := range appaccount.LocalKeyAlgorithms {
			k, err := appaccount.GenerateKey(a)
			if err != nil {
				keyCache.err = err
				return
			}
			keyCache.keys[a] = k
		}
	})
	if keyCache.err != nil {
		return nil, keyCache.err
	}
	return keyCache.keys[alg], nil
}

var keyCache struct {
	once sync.Once
	keys map[account.KeyAlgorithm]crypto.Signer
	err  error
}

func (e *accountEnv) register(t *testing.T, username string) appidentity.Registration {
	t.Helper()
	e.clock.Advance(time.Second)
	reg, err := e.identity.RegisterLocalUser(t.Context(), appidentity.RegisterInput{
		Username: username, Password: "correct horse battery staple", DisplayName: "Display " + username,
	})
	if err != nil {
		t.Fatalf("RegisterLocalUser(%q) = %v", username, err)
	}
	return reg
}

func accountRegisters(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.register(t, "Alice")

	user, err := e.h.Identity.FindUser(ctx, reg.User.ID)
	if err != nil || !user.CreatedAt.Equal(reg.User.CreatedAt) {
		t.Fatalf("FindUser = %+v, %v; want %+v", user, err, reg.User)
	}
	cred, err := e.h.Identity.FindCurrentPasswordCredential(ctx, reg.User.ID)
	if err != nil {
		t.Fatalf("FindCurrentPasswordCredential = %v", err)
	}
	if ok, _, err := e.hasher.Verify(ctx, "correct horse battery staple", cred.Hash); !ok || err != nil {
		t.Errorf("stored hash %q does not verify the password: %v, %v", cred.Hash, ok, err)
	}

	for _, name := range []string{"alice", "ALICE", "Alice"} {
		got, err := e.accounts.FindByHandle(ctx, name, account.LocalDomain)
		if err != nil || got.Account.ID != reg.AccountID {
			t.Fatalf("FindByHandle(%q) = %+v, %v; want account %s", name, got.Account, err, reg.AccountID)
		}
	}
	actor, err := e.accounts.FindByID(ctx, reg.AccountID)
	if err != nil {
		t.Fatalf("FindByID = %v", err)
	}
	a := actor.Account
	if a.URI != "https://"+contractHost+"/ap/actors/"+a.ID || a.Domain != account.LocalDomain || a.ActorType != account.ActorPerson {
		t.Errorf("account = %+v, want a local person at /ap/actors/{id}", a)
	}
	if actor.Local == nil || actor.Local.UserID != reg.User.ID || actor.Local.Username != "Alice" || actor.Local.UsernameNormalized != "alice" {
		t.Errorf("local account = %+v, want user %s and username Alice", actor.Local, reg.User.ID)
	}
	if actor.Handle == nil || *actor.Handle != (account.Handle{AccountID: a.ID, Username: "Alice", UsernameNormalized: "alice", Domain: account.LocalDomain}) {
		t.Errorf("handle = %+v", actor.Handle)
	}
	if actor.Profile == nil || actor.Profile.Seq != 1 || actor.Profile.DisplayName != "Display Alice" {
		t.Errorf("profile = %+v, want revision 1 with the display name", actor.Profile)
	}
	if actor.Lifecycle.State != account.StateActive {
		t.Errorf("lifecycle = %+v, want active", actor.Lifecycle)
	}
	checkKeys(t, e, actor)
}

// checkKeys verifies that an actor has an RSA main key and an Ed25519 key whose sealed private
// halves open to the stored public keys.
func checkKeys(t *testing.T, e *accountEnv, actor appaccount.Actor) {
	t.Helper()
	if len(actor.Keys) != 2 {
		t.Fatalf("keys = %+v, want an RSA and an Ed25519 key", actor.Keys)
	}
	seen := map[account.KeyAlgorithm]bool{}
	for _, k := range actor.Keys {
		seen[k.Algorithm] = true
		if k.KeyID != account.KeyIRI(actor.Account.URI, k.Algorithm) || k.AccountID != actor.Account.ID {
			t.Errorf("key %+v: wrong key ID or account", k)
		}
		pub, err := appaccount.ParsePublicKeyPEM(k.PublicKeyPEM)
		if err != nil {
			t.Fatalf("parse public key %s: %v", k.KeyID, err)
		}
		priv, err := e.accounts.PrivateKey(t.Context(), k)
		if err != nil {
			t.Fatalf("PrivateKey(%s) = %v", k.KeyID, err)
		}
		switch p := priv.(type) {
		case *rsa.PrivateKey:
			if k.Algorithm != account.KeyRSASHA256 || p.N.BitLen() != appaccount.RSAKeyBits || !p.PublicKey.Equal(pub) {
				t.Errorf("key %s: RSA private key does not match", k.KeyID)
			}
		case ed25519.PrivateKey:
			if k.Algorithm != account.KeyEd25519 || !p.Public().(ed25519.PublicKey).Equal(pub) {
				t.Errorf("key %s: Ed25519 private key does not match", k.KeyID)
			}
		default:
			t.Errorf("key %s: private key is a %T", k.KeyID, priv)
		}
	}
	if !seen[account.KeyRSASHA256] || !seen[account.KeyEd25519] {
		t.Errorf("key algorithms = %v, want rsa_sha256 and ed25519", seen)
	}
	// Outbound requests are signed with the RSA key, {actor}#main-key.
	keyID, signer, err := e.accounts.SigningKey(t.Context(), actor.Account.ID)
	if _, rsaKey := signer.(*rsa.PrivateKey); err != nil || keyID != account.KeyIRI(actor.Account.URI, account.KeyRSASHA256) || !rsaKey {
		t.Errorf("SigningKey = %q, %T, %v; want the RSA key", keyID, signer, err)
	}
	if _, _, err := e.accounts.SigningKey(t.Context(), NamedID("no-such-account")); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("SigningKey(unknown) = %v, want ErrNotFound", err)
	}
}

func accountRejectsTakenUsername(t *testing.T, e *accountEnv) {
	e.register(t, "bob")
	_, err := e.identity.RegisterLocalUser(t.Context(), appidentity.RegisterInput{Username: "BOB", Password: "another password"})
	if !errors.Is(err, appaccount.ErrUsernameTaken) {
		t.Fatalf("RegisterLocalUser(BOB) = %v, want ErrUsernameTaken", err)
	}
	handles, err := e.h.Accounts.ListHandles(t.Context())
	if err != nil || len(handles) != 1 {
		t.Fatalf("handles = %+v, %v; want only bob (the failed registration must roll back)", handles, err)
	}
}

func accountRejectsInvalidInput(t *testing.T, e *accountEnv) {
	for _, in := range []appidentity.RegisterInput{
		{Username: "", Password: "long enough"},
		{Username: "bad name", Password: "long enough"},
		{Username: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Password: "long enough"},
	} {
		if _, err := e.identity.RegisterLocalUser(t.Context(), in); !errors.Is(err, account.ErrInvalidUsername) {
			t.Errorf("RegisterLocalUser(%q) = %v, want ErrInvalidUsername", in.Username, err)
		}
	}
	if _, err := e.identity.RegisterLocalUser(t.Context(), appidentity.RegisterInput{Username: "carol", Password: "short"}); !errors.Is(err, identity.ErrInvalidPassword) {
		t.Errorf("RegisterLocalUser with a short password = %v, want ErrInvalidPassword", err)
	}
	if handles, _ := e.h.Accounts.ListHandles(t.Context()); len(handles) != 0 {
		t.Errorf("handles = %+v after invalid input, want none", handles)
	}
}

func accountCreatesInstanceActorOnce(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	if _, err := e.accounts.InstanceActor(ctx); !errors.Is(err, appaccount.ErrNotFound) {
		t.Fatalf("InstanceActor before creation = %v, want ErrNotFound", err)
	}
	const callers = 4
	ids := make(chan string, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			a, err := e.accounts.EnsureInstanceActor(ctx)
			if err != nil {
				t.Errorf("EnsureInstanceActor = %v", err)
			}
			ids <- a.ID
		})
	}
	wg.Wait()
	close(ids)
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("EnsureInstanceActor returned %s and %s, want one actor", first, id)
		}
	}
	again, err := e.accounts.EnsureInstanceActor(ctx)
	if err != nil || again.ID != first {
		t.Fatalf("EnsureInstanceActor again = %s, %v; want %s", again.ID, err, first)
	}
	actor, err := e.accounts.InstanceActor(ctx)
	if err != nil {
		t.Fatalf("InstanceActor = %v", err)
	}
	if actor.Account.ID != first || actor.Account.ActorType != account.ActorApplication || !actor.Account.IsLocal() {
		t.Errorf("instance actor = %+v, want the local Application actor %s", actor.Account, first)
	}
	if actor.Local != nil || actor.Handle != nil {
		t.Errorf("instance actor has local account %+v and handle %+v, want neither", actor.Local, actor.Handle)
	}
	checkKeys(t, e, actor)
}

func accountReportsMissing(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	missing := "00000000-0000-7000-8000-000000000000"
	if _, err := e.accounts.FindByID(ctx, missing); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindByID(missing) = %v, want ErrNotFound", err)
	}
	if _, err := e.accounts.FindByHandle(ctx, "nobody", account.LocalDomain); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindByHandle(nobody) = %v, want ErrNotFound", err)
	}
	if _, err := e.h.Identity.FindUser(ctx, missing); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindUser(missing) = %v, want ErrNotFound", err)
	}
	if _, err := e.h.Identity.FindCurrentPasswordCredential(ctx, missing); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindCurrentPasswordCredential(missing) = %v, want ErrNotFound", err)
	}
	if _, _, err := e.h.Identity.FindRole(ctx, "nope"); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindRole(nope) = %v, want ErrNotFound", err)
	}
	// ExistingIDs keeps the accounts that exist, in ID order, in one lookup.
	reg := e.register(t, "erin")
	got, err := e.accounts.ExistingIDs(ctx, []string{reg.AccountID, missing, reg.AccountID})
	if err != nil || !reflect.DeepEqual(got, []string{reg.AccountID}) {
		t.Errorf("ExistingIDs = %v, %v; want [%s]", got, err, reg.AccountID)
	}
	if got, err := e.accounts.ExistingIDs(ctx, nil); err != nil || len(got) != 0 {
		t.Errorf("ExistingIDs(nil) = %v, %v; want none", got, err)
	}
}

func accountRevisesProfiles(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.register(t, "dave")
	in := appaccount.ProfileInput{
		DisplayName: "Dave", NoteHTML: "<p>hi</p>", Locked: true, Bot: true, Discoverable: true, Indexable: true, AIOptOut: true,
		AvatarMediaID: "0192f3a4-0000-7000-8000-000000000001",
		Fields:        []account.ProfileField{{Name: "web", ValueHTML: "<a>x</a>"}, {Name: "pronouns", ValueHTML: "he/him"}},
	}
	e.clock.Advance(time.Second)
	rev, err := e.accounts.ReviseProfile(ctx, reg.AccountID, in)
	if err != nil || rev.Seq != 2 {
		t.Fatalf("ReviseProfile = %+v, %v; want seq 2", rev, err)
	}
	actor, err := e.accounts.FindByID(ctx, reg.AccountID)
	if err != nil || actor.Profile == nil {
		t.Fatalf("FindByID = %+v, %v", actor, err)
	}
	got := *actor.Profile
	if !got.RevisedAt.Equal(rev.RevisedAt) {
		t.Errorf("RevisedAt = %v, want %v", got.RevisedAt, rev.RevisedAt)
	}
	got.RevisedAt = rev.RevisedAt
	if !reflect.DeepEqual(got, rev) {
		t.Errorf("current profile = %+v, want %+v", got, rev)
	}
	if got.Fields[1].Position != 1 {
		t.Errorf("field positions = %+v, want 0 and 1", got.Fields)
	}
	header := "0192f3a4-0000-7000-8000-000000000002"
	e.clock.Advance(time.Second)
	if _, err := e.accounts.ReviseProfile(ctx, reg.AccountID, appaccount.ProfileInput{DisplayName: "Dave", HeaderMediaID: header}); err != nil {
		t.Fatalf("ReviseProfile: %v", err)
	}
	// Media files stay referenced by revisions that are no longer current.
	for id, want := range map[string]bool{in.AvatarMediaID: true, header: true, "0192f3a4-0000-7000-8000-000000000003": false} {
		if got, err := e.accounts.IsMediaReferenced(ctx, id); err != nil || got != want {
			t.Errorf("IsMediaReferenced(%s) = %v, %v; want %v", id, got, err, want)
		}
	}

	// Concurrent revisions each take the next seq; none is lost.
	const writers = 8
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			if _, err := e.accounts.ReviseProfile(ctx, reg.AccountID, appaccount.ProfileInput{DisplayName: "concurrent"}); err != nil {
				t.Errorf("concurrent ReviseProfile = %v", err)
			}
		})
	}
	wg.Wait()
	if actor, err := e.accounts.FindByID(ctx, reg.AccountID); err != nil || actor.Profile == nil || actor.Profile.Seq != 3+writers {
		t.Errorf("after %d concurrent revisions: profile = %+v, %v; want seq %d", writers, actor.Profile, err, 3+writers)
	}
}

// contractProfileImage is the only media file that the contract's accounts may use as an avatar
// or header.
const contractProfileImage = "0192f3a4-0000-7000-8000-0000000000aa"

// accountUpdatesLocalProfiles checks UpdateProfile: every update is a new revision holding the
// typed text as its source, properties left out keep their values, and invalid content and
// unusable images change nothing.
func accountUpdatesLocalProfiles(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.register(t, "fern")
	ptr := func(s string) *string { return &s }
	yes := true
	fields := []account.FieldSource{{Name: "web", Value: "https://fern.example"}, {Name: "pronouns", Value: "she/her"}}
	e.clock.Advance(time.Second)
	got, err := e.accounts.UpdateProfile(ctx, reg.AccountID, appaccount.ProfileUpdate{
		DisplayName: ptr("Fern"), Note: ptr("hello\nworld"), Fields: &fields, AvatarMediaID: ptr(contractProfileImage), Locked: &yes,
	})
	if err != nil {
		t.Fatalf("UpdateProfile = %v", err)
	}
	want := account.ProfileRevision{
		ID: got.Profile.ID, AccountID: reg.AccountID, Seq: 2, DisplayName: "Fern", NoteHTML: "<p>rendered hello\nworld</p>",
		Locked: true, AvatarMediaID: contractProfileImage, RevisedAt: e.clock.Now(),
		Fields: []account.ProfileField{
			{Position: 0, Name: "web", ValueHTML: "<p>rendered https://fern.example</p>"},
			{Position: 1, Name: "pronouns", ValueHTML: "<p>rendered she/her</p>"},
		},
		Source: &account.ProfileSource{Note: "hello\nworld", FieldValues: []string{"https://fern.example", "she/her"}},
	}
	stored, err := e.accounts.FindByID(ctx, reg.AccountID)
	if err != nil || stored.Profile == nil {
		t.Fatalf("FindByID = %+v, %v", stored, err)
	}
	for name, p := range map[string]account.ProfileRevision{"returned": *got.Profile, "stored": *stored.Profile} {
		if !p.RevisedAt.Equal(want.RevisedAt) {
			t.Errorf("%s RevisedAt = %v, want %v", name, p.RevisedAt, want.RevisedAt)
		}
		p.RevisedAt = want.RevisedAt
		if !reflect.DeepEqual(p, want) {
			t.Errorf("%s profile = %+v, want %+v", name, p, want)
		}
	}

	// A partial update keeps the rest, sources included, and removes the avatar with "".
	got, err = e.accounts.UpdateProfile(ctx, reg.AccountID, appaccount.ProfileUpdate{DisplayName: ptr("Fern F."), AvatarMediaID: ptr("")})
	if err != nil {
		t.Fatalf("UpdateProfile(display name) = %v", err)
	}
	if p := got.Profile; p.Seq != 3 || p.DisplayName != "Fern F." || p.NoteHTML != want.NoteHTML || !p.Locked || p.AvatarMediaID != "" ||
		!reflect.DeepEqual(p.Fields, want.Fields) || !reflect.DeepEqual(p.Source, want.Source) {
		t.Errorf("after a partial update: profile = %+v (source %+v), want seq 3 with the rest kept", p, p.Source)
	}
	if stored, err := e.accounts.FindByID(ctx, reg.AccountID); err != nil || !reflect.DeepEqual(stored.Profile.Source, want.Source) {
		t.Errorf("stored source = %+v, %v; want %+v", stored.Profile.Source, err, want.Source)
	}

	tooMany := make([]account.FieldSource, account.MaxProfileFields+1)
	for _, tc := range []struct {
		name string
		in   appaccount.ProfileUpdate
		want account.ProfileError
	}{
		{"long display name", appaccount.ProfileUpdate{DisplayName: ptr(strings.Repeat("あ", account.MaxDisplayNameLength+1))},
			account.ProfileError{Property: account.PropertyDisplayName, Problem: account.ProblemTooLong, Max: account.MaxDisplayNameLength}},
		{"line break in the display name", appaccount.ProfileUpdate{DisplayName: ptr("a\nb")},
			account.ProfileError{Property: account.PropertyDisplayName, Problem: account.ProblemInvalid}},
		{"too many fields", appaccount.ProfileUpdate{Fields: &tooMany},
			account.ProfileError{Property: account.PropertyFields, Problem: account.ProblemTooMany, Max: account.MaxProfileFields}},
		{"another account's image", appaccount.ProfileUpdate{HeaderMediaID: ptr("0192f3a4-0000-7000-8000-0000000000bb")},
			account.ProfileError{Property: account.PropertyHeader, Problem: account.ProblemInvalid}},
	} {
		_, err := e.accounts.UpdateProfile(ctx, reg.AccountID, tc.in)
		var pe *account.ProfileError
		if !errors.As(err, &pe) || *pe != tc.want || !errors.Is(err, account.ErrInvalidProfile) {
			t.Errorf("%s: UpdateProfile = %v, want %+v", tc.name, err, tc.want)
		}
	}
	if stored, err := e.accounts.FindByID(ctx, reg.AccountID); err != nil || stored.Profile.Seq != 3 {
		t.Errorf("after rejected updates: seq = %d, %v; want 3", stored.Profile.Seq, err)
	}

	// The instance actor has no profile to edit.
	ia, err := e.accounts.EnsureInstanceActor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.accounts.UpdateProfile(ctx, ia.ID, appaccount.ProfileUpdate{DisplayName: ptr("x")}); !errors.Is(err, appaccount.ErrNotLocal) {
		t.Errorf("UpdateProfile(instance actor) = %v, want ErrNotLocal", err)
	}
}

func accountRecordsLifecycle(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.register(t, "erin")
	record := func(kind account.LifecycleKind, due *time.Time) account.LifecycleEvent {
		t.Helper()
		e.clock.Advance(time.Second)
		ev, err := e.accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
			AccountID: reg.AccountID, Kind: kind, InitiatedBy: account.InitiatedBySelf, DeletionDueAt: due,
		})
		if err != nil {
			t.Fatalf("RecordLifecycleEvent(%s) = %v", kind, err)
		}
		return ev
	}
	state := func() account.LifecycleState {
		t.Helper()
		actor, err := e.accounts.FindByID(ctx, reg.AccountID)
		if err != nil {
			t.Fatalf("FindByID = %v", err)
		}
		return actor.Lifecycle
	}

	ev := record(account.LifecycleDeactivated, nil)
	if s := state(); s.State != account.StateDeactivated || !s.Since.Equal(ev.OccurredAt) {
		t.Errorf("after deactivated: %+v", s)
	}
	due := e.clock.Now().Add(30 * 24 * time.Hour)
	record(account.LifecycleDeletionScheduled, &due)
	if s := state(); s.State != account.StateDeletionScheduled || s.DeletionDueAt == nil || !s.DeletionDueAt.Equal(due) {
		t.Errorf("after deletion_scheduled: %+v, want due %v", s, due)
	}
	record(account.LifecycleDeletionCancelled, nil)
	if s := state(); s.State != account.StateActive || s.DeletionDueAt != nil {
		t.Errorf("after deletion_cancelled: %+v", s)
	}
	if _, err := e.accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: reg.AccountID, Kind: account.LifecycleDeletionScheduled, InitiatedBy: account.InitiatedBySelf,
	}); err == nil {
		t.Error("RecordLifecycleEvent(deletion_scheduled) without a due time succeeded")
	}
}

func accountCountsLocalUsers(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	count := func() int {
		t.Helper()
		n, err := e.accounts.CountLocalUsers(ctx)
		if err != nil {
			t.Fatalf("CountLocalUsers = %v", err)
		}
		return n
	}
	if n := count(); n != 0 {
		t.Errorf("CountLocalUsers on an empty database = %d, want 0", n)
	}
	if _, err := e.accounts.EnsureInstanceActor(ctx); err != nil {
		t.Fatalf("EnsureInstanceActor = %v", err)
	}
	gone := e.register(t, "gone")
	e.register(t, "kept")
	if _, err := e.accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: gone.AccountID, Kind: account.LifecycleDeleted, InitiatedBy: account.InitiatedBySelf,
	}); err != nil {
		t.Fatalf("RecordLifecycleEvent(deleted) = %v", err)
	}
	if n := count(); n != 1 {
		t.Errorf("CountLocalUsers = %d, want 1 (the instance actor and deleted accounts are not counted)", n)
	}
}

func accountEnsuresBuiltinRoles(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	if err := e.identity.EnsureBuiltinRoles(ctx); err != nil {
		t.Fatalf("EnsureBuiltinRoles = %v", err)
	}
	first := map[string]identity.Role{}
	for _, b := range identity.BuiltinRoles() {
		r, _, err := e.h.Identity.FindRole(ctx, b.Key)
		if err != nil {
			t.Fatalf("FindRole(%s) = %v", b.Key, err)
		}
		first[b.Key] = r
	}
	e.clock.Advance(time.Hour)
	if err := e.identity.EnsureBuiltinRoles(ctx); err != nil {
		t.Fatalf("EnsureBuiltinRoles again = %v", err)
	}
	for _, b := range identity.BuiltinRoles() {
		r, perms, err := e.h.Identity.FindRole(ctx, b.Key)
		if err != nil {
			t.Fatalf("FindRole(%s) = %v", b.Key, err)
		}
		if r.ID != first[b.Key].ID || !r.CreatedAt.Equal(first[b.Key].CreatedAt) || r.Position != b.Position {
			t.Errorf("role %s = %+v after a second run, want %+v", b.Key, r, first[b.Key])
		}
		want := map[identity.Permission]bool{}
		for _, p := range b.Permissions {
			want[p] = true
		}
		got := map[identity.Permission]bool{}
		for _, p := range perms {
			got[p] = true
		}
		if !reflect.DeepEqual(got, want) || len(perms) != len(b.Permissions) {
			t.Errorf("role %s permissions = %v, want %v", b.Key, perms, b.Permissions)
		}
	}

	// An admin role created by an older version lacks the permissions added since; ensuring the
	// built-in roles again, from several processes starting at once, gives it all of them.
	admin := first[identity.RoleAdmin]
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.h.Identity.SetRolePermissions(ctx, tx, admin.ID, identity.AllPermissions()[:1])
	}); err != nil {
		t.Fatalf("SetRolePermissions(admin) = %v", err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Go(func() { errs[i] = e.identity.EnsureBuiltinRoles(ctx) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent EnsureBuiltinRoles %d = %v", i, err)
		}
	}
	_, perms, err := e.h.Identity.FindRole(ctx, identity.RoleAdmin)
	if err != nil {
		t.Fatalf("FindRole(admin) = %v", err)
	}
	if got := slices.Sorted(slices.Values(perms)); !reflect.DeepEqual(got, slices.Sorted(slices.Values(identity.AllPermissions()))) {
		t.Errorf("admin permissions after an upgrade = %v, want %v", perms, identity.AllPermissions())
	}
}

func accountCreatesAdmin(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	e.clock.Advance(time.Second)
	reg, err := e.identity.CreateAdmin(ctx, appidentity.CreateAdminInput{
		Username: "Root", Email: "Root@Example.COM", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("CreateAdmin = %v", err)
	}
	if id, err := e.h.Identity.FindUserIDByEmail(ctx, "root@example.com"); err != nil || id != reg.User.ID {
		t.Errorf("FindUserIDByEmail = %q, %v; want %s", id, err, reg.User.ID)
	}
	cred, err := e.h.Identity.FindCurrentPasswordCredential(ctx, reg.User.ID)
	if err != nil {
		t.Fatalf("FindCurrentPasswordCredential = %v", err)
	}
	if ok, _, err := e.hasher.Verify(ctx, "correct horse battery staple", cred.Hash); !ok || err != nil {
		t.Errorf("stored hash does not verify the password: %v, %v", ok, err)
	}
	roles, err := e.h.Identity.ListUserRoles(ctx, reg.User.ID)
	if err != nil || len(roles) != 1 || roles[0].Key != identity.RoleAdmin {
		t.Fatalf("ListUserRoles = %+v, %v; want admin", roles, err)
	}
	if _, perms, err := e.h.Identity.FindRole(ctx, identity.RoleAdmin); err != nil || len(perms) != len(identity.AllPermissions()) {
		t.Errorf("admin role permissions = %v, %v; want all", perms, err)
	}
	if ok, err := e.identity.HasPermission(ctx, reg.User.ID, identity.PermManageJobs); err != nil || !ok {
		t.Errorf("HasPermission(admin, manage_jobs) = %v, %v; want true", ok, err)
	}
	actor, err := e.accounts.FindByHandle(ctx, "root", account.LocalDomain)
	if err != nil || actor.Account.ID != reg.AccountID || actor.Local == nil || actor.Local.UserID != reg.User.ID {
		t.Fatalf("FindByHandle(root) = %+v, %v; want the admin's account %s", actor.Local, err, reg.AccountID)
	}

	// A second admin reuses the built-in roles.
	e.clock.Advance(time.Second)
	second, err := e.identity.CreateAdmin(ctx, appidentity.CreateAdminInput{
		Username: "root2", Email: "root2@example.com", Password: "another long password",
	})
	if err != nil {
		t.Fatalf("CreateAdmin(root2) = %v", err)
	}
	again, err := e.h.Identity.ListUserRoles(ctx, second.User.ID)
	if err != nil || len(again) != 1 || again[0].ID != roles[0].ID {
		t.Errorf("second admin roles = %+v, %v; want the same admin role %s", again, err, roles[0].ID)
	}
	plain := e.register(t, "plain").User.ID
	if none, err := e.h.Identity.ListUserRoles(ctx, plain); err != nil || len(none) != 0 {
		t.Errorf("roles of a registered user = %+v, %v; want none", none, err)
	}
	if ok, err := e.identity.HasPermission(ctx, plain, identity.PermManageJobs); err != nil || ok {
		t.Errorf("HasPermission(plain, manage_jobs) = %v, %v; want false", ok, err)
	}
}

func accountRejectsTakenAdmin(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	first, err := e.identity.CreateAdmin(ctx, appidentity.CreateAdminInput{
		Username: "root", Email: "root@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("CreateAdmin = %v", err)
	}
	for _, tc := range []struct {
		in   appidentity.CreateAdminInput
		want error
	}{
		{appidentity.CreateAdminInput{Username: "ROOT", Email: "other@example.com", Password: "long enough"}, appaccount.ErrUsernameTaken},
		{appidentity.CreateAdminInput{Username: "other", Email: "ROOT@example.com", Password: "long enough"}, appidentity.ErrEmailTaken},
		{appidentity.CreateAdminInput{Username: "other", Email: "not an address", Password: "long enough"}, identity.ErrInvalidEmail},
		{appidentity.CreateAdminInput{Username: "other", Email: "other@example.com", Password: "short"}, identity.ErrInvalidPassword},
	} {
		if _, err := e.identity.CreateAdmin(ctx, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("CreateAdmin(%q, %q) = %v, want %v", tc.in.Username, tc.in.Email, err, tc.want)
		}
	}
	// The failed attempts rolled back: one handle, and the email still belongs to the first admin.
	if handles, err := e.h.Accounts.ListHandles(ctx); err != nil || len(handles) != 1 {
		t.Errorf("handles = %+v, %v; want only root", handles, err)
	}
	if id, err := e.h.Identity.FindUserIDByEmail(ctx, "root@example.com"); err != nil || id != first.User.ID {
		t.Errorf("FindUserIDByEmail = %q, %v; want %s", id, err, first.User.ID)
	}
	if _, err := e.h.Identity.FindUserIDByEmail(ctx, "other@example.com"); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindUserIDByEmail(other) = %v, want ErrNotFound", err)
	}
}

// accountProjectionEqualsRebuild runs random sequences of commands, which update the projections
// incrementally through the synchronous projectors, and checks that rebuilding the projections
// from their events yields exactly the same rows (data-model §9).
func accountProjectionEqualsRebuild(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	rng := rand.New(rand.NewPCG(58, 20261016)) //nolint:gosec // deterministic test input, not a secret
	kinds := []account.LifecycleKind{
		account.LifecycleDeactivated, account.LifecycleReactivated, account.LifecycleDeletionScheduled,
		account.LifecycleDeletionCancelled, account.LifecycleDeleted, account.LifecycleRemoteDeleted,
	}
	var ids []string
	for round := range 3 {
		for step := range 40 {
			e.clock.Advance(time.Duration(rng.IntN(5000)+1) * time.Millisecond)
			switch op := rng.IntN(10); {
			case op < 2 || len(ids) == 0:
				reg := e.register(t, fmt.Sprintf("u%d_%d", round, step))
				ids = append(ids, reg.AccountID)
			case op < 6:
				id := ids[rng.IntN(len(ids))]
				fields := make([]account.ProfileField, rng.IntN(4))
				for i := range fields {
					fields[i] = account.ProfileField{Name: fmt.Sprint("f", i), ValueHTML: fmt.Sprint(rng.IntN(100))}
				}
				in := appaccount.ProfileInput{DisplayName: fmt.Sprint("name ", step), Locked: rng.IntN(2) == 0, Fields: fields}
				if _, err := e.accounts.ReviseProfile(ctx, id, in); err != nil {
					t.Fatalf("ReviseProfile = %v", err)
				}
			default:
				in := appaccount.LifecycleInput{
					AccountID:   ids[rng.IntN(len(ids))],
					Kind:        kinds[rng.IntN(len(kinds))],
					InitiatedBy: account.InitiatedBySystem,
				}
				if in.Kind == account.LifecycleDeletionScheduled {
					due := e.clock.Now().Add(time.Duration(rng.IntN(30)+1) * 24 * time.Hour)
					in.DeletionDueAt = &due
				}
				if _, err := e.accounts.RecordLifecycleEvent(ctx, in); err != nil {
					t.Fatalf("RecordLifecycleEvent = %v", err)
				}
			}
		}
		incremental := snapshotAccountProjections(t, e.h.Accounts)
		if len(incremental.handles) == 0 || len(incremental.heads) == 0 {
			t.Fatalf("round %d: projections are empty: %+v", round, incremental)
		}
		if err := appaccount.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Accounts); err != nil {
			t.Fatalf("RebuildProjections = %v", err)
		}
		rebuilt := snapshotAccountProjections(t, e.h.Accounts)
		if !reflect.DeepEqual(incremental, rebuilt) {
			t.Fatalf("round %d: incremental projections differ from a rebuild:\nincremental %+v\nrebuilt     %+v", round, incremental, rebuilt)
		}
	}
}

type accountProjections struct {
	handles []account.Handle
	heads   []account.ProfileHead
	states  []account.LifecycleState
	moves   []account.MoveState
}

func snapshotAccountProjections(t *testing.T, repo appaccount.Repository) accountProjections {
	t.Helper()
	var (
		p   accountProjections
		err error
	)
	if p.handles, err = repo.ListHandles(t.Context()); err != nil {
		t.Fatalf("ListHandles = %v", err)
	}
	if p.heads, err = repo.ListProfileHeads(t.Context()); err != nil {
		t.Fatalf("ListProfileHeads = %v", err)
	}
	if p.states, err = repo.ListLifecycleStates(t.Context()); err != nil {
		t.Fatalf("ListLifecycleStates = %v", err)
	}
	if p.moves, err = repo.ListMoveStates(t.Context()); err != nil {
		t.Fatalf("ListMoveStates = %v", err)
	}
	return p
}

func accountReadsRemoteInboxes(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	local := e.register(t, "inboxless").AccountID
	if _, err := e.accounts.RemoteInbox(ctx, local); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("RemoteInbox(local) = %v, want ErrNotFound", err)
	}
	remote := e.ids.NewID()
	upsert := func(h account.RemoteHead) {
		t.Helper()
		err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.h.Accounts.UpsertRemoteHead(ctx, tx, h) })
		if err != nil {
			t.Fatalf("UpsertRemoteHead: %v", err)
		}
	}
	head := account.RemoteHead{
		AccountID: remote, SnapshotID: e.ids.NewID(), InboxURL: "https://remote.example/users/r/inbox",
		SharedInboxURL: "https://remote.example/inbox", LastFetchedAt: e.clock.Now(),
	}
	upsert(head)
	if got, err := e.accounts.RemoteInbox(ctx, remote); err != nil || got != (appaccount.RemoteInbox{InboxURL: head.InboxURL, SharedInboxURL: head.SharedInboxURL}) {
		t.Errorf("RemoteInbox = %+v, %v; want the head's inboxes", got, err)
	}
	head.SnapshotID, head.SharedInboxURL, head.ConsecutiveFetchFailures = e.ids.NewID(), "", 2
	head.LastFetchedAt = e.clock.Advance(time.Hour)
	upsert(head)
	if got, err := e.h.Accounts.FindRemoteHead(ctx, remote); err != nil || got != head {
		t.Errorf("FindRemoteHead = %+v, %v; want %+v", got, err, head)
	}
}

func accountFindsByURIAndDomain(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	local := e.register(t, "urial").AccountID
	localURI := account.ActorURI(contractHost, local)
	remote := account.Account{
		ID: e.ids.NewID(), URI: "https://remote.example/users/r", Domain: "remote.example",
		ActorType: account.ActorPerson, CreatedAt: e.clock.Now(),
	}
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.h.Accounts.InsertAccount(ctx, tx, remote) }); err != nil {
		t.Fatalf("InsertAccount: %v", err)
	}
	for uri, want := range map[string]string{localURI: local, remote.URI: remote.ID} {
		if got, err := e.accounts.FindByURI(ctx, uri); err != nil || got.Account.ID != want {
			t.Errorf("FindByURI(%s) = %+v, %v; want account %s", uri, got, err, want)
		}
	}
	if _, err := e.accounts.FindByURI(ctx, "https://remote.example/users/nobody"); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindByURI(unknown) = %v, want ErrNotFound", err)
	}

	missing := e.ids.NewID()
	for _, tc := range []struct {
		domain string
		ids    []string
		want   bool
	}{
		{"remote.example", []string{local, remote.ID}, true},
		{"remote.example", []string{remote.ID}, true},
		{"remote.example", []string{local, missing}, false},
		{"other.example", []string{remote.ID}, false},
		{account.LocalDomain, []string{missing, local}, true},
		{"remote.example", nil, false},
	} {
		if got, err := e.accounts.AnyOnDomain(ctx, tc.domain, tc.ids); err != nil || got != tc.want {
			t.Errorf("AnyOnDomain(%q, %v) = %v, %v; want %v", tc.domain, tc.ids, got, err, tc.want)
		}
	}
}
