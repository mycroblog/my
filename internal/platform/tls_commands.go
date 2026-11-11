// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/mycroblog/my/internal/app/instance"
	domaininstance "github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/internal/platform/tlsserve"
)

// TLSRecentOrders is how many of the latest orders of each host TLSStatus returns.
const TLSRecentOrders = 5

// ErrTLSNotACME refuses to order or revoke a certificate when the server does not obtain its
// certificates with ACME (tls other than acme).
var ErrTLSNotACME = errors.New("the server does not obtain its certificates with ACME")

// TLSStatus is the state of the certificates that the server serves itself (my tls status).
type TLSStatus struct {
	// Mode is the tls setting.
	Mode string
	// Hosts are the hosts of tls = acme, with their certificates.
	Hosts []TLSHostStatus
	// File is the certificate of tls_cert_file with tls = files; FileErr says why it is nil.
	File    *x509.Certificate
	FileErr error
}

// TLSHostStatus is the state of the certificate of a host with tls = acme.
type TLSHostStatus struct {
	Host string
	// Certificate is the current certificate, and Leaf its leaf; nil when the host has none.
	Certificate *domaininstance.TLSCertificate
	Leaf        *x509.Certificate
	// Renewal is when it is renewed; RenewalErr reports a failed read of the CA's window, when
	// Renewal is derived from the lifetime instead.
	Renewal    instance.TLSRenewalPlan
	RenewalErr error
	// Backoff is the number of orders since the current certificate that obtained none, and
	// when the next one may go out.
	Backoff domaininstance.TLSOrderBackoff
	// Orders are the latest orders, newest first (TLSRecentOrders).
	Orders []domaininstance.TLSOrderAttemptResult
}

// TLSStatus returns the state of the certificates of the hosts: from the database with tls =
// acme, from tls_cert_file with tls = files, and nothing with tls = off.
func (a *App) TLSStatus(ctx context.Context) (TLSStatus, error) {
	st := TLSStatus{Mode: a.Config.TLS}
	switch a.Config.TLS {
	case config.TLSFiles:
		pair, err := tls.LoadX509KeyPair(a.Config.TLSCertFile, a.Config.TLSKeyFile)
		if err != nil {
			st.FileErr = err
		} else {
			st.File = pair.Leaf
		}
		return st, nil
	case config.TLSACME:
	default:
		return st, nil
	}
	m, err := a.tlsCommandManager()
	if err != nil {
		return st, err
	}
	for _, host := range m.Hosts() {
		h := TLSHostStatus{Host: host}
		c, err := a.Instance.CurrentTLSCertificateRecord(ctx, host)
		switch {
		case err == nil:
			h.Certificate = &c
			if h.Leaf, err = parseChainLeaf(c.ChainPEM); err != nil {
				return st, err
			}
			h.Renewal, h.RenewalErr = m.RenewalPlan(ctx, c)
		case !errors.Is(err, instance.ErrNotFound):
			return st, err
		}
		if h.Backoff, err = a.Instance.TLSOrderBackoff(ctx, host); err != nil {
			return st, err
		}
		if h.Orders, err = a.Instance.RecentTLSOrders(ctx, host, TLSRecentOrders); err != nil {
			return st, err
		}
		st.Hosts = append(st.Hosts, h)
	}
	return st, nil
}

// RenewTLS orders a certificate of host now (my tls renew), skipping its renewal time and the
// wait after failed orders, and returns the certificate obtained. The processes that serve host
// load it from the stream broker's notice, or from their periodic check.
func (a *App) RenewTLS(ctx context.Context, host string) (domaininstance.TLSCertificate, error) {
	m, err := a.tlsOrderingManager()
	if err != nil {
		return domaininstance.TLSCertificate{}, err
	}
	if err := m.OrderNow(ctx, host); err != nil {
		return domaininstance.TLSCertificate{}, err
	}
	return a.Instance.CurrentTLSCertificateRecord(ctx, normalizeTLSHost(host))
}

// RevokeTLS revokes the current certificate of host (my tls revoke): it orders its replacement,
// asks the CA to revoke it, records the revocation, and notifies the processes that serve host,
// which stop serving it (instance.TLSManager.Revoke).
func (a *App) RevokeTLS(ctx context.Context, host string, reason domaininstance.TLSRevocationReason) (instance.TLSRevocation, error) {
	m, err := a.tlsOrderingManager()
	if err != nil {
		return instance.TLSRevocation{}, err
	}
	return m.Revoke(ctx, host, reason)
}

// tlsOrderingManager returns the manager that orders from this process for a command: the
// processes that serve the hosts answer the CA's challenges from the shared kv store, which
// kv = memory is not.
func (a *App) tlsOrderingManager() (*instance.TLSManager, error) {
	m, err := a.tlsCommandManager()
	if err != nil {
		return nil, err
	}
	if a.Config.KV == "memory" {
		return nil, errors.New("with kv = memory the server keeps the answers to the certificate authority's checks in its own memory, where this command cannot put them; set kv to db or valkey")
	}
	return m, nil
}

// tlsCommandManager returns the manager of tls = acme for a command: the App's own when it has
// one, or one whose certificates go to a holder of its own (a process that does not listen with
// TLS).
func (a *App) tlsCommandManager() (*instance.TLSManager, error) {
	switch a.Config.TLS {
	case config.TLSACME:
	case config.TLSFiles:
		return nil, fmt.Errorf("%w: with tls = files, the certificate comes from tls_cert_file and tls_key_file; renew or revoke it with the tool that writes them", ErrTLSNotACME)
	default:
		return nil, fmt.Errorf("%w: tls is off", ErrTLSNotACME)
	}
	if a.TLSManager != nil {
		return a.TLSManager, nil
	}
	return a.newTLSManager(tlsserve.NewHolder(tlsHosts(a.Config)...))
}

// normalizeTLSHost returns host as the manager compares it: lower case, without a port or a
// trailing dot.
func normalizeTLSHost(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

// parseChainLeaf returns the leaf of a chain of PEM certificates.
func parseChainLeaf(chainPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(chainPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("the TLS chain does not start with a PEM certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}
