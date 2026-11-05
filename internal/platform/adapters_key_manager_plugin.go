// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform

import (
	"context"
	"fmt"
	"io"

	keymanagerplugin "github.com/mycroblog/my/internal/adapter/keymanager/plugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

// newPluginKeyManager returns the KeyManager of the key_manager plugin after its self-test. Unlike every other kind, a
// plugin that does not answer in time or fails the test stops the start (ADR-0146 §14, §15): the server opens secrets as
// it starts, and it must not run on a plugin that may not protect them.
func newPluginKeyManager(ctx context.Context, cfg *config.Config, meter kernel.Meter) (kernel.KeyManager, io.Closer, error) {
	conn, err := NewPluginConn(cfg, kind.KeyManager, meter)
	if err != nil {
		return nil, nil, fmt.Errorf("key_manager = plugin: %w", err)
	}
	km, err := keymanagerplugin.Start(ctx, conn, keymanagerplugin.StartWait)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("key_manager = plugin: %w", err)
	}
	return km, km, nil
}
