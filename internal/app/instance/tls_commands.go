// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/domain/instance"
)

// ErrNoTLSCertificate reports that a host has no current certificate to act on.
var ErrNoTLSCertificate = errors.New("instance: the host has no TLS certificate")

// acmeProblemAlreadyRevoked is the problem of a revocation of a certificate the CA revoked
// already (RFC 8555 §6.7).
const acmeProblemAlreadyRevoked = "urn:ietf:params:acme:error:alreadyRevoked"

// TLSRenewalPlan is when the current certificate of a host is renewed (ADR-0163 §7), as
// `my tls status` shows it: the window that the CA suggests, inside which each serving process
// picks its time, or without a window the time derived from the lifetime of the certificate.
type TLSRenewalPlan struct {
	// WindowStart and WindowEnd are the CA's window (ARI); zero without one.
	WindowStart, WindowEnd time.Time
	// At is the renewal time without a window; zero with one.
	At time.Time
	// OtherCA is set when another CA than the configured one issued the certificate: the server
	// orders one from the configured CA at once.
	OtherCA bool
}

// RenewalPlan returns when c, a certificate of the hosts of the manager, is renewed. It asks the
// CA for its window, and falls back to the lifetime of c when the CA offers none; a failed read
// of the window returns the fallback with the error.
func (m *TLSManager) RenewalPlan(ctx context.Context, c instance.TLSCertificate) (TLSRenewalPlan, error) {
	if c.DirectoryURL != m.opts.Client.DirectoryURL() {
		return TLSRenewalPlan{OtherCA: true}, nil
	}
	leaf, err := parseLeaf(c.ChainPEM)
	if err != nil {
		return TLSRenewalPlan{}, err
	}
	info, err := m.opts.Client.RenewalInfo(ctx, leaf)
	switch {
	case err == nil && info.WindowEnd.After(info.WindowStart):
		return TLSRenewalPlan{WindowStart: info.WindowStart, WindowEnd: info.WindowEnd}, nil
	case err == nil || errors.Is(err, ErrACMERenewalInfoUnsupported):
		return TLSRenewalPlan{At: lifetimeRenewalTime(leaf)}, nil
	}
	return TLSRenewalPlan{At: lifetimeRenewalTime(leaf)}, fmt.Errorf("instance: read the renewal information of %s: %w", c.Host, err)
}

// TLSRevocation is what Revoke did.
type TLSRevocation struct {
	// Revoked is the certificate revoked.
	Revoked instance.TLSCertificate
	// ReplacementID is the certificate that replaces it, empty when its order failed with
	// ReplacementErr; the server orders one again in its rounds then.
	ReplacementID  string
	ReplacementErr error
}

// Revoke revokes the current certificate of host, for an operator who asks for it (my tls
// revoke): under the lease of the orders of host, it orders the certificate that replaces it,
// asks the CA to revoke it, records the revocation, and tells the other processes, which stop
// serving it. The replacement comes first, so that the processes move from the revoked
// certificate to the new one; when its order fails, the revocation goes on, and the processes
// serve the certificate of host issued before, if one is not revoked, or none until the server
// obtains a new one. It reports ErrTLSOrderInProgress while another process holds the lease, and
// ErrNoTLSCertificate when host has no certificate.
func (m *TLSManager) Revoke(ctx context.Context, host string, reason instance.TLSRevocationReason) (TLSRevocation, error) {
	host = normalizeChallengeHost(host)
	if !slices.Contains(m.opts.Hosts, host) {
		return TLSRevocation{}, fmt.Errorf("instance: %q is not a host of the TLS certificates", host)
	}
	code, ok := acmeRevocationReasons[reason]
	if !ok {
		return TLSRevocation{}, fmt.Errorf("instance: unknown revocation reason %q", reason)
	}
	var res TLSRevocation
	held, err := m.lease.hold(ctx, host, func(ctx context.Context) error {
		if err := m.Reload(ctx, host); err != nil {
			return err
		}
		current, err := m.opts.Service.CurrentTLSCertificateRecord(ctx, host)
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("%w: %s", ErrNoTLSCertificate, host)
		}
		if err != nil {
			return err
		}
		if current.DirectoryURL != m.opts.Client.DirectoryURL() {
			return fmt.Errorf("instance: the TLS certificate of %s was issued by %s, not by the configured CA", host, current.DirectoryURL)
		}
		leaf, err := parseLeaf(current.ChainPEM)
		if err != nil {
			return err
		}
		res.Revoked = current
		if err := m.order(ctx, host); err != nil {
			res.ReplacementErr = err
		} else {
			res.ReplacementID = m.CertificateID(host)
		}
		if err := m.revoke(ctx, leaf, code); err != nil {
			return err
		}
		if err := m.opts.Service.RecordTLSCertificateRevocation(context.WithoutCancel(ctx), current.ID, reason, ""); err != nil {
			return err
		}
		m.opts.Logger.InfoContext(ctx, "revoked a TLS certificate", slog.String("host", host),
			slog.String("serial", current.Serial), slog.String("reason", string(reason)))
		// Without a replacement, the certificate before it, or none, is current now.
		if err := m.Reload(context.WithoutCancel(ctx), host); err != nil {
			return err
		}
		if m.opts.Notify != nil {
			// The notice names the current certificate, or the revoked one when none is left: it
			// only tells the processes to read the database again.
			id := m.CertificateID(host)
			if id == "" {
				id = current.ID
			}
			m.opts.Notify(host, id)
		}
		return nil
	})
	if err == nil && !held {
		return res, ErrTLSOrderInProgress
	}
	return res, err
}

// revoke asks the CA to revoke leaf; a certificate that the CA revoked already counts as revoked.
func (m *TLSManager) revoke(ctx context.Context, leaf *x509.Certificate, code ACMERevocationReason) error {
	account, err := m.acmeAccount(ctx)
	if err != nil {
		return err
	}
	err = m.opts.Client.Revoke(ctx, account, leaf, code)
	var p *ACMEProblem
	if err != nil && !(errors.As(err, &p) && p.Type == acmeProblemAlreadyRevoked) {
		return fmt.Errorf("instance: revoke the TLS certificate %s: %w", leaf.SerialNumber.Text(16), err)
	}
	return nil
}

// acmeRevocationReasons are the reason codes (RFC 5280 §5.3.1) of the reasons of a revocation.
var acmeRevocationReasons = map[instance.TLSRevocationReason]ACMERevocationReason{
	instance.TLSRevocationKeyCompromise: ACMERevocationKeyCompromise,
	instance.TLSRevocationSuperseded:    ACMERevocationSuperseded,
	instance.TLSRevocationOperator:      ACMERevocationUnspecified,
}
