// SPDX-License-Identifier: AGPL-3.0-only
// Proves the rules of restrictions.js and styles.js as the workspace runs them: `bun --bun oxlint` with .oxlintrc.json
// lints the fixtures (web/devtools/fixtures/restrictions/ and the fixtures/ of the layers), and each finding must be
// one that a fixture line expects with an `error: <Component> prop` comment (design-system-props), an `error: layers
// @my/<x>` comment (layer-imports), an `error: platform <what>` comment (platform-globals), an `error: e2e <what>`
// comment (e2e-queries), an `error: e2e-flaky <what>` comment (e2e-flaky), an `error: color <colour>` comment
// (token-colors), an `error: layout <class>` or `error: layout-part <name>` comment (layout-parts), an `error:
// viewport <what>` comment (viewport-breakpoints), an `error: switch <what>` comment (switch-in-form), an `error:
// section <element>` comment (settings-sections), an `error: heading <element>` comment (heading-size), an `error:
// count <element>` comment (settings-count), an `error: date <what>` comment (zone-dates), or an `error: html <what>`
// comment (html-sinks), and each expected one
// must be found. Oxlint's RuleTester refuses to run on Bun (ADR-0025 §8, §10). .oxlintrc.json ignores the fixtures, so that no
// other lint run fails on them; this lints copies of them beside them, under the same overrides, and removes the
// copies. It also holds the lists of the rules to the code: the files layout-parts.js names exist, and the
// breakpoints are the tokens'.

