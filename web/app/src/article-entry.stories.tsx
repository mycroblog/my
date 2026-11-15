// SPDX-License-Identifier: AGPL-3.0-only

// Where the article editor is opened from (#3091), played on the fake server as yuki reads her home: on a PC, New
// article under New post in the main navigation, whose New post has no menu; on a phone, the place New article that
// starts the Menu sheet, and the menu of other things to write of the floating New post (long press, right click or
// Alt+ArrowDown); and none of them while the server keeps the reader from writing new articles. In en and in ja.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { minimumWidth, screen, screenMeta } from "./app.samples";

const meta = {
  title: "Article entry",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The words of the shell and the editor the plays read, in each locale they run in. */
const words = {
  en: {
    navigation: "Main",
    newPost: "New post",
    newArticle: "New article",
    menu: "Menu",
    postMenu: "Other things to write",
    editor: "New article",
  },
  ja: {
    navigation: "メイン",
    newPost: "投稿する",
    newArticle: "新しい記事",
    menu: "メニュー",
    postMenu: "ほかに書けるもの",
    editor: "新しい記事",
  },
} as const;

type Lang = keyof typeof words;
type Words = (typeof words)[Lang];

const pc = { viewport: { value: "pc" } };
const permission = screen({ at: "/", server: { articles: { authoring: "permission" } } });

const canvasOf = (root: HTMLElement) => within(root);

/** The queries of the canvas. */
type Canvas = ReturnType<typeof canvasOf>;

/** The home in the canvas in `lang`, once it shows its posts. */
async function home(context: StoryContext, lang: Lang): Promise<{ canvas: Canvas; w: Words }> {
  await screenMeta.play(context);
  await expect(document.documentElement.lang).toBe(lang);
  return { canvas: canvasOf(context.canvasElement), w: words[lang] };
}

/** The editor of a new article, opened in place of the screen. */
async function opened(canvas: Canvas, w: Words) {
  await waitFor(() => expect(canvas.getByRole("heading", { level: 1, name: w.editor })).toBeVisible());
}

/** On a PC: New article under New post opens the editor; New post opens no menu. */
async function navigationEntry(context: StoryContext, lang: Lang) {
  const { canvas, w } = await home(context, lang);
  const nav = within(canvas.getByRole("navigation", { name: w.navigation }));
  const newPost = nav.getByRole("button", { name: w.newPost });
  await expect(newPost).not.toHaveAttribute("aria-haspopup");
  await userEvent.click(nav.getByRole("button", { name: w.newArticle }));
  await opened(canvas, w);
}

export const NavigationPC: Story = {
  globals: { locale: "en", ...pc },
  play: async (context) => navigationEntry(context, "en"),
};

export const NavigationJapanesePC: Story = {
  globals: { locale: "ja", ...pc },
  play: async (context) => navigationEntry(context, "ja"),
};

/** On a phone: New article starts the Menu sheet, and opens the editor. */
async function menuSheetEntry(context: StoryContext, lang: Lang) {
  const { canvas, w } = await home(context, lang);
  await expect(canvas.queryByRole("button", { name: w.newArticle })).toBeNull();
  await userEvent.click(
    within(canvas.getByRole("navigation", { name: w.navigation })).getByRole("button", { name: w.menu }),
  );
  const sheet = within(await within(document.body).findByRole("dialog", { name: w.menu }));
  const [first] = sheet.getAllByRole("link");
  await expect(first).toHaveAccessibleName(w.newArticle);
  await userEvent.click(sheet.getByRole("link", { name: w.newArticle }));
  await opened(canvas, w);
}

export const MenuSheetMinimumWidth: Story = {
  globals: { locale: "en", ...minimumWidth },
  play: async (context) => menuSheetEntry(context, "en"),
};

export const MenuSheetJapaneseMinimumWidth: Story = {
  globals: { locale: "ja", ...minimumWidth },
  play: async (context) => menuSheetEntry(context, "ja"),
};

/** On a phone: Alt+ArrowDown on the floating New post opens the menu of other things to write, whose New article opens the editor. */
async function postMenuEntry(context: StoryContext, lang: Lang) {
  const { canvas, w } = await home(context, lang);
  const newPost = canvas.getByRole("button", { name: w.newPost });
  newPost.focus();
  await userEvent.keyboard("{Alt>}{ArrowDown}{/Alt}");
  const menu = within(await within(document.body).findByRole("menu", { name: w.postMenu }));
  await expect(menu.getAllByRole("menuitem").map((item) => item.textContent)).toEqual([w.newArticle]);
  await userEvent.click(menu.getByRole("menuitem", { name: w.newArticle }));
  await opened(canvas, w);
}

export const PostMenuMinimumWidth: Story = {
  globals: { locale: "en", ...minimumWidth },
  play: async (context) => postMenuEntry(context, "en"),
};

export const PostMenuJapaneseMinimumWidth: Story = {
  globals: { locale: "ja", ...minimumWidth },
  play: async (context) => postMenuEntry(context, "ja"),
};

/** A reader the server keeps from writing new articles: no New article in the navigation. */
async function noNavigationEntry(context: StoryContext, lang: Lang) {
  const { canvas, w } = await home(context, lang);
  const nav = within(canvas.getByRole("navigation", { name: w.navigation }));
  await expect(nav.getByRole("button", { name: w.newPost })).toBeVisible();
  await expect(nav.queryByRole("button", { name: w.newArticle })).toBeNull();
}

export const NotAllowedPC: Story = {
  globals: { locale: "en", ...pc },
  parameters: permission,
  play: async (context) => noNavigationEntry(context, "en"),
};

export const NotAllowedJapanesePC: Story = {
  globals: { locale: "ja", ...pc },
  parameters: permission,
  play: async (context) => noNavigationEntry(context, "ja"),
};

/** A reader the server keeps from writing new articles, on a phone: no New article in the Menu sheet, no post menu. */
async function noPhoneEntry(context: StoryContext, lang: Lang) {
  const { canvas, w } = await home(context, lang);
  const newPost = canvas.getByRole("button", { name: w.newPost });
  await expect(newPost).not.toHaveAttribute("aria-haspopup");
  await userEvent.click(
    within(canvas.getByRole("navigation", { name: w.navigation })).getByRole("button", { name: w.menu }),
  );
  const dialog = await within(document.body).findByRole("dialog", { name: w.menu });
  await expect(within(dialog).queryByRole("link", { name: w.newArticle })).toBeNull();
  await risen(dialog);
}

/**
 * Waits until the sheet has risen into place (its panel opaque and unmoved), so that the story tests check it as the
 * reader sees it, not its colors mid-fade.
 */
async function risen(dialog: HTMLElement) {
  await waitFor(
    () => {
      const panel = dialog.parentElement;
      const style = panel === null ? undefined : getComputedStyle(panel);
      if (style?.opacity !== "1" || !["none", "matrix(1, 0, 0, 1, 0, 0)"].includes(style.transform))
        throw new Error("the sheet is still rising");
    },
    { timeout: 5000 },
  );
}

export const NotAllowedMinimumWidth: Story = {
  globals: { locale: "en", ...minimumWidth },
  parameters: permission,
  play: async (context) => noPhoneEntry(context, "en"),
};

export const NotAllowedJapaneseMinimumWidth: Story = {
  globals: { locale: "ja", ...minimumWidth },
  parameters: permission,
  play: async (context) => noPhoneEntry(context, "ja"),
};
