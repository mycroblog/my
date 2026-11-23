// SPDX-License-Identifier: AGPL-3.0-only

// Which font draws a script (ADR-0009 §5, ADR-0091 §8–§10): Latin, Greek and Cyrillic with the self-hosted Noto Sans,
// Han, kana and Hangul with Noto Sans CJK of the text's language's region, an emoji with Noto Color Emoji; and a page
// fetches the files of the characters it shows and no other. Chromium tells which font drew a piece of text over the
// DevTools protocol; it skips the OpenType-SVG source of each emoji face, which it cannot draw, for the COLRv1 one.

import { drawing, FontFiles, notoCjk, notoEmoji, notoSans } from "../fonts";
import { expect, test } from "../test";

test("Latin alone draws with Noto Sans, and fetches no slice of Noto Sans CJK and no emoji font", async ({
  page,
  open,
}) => {
  const files = new FontFiles(page);
  await open("design-system-cjk--latin-only");
  await expect.poll(() => drawing(page, "A post in English draws its Latin letters")).toEqual([notoSans]);
  await files.settled();
  // emoji.css is on the page, a face per split, and none of them is loaded.
  await expect.poll(() => files.emojiFaces()).toEqual({ declared: 11, loaded: 0 });
  expect(files.emoji).toEqual([]);
  expect(files.cjk).toEqual([]);
});

// The tests of which font draws, without the files a page fetches: one page opens a story per step, the fonts it
// fetched before kept by the browser.
test("Han characters draw with Noto Sans CJK of the region of their text's language, of every tag of that region and of the CJK language around them, also in code", async ({
  page,
  open,
}) => {
  await test.step("Han characters draw with Noto Sans CJK of the region of their text's language, the page's for English", async () => {
    await open("design-system-cjk--regions");
    // The same Han characters, whose glyphs differ by region; the Latin and the spaces of a line draw with Noto Sans.
    const lines = [
      ["en", "Straight bone: 直 骨 角 写 誤", [notoSans, notoCjk("jp")]],
      ["ja", "直線の骨と角を写す誤り", [notoCjk("jp")]],
      ["zh-CN", "直线的骨与角写错了", [notoCjk("sc")]],
      ["zh-TW", "直線的骨與角寫錯了", [notoCjk("tc")]],
      ["ko", "직선 直 骨 角 寫 誤 한국어", [notoSans, notoCjk("kr")]],
    ] as const;
    for (const [language, line, fonts] of lines)
      await expect.poll(async () => ({ language, fonts: await drawing(page, line) })).toEqual({ language, fonts });
  });

  await test.step("a Han character takes the region of every tag of that region, and of the CJK language around a text in another", async () => {
    // A page with no Han of its own, where each probe is the only Han character.
    await open("design-system-cjk--latin-only");
    const probes = [
      ["zh-Hans", undefined, "sc"],
      ["zh-Hant", undefined, "tc"],
      ["zh-HK", undefined, "tc"],
      ["zh-MO", undefined, "tc"],
      ["en", "zh-TW", "tc"],
      ["fr", "ko", "kr"],
      ["en", "zh-CN", "sc"],
    ] as const;
    for (const [lang, around, region] of probes) {
      await page.evaluate(
        ([language, outside]) => {
          const outer = document.createElement("div");
          outer.id = "probe";
          if (outside !== undefined) outer.lang = outside;
          const probe = document.createElement("span");
          probe.lang = language;
          probe.textContent = "直";
          outer.append(probe);
          document.body.append(outer);
        },
        [lang, around] as const,
      );
      await expect
        .poll(async () => ({ lang, around, fonts: await drawing(page, "直") }))
        .toEqual({ lang, around, fonts: [notoCjk(region)] });
      await page.evaluate(() => {
        document.getElementById("probe")?.remove();
      });
    }
  });

  await test.step("code in a post keeps the mono stack, and its Han characters the region of the code's language", async () => {
    await open("design-system-cjk--code");
    const regions = [
      ["inline", "const 茶", "jp"],
      ["code-lang", "直线", "sc"],
      ["block", "// 直", "jp"],
      ["part-lang", "한국어 直", "kr"],
    ] as const;
    for (const [probe, text, region] of regions) {
      const stack = await page.evaluate(
        (name) => getComputedStyle(document.querySelectorAll(`[data-probe="${name}"]`)[0] ?? document.body).fontFamily,
        probe,
      );
      expect(stack, probe).toContain("ui-monospace");
      expect(stack, probe).not.toContain("system-ui");
      await expect.poll(() => drawing(page, text), probe).toContainEqual(notoCjk(region));
    }
  });
});

