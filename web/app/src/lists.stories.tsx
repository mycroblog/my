// SPDX-License-Identifier: AGPL-3.0-only

// The reader's lists, which the menu leads to, a list's timeline and its members: a story per state every screen is
// shown in (app.samples.tsx), of each; a list deleted; and managing them (#3124): a list made in place and refused at
// the limit of lists, its name in its field, its deletion asked, an account added to its members and refused at the
// limit of members, and Add to list on a profile, for an account the reader follows (and a list checked there) and one they do
// not. What a press alone reaches (a refusal, a field in the title's place, a menu, a confirmation) a play presses, in
// the story's locale, and asserts nothing: the features' tests hold what each state shows.

import { featureStringsIn } from "@my/app-features";
import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { userEvent, waitFor, within } from "storybook/test";
import { screen, screenMeta } from "./app.samples";

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
export const ListTimeline: Story = { parameters: screen({ at: list }) };
export const ListEmpty: Story = { parameters: screen({ at: list, state: "empty" }) };
export const ListLoading: Story = { parameters: screen({ at: list, state: "loading" }) };
export const ListFailed: Story = { parameters: screen({ at: list, state: "error" }) };
export const ListGone: Story = { parameters: screen({ at: "/lists/gone" }) };
export const ListLongText: Story = { parameters: screen({ at: "list:2", state: "long" }) };
/** The list whose name runs long, with its own posts. */
export const ListLongName: Story = { parameters: screen({ at: "list:2" }) };

// ---- Managing the lists (ADR-0013 §11–§15) -------------------------------------------------------------------------

const create = "/lists?create=1";

/** The screen once it shows its state: its main region, the page (a menu, a dialog), the features' words. */
async function screenOf(context: StoryContext) {
  await screenMeta.play(context);
  const root = context.canvasElement;
  const { t } = featureStringsIn(String(context.globals["locale"] ?? "en"));
  return { t, main: within(root.querySelector("main") ?? root), page: within(root.ownerDocument.body) };
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

/** The index opened to make a list: the name's field in place at its top, New list gone while it is open. */
export const Creating: Story = { parameters: screen({ at: create }) };

/** A new list refused at the server's limit (here 3 lists): said under the name, Create dimmed. */
export const CreateLimit: Story = {
  parameters: screen({ at: create, server: { listLimits: { lists: 3 } } }),
  play: async (context) => {
    const { t, main } = await screenOf(context);
    const name = await main.findByRole("textbox", { name: t("timeline.list_name") });
    await userEvent.type(name, "Friends");
    await userEvent.click(main.getByRole("button", { name: t("timeline.list_create") }));
    await waitFor(() => {
      if (name.getAttribute("aria-invalid") !== "true") throw new Error("the server has not refused the list yet");
    });
  },
};

/** Opens the list's menu, List options, and chooses `item`. */
async function fromMenu(context: StoryContext, item: "timeline.list_rename" | "timeline.list_delete") {
  const { t, main, page } = await screenOf(context);
  await userEvent.click(await main.findByRole("button", { name: t("timeline.list_more") }));
  const menu = await page.findByRole("menu", { name: t("timeline.list_more") });
  await userEvent.click(within(menu).getByRole("menuitem", { name: t(item) }));
  return { t, main, page };
}

/** Rename chosen from the list's menu: the name in place of the title, under the back button. */
export const Renaming: Story = {
  parameters: screen({ at: list }),
  play: async (context) => {
    const { t, main } = await fromMenu(context, "timeline.list_rename");
    await main.findByRole("textbox", { name: t("timeline.list_name") });
  },
};

/** Delete list… chosen from the list's menu: its confirmation over the page. */
export const DeleteConfirm: Story = {
  parameters: screen({ at: list }),
  play: async (context) => {
    const { page } = await fromMenu(context, "timeline.list_delete");
    await risen(await page.findByRole("alertdialog"));
  },
};

/** The members of the first list, each with Remove, under the field that adds. */
export const Members: Story = { parameters: screen({ at: "members:0" }) };
export const MembersEmpty: Story = { parameters: screen({ at: "members:0", state: "empty" }) };
export const MembersLoading: Story = { parameters: screen({ at: "members:0", state: "loading" }) };
export const MembersFailed: Story = { parameters: screen({ at: "members:0", state: "error" }) };
/** The members of the list whose name runs long. */
export const MembersLongName: Story = { parameters: screen({ at: "members:2" }) };

/** Finds `q` among the accounts the reader follows, and presses Add on the first found: what was found. */
async function adding(context: StoryContext, q: string) {
  const { t, main } = await screenOf(context);
  await userEvent.type(await main.findByRole("searchbox", { name: t("timeline.list_add") }), q);
  const found = within(await main.findByRole("group", { name: t("timeline.list_add") }));
  const add = await found.findByRole("button", { name: t("timeline.list_add_button") });
  await userEvent.click(add);
  return { found, add };
}

/** Layla found and added: at the top of the members, in the count, and Added where she was found. */
export const MembersAdding: Story = {
  parameters: screen({ at: "members:0" }),
  play: async (context) => {
    const { found } = await adding(context, "lay");
    await waitFor(() => {
      if (found.queryAllByRole("button").length > 0) throw new Error("the account is not added yet");
    });
  },
};

/** Adding refused at the server's limit (here 2 members): said under the field, Add dimmed. */
export const MembersLimit: Story = {
  parameters: screen({ at: "members:0", server: { listLimits: { members: 2 } } }),
  play: async (context) => {
    const { add } = await adding(context, "lay");
    await waitFor(() => {
      if (!add.hasAttribute("disabled")) throw new Error("the server has not refused the account yet");
    });
  },
};

/** Opens the profile's menu (the first More of the screen, before any post's), and Add to list when `into`. */
async function profileMenu(context: StoryContext, into: boolean) {
  const { t, main, page } = await screenOf(context);
  const [more] = await main.findAllByRole("button", { name: t("profile.more") });
  if (more === undefined) throw new Error("the profile offers no menu");
  await userEvent.click(more);
  const menu = await page.findByRole("menu", { name: t("profile.more") });
  if (!into) return menu;
  const add = t("profile.add_to_list");
  await userEvent.click(within(menu).getByRole("menuitem", { name: (name: string) => name.startsWith(add) }));
  return page.findByRole("menu", { name: add });
}

/** Add to list on the profile of someone the reader follows: the lists, those holding sam checked, then New list. */
export const AddToListFollowed: Story = {
  parameters: screen({ at: "/@sam" }),
  play: async (context) => {
    const menu = await profileMenu(context, true);
    await within(menu).findAllByRole("menuitemcheckbox", { checked: true });
    await risen(menu);
  },
};

/** A list that did not hold sam checked in Add to list: it holds sam at once, the submenu still open. */
export const AddToListAdding: Story = {
  parameters: screen({ at: "/@sam" }),
  play: async (context) => {
    const menu = await profileMenu(context, true);
    await within(menu).findAllByRole("menuitemcheckbox", { checked: true });
    const [other] = within(menu).getAllByRole("menuitemcheckbox", { checked: false });
    if (other === undefined) throw new Error("every list holds the account");
    await userEvent.click(other);
    await waitFor(() => {
      if (other.getAttribute("aria-checked") !== "true") throw new Error("the list does not hold the account yet");
    });
    await risen(menu);
  },
};

/** The profile's menu of someone the reader does not follow: Add to list muted, saying why. */
export const AddToListNotFollowed: Story = {
  parameters: screen({ at: "/@marco" }),
  play: async (context) => {
    await risen(await profileMenu(context, false));
  },
};
