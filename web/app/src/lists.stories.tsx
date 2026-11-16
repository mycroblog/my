// SPDX-License-Identifier: AGPL-3.0-only

// The reader's lists, which the menu leads to, a list's timeline and its members: a story per state every screen is
// shown in (app.samples.tsx), of each; a list deleted; the minimum width and right to left; and managing them (#3124):
// a list made in place and refused at the limit of lists, renamed, its deletion confirmed, an account added to its
// members and refused at the limit of members, a member removed, and Add to list on a profile, for an account the
// reader follows (and a list checked there) and one they do not.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

const index = "/lists";
const list = "list:0";

const meta = {
  title: "Lists",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: index }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Index: Story = {};
export const IndexEmpty: Story = { parameters: screen({ at: index, state: "empty" }) };
export const IndexLoading: Story = { parameters: screen({ at: index, state: "loading" }) };
export const IndexFailed: Story = { parameters: screen({ at: index, state: "error" }) };
export const IndexMinimumWidth: Story = { globals: minimumWidth };
export const ListTimeline: Story = { parameters: screen({ at: list }) };
export const ListEmpty: Story = { parameters: screen({ at: list, state: "empty" }) };
export const ListLoading: Story = { parameters: screen({ at: list, state: "loading" }) };
export const ListFailed: Story = { parameters: screen({ at: list, state: "error" }) };
export const ListGone: Story = { parameters: screen({ at: "/lists/gone" }) };
export const ListLongText: Story = { parameters: screen({ at: "list:2", state: "long" }) };
export const ListRightToLeft: Story = { parameters: screen({ at: list }), globals: rightToLeft };
export const ListMinimumWidth: Story = { parameters: screen({ at: "list:2" }), globals: minimumWidth };

// ---- Managing the lists (ADR-0013 §11–§15), the plays in English --------------------------------------------------

const en = { locale: "en" } as const;

/** The queries of the screen's main region and of the page (a menu, a dialog). */
async function screenOf(context: StoryContext) {
  await screenMeta.play(context);
  const root = context.canvasElement;
  return { main: within(root.querySelector("main") ?? root), page: within(root.ownerDocument.body) };
}

/** Waits until every box around `el` is opaque and unmoved: a sheet or a dialog risen into place. */
async function risen(el: Element) {
  await waitFor(() => {
    for (let at: Element | null = el; at !== null; at = at.parentElement) {
      const style = getComputedStyle(at);
      if (style.opacity !== "1" || !["none", "matrix(1, 0, 0, 1, 0, 0)"].includes(style.transform))
        throw new Error("still rising");
    }
  });
}

/** New list pressed: the name's field in place at the index's top, New list gone while it is open. */
export const Creating: Story = {
  globals: en,
  play: async (context) => {
    const { main } = await screenOf(context);
    await userEvent.click(await main.findByRole("button", { name: "New list" }));
    await expect(await main.findByRole("textbox", { name: "List name" })).toHaveFocus();
    await expect(main.queryByRole("button", { name: "New list" })).toBeNull();
  },
};

/** A new list refused at the server's limit (here 3 lists): said under the name, Create dimmed. */
export const CreateLimit: Story = {
  globals: en,
  parameters: screen({ at: index, server: { listLimits: { lists: 3 } } }),
  play: async (context) => {
    const { main } = await screenOf(context);
    await userEvent.click(await main.findByRole("button", { name: "New list" }));
    await userEvent.type(await main.findByRole("textbox", { name: "List name" }), "Friends");
    await userEvent.click(main.getByRole("button", { name: "Create" }));
    await expect(await main.findByText(/at most 50 lists/)).toBeVisible();
    await expect(main.getByRole("button", { name: "Create" })).toBeDisabled();
  },
};

/** The first More of the screen: its title's (a list's, a profile's), before any post's. */
const firstMore = async (main: Awaited<ReturnType<typeof screenOf>>["main"]) => {
  const [more] = await main.findAllByRole("button", { name: "More" });
  if (more === undefined) throw new Error("no More");
  return more;
};

/** Opens the list's menu and chooses `item`. */
async function fromMenu(context: StoryContext, item: string) {
  const { main, page } = await screenOf(context);
  await userEvent.click(await firstMore(main));
  const menu = await page.findByRole("menu", { name: "More" });
  await userEvent.click(within(menu).getByRole("menuitem", { name: item }));
  return { main, page };
}

/** Rename chosen from the list's menu: the name in place of the title, under the back button. */
export const Renaming: Story = {
  globals: en,
  parameters: screen({ at: list }),
  play: async (context) => {
    const { main } = await fromMenu(context, "Rename");
    await expect(await main.findByRole("textbox", { name: "List name" })).toHaveValue("Bread and coffee");
  },
};

/** A new name saved: the list's title says it, the field gone. */
export const Renamed: Story = {
  globals: en,
  parameters: screen({ at: list }),
  play: async (context) => {
    const { main } = await fromMenu(context, "Rename");
    const field = await main.findByRole("textbox", { name: "List name" });
    await userEvent.clear(field);
    await userEvent.type(field, "Bakers");
    await userEvent.click(main.getByRole("button", { name: "Save" }));
    await expect(await main.findByRole("heading", { level: 1, name: "Bakers" })).toBeVisible();
    await waitFor(() => expect(main.queryByRole("textbox", { name: "List name" })).toBeNull());
  },
};

