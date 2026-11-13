// SPDX-License-Identifier: AGPL-3.0-only

// Package instance embeds the desired-state schema of the tables of the instance module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/instance imports (kernel, uow, and queue are
// 10-infrastructure).
package instance

import (
	_ "embed"

	account "github.com/mycroblog/my/schema/02-account"
	moderation "github.com/mycroblog/my/schema/03-moderation"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 09-instance.
var Module = ddl.New("09-instance", sqlite, postgres, account.Module, moderation.Module, infrastructure.Module).
	WithPrepares(ddl.Prepare{Table: "instance_theme_tokens", SQL: RetiredThemeTokens})

// RetiredThemeTokens deletes the overrides of the tokens that a theme may no longer override, which
// the CHECK of instance_theme_tokens.token refuses: they stopped applying when their tokens left
// @my/design-tokens (ADR-0059 §14). Its list is the CHECK's.
const RetiredThemeTokens = `DELETE FROM instance_theme_tokens WHERE token NOT IN (
  'font-sans', 'text-tagline', 'text-tagline--line-height', 'text-title', 'text-title--line-height',
  'text-body', 'text-body--line-height', 'text-caption', 'text-caption--line-height',
  'color-surface', 'color-text', 'color-muted-text', 'color-accent', 'color-on-accent',
  'color-accent-text', 'color-surface-raised', 'color-surface-raised-hover',
  'color-surface-raised-pressed', 'color-surface-hover', 'color-surface-pressed', 'color-quote-bar',
  'radius-control', 'radius-overlay', 'duration-spin'
)`
