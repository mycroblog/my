// SPDX-License-Identifier: AGPL-3.0-only

package dbupgrade

import (
	"os"
	"testing"

	"github.com/mycroblog/my/pkg/testkit/pgtest"
)

func TestMain(m *testing.M) {
	os.Exit(pgtest.Main(m))
}
