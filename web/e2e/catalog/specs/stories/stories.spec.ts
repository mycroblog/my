// SPDX-License-Identifier: AGPL-3.0-only

// The check of every story at once (ADR-0171 §6), an upper layer that runs in the slow suite and on no pull request:
// each story of the built catalog's index, at the minimum width, from left to right (en) and from right to left
// (ar-XB) in the light theme, and in the dark theme when it is tagged `themes`. A story passes when it is drawn and
// throws nothing, does not scroll sideways (ADR-0110 §10), and has no axe violation of the tags of ADR-0110 §9. The
// check reads the page once, at rest (rest.ts), and tries nothing again: a story that passes only sometimes has a
// cause. What it finds is a gap of a lower layer: the fix adds the smallest test there, never a case here.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { root } from "../../../src/server/paths";
import { violationsOfTags } from "../axe";
import { sidewaysScroll } from "../boxes";
import { hold } from "../rest";
import { expect, minimum, openStory, test, type View } from "../test";

/** A story as the catalog's index lists it. */
type Story = { type: string; id: string; tags?: string[] };

/** The stories of the built catalog. */
function stories(): Story[] {
  const index = join(root, "web/storybook/dist/index.json");
  let read: string;
  try {
    read = readFileSync(index, "utf8");
  } catch {
    throw new Error(
      "catalog: no built catalog in web/storybook/dist; run the check with scripts/task.sh web:stories-check",
    );
  }
  const { entries } = JSON.parse(read) as { entries: Record<string, Story> };
  return Object.values(entries).filter((entry) => entry.type === "story");
}

/** How every story is shown, and how the stories whose look the theme changes are shown once more. */
const views: { name: string; view: View; tag?: string }[] = [
  { name: "from left to right", view: { locale: "en", theme: "light" } },
  { name: "from right to left", view: { locale: "ar-XB", theme: "light" } },
  { name: "in the dark theme", view: { locale: "en", theme: "dark" }, tag: "themes" },
];

/**
 * What the page shows of its story: what the catalog says when it cannot show it, else whether anything the story
 * put on the page, in the catalog's root or over it, has a box.
 */
const shown = (page: Page): Promise<string> =>
  page.evaluate(() => {
    const shows = (name: string) => document.body.classList.contains(`sb-show-${name}`);
    if (!shows("main")) return document.body.innerText.trim().slice(0, 400) || "no story";
    const catalogs = "#storybook-docs, #storybook-highlights-root, .sb-wrapper";
    const boxed = [...document.body.querySelectorAll("*")].some((element) => {
      const { width, height } = element.getBoundingClientRect();
      return width > 0 && height > 0 && element.id !== "storybook-root" && element.closest(catalogs) === null;
    });
    return boxed ? "drawn" : "nothing drawn";
  });

for (const story of stories())
  for (const { name, view, tag } of views) {
    if (tag !== undefined && story.tags?.includes(tag) !== true) continue;
    test(`${story.id} ${name}`, async ({ page }) => {
      const thrown: string[] = [];
      page.on("pageerror", (error) => thrown.push(error.message));
      const held = await hold(page);
      await openStory(page, story.id, { ...view, width: minimum });
      await held.rest();
      const found = {
        thrown,
        shown: await shown(page),
        sideways: await sidewaysScroll(page),
        axe: await held.during(violationsOfTags(page)),
      };
      expect(found).toEqual({ thrown: [], shown: "drawn", sideways: [], axe: [] });
    });
  }
