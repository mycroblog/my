// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mycroblog/my/internal/app/instance"
	domaininstance "github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

// tlsAppOptions are the options of the App that `my tls` builds. Tests set ACMEClient.
var tlsAppOptions platform.AppOptions

// runTLS implements `my tls <subcommand>`: show, renew, and revoke the certificates that the
// server serves itself (ADR-0163 §11).
func runTLS(args []string, stdout, stderr io.Writer) int {
	sub, code := dispatch("tls", args, []string{"status", "renew", "revoke"}, stderr, tlsUsage)
	if code >= 0 {
		return code
	}
	var host, reason string
	c, code := parseSubcommand("my tls "+sub, args[1:], 0, stderr, tlsUsage, func(fs *flag.FlagSet) {
		if sub == "status" {
			return
		}
		fs.StringVar(&host, "host", "", "the host whose certificate to "+sub)
		if sub == "revoke" {
			fs.StringVar(&reason, "reason", "", "why: key_compromise, superseded, or operator")
		}
	})
	if code >= 0 {
		return code
	}
	if sub != "status" && host == "" {
		_, _ = fmt.Fprintf(stderr, "%s: -host is required\n\n", c.name)
		tlsUsage(stderr)
		return 2
	}
	if sub == "revoke" && !domaininstance.TLSRevocationReason(reason).Valid() {
		_, _ = fmt.Fprintf(stderr, "%s: -reason must be key_compromise, superseded, or operator\n\n", c.name)
		tlsUsage(stderr)
		return 2
	}
	ctx := context.Background()
	cfg, _, err := config.Load(config.Source{File: *c.file})
	if err != nil {
		printLoadError(stderr, err)
		return 1
	}
	app, err := platform.NewApp(ctx, cfg, tlsAppOptions)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	defer func() { _ = app.Close() }()
	switch sub {
	case "status":
		return tlsStatus(ctx, app, c.name, stdout, stderr)
	case "renew":
		return tlsRenew(ctx, app, c.name, host, stdout, stderr)
	}
	return tlsRevoke(ctx, app, c.name, host, domaininstance.TLSRevocationReason(reason), stdout, stderr)
}

