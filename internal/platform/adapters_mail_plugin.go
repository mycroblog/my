// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform

import (
	mailplugin "github.com/mycroblog/my/internal/adapter/mail/plugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

// newPluginMailer returns the Mailer of mail = plugin (ADR-0146 §11). It opens no connection: a plugin that is down at
// start delays the email, and the server still starts (ADR-0146 §15).
func newPluginMailer(cfg *config.Config, meter kernel.Meter) (kernel.Mailer, error) {
	conn, err := NewPluginConn(cfg, kind.Mailer, meter)
	if err != nil {
		return nil, err
	}
	return mailplugin.New(conn, cfg.MailFrom)
}
