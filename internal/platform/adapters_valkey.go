// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_valkey

package platform

import (
	"context"

	kvvalkey "github.com/mycroblog/my/internal/adapter/kv/valkey"
	valkeystream "github.com/mycroblog/my/internal/adapter/stream/valkey"
	"github.com/mycroblog/my/internal/adapter/valkey"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/platform/config"
)

// valkeyConn is the valkeyServer of a valkey.Client.
type valkeyConn struct{ valkey.Client }

func (c valkeyConn) Ping(ctx context.Context) error { return valkey.Ping(ctx, c.Client) }

// valkeyClient returns the client of valkey_url, connecting it on the first call; Close closes it.
func (a *App) valkeyClient(ctx context.Context) (valkey.Client, error) {
	if a.valkey == nil {
		c, err := valkey.Open(ctx, valkey.Options{URL: a.Config.ValkeyURL, TLSCAFile: a.Config.ValkeyTLSCAFile, Meter: a.Meter})
		if err != nil {
			return nil, err
		}
		a.valkey = valkeyConn{c}
	}
	return a.valkey.(valkeyConn).Client, nil
}

// newValkeyKV returns the key-value store of kv = valkey.
func (a *App) newValkeyKV(ctx context.Context, cfg *config.Config) (kernel.KeyValueStore, error) {
	c, err := a.valkeyClient(ctx)
	if err != nil {
		return nil, err
	}
	return kvvalkey.New(c, cfg.ValkeyKeyPrefix, a.Clock), nil
}

// newValkeyBus returns the stream.Bus of stream = valkey.
func (a *App) newValkeyBus(ctx context.Context, cfg *config.Config) (stream.Bus, error) {
	c, err := a.valkeyClient(ctx)
	if err != nil {
		return nil, err
	}
	return valkeystream.New(c, cfg.ValkeyKeyPrefix, a.ErrorReporter), nil
}
