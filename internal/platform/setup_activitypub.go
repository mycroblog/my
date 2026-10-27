// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"

	domaininstance "github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/activitypub"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpActivityPub registers the ActivityPub handler and wraps the web client's pages with it.
// It comes after setUpWeb, whose pages it wraps, and setUpRemoteActors, whose keys verify the
// signed GETs (newObjects).
func (a *App) setUpActivityPub(cfg *config.Config, version string) {
	ap := activitypub.New(activitypub.Options{
		Host: cfg.Host, AccountDomain: cfg.AccountDomain, SoftwareVersion: version, SourceURL: cfg.SourceURL,
		InstanceName: a.instanceName,
		Metadata:     a.nodeMetadata,
		// Sign-ups that wait for a moderator's approval are open, as Mastodon reports them.
		OpenRegistrations: func(ctx context.Context) (bool, error) {
			mode, err := a.Instance.RegistrationMode(ctx)
			return mode == domaininstance.RegistrationOpen || mode == domaininstance.RegistrationApproval, err
		},
		Accounts: a.Accounts, Objects: a.newObjects(cfg), Graph: a.Graph, Inbox: a.Inbox,
		HTML: a.page, Problem: a.writeStatusProblem, Logger: a.Logger, Errors: a.ErrorReporter,
		Suspensions: a.ModerationPolicy,
		LocalPosts:  a.Content.LocalPostCount,
		Images:      a.Media.Images,
		EmojiTags:   a.Media.EmojiTags,
		Aliases:     a.Accounts,
		Moves:       a.Accounts,
		// The federation mode decides which documents are served (ADR-0076), and the service
		// closure before it (ADR-0088 §2), so the handler reads the setting as it is.
		Modes: settingModes{a}, Closure: a.closureState,
	})
	ap.Register(surfaceRouter{a, surfaceActivityPub})
	// The web client's pages answer the requests that ask for ActivityPub: a post page redirects
	// to the post's id, and the others are 404 (ADR-0053).
	a.page = ap.Pages()
}
