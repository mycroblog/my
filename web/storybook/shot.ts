// SPDX-License-Identifier: AGPL-3.0-only

// Screenshots of stories for the visual evaluator (ADR-0169 §2), which sees the rendered page and nothing of its
// source: `scripts/task.sh web:shot -- [--section <story id>[#<element id>]]... [--options <id>] [--scheme light|dark]
// [--landscape] [--lang <locale>] [--out <dir>] [<url>]`. The URL is a story's (Storybook's `?path=/story/<id>` or
// `iframe.html?id=<id>`) or the Storybook's own; without one, the running Storybook's (`agentkit feedback url`). It
// shoots each story alone (iframe.html) at the device viewports of @my/devtools/sizes (ADR-0169 §1), portrait but for
// PC (with --landscape, each in landscape: mobile 812×375), in the light and the dark scheme of the host's theme
// (--scheme keeps one, for a story that already shows both side by side), at 1:1, without the overlay. With --lang, it
// shoots the story in that locale (en, ja, ar-XB, which reads right-to-left). With --section, it shoots that story, once
// per --section, or with `#<element id>` only that element of it; without, the story the URL names. A shot taller
// than a tile is split into numbered tiles, so the evaluator can read a glyph. With --options, it takes one set per
// variant of that <Options>, on the first --section's story, else the URL's, else the first story that has it, so the
// evaluator can answer before the decider picks. The shots go to .tmp/visual/<options id, else the sections, else the
// story> of the main checkout, which outlives the worktree, as <variant>[-<locale>]-<preset>-<scheme>[-<section>][-<tile>].png;
// a logged pick's shots are copied from there to .tmp/picks for the agreement check (scripts/visual-agreement.sh keep).
// It prints the path of each shot.

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
    lang: { type: "string" },
    out: { type: "string" },
  },
  allowPositionals: true,
});
const isScheme = (s: string): s is Scheme => (schemes as readonly string[]).includes(s);
if (values.scheme !== undefined && !isScheme(values.scheme))
  throw new Error(`--scheme ${values.scheme}: light or dark`);
