// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Severity tells whether a Finding of Readiness must be fixed before going live.
type Severity string

// Severities.
const (
	// SeverityError is a setting that breaks a production instance or weakens its security, such
	// as a key-encryption key that is not random. `my config check` exits 1 on one.
	SeverityError Severity = "error"
	// SeverityWarning is a setting that is usual in development and may be deliberate in
	// production, such as mail = none.
	SeverityWarning Severity = "warning"
)

// Finding is one problem of Readiness with a valid configuration.
type Finding struct {
	Severity Severity
	// Key is the setting the finding is about.
	Key string
	// Message is the technical detail in English.
	Message string
}

// Env returns the environment variable of the setting Key, or "" when there is none.
func (f Finding) Env() string { return Issue{Key: f.Key}.Env() }

// Readiness checks a valid configuration for the settings that work but do not suit a production
// instance: an address that other servers cannot reach, a secret that is not random or is a
// provider's test key, data kept in memory or in a temporary directory, or a feature left off.
// It returns nothing in the development profile. The errors come before the warnings, each in
// the order of the settings.
func Readiness(c *Config) []Finding {
	if c.Profile != "production" {
		return nil
	}
	var errs, warns []Finding
	errorf := func(key, format string, args ...any) {
		errs = append(errs, Finding{Severity: SeverityError, Key: key, Message: fmt.Sprintf(format, args...)})
	}
	warnf := func(key, format string, args ...any) {
		warns = append(warns, Finding{Severity: SeverityWarning, Key: key, Message: fmt.Sprintf(format, args...)})
	}

	if msg := publicDomainProblem(c.Host); msg != "" {
		errorf("host", "%s", msg)
	}
	if c.AccountDomain != c.Host {
		if msg := publicDomainProblem(c.AccountDomain); msg != "" {
			errorf("account_domain", "%s", msg)
		}
	}
	if u, err := url.Parse(c.WebOrigin); err == nil && u.Scheme != "https" {
		errorf("web_origin", "is %s, not https; browsers send the sign-in cookies only over https", c.WebOrigin)
	}
	if c.DB == "sqlite" {
		path, memory := sqlitePath(c.DatabaseURL)
		switch {
		case memory:
			errorf("database_url", "keeps the SQLite database in memory, so every restart loses all data")
		case inTempDir(path):
			warnf("database_url", "puts the SQLite database in the temporary directory %s, which the system may clear", filepath.Dir(path))
		}
	}
	if c.KeyManager == "local" && len(c.KEK) > 0 && !looksRandom(c.KEK) {
		errorf("kek", "is not random (%d distinct bytes in %d); generate one with `openssl rand -base64 32`", distinctBytes(c.KEK), len(c.KEK))
	}
	if c.Captcha != "none" && (captchaTestKeys[c.CaptchaSecretKey] || captchaTestKeys[c.CaptchaSiteKey]) {
		errorf("captcha_secret_key", "is a test key of %s, which lets every request through or blocks every one", c.Captcha)
	}

	if c.Queue == "inline" {
		warnf("queue", "is inline, which runs jobs synchronously in the process that enqueues them, for tests and development; use db or a queue service")
	}
	if c.Blob == "fs" && inTempDir(c.BlobFSDir) {
		warnf("blob_fs_dir", "is in the temporary directory %s, which the system may clear", c.BlobFSDir)
	}
	if c.Mail == "none" {
		warnf("mail", "is none, so no email reaches users: addresses go unverified and open sign-ups wait for a moderator's approval")
	}
	if c.VAPIDPrivateKey == "" {
		warnf("vapid_private_key", "is not set, so the server sends no Web Push notifications; generate one with `my config vapid-key`")
	}
	switch {
	case c.PaymentsProvider == "stripe" && (strings.HasPrefix(c.PaymentsStripeSecretKey, "sk_test_") ||
		strings.HasPrefix(c.PaymentsStripeSecretKey, "rk_test_")):
		warnf("payments_stripe_secret_key", "is a test-mode key, so supporters are not charged")
	case c.PaymentsProvider == "payjp" && strings.HasPrefix(c.PaymentsPayjpSecretKey, "sk_test_"):
		warnf("payments_payjp_secret_key", "is a test-mode key, so supporters are not charged")
	}
	return append(errs, warns...)
}

