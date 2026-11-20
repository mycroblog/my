// SPDX-License-Identifier: AGPL-3.0-only

// The shell's Content-Security-Policy on the real server (#3211): no style or script of the app is refused on the
// home timeline, in a dialog, and in the settings, and the pressables take the rule React Aria would otherwise put in
// a style of its own (`touch-action`), from the design system's stylesheet. Each refusal is a `securitypolicyviolation`
// event of the document, which a script the page runs before its own collects.

import type { Page } from "@playwright/test";
import { Frame, Settings } from "./screens";
import { expect, test } from "./test";

/** What the policy refused on the page so far, each as its directive, what it blocked and the start of it. */
const refused = (page: Page): Promise<string[]> =>
  page.evaluate(() => (window as unknown as { refused?: string[] }).refused ?? []);

/**
 * The `touch-action` of the first pressable of React Aria on the page, and whether a style claims its rule. Chromium
 * reads `pan-x pan-y pinch-zoom` back as `manipulation`, which allows the same; a pressable without the rule is `auto`.
 */
const pressables = (page: Page) =>
  page.evaluate(() => {
    const pressable = document.querySelector("[data-react-aria-pressable]");
    return {
      touchAction: pressable === null ? "no pressable" : getComputedStyle(pressable).touchAction,
      injected: document.querySelector("style#react-aria-pressable-style") !== null,
    };
  });

test("the shell's policy refuses nothing on the home timeline, in a dialog and in the settings, and a pressable takes its rule from the stylesheet", async ({
  page,
  signIn,
  fresh,
}) => {
  await page.addInitScript(() => {
    const seen: string[] = [];
    (window as unknown as { refused: string[] }).refused = seen;
    document.addEventListener("securitypolicyviolation", (event) => {
      seen.push(`${event.effectiveDirective} ${event.blockedURI} ${event.sample}`.trim());
    });
  });
  await signIn(await fresh.person());
  const frame = new Frame(page);
  await expect.poll(() => pressables(page)).toEqual({ touchAction: "manipulation", injected: false });
  await expect.poll(() => refused(page)).toEqual([]);

  const composer = await frame.newPost();
  await expect(composer.dialog).toBeVisible();
  await expect.poll(() => refused(page)).toEqual([]);
  await page.keyboard.press("Escape");
  await expect(composer.dialog).toBeHidden();

  await frame.goTo("Settings");
  await expect(new Settings(page).heading).toBeVisible();
  await expect.poll(() => pressables(page)).toEqual({ touchAction: "manipulation", injected: false });
  await expect.poll(() => refused(page)).toEqual([]);
});
