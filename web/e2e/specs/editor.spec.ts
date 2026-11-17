// SPDX-License-Identifier: AGPL-3.0-only

// The composer's text and content warning are a Lexical editor (#3097, ADR-0092 §4), and this is what of it needs a
// browser or the server: a writer types and edits at the caret and composes with a Japanese input method, a custom
// emoji is one unit for the caret, hashtags, mentions and URLs never break inside, the editor's code is fetched again
// after it failed to come, and what is sent is the same plain text the server keeps and counts.
// Android's input (Gboard) cannot run here: it is checked on a device or an emulator, and stated in the pull request.

import { Frame } from "./screens";
import { expect, test } from "./test";

// Without the app's Service Worker: a chunk it precached would never fail to come, and the failures below are those of
// a visit before it has (page.route sees the page's own fetches alone).
test.use({ serviceWorkers: "block" });

test("the text is a named text box of several lines; typed and edited in the middle, it holds what was written", async ({
  page,
  signIn,
  fresh,
}) => {
  await signIn(await fresh.person());
  const composer = await new Frame(page).newPost();
  await expect(composer.text).toBeFocused();
  await expect(composer.text).toHaveAttribute("aria-multiline", "true");

  await composer.write("Hello world");
  await composer.holds("Hello world");
  await composer.text.press("Home");
  for (const _ of "Hello") await composer.text.press("ArrowRight");
  await page.keyboard.type(",");
  await composer.holds("Hello, world");

  // Enter writes a line break.
  await composer.text.press("End");
  await page.keyboard.press("Enter");
  await page.keyboard.type("Second line");
  await composer.holds("Hello, world\nSecond line");
});

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

test("a custom emoji shows as its image, is one unit for the caret and Backspace, and is read as its shortcode", async ({
  page,
  signIn,
  fresh,
}) => {
  await signIn(await fresh.person());
  const composer = await new Frame(page).newPost();
  await composer.write("Tea :teacup: ");
  await expect(composer.text.getByRole("img", { name: ":teacup:", exact: true })).toBeVisible();
  await composer.holds("Tea :teacup: ");

  // Backspace takes the space, then the whole emoji.
  await page.keyboard.press("Backspace");
  await composer.holds("Tea :teacup:");
  await page.keyboard.press("Backspace");
  await composer.holds("Tea ");
  await expect(composer.text.getByRole("img", { name: ":teacup:", exact: true })).toHaveCount(0);

  // One ArrowLeft moves the caret over the whole emoji.
  await page.keyboard.type(":teacup:");
  await expect(composer.text.getByRole("img", { name: ":teacup:", exact: true })).toBeVisible();
  await page.keyboard.press("ArrowLeft");
  await page.keyboard.type("x");
  await composer.holds("Tea x:teacup:");
});

test("an input method composes in the content warning too", async ({ page, signIn, fresh }) => {
  await signIn(await fresh.person());
  const composer = await new Frame(page).newPost();
  await composer.addWarning();
  await page.keyboard.type("Spoilers ");
  await composer.compose(["ね", "ねた", "ネタ"]);
  await composer.commit("ネタ");
  await composer.warningHolds("Spoilers ネタ");
});

test("a Japanese input method composes in the text, also inside a hashtag, and the caret stays after what it wrote", async ({
  page,
  signIn,
  fresh,
}) => {
  await signIn(await fresh.person());
  const composer = await new Frame(page).newPost();
  await composer.write("今日は");
  await composer.compose(["や", "やま", "山"]);
  await composer.holds("今日は山");
  await composer.commit("山");
  await composer.holds("今日は山");
  await page.keyboard.type("!");
  await composer.holds("今日は山!");

  // Inside the hashtag #山, after its 山: the composition and its commit go at the caret, and so does what follows.
  await composer.replace("#山 walk");
  await composer.holds("#山 walk");
  await composer.text.press("Home");
  for (const _ of "#山") await composer.text.press("ArrowRight");
  await composer.compose(["の", "のう", "のうえ"]);
  await composer.holds("#山のうえ walk");
  await composer.compose(["の上"]);
  await composer.commit("の上");
  await composer.holds("#山の上 walk");
  await page.keyboard.type("の小屋");
  await composer.holds("#山の上の小屋 walk");
  await expect.poll(() => composer.linesOf("#山の上の小屋")).toEqual([1]);
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

test("hashtags and mentions never break inside at 320 px, also right to left; a URL longer than a line starts one", async ({
  page,
  signIn,
  fresh,
}) => {
  await signIn(await fresh.person());
  const composer = await new Frame(page).newPost();
  await page.setViewportSize({ width: 320, height: 640 });

  // Japanese breaks between any two characters, so the tokens land at every place of a line.
  const japanese = Array.from({ length: 10 }, (_, i) => `${"あ".repeat(2 * i + 1)} #山の上の小屋 @mountain_hut `).join(
    "",
  );
  await composer.replace(japanese);
  await expect.poll(() => composer.linesOf("#山の上の小屋")).toEqual(Array(10).fill(1));
  await expect.poll(() => composer.linesOf("@mountain_hut")).toEqual(Array(10).fill(1));

  const arabic = Array.from({ length: 10 }, (_, i) => `${"م".repeat(2 * i + 1)} مرحبا #سلام_عليكم @layla `).join("");
  await composer.replace(arabic);
  await expect.poll(() => composer.linesOf("#سلام_عليكم")).toEqual(Array(10).fill(1));
  await expect.poll(() => composer.linesOf("@layla")).toEqual(Array(10).fill(1));
  // Its first line reads from the right.
  await expect.poll(async () => (await composer.sideOf("م مرحبا"))[0]).toBe("right");

  const url = `https://example.com/${"long-path-segment-".repeat(6)}end`;
  await composer.replace(`Read this: ${url} and more`);
  await expect.poll(async () => (await composer.linesOf(url))[0]).toBeGreaterThan(1);
  // The text before it is on the line above its first.
  await expect
    .poll(async () => {
      const [alone = 0] = await composer.linesOf(url);
      const [withBefore = 0] = await composer.linesOf(`this: ${url}`);
      return withBefore - alone;
    })
    .toBe(1);
  await expect.poll(() => composer.sideOf(url)).toEqual(["left"]);
});
