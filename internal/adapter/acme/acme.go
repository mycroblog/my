// SPDX-License-Identifier: AGPL-3.0-only

// Package acme is the adapter of instance.ACMEClient over github.com/mholt/acmez/v3 (Apache-2.0):
// the ACME protocol (RFC 8555) with renewal information (RFC 9773) and the profiles of a CA
// (ADR-0163 §2). It holds no state of its own: the accounts, the orders, and the certificates are
// kept by the instance module, and the answers of the challenges by the instance.ACMESolver of
// each order.
package acme

import (
	"context"
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/mholt/acmez/v3"
	acmeproto "github.com/mholt/acmez/v3/acme"

	appinstance "github.com/mycroblog/my/internal/app/instance"
)

// userAgent identifies the server to the CA, which asks clients to name themselves.
const userAgent = "my"

// Options configure a Client.
type Options struct {
	// DirectoryURL is the URL of the CA's directory.
	DirectoryURL string
	// HTTPClient reaches the CA. The CA is a destination the operator configures, so it is not
	// the SSRF-guarded kernel.HTTPClient; internal/platform builds one that follows no
	// redirects.
	HTTPClient *http.Client
	// Logger logs the protocol at the debug level; nil logs nothing.
	Logger *slog.Logger
	// PollInterval is the wait between polls of an authorization or an order when the CA sends
	// no Retry-After; zero is acmez's default. Tests only.
	PollInterval time.Duration
}

// Client is the instance.ACMEClient of one CA.
type Client struct {
	dir string
	// proto is shared by every order: it keeps the CA's directory and nonces.
	proto *acmeproto.Client
}

var _ appinstance.ACMEClient = (*Client)(nil)

// New returns the client of opts.DirectoryURL.
func New(opts Options) (*Client, error) {
	if opts.DirectoryURL == "" {
		return nil, errors.New("acme: no directory URL")
	}
	if opts.HTTPClient == nil {
		return nil, errors.New("acme: no HTTP client")
	}
	return &Client{
		dir: opts.DirectoryURL,
		proto: &acmeproto.Client{
			Directory: opts.DirectoryURL, HTTPClient: opts.HTTPClient, UserAgent: userAgent,
			Logger: opts.Logger, PollInterval: opts.PollInterval,
		},
	}, nil
}

// DirectoryURL implements instance.ACMEClient.
func (c *Client) DirectoryURL() string { return c.dir }

// RegisterAccount implements instance.ACMEClient.
func (c *Client) RegisterAccount(ctx context.Context, key crypto.Signer, contact []string) (string, error) {
	a, err := c.proto.NewAccount(ctx, acmeproto.Account{Contact: contact, TermsOfServiceAgreed: true, PrivateKey: key})
	if err != nil {
		return "", wrap("register the account", err)
	}
	if a.Location == "" {
		return "", errors.New("acme: the CA returned no account URL")
	}
	return a.Location, nil
}

// account returns the registered account of a, with its status from the CA.
func (c *Client) account(ctx context.Context, a appinstance.ACMEAccount) (acmeproto.Account, error) {
	got, err := c.proto.GetAccount(ctx, acmeproto.Account{PrivateKey: a.Key})
	if err != nil {
		return acmeproto.Account{}, wrap("look up the account", err)
	}
	if got.Location == "" {
		got.Location = a.URL
	}
	return got, nil
}