// tlsStatus prints the certificate of each host, its renewal, and its latest orders.
func tlsStatus(ctx context.Context, app *platform.App, name string, stdout, stderr io.Writer) int {
	st, err := app.TLSStatus(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	now := app.Clock.Now()
	switch st.Mode {
	case config.TLSFiles:
		_, _ = fmt.Fprintf(stdout, "tls = files: the certificate comes from %s\n", app.Config.TLSCertFile)
		if st.FileErr != nil {
			_, _ = fmt.Fprintf(stderr, "%s: the certificate cannot be read: %v\n", name, st.FileErr)
			return 1
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		printLeaf(tw, st.File, now)
		_, _ = fmt.Fprintf(tw, "  names\t%s\n", strings.Join(st.File.DNSNames, ", "))
		_ = tw.Flush()
		return 0
	case config.TLSACME:
	default:
		_, _ = fmt.Fprintln(stdout, "tls is off: the server serves no certificate of its own")
		return 0
	}
	_, _ = fmt.Fprintf(stdout, "tls = acme: the certificates come from %s\n", app.Config.TLSACMEDirectory)
	for _, h := range st.Hosts {
		_, _ = fmt.Fprintf(stdout, "\n%s\n", h.Host)
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		if h.Certificate == nil {
			_, _ = fmt.Fprintln(tw, "  certificate\tnone: the server orders one")
		} else {
			printLeaf(tw, h.Leaf, now)
			_, _ = fmt.Fprintf(tw, "  renewal\t%s\n", describeRenewal(h.Renewal))
			if h.RenewalErr != nil {
				_, _ = fmt.Fprintf(tw, "  \t(the certificate authority's window could not be read: %v)\n", h.RenewalErr)
			}
		}
		if h.Backoff.Failures > 0 {
			_, _ = fmt.Fprintf(tw, "  failures\t%d order(s) in a row obtained no certificate; the next one waits until %s\n",
				h.Backoff.Failures, formatTime(h.Backoff.NotBefore))
		}
		if len(h.Orders) == 0 {
			_, _ = fmt.Fprintln(tw, "  orders\tnone")
		}
		for i, o := range h.Orders {
			label := ""
			if i == 0 {
				label = "orders"
			}
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\n", label, formatTime(o.Attempt.OccurredAt), describeOrder(o))
		}
		_ = tw.Flush()
	}
	return 0
}

// printLeaf prints the issuer, the serial, and the expiry of leaf.
func printLeaf(w io.Writer, leaf *x509.Certificate, now time.Time) {
	_, _ = fmt.Fprintf(w, "  certificate\tserial %s, issued by %s\n", leaf.SerialNumber.Text(16), leaf.Issuer.String())
	left := leaf.NotAfter.Sub(now)
	state := fmt.Sprintf("%d days left", int(left.Hours()/24))
	if left <= 0 {
		state = "expired"
	}
	_, _ = fmt.Fprintf(w, "  expires\t%s (%s)\n", formatTime(leaf.NotAfter), state)
}

// describeRenewal says when a certificate is renewed.
func describeRenewal(p instance.TLSRenewalPlan) string {
	switch {
	case p.OtherCA:
		return "now: another certificate authority issued it"
	case !p.WindowStart.IsZero():
		return fmt.Sprintf("at a time the server picks between %s and %s (the certificate authority's window)",
			formatTime(p.WindowStart), formatTime(p.WindowEnd))
	}
	return "at " + formatTime(p.At)
}

// describeOrder says how an order ended.
func describeOrder(o domaininstance.TLSOrderAttemptResult) string {
	switch {
	case o.CertificateID != "":
		return "issued"
	case o.Failure != nil:
		return "failed: " + o.Failure.Detail
	}
	return "no result: in progress, or its process stopped"
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// tlsRenew orders a certificate of host now.
func tlsRenew(ctx context.Context, app *platform.App, name, host string, stdout, stderr io.Writer) int {
	c, err := app.RenewTLS(ctx, host)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", name, describeTLSError(err))
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "obtained the certificate %s of %s, which expires %s\n", c.Serial, c.Host, formatTime(c.NotAfter))
	return 0
}

// tlsRevoke revokes the certificate of host and orders its replacement.
func tlsRevoke(ctx context.Context, app *platform.App, name, host string, reason domaininstance.TLSRevocationReason, stdout, stderr io.Writer) int {
	res, err := app.RevokeTLS(ctx, host, reason)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", name, describeTLSError(err))
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "revoked the certificate %s of %s (%s)\n", res.Revoked.Serial, res.Revoked.Host, reason)
	if res.ReplacementErr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: the replacement could not be ordered: %v\n%s: the server serves the certificate before it, if one is not revoked, and orders a new one in its next rounds; run my tls renew -host %s to order one now\n",
			name, res.ReplacementErr, name, res.Revoked.Host)
		return 0
	}
	_, _ = fmt.Fprintf(stdout, "replaced it with the certificate %s\n", res.ReplacementID)
	return 0
}

// describeTLSError explains the errors that the operator can act on.
func describeTLSError(err error) string {
	switch {
	case errors.Is(err, instance.ErrTLSOrderInProgress):
		return "another process is ordering the certificate of this host now; try again in a few minutes"
	case errors.Is(err, instance.ErrNoTLSCertificate):
		return "the host has no certificate to revoke"
	}
	return err.Error()
}

func tlsUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my tls status [-config <file>]
  my tls renew -host <host> [-config <file>]
  my tls revoke -host <host> -reason <key_compromise|superseded|operator> [-config <file>]

Shows and controls the certificates that the server serves itself (tls = acme
or tls = files).

status shows, for each host, the current certificate (its serial, issuer, and
expiry), when it is renewed, the orders in a row that obtained no certificate,
and the latest orders with their results. With tls = files it shows the
certificate of tls_cert_file.

renew orders a certificate of the host from the certificate authority now,
skipping its renewal time and the wait after failed orders. The processes that
serve the host answer the authority's checks, so they must be running, and
they start serving the new certificate within seconds through the stream
broker, or within 5 minutes without one.

revoke orders a certificate that replaces the current one of the host, asks the
certificate authority to revoke the current one, and records the revocation.
Every process stops serving the revoked certificate: it serves the replacement
or, when the replacement could not be ordered, the certificate of the host
issued before it, or none until the server obtains a new one. Use
key_compromise when the private key may be known to others.

renew and revoke need tls = acme and a kv other than memory. They exit with 1
when another process is ordering the certificate of the host at that moment.
`+docsLine("docs/operations/running.md"))
}
