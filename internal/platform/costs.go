// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/identity"
)

// The costs that the composition root gives the modules: the Argon2id parameters of the password
// hashes (ADR-0019 §3) and the generator of the actors' keys (nil: account.GenerateKey). Nothing
// changes them but the tests of this package (export_test.go), which assemble the app hundreds of
// times and are not testing those costs (ADR-0153 §2).
var (
	argon2Params = identity.DefaultArgon2Params
	generateKey  account.KeyGenerator
)
