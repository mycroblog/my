// SPDX-License-Identifier: AGPL-3.0-only

//go:build federationlive

package federationlive

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// suspendedUser is the local user that TestSuspension suspends, so that alice keeps her
// sessions for the other scenarios.
const suspendedUser = "frank"

// mastodonAccount is the part of a Mastodon account that TestSuspension reads.
type mastodonAccount struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Suspended   bool   `json:"suspended"`
}

// suspendedOnPeer reads whether the peer shows an account as suspended: the account with
// suspended (Mastodon 4.x), or 410 Gone.
func suspendedOnPeer(ctx context.Context, p *mastodonAPI, id string) (bool, string, error) {
	var a mastodonAccount
	err := p.call(ctx, http.MethodGet, "/api/v1/accounts/"+url.PathEscape(id), nil, &a)
	var ae *apiError
	if errors.As(err, &ae) && ae.status == http.StatusGone {
		return true, "410 Gone", nil
	}
	if err != nil {
		return false, "", err
	}
	return a.Suspended, fmt.Sprintf("%+v", a), nil
}

// TestSuspension: a moderator of this server suspends a local user, and Mastodon shows the user
// as suspended from the Update of the actor (suspended: true) that reaches its shared inbox;
// lifting the suspension sends the profile again and Mastodon shows the user again
// (federation.md §11.1, ADR-0058). Mastodon only: how Misskey and GoToSocial treat the
// suspended actor is an open question of ADR-0058.
func TestSuspension(t *testing.T) {
	e := stack(t)
	var m *peerClient
	for _, p := range e.peers {
		if p.name == "mastodon" {
			m = p
		}
	}
	mastodon, ok := m.peerAPI.(*mastodonAPI)
	if !ok {
		t.Fatal("no Mastodon peer")
	}
	ctx := t.Context()

	// alice, an administrator, finds frank; bob makes Mastodon fetch frank's actor, so that
	// Mastodon knows him when the Update arrives.
	local, err := e.local.resolve(ctx, suspendedUser)
	if err != nil {
		t.Fatal(err)
	}
	remote, err := m.resolve(ctx, suspendedUser+"@"+localHost)
	if err != nil {
		t.Fatal(err)
	}
	if suspended, state, err := suspendedOnPeer(ctx, mastodon, remote.ID); err != nil || suspended {
		t.Fatalf("%s shows %s before the suspension as %s (%v)", m.host, suspendedUser, state, err)
	}

	var action struct {
		ID string `json:"id"`
	}
	if err := e.local.api(ctx, http.MethodPost, "/admin/accounts/"+url.PathEscape(local.ID)+"/actions",
		map[string]any{"kind": "suspend", "reason_category": "other", "notify_target": false}, &action); err != nil {
		t.Fatal(err)
	}
	eventually(t, 2*time.Minute, m.host+" shows the suspension", func() (bool, string) {
		suspended, state, err := suspendedOnPeer(ctx, mastodon, remote.ID)
		if err != nil {
			return false, err.Error()
		}
		return suspended, state
	})

	if err := e.local.api(ctx, http.MethodPost, "/admin/account_actions/"+url.PathEscape(action.ID)+"/lift",
		map[string]any{"reason": "live test"}, nil); err != nil {
		t.Fatal(err)
	}
	eventually(t, 2*time.Minute, m.host+" shows the account again", func() (bool, string) {
		suspended, state, err := suspendedOnPeer(ctx, mastodon, remote.ID)
		if err != nil {
			return false, err.Error()
		}
		return !suspended, state
	})
}
