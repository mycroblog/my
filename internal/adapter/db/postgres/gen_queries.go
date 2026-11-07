// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"github.com/mycroblog/my/internal/adapter/db/postgres/accountgen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/contentgen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/dbupgradegen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/diagnosticsgen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/federationgen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/financegen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/graphgen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/hookgen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/instancegen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/moderationgen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/notificationgen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/searchgen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/supportersgen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/timelinegen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/translationgen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/voicegen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/webhookgen"
)

// genQueries holds the queries that sqlc generates for each module (sqlc.yaml, ADR-0153 §3), the
// packages internal/adapter/db/postgres/<module>gen, behind one value on one connection or transaction:
// a query's name is unique across the packages (a repeated name makes the method ambiguous, and its
// callers fail to build), so each method is promoted. A module added to sqlc.yaml gets a field here.
type genQueries struct {
	accountQueries
	contentQueries
	dbupgradeQueries
	diagnosticsQueries
	federationQueries
	financeQueries
	graphQueries
	hookQueries
	identityQueries
	instanceQueries
	moderationQueries
	notificationQueries
	searchQueries
	supportersQueries
	timelineQueries
	translationQueries
	voiceQueries
	webhookQueries
}

// The embedded fields take the names of these aliases, since every package names its type Queries.
type (
	accountQueries      = accountgen.Queries
	contentQueries      = contentgen.Queries
	dbupgradeQueries    = dbupgradegen.Queries
	diagnosticsQueries  = diagnosticsgen.Queries
	federationQueries   = federationgen.Queries
	financeQueries      = financegen.Queries
	graphQueries        = graphgen.Queries
	hookQueries         = hookgen.Queries
	identityQueries     = identitygen.Queries
	instanceQueries     = instancegen.Queries
	moderationQueries   = moderationgen.Queries
	notificationQueries = notificationgen.Queries
	searchQueries       = searchgen.Queries
	supportersQueries   = supportersgen.Queries
	timelineQueries     = timelinegen.Queries
	translationQueries  = translationgen.Queries
	voiceQueries        = voicegen.Queries
	webhookQueries      = webhookgen.Queries
)

// newGenQueries returns the generated queries of every module on db, a PostgreSQL connection or transaction.
func newGenQueries(db accountgen.DBTX) *genQueries {
	return &genQueries{
		accountQueries:      *accountgen.New(db),
		contentQueries:      *contentgen.New(db),
		dbupgradeQueries:    *dbupgradegen.New(db),
		diagnosticsQueries:  *diagnosticsgen.New(db),
		federationQueries:   *federationgen.New(db),
		financeQueries:      *financegen.New(db),
		graphQueries:        *graphgen.New(db),
		hookQueries:         *hookgen.New(db),
		identityQueries:     *identitygen.New(db),
		instanceQueries:     *instancegen.New(db),
		moderationQueries:   *moderationgen.New(db),
		notificationQueries: *notificationgen.New(db),
		searchQueries:       *searchgen.New(db),
		supportersQueries:   *supportersgen.New(db),
		timelineQueries:     *timelinegen.New(db),
		translationQueries:  *translationgen.New(db),
		voiceQueries:        *voicegen.New(db),
		webhookQueries:      *webhookgen.New(db),
	}
}
