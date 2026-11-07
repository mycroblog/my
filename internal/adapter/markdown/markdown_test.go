// SPDX-License-Identifier: AGPL-3.0-only

package markdown_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/markdown"
	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/pkg/testkit/instancetest"
)

func TestContract(t *testing.T) {
	instancetest.DocumentRendererContract(t, func(*testing.T) instance.DocumentRenderer { return markdown.New() })
}
