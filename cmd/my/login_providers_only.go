// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mycroblog/my/internal/platform"
)

// printLoginProvidersOnlyImpact tells the operator what the instance setting login_providers_only
// means now: nothing without an enabled provider or with the setting off, or how many active
// users could not sign in. It never refuses anything: the administrator decides.
func printLoginProvidersOnlyImpact(ctx context.Context, app *platform.App, w io.Writer) error {
	impact, err := app.Identity.LoginProvidersOnlyImpact(ctx, "", "")
	if err != nil {
		return err
	}
	switch {
	case !impact.SettingOn:
		_, err = fmt.Fprintln(w, "login_providers_only is off: passwords and passkeys sign in")
	case impact.EnabledProviders == 0:
		_, err = fmt.Fprintln(w, "login_providers_only is on, but no login provider is enabled, so it does not apply: passwords and passkeys sign in")
	default:
		_, err = fmt.Fprintf(w, "sign-in is limited to %d login provider(s); %d active user(s) have no link to one and cannot sign in once their sessions end\n",
			impact.EnabledProviders, impact.UsersWithoutProvider)
	}
	return err
}
