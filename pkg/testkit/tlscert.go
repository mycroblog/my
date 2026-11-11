// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TLSCertificate is a self-signed certificate for tests, in the forms the server reads.
type TLSCertificate struct {
	// CertPEM is the certificate in PEM; KeyPEM its key in PEM (PKCS #8), and KeyPKCS8 the same
	// key in DER.
	CertPEM  []byte
	KeyPEM   []byte
	KeyPKCS8 []byte
	Leaf     *x509.Certificate
}

// Pool returns a certificate pool that trusts only c.
func (c TLSCertificate) Pool() *x509.CertPool {
	p := x509.NewCertPool()
	p.AddCert(c.Leaf)
	return p
}

// NewTLSCertificate returns a self-signed ECDSA certificate for hosts (DNS names or IP
// addresses), valid from notBefore for validity, with the serial number serial.
func NewTLSCertificate(t testing.TB, serial int64, notBefore time.Time, validity time.Duration, hosts ...string) TLSCertificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		NotBefore:    notBefore,
		NotAfter:     notBefore.Add(validity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		// A self-signed leaf is its own root for the pool of Pool.
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	if len(hosts) > 0 {
		tmpl.Subject = pkix.Name{CommonName: hosts[0]}
	}
	tmpl.DNSNames = append(tmpl.DNSNames, hosts...)
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return TLSCertificate{
		CertPEM:  pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		KeyPEM:   pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}),
		KeyPKCS8: pkcs8,
		Leaf:     leaf,
	}
}

// WriteFiles writes the certificate and its key to cert.pem and key.pem in dir, setting their
// modification time to modTime (zero keeps the current time), and returns their paths.
func (c TLSCertificate) WriteFiles(t testing.TB, dir string, modTime time.Time) (certFile, keyFile string) {
	t.Helper()
	certFile, keyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	for name, data := range map[string][]byte{certFile: c.CertPEM, keyFile: c.KeyPEM} {
		if err := os.WriteFile(name, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if !modTime.IsZero() {
			if err := os.Chtimes(name, modTime, modTime); err != nil {
				t.Fatal(err)
			}
		}
	}
	return certFile, keyFile
}
