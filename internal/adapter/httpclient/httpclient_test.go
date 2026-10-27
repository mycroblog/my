// SPDX-License-Identifier: AGPL-3.0-only

package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestContract(t *testing.T) {
	testkit.HTTPClientContract(t, func(_ *testing.T, n testkit.HTTPClientNetwork) kernel.HTTPClient {
		return New(Options{
			UserAgent:       "my/test (+https://social.example/)",
			AllowHTTP:       n.AllowHTTP,
			Ports:           n.Ports,
			RootCAs:         n.RootCAs,
			LookupNetIP:     n.LookupNetIP,
			ExemptAddrPorts: n.Exempt,
		})
	})
}

func TestAllowedAddr(t *testing.T) {
	for _, a := range []string{"8.8.8.8", "1.1.1.1", "172.32.0.1", "100.128.0.1", "2606:4700:4700::1111", "2001:4860:4860::8888", "64:ff9b::808:808", "2002:808:808::1", "::ffff:8.8.8.8"} {
		if !allowedAddr(netip.MustParseAddr(a)) {
			t.Errorf("allowedAddr(%s) = false, want true", a)
		}
	}
	for _, a := range []string{"10.0.0.1", "fe80::1%eth0", "2606:4700::1%1", "64:ff9b::a00:1", "::ffff:192.168.0.1"} {
		if allowedAddr(netip.MustParseAddr(a)) {
			t.Errorf("allowedAddr(%s) = true, want false", a)
		}
	}
}

func TestParseHostAddr(t *testing.T) {
	for host, want := range map[string]string{
		"2130706433":          "127.0.0.1",
		"0x7f000001":          "127.0.0.1",
		"017700000001":        "127.0.0.1",
		"0x7f.1":              "127.0.0.1",
		"127.1":               "127.0.0.1",
		"127.0.1":             "127.0.0.1",
		"0xa9.0xfe.0xa9.0xfe": "169.254.169.254",
		"8.8.8.8":             "8.8.8.8",
		"8.8.8.8.":            "8.8.8.8",
		"::1":                 "::1",
		"0":                   "0.0.0.0",
	} {
		a, numeric, ok := parseHostAddr(host)
		if !ok || !numeric || a.String() != want {
			t.Errorf("parseHostAddr(%q) = %v, %v, %v; want %s", host, a, numeric, ok, want)
		}
	}
	for _, host := range []string{"256.0.0.1", "1.2.3.4.5", "1.2.3.256", "4294967296", "1.08", "example.1x"} {
		if _, numeric, ok := parseHostAddr(host); ok || (!numeric && host != "example.1x") {
			t.Errorf("parseHostAddr(%q) = numeric %v, ok %v; want a malformed address", host, numeric, ok)
		}
	}
	for _, host := range []string{"social.example", "1.example", "example.com", "0x.example"} {
		if _, numeric, _ := parseHostAddr(host); numeric {
			t.Errorf("parseHostAddr(%q) is numeric, want a name", host)
		}
	}
}

func TestIgnoresProxyEnvironment(t *testing.T) {
	c := New(Options{})
	if tr := c.http.Transport.(*http.Transport); tr.Proxy != nil {
		t.Error("Transport.Proxy is set; proxy environment variables must be ignored")
	}
}

func TestAllowPrivateNetworks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	t.Cleanup(srv.Close)
	req := &kernel.HTTPRequest{URL: srv.URL, Purpose: kernel.HTTPPurposeJSON}
	if _, err := New(Options{AllowHTTP: true, AnyPort: true}).Do(t.Context(), req); !errors.Is(err, kernel.ErrForbiddenDestination) {
		t.Fatalf("Do(loopback) = %v, want ErrForbiddenDestination", err)
	}
	resp, err := New(Options{AllowHTTP: true, AnyPort: true, AllowPrivateNetworks: true}).Do(t.Context(), req)
	if err != nil {
		t.Fatalf("Do(loopback) with AllowPrivateNetworks = %v", err)
	}
	if b, err := resp.ReadAll(); err != nil || string(b) != "ok" {
		t.Errorf("body = %q, %v", b, err)
	}
}

