// SPDX-License-Identifier: AGPL-3.0-only

package tlsserve

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
)

// ACMEChallengePrefix is the path of the ACME HTTP-01 challenges (RFC 8555 §8.3).
const ACMEChallengePrefix = "/.well-known/acme-challenge/"

// RedirectHandler is the handler of port 80 (ADR-0163 §3). It sends GET and HEAD to the same
// path on https with 301 and answers 400 to every other method, so that no request body is read
// in plain text. challenge, when not nil, answers the paths under ACMEChallengePrefix (the
// HTTP-01 challenge) instead.
//
// The redirect goes to the request's host when it is a host of h, and to fallbackHost otherwise,
// so that it never points at a name the server does not serve. httpsPort is the port of the TLS
// listener; 443 leaves it out of the URL.
func RedirectHandler(h *Holder, fallbackHost, httpsPort string, challenge http.Handler) http.Handler {
	fallbackHost = normalizeHost(fallbackHost)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if challenge != nil && strings.HasPrefix(r.URL.Path, ACMEChallengePrefix) {
			challenge.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Connection", "close")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		host := normalizeHost(r.Host)
		if !h.Serves(host) {
			host = fallbackHost
		}
		if httpsPort != "" && httpsPort != "443" {
			host = net.JoinHostPort(host, httpsPort)
		}
		target := "https://" + host + r.URL.RequestURI()
		if r.URL.RequestURI() == "*" {
			target = "https://" + host + "/"
		}
		w.Header().Set("Location", target)
		w.WriteHeader(http.StatusMovedPermanently)
	})
}

// HTTP01Handler answers the ACME HTTP-01 challenges under ACMEChallengePrefix (RFC 8555 §8.3)
// with the key authorization that answer returns for the token, and 404 when it returns an
// error. It answers GET and HEAD only, 400 to every other method.
func HTTP01Handler(answer func(ctx context.Context, token string) (string, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Connection", "close")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		token, ok := strings.CutPrefix(r.URL.Path, ACMEChallengePrefix)
		if !ok || token == "" || strings.Contains(token, "/") {
			http.NotFound(w, r)
			return
		}
		keyAuth, err := answer(r.Context(), token)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.WriteString(w, keyAuth)
	})
}
