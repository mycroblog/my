// SPDX-License-Identifier: AGPL-3.0-only

// The one menu as the reader uses it (#3129), played on the menus of "Menu everywhere" and on the Menu of the tab bar:
// at 320 px a sheet along the bottom edge, headed by the menu's name and Close; from `tablet` a popover by its trigger
// with no head outside a submenu. Close, Escape and a press on the backdrop close it and give the focus back to its
// trigger; a value is a menuitemradio, a toggle a menuitemcheckbox, each read as checked; a submenu takes the menu's
// place under Back and its title, which names it, and Back puts the menu back; a group that stays keeps it open; the
// Menu marks the place the reader is at.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { OpenActions, OpenSubmenu, OpenValues } from "./menu-surface.stories";
import { Frame, FrameDock, FrameMain, NavItem, NavList, NavMenu } from "./shell";

const words = {
  en: {
    more: "More",
    close: "Close",
    back: "Back",
    copyLink: "Copy link",
    addToList: "Add to list",
    visibility: "Who can see it",
    public: "Public",
    followers: "Followers only",
    photographers: "Photographers",
    friends: "Friends",
  },
  ja: {
    more: "その他",
    close: "閉じる",
    back: "戻る",
    copyLink: "リンクをコピー",
    addToList: "リストに追加",
    visibility: "見られる人",
    public: "公開",
    followers: "フォロワー限定",
    photographers: "写真家",
    friends: "友だち",
  },
};

type Lang = keyof typeof words;

const en = { locale: "en" };
const ja = { locale: "ja" };
const tablet = { viewport: { value: "tablet" } };

const meta = { title: "Menu states", parameters: { layout: "fullscreen" } } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** What a story of "Menu everywhere" renders, played here. */
function rendered(story: { render?: unknown }): NonNullable<Story["render"]> {
  const { render } = story;
  if (typeof render !== "function") throw new Error("the story renders nothing of its own");
  return render as NonNullable<Story["render"]>;
}

const actions = rendered(OpenActions);
const values = rendered(OpenValues);
const submenuOpen = rendered(OpenSubmenu);

/** The page's body, where a menu shows over the story, and the words of `lang`. */
function page(context: StoryContext, lang: Lang) {
  const doc = context.canvasElement.ownerDocument;
  return { doc, body: within(doc.body), w: words[lang] };
}

/** The button of the story that opens its menu. */
function trigger(context: StoryContext, name: string): HTMLElement {
  return within(context.canvasElement).getByRole("button", { name });
}

/** Waits until the open menu has risen into place (a sheet's; a popover does not move), as the reader sees it. */
async function risen(doc: Document) {
  await waitFor(
    () => {
      const menu = doc.querySelector('[role="menu"]');
      if (menu === null) throw new Error("no menu is open");
      for (let el: Element | null = menu; el !== null; el = el.parentElement) {
        const style = getComputedStyle(el);
        if (style.opacity !== "1" || !["none", "matrix(1, 0, 0, 1, 0, 0)"].includes(style.transform))
          throw new Error("the menu is still rising");
      }
    },
    { timeout: 5000 },
  );
}

/** Opens the menu by its trigger, and returns the menu once it has risen. */
async function open(context: StoryContext, button: string, menu: string): Promise<HTMLElement> {
  const { doc, body } = page(context, "en");
  await userEvent.click(trigger(context, button));
  const found = await body.findByRole("menu", { name: menu });
  await risen(doc);
  return found;
}

/**
 * The head of the open menu, by its title: a heading, and the controls beside it (Back before it in a submenu, Close
 * after it on a phone). React Aria's hidden Dismiss for screen readers is named Close too in ja, so Close is found here.
 */
function head(doc: Document, title: string) {
  const heading = within(doc.body).getByRole("heading", { name: title });
  if (heading.parentElement === null) throw new Error(`the heading ${title} stands alone`);
  return within(heading.parentElement);
}

