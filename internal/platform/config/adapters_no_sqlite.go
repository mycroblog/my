// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_sqlite

package config

func init() { exclude("no_sqlite", "database", "sqlite") }
