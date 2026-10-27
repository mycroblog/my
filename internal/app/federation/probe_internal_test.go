// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// nodeInfoServer answers the NodeInfo discovery and document of host.
func nodeInfoServer(host string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/.well-known/nodeinfo" {
			_, _ = io.WriteString(w, `{"links":[{"rel":"http://nodeinfo.diaspora.software/ns/schema/2.1","href":"https://`+host+`/nodeinfo/2.1"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"version":"2.1","software":{"name":"my"}}`)
	}
}

// The probe of unavailable servers applies the domain blocks of fetches: it does not reach this
// instance or a suspended server, neither as the probed server nor through a redirect.
func TestProbeAppliesTheDomainBlocks(t *testing.T) {
	cases := []struct {
		name, domain string
		ok           bool
	}{
		{"Answers", "remote.example", true},
		{"Suspended", "blocked.example", false},
		{"ThisInstance", "social.example", false},
		{"RedirectsToSuspended", "moved.example", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := newRedirectNet()
			n.handle("remote.example", nodeInfoServer("remote.example"))
			n.handle("blocked.example", nodeInfoServer("blocked.example"))
			n.handle("social.example", nodeInfoServer("social.example"))
			n.handle("moved.example", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "https://blocked.example/.well-known/nodeinfo", http.StatusFound)
			})
			p := NewProber(ProbeDeps{HTTP: n, Domains: suspendedDomains{"blocked.example"}, Host: "social.example"})
			err := p.probe(t.Context(), tc.domain)
			if (err == nil) != tc.ok {
				t.Errorf("probe(%s) = %v, want success %v", tc.domain, err, tc.ok)
			}
			for _, req := range n.seen() {
				u, err := url.Parse(strings.TrimPrefix(req, "GET "))
				if err == nil && (u.Hostname() == "blocked.example" || u.Hostname() == "social.example") {
					t.Errorf("request %q reached a refused server", req)
				}
			}
		})
	}
}
