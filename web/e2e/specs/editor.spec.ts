// SPDX-License-Identifier: AGPL-3.0-only

// The composer's text and content warning are a Lexical editor (#3097, ADR-0092 §4), and this is what of it needs the
// app's own build or the server: the editor's code is fetched again after it failed to come, and what is sent is the
// same plain text the server keeps and counts. What it does under an input method and the caret's keys, and how its
// lines break, are tests in a browser on its story (catalog/).

import { Frame } from "./screens";
import { expect, test } from "./test";

// Without the app's Service Worker: a chunk it precached would never fail to come, and the failures below are those of
// a visit before it has (page.route sees the page's own fetches alone).
test.use({ serviceWorkers: "block" });

test("a composer or an editor whose code could not be fetched says so, and Try again fetches it again", async ({
  page,
  signIn,
  fresh,
}) => {
  // The composer's view and its editor are chunks of their own (composer-<hash>.js, text-<hash>.js); every fetch of
  // them fails until the network comes back.
  const chunks = /\/assets\/(?:composer|text)-[\w-]{8}\.js$/;
  await page.route(chunks, (route) => route.abort("failed"));
  await signIn(await fresh.person());
  const frame = new Frame(page);
  const composer = await frame.newPost();
  await expect(composer.dialog.getByRole("alert")).toHaveText("The post form could not be loaded. Try again");
  await page.unroute(chunks);
  // Only the editor fails now.
  const text = /\/assets\/text-[\w-]{8}\.js$/;
  await page.route(text, (route) => route.abort("failed"));
  await composer.dialog.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(composer.dialog.getByRole("alert")).toHaveText("The text field could not be loaded. Try again");
  await expect(composer.postButton).toBeVisible();
  await page.unroute(text);
  await composer.dialog.getByRole("button", { name: "Try again", exact: true }).click();
  await composer.write("Back online.");
  await composer.holds("Back online.");
});

test("what is sent from the editor is the text and content warning the server keeps, exactly", async ({
  page,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  await signIn(writer);
  const composer = await new Frame(page).newPost();
  const text = "朝の散歩 #山 :teacup: 😀\nhttps://example.com/path?q=1 は\n\n終わり";
  await composer.write(text);
  await composer.holds(text);
  await composer.addWarning();
  await page.keyboard.type("Mountain :wave: 写真");
  await composer.warningHolds("Mountain :wave: 写真");
  const id = await composer.postedId();
  const kept = await fresh.source(writer, id);
  expect(kept).toEqual({ text, spoiler: "Mountain :wave: 写真" });
});

test("the count agrees with the server's: a post at the limit is sent, one over it is held back and its excess marked", async ({
  page,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  await signIn(writer);
  // The server counts the code points of the text and the content warning together, 500 at most: a custom emoji's
  // shortcode counts its characters, an astral emoji one.
  const spoiler = "注意 😀";
  const head = "Limit :teacup: 😀 日本語 ";
  const atLimit = head + "a".repeat(500 - Array.from(spoiler).length - Array.from(head).length);

  let composer = await new Frame(page).newPost();
  await composer.addWarning();
  await page.keyboard.type(spoiler);
  await composer.replace(atLimit);
  await composer.holds(atLimit);
  await expect(composer.count).toHaveText("0 characters left");
  const id = await composer.postedId();
  const kept = await fresh.source(writer, id);
  expect(kept).toEqual({ text: atLimit, spoiler });

  composer = await new Frame(page).newPost();
  await composer.addWarning();
  await page.keyboard.type(spoiler);
  await composer.replace(`${atLimit}xyz`);
  await expect(composer.count).toHaveText("3 characters over");
  await expect.poll(() => composer.highlighted()).toContain("xyz");
  // Post sends nothing: the text says why, as its description.
  const sent: string[] = [];
  page.on("request", (r) => {
    if (r.method() === "POST" && new URL(r.url()).pathname.endsWith("/statuses")) sent.push(r.url());
  });
  await composer.postButton.click();
  const why = "The text and content warning must be at most 500 characters.";
  await expect(composer.dialog.getByText(why, { exact: true })).toBeVisible();
  await expect(composer.text).toHaveAttribute("aria-invalid", "true");
  await expect(composer.text).toHaveAccessibleDescription(why);
  await expect(composer.dialog).toBeVisible();
  expect(sent).toEqual([]);
});
