// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Rate limits of the fetch of Client ID Metadata Documents (ADR-0090 §7). The fetch is an
// outbound request that anyone can cause with a client_id, so it is limited per document URL
// and per host. A fresh stored copy is used without a fetch and counts for neither.
var (
	MetadataDocumentURLLimit  = kernel.Limit{Name: "oauth.cimd.url", Max: 1, Window: 5 * time.Minute}
	MetadataDocumentHostLimit = kernel.Limit{Name: "oauth.cimd.host", Max: 30, Window: time.Hour}
)

// takeMetadataDocumentFetch counts one fetch of the document at clientID on host against the
// limits of the URL and of the host. It fails with a *kernel.RateLimitedError when either is
// used up, and lets the fetch through without a RateLimiter. A fetch refused by the URL limit
// does not count against the host.
func (s *Service) takeMetadataDocumentFetch(ctx context.Context, clientID, host string) error {
	if s.d.RateLimiter == nil {
		return nil
	}
	for _, c := range []struct {
		lim kernel.Limit
		key string
	}{{MetadataDocumentURLLimit, clientID}, {MetadataDocumentHostLimit, host}} {
		d, err := s.d.RateLimiter.Take(ctx, c.lim, c.key)
		if err != nil {
			return err
		}
		if !d.Allowed {
			return &kernel.RateLimitedError{RetryAfter: d.RetryAfter}
		}
	}
	return nil
}