test("Traditional Chinese draws with Noto Sans CJK TC, and fetches the slices of its characters alone", async ({
  page,
  open,
}) => {
  const files = new FontFiles(page);
  await open("design-system-scripts--traditional-chinese");
  await expect.poll(() => drawing(page, "我們一起喝茶")).toEqual([notoCjk("tc")]);
  await files.settled();
  // The slice of TC that holds 茶 is fetched; no slice of another region; and each slice fetched claims a character
  // the page shows.
  const tea = await files.sliceOf("tc", "茶");
  expect(tea).toBeDefined();
  expect(files.cjk.filter((s) => s.region === "tc").map((s) => s.slice)).toContain(tea);
  expect(files.cjk.filter((s) => s.region !== "tc")).toEqual([]);
  await expect.poll(() => files.unclaimed()).toEqual([]);
});

test("an emoji draws with the self-hosted Noto Color Emoji, and fetches its split alone", async ({ page, open }) => {
  const files = new FontFiles(page);
  await open("design-system-scripts--one-emoji");
  await expect.poll(() => drawing(page, "🍞")).toContainEqual(notoEmoji);
  await files.settled();
  expect(files.emoji).toEqual(["noto-emoji-colrv1-food-drink"]);
});

// As the test of Han characters, one page for the steps.
test("a word in letters that Noto Sans CJK lacks draws with one font; each split of the emoji font loads for the emoji it claims; a theme's own font draws the Latin alone", async ({
  page,
  open,
}) => {
  await test.step("a word in letters that Noto Sans CJK lacks draws with one font, also inside Japanese text", async () => {
    await open("design-system-scripts--letters-outside-cjk");
    // Polish and Ukrainian, whose ż ó ł ć ę ś ą ń ї Noto Sans CJK lacks: each word drew with two fonts before.
    for (const [language, line] of [
      ["pl", "Zażółć gęślą jaźń"],
      ["uk", "їжак"],
      ["ja", "Zażółć gęślą jaźń їжак"],
    ] as const)
      await expect
        .poll(async () => ({ language, fonts: await drawing(page, line) }))
        .toEqual({ language, fonts: [notoSans] });
  });

  await test.step("each split of the emoji font loads for the emoji it claims, and the text symbols stay with the text fonts", async () => {
    await open("design-system-emoji--all");
    const font = '16px "my Noto Color Emoji"';
    // The story shows a sample of each split, named by the split.
    const samples = await page.evaluate(() =>
      Array.from(document.querySelectorAll<HTMLElement>("[data-split]"), (row) => ({
        split: row.dataset["split"] ?? "",
        emoji: row.querySelector("span")?.textContent ?? "",
      })),
    );
    expect(samples.map((s) => s.split)).toHaveLength(11);

    for (const { split, emoji } of samples) {
      const loaded = await page.evaluate(
        async ([face, text]) => {
          // A face of the family claims the emoji, it loads, and the family then draws them all.
          const faces = await document.fonts.load(face, text);
          return {
            faces: faces.length,
            errors: faces.filter((f) => f.status === "error").length,
            loaded: faces.every((f) => f.status === "loaded"),
          };
        },
        [font, emoji] as const,
      );
      expect({ split, ...loaded }).toEqual({ split, faces: loaded.faces, errors: 0, loaded: true });
      expect(loaded.faces, split).toBeGreaterThan(0);
    }
    // The keycap bases and the symbols of running text stay with the text fonts: no face claims them.
    const claimed = await page.evaluate(
      async ([face]) => (await document.fonts.load(face as string, "#*0123456789©®™‼⁉↔")).length,
      [font] as const,
    );
    expect(claimed).toBe(0);
  });

  await test.step("a theme's own font draws the Latin, and the Han and the emoji stay with the self-hosted fonts", async () => {
    await open("design-system-scripts--latin-han-and-emoji");
    await expect.poll(() => drawing(page, "Tea time 茶 🍵")).toEqual([notoEmoji, notoSans, notoCjk("jp")]);
    // A theme sets the token (ADR-0059 §2): a font of the system with Latin alone, which Playwright's Chromium
    // installs with its dependencies.
    await page.addStyleTag({ content: ':root { --font-sans-override: "Liberation Sans"; }' });
    await expect
      .poll(() => drawing(page, "Tea time 茶 🍵"))
      .toEqual([{ familyName: "Liberation Sans", isCustomFont: false }, notoEmoji, notoCjk("jp")]);
  });
});

test("an emoji written in a text box draws with the self-hosted Noto Color Emoji", async ({ page, open }) => {
  const files = new FontFiles(page);
  await open("design-system-editor--empty");
  const text = page.getByRole("textbox", { name: "Text", exact: true });
  await text.click();
  await page.keyboard.insertText("Lunch on the ridge 🍙");
  await expect(text).toHaveText("Lunch on the ridge 🍙");
  await expect.poll(() => drawing(page, "🍙")).toContainEqual(notoEmoji);
  await files.settled();
  expect(files.emoji).toEqual(["noto-emoji-colrv1-food-drink"]);
});
