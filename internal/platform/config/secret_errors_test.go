// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"strings"
	"testing"
)

func TestValidationErrorsDoNotQuoteSecretURLs(t *testing.T) {
	for _, tc := range []struct{ key, env string }{
		{"valkey_url", "MY_VALKEY_URL"},
	} {
		env := with(minimal(), "MY_KV", "valkey", tc.env, "valkey://user:s3cr3tpw@cache:notaport/0")
		_, _, err := Load(source(env, nil))
		if err == nil {
			continue // the database_url of this build accepts it
		}
		if msg := err.Error(); strings.Contains(msg, "s3cr3tpw") {
			t.Errorf("%s: the error quotes the password: %s", tc.key, msg)
		}
	}
}
