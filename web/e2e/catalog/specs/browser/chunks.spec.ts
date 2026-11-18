// SPDX-License-Identifier: AGPL-3.0-only

// The composer's code comes as chunks of its own when it first opens (#3097, #3129), on the app's screen story: a chunk
// the network does not bring, and one it brings late, are the browser's own fetches, which no DOM simulation makes (a
// component test has every chunk before it renders). The catalog's build splits them as the app's does.

import { Frame } from "../../../specs/screens";
import type { Page, Route } from "@playwright/test";
import { expect, minimum, pc, test } from "../test";

/** The composer's view and its text's field, each a chunk of its own. */
const viewAndText = /\/assets\/(?:composer|text)-[\w-]{8}\.js$/;
/** The text's field alone. */
const text = /\/assets\/text-[\w-]{8}\.js$/;
/** The editor the text's field is made of (Lexical), a chunk of its own too. */
const editor = /\/assets\/editor-[\w-]{8}\.js$/;

/**
 * Until `back()`, the network brings no chunk named `named` that the page imports after its story is drawn: on first
 * use, or ahead of it while the page is idle. `refused` holds the ones it refused, in order.
 *
 * A name alone does not tell such a chunk: the build names a chunk after the module it begins with, so one the story
 * is drawn with can bear the name of one imported later (`composer-<hash>.js` is the design system's part of the
 * timeline and the composer's view). When the page asks does: a story is on the page only once every chunk it is
 * drawn with has come, so a chunk asked for while the page shows no story is one of those, and is brought.
 */
async function withoutImported(page: Page, named: RegExp) {
  const refused: string[] = [];
  let drawn = false;
  const refuse = async (route: Route) => {
    // The request waits for this answer, so a chunk the story is drawn with never finds the story on the page.
    drawn ||= await page.evaluate(() => (document.getElementById("storybook-root")?.childElementCount ?? 0) > 0);
    if (!drawn) return route.continue();
    refused.push(new URL(route.request().url()).pathname);
    return route.abort("failed");
  };
  await page.route(named, refuse);
  return { refused, back: () => page.unroute(named, refuse) };
}

test("a composer or an editor whose code could not be fetched says so, and Try again fetches it again", async ({
  page,
  open,
}) => {
  // Every fetch of them fails until the network comes back.
  const both = await withoutImported(page, viewAndText);
  await open("screens-timeline--normal", { width: pc, height: 800 });
  const composer = await new Frame(page).newPost();
  await expect(composer.dialog.getByRole("alert")).toHaveText("The post form could not be loaded. Try again");
  expect(both.refused).not.toEqual([]);
  await both.back();
  // Only the editor fails now.
  const field = await withoutImported(page, text);
  await composer.dialog.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(composer.dialog.getByRole("alert")).toHaveText("The text field could not be loaded. Try again");
  await expect(composer.postButton).toBeVisible();
  expect(field.refused).not.toEqual([]);
  await field.back();
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