const inSchemes = values.scheme === undefined ? schemes : [values.scheme];
const given = new URL(positionals[0] ?? run([join(repo, "scripts/agentkit.sh"), "feedback", "url"]));
// The Storybook's own address, and the story the URL names, if any.
const base = new URL(given.pathname.endsWith("/iframe.html") ? "./" : given.pathname.replace(/[^/]*$/, ""), given);
const named = given.searchParams.get("id") ?? given.searchParams.get("path")?.replace(/^\/(?:story|docs)\//, "");
const locale = values.lang === undefined ? undefined : (Intl.getCanonicalLocales(values.lang)[0] ?? values.lang);

/** A section: a story, and the element of it to shoot, if any. */
type Section = { story: string; element?: string | undefined; label: string };
const sectionOf = (arg: string): Section => {
  const [story = "", element] = arg.split("#");
  return { story, element, label: arg.replace("#", "-") };
};
const sections = (values.section ?? []).map(sectionOf);
if (sections.length === 0 && values.options === undefined) {
  if (named === undefined || named === "")
    throw new Error(`${given.href} names no story: give a story's URL, or --section <story id>`);
  sections.push({ story: named, label: "" });
}

const dir = values.options ?? (values.section === undefined ? named : sections.map((s) => s.label).join("-"));
const out = resolve(values.out ?? join(main, ".tmp/visual", dir ?? "story"));
// Every logged pick names a folder under .tmp/picks (`agentkit docs lint` holds it), where scripts/visual-agreement.sh
// keeps its shots for the replay: no shot goes there.
const kept = join(main, ".tmp/picks");
if (out === kept || out.startsWith(`${kept}/`))
  throw new Error(
    `shots into ${out}: ${kept} keeps the shots of logged picks, which no shot may overwrite; shoot into ${join(main, ".tmp/visual")}/<options id> (--options <id> without --out), then keep a pick's shots with scripts/visual-agreement.sh keep`,
  );
await mkdir(out, { recursive: true });

/** The story alone, in a scheme and the locale. */
const storyURL = (story: string, scheme: Scheme) => {
  const u = new URL("iframe.html", base);
  u.searchParams.set("id", story);
  u.searchParams.set("viewMode", "story");
  u.searchParams.set("globals", [`theme:${scheme}`, ...(locale ? [`locale:${locale}`] : [])].join(";"));
  return u.href;
};

// The story once Storybook shows it and what it fetches has come; the overlay's own unmount removes the overlay, so
// that it covers no shot.
async function load(page: Page, url: string) {
  await page.goto(url, { waitUntil: "networkidle" });
  await page.waitForFunction(
    () => ["sb-show-main", "sb-show-errordisplay"].some((c) => document.body.classList.contains(c)),
    undefined,
    { timeout: 30_000 },
  );
  if (await page.evaluate(() => document.body.classList.contains("sb-show-errordisplay")))
    throw new Error(`${url} renders an error: ${(await page.locator("#error-message").textContent()) ?? ""}`);
  await page.evaluate(() => {
    (window as { myDevtoolsUnmount?: () => void }).myDevtoolsUnmount?.();
  });
}

// The box of an element of the story, or of the whole page, in page coordinates, on whole pixels.
async function box(page: Page, element: string | undefined) {
  const b = await page.evaluate((id) => {
    if (id === undefined) {
      const html = document.documentElement;
      return { left: 0, top: 0, right: html.scrollWidth, bottom: html.scrollHeight };
    }
    const el = document.getElementById(id);
    if (el === null) return null;
    const r = el.getBoundingClientRect();
    return { left: r.left + scrollX, top: r.top + scrollY, right: r.right + scrollX, bottom: r.bottom + scrollY };
  }, element);
  if (b === null) throw new Error(`no element with id "${element ?? ""}" on ${page.url()}`);
  const x = Math.floor(b.left);
  const y = Math.floor(b.top);
  return { x, y, width: Math.ceil(b.right) - x, height: Math.ceil(b.bottom) - y };
}

async function shoot(page: Page, variant: string, shown: readonly Section[]) {
  for (const [size, { width, height }] of Object.entries(sizes)) {
    const viewport = values.landscape
      ? { width: Math.max(width, height), height: Math.min(width, height) }
      : { width, height };
    await page.setViewportSize(viewport);
    const preset = `${size}${values.landscape ? "-landscape" : ""}`;
    for (const scheme of inSchemes) {
      for (const section of shown) {
        await load(page, storyURL(section.story, scheme));
        const { x, y, width: w, height: h } = await box(page, section.element);
        const tiles = Math.ceil(h / tileHeight);
        for (let i = 0; i < tiles; i++) {
          const tile = tiles > 1 ? String(i + 1) : undefined;
          const name = [variant, locale, preset, scheme, section.label, tile].filter(Boolean).join("-");
          const path = join(out, `${name}.png`);
          const top = y + i * tileHeight;
          const clip = { x, y: top, width: w, height: Math.min(tileHeight, y + h - top) };
          await page.screenshot({ path, fullPage: true, clip, animations: "disabled" });
          console.log(path);
        }
      }
    }
  }
}

/** The story that has the <Options> of `id`: the first section's, else the URL's, else the first in the index. */
async function storyWithOptions(page: Page, id: string): Promise<string> {
  const first = sections[0]?.story ?? named;
  const index = (await (await fetch(new URL("index.json", base))).json()) as {
    entries: Record<string, { id: string; type: string }>;
  };
  const stories = Object.values(index.entries)
    .filter((e) => e.type === "story")
    .map((e) => e.id);
  for (const story of first ? [first, ...stories.filter((s) => s !== first)] : stories) {
    await load(page, storyURL(story, "light"));
    if ((await page.locator(`[${optionsAttribute}=${JSON.stringify(id)}]`).count()) > 0) return story;
  }
  throw new Error(`no <Options id="${id}"> in any story of ${base.href}`);
}

const browser = await chromium.launch();
try {
  const page = await browser.newPage({ deviceScaleFactor: 1 });
  const id = values.options;
  if (id === undefined) await shoot(page, "page", sections);
  else {
    const story = await storyWithOptions(page, id);
    const names = await page
      .locator(`[${optionsAttribute}=${JSON.stringify(id)}]`)
      .getAttribute("data-variants", { timeout: 5000 })
      .catch(() => null);
    if (names === null) throw new Error(`no <Options id="${id}"> in ${story}`);
    const shown = sections.length > 0 ? sections : [{ story, label: "" }];
    for (const variant of JSON.parse(names) as string[]) {
      await page.evaluate(
        ({ key, value }) => {
          sessionStorage.setItem(key, value);
        },
        { key: `my-devtools:option:${id}`, value: variant },
      );
      await shoot(page, variant, shown);
    }
  }
} finally {
  await browser.close();
}
