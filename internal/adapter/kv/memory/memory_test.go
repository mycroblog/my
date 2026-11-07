// SPDX-License-Identifier: AGPL-3.0-only

package memory_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/kv/memory"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/kerneltest"
)

func TestKeyValueStoreContract(t *testing.T) {
	kerneltest.KeyValueStoreContract(t, func(_ *testing.T, clock *testkit.Clock) kernel.KeyValueStore {
		return memory.New(clock)
	})
}
