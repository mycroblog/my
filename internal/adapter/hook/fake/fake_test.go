// SPDX-License-Identifier: AGPL-3.0-only

package fake_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/hook/fake"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestContract(t *testing.T) {
	testkit.HookPluginContract(t, func(*testing.T) testkit.HookPluginHarness {
		p := fake.New()
		return testkit.HookPluginHarness{
			Plugin: p, Answer: p.Answer,
			Fail: func() { p.Fail(apphook.ErrPluginFailed) },
		}
	})
}
