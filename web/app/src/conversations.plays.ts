// SPDX-License-Identifier: AGPL-3.0-only

// The plays of the Direct messages stories, in the story's locale: what the list shows, unread and read, with the
// count of the unread in the navigation (the rail's Direct messages from md up, the tab bar's Menu below); the list with
// none; a conversation hidden from it; and a conversation's posts oldest first with the reply in place, which opening
// marks read. The scenario's reader is Yuki: Sam's conversation ends with Sam's post (unread), the group's with Yuki's.

import { catalogReader } from "@my/design-system";
import type { StoryContext } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import { screenMeta } from "./app.samples";
import { messages, type MessageId } from "./messages.gen";

const stringsIn = catalogReader<MessageId>(messages);
type T = ReturnType<typeof stringsIn>["t"];

/** The display names of the scenario's people (web/e2e/src/scenario.ts), which no locale changes. */
const names: Record<string, string> = { yuki: "Yuki 雪", sam: "Sam Rivera", hana: "Hana 花", layla: "ليلى Layla" };
const name = (username: string) => names[username] ?? username;

/** The elements of `elements` that show at the story's width (the shell keeps a part per width, one showing). */
const shown = (elements: HTMLElement[]) => elements.filter((e) => e.getClientRects().length > 0);

/** What the story shows, once the screen settled, and the app's strings in its locale. */
async function settled(context: StoryContext) {
  await screenMeta.play(context);
  const { t } = stringsIn(String(context.globals["locale"] ?? "en"));
  const main = within(within(context.canvasElement).getByRole("main"));
  return { t, root: context.canvasElement, main };
}

/**
 * Waits until the navigation says `count` unread conversations (none for 0) at the story's width: on the place
 * Direct messages where it shows, or else on Menu, which holds it.
 */
async function navigationCounts(root: HTMLElement, t: T, count: number): Promise<void> {
  const canvas = within(root);
  const place = () =>
    shown(canvas.queryAllByRole("link", { name: (n: string) => n.startsWith(t("app.direct_messages")) })).filter(
      (e) => e.closest("main") === null,
    );
  const menu = () => shown(canvas.queryAllByRole("button", { name: (n: string) => n.startsWith(t("app.menu")) }));
  const inMenu = t("app.menu_direct", { count: 1 }).replace("1", "");
  await waitFor(async () => {
    const [link] = place();
    if (link !== undefined) {
      await expect(link).toHaveAccessibleName(
        count === 0
          ? t("app.direct_messages")
          : `${t("app.direct_messages")} ${t("app.direct_messages_count", { count })}`,
      );
      return;
    }
    const [button] = menu();
    if (button === undefined) throw new Error("the navigation shows neither Direct messages nor Menu");
    if (count === 0) await expect(button).not.toHaveAccessibleName(expect.stringContaining(inMenu));
    else await expect(button).toHaveAccessibleName(expect.stringContaining(t("app.menu_direct", { count })));
  });
}

/** Waits until the feed's posts are those of `people` (usernames), in order. */
async function postsBy(feed: HTMLElement, people: string[]): Promise<void> {
  await waitFor(async () => {
    const posts = within(feed).getAllByRole("article");
    await expect(posts).toHaveLength(people.length);
    for (const [i, post] of posts.entries()) await expect(post).toHaveAccessibleName(name(people[i] ?? ""));
  });
}

/** The list: the group's conversation, then Sam's; one unread, which the navigation counts. */
export async function listed(context: StoryContext): Promise<void> {
  const { t, root, main } = await settled(context);
  await expect(main.getByRole("heading", { level: 1, name: t("app.direct_messages") })).toBeVisible();
  const feed = await main.findByRole("feed");
  await waitFor(() => expect(within(feed).getAllByRole("article")).toHaveLength(2));
  await expect(within(feed).getByRole("article", { name: name("sam") })).toBeVisible();
  await navigationCounts(root, t, 1);
}

/** No conversation: none in the list, and no count in the navigation. */
export async function none(context: StoryContext): Promise<void> {
  const { t, root, main } = await settled(context);
  await expect(main.queryAllByRole("article")).toHaveLength(0);
  await navigationCounts(root, t, 0);
}

/** After the group's conversation was hidden: Sam's alone, and a line that names the group's people. */
export async function hiddenOne(context: StoryContext): Promise<void> {
  const { t, root, main } = await settled(context);
  await waitFor(() => expect(main.getAllByRole("article")).toHaveLength(1));
  await expect(main.getByRole("article")).toHaveAccessibleName(name("sam"));
  const said = main.getAllByRole("status").map((s) => s.textContent);
  await expect(said.some((s) => s.includes(name("hana")) && s.includes(name("layla")))).toBe(true);
  await navigationCounts(root, t, 1);
}

/** Sam's conversation: its posts oldest first, the reply in place, and the conversation read once open. */
export async function withSam(context: StoryContext): Promise<void> {
  const { t, root, main } = await settled(context);
  await expect(main.getByRole("heading", { level: 1, name: t("app.conversation") })).toBeVisible();
  await postsBy(await main.findByRole("feed"), ["sam", "yuki", "sam"]);
  await expect(main.getByRole("textbox")).toBeVisible();
  await navigationCounts(root, t, 0);
}

/** The group's conversation: Hana's, Layla's and Yuki's posts, oldest first, with the reply in place. */
export async function withGroup(context: StoryContext): Promise<void> {
  const { t, main } = await settled(context);
  await expect(main.getByRole("heading", { level: 1, name: t("app.conversation") })).toBeVisible();
  await postsBy(await main.findByRole("feed"), ["hana", "layla", "yuki"]);
  await expect(main.getByRole("textbox")).toBeVisible();
}
