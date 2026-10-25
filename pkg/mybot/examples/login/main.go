// SPDX-License-Identifier: MIT

// Command login gets a token for a bot with the Authorization Code flow and PKCE, receiving the code on a
// loopback redirect (RFC 8252 §7.3), as an alternative to issuing the token on the settings page of the server.
//
// Register the app on the settings page with the redirect URI http://127.0.0.1/callback (the server ignores the
// port of a loopback redirect URI), then run, with the environment of the bot:
//
//	MYBOT_SERVER=https://example.com MYBOT_CLIENT_ID=... MYBOT_CLIENT_SECRET=... go run ./examples/login
//
// It prints the URL of the authorization page: open it, signed in as the bot's account, and approve. The refresh
// token is printed as MYBOT_REFRESH_TOKEN=..., for the bot's first start. MYBOT_SCOPE changes the scopes it asks
// for (DefaultScope by default); MYBOT_CLIENT_SECRET is empty for a public app.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mycroblog/my/pkg/mykit"
)

func main() {
	if err := run(); err != nil {
		slog.Error("log in", "err", err)
		os.Exit(1)
	}
}

// run logs in with the app of the environment and prints the refresh token.
func run() error {
	scope := os.Getenv("MYBOT_SCOPE")
	if scope == "" {
		scope = DefaultScope
	}
	cfg := mykit.OAuthConfig{
		Server:       os.Getenv("MYBOT_SERVER"),
		ClientID:     os.Getenv("MYBOT_CLIENT_ID"),
		ClientSecret: os.Getenv("MYBOT_CLIENT_SECRET"),
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	tok, err := login(ctx, cfg, scope, func(u string) error {
		_, err := fmt.Fprintf(os.Stderr, "Open this URL, signed in as the bot's account, and approve:\n\n%s\n\n", u)
		return err
	})
	if err != nil {
		return err
	}
	fmt.Printf("MYBOT_REFRESH_TOKEN=%s\n", tok.RefreshToken)
	_, err = fmt.Fprintf(os.Stderr, "Granted scopes: %s\n", tok.Scope)
	return err
}
