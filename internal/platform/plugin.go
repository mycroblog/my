// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform

import (
	"fmt"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

// NewPluginConn returns the connection of the kind's configured plugin, for the adapter of a port or a hook that the
// settings select. The server builds it only for a selected kind, so one that delegates nothing opens no connection
// (ADR-0146 §4). A build with the tag no_plugins has neither this nor those adapters: their file there reports
// config.ExcludedError instead, as newLiveKitTransport does.
func NewPluginConn(cfg *config.Config, name string, meter kernel.Meter) (*client.Conn, error) {
	k, ok := kind.ByName(name)
	if !ok {
		return nil, fmt.Errorf("plugin: unknown kind %q", name)
	}
	d := cfg.PluginDestination(name)
	if !d.Enabled() {
		return nil, fmt.Errorf("plugin: %s is required", k.Key(kind.SuffixAddress))
	}
	return client.New(client.Options{Kind: k, Destination: d, Meter: meter})
}
