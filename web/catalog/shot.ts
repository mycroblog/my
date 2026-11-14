// SPDX-License-Identifier: AGPL-3.0-only

// Screenshots of the catalog page for the visual evaluator (ADR-0169 §2), which sees the rendered page and nothing of
// its source: `scripts/task.sh web:shot -- [--section <id>]... [--options <id>] [--scheme light|dark] [--landscape]
// [--lang <locale>] [--out <dir>] [<url>]`. It shoots at the device viewports of @my/devtools/sizes (ADR-0169 §1),
// portrait but for PC (with --landscape, each in landscape: mobile 812×375), in the light and the dark scheme of the
// page's data-theme (--scheme keeps one, for a section that already shows both side by side), at 1:1, without the
// overlay. With --lang, it shoots the page in that locale (`?lang=`, as the overlay's locale switch sets it; ar-XB
// reads right-to-left). With --section, it shoots only that section (the <section> its heading of that id labels, or
// the element of that id), once per --section, on the catalog page that holds it (src/pages.ts names the page of each
// section; for another id, and for --options, it looks on the given page, then on each catalog page); without, the
// whole page. A shot taller than a tile is split into
// numbered tiles, so the evaluator can read a glyph. With --options, it takes one set per variant of that <Options>,
// so the evaluator can answer before the decider picks. The shots go to .tmp/visual/<options id, else the sections,
// else page> of the main checkout, which outlives the worktree, as
// <variant>[-<locale>]-<preset>-<scheme>[-<section>][-<tile>].png; a logged pick's shots are copied from there to
// .tmp/picks for the agreement check (scripts/visual-agreement.sh keep). It prints the path of each shot.

import { mkdir } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { parseArgs } from "node:util";
import { sizes } from "@my/devtools/sizes";
import { chromium, type Page } from "playwright";
import { pageOf, pages } from "./src/pages";

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
    lang: { type: "string" },
    out: { type: "string" },
  },
  allowPositionals: true,
});
const isScheme = (s: string): s is Scheme => (schemes as readonly string[]).includes(s);
if (values.scheme !== undefined && !isScheme(values.scheme))
  throw new Error(`--scheme ${values.scheme}: light or dark`);
const inSchemes = values.scheme === undefined ? schemes : [values.scheme];
const sections: (string | undefined)[] = values.section ?? [undefined];
const target = new URL(positionals[0] ?? run([join(repo, "scripts/agentkit.sh"), "feedback", "url"]));
if (values.lang !== undefined) target.searchParams.set("lang", Intl.getCanonicalLocales(values.lang)[0] ?? values.lang);
const locale = target.searchParams.get("lang") ?? undefined;
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
async function load(page: Page, url: string | null) {
  if (url === null) await page.reload({ waitUntil: "networkidle" });
  else await page.goto(url, { waitUntil: "networkidle" });
  await page.evaluate(() => {
    (window as { myDevtoolsUnmount?: () => void }).myDevtoolsUnmount?.();
  });
}

// The URL of a catalog page, with the target's search (its locale).
const at = (path: string) => {
  const u = new URL(target);
  u.pathname = path;
  u.hash = "";
  return u.href;
};

// The page that has an element, among the page src/pages.ts names for a section id, the target and each catalog page.
async function find(page: Page, selector: string, id: string, what: string) {
  const named = pageOf(id);
  const candidates = [...new Set([...(named ? [at(named.path)] : []), target.href, ...pages.map((p) => at(p.path))])];
  for (const url of candidates) {
    await load(page, url);
    if ((await page.locator(selector).count()) > 0) return url;
  }
  throw new Error(`no ${what} on ${target.href} or on any catalog page`);
}

const sectionSelector = (id: string) => `section[aria-labelledby=${JSON.stringify(id)}], [id=${JSON.stringify(id)}]`;

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
  if (b === null) throw new Error(`no section or element with id "${section}" on ${page.url()}`);
  const x = Math.floor(b.left);
  const y = Math.floor(b.top);
  return { x, y, width: Math.ceil(b.right) - x, height: Math.ceil(b.bottom) - y };
}

async function shoot(page: Page, variant: string, sections: readonly (string | undefined)[]) {
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
          const path = join(out, `${[variant, locale, preset, scheme, section, tile].filter(Boolean).join("-")}.png`);
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
  const id = values.options;
  if (id === undefined) {
    if (values.section === undefined) {
      await load(page, target.href);
      await shoot(page, "page", [undefined]);
    } else {
      // The sections by the page that holds them, each page loaded once.
      const byUrl = new Map<string, string[]>();
      for (const section of values.section) {
        const url = await find(page, sectionSelector(section), section, `section or element with id "${section}"`);
        byUrl.set(url, [...(byUrl.get(url) ?? []), section]);
      }
      for (const [url, inPage] of byUrl) {
        await load(page, url);
        await shoot(page, "page", inPage);
      }
    }
  } else {
    const selector = `[${optionsAttribute}=${JSON.stringify(id)}]`;
    await load(page, await find(page, selector, values.section?.[0] ?? id, `<Options id="${id}">`));
    const names = await page
      .locator(selector)
      .getAttribute("data-variants", { timeout: 5000 })
      .catch(() => null);
    if (names === null) throw new Error(`no <Options id="${id}"> on ${page.url()}`);
    for (const variant of JSON.parse(names) as string[]) {
      await page.evaluate(
        ({ key, value }) => {
          sessionStorage.setItem(key, value);
        },
        { key: `my-devtools:option:${id}`, value: variant },
      );
      await load(page, null);
      await shoot(page, variant, sections);
    }
  }
} finally {
  await browser.close();
}
