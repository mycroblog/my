# Diagrams in the documents

How to draw a diagram for a document so that every diagram looks and works the same. The reference examples are
[`architecture.svg`](../operations/images/architecture.svg) and [`growth.svg`](../operations/images/growth.svg), embedded by
[the operator overview](../operations/overview.md). Copy their style; do not invent a new one.

## When a document needs a diagram

Add a diagram when a reader must hold a structure or a flow in their head that a list does not show: how parts connect,
which way data moves, or what changes from one step to the next. Do not add one to decorate a page, to repeat a short
list, or to show a screen.

A diagram never replaces the text. Everything it says is also written in the prose after it, so a reader who cannot see
the image, a search engine, and the plain Markdown copy of the page lose nothing.

## Conventions

- **Hand-written SVG**, committed as `images/<name>.svg` next to the document that shows it. No PNG exports and no diagram
  tool whose source is not in the repository. The documentation site copies the images of the published directories
  (`docs/operations/`, `docs/reference/`, and the like) beside the pages (`app/docs/scripts/import-docs.ts`); an image
  elsewhere is not published.
- **License header.** Start the file with `<!-- SPDX-License-Identifier: AGPL-3.0-only -->` inside the `<svg>` element.
- **Colors switch inside the SVG.** Put the light palette in the SVG's own `<style>`, override it in
  `@media (prefers-color-scheme: dark)`, and draw your own background (a `<rect class="bg">` over the whole canvas).
  The reason: GitHub picks the theme by the viewer's OS setting, while the site has its own theme toggle that can differ
  from the OS. An image cannot see which of the two decided the page's background, so it must not depend on it. With its
  own background and a palette readable on that background, the diagram is legible whichever theme the page is in.
- **Names true to the code and configuration.** Every role, adapter, configuration key, command, and value in a diagram
  is spelled as in the code and in [the configuration reference](../reference/configuration.md) (`role = worker`,
  `PostgreSQL`, `Cloud Tasks`). Check each one when you draw it and again when the code changes; a stale diagram is a bug.
- **A clear flow.** Read it top to bottom or left to right, with arrows pointing the way data or control goes, and no
  crossing lines where a different layout avoids them. Group related boxes in a labeled container.