// Order implements instance.ACMEClient.
func (c *Client) Order(ctx context.Context, req appinstance.ACMEOrderRequest) (string, error) {
	if req.Solver == nil || len(req.Challenges) == 0 {
		return "", errors.New("acme: an order needs a solver and a challenge type")
	}
	solvers := map[string]acmez.Solver{}
	for _, t := range req.Challenges {
		switch t {
		case appinstance.ACMEChallengeTLSALPN01:
			solvers[acmeproto.ChallengeTypeTLSALPN01] = solver{typ: t, s: req.Solver}
		case appinstance.ACMEChallengeHTTP01:
			solvers[acmeproto.ChallengeTypeHTTP01] = solver{typ: t, s: req.Solver}
		default:
			return "", fmt.Errorf("acme: the %q challenge is not answered", t)
		}
	}
	account, err := c.account(ctx, req.Account)
	if err != nil {
		return "", err
	}
	csr, err := acmez.NewCSR(req.Key, []string{req.Host})
	if err != nil {
		return "", fmt.Errorf("acme: the certificate request of %s: %w", req.Host, err)
	}
	params, err := acmez.OrderParametersFromCSR(account, csr)
	if err != nil {
		return "", fmt.Errorf("acme: the order of %s: %w", req.Host, err)
	}
	params.Profile, params.Replaces = req.Profile, req.Replaces
	z := &acmez.Client{Client: c.proto, ChallengeSolvers: solvers}
	chains, err := z.ObtainCertificate(ctx, params)
	if err != nil {
		return "", wrap("order the certificate of "+req.Host, err)
	}
	if len(chains) == 0 || len(chains[0].ChainPEM) == 0 {
		return "", fmt.Errorf("acme: the CA returned no certificate for %s", req.Host)
	}
	return string(chains[0].ChainPEM), nil
}

// Revoke implements instance.ACMEClient.
func (c *Client) Revoke(ctx context.Context, a appinstance.ACMEAccount, leaf *x509.Certificate, reason appinstance.ACMERevocationReason) error {
	account, err := c.account(ctx, a)
	if err != nil {
		return err
	}
	if err := c.proto.RevokeCertificate(ctx, account, leaf, nil, int(reason)); err != nil {
		return wrap("revoke the certificate", err)
	}
	return nil
}

// RenewalInfo implements instance.ACMEClient.
func (c *Client) RenewalInfo(ctx context.Context, leaf *x509.Certificate) (appinstance.ACMERenewalInfo, error) {
	ri, err := c.proto.GetRenewalInfo(ctx, leaf)
	if errors.Is(err, acmeproto.ErrUnsupported) {
		return appinstance.ACMERenewalInfo{}, fmt.Errorf("%w: %w", appinstance.ErrACMERenewalInfoUnsupported, err)
	}
	if err != nil {
		return appinstance.ACMERenewalInfo{}, wrap("read the renewal information", err)
	}
	info := appinstance.ACMERenewalInfo{
		WindowStart: ri.SuggestedWindow.Start, WindowEnd: ri.SuggestedWindow.End, ExplanationURL: ri.ExplanationURL,
	}
	if ri.RetryAfter != nil {
		info.RetryAfter = *ri.RetryAfter
	}
	return info, nil
}

// wrap returns err of the step what, with the CA's problem as an *instance.ACMEProblem when there
// is one.
func wrap(what string, err error) error {
	var p acmeproto.Problem
	if errors.As(err, &p) {
		return fmt.Errorf("acme: %s: %w", what, &problem{ACMEProblem: appinstance.ACMEProblem{Type: p.Type, Detail: p.Detail}, err: err})
	}
	return fmt.Errorf("acme: %s: %w", what, err)
}

// problem is the CA's problem with the error that reported it.
type problem struct {
	appinstance.ACMEProblem
	err error
}

func (p *problem) Error() string { return p.err.Error() }

func (p *problem) As(target any) bool {
	if t, ok := target.(**appinstance.ACMEProblem); ok {
		*t = &p.ACMEProblem
		return true
	}
	return false
}

func (p *problem) Unwrap() error { return p.err }

// solver answers the challenges of one type with the order's instance.ACMESolver.
type solver struct {
	typ string
	s   appinstance.ACMESolver
}

func (s solver) challenge(ch acmeproto.Challenge) appinstance.ACMEChallenge {
	return appinstance.ACMEChallenge{
		Type: s.typ, Host: ch.Identifier.Value, Token: ch.Token, KeyAuthorization: ch.KeyAuthorization,
	}
}

func (s solver) Present(ctx context.Context, ch acmeproto.Challenge) error {
	return s.s.Present(ctx, s.challenge(ch))
}

func (s solver) CleanUp(ctx context.Context, ch acmeproto.Challenge) error {
	return s.s.CleanUp(ctx, s.challenge(ch))
}
