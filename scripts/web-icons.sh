#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Renders the PNG app icons of the web client from their SVG sources in app/web/public (ADR-0028):
# the 192 and 512 px icons that Chrome's install criteria ask for, a 512 px maskable icon, and the
# 180 px apple-touch-icon.png that iOS puts on the home screen. It also renders the server's built-in
# link preview image (ADR-0059 §8), the colourful wordmark of app/ui/src/logos on white at 1200 x 630,
# which internal/interface/web embeds. The PNGs are committed; run this after changing icon.svg,
# icon-maskable.svg, or the wordmark. Needs ImageMagick (convert) with an SVG renderer.
# Usage: scripts/web-icons.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
public="$root/app/web/public"

render() {
	local source="$1" size="$2" out="$3"
	# Render at twice the size, then scale down, for smooth edges; strip metadata so the output is stable.
	convert -background none -density "$((size * 2 * 72 / 512))" "$public/$source" \
		-resize "${size}x${size}" -strip "PNG32:$public/$out"
}

render icon.svg 192 icon-192.png
render icon.svg 512 icon-512.png
render icon-maskable.svg 512 icon-maskable-512.png
render icon-maskable.svg 180 apple-touch-icon.png

# The built-in link preview image: opaque white, the wordmark fitted inside the middle 80 % of the
# width and 50 % of the height, as the admin screen draws an instance's logo (preview-1200x630).
preview() {
	local source="$1" out="$2"
	convert -background none -density 288 "$source" -resize "960x315" \
		-background white -gravity center -extent 1200x630 -alpha remove -alpha off -strip "PNG24:$out"
}

preview "$root/app/ui/src/logos/Wordmark-Primary-Color.svg" "$root/internal/interface/web/preview-image.png"
