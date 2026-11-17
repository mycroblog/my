// SPDX-License-Identifier: AGPL-3.0-only

// The lines of the composer's text at the minimum width, on the app's screen story: a hashtag and a mention are
// never broken inside, wherever the line ends and in either direction of the text, and an address longer than a line
// starts one.

import { Frame } from "../../../specs/screens";
import { expect, minimum, pc, test } from "../test";

test("in the composer's text at the minimum width, a hashtag and a mention stay whole on a line, and a long address starts one", async ({
  page,
  open,
}) => {
  await open("screens-timeline--normal", { width: pc, height: 800 });
  const composer = await new Frame(page).newPost();
  await page.setViewportSize({ width: minimum, height: 640 });

  await test.step("a hashtag that the end of a line falls in goes to the next line whole", async () => {
    const tag = "#川沿いの公園の写真";
    // Japanese breaks between any two characters, so some of these lengths put the hashtag across the line's end;
    // the comma lets the hashtag start, as a letter before `#` would not.
    let wrapped = false;
    for (let length = 6; length <= 24; length += 1) {
      const text = `${"あ".repeat(length)}、${tag}`;
      await composer.replace(text);
      await composer.holds(text);
      await expect.poll(() => composer.linesOf(tag)).toEqual([1]);
      // On a line under the text's first: the first line reads from the left, the hashtag alone starts the next.
      wrapped ||= (await composer.linesOf(text))[0] === 2;
    }
    expect(wrapped).toBe(true);
  });

  await test.step("hashtags and mentions at every place of a line, left to right and right to left", async () => {
    const japanese = Array.from(
      { length: 10 },
      (_, i) => `${"あ".repeat(2 * i + 1)} #山の上の小屋 @mountain_hut `,
    ).join("");
    await composer.replace(japanese);
    await expect.poll(() => composer.linesOf("#山の上の小屋")).toEqual(Array(10).fill(1));
    await expect.poll(() => composer.linesOf("@mountain_hut")).toEqual(Array(10).fill(1));

    const arabic = Array.from({ length: 10 }, (_, i) => `${"م".repeat(2 * i + 1)} مرحبا #سلام_عليكم @layla `).join("");
    await composer.replace(arabic);
    await expect.poll(() => composer.linesOf("#سلام_عليكم")).toEqual(Array(10).fill(1));
    await expect.poll(() => composer.linesOf("@layla")).toEqual(Array(10).fill(1));
    // Its first line reads from the right.
    await expect.poll(async () => (await composer.sideOf("م مرحبا"))[0]).toBe("right");
  });

  await test.step("an address longer than a line starts a line of its own, from the left", async () => {
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
});
