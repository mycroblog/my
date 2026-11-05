// SPDX-License-Identifier: AGPL-3.0-only

package client

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/adapter/plugin/kind"
)

// reloadInterval is how often a new connection looks at the TLS files for a change, with a stat of each. A short-lived
// client certificate (Vault PKI, cert-manager, SPIFFE) or a rotated CA renewed in place reaches the next connection
// within it, with no restart.
const reloadInterval = 5 * time.Second

// tlsFiles is a destination's CA pool and client certificate, read again when their files change.
type tlsFiles struct {
	host string
	ca   *watched[*x509.CertPool]
	cert *watched[*tls.Certificate]
}

// newTLSConfig always verifies the server, against the CA file or the system's roots; no setting turns that off
// (ADR-0146 §6). The minimum version is Go's default. The CA file and the client certificate are read now, so that a
// mistake stops the start, and again on a new connection after their files change.
func newTLSConfig(d kind.Destination, addr kind.Address) (*tls.Config, *tlsFiles, error) {
	u, err := url.Parse(addr.BaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse the address: %w", err)
	}
	f := &tlsFiles{host: u.Hostname()}
	c := new(tls.Config)
	if d.TLSCAFile != "" {
		f.ca, err = watch([]string{d.TLSCAFile}, func() (*x509.CertPool, error) { return loadCAFile(d.TLSCAFile) })
		if err != nil {
			return nil, nil, err
		}
		// The CA pool changes, and RootCAs is read only when the handshake starts from this config, so the server is
		// verified here instead, against the pool of the moment: InsecureSkipVerify only moves the verification to
		// VerifyConnection, which checks the chain and the name.
		c.InsecureSkipVerify = true
		c.VerifyConnection = f.verify
	}
	if d.TLSCertFile != "" {
		f.cert, err = watch([]string{d.TLSCertFile, d.TLSKeyFile}, func() (*tls.Certificate, error) {
			cert, err := tls.LoadX509KeyPair(d.TLSCertFile, d.TLSKeyFile)
			if err != nil {
				return nil, fmt.Errorf("read the client certificate: %w", err)
			}
			return &cert, nil
		})
		if err != nil {
			return nil, nil, err
		}
		c.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return f.cert.get(), nil }
	}
	return c, f, nil
}

// setInterval changes how often the files are looked at, for tests.
func (f *tlsFiles) setInterval(d time.Duration) {
	if f.ca != nil {
		f.ca.setInterval(d)
	}
	if f.cert != nil {
		f.cert.setInterval(d)
	}
}

func (f *tlsFiles) verify(cs tls.ConnectionState) error {
	if len(cs.PeerCertificates) == 0 {
		return errors.New("the plugin sent no certificate")
	}
	intermediates := x509.NewCertPool()
	for _, c := range cs.PeerCertificates[1:] {
		intermediates.AddCert(c)
	}
	_, err := cs.PeerCertificates[0].Verify(x509.VerifyOptions{
		DNSName:       f.host,
		Roots:         f.ca.get(),
		Intermediates: intermediates,
	})
	return err
}

func loadCAFile(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path) //nolint:gosec // G304: the operator's setting names the file
	if err != nil {
		return nil, fmt.Errorf("read the CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("the CA file %s holds no certificate", path)
	}
	return pool, nil
}

// watched is a value read from files, read again when a stat, at most once an interval, sees one of them change. A
// read that fails keeps the value it has, and the next look tries again: a certificate and its key renewed one after
// the other are read once both are in place.
type watched[T any] struct {
	paths    []string
	load     func() (T, error)
	mu       sync.Mutex
	interval time.Duration
	value    T
	stamps   []stamp
	checked  time.Time
}

// stamp is what tells that a file changed.
type stamp struct {
	mod  time.Time
	size int64
}

func (s stamp) same(o stamp) bool { return s.mod.Equal(o.mod) && s.size == o.size }

func watch[T any](paths []string, load func() (T, error)) (*watched[T], error) {
	value, err := load()
	if err != nil {
		return nil, err
	}
	w := &watched[T]{paths: paths, load: load, interval: reloadInterval, value: value, checked: time.Now()}
	if w.stamps, err = w.stat(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *watched[T]) setInterval(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.interval = d
}

func (w *watched[T]) get() T {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	if now.Sub(w.checked) < w.interval {
		return w.value
	}
	w.checked = now
	stamps, err := w.stat()
	if err != nil || slices.EqualFunc(stamps, w.stamps, stamp.same) {
		return w.value
	}
	if value, err := w.load(); err == nil {
		w.value, w.stamps = value, stamps
	}
	return w.value
}

func (w *watched[T]) stat() ([]stamp, error) {
	out := make([]stamp, len(w.paths))
	for i, p := range w.paths {
		fi, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		out[i] = stamp{mod: fi.ModTime(), size: fi.Size()}
	}
	return out, nil
}