import { expect, test } from "bun:test";
import { cpSync, existsSync, readdirSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { breakpoint } from "../../design-tokens/src/tokens.ts";
import { LAYOUT_PARTS, PAGE_FRAMES } from "./layout-parts.js";
import { BREAKPOINTS, layoutOf, viewportOf } from "./styles.js";

const root = new URL("../../../", import.meta.url).pathname;
/** The packages with fixtures, each in the directory whose rules it proves. */
const dirs = [
  "web/devtools",
  "web/design-system",
  "web/app-components",
  "web/app-features",
  "web/app",
  "web/platform",
  "web/e2e",
];
const fixtures = dirs.flatMap((dir) =>
  readdirSync(`${root}${dir}/fixtures`, { recursive: true, encoding: "utf8" })
    .filter((name) => /\.tsx?$/.test(name))
    .map((name) => `${dir}/fixtures/${name}`),
);

/** What each rule's message says of a finding, as a fixture writes it after `error: `. */
const findings = {
  "restrict(design-system-props)": (/** @type {string} */ message) => {
    const [, component, prop] = /^(<\S+>) (?:of|above) @my\/design-system gets `(\w+)`/.exec(message) ?? [];
    return `${component} ${prop}`;
  },
  "restrict(layer-imports)": (/** @type {string} */ message) =>
    `layers ${/^@my\/\S+ imports (@my\/[\w-]+)/.exec(message)?.[1]}`,
  "restrict(platform-globals)": (/** @type {string} */ message) => `platform ${/^`([^`]+)`/.exec(message)?.[1]}`,
  "restrict(e2e-queries)": (/** @type {string} */ message) => `e2e ${/^`([^`]+)`/.exec(message)?.[1]}`,
  "restrict(e2e-flaky)": (/** @type {string} */ message) => `e2e-flaky ${/^`([^`]+)`/.exec(message)?.[1]}`,
  "restrict(token-colors)": (/** @type {string} */ message) => `color ${/^`([^`]+)`/.exec(message)?.[1]}`,
  "restrict(layout-parts)": (/** @type {string} */ message) => styleFinding("layout", message),
  "restrict(viewport-breakpoints)": (/** @type {string} */ message) => styleFinding("viewport", message),
  "restrict(switch-in-form)": (/** @type {string} */ message) => `switch ${/^`([^`]+)`/.exec(message)?.[1]}`,
  "restrict(settings-sections)": (/** @type {string} */ message) => `section ${/^`([^`]+)`/.exec(message)?.[1]}`,
  "restrict(heading-size)": (/** @type {string} */ message) => `heading ${/^`([^`]+)`/.exec(message)?.[1]}`,
  "restrict(settings-count)": (/** @type {string} */ message) => `count ${/^`([^`]+)`/.exec(message)?.[1]}`,
  "restrict(zone-dates)": (/** @type {string} */ message) => `date ${/^`([^`]+)`/.exec(message)?.[1]}`,
  "restrict(pager-parts)": (/** @type {string} */ message) => `pager ${/^`([^`]+)`/.exec(message)?.[1]}`,
  "restrict(page-trail)": () => "trail",
  "restrict(html-sinks)": (/** @type {string} */ message) => `html ${/^`([^`]+)`/.exec(message)?.[1]}`,
};

/** A finding of a rule of the styles: what a declaration wrote, or a name of layout-parts.js. */
function styleFinding(/** @type {string} */ rule, /** @type {string} */ message) {
  const named = /^`([^`]+)` is named in /.exec(message)?.[1];
  return named === undefined ? `${rule} ${/^`([^`]+)`/.exec(message)?.[1]}` : `${rule}-part ${named}`;
}

/** The findings of the rules, as `file:line <what>`. */
function lint() {
  const copy = `.fixtures-${process.pid}-${Date.now()}`;
  // Oxlint writes its report to a file: it exits with what a pipe has not taken yet unwritten, so a report longer
  // than the pipe's buffer (64 KiB) reaches a pipe cut short.
  const reportFile = `${tmpdir()}/oxlint${copy}.json`;
  let output = "";
  try {
    for (const dir of dirs) cpSync(`${root}${dir}/fixtures`, `${root}${dir}/${copy}`, { recursive: true });
    const copies = fixtures.map((file) => file.replace("/fixtures/", `/${copy}/`));
    Bun.spawnSync([process.execPath, "--bun", "oxlint", "--format", "json", ...copies], {
      cwd: root,
      stdout: Bun.file(reportFile),
    });
    output = readFileSync(reportFile, "utf8");
  } finally {
    for (const dir of dirs) rmSync(`${root}${dir}/${copy}`, { recursive: true, force: true });
    rmSync(reportFile, { force: true });
  }
  /** @type {{ diagnostics: { code: string, message: string, filename: string, labels: { span: { line: number } }[] }[] }} */
  const report = JSON.parse(output.slice(output.indexOf("{")));
  return report.diagnostics
    .filter((d) => Object.hasOwn(findings, d.code))
    .map((d) => {
      const what = findings[/** @type {keyof typeof findings} */ (d.code)](d.message);
      return `${d.filename.replace(`/${copy}/`, "/fixtures/")}:${d.labels[0]?.span.line} ${what}`;
    })
    .sort();
}

/** What the fixtures expect, as `file:line <what>`. */
async function expected() {
  const found = [];
  for (const file of fixtures) {
    const lines = (await Bun.file(`${root}${file}`).text()).split("\n");
    for (const [index, line] of lines.entries()) {
      const match =
        /error: (<\S+> (?:className|style)|layers @my\/[\w-]+|platform (?:"\w+" in|\S+)|e2e \S+|e2e-flaky .*\S|(?:color|layout|layout-part|viewport|switch|section|heading|count|date|pager) \S+|html \S+(?: =)?|trail(?= |$))/.exec(
          line,
        );
      if (match) found.push(`${file}:${index + 1} ${match[1]}`);
    }
  }
  return found.sort();
}

test("the rules find exactly what the fixtures expect, and nothing in the allowed lines", async () => {
  expect(fixtures).toContain("web/devtools/fixtures/restrictions/allowed.tsx");
  expect(fixtures).toContain("web/design-system/fixtures/inside.tsx");
  expect(fixtures).toContain("web/app/fixtures/layers.tsx");
  expect(fixtures).toContain("web/platform/fixtures/inside.ts");
  expect(fixtures).toContain("web/e2e/fixtures/queries.ts");
  expect(fixtures).toContain("web/e2e/fixtures/flaky.ts");
  expect(fixtures).toContain("web/e2e/fixtures/specs/timing.ts");
  expect(fixtures).toContain("web/e2e/fixtures/config/playwright.config.ts");
  expect(fixtures).toContain("web/design-system/fixtures/colors.tsx");
  expect(fixtures).toContain("web/design-system/fixtures/layout.tsx");
  expect(fixtures).toContain("web/design-system/fixtures/viewport.tsx");
  expect(fixtures).toContain("web/design-system/fixtures/dates.tsx");
  expect(fixtures).toContain("web/devtools/fixtures/restrictions/settings.tsx");
  expect(fixtures).toContain("web/app-features/fixtures/pagers.tsx");
  expect(fixtures).toContain("web/app-features/fixtures/staff/trail.tsx");
  expect(fixtures).toContain("web/devtools/fixtures/restrictions/html-sinks.tsx");
  const want = await expected();
  expect(want.some((line) => line.includes(" e2e-flaky "))).toBe(true);
  expect(want.some((line) => line.endsWith(" className") || line.endsWith(" style"))).toBe(true);
  expect(want.some((line) => line.includes(" layers @my/"))).toBe(true);
  expect(want.some((line) => line.includes(" platform "))).toBe(true);
  expect(want.some((line) => line.includes(" e2e "))).toBe(true);
  expect(want.some((line) => line.endsWith(" trail"))).toBe(true);
  for (const kind of [
    "color",
    "layout",
    "layout-part",
    "viewport",
    "switch",
    "section",
    "heading",
    "count",
    "date",
    "pager",
    "html",
  ])
    expect([kind, want.some((line) => line.includes(` ${kind} `))]).toEqual([kind, true]);
  expect(lint()).toEqual(want);
}, 60_000);

test("layoutOf tells layout from a part's own box", () => {
  const layout = ["flex", "md:inline-grid", "-mt-2", "gap-x-feed", "w-full", "h-[1lh]", "size-1/2", "max-h-chip"];
  const own = [
    "p-chip-inline",
    "px-0",
    "h-chip",
    "size-spinner",
    "min-h-control-form",
    "hidden",
    "contents",
    "text-body",
  ];
  for (const c of layout) expect([c, layoutOf(c) === null]).toEqual([c, false]);
  for (const c of own) expect([c, layoutOf(c)]).toEqual([c, null]);
});

test("viewportOf reads the breakpoints of the tokens, and no container query", () => {
  expect(BREAKPOINTS).toEqual(Object.keys(breakpoint));
  for (const name of BREAKPOINTS)
    expect([name, viewportOf(`${name}:flex`), viewportOf(`max-${name}:flex`)]).toEqual([name, true, true]);
  for (const c of ["@reply:flex", "@max-[20rem]:flex", "hover:flex", "max-w-full", "min-h-dvh"])
    expect([c, viewportOf(c)]).toEqual([c, false]);
});

test("the files that layout-parts.js names exist, so that the lint reads every entry", () => {
  const named = [LAYOUT_PARTS, PAGE_FRAMES].flatMap((files) => Object.keys(files));
  expect(named.length).toBeGreaterThan(0);
  for (const file of named) expect([file, existsSync(`${root}${file}`)]).toEqual([file, true]);
  // A frame of a page is a layout part.
  for (const [file, frames] of Object.entries(PAGE_FRAMES))
    for (const frame of frames) expect([frame, LAYOUT_PARTS[file]?.includes(frame)]).toEqual([frame, true]);
});
