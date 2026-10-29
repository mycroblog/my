// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_gcs

package config

func init() { exclude("no_gcs", "blob", "gcs") }
