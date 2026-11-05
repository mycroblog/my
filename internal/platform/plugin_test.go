// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	probev1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/probe/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/probe/v1/probev1connect"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

func TestNewPluginConn(t *testing.T) {
	h, rec := fakeplugin.Connect(fakeplugin.Options{})
	addr := fakeplugin.ServeUnix(t, h)
	cfg := &config.Config{Plugins: map[string]kind.Destination{kind.Mailer: {Address: addr}}}

	if _, err := NewPluginConn(cfg, "nope", kernel.NopMeter{}); err == nil {
		t.Error("an unknown kind: no error")
	}
	if _, err := NewPluginConn(cfg, kind.Billing, kernel.NopMeter{}); err == nil {
		t.Error("a kind without an address: no error")
	}
	conn, err := NewPluginConn(cfg, kind.Mailer, kernel.NopMeter{})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if rec.Calls() != 0 {
		t.Errorf("building a connection made %d calls", rec.Calls())
	}
	if _, err := probev1connect.NewProbeServiceClient(conn.RPC).Echo(t.Context(), &probev1.EchoRequest{}); err != nil {
		t.Errorf("Echo: %v", err)
	}
}
