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

// printReadiness writes the findings of config.Readiness and of the instance settings to w,
// the errors and then the warnings, each group under a summary in the locale of the process
// environment; the English technical detail follows the name of each setting.
func printReadiness(w io.Writer, findings []config.Finding, lookupEnv func(string) (string, bool)) {
	var errs, warns []config.Finding
	for _, f := range findings {
		if f.Severity == config.SeverityError {
			errs = append(errs, f)
		} else {
			warns = append(warns, f)
		}
	}
	b, err := i18n.Load()
	if err != nil {
		for _, f := range append(errs, warns...) {
			_, _ = fmt.Fprintf(w, "%s: %s\n", f.Severity, findingLine(f))
		}
		return
	}
	l := b.Localizer(b.EnvLocale(lookupEnv))
	if len(errs) > 0 {
		_, _ = fmt.Fprintln(w, l.ServerConfigReadinessErrors(len(errs)))
		for _, f := range errs {
			_, _ = fmt.Fprintf(w, "  - %s\n", findingLine(f))
		}
	}
	if len(warns) > 0 {
		_, _ = fmt.Fprintln(w, l.ServerConfigReadinessWarnings(len(warns)))
		for _, f := range warns {
			_, _ = fmt.Fprintf(w, "  - %s\n", findingLine(f))
		}
	}
}

// findingLine is a finding's environment variable (or key) and its detail; a finding without a
// key is its detail alone.
func findingLine(f config.Finding) string {
	name := f.Env()
	if name == "" {
		name = f.Key
	}
	if name == "" {
		return f.Message
	}
	return name + ": " + f.Message
}

// warnReadiness logs a warning for each finding of config.Readiness. my serve starts anyway;
// `my config check` fails on the errors among them.
func warnReadiness(ctx context.Context, logger *slog.Logger, cfg *config.Config) {
	for _, f := range config.Readiness(cfg) {
		logger.WarnContext(ctx, "a setting is not ready for production",
			slog.String("env", f.Env()), slog.String("severity", string(f.Severity)), slog.String("detail", f.Message))
	}
}
