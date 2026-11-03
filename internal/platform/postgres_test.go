// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"os"
	"testing"

	"github.com/mycroblog/my/pkg/testkit/pgtest"
	"github.com/mycroblog/my/pkg/testkit/valkeytest"
)

func TestMain(m *testing.M) {
	noSecondConnections := checkSecondConnections()
	code := pgtest.Main(m)
	removeSQLiteTemplate()
	if code == 0 && !noSecondConnections() {
		code = 1
	}
	valkeytest.Close()
	os.Exit(code)
}

// newPostgresDatabase returns the URL of a new database with the product schema on the test server
// (package pgtest), for the few tests of this package that run on both dialects; most tests use
// SQLite and never start the server.
func newPostgresDatabase(t *testing.T) string {
	t.Helper()
	return pgtest.NewDatabase(t)
}
