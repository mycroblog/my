// SPDX-License-Identifier: AGPL-3.0-only

// Package tlsserve serves HTTPS from the server itself (ADR-0163 §1, §3, §9): the TLS
// configuration of the listener, the holder of the certificates in memory that the handshake
// reads, the source that loads them from the operator's files, and the handler of port 80.
//
// The handshake reads only the Holder, never a database, a KMS, or a KV. Whatever obtains a
// certificate (the files of tls = files, or an ACME manager) puts it into the Holder with Set.
package tlsserve

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
)

// ACMETLS1Protocol is the ALPN protocol of the ACME TLS-ALPN-01 challenge (RFC 8737).
const ACMETLS1Protocol = "acme-tls/1"

// ErrUnknownHost fails the handshake of a server name the Holder serves no certificate for.
var ErrUnknownHost = errors.New("tlsserve: no certificate for this server name")

// Holder knows which certificate serves which host. The hosts are fixed when it is made; a host
// without a certificate yet, like a server name it does not know, fails the handshake. It is safe
// for concurrent use.
type Holder struct {
	hosts []string
	mu    sync.RWMutex
	certs map[string]*tls.Certificate
	// alpn answers the handshakes that offer only acme-tls/1; nil fails them.
	alpn func(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

// NewHolder returns a Holder for hosts, names without a port (a port is cut off), compared without
// regard to case.
func NewHolder(hosts ...string) *Holder {
	h := &Holder{certs: map[string]*tls.Certificate{}}
	for _, host := range hosts {
		if n := normalizeHost(host); n != "" && !slices.Contains(h.hosts, n) {
			h.hosts = append(h.hosts, n)
		}
	}
	return h
}

// normalizeHost lower-cases host and drops a port and a trailing dot.
func normalizeHost(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

// Hosts returns the hosts of the Holder, in the order they were given.
func (h *Holder) Hosts() []string { return slices.Clone(h.hosts) }

// Serves reports whether host is one of the hosts of the Holder.
func (h *Holder) Serves(host string) bool { return slices.Contains(h.hosts, normalizeHost(host)) }

// Set makes cert serve host from the next handshake on. A host the Holder does not serve is an
// error.
func (h *Holder) Set(host string, cert *tls.Certificate) error {
	host = normalizeHost(host)
	if !slices.Contains(h.hosts, host) {
		return fmt.Errorf("tlsserve: %q is not a host of this server", host)
	}
	if cert == nil {
		return errors.New("tlsserve: no certificate")
	}
	h.mu.Lock()
	h.certs[host] = cert
	h.mu.Unlock()
	return nil
}

// Certificate returns the certificate that serves host, or nil.
func (h *Holder) Certificate(host string) *tls.Certificate {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.certs[normalizeHost(host)]
}

// SetALPNChallenge sets what answers the handshakes that offer only acme-tls/1 (the TLS-ALPN-01
// challenge of ACME); nil, the default, fails them. Call it before the listener starts.
func (h *Holder) SetALPNChallenge(answer func(*tls.ClientHelloInfo) (*tls.Certificate, error)) {
	h.alpn = answer
}

// GetCertificate is tls.Config.GetCertificate: the certificate in memory of the server name. A
// server name that is not one of the hosts, a missing one, and a host without a certificate yet
// fail the handshake; nothing here starts an order.
func (h *Holder) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	host := normalizeHost(hello.ServerName)
	if !slices.Contains(h.hosts, host) {
		return nil, ErrUnknownHost
	}
	if len(hello.SupportedProtos) == 1 && hello.SupportedProtos[0] == ACMETLS1Protocol {
		if h.alpn == nil {
			return nil, errors.New("tlsserve: no acme-tls/1 challenge is answered")
		}
		return h.alpn(hello)
	}
	if c := h.Certificate(host); c != nil {
		return c, nil
	}
	return nil, fmt.Errorf("tlsserve: no certificate for %q yet", host)
}

// Config returns the TLS configuration of the listener: TLS 1.2 or later with Go's default cipher
// suites, the certificates of h, and HTTP/2 and HTTP/1.1 (and acme-tls/1 when a challenge is
// answered).
func (h *Holder) Config() *tls.Config {
	protos := []string{"h2", "http/1.1"}
	if h.alpn != nil {
		protos = append(protos, ACMETLS1Protocol)
	}
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: h.GetCertificate,
		NextProtos:     protos,
	}
}
