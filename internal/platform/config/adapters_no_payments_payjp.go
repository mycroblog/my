// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_payments_payjp

package config

func init() { exclude("no_payments_payjp", "payments_provider", "payjp") }
