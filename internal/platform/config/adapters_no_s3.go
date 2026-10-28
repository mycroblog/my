// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_s3

package config

func init() { exclude("no_s3", "blob", "s3") }
