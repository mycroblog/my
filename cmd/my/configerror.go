// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/platform/config"
)

// printLoadError writes an error of config.Load to w. A *config.ValidationError is
// listed with the server:config.* messages in the locale of the process environment
// (LANGUAGE, LC_ALL, LC_MESSAGES, LANG); the English technical detail of each issue
// follows its localized line. Any other error is printed as is.
func printLoadError(w io.Writer, err error) {
	printLoadErrorEnv(w, err, os.LookupEnv)
}

// unknownEnvMessage describes an environment variable of config.UnknownEnv.
func unknownEnvMessage(name string) string {
	return fmt.Sprintf("the environment variable %s starts with %s but is not a setting, so it is ignored", name, config.EnvPrefix)
}

// warnUnknownEnv logs a warning for each environment variable of config.UnknownEnv.
func warnUnknownEnv(ctx context.Context, logger *slog.Logger, src config.Source) {
	for _, name := range config.UnknownEnv(src) {
		logger.WarnContext(ctx, unknownEnvMessage(name), slog.String("env", name))
	}
}

func printLoadErrorEnv(w io.Writer, err error, lookupEnv func(string) (string, bool)) {
	var ve *config.ValidationError
	b, bErr := i18n.Load()
	if !errors.As(err, &ve) || bErr != nil {
		_, _ = fmt.Fprintln(w, err)
		return
	}
	l := b.Localizer(b.EnvLocale(lookupEnv))
	_, _ = fmt.Fprintln(w, l.ServerConfigErrors(len(ve.Issues)))
	for _, is := range ve.Issues {
		if is.Key == "" {
			_, _ = fmt.Fprintf(w, "  - %s\n", is.Message)
			continue
		}
		name := is.Env()
		if name == "" {
			name = is.Key
		}
		if is.Missing {
			_, _ = fmt.Fprintf(w, "  - %s\n", l.ServerConfigMissing(name))
		} else {
			_, _ = fmt.Fprintf(w, "  - %s\n", l.ServerConfigInvalid(name))
		}
		if is.Message != "" {
			_, _ = fmt.Fprintf(w, "    %s: %s\n", is.Key, is.Message)
		}
	}
}
