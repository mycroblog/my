// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_voice_sfu

package config

func init() { exclude("no_voice_sfu", "voice", "sfu") }
