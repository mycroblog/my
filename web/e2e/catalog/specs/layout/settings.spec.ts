// SPDX-License-Identifier: AGPL-3.0-only

// The settings in the app's frame (ADR-0110 §5), on the app's own stories: the menu at /settings from the minimum
// width to a desktop's, in English and in Japanese and in both themes, its rows one under another inside the screen;
// and an entry's screen, which under `wide` stands alone with the way back to the menu at its head (the menu first,
// then the screen), and from `wide` has the menu beside it, in the frame's column at the end, marking the entry. What
// the menu holds and where its rows lead is the app's component test (web/app/src/settings.dom.test.tsx); that the
// column is at the left when the page reads from right to left is the frame's (shell.spec.ts).

import { expectNoComputedViolations } from "../axe";
import { boxesAtRest, expectInside, expectNoSidewaysScroll } from "../boxes";
import { expect, from, minimum, pc, test } from "../test";

const height = 640;

// The words of the menu in each language the app has.
const menus = [
  { locale: "en", settings: "Settings", group: "General", display: /^Display and posting.+$/, signOut: "Sign out…" },
  { locale: "ja", settings: "設定", group: "全般", display: /^表示と投稿.+$/, signOut: "ログアウト…" },
] as const;

for (const { locale, settings, group, display, signOut } of menus)
  test(`the menu in ${locale}, at the minimum width and on a desktop, in both themes: its rows one under another inside the screen`, async ({
    page,
    open,
  }) => {
    const main = page.getByRole("main");
    const menu = page.getByRole("navigation", { name: settings, exact: true });
    const parts = {
      main,
      heading: main.getByRole("heading", { level: 1, name: settings, exact: true }),
      group: menu.getByRole("heading", { name: group, exact: true }),
      display: menu.getByRole("link", { name: display }),
      signOut: menu.getByRole("link", { name: signOut, exact: true }),
    };

    await open("screens-settings--menu", { locale, height });

    for (const width of [minimum, pc]) {
      await page.setViewportSize({ width, height });
      await expectNoSidewaysScroll(page);
      // The menu is the screen: one navigation of the settings, inside the main column.
      await expect(menu).toHaveCount(1);
      await expectInside(menu, main);
      const boxes = await boxesAtRest(parts);
      expect(boxes.group.top).toBeGreaterThanOrEqual(boxes.heading.bottom);
      expect(boxes.display.top).toBeGreaterThanOrEqual(boxes.group.bottom);
      expect(boxes.signOut.top).toBeGreaterThanOrEqual(boxes.display.bottom);
      expect(boxes.signOut.left).toBe(boxes.display.left);
      // A row's surface may reach past the menu's text toward the screen's edges, never past the screen.
      await expectInside(parts.display, main);
      await expectInside(parts.signOut, main);
      await expectNoComputedViolations(page);
    }
  });

test("an entry's screen has the way back to the menu under `wide`, and the menu beside it from `wide`", async ({
  page,
  open,
}) => {
  const navigation = page.getByRole("navigation", { name: "Main", exact: true });
  const main = page.getByRole("main");
  const menu = page.getByRole("navigation", { name: "Settings", exact: true });
  const back = main.getByRole("button", { name: "Settings", exact: true });
  const language = main.getByRole("combobox", { name: "Display language", exact: true });
  const arriving = main.getByRole("switch", { name: "Show new posts as they arrive", exact: true });
  const at = async (width: number) => {
    await page.setViewportSize({ width, height });
    await expectNoSidewaysScroll(page);
  };

  await open("screens-settings--display", { height });

  await test.step("under `wide`: the screen alone, its way back at its head, every control inside it", async () => {
    for (const width of [minimum, from.tablet, from.desktop, from.wide - 1]) {
      await at(width);
      await expect(menu).toBeHidden();
      await expect(back).toBeVisible();
      const alone = await boxesAtRest({ back, language });
      expect(alone.back.bottom).toBeLessThanOrEqual(alone.language.top);
      await expectInside(language, main);
      await expectInside(arriving, main);
    }
    await at(minimum);
    await expectNoComputedViolations(page);
  });

  await test.step("from `wide`: the menu beside the screen, at the end, marking the entry, and no way back", async () => {
    for (const width of [from.wide, pc]) {
      await at(width);
      await expect(menu).toBeVisible();
      await expect(back).toBeHidden();
      await expect(menu.getByRole("link", { name: "Display and posting", exact: true })).toHaveAttribute(
        "aria-current",
        "page",
      );
      const wide = await boxesAtRest({ navigation, main, menu });
      expect(wide.main.left).toBeGreaterThanOrEqual(wide.navigation.right);
      expect(wide.menu.left).toBeGreaterThanOrEqual(wide.main.right);
      expect(wide.menu.right).toBeLessThanOrEqual(width);
      await expectInside(language, main);
    }
    await expectNoComputedViolations(page);
  });
});
