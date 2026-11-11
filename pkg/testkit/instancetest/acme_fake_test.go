// SPDX-License-Identifier: AGPL-3.0-only

package instancetest_test

import (
	"testing"

	"github.com/mycroblog/my/pkg/testkit/instancetest"
)

// The fake CA passes the contract of the CA, so that the flows that use it test what the adapter
// does.
func TestFakeACMEClientContract(t *testing.T) {
	t.Parallel()
	instancetest.ACMEClientContract(t, instancetest.FakeACMEHarness)
}