/** The deletion confirmed: back on the index, which no longer holds the list. */
export const Deleted: Story = {
  globals: en,
  parameters: screen({ at: list }),
  play: async (context) => {
    const { main, page } = await fromMenu(context, "Delete list…");
    const confirm = await page.findByRole("alertdialog", { name: "Delete the list Bread and coffee?" });
    await userEvent.click(within(confirm).getByRole("button", { name: "Delete" }));
    await expect(await main.findByRole("heading", { level: 1, name: "Lists" })).toBeVisible();
    const index = await main.findByRole("navigation", { name: "Your lists" });
    await expect(within(index).getByRole("link", { name: /^翻訳の仲間 \d+ members?$/ })).toBeVisible();
    await expect(within(index).queryByRole("link", { name: /^Bread and coffee \d+ members?$/ })).toBeNull();
  },
};

/** Delete list… chosen from the list's menu: its confirmation over the page. */
export const DeleteConfirm: Story = {
  globals: en,
  parameters: screen({ at: list }),
  play: async (context) => {
    const { page } = await fromMenu(context, "Delete list…");
    await risen(await page.findByRole("alertdialog", { name: "Delete the list Bread and coffee?" }));
  },
};

/** The members of the first list, each with Remove, under the field that adds. */
export const Members: Story = { globals: en, parameters: screen({ at: "members:0" }) };
export const MembersEmpty: Story = { parameters: screen({ at: "members:0", state: "empty" }) };
export const MembersLoading: Story = { parameters: screen({ at: "members:0", state: "loading" }) };
export const MembersFailed: Story = { parameters: screen({ at: "members:0", state: "error" }) };
export const MembersMinimumWidth: Story = { parameters: screen({ at: "members:2" }), globals: minimumWidth };

/** Finds `q` among the accounts the reader follows, and adds the first found. */
async function adding(context: StoryContext, q: string) {
  const { main } = await screenOf(context);
  await userEvent.type(await main.findByRole("textbox", { name: "Add an account you follow" }), q);
  const found = await main.findByRole("group", { name: "Add an account you follow" });
  await userEvent.click(await within(found).findByRole("button", { name: "Add" }));
  return { main, found };
}

/** Layla found and added: at the top of the members, in the count, and Added where she was found. */
export const MembersAdding: Story = {
  globals: en,
  parameters: screen({ at: "members:0" }),
  play: async (context) => {
    const { main, found } = await adding(context, "lay");
    await expect(await within(found).findByText("Added")).toBeVisible();
    await expect(main.getByText("3 members")).toBeVisible();
  },
};

/** Adding refused at the server's limit (here 2 members): said under the field, Add dimmed. */
export const MembersLimit: Story = {
  globals: en,
  parameters: screen({ at: "members:0", server: { listLimits: { members: 2 } } }),
  play: async (context) => {
    const { main, found } = await adding(context, "lay");
    await expect(await main.findByText(/at most 1,000 members/)).toBeVisible();
    await expect(within(found).getByRole("button", { name: "Add" })).toBeDisabled();
  },
};

/** Sam removed from the first list's members: gone from them, and the count down to one. */
export const MembersRemoving: Story = {
  globals: en,
  parameters: screen({ at: "members:0" }),
  play: async (context) => {
    const { main } = await screenOf(context);
    const sam = await main.findByRole("link", { name: /Sam Rivera/ });
    // Each member's Remove is named Remove alone: Sam's is the one whose row, the nearest box around it with a link,
    // holds Sam's.
    const remove = main.getAllByRole("button", { name: "Remove" }).find((button) => {
      let row = button.parentElement;
      while (row !== null && row.querySelector("a") === null) row = row.parentElement;
      return row?.contains(sam) === true;
    });
    if (remove === undefined) throw new Error("no Remove beside Sam");
    await userEvent.click(remove);
    await waitFor(() => expect(main.queryByRole("link", { name: /Sam Rivera/ })).toBeNull());
    await expect(main.getByText("1 member")).toBeVisible();
    await expect(main.getByRole("link", { name: /Hana 花/ })).toBeVisible();
  },
};

/** Opens the profile's menu, and Add to list when `into`. */
async function profileMenu(context: StoryContext, into: boolean) {
  const { main, page } = await screenOf(context);
  await userEvent.click(await firstMore(main));
  const menu = await page.findByRole("menu", { name: "More" });
  if (!into) return { menu, page };
  await userEvent.click(within(menu).getByRole("menuitem", { name: /^Add to list/ }));
  return { menu: await page.findByRole("menu", { name: "Add to list" }), page };
}

/** Add to list on the profile of someone the reader follows: the lists, those holding sam checked, then New list. */
export const AddToListFollowed: Story = {
  globals: en,
  parameters: screen({ at: "/@sam" }),
  play: async (context) => {
    const { menu } = await profileMenu(context, true);
    await waitFor(() =>
      expect(within(menu).getByRole("menuitemcheckbox", { name: "Bread and coffee" })).toHaveAttribute(
        "aria-checked",
        "true",
      ),
    );
    await risen(menu);
  },
};

/** A list that did not hold sam checked in Add to list: it holds sam at once, the submenu still open. */
export const AddToListAdding: Story = {
  globals: en,
  parameters: screen({ at: "/@sam" }),
  play: async (context) => {
    const { menu } = await profileMenu(context, true);
    const translators = await within(menu).findByRole("menuitemcheckbox", { name: "翻訳の仲間" });
    await expect(translators).toHaveAttribute("aria-checked", "false");
    await userEvent.click(translators);
    await waitFor(() => expect(translators).toHaveAttribute("aria-checked", "true"));
    await expect(within(menu).getByRole("menuitemcheckbox", { name: "Bread and coffee" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await risen(menu);
  },
};

/** The profile's menu of someone the reader does not follow: Add to list muted, saying why. */
export const AddToListNotFollowed: Story = {
  globals: en,
  parameters: screen({ at: "/@marco" }),
  play: async (context) => {
    const { menu } = await profileMenu(context, false);
    await expect(within(menu).getByRole("menuitem", { name: /^Add to list/ })).toHaveAttribute("aria-disabled", "true");
    await risen(menu);
  },
};
