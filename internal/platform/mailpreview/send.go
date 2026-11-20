// SPDX-License-Identifier: AGPL-3.0-only

package mailpreview

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Schemes are the color schemes that Send sends each email in: as the mail client prefers, and
// dark whatever it prefers (Dark).
var Schemes = []string{"light", "dark"}

// Send renders every entry of the catalog in locales and sends each to m in each of Schemes, its
// subject marked with the locale and the scheme, so that a mail server for development, such as
// Mailpit, shows every email the server sends. It returns how many it sent.
func Send(ctx context.Context, m kernel.Mailer, locales []string) (int, error) {
	bundle, err := Load()
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, locale := range locales {
		rendered, err := Render(bundle, locale)
		if err != nil {
			return sent, err
		}
		for _, r := range rendered {
			for _, scheme := range Schemes {
				msg := r.Message
				msg.Subject = fmt.Sprintf("[%s %s %s] %s", locale, scheme, r.Entry.Name(), msg.Subject)
				if scheme == "dark" {
					msg.HTML = Dark(msg.HTML)
				}
				if err := m.Send(ctx, msg); err != nil {
					return sent, fmt.Errorf("mailpreview: send %s in %s, %s: %w", r.Entry.Name(), locale, scheme, err)
				}
				sent++
			}
		}
	}
	return sent, nil
}