/** Waits until no menu is open, and the focus is back on the trigger. */
async function closedOnto(context: StoryContext, button: string) {
  const { body } = page(context, "en");
  await waitFor(() => expect(body.queryByRole("menu")).toBeNull());
  await waitFor(() => expect(trigger(context, button)).toHaveFocus());
}

/**
 * At 320 px the menu is a sheet: as wide as the viewport and on its bottom edge, under the trigger at the top, headed
 * by its name and Close.
 */
async function sheet(context: StoryContext, lang: Lang) {
  const { doc, body, w } = page(context, lang);
  await userEvent.click(trigger(context, w.more));
  const menu = await body.findByRole("menu", { name: w.more });
  await risen(doc);
  const view = doc.documentElement;
  const box = menu.getBoundingClientRect();
  const button = trigger(context, w.more).getBoundingClientRect();
  await expect(box.top).toBeGreaterThan(button.bottom);
  await expect(box.width).toBeGreaterThan(view.clientWidth * 0.8);
  await expect(view.clientHeight - box.bottom).toBeLessThan(48);
  const named = head(doc, w.more);
  await expect(named.getByRole("button", { name: w.close })).toBeVisible();
  await expect(named.queryByRole("button", { name: w.back })).toBeNull();
}

export const SheetAtMinimumWidth: Story = {
  globals: en,
  render: actions,
  play: async (c) => sheet(c, "en"),
};
export const SheetAtMinimumWidthJapanese: Story = {
  globals: ja,
  render: actions,
  play: async (c) => sheet(c, "ja"),
};

/**
 * What is painted on top at the centre of each item of the open menu, by `elementFromPoint`. Hit testing passes over an
 * inert element (a backdrop that takes no press is one) however it is painted, so every element is hit for the probe
 * and inert again after it: the probe sees the paint order, as the reader's eye does.
 */
function onTop(doc: Document, items: HTMLElement[]): { item: HTMLElement; top: Element | null }[] {
  const inert = [...doc.querySelectorAll<HTMLElement>("[inert]")];
  for (const el of inert) el.inert = false;
  try {
    return items.map((item) => {
      const box = item.getBoundingClientRect();
      return { item, top: doc.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2) };
    });
  } finally {
    for (const el of inert) el.inert = true;
  }
}

/**
 * At 320 px the sheet is painted above its backdrop: what is on top at the centre of each item is the item itself (or
 * what it holds), not the dimmed page.
 */
export const SheetAboveBackdrop: Story = {
  globals: en,
  render: actions,
  play: async (context) => {
    const { doc, w } = page(context, "en");
    const menu = within(await open(context, w.more, w.more));
    for (const { item, top } of onTop(doc, menu.getAllByRole("menuitem")))
      await expect(
        top !== null && item.contains(top),
        `${item.textContent} is under ${top?.outerHTML.slice(0, 160) ?? "nothing"}`,
      ).toBe(true);
  },
};

/** From `tablet` the menu is a popover just under its trigger, narrower than the page, with no head and no Close. */
export const PopoverAtTablet: Story = {
  globals: { ...en, ...tablet },
  render: actions,
  play: async (context) => {
    const { doc, body, w } = page(context, "en");
    const menu = await open(context, w.more, w.more);
    const box = menu.getBoundingClientRect();
    const button = trigger(context, w.more).getBoundingClientRect();
    await expect(box.top).toBeGreaterThanOrEqual(button.bottom - 1);
    await expect(box.top - button.bottom).toBeLessThan(32);
    await expect(box.width).toBeLessThan(doc.documentElement.clientWidth / 2);
    await expect(body.queryByRole("heading", { name: w.more })).toBeNull();
    await expect(body.queryByRole("button", { name: w.close })).toBeNull();
    await expect(body.queryByRole("button", { name: w.back })).toBeNull();
  },
};

/** Close closes the sheet and gives the focus back to the trigger. */
export const CloseReturnsFocus: Story = {
  globals: en,
  render: actions,
  play: async (context) => {
    const { doc, w } = page(context, "en");
    await open(context, w.more, w.more);
    await userEvent.click(head(doc, w.more).getByRole("button", { name: w.close }));
    await closedOnto(context, w.more);
  },
};

