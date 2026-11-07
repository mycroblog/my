// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/mycroblog/my/internal/adapter/markdown"
	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	domaininstance "github.com/mycroblog/my/internal/domain/instance"
)

// setUpInstance registers the instance module's projectors and builds its service, which holds
// the instance settings and texts.
func (a *App) setUpInstance(repo instance.Repository) {
	instance.RegisterProjectors(a.Projectors, repo)
	a.Instance = instance.NewService(instance.Deps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock, IDs: a.IDs,
		// The preview_image setting refers to a media file, which setMedia sets up later.
		Media: instanceMedia{a},
		// The contact_account setting names a local account.
		Accounts: instanceAccounts{a},
		// The terms of service, the privacy policy (#489), and the announcements (FR-726) are Markdown.
		Documents: markdown.New(),
		// The checks before the closure of the service count over other modules.
		ClosureCounts: closureCounts{a},
		// The one-person server counts the local accounts with the identity module (ADR-0152 §1).
		SingleUserAccounts: singleUserAccounts{a},
		// The progress of the closure's Deletes is the federation module's, set up later.
		ClosureDeliveries: closureDeliveries{a},
		// The files of the brand images live in the blob store, which setMedia sets up later.
		Blobs: instanceBlobs{a},
		// The secrets of the instance, such as the media proxy's key, are sealed like the actor keys.
		Sealer: a.Sealer,
	})
}

// domainsStartTimeout bounds the check of the domains in NewApp, so that an unreachable database
// delays the start only this long.
const domainsStartTimeout = 5 * time.Second

// defaultDomainsRetry is how often Serve tries again when NewApp could not check the domains.
const defaultDomainsRetry = 5 * time.Second

// verifyDomainsAtStart checks host and account_domain against the database, and unless checkOnly also
// records them on a database that has none (ADR-0018). Only a mismatch is an error: without a
// schema or a database the App still starts, and Ready and Serve verify the domains later.
func (a *App) verifyDomainsAtStart(ctx context.Context, checkOnly bool) error {
	ctx, cancel := context.WithTimeout(ctx, domainsStartTimeout)
	defer cancel()
	var err error
	if checkOnly {
		// The App stays unverified: it records nothing, and these commands never call Ready.
		err = a.Instance.CheckDomains(ctx, a.configuredDomains())
	} else {
		err = a.verifyDomains(ctx)
	}
	var mismatch *domaininstance.DomainMismatchError
	if errors.As(err, &mismatch) {
		return err
	}
	return nil
}

// verifyDomains records or compares the domains unless a call has done so already.
func (a *App) verifyDomains(ctx context.Context) error {
	if a.domainsVerified.Load() {
		return nil
	}
	if err := a.Instance.EnsureDomains(ctx, a.configuredDomains()); err != nil {
		return err
	}
	a.domainsVerified.Store(true)
	return nil
}

func (a *App) configuredDomains() domaininstance.Domains {
	return domaininstance.Domains{Host: a.Config.Host, AccountDomain: a.Config.AccountDomain}
}

// awaitDomains verifies the domains every interval until that succeeds or ctx ends, and returns
// the mismatch when they differ.
func (a *App) awaitDomains(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		err := a.verifyDomains(ctx)
		if err == nil {
			return nil
		}
		var mismatch *domaininstance.DomainMismatchError
		if errors.As(err, &mismatch) {
			return err
		}
	}
}

// instanceMedia checks the media files of the instance settings with the media use cases.
type instanceMedia struct{ a *App }

func (m instanceMedia) CheckImage(ctx context.Context, mediaID string) error {
	if m.a.Media == nil {
		return errors.New("media files are not set up")
	}
	return m.a.Media.CheckImage(ctx, mediaID)
}

// instanceBlobs reaches the blob store, which is set up after the instance module.
type instanceBlobs struct{ a *App }

var errNoBlobs = errors.New("the blob store is not set up")

func (b instanceBlobs) Put(ctx context.Context, key string, r io.Reader, opts blob.PutOptions) (blob.Info, error) {
	if b.a.Blobs == nil {
		return blob.Info{}, errNoBlobs
	}
	return b.a.Blobs.Put(ctx, key, r, opts)
}

func (b instanceBlobs) Get(ctx context.Context, key string) (io.ReadCloser, blob.Info, error) {
	if b.a.Blobs == nil {
		return nil, blob.Info{}, errNoBlobs
	}
	return b.a.Blobs.Get(ctx, key)
}

func (b instanceBlobs) Head(ctx context.Context, key string) (blob.Info, error) {
	if b.a.Blobs == nil {
		return blob.Info{}, errNoBlobs
	}
	return b.a.Blobs.Head(ctx, key)
}

func (b instanceBlobs) Delete(ctx context.Context, key string) error {
	if b.a.Blobs == nil {
		return errNoBlobs
	}
	return b.a.Blobs.Delete(ctx, key)
}

func (b instanceBlobs) PublicURL(key string) string {
	if b.a.Blobs == nil {
		return ""
	}
	return b.a.Blobs.PublicURL(key)
}

// singleUserAccounts counts the local accounts for the setting single_user (ADR-0152 §1) with the
// identity module, which is set up later and read at call time.
type singleUserAccounts struct{ a *App }

func (c singleUserAccounts) CountSingleUserAccountsTx(ctx context.Context, tx uow.Tx) (int, error) {
	return c.a.Identity.CountSingleUserAccountsTx(ctx, tx)
}

// closureCounts counts what the closure of the service ends (ADR-0088 §1) with the account,
// supporters, moderation, and federation modules. The modules set up later are read at call time.
type closureCounts struct{ a *App }

func (c closureCounts) CountLocalUsers(ctx context.Context) (int, error) {
	return c.a.Accounts.CountLocalUsers(ctx)
}

func (c closureCounts) CountKnownSharedInboxes(ctx context.Context) (int, error) {
	return c.a.Accounts.CountKnownSharedInboxes(ctx)
}

func (c closureCounts) LiveSubscriptionCount(ctx context.Context) (int, error) {
	if c.a.Supporters == nil {
		return 0, nil
	}
	return c.a.Supporters.LiveSubscriptionCount(ctx)
}

// PendingMigrations counts the Moves of local accounts that the federation module still sends
// (ADR-0089).
func (c closureCounts) PendingMigrations(ctx context.Context) (int, error) {
	if c.a.ClosureDeletes == nil {
		return 0, nil
	}
	return c.a.ClosureDeletes.PendingMoves(ctx)
}

func (c closureCounts) HeldAccounts(ctx context.Context) (int, error) {
	ids, err := c.a.Moderation.HeldAccountIDs(ctx)
	return len(ids), err
}