func TestTimeout(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.(http.Flusher).Flush()
		<-block
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(block) })
	c := New(Options{
		AllowHTTP: true, AnyPort: true, AllowPrivateNetworks: true,
		Limits: map[kernel.HTTPPurpose]kernel.HTTPLimits{kernel.HTTPPurposeJSON: {Timeout: 100 * time.Millisecond}},
	})
	resp, err := c.Do(t.Context(), &kernel.HTTPRequest{URL: srv.URL, Purpose: kernel.HTTPPurposeJSON})
	if err != nil {
		t.Fatalf("Do() = %v", err)
	}
	start := time.Now()
	if _, err := resp.ReadAll(); err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Errorf("ReadAll() = %v, want the deadline", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("ReadAll() took %v", d)
	}
}

// TestExemptDestinations checks the destinations that the operator exempts from the guard for
// the webhooks (webhook_allowed_destinations) and for the identity providers
// (login_provider_allowed_destinations): each list serves its own purpose alone.
func TestExemptDestinations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/moved" {
			http.Redirect(w, r, "/end", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)
	addr := netip.MustParseAddrPort(strings.TrimPrefix(srv.URL, "http://"))
	resolve := func(_ context.Context, host string) ([]netip.Addr, error) {
		if host == "n8n.internal" || host == "hooks.example.com" {
			return []netip.Addr{addr.Addr()}, nil
		}
		return nil, errors.New("no such host")
	}
	port := ":" + strconv.Itoa(int(addr.Port()))
	for _, p := range []struct {
		name    string
		purpose kernel.HTTPPurpose
		other   kernel.HTTPPurpose
		set     func(*Options, Destinations)
	}{
		{"webhook", kernel.HTTPPurposeWebhook, kernel.HTTPPurposeLoginProvider, func(o *Options, d Destinations) { o.Webhook = d }},
		{"login provider", kernel.HTTPPurposeLoginProvider, kernel.HTTPPurposeWebhook, func(o *Options, d Destinations) { o.LoginProvider = d }},
	} {
		newClient := func(d Destinations) *Client {
			o := Options{LookupNetIP: resolve}
			p.set(&o, d)
			return New(o)
		}
		byName := newClient(Destinations{Hosts: []string{"n8n.internal"}})
		byRange := newClient(Destinations{Prefixes: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
		none := newClient(Destinations{})
		for _, tc := range []struct {
			name    string
			c       *Client
			url     string
			purpose kernel.HTTPPurpose
			ok      bool
		}{
			{"exempt name", byName, "http://n8n.internal" + port + "/", p.purpose, true},
			{"exempt name, other purpose", byName, "http://n8n.internal" + port + "/", kernel.HTTPPurposeJSON, false},
			{"exempt name, other exempting purpose", byName, "http://n8n.internal" + port + "/", p.other, false},
			{"exempt range", byRange, "http://127.0.0.1" + port + "/", p.purpose, true},
			{"name resolving into the exempt range", byRange, "http://hooks.example.com" + port + "/", p.purpose, false},
			{"exempt range, other purpose", byRange, "http://127.0.0.1" + port + "/", kernel.HTTPPurposeImage, false},
			{"exempt range, other exempting purpose", byRange, "http://127.0.0.1" + port + "/", p.other, false},
			{"no exemption", none, "http://127.0.0.1" + port + "/", p.purpose, false},
			{"other name", byName, "https://other.internal/", p.purpose, false},
		} {
			resp, err := tc.c.Do(t.Context(), &kernel.HTTPRequest{Method: http.MethodPost, URL: tc.url, Body: []byte("{}"), Purpose: tc.purpose})
			if tc.ok {
				if err != nil {
					t.Errorf("%s: %s: Do = %v", p.name, tc.name, err)
					continue
				}
				_, _ = resp.ReadAll()
				continue
			}
			if !errors.Is(err, kernel.ErrForbiddenDestination) {
				t.Errorf("%s: %s: Do = %v, want ErrForbiddenDestination", p.name, tc.name, err)
			}
		}
		// Neither purpose follows a redirect: the 3xx is the answer.
		resp, err := byRange.Do(t.Context(), &kernel.HTTPRequest{
			Method: http.MethodPost, URL: "http://127.0.0.1" + port + "/moved", Purpose: p.purpose,
		})
		if err != nil {
			t.Fatalf("%s: Do(redirect) = %v", p.name, err)
		}
		_, _ = resp.ReadAll()
		if resp.StatusCode != http.StatusFound {
			t.Errorf("%s: status = %d, want the 302 itself", p.name, resp.StatusCode)
		}
	}
}
