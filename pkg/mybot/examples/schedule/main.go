// SPDX-License-Identifier: MIT

// Command schedule is the example bot that posts a greeting every morning.
//
// It reads the server and the token from the environment: MYBOT_SERVER (such as https://example.com),
// MYBOT_CLIENT_ID, MYBOT_CLIENT_SECRET, and MYBOT_REFRESH_TOKEN, the app and the token that the settings page of
// the server issued to the bot's own account, MYBOT_STATE, the state file (./mybot-state.json by default), and
// MYBOT_TIME_ZONE, the IANA time zone of the morning, such as Asia/Tokyo (UTC by default).
// It stops on SIGINT or SIGTERM.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	// The time zones, for a container image without them.
	_ "time/tzdata"

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
	bot, err := newBot(cfg, os.Getenv("MYBOT_TIME_ZONE"))
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return bot.Run(ctx)
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
		UserAgent:    "mybot-example-schedule/1.0",
	}, nil
}
