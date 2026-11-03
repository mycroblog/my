// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_payments_stripe

package config

func init() { exclude("no_payments_stripe", "payments", "stripe") }
