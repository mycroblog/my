// SPDX-License-Identifier: AGPL-3.0-only

// The test of the catalog's specs: Playwright's, with the served catalog as its base URL, and `open`, which shows one
// story as the test asks for it: at a viewport's width, in a locale (ar-XB reads from right to left) and a theme. A
// story pins none of them (ADR-0171 §4), and a test never depends on a story's `play`.

import { breakpoint, rootPx } from "@my/design-tokens";
import { expect, test as base, type Page } from "@playwright/test";

export { expect };

/** The minimum width no page may scroll sideways at (ADR-0110 §10). */
export const minimum = 320;

/** The viewport's width, in CSS px, from which the app's frame changes (the tokens' breakpoints). */
export const from = {
  tablet: breakpoint.tablet * rootPx,
  desktop: breakpoint.desktop * rootPx,
  wide: breakpoint.wide * rootPx,
} as const;

/** The width of a desktop's window, past every breakpoint. */
export const pc = 1440;

/** How a test asks for a story. */
export type View = {
  /** The viewport's width in CSS px; the minimum width by default. */
  width?: number;
  /** The viewport's height in CSS px. */
  height?: number;
  /** The catalog's locale; `ar-XB`, the pseudo-locale made from `en`, reads from right to left. */
  locale?: "en" | "ja" | "ar-XB";
  theme?: "light" | "dark";
  /** The story's args, as its controls would set them. */
  args?: Readonly<Record<string, string | number | boolean>>;
};

/** The catalog's origin, which global-setup.ts set for the workers. */
function catalogURL(): string {
  const url = process.env["CATALOG_URL"];
  if (url === undefined)
    throw new Error("catalog: no served catalog; run the tests with scripts/task.sh web:layout-test");
  return url;
}

/** The address of a story alone on its page, as the catalog's canvas frames it. */
export function storyURL(id: string, { locale = "en", theme = "light", args = {} }: View = {}): string {
  const query = new URLSearchParams({ id, viewMode: "story", globals: `locale:${locale};theme:${theme}` });
  const set = Object.entries(args).map(
    ([name, value]) => `${name}:${typeof value === "boolean" ? "!" : ""}${String(value)}`,
  );
  if (set.length > 0) query.set("args", set.join(";"));
  return `/iframe.html?${query.toString()}`;
}

/** Shows the story `id` on `page` and returns once it is drawn with the fonts it asked for. */
export async function openStory(page: Page, id: string, view: View = {}): Promise<void> {
  await page.setViewportSize({ width: view.width ?? minimum, height: view.height ?? 640 });
  await page.goto(storyURL(id, view));
  await drawn(page);
}

/**
 * Returns once the page shows its story with every font it started to load, and throws what the catalog says when
 * it cannot show it (no story of that id, or one that throws).
 */
export async function drawn(page: Page): Promise<void> {
  const failure = await page.waitForFunction(async () => {
    const shows = (state: string) => document.body.classList.contains(`sb-show-${state}`);
    if (shows("errordisplay") || shows("nopreview")) return document.body.innerText.trim().slice(0, 400) || "no story";
    if (!shows("main") || (document.getElementById("storybook-root")?.childElementCount ?? 0) === 0) return false;
    await document.fonts.ready;
    return "";
  });
  const said = await failure.jsonValue();
  if (said !== "") throw new Error(`catalog: ${page.url()} shows no story: ${String(said)}`);
}

type Fixtures = {
  /** Shows a story of the catalog on the test's page. */
  open: (id: string, view?: View) => Promise<void>;
};

export const test = base.extend<Fixtures>({
  open: async ({ page }, use) => {
    await use((id, view) => openStory(page, id, view));
  },
  // A fixture, not `test.use` in this module: a module runs once per worker, so its `test.use` reaches only the spec
  // file that imported it first.
  baseURL: async ({ baseURL }, use) => {
    await use(baseURL ?? catalogURL());
  },
});