/** Escape closes the sheet and gives the focus back to the trigger. */
export const EscapeReturnsFocus: Story = {
  globals: en,
  render: actions,
  play: async (context) => {
    const { w } = page(context, "en");
    await open(context, w.more, w.more);
    await userEvent.keyboard("{Escape}");
    await closedOnto(context, w.more);
  },
};

/**
 * A press on the dimmed page above the sheet, whatever takes it there (the top corner, away from the trigger and the
 * sheet), closes it and gives the focus back to the trigger.
 */
export const BackdropReturnsFocus: Story = {
  globals: en,
  render: actions,
  play: async (context) => {
    const { doc, w } = page(context, "en");
    const menu = await open(context, w.more, w.more);
    const backdrop = doc.elementFromPoint(8, 8);
    if (!(backdrop instanceof HTMLElement)) throw new Error("nothing is over the page's corner");
    await expect(menu.getBoundingClientRect().top).toBeGreaterThan(8);
    await expect(trigger(context, w.more).contains(backdrop)).toBe(false);
    await userEvent.click(backdrop);
    await closedOnto(context, w.more);
  },
};

/** Escape closes the popover from `tablet` too, and gives the focus back to the trigger. */
export const PopoverEscapeReturnsFocus: Story = {
  globals: { ...en, ...tablet },
  render: actions,
  play: async (context) => {
    const { w } = page(context, "en");
    await open(context, w.more, w.more);
    await userEvent.keyboard("{Escape}");
    await closedOnto(context, w.more);
  },
};

/** The values of one setting are menuitemradios, the chosen one read as checked; a choice closes the menu. */
export const ValuesAreRadios: Story = {
  globals: en,
  render: values,
  play: async (context) => {
    const { body, w } = page(context, "en");
    const menu = within(await open(context, w.visibility, w.visibility));
    await expect(menu.getByRole("menuitemradio", { name: w.public })).toHaveAttribute("aria-checked", "true");
    await expect(menu.getByRole("menuitemradio", { name: w.followers })).toHaveAttribute("aria-checked", "false");
    await expect(menu.queryAllByRole("menuitem")).toHaveLength(0);
    await userEvent.click(menu.getByRole("menuitemradio", { name: w.followers }));
    await closedOnto(context, w.visibility);
    const again = within(await open(context, w.visibility, w.visibility));
    await expect(again.getByRole("menuitemradio", { name: w.followers })).toHaveAttribute("aria-checked", "true");
    await expect(again.getByRole("menuitemradio", { name: w.public })).toHaveAttribute("aria-checked", "false");
    await expect(body.getAllByRole("menu")).toHaveLength(1);
  },
};

/**
 * A submenu's toggles are menuitemcheckboxes, each one on read as checked; in a group that stays, a press turns one
 * and the menu stays open. The submenu is named by its title, under Back.
 */
export const TogglesAreCheckboxesAndStay: Story = {
  globals: en,
  render: submenuOpen,
  play: async (context) => {
    const { doc, body, w } = page(context, "en");
    const menu = within(await open(context, w.more, w.addToList));
    await expect(menu.getByRole("menuitemcheckbox", { name: w.photographers })).toHaveAttribute("aria-checked", "true");
    await expect(menu.getByRole("menuitemcheckbox", { name: w.friends })).toHaveAttribute("aria-checked", "false");
    await userEvent.click(menu.getByRole("menuitemcheckbox", { name: w.friends }));
    await waitFor(() =>
      expect(menu.getByRole("menuitemcheckbox", { name: w.friends })).toHaveAttribute("aria-checked", "true"),
    );
    await expect(body.getByRole("menu", { name: w.addToList })).toBeVisible();
    await expect(head(doc, w.addToList).getByRole("button", { name: w.back })).toBeVisible();
  },
};

/**
 * An item that opens a submenu keeps the menu open and puts the submenu in its place, named by its title under Back;
 * Back puts the menu back, named by its own name again.
 */
