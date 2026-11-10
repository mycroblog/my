// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_billing_cloudflare

package config

func init() { exclude("no_billing_cloudflare", "billing_sources", "cloudflare") }

// checkBillingCloudflareAccountID accepts every value: no adapter of this build reads
// billing_cloudflare_account_id.
func checkBillingCloudflareAccountID(string) error { return nil }
