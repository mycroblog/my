// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_postgres

package config

func init() {
	exclude("no_postgres", "database", "postgres")
	exclude("no_postgres", "stream", "postgres")
}
