// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/instance"
)

// ProblemTypeFederationRestricted is the problem of an operation on remote accounts or posts that
// the federation mode (FR-413, ADR-0006) does not let through.
const ProblemTypeFederationRestricted = "/api/v1/problems/federation-restricted"

// The flows that the client API's operations on remote accounts and posts need (ADR-0006, client
// API と Web).
var (
	// remoteAccountFlows are those of following and resolving a remote account: its posts come in
	// (ingest) through the follow (follow_out).
	remoteAccountFlows = []federation.Flow{federation.FlowIngest, federation.FlowFollowOut}
	// remoteQuoteFlows are those of quoting a remote post: the QuoteRequest goes out (publish).
	remoteQuoteFlows = []federation.Flow{federation.FlowPublish}
)

// federationMode returns the federation mode that settings hold.
func federationMode(settings instance.Settings) federation.Mode {
	return federation.Mode(settings.FederationMode())
}

// federationAllows reports whether the current federation mode lets every one of flows through.
// It reads the mode per request, so that a change applies at once; without the instance
// settings it federates both ways.
func (s *Server) federationAllows(ctx context.Context, flows ...federation.Flow) (bool, error) {
	var settings instance.Settings
	if s.o.Instance != nil {
		var err error
		if settings, err = s.o.Instance.Settings(ctx); err != nil {
			return false, fmt.Errorf("clientapi: instance settings: %w", err)
		}
	}
	mode := federationMode(settings)
	for _, f := range flows {
		if !federation.Allows(mode, f) {
			return false, nil
		}
	}
	return true, nil
}

// federationRestricted is the 403 of an operation that the federation mode does not let through.
func (s *Server) federationRestricted(ctx context.Context) problem {
	return s.problem(ctx, http.StatusForbidden, ProblemTypeFederationRestricted, s.Localizer(ctx).ServerErrorsFederationRestricted(), "")
}
