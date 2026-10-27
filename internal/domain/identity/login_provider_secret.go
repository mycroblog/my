// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// LoginProviderSecretColumn names the column of a sealed client secret in its additional
// authenticated data, which also binds the secret's row ID.
const LoginProviderSecretColumn = "login_provider_secrets.sealed_secret"

// MaxLoginProviderClientSecret is the number of bytes of a client secret.
const MaxLoginProviderClientSecret = 1024

// ErrInvalidLoginProviderClientSecret reports a client secret that is empty, too long, or has
// control characters.
var ErrInvalidLoginProviderClientSecret = errors.New("identity: invalid login provider client_secret")

// NormalizeLoginProviderClientSecret trims the white space around a client secret that an
// administrator entered.
func NormalizeLoginProviderClientSecret(secret string) string { return strings.TrimSpace(secret) }

// ValidateLoginProviderClientSecret checks a normalized client secret: 1 to
// MaxLoginProviderClientSecret bytes of valid UTF-8 without control characters.
func ValidateLoginProviderClientSecret(secret string) error {
	if secret == "" || len(secret) > MaxLoginProviderClientSecret || !utf8.ValidString(secret) ||
		strings.ContainsFunc(secret, unicode.IsControl) {
		return ErrInvalidLoginProviderClientSecret
	}
	return nil
}

// LoginProviderCallbackPath is the path of the callback of a provider at this server, which the
// administrator registers at the provider: /auth/external/<id>/callback.
func LoginProviderCallbackPath(providerID string) string {
	return "/auth/external/" + providerID + "/callback"
}
