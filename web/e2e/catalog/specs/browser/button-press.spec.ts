// SPDX-License-Identifier: AGPL-3.0-only

// A press on a button while it was disabled is not held against it: once the button is enabled without a pointer, every
// press with a real mouse counts, which no DOM simulation shows. The Counting story of the button shows both kinds of
// button disabled, and the test enables them through Storybook's channel as the page's script would, so the page is not
// reloaded and the mouse stays where it pressed.

import type { Page } from "@playwright/test";
import { expect, test } from "../test";

const story = "design-system-button--counting";

/** Sets the args of the open story, as the catalog's controls do, without a pointer and without a reload. */
async function setArgs(page: Page, updatedArgs: Record<string, boolean>): Promise<void> {
  await page.evaluate(
    ([storyId, args]) => {
      const channel = (
        window as unknown as { __STORYBOOK_ADDONS_CHANNEL__: { emit: (name: string, ...a: unknown[]) => void } }
      ).__STORYBOOK_ADDONS_CHANNEL__;
      channel.emit("updateStoryArgs", { storyId, updatedArgs: args });
    },
    [story, updatedArgs] as const,
  );
}

for (const [kind, label, counter] of [
  ["button", "Press the button", "Button pressed"],
  ["text button", "Press the text button", "Text button pressed"],
] as const) {
  test(`every press of the ${kind} counts once it is enabled after a press on it while it was disabled`, async ({
    page,
    open,
  }) => {
    await open(story);
    const target = page.getByRole("button", { name: label, exact: true });
    const count = page.getByLabel(counter, { exact: true });
    await expect(target).toBeDisabled();
    const box = await target.boundingBox();
    if (box === null) throw new Error("test: the disabled button has no box");
    const middle = { x: box.x + box.width / 2, y: box.y + box.height / 2 };

    await page.mouse.move(middle.x, middle.y);
    await page.mouse.down();
    await page.mouse.up();
    await expect(count).toHaveText("0");

    await setArgs(page, { isDisabled: false });
    await expect(target).toBeEnabled();

    for (const pressed of ["1", "2", "3"]) {
      await page.mouse.down();
      await page.mouse.up();
      await expect(count).toHaveText(pressed);
    }
  });
}