async function submenu(context: StoryContext) {
  const { doc, body, w } = page(context, "en");
  const menu = within(await open(context, w.more, w.more));
  // The item reads what the submenu holds now (two lists) after its label; the one that cannot be done now does not.
  await userEvent.click(menu.getByRole("menuitem", { name: new RegExp(`^${w.addToList}\\s*2$`) }));
  const sub = await body.findByRole("menu", { name: w.addToList });
  await expect(within(sub).getAllByRole("menuitemcheckbox")).toHaveLength(4);
  await expect(body.queryByRole("menu", { name: w.more })).toBeNull();
  const named = head(doc, w.addToList);
  await expect(named.getByRole("button", { name: w.back })).toBeVisible();
  await userEvent.click(named.getByRole("button", { name: w.back }));
  const parent = within(await body.findByRole("menu", { name: w.more }));
  await expect(parent.getByRole("menuitem", { name: w.copyLink })).toBeVisible();
  await expect(body.queryByRole("menu", { name: w.addToList })).toBeNull();
}

export const SubmenuBackAtMinimumWidth: Story = { globals: en, render: actions, play: submenu };
export const SubmenuBackAtTablet: Story = { globals: { ...en, ...tablet }, render: actions, play: submenu };

/** The tab bar's Menu, its places link items, the one where the reader is marked as the current page. */
function TabMenu() {
  return (
    <Frame>
      <FrameMain>
        <p>Home</p>
      </FrameMain>
      <FrameDock
        label="Main"
        tabs={
          <NavList>
            <NavItem href="/" icon="home" label="Home" />
            <NavMenu label="Menu">
              <NavList>
                <NavItem href="/bookmarks" icon="bookmark" label="Bookmarks" current />
                <NavItem href="/lists" icon="list" label="Lists" />
              </NavList>
            </NavMenu>
          </NavList>
        }
      />
    </Frame>
  );
}

/**
 * The press that opens the tab bar's Menu does not run the item the sheet raises under it: a mouse press starts on
 * Menu, the sheet opens, and the press ends over its first place, which is not followed and leaves the sheet open.
 */
export const OpeningPressRunsNoItem: Story = {
  globals: en,
  render: () => <TabMenu />,
  play: async (context) => {
    const { doc, body } = page(context, "en");
    const button = within(context.canvasElement).getByRole("button", { name: "Menu" });
    let followed = false;
    const follow = (event: Event) => {
      if (event.target instanceof Element && event.target.closest("a") !== null) {
        followed = true;
        event.preventDefault();
      }
    };
    doc.addEventListener("click", follow, true);
    try {
      await userEvent.pointer({ keys: "[MouseLeft>]", target: button });
      const menu = within(await body.findByRole("menu", { name: "Menu" }));
      await risen(doc);
      await userEvent.pointer({ keys: "[/MouseLeft]", target: menu.getByRole("menuitem", { name: "Bookmarks" }) });
      await expect(body.getByRole("menu", { name: "Menu" })).toBeVisible();
      await expect(followed).toBe(false);
    } finally {
      doc.removeEventListener("click", follow, true);
    }
  },
};

/** The Menu's places are items that lead there, and the one where the reader is says it is the current page. */
export const NavMenuCurrentPage: Story = {
  globals: en,
  render: () => <TabMenu />,
  play: async (context) => {
    const { doc, body } = page(context, "en");
    await userEvent.click(within(context.canvasElement).getByRole("button", { name: "Menu" }));
    const menu = within(await body.findByRole("menu", { name: "Menu" }));
    await risen(doc);
    const here = menu.getByRole("menuitem", { name: "Bookmarks" });
    const there = menu.getByRole("menuitem", { name: "Lists" });
    await expect(here).toHaveAttribute("aria-current", "page");
    await expect(here).toHaveAttribute("href", "/bookmarks");
    await expect(there).not.toHaveAttribute("aria-current");
    await expect(there).toHaveAttribute("href", "/lists");
  },
};