- **English labels at a readable size.** Use 15px for a note, 16px for a box label, and 17px for a group heading, with
  a `viewBox` exactly 760 wide (see [Width](#width)) so the image is not scaled below those sizes in the content column. Keep labels short; put
  the detail in the prose.
- **Fonts that need no download.** Use the system stack of the examples
  (`-apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans", Helvetica, Arial, sans-serif`). An SVG shown as an
  `<img>` cannot load web fonts.
- **Text alternatives.** Three, and they say the same thing:
  1. `<title>` (a short name) and `<desc>` (the whole content in prose, in reading order) inside the SVG, linked with
     `role="img" aria-labelledby="title desc"`.
  2. The alt text of the Markdown image: one or two sentences about what the diagram shows and where the details are.
  3. Prose right after the diagram that says everything in it, as a list that follows the flow of the picture.
- **No script and no external references**: no `<script>`, no link to another file or URL, no embedded raster image.

## Width

Every diagram is 760 wide: `viewBox="0 0 760 <height>"` and `width="760"`. The font sizes stay as written above (15, 16
and 17px); a diagram that needs more room grows taller, never wider and never smaller in type. Red for a failure is
always `danger` (a box) or `danger-text` (text), with the colors of the table below; do not add another red, and do not
name it after the case (`refused`, `lost`). A red arrow is `class="arrow danger"` with a head of `class="head danger"`.

## Line styles

Lines that carry different things look different, in every diagram that needs more than the plain `arrow`. The class
sets the stroke; copy the values.

| Class (head) | Use | Style | Light | Dark |
| --- | --- | --- | --- | --- |
| `arrow` (`head`) | the usual flow | solid, 2px | `#59636e` | `#9198a1` |
| `signal` (`signal-head`) | a signal or control message | dashed `6 4`, 2px | `#0969da` | `#4493f8` |
| `data` (`data-head`) | data that flows | solid, 3px | `#1a7f37` | `#3fb950` |
| `reply` (uses `head`) | a reply to a request | dashed `8 5`, 2px | `#59636e` | `#9198a1` |
| `arrow danger` (`head danger`) | a failure | solid, 2px | `#cf222e` | `#f85149` |

The examples are [`voice-methods.svg`](../operations/images/voice-methods.svg) and
[`supporter-flow.svg`](../operations/images/supporter-flow.svg) (signal, data) and
[`token-flow.svg`](../developers/images/token-flow.svg) (reply). Give each style its own arrowhead marker with the
matching fill, and say in the prose what the styles mean when a diagram uses more than one.

## Embedding

```markdown
![The architecture of a server: clients reach the processes over HTTPS, and the processes use the parts chosen by configuration.](images/architecture.svg)

From the top of the diagram down:

- **Who connects.** ...
```

Use a relative path. `scripts/task.sh docs:rules` checks it, and the site rewrites it.

## Template

A minimal diagram: three boxes and two arrows. The classes are the palette of the existing diagrams; use them before
adding a color.

| Class | Use | Light (fill / stroke) | Dark (fill / stroke) |
| --- | --- | --- | --- |
| `bg` | the canvas | `#ffffff` | `#0d1117` |
| `t`, `m`, `h` | label, note, heading text | `#1f2328`, `#59636e`, `#1f2328` | `#e6edf3`, `#9198a1`, `#e6edf3` |
| `group` | a container of boxes | `#f6f8fa` / `#d1d9e0` | `#151b23` / `#3d444d` |
| `client` | who connects from outside | `#dafbe1` / `#1a7f37` | `#0f2d1b` / `#3fb950` |
| `proxy` | infrastructure in front | `#ffffff` / `#59636e` | `#0d1117` / `#9198a1` |
| `proc` | the `my` processes | `#ddf4ff` / `#0969da` | `#0c2d6b` / `#4493f8` |
| `port` | a part chosen by configuration | `#ffffff` / `#8c959f` | `#0d1117` / `#656c76` |
| `svc` | an outside service | `#fbefff` / `#8250df` | `#2a1650` / `#ab7df8` |
| `danger` | a failure, a refusal, a loss, and what follows it | `#ffebe9` / `#cf222e` | `#3c0d12` / `#f85149` |
| `danger-text` | text that is red without a box | `#cf222e` | `#f85149` |
| `arrow`, `head` | arrows and heads | `#59636e` | `#9198a1` |

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 760 190" width="760" height="190" role="img" aria-labelledby="title desc">
<!-- SPDX-License-Identifier: AGPL-3.0-only -->
  <title id="title">A request reaches the web process</title>
  <desc id="desc">A web client sends a request to the my process with role web, which stores data in the database.</desc>

  <style>
    svg { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans", Helvetica, Arial, sans-serif; }
    .bg { fill: #ffffff; }
    .t { fill: #1f2328; font-size: 16px; }
    .b { font-weight: 600; }
    .m { fill: #59636e; font-size: 15px; }
    .client { fill: #dafbe1; stroke: #1a7f37; stroke-width: 1.5; }
    .proc { fill: #ddf4ff; stroke: #0969da; stroke-width: 2; }
    .port { fill: #ffffff; stroke: #8c959f; stroke-width: 1.5; }
    .arrow { stroke: #59636e; stroke-width: 2; fill: none; }
    .head { fill: #59636e; }
    @media (prefers-color-scheme: dark) {
      .bg { fill: #0d1117; }
      .t { fill: #e6edf3; }
      .m { fill: #9198a1; }
      .client { fill: #0f2d1b; stroke: #3fb950; }
      .proc { fill: #0c2d6b; stroke: #4493f8; }
      .port { fill: #0d1117; stroke: #656c76; }
      .arrow { stroke: #9198a1; }
      .head { fill: #9198a1; }
    }
  </style>
  <defs>
    <marker id="arrowhead" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
      <path class="head" d="M 0 0 L 10 5 L 0 10 z"/>
    </marker>
  </defs>
  <rect class="bg" x="0" y="0" width="760" height="190" rx="12"/>
  <rect class="client" x="20" y="59" width="200" height="72" rx="8"/>
  <text class="t b" x="120" y="89" text-anchor="middle">Web client</text>
  <text class="m" x="120" y="111" text-anchor="middle">in the browser</text>
  <path class="arrow" d="M 220 95 L 276 95" marker-end="url(#arrowhead)"/>
  <rect class="proc" x="280" y="59" width="200" height="72" rx="8"/>
  <text class="t b" x="380" y="89" text-anchor="middle">role = web</text>
  <text class="m" x="380" y="111" text-anchor="middle">the HTTP server</text>
  <path class="arrow" d="M 480 95 L 536 95" marker-end="url(#arrowhead)"/>
  <rect class="port" x="540" y="59" width="200" height="72" rx="8"/>
  <text class="t b" x="640" y="89" text-anchor="middle">Database</text>
  <text class="m" x="640" y="111" text-anchor="middle">SQLite · PostgreSQL</text>
</svg>
```

Keep the ids `title` and `desc`, and put the marker `<defs>` before the shapes. Use `stroke-width` 1.5 for quiet boxes
and 2 for the main ones, and `rx="8"` for boxes (`12` for groups and the canvas). A color that is not in the table is
added for both themes in the same change, with its contrast checked in both.

## Checking light and dark

A diagram is done when you have seen it in both themes, in both places it is shown.

1. **Locally**: open the `.svg` in a browser and switch the OS setting, or the emulated `prefers-color-scheme` in the
   browser's developer tools, between light and dark.
2. **On GitHub**: open the changed document on the pull request's branch and switch the OS setting or the GitHub theme
   (Settings, Appearance). The image follows the OS setting.
3. **On the site**: a pull request that changes the published documents gets a documentation preview
   (`docs-site-preview.yml`; its pull request comment holds the URL and the pages it changes). Open the page, then set
   the site's theme toggle to the opposite of your OS setting. The diagram must stay readable, which is why it carries
   its own background.
4. **Text alternatives**: read the `<desc>` and the alt text, and confirm the prose after the diagram says the same.

Then run `scripts/task.sh docs:check`, and `scripts/task.sh site:check` for a published document.
