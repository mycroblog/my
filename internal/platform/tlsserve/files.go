// SPDX-License-Identifier: AGPL-3.0-only

package tlsserve

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// FileCheckInterval is how often a FileSource looks at the modification times of its files.
const FileCheckInterval = time.Minute

// LoadKeyPair reads the certificate chain (PEM) at certFile and its key (PEM) at keyFile, and
// fails when the key does not match the certificate. The returned certificate has its Leaf.
func LoadKeyPair(certFile, keyFile string) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	if cert.Leaf == nil {
		if cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0]); err != nil {
			return nil, err
		}
	}
	return &cert, nil
}

// FileSource puts the certificate of the operator's files into a Holder (tls = files), for every
// host of the Holder that the certificate covers, and loads them again when either file changes.
type FileSource struct {
	holder   *Holder
	certFile string
	keyFile  string
	logger   *slog.Logger
	// stamp is the modification times of the files last loaded.
	stamp [2]time.Time
}

// NewFileSource returns the source of the certificate at certFile and its key at keyFile.
func NewFileSource(h *Holder, certFile, keyFile string, logger *slog.Logger) *FileSource {
	return &FileSource{holder: h, certFile: certFile, keyFile: keyFile, logger: logger}
}

func (s *FileSource) modTimes() ([2]time.Time, error) {
	var out [2]time.Time
	for i, name := range []string{s.certFile, s.keyFile} {
		fi, err := os.Stat(name)
		if err != nil {
			return out, err
		}
		out[i] = fi.ModTime()
	}
	return out, nil
}

// Load reads the files and puts the certificate into the Holder for every host it covers. It
// fails, changing nothing, when the files cannot be read, the key does not match, or the
// certificate covers none of the hosts.
func (s *FileSource) Load() error {
	stamp, err := s.modTimes()
	if err != nil {
		return fmt.Errorf("tls certificate files: %w", err)
	}
	cert, err := LoadKeyPair(s.certFile, s.keyFile)
	if err != nil {
		return fmt.Errorf("tls certificate files: %w", err)
	}
	var covered []string
	for _, host := range s.holder.Hosts() {
		if cert.Leaf.VerifyHostname(host) == nil {
			covered = append(covered, host)
		}
	}
	if len(covered) == 0 {
		return fmt.Errorf("tls certificate files: %s covers none of %v", s.certFile, s.holder.Hosts())
	}
	for _, host := range covered {
		if err := s.holder.Set(host, cert); err != nil {
			return err
		}
	}
	s.stamp = stamp
	s.logger.Info("TLS certificate loaded", slog.String("file", s.certFile), slog.Any("hosts", covered),
		slog.Time("not_after", cert.Leaf.NotAfter))
	return nil
}

// Reload loads the files again when the modification time of either has changed since the last
// load, and reports whether it did. A failure keeps the certificate in memory, so a half-written
// pair (a new certificate before its new key) is picked up on a later check.
func (s *FileSource) Reload() (bool, error) {
	stamp, err := s.modTimes()
	if err != nil {
		return false, fmt.Errorf("tls certificate files: %w", err)
	}
	if stamp == s.stamp {
		return false, nil
	}
	if err := s.Load(); err != nil {
		return false, err
	}
	return true, nil
}

// Watch calls Reload every interval until ctx ends, logging the failures.
func (s *FileSource) Watch(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := s.Reload(); err != nil && !errors.Is(err, context.Canceled) {
				s.logger.WarnContext(ctx, "TLS certificate not reloaded; the previous one is still served", slog.Any("error", err))
			}
		}
	}
}
