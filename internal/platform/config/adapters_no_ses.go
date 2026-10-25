// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_ses

package config

func init() { exclude("no_ses", "mail", "ses") }
