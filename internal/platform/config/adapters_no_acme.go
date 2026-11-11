// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_acme

package config

func init() { exclude("no_acme", "tls", "acme") }
