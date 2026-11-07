// SPDX-License-Identifier: AGPL-3.0-only

package fake_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/hook/fake"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/pkg/testkit/hooktest"
)

func TestContract(t *testing.T) {
	hooktest.HookPluginContract(t, func(*testing.T) hooktest.HookPluginHarness {
		p := fake.New()
		return hooktest.HookPluginHarness{
			Plugin: p, Answer: p.Answer,
			Fail: func() { p.Fail(apphook.ErrPluginFailed) },
		}
	})
}
