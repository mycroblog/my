// SPDX-License-Identifier: AGPL-3.0-only

package kind_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/queue"
)

func TestPlainTCPRefusedOnlyForTheKindsThatCarrySecretsPostsOrPersonalData(t *testing.T) {
	refused := map[string]bool{
		kind.KeyManager: true, kind.Mailer: true, kind.Translator: true,
		kind.InboundStatus: true, kind.LocalStatus: true, kind.SignupCheck: true,
	}
	for _, k := range kind.Delegated() {
		for _, addr := range []string{"http://127.0.0.1:8080", "http://plugin.internal:8080", "http://localhost"} {
			problems := k.Check(kind.Destination{Address: addr})
			if refused[k.Name] && !has(problems, kind.SuffixAddress) {
				t.Errorf("%s at %s: want a problem with the address, got %v", k.Name, addr, problems)
			}
			if !refused[k.Name] {
				if len(problems) != 0 {
					t.Errorf("%s at %s: got %v", k.Name, addr, problems)
				}
				if w := k.Warnings(kind.Destination{Address: addr}); len(w) != 1 ||
					!strings.Contains(w[0].Message, k.Name) || !strings.Contains(w[0].Message, addr) {
					t.Errorf("%s at %s: want a warning that names both, got %v", k.Name, addr, w)
				}
			}
		}
		for _, addr := range []string{"unix:///run/my/plugin.sock", "https://plugin.internal:8443"} {
			if problems := k.Check(kind.Destination{Address: addr}); len(problems) != 0 {
				t.Errorf("%s at %s: got %v", k.Name, addr, problems)
			}
		}
	}
}

func TestTokenNeedsTLSOrAUnixSocket(t *testing.T) {
	translator, _ := kind.ByName(kind.Translator)
	if problems := translator.Check(kind.Destination{Address: "http://127.0.0.1:1", Token: "t"}); !has(problems, kind.SuffixToken) {
		t.Errorf("token over http: %v", problems)
	}
	for _, addr := range []string{"unix:///run/p.sock", "https://p.internal"} {
		if problems := translator.Check(kind.Destination{Address: addr, Token: "t"}); len(problems) != 0 {
			t.Errorf("token at %s: %v", addr, problems)
		}
	}
}

func TestCheck(t *testing.T) {
	media, _ := kind.ByName(kind.MediaVideo)
	translator, _ := kind.ByName(kind.Translator)
	cases := []struct {
		name string
		kind kind.Kind
		d    kind.Destination
		want string
	}{
		{"streaming refuses json", media, kind.Destination{Address: "unix:///s", Protocol: kind.ProtocolJSON}, kind.SuffixProtocol},
		{"unknown protocol", translator, kind.Destination{Address: "unix:///s", Protocol: "soap"}, kind.SuffixProtocol},
		{"ca on a socket", translator, kind.Destination{Address: "unix:///s", TLSCAFile: "/c"}, kind.SuffixTLSCAFile},
		{"cert on http", translator, kind.Destination{Address: "http://p", TLSCertFile: "/c", TLSKeyFile: "/k"}, kind.SuffixTLSCertFile},
		{"cert without key", translator, kind.Destination{Address: "https://p", TLSCertFile: "/c"}, kind.SuffixTLSKeyFile},
		{"key without cert", translator, kind.Destination{Address: "https://p", TLSKeyFile: "/k"}, kind.SuffixTLSCertFile},
		{"timeout at the lease", translator, kind.Destination{Address: "https://p", Timeout: 5 * time.Minute}, kind.SuffixTimeout},
		{"negative timeout", translator, kind.Destination{Address: "https://p", Timeout: -time.Second}, kind.SuffixTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if problems := tc.kind.Check(tc.d); !has(problems, tc.want) {
				t.Errorf("want a problem with %s, got %v", tc.want, problems)
			}
		})
	}
}

