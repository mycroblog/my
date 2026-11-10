// SPDX-License-Identifier: AGPL-3.0-only

//go:build federationlive

package federationlive

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

// featuredTag is a hashtag that an account features, as both client APIs present it.
type featuredTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// uniqueTag returns a hashtag name of this run: prefix and digits.
func uniqueTag(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
}

// featuredNames returns the names of tags, lower-cased.
func featuredNames(tags []featuredTag) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		out = append(out, strings.ToLower(t.Name))
	}
	return out
}

// TestFeaturedTags exchanges the featured hashtags of profiles with Mastodon, the peer that has
// them (ADR-0006 §17; GoToSocial and Misskey neither publish nor read featuredTags):
//
//   - out: bob follows alice; alice features two hashtags (PUT /featured_tags), which sends the
//     Update of her actor; Mastodon fetches her featuredTags collection and lists them, in her
//     order.
//   - in: alice follows bob; bob features a hashtag on Mastodon, which sends the Add of a Hashtag
//     to his featured collection; this server fetches his featuredTags collection and lists it
//     (GET /accounts/{id}/featured_tags).
func TestFeaturedTags(t *testing.T) {
	e := stack(t)
	for _, p := range e.peers {
		if p.name != "mastodon" {
			continue
		}
		t.Run(p.name, func(t *testing.T) {
			ctx := t.Context()
			api, ok := p.peerAPI.(*mastodonAPI)
			if !ok {
				t.Fatalf("the peer %s does not speak the Mastodon client API", p.name)
			}

			followPeerToLocal(t, p)
			alice, err := p.resolve(ctx, localHandle)
			if err != nil {
				t.Fatal(err)
			}
			first, second := uniqueTag("featured"), uniqueTag("featuredtoo")
			var set struct {
				Items []featuredTag `json:"items"`
			}
			if err := e.local.api(ctx, http.MethodPut, "/featured_tags", map[string]any{"names": []string{first, second}}, &set); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = e.local.api(context.Background(), http.MethodPut, "/featured_tags", map[string]any{"names": []string{}}, nil)
			})
			want := []string{strings.ToLower(first), strings.ToLower(second)}
			eventually(t, deliveryTimeout, p.host+" lists the featured hashtags of "+localHandle, func() (bool, string) {
				var tags []featuredTag
				err := api.call(ctx, http.MethodGet, "/api/v1/accounts/"+url.PathEscape(alice.ID)+"/featured_tags", nil, &tags)
				got := featuredNames(tags)
				slices.Sort(got)
				sorted := slices.Clone(want)
				slices.Sort(sorted)
				return err == nil && slices.Equal(got, sorted), fmt.Sprintf("%v %v", got, err)
			})

			bob := followLocalToPeer(t, e, p)
			tag := uniqueTag("peerfeatured")
			var created featuredTag
			if err := api.call(ctx, http.MethodPost, "/api/v1/featured_tags", map[string]any{"name": tag}, &created); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = api.call(context.Background(), http.MethodDelete, "/api/v1/featured_tags/"+url.PathEscape(created.ID), nil, nil)
			})
			eventually(t, deliveryTimeout, localHandle+" lists the featured hashtags of "+p.handle(), func() (bool, string) {
				var got struct {
					Items []featuredTag `json:"items"`
				}
				err := e.local.api(ctx, http.MethodGet, "/accounts/"+url.PathEscape(bob.ID)+"/featured_tags", nil, &got)
				names := featuredNames(got.Items)
				return err == nil && slices.Contains(names, strings.ToLower(tag)), fmt.Sprintf("%v %v", names, err)
			})
		})
	}
}
