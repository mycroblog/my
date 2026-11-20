// SPDX-License-Identifier: AGPL-3.0-only

// The theme's screen (web/app/src/admin-theme.stories.tsx) calls for Publish in the accent only while the draft differs
// from the theme in effect: a draft not published yet draws Publish apart from Preview, and a draft that is the
// published theme draws both alike. What the presses do is the app's route tests' (web/app/src/admin-theme.dom.test.tsx);
// how a button looks is what only a browser computes.

import type { Page } from "@playwright/test";
import { expect, pc, test } from "../test";

/** The story of the theme's screen named `name`. */
const story = (name: string) => `screens-administration-theme--${name}`;

/** Whether Publish is drawn on the same background as Preview, once both are on the screen. */
async function publishLikePreview(page: Page): Promise<boolean> {
  const main = page.getByRole("main");
  const publish = main.getByRole("button", { name: "Publish", exact: true });
  const preview = main.getByRole("button", { name: "Preview", exact: true });
  await expect(publish).toBeVisible();
  await expect(preview).toBeVisible();
  const backgroundOf = (button: HTMLElement | SVGElement) => getComputedStyle(button).backgroundColor;
  const [own, other] = await Promise.all([publish.evaluate(backgroundOf), preview.evaluate(backgroundOf)]);
  return own === other;
}

test("Publish is called for in the accent while the draft differs from the theme in effect, and is not once it is that theme", async ({
  page,
  open,
}) => {
  await open(story("drafted"), { width: pc });
  await expect.poll(() => publishLikePreview(page)).toBe(false);

  await open(story("published"), { width: pc });
  await expect.poll(() => publishLikePreview(page)).toBe(true);
});
