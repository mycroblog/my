// SPDX-License-Identifier: MIT

// Command commands is the example bot that answers the commands help, ping, and dice.
//
// It reads the server and the token from the environment: MYBOT_SERVER (such as https://example.com),
// MYBOT_CLIENT_ID, MYBOT_CLIENT_SECRET, and MYBOT_REFRESH_TOKEN, the app and the token that the settings page of
// the server issued to the bot's own account, and MYBOT_STATE, the state file (./mybot-state.json by default).
// It stops on SIGINT or SIGTERM.
package main

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/signal"
	"syscall"

	"github.com/mycroblog/my/pkg/mybot"
	"github.com/mycroblog/my/pkg/mykit"
)

func main() {
	if err := run(); err != nil {
		slog.Error("run the bot", "err", err)
		os.Exit(1)
	}
}

// run configures the bot from the environment and runs it until a signal stops it.
func run() error {
	cfg, err := configFromEnv()
	if err != nil {
		return err
	}
	bot, err := newBot(cfg, roll)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return bot.Run(ctx)
}

// roll returns a number from 1 to n, a throw of a die of n sides.
func roll(n int) int {
	return rand.IntN(n) + 1 //nolint:gosec // G404: a die of a game, not a secret
}

// configFromEnv reads the Config of the bot from the environment.
func configFromEnv() (mybot.Config, error) {
	path := os.Getenv("MYBOT_STATE")
	if path == "" {
		path = mybot.DefaultStatePath
	}
	store, err := mybot.NewFileStore(path)
	if err != nil {
		return mybot.Config{}, err
	}
	return mybot.Config{
		Server: os.Getenv("MYBOT_SERVER"),
		OAuth: &mykit.OAuthConfig{
			ClientID:     os.Getenv("MYBOT_CLIENT_ID"),
			ClientSecret: os.Getenv("MYBOT_CLIENT_SECRET"),
		},
		RefreshToken: os.Getenv("MYBOT_REFRESH_TOKEN"),
		Store:        store,
		UserAgent:    "mybot-example-commands/1.0",
	}, nil
}
