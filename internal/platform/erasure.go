// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"fmt"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/timeline"
	appvoice "github.com/mycroblog/my/internal/app/voice"
)

// setUpErasure registers the erasure of deleted local accounts' data and its schedule
// (data-model §8, FR-008). Every database adapter's repositories implement the modules'
// erasure repositories. The erasers run in order: the statuses first, whose notifications the
// notification module erases with them, then the account's interactions, follows, timeline and
// notification rows, its user's reads of the announcements, its uploads with their blobs, and
// the names and descriptions of the voice chat room revisions it wrote, and its user's identity
// data; the account module erases the profile and keys last. A retention hold stops it, and the statuses that the
// moderation records need stay with their media files (ADR-0038). It comes after the content,
// graph, timeline, notification, and media services, the blob store, and the data exports and imports.
func (a *App) setUpErasure(ids identity.Repository, accounts account.Repository, statuses content.Repository,
	follows graph.Repository, feeds timeline.Repository, notifs notification.Repository, medias content.MediaRepository,
	rooms appvoice.Repository,
) error {
	identities, okI := ids.(identity.ErasureRepository)
	profiles, okA := accounts.(account.ErasureRepository)
	posts, okC := statuses.(content.ErasureRepository)
	edges, okG := follows.(graph.ErasureRepository)
	entries, okT := feeds.(timeline.ErasureRepository)
	notes, okN := notifs.(notification.ErasureRepository)
	if !okI || !okA || !okC || !okG || !okT || !okN {
		return fmt.Errorf("platform: the database adapter does not implement the erasure of deleted accounts")
	}
	notifications := notification.NewEraser(a.UnitOfWork, notes)
	account.NewEraser(account.ErasureDeps{
		UnitOfWork: a.UnitOfWork, Repository: profiles, Holds: a.Moderation, Jobs: a.Jobs, Clock: a.Clock,
		Erasers: []account.DataEraser{
			content.NewStatusEraser(content.ErasureDeps{
				UnitOfWork: a.UnitOfWork, Repository: statuses, Erasure: posts, Notifications: notifications,
				Evidence: a.Moderation, Clock: a.Clock,
			}),
			content.NewInteractionEraser(a.UnitOfWork, posts),
			graph.NewEraser(a.UnitOfWork, follows, edges, a.Clock),
			timeline.NewEraser(a.UnitOfWork, entries),
			notifications,
			// The user's reads of the announcements (FR-726).
			a.Instance,
			// The instance settings' images stay; the profile's go with the account.
			content.NewMediaEraser(content.MediaErasureDeps{
				UnitOfWork: a.UnitOfWork, Repository: medias, Blobs: a.Blobs, Referrers: []content.MediaReferrer{a.Instance},
			}),
			// The user's data exports with their archives (FR-008).
			a.Exports,
			// The user's CSV imports (FR-008).
			a.Imports,
			// The user's listing on the supporter list and the payment provider's customers; the
			// subscriptions stay for the accounting (ADR-0072 §10).
			a.Supporters,
			// The names and descriptions of the revisions of the rooms it wrote (ADR-0100 §13).
			appvoice.NewEraser(a.UnitOfWork, rooms),
			identity.NewEraser(a.UnitOfWork, identities),
		},
	}).RegisterJobs(a.Handlers, a.Scheduler)
	return nil
}
