// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_billing_cloudflare

package config

import billingcloudflare "github.com/mycroblog/my/internal/adapter/billing/cloudflare"

func checkBillingCloudflareAccountID(v string) error { return billingcloudflare.CheckAccountID(v) }
