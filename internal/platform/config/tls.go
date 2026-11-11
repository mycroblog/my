// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/netip"
	"time"
)

// The values of the setting tls (ADR-0163 §10).
const (
	TLSOff   = "off"
	TLSFiles = "files"
)

// TLSCertificateWarning is how long before its end a certificate of tls = files makes `my config
// check` warn.
const TLSCertificateWarning = 14 * 24 * time.Hour

// ServesTLS reports whether this process listens with TLS: tls is not off and the role is web or
// all. A worker serves its health checks in plain HTTP.
func (c *Config) ServesTLS() bool {
	return c.TLS != "" && c.TLS != TLSOff && c.Role != RoleWorker
}

// defaultListen returns the default of listen: :443 when the process listens with TLS, :8080
// otherwise.
func defaultListen(mode string, role Role) string {
	if mode != "" && mode != TLSOff && role != RoleWorker {
		return ":443"
	}
	return ":8080"
}

// effectiveListen returns listen of the valid settings vals, or its default.
func effectiveListen(vals map[string]string) string {
	if v := vals["listen"]; v != "" {
		return v
	}
	return defaultListen(vals["tls"], Role(vals["role"]))
}

// tlsChecks checks the settings of tls that depend on each other.
func tlsChecks(vals map[string]string, present map[string]bool) []Issue {
	var issues []Issue
	if vals["tls"] == TLSFiles {
		for _, key := range []string{"tls_cert_file", "tls_key_file"} {
			if !present[key] {
				issues = append(issues, Issue{Key: key, Missing: true, Message: "required when tls is files"})
			}
		}
	}
	if vals["tls"] != "" && vals["tls"] != TLSOff && vals["role"] != string(RoleWorker) {
		if l := vals["tls_http_listen"]; l != "" && l == effectiveListen(vals) {
			issues = append(issues, Issue{Key: "tls_http_listen", Message: "must differ from listen, which serves https"})
		}
		if l := vals["tls_http_listen"]; l != "" && l == vals["metrics_listen"] && vals["metrics_exporter"] == "prometheus" {
			issues = append(issues, Issue{Key: "tls_http_listen", Message: "must differ from metrics_listen"})
		}
	}
	return issues
}

// TLSFindings checks how the server serves HTTPS, at now, for `my config check` in every
// profile (ADR-0163 §9, §10):
//
//   - with tls = files, the files are readable, the key matches the certificate, and the
//     certificate covers host and has not expired (errors), with a warning when fewer than 14
//     days are left;
//   - with tls = off, a warning when listen is port 80 or 443 on an address other than loopback,
//     since nothing seems to terminate TLS in front of the server;
//   - with tls other than off, a warning when trusted_proxies is set, since the peer of the
//     connection is then the client.
func TLSFindings(c *Config, now time.Time) []Finding {
	var errs, warns []Finding
	errorf := func(key, format string, args ...any) {
		errs = append(errs, Finding{Severity: SeverityError, Key: key, Message: fmt.Sprintf(format, args...)})
	}
	warnf := func(key, format string, args ...any) {
		warns = append(warns, Finding{Severity: SeverityWarning, Key: key, Message: fmt.Sprintf(format, args...)})
	}
	switch c.TLS {
	case "", TLSOff:
		if PlainListenWithoutProxy(c.Listen) {
			warnf("tls", "is off while listen is %s, a port of the web on an address other than loopback: nothing seems to terminate TLS in front of the server; set tls, or put a reverse proxy in front", c.Listen)
		}
	case TLSFiles:
		checkTLSFiles(c, now, errorf, warnf)
	}
	if c.TLS != "" && c.TLS != TLSOff && len(c.TrustedProxies) > 0 {
		warnf("trusted_proxies", "is set while the server terminates TLS itself; leave it empty unless a CDN in front connects to the server over https")
	}
	return append(errs, warns...)
}

// checkTLSFiles checks the certificate and the key of tls = files.
func checkTLSFiles(c *Config, now time.Time, errorf, warnf func(key, format string, args ...any)) {
	if c.TLSCertFile == "" || c.TLSKeyFile == "" {
		return // Load reports them missing
	}
	pair, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile)
	if err != nil {
		errorf("tls_cert_file", "cannot be used with tls_key_file: %v", err)
		return
	}
	leaf := pair.Leaf
	if leaf == nil {
		if leaf, err = x509.ParseCertificate(pair.Certificate[0]); err != nil {
			errorf("tls_cert_file", "cannot be parsed: %v", err)
			return
		}
	}
	host := c.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if err := leaf.VerifyHostname(host); err != nil {
		errorf("tls_cert_file", "does not cover host %s: %v", host, err)
	}
	switch left := leaf.NotAfter.Sub(now); {
	case left <= 0:
		errorf("tls_cert_file", "expired on %s", leaf.NotAfter.UTC().Format(time.RFC3339))
	case left < TLSCertificateWarning:
		warnf("tls_cert_file", "expires on %s, in less than 14 days; renew it", leaf.NotAfter.UTC().Format(time.RFC3339))
	}
	if now.Before(leaf.NotBefore) {
		errorf("tls_cert_file", "is not valid before %s", leaf.NotBefore.UTC().Format(time.RFC3339))
	}
}

// PlainListenWithoutProxy reports whether listen, a host:port, is port 80 or 443 on an address
// other than loopback: where browsers connect, with nothing in front to terminate TLS.
func PlainListenWithoutProxy(listen string) bool {
	host, port, err := net.SplitHostPort(listen)
	if err != nil || (port != "80" && port != "443") {
		return false
	}
	if host == "localhost" {
		return false
	}
	if a, err := netip.ParseAddr(host); err == nil && a.IsLoopback() {
		return false
	}
	return true
}
