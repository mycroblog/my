// SPDX-License-Identifier: AGPL-3.0-only

// The composer's code comes as chunks of its own when it first opens (#3097, #3129), on the app's screen story: a chunk
// the network does not bring, and one it brings late, are the browser's own fetches, which no DOM simulation makes (a
// component test has every chunk before it renders). The catalog's build splits them as the app's does.

import { Frame } from "../../../specs/screens";
import { expect, minimum, pc, test } from "../test";

/** The composer's view and its text's field, each a chunk of its own. */
const viewAndText = /\/assets\/(?:composer|text)-[\w-]{8}\.js$/;
/** The text's field alone. */
const text = /\/assets\/text-[\w-]{8}\.js$/;
/** The editor the text's field is made of (Lexical), a chunk of its own too. */
const editor = /\/assets\/editor-[\w-]{8}\.js$/;

test("a composer or an editor whose code could not be fetched says so, and Try again fetches it again", async ({
  page,
  open,
}) => {
  // Every fetch of them fails until the network comes back.
  await page.route(viewAndText, (route) => route.abort("failed"));
  await open("screens-timeline--normal", { width: pc, height: 800 });
  const composer = await new Frame(page).newPost();
  await expect(composer.dialog.getByRole("alert")).toHaveText("The post form could not be loaded. Try again");
  await page.unroute(viewAndText);
  // Only the editor fails now.
  await page.route(text, (route) => route.abort("failed"));
  await composer.dialog.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(composer.dialog.getByRole("alert")).toHaveText("The text field could not be loaded. Try again");
  await expect(composer.postButton).toBeVisible();
  await page.unroute(text);
  await composer.dialog.getByRole("button", { name: "Try again", exact: true }).click();
  await composer.write("Back online.");
  await composer.holds("Back online.");
});

test("on a phone, the composer's text, come after its menu closed, leaves the focus on the menu's button", async ({
  page,
  open,
}) => {
  // The editor's chunk is held until the menu has closed, as a slow network brings it.
  const held: (() => void)[] = [];
  let holding = true;
  await page.route(editor, async (route) => {
    if (holding) await new Promise<void>((resolve) => held.push(resolve));
    await route.continue();
  });
  // The narrowest screen the client supports (ADR-0110 §10), where a menu is a sheet.
  await open("screens-timeline--normal", { width: minimum });
  await page.getByRole("button", { name: "New post", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "New post", exact: true });
  const audience = dialog.getByRole("button", { name: /^Who can see it: .+$/ });
  await audience.click();
  const menu = page.getByRole("menu", { name: "Who can see and reply", exact: true });
  await expect(menu).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(menu).toBeHidden();
  await expect(audience).toBeFocused();
  // The chunk was asked for and is still on its way: the text is not there yet.
  await expect.poll(() => held.length).toBeGreaterThan(0);
  const box = dialog.getByRole("textbox", { name: "Text", exact: true });
  await expect(box).toHaveCount(0);
  holding = false;
  for (const resolve of held.splice(0)) resolve();
  await expect(box).toBeVisible();
  await expect(audience).toBeFocused();
});
