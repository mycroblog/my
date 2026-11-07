// SPDX-License-Identifier: AGPL-3.0-only

package testkit_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/blobtest"
)

func TestMemoryBlobStoreContract(t *testing.T) {
	blobtest.BlobStoreContract(t, func(*testing.T) blob.Store { return testkit.NewMemoryBlobStore(testkit.NewClock(start)) })
}
