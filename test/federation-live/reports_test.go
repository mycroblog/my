// SPDX-License-Identifier: AGPL-3.0-only

//go:build federationlive

package federationlive

import (
	"context"
	"net/http"
	"os"
)

// report files a report of the account id as alice (POST /api/v1/reports), forwarded to the
// account's server when forward is set, and returns whether the server recorded it as forwarded.
func (l *localClient) report(ctx context.Context, id string, statusIDs []string, comment string, forward bool) (bool, error) {
	var r struct {
		Forwarded bool `json:"forwarded"`
	}
	err := l.call(ctx, http.MethodPost, "/reports", map[string]any{
		"account_id": id, "status_ids": statusIDs, "category": "spam", "comment": comment, "forward": forward,
	}, &r)
	return r.Forwarded, err
}

// peerReportComments returns the comments of the open reports in the moderators' list of the
// peer, read with the token of its administrator (scripts/federation-live.sh), or why the peer is
// not checked.
func peerReportComments(ctx context.Context, e *env, p *peerClient) ([]string, string, error) {
	var comments []string
	switch p.name {
	case "mastodon":
		var reports []struct {
			Comment string `json:"comment"`
		}
		err := call(ctx, e.http, os.Getenv("FEDERATION_LIVE_MASTODON_ADMIN_TOKEN"), http.MethodGet,
			"https://"+p.host+"/api/v1/admin/reports?limit=200", nil, &reports)
		for _, r := range reports {
			comments = append(comments, r.Comment)
		}
		return comments, "", err
	case "misskey":
		var reports []struct {
			Comment string `json:"comment"`
		}
		err := call(ctx, e.http, "", http.MethodPost, "https://"+p.host+"/api/admin/abuse-user-reports",
			map[string]any{"i": os.Getenv("FEDERATION_LIVE_MISSKEY_ADMIN_TOKEN"), "limit": 100}, &reports)
		for _, r := range reports {
			// Misskey appends the URIs of the Flag's object to its content.
			comments = append(comments, r.Comment)
		}
		return comments, "", err
	}
	return nil, "the scenarios have no administrator of " + p.name, nil
}