func TestParseAddress(t *testing.T) {
	for _, bad := range []string{
		"", "plugin:8080", "ftp://p", "unix://relative.sock", "unix:///", "unix:///a/../b", "unix://host/s", "https://", "http://:80",
		"https://user:pw@p", "https://p?x=1", "https://p#f", "tcp://p:1",
	} {
		if _, err := kind.ParseAddress(bad); err == nil {
			t.Errorf("ParseAddress(%q) = nil error", bad)
		}
	}
	a, err := kind.ParseAddress("unix:///run/my/mailer.sock")
	if err != nil || a.Scheme != kind.SchemeUnix || a.SocketPath != "/run/my/mailer.sock" || a.BaseURL == "" {
		t.Errorf("unix = %+v, %v", a, err)
	}
	a, err = kind.ParseAddress("https://mailer.internal:8443/prefix")
	if err != nil || a.Scheme != kind.SchemeHTTPS || a.BaseURL != "https://mailer.internal:8443/prefix" {
		t.Errorf("https = %+v, %v", a, err)
	}
}

func TestEveryKindBoundsItsCalls(t *testing.T) {
	if kind.MaxTimeout != queue.DefaultLease {
		t.Errorf("MaxTimeout = %s, the lease of a job = %s", kind.MaxTimeout, queue.DefaultLease)
	}
	for _, name := range kind.Names() {
		k, ok := kind.ByName(name)
		if !ok {
			t.Fatalf("ByName(%q) not found", name)
		}
		// A gRPC server takes 4 MiB by default, so the server never sends more (ADR-0146 §12).
		if k.Timeout <= 0 || k.Timeout >= k.TimeoutBound() || k.MaxSend <= 0 || k.MaxSend > 4<<20 || k.MaxReceive <= 0 || k.Concurrency <= 0 {
			t.Errorf("%s: %+v", name, k)
		}
		if problems := k.Check(kind.Destination{Address: "unix:///s", Protocol: k.Protocol, Timeout: k.Timeout}); len(problems) != 0 {
			t.Errorf("%s: its own defaults: %v", name, problems)
		}
	}
	// A kind that a request waits for takes a deadline below 30 seconds; one only jobs call, below the lease.
	for name, inRequests := range map[string]bool{kind.KeyManager: true, kind.SignupCheck: true, kind.LocalStatus: true, kind.LocalProfile: true, kind.Translator: true, kind.MediaCheck: true, kind.Mailer: false, kind.MediaImage: false, kind.MediaVideo: false, kind.MediaAudio: false, kind.Billing: false, kind.InboundStatus: false} {
		k, _ := kind.ByName(name)
		long := kind.Destination{Address: "unix:///s", Timeout: kind.MaxRequestTimeout}
		if got := len(k.Check(long)) != 0; got != inRequests {
			t.Errorf("%s with a deadline of %s refused = %v, want %v", name, kind.MaxRequestTimeout, got, inRequests)
		}
		if problems := k.Check(kind.Destination{Address: "unix:///s", Timeout: kind.MaxRequestTimeout - time.Second}); len(problems) != 0 {
			t.Errorf("%s: %v", name, problems)
		}
	}
	if _, ok := kind.ByName("nope"); ok {
		t.Error("ByName found an unknown kind")
	}
	// The dots of a hook name do not reach the key.
	k, _ := kind.ByName(kind.InboundStatus)
	if got := k.Key(kind.SuffixAddress); got != "plugin_inbound_status_address" {
		t.Errorf("Key = %q", got)
	}
}

func TestWarningsAboutNoAuthentication(t *testing.T) {
	k, _ := kind.ByName(kind.Mailer)
	if w := k.Warnings(kind.Destination{Address: "https://m"}); len(w) != 1 {
		t.Errorf("https alone: %v", w)
	}
	for _, d := range []kind.Destination{
		{Address: "https://m", Token: "t"},
		{Address: "https://m", TLSCertFile: "/c", TLSKeyFile: "/k"},
		{Address: "unix:///s"},
	} {
		if w := k.Warnings(d); len(w) != 0 {
			t.Errorf("%+v: %v", d, w)
		}
	}
}

func has(problems []kind.Problem, suffix string) bool {
	for _, p := range problems {
		if p.Suffix == suffix {
			return true
		}
	}
	return false
}
