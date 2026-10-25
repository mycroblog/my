// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/interface/activitypub"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// The instance's contact (the contact_account and contact_email settings) and its thumbnail
// (the thumbnail setting with the thumbnail_alt text), which GET /instance, the web client's
// about page, and NodeInfo publish (#663).

// instanceAccounts checks the local account of the contact_account setting with the account
// use cases.
type instanceAccounts struct{ a *App }

func (c instanceAccounts) CheckLocalAccount(ctx context.Context, username string) error {
	actor, err := c.a.Accounts.FindByHandle(ctx, username, account.LocalDomain)
	if err != nil {
		return err
	}
	if actor.Local == nil || actor.IsInstanceActor() || actor.Deleted() {
		return fmt.Errorf("%s is not the account of a local user", username)
	}
	return nil
}

// instanceThumbnail returns the instance's thumbnail, or nil when it is
// unset or its media file is not a processed image.
func (a *App) instanceThumbnail(ctx context.Context) (*clientapi.InstanceImage, error) {
	setting, err := a.Instance.Thumbnail(ctx)
	if err != nil || setting.MediaID == "" {
		return nil, err
	}
	img, err := a.Previews.Image(ctx, setting.MediaID, setting.Alt)
	if err != nil || img == nil {
		return nil, err
	}
	return &clientapi.InstanceImage{URL: img.URL, Width: img.Width, Height: img.Height, Alt: img.Alt}, nil
}

// nodeMetadata returns what NodeInfo's metadata says about the instance besides its name, in the
// instance's default locale: its summary, its contact, its thumbnail, and the closure of the
// service. A contact account that
// is gone or suspended is left out.
func (a *App) nodeMetadata(ctx context.Context) (activitypub.NodeMetadata, error) {
	settings, err := a.Instance.Settings(ctx)
	if err != nil {
		return activitypub.NodeMetadata{}, err
	}
	out := activitypub.NodeMetadata{Description: a.instanceSummary(ctx), ContactEmail: settings.ContactEmail()}
	if username := settings.ContactAccount(); username != "" {
		actor, err := a.Accounts.FindByHandle(ctx, username, account.LocalDomain)
		switch {
		case errors.Is(err, appaccount.ErrNotFound):
		case err != nil:
			return activitypub.NodeMetadata{}, err
		case actor.Local != nil && !actor.Deleted():
			suspended, err := a.ModerationPolicy.AccountFederationSuspended(ctx, actor.Account.ID)
			if err != nil {
				return activitypub.NodeMetadata{}, err
			}
			if !suspended {
				out.ContactActorURI = actor.Account.URI
				out.ContactName = actor.Local.Username
				if actor.Profile != nil && actor.Profile.DisplayName != "" {
					out.ContactName = actor.Profile.DisplayName
				}
			}
		}
	}
	thumbnail, err := a.instanceThumbnail(ctx)
	if err != nil {
		return activitypub.NodeMetadata{}, err
	}
	if thumbnail != nil {
		out.ThumbnailURL, out.ThumbnailAlt = thumbnail.URL, thumbnail.Alt
	}
	closure, err := a.Instance.Closure(ctx)
	if err != nil {
		return activitypub.NodeMetadata{}, err
	}
	out.Closure = activitypub.NodeClosure{
		State: string(closure.CurrentState()), ClosesAt: closure.Schedule.ClosesAt, ExportUntil: closure.Schedule.ExportUntil,
	}
	return out, nil
}
