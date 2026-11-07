// SPDX-License-Identifier: AGPL-3.0-only

package httpsig_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/httpsig"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
)

func TestContract(t *testing.T) {
	a := httpsig.New()
	federationtest.HTTPSignatureContract(t, a, a)
}