// publicDomainProblem describes why domain (host or account_domain, valid by checkDomain) is not
// a domain that other servers reach, or returns "".
func publicDomainProblem(domain string) string {
	if h, _, err := net.SplitHostPort(domain); err == nil {
		return fmt.Sprintf("has a port (%s); other servers reach https://%s/ on port 443 only", domain, h)
	}
	name := domain
	if name == "localhost" || !strings.Contains(name, ".") {
		return fmt.Sprintf("%s is not a public domain name, so other servers cannot reach it", name)
	}
	for _, tld := range reservedTLDs {
		if strings.HasSuffix(name, "."+tld) {
			return fmt.Sprintf("%s is under .%s, a name reserved for local use or tests, so other servers cannot reach it", name, tld)
		}
	}
	return ""
}

// reservedTLDs are the top-level domains that never resolve on the public internet (RFC 2606,
// RFC 6761, RFC 6762, RFC 8375).
var reservedTLDs = []string{"localhost", "local", "test", "example", "invalid", "internal", "home.arpa"}

// sqlitePath returns the file of an SQLite database_url ("my.db", "file:my.db?_pragma=...",
// "file:///var/lib/my/my.db"), and whether it is an in-memory database instead.
func sqlitePath(dsn string) (path string, memory bool) {
	rest, query, _ := strings.Cut(dsn, "?")
	if strings.HasPrefix(rest, "file:") {
		rest = strings.TrimPrefix(strings.TrimPrefix(rest, "file:"), "//")
	}
	if q, err := url.ParseQuery(query); err == nil && q.Get("mode") == "memory" {
		return "", true
	}
	if rest == "" || rest == ":memory:" {
		return "", true
	}
	return rest, false
}

// inTempDir reports whether the absolute path is in a temporary directory that the system may
// clear on reboot. A relative path is relative to the working directory, so it is not.
func inTempDir(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	path = filepath.Clean(path)
	for _, dir := range []string{os.TempDir(), "/tmp", "/var/tmp", "/dev/shm"} {
		dir = filepath.Clean(dir)
		if path == dir || strings.HasPrefix(path, dir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// looksRandom reports whether a 32-byte key has as many distinct bytes as random bytes have:
// 32 random bytes have about 30, and fewer than 16 with a probability far below 2^-100, while a
// key typed by hand, a repeated byte, or zeros have few.
func looksRandom(b []byte) bool { return distinctBytes(b) >= 16 }

func distinctBytes(b []byte) int {
	var seen [256]bool
	n := 0
	for _, c := range b {
		if !seen[c] {
			seen[c] = true
			n++
		}
	}
	return n
}

// captchaTestKeys are the keys that the CAPTCHA services publish for tests, which accept or
// refuse every response: Cloudflare Turnstile, hCaptcha, and Google reCAPTCHA v2.
var captchaTestKeys = map[string]bool{
	"1x00000000000000000000AA":                   true,
	"2x00000000000000000000AB":                   true,
	"1x00000000000000000000BB":                   true,
	"2x00000000000000000000BB":                   true,
	"3x00000000000000000000FF":                   true,
	"1x0000000000000000000000000000000AA":        true,
	"2x0000000000000000000000000000000AA":        true,
	"3x0000000000000000000000000000000AA":        true,
	"10000000-ffff-ffff-ffff-000000000001":       true,
	"20000000-ffff-ffff-ffff-000000000002":       true,
	"30000000-ffff-ffff-ffff-000000000003":       true,
	"0x0000000000000000000000000000000000000000": true,
	"6LeIxAcTAAAAAJcZVRqyHh71UMIEGqQ_MXjiZKhI":   true,
	"6LeIxAcTAAAAAGG-vFI1TnRWxMZNFuojJ4WifJWe":   true,
}
