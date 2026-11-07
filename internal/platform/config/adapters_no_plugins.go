// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_plugins

package config

func init() {
	pluginsLeftOutTag = "no_plugins"
	exclude("no_plugins", "billing_sources", "plugin")
	exclude("no_plugins", "mail", "plugin")
	exclude("no_plugins", "search", "plugin")
	exclude("no_plugins", "media_image", "plugin")
	exclude("no_plugins", "media_video", "plugin")
	exclude("no_plugins", "media_audio", "plugin")
	exclude("no_plugins", "key_manager", "plugin")
}
