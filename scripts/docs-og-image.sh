#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Renders the og:image of the documentation site, app/docs/public/og-image.png (1200 x 630, the size link previews
# expect), from its SVG source app/docs/src/assets/og-image.svg. The PNG is committed; run this after changing the
# SVG. Needs ImageMagick (convert) with an SVG renderer, like scripts/web-icons.sh.
# Usage: scripts/docs-og-image.sh
set -euo pipefail

docs="$(cd "$(dirname "$0")/.." && pwd)/app/docs"

# Render at twice the size, then scale down, for smooth edges; strip metadata so the output is stable.
convert -background white -density 144 "$docs/src/assets/og-image.svg" \
	-resize 1200x630 -strip "PNG24:$docs/public/og-image.png"
