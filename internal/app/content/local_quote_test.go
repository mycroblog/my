// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"context"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
)

// statusesByURI answers FindStatusByURI alone, with ErrNotFound for a URI it does not hold.
type statusesByURI struct {
	appcontent.Repository
	statuses map[string]content.Status
}

func (r statusesByURI) FindStatusByURI(_ context.Context, uri string) (content.Status, error) {
	if st, ok := r.statuses[uri]; ok {
		return st, nil
	}
	return content.Status{}, appcontent.ErrNotFound
}

// A federated Note names no local-visibility post, nor a post of this server that is gone, whose
// visibility is not known any more; a remote post that is not stored keeps its own URI (ADR-0018 §15).
func TestFederatedQuotePage(t *testing.T) {
	const (
		localOnly = "https://social.example/ap/objects/local"
		gone      = "https://social.example/ap/objects/gone"
		remote    = "https://remote.example/notes/1"
	)
	s := appcontent.NewService(appcontent.Deps{Host: "social.example", Repository: statusesByURI{statuses: map[string]content.Status{
		localOnly: {URI: localOnly, IsLocal: true, Visibility: content.VisibilityLocal},
	}}})
	for _, tc := range []struct {
		uri, page string
		ok        bool
	}{
		{localOnly, "", false},
		{gone, gone, false},
		{remote, remote, true},
	} {
		page, ok, err := s.FederatedQuotePage(t.Context(), tc.uri)
		if err != nil || ok != tc.ok || (ok && page != tc.page) {
			t.Errorf("FederatedQuotePage(%s) = %q, %v, %v; want %q, %v", tc.uri, page, ok, err, tc.page, tc.ok)
		}
	}
}
