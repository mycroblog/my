// SPDX-License-Identifier: AGPL-3.0-only

// Screenshots of the catalog page for the visual evaluator (ADR-0169 §2), which sees the rendered page and nothing of
// its source: `scripts/task.sh web:shot -- [--section <id>]... [--options <id>] [--scheme light|dark] [--landscape]
// [--out <dir>] [<url>]`. It shoots at the device viewports of @my/devtools/sizes (ADR-0169 §1), portrait but for PC
// (with --landscape, each in landscape: mobile 812×375), in the light and the dark scheme of the page's data-theme
// (--scheme keeps one, for a section that already shows both side by side), at 1:1, without the overlay. With
// --section, it shoots only that section (the <section> its heading of that id labels, or the element of that id),
// once per --section; without, the whole page. A shot taller than a tile is split into numbered tiles, so the
// evaluator can read a glyph. With --options, it takes one set per variant of that <Options>, so the evaluator can
// answer before the decider picks. The shots go to .tmp/visual/<options id, else the sections, else page> of the main
// checkout, which outlives the worktree, as <variant>-<preset>-<scheme>[-<section>][-<tile>].png; a logged pick's
// shots are copied from there to .tmp/picks for the agreement check (scripts/visual-agreement.sh keep). It prints the
// path of each shot.

import { mkdir } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { parseArgs } from "node:util";
import { sizes } from "@my/devtools/sizes";
import { chromium, type Page } from "playwright";

const optionsAttribute = "data-my-devtools-options";
const schemes = ["light", "dark"] as const;
type Scheme = (typeof schemes)[number];
// Under the 2,000 px of an image the evaluator reads whole.
const tileHeight = 1800;

const run = (cmd: string[]) => {
  const p = Bun.spawnSync(cmd, { stderr: "inherit" });
  if (p.exitCode !== 0) throw new Error(`${cmd.join(" ")} exited ${p.exitCode}`);
  return p.stdout.toString().trim();
};

const repo = run(["git", "rev-parse", "--show-toplevel"]);
const main = dirname(run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"]));

const { values, positionals } = parseArgs({
  args: Bun.argv.slice(2),
  options: {
    section: { type: "string", multiple: true },
    options: { type: "string" },
    scheme: { type: "string" },
    landscape: { type: "boolean" },
    out: { type: "string" },
  },
  allowPositionals: true,
});
const isScheme = (s: string): s is Scheme => (schemes as readonly string[]).includes(s);
if (values.scheme !== undefined && !isScheme(values.scheme))
  throw new Error(`--scheme ${values.scheme}: light or dark`);
const inSchemes = values.scheme === undefined ? schemes : [values.scheme];
const sections: (string | undefined)[] = values.section ?? [undefined];
const url = positionals[0] ?? run([join(repo, "scripts/agentkit.sh"), "feedback", "url"]);
const dir = values.options ?? values.section?.join("-") ?? "page";
const out = resolve(values.out ?? join(main, ".tmp/visual", dir));
// Every logged pick names a folder under .tmp/picks (`agentkit docs lint` holds it), where scripts/visual-agreement.sh
// keeps its shots for the replay: no shot goes there.
const kept = join(main, ".tmp/picks");
if (out === kept || out.startsWith(`${kept}/`))
  throw new Error(
    `shots into ${out}: ${kept} keeps the shots of logged picks, which no shot may overwrite; shoot into ${join(main, ".tmp/visual")}/<options id> (--options <id> without --out), then keep a pick's shots with scripts/visual-agreement.sh keep`,
  );
await mkdir(out, { recursive: true });

// The overlay is mounted on <html>, outside <body>; its own unmount removes it, so that it covers no shot.
async function load(page: Page, reload: boolean) {
  if (reload) await page.reload({ waitUntil: "networkidle" });
  else await page.goto(url, { waitUntil: "networkidle" });
  await page.evaluate(() => {
    (window as { myDevtoolsUnmount?: () => void }).myDevtoolsUnmount?.();
  });
}

// The box of a section, or of the whole page, in page coordinates, on whole pixels.
async function box(page: Page, section: string | undefined) {
  const b = await page.evaluate((id) => {
    if (id === undefined) {
      const html = document.documentElement;
      return { left: 0, top: 0, right: html.scrollWidth, bottom: html.scrollHeight };
    }
    const el = document.querySelector(`section[aria-labelledby="${CSS.escape(id)}"]`) ?? document.getElementById(id);
    if (el === null) return null;
    const r = el.getBoundingClientRect();
    return { left: r.left + scrollX, top: r.top + scrollY, right: r.right + scrollX, bottom: r.bottom + scrollY };
  }, section);
  if (b === null) throw new Error(`no section or element with id "${section}" on ${url}`);
  const x = Math.floor(b.left);
  const y = Math.floor(b.top);
  return { x, y, width: Math.ceil(b.right) - x, height: Math.ceil(b.bottom) - y };
}

async function shoot(page: Page, variant: string) {
  for (const [size, { width, height }] of Object.entries(sizes)) {
    const viewport = values.landscape
      ? { width: Math.max(width, height), height: Math.min(width, height) }
      : { width, height };
    await page.setViewportSize(viewport);
    const preset = `${size}${values.landscape ? "-landscape" : ""}`;
    for (const scheme of inSchemes) {
      await page.evaluate((s) => {
        document.documentElement.setAttribute("data-theme", s);
      }, scheme);
      for (const section of sections) {
        const { x, y, width: w, height: h } = await box(page, section);
        const tiles = Math.ceil(h / tileHeight);
        for (let i = 0; i < tiles; i++) {
          const tile = tiles > 1 ? String(i + 1) : undefined;
          const path = join(out, `${[variant, preset, scheme, section, tile].filter(Boolean).join("-")}.png`);
          const top = y + i * tileHeight;
          const clip = { x, y: top, width: w, height: Math.min(tileHeight, y + h - top) };
          await page.screenshot({ path, fullPage: true, clip, animations: "disabled" });
          console.log(path);
        }
      }
    }
  }
}

const browser = await chromium.launch();
try {
  const page = await browser.newPage({ deviceScaleFactor: 1 });
  await load(page, false);
  const id = values.options;
  if (id === undefined) {
    await shoot(page, "page");
  } else {
    const names = await page
      .locator(`[${optionsAttribute}="${id}"]`)
      .getAttribute("data-variants", { timeout: 5000 })
      .catch(() => null);
    if (names === null) throw new Error(`no <Options id="${id}"> on ${url}`);
    for (const variant of JSON.parse(names) as string[]) {
      await page.evaluate(
        ({ key, value }) => {
          sessionStorage.setItem(key, value);
        },
        { key: `my-devtools:option:${id}`, value: variant },
      );
      await load(page, true);
      await shoot(page, variant);
    }
  }
} finally {
  await browser.close();
}
