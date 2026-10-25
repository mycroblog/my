// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"github.com/mycroblog/my/internal/adapter/httpsig"
	"github.com/mycroblog/my/internal/adapter/jsonld"
	"github.com/mycroblog/my/internal/app/federation"
)

// setUpDelivery registers the delivery of outbound activities, the instance_health projector,
// and the daily probe of unavailable servers (docs/architecture/federation.md §7.3). It needs
// the HTTP client, so it runs after newHTTPClient.
func (a *App) setUpDelivery(repo federation.Repository) error {
	ld, err := jsonld.New()
	if err != nil {
		return err
	}
	federation.RegisterHealthProjectors(a.Projectors, federation.HealthProjectorDeps{Repository: repo, IDs: a.IDs})
	a.InstanceHealth = federation.NewHealth(federation.HealthDeps{UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock})
	a.Deliverer = federation.NewDeliverer(federation.DeliverDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo,
		Client: federation.NewSignedHTTPClient(a.HTTPClient, httpsig.New(), a.KV, a.Clock),
		Keys:   federation.AccountSigningKeys{Accounts: a.Accounts},
		Proofs: federation.AccountProofKeys{Accounts: a.Accounts},
		// The Create, Update, and Delete of public posts carry a linked data signature, so that
		// relays can forward them to Mastodon and Misskey (ADR-0061).
		LinkedData: ld,
		Domains:    a.ModerationPolicy,
		Clock:      a.Clock, IDs: a.IDs, Logger: a.Logger, Meter: a.Meter, Tracer: a.Tracer,
		// The federation mode decides what goes out when each delivery runs (ADR-0076), and the
		// service closure before it (ADR-0088 §2).
		Modes: federationModes{a}, Closure: closureSettings{a},
	})
	a.Deliverer.RegisterJobs(a.Handlers)
	federation.NewProber(federation.ProbeDeps{
		Repository: repo, Health: a.InstanceHealth, HTTP: a.HTTPClient, Jobs: a.Jobs, Logger: a.Logger,
		Modes: federationModes{a},
	}).RegisterJobs(a.Handlers, a.Scheduler)
	return nil
}
