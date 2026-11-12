---
name: draw-diagram
description: Draw or update a diagram (hand-written SVG) for a document under docs/, in the style every diagram shares, legible in light and dark. Use when a document needs a diagram, or when a diagram's names or flow went stale after a code change.
---

# Draw-diagram skill

Copy the style of `docs/operations/images/architecture.svg` and `growth.svg` (embedded by
`docs/operations/overview.md`); do not invent a new one. Start from [template.svg](template.svg); the classes and the
line styles are in [palette.md](palette.md).

## When

Draw one when a reader must hold a structure or a flow a list does not show: how parts connect, which way data moves,
what changes from one step to the next. Never to decorate, to repeat a short list, or to show a screen. A diagram never
replaces the text: the prose after it says everything it shows.

## Rules

- Hand-written SVG at `images/<name>.svg` next to the document. No PNG, no tool whose source is not in the repository.
  Only the images of the published directories reach the documentation site.
- `<!-- SPDX-License-Identifier: AGPL-3.0-only -->` inside the `<svg>` element.
- Light palette in the SVG's own `<style>`, overridden in `@media (prefers-color-scheme: dark)`, over its own
  background (`<rect class="bg">` on the whole canvas). GitHub follows the OS theme and the site has its own toggle; an
  image cannot tell which one set the page, so it must not depend on the page's background.
- Every role, adapter, configuration key, command, and value is spelled as in the code and
  `docs/reference/configuration.md` (`role = worker`, `PostgreSQL`). Check each when you draw it; a stale diagram is a bug.
- Read top to bottom or left to right, arrows the way data or control goes, no crossing lines another layout avoids.
  Group related boxes in a labeled container.
- `viewBox="0 0 760 <height>"` and `width="760"`, always. English labels: 15px note, 16px box label, 17px group
  heading. Need more room: grow taller, never wider or smaller. Keep labels short; the detail goes in the prose.
- The template's system font stack: an SVG shown as `<img>` cannot load web fonts.
- No `<script>`, no link to another file or URL, no embedded raster image.
- Use the palette's classes. A new color is added for both themes in the same change, its contrast checked in both.
  Red is only `danger` / `danger-text`, never named after the case (`refused`, `lost`).

## Text alternatives

Three, saying the same thing:

1. `<title id="title">` (a short name) and `<desc id="desc">` (the whole content in prose, in reading order), linked
   by `role="img" aria-labelledby="title desc"`.
2. The Markdown image's alt text, by a relative path: one or two sentences on what it shows and where the details are.
3. Right after the image, prose that says everything in it, as a list following the flow of the picture
   (`From the top of the diagram down:` then `- **Who connects.** ...`).

## Check

Done when seen in both themes, in every place it is shown:

1. Locally: open the `.svg` in a browser and switch the OS setting or the emulated `prefers-color-scheme`.
2. On GitHub: the document on the pull request's branch, switching the OS setting or the GitHub theme.
3. On the site: the documentation preview of the pull request (its comment holds the URL); set the site's theme toggle
   opposite to the OS setting. It must stay readable.
4. Read `<desc>` and the alt text against the prose after the image.

Then `scripts/task.sh docs:check`, and `scripts/task.sh site:check` for a published document.
