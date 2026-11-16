// SPDX-License-Identifier: AGPL-3.0-only

// The plays of the Profile stories that press something before the story shows its state, in the story's locale: the
// note's editor opened, a confirmation of the profile's menu open, a post's menu open on another's post, and that post
// once the reader muted or blocked its author or muted its server from it. They find what to press by the features'
// words, in the locale the story renders.

import { featureStringsIn, type FeatureMessageId as MessageId } from "@my/app-features";
import type { StoryContext } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { screenMeta } from "./app.samples";

/** What the story shows once the screen settled, and the features' strings in its locale. */
async function settled(context: StoryContext) {
  await screenMeta.play(context);
  const { t } = featureStringsIn(String(context.globals["locale"] ?? "en"));
  return { t, main: within(within(context.canvasElement).getByRole("main")) };
}

/** Opens the editor of the reader's note on the profile, by pressing its line. */
export async function noteEditing(context: StoryContext): Promise<void> {
  const { t, main } = await settled(context);
  await userEvent.click(await main.findByRole("button", { name: (n: string) => n.startsWith(t("profile.note_edit")) }));
  await expect(await main.findByRole("textbox", { name: t("profile.note") })).toBeVisible();
}

/** Opens the profile's menu, and asks before the action of `item`, whose confirmation then shows. */
async function confirming(context: StoryContext, item: MessageId): Promise<void> {
  const { t, main } = await settled(context);
  // The profile's more, before those of its posts.
  const [more] = await main.findAllByRole("button", { name: t("profile.more") });
  if (more === undefined) throw new Error("the profile offers no menu");
  await userEvent.click(more);
  await userEvent.click(await screen.findByRole("menuitem", { name: t(item) }));
  await still(await screen.findByRole("alertdialog"));
}

/** Waits until `element` has come in: no box around it is still fading in, so that it reads as it will stay. */
async function still(element: HTMLElement): Promise<void> {
  await waitFor(() => {
    for (let e: HTMLElement | null = element; e !== null; e = e.parentElement)
      if (Number(getComputedStyle(e).opacity) < 1) throw new Error("the dialog is still coming in");
  });
}

/** The confirmation of blocking the account. */
export const blockConfirm = (context: StoryContext) => confirming(context, "profile.block");

/** The confirmation of removing the account from the reader's followers. */
export const removeFollowerConfirm = (context: StoryContext) => confirming(context, "profile.remove_follower");

/** The text of the post of another server in the home timeline, which sam boosted. */
const remotePost = "Developed the first roll from the coast.";

/** Opens the more menu of the post of another server in the home timeline. */
async function postMenu(context: StoryContext) {
  const { t, main } = await settled(context);
  const article = await waitFor(() => {
    const found = main.getAllByRole("article").find((a) => a.textContent.includes(remotePost));
    if (found === undefined) throw new Error("the home timeline shows no post of another server");
    return found;
  });
  // The more menu is the last of the post's menus, after boost's.
  const more = [...article.querySelectorAll<HTMLElement>('button[aria-haspopup="true"]')].at(-1);
  if (more === undefined) throw new Error("the post offers no menu");
  await userEvent.click(more);
  return { t, main };
}

/** The menu of another's post, with its author's actions and its server's. */
export async function postMenuOpen(context: StoryContext): Promise<void> {
  const { t } = await postMenu(context);
  await expect(await screen.findByRole("menuitem", { name: t("profile.mute_server") })).toBeVisible();
}

/** The post of another server once the reader did `item` from its menu: a line in its place, with its undoing. */
async function done(context: StoryContext, item: MessageId, undo: MessageId): Promise<void> {
  const { t, main } = await postMenu(context);
  await userEvent.click(await screen.findByRole("menuitem", { name: t(item) }));
  if (item === "profile.block")
    await userEvent.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: t("profile.block_confirm") }),
    );
  await expect(await main.findByRole("button", { name: t(undo) })).toBeVisible();
  // The confirmation and the menu have left, so that the story shows the line alone.
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
}

/** The post once the reader muted its author from it. */
export const mutedFromPost = (context: StoryContext) => done(context, "profile.mute", "profile.undo");

/** The post once the reader blocked its author from it, after the confirmation. */
export const blockedFromPost = (context: StoryContext) => done(context, "profile.block", "profile.unblock");

/** The post once the reader muted its author's server from it. */
export const serverMutedFromPost = (context: StoryContext) => done(context, "profile.mute_server", "profile.undo");

// The plays that check a state as the profile opened, pressing nothing, and the home timeline once the reader muted a
// server from a post and came back to it.

/** The text a reader reads, without the marks that isolate a name's direction (U+2068, U+2069). */
const plain = (s: string | null) => (s ?? "").replaceAll(/[⁨⁩]/gu, "").trim();

/** The profile's card: the region of the account. */
async function card(context: StoryContext) {
  const { t, main } = await settled(context);
  const [region] = main.getAllByRole("region");
  if (region === undefined) throw new Error("the profile shows no card");
  return { t, main, card: within(region) };
}

/** The bell of an account the reader follows: off, or on once they asked to be notified of its posts. */
async function bell(context: StoryContext, on: boolean): Promise<void> {
  const { t, card: c } = await card(context);
  await expect(c.getByRole("button", { name: t("profile.notify") })).toHaveAttribute("aria-pressed", String(on));
}

export const notifyOff = (context: StoryContext) => bell(context, false);
export const notifyOn = (context: StoryContext) => bell(context, true);

/** An account the reader does not follow offers no bell. */
export async function noBell(context: StoryContext): Promise<void> {
  const { t, card: c } = await card(context);
  await expect(c.queryByRole("button", { name: t("profile.notify") })).toBeNull();
}

/** The profile of an account whose server the reader mutes says so, with the undoing. */
export async function serverMuted(context: StoryContext): Promise<void> {
  const { t, card: c } = await card(context);
  const line = t("profile.server_muted", { domain: "photos.example" });
  const button = c.getByRole("button", { name: t("profile.unmute_server") });
  await expect(plain(button.closest("p")?.textContent ?? "")).toContain(line);
}

/** The reader's note shows on the profile, as the button that edits it. */
export async function noteWritten(context: StoryContext): Promise<void> {
  const { t, card: c } = await card(context);
  const note = c.getByRole("button", { name: (n: string) => n.startsWith(t("profile.note_edit")) });
  await expect(plain(note.textContent)).toContain("Ask about the rye starter.");
}

/**
 * The home timeline once the reader muted the server of a post sam boosted from its menu (ADR-0013 §30): the line in
 * the post's place says so; once the reader left home and came back, the boost has left it, and sam's own posts stay.
 */
export async function homeAfterServerMute(context: StoryContext): Promise<void> {
  await done(context, "profile.mute_server", "profile.undo");
  const { t } = featureStringsIn(String(context.globals["locale"] ?? "en"));
  const canvas = within(context.canvasElement);
  const main = () => within(canvas.getByRole("main"));
  const line = t("profile.server_muted_now", { domain: "photos.example" });
  await expect(plain(canvas.getByRole("main").textContent)).toContain(line);
  const heading = () => plain(main().getByRole("heading", { level: 1 }).textContent);
  const home = heading();
  const to = async (href: string) => {
    const link = canvas.getAllByRole("link").find((a) => a.getAttribute("href") === href && a.closest("main") === null);
    if (link === undefined) throw new Error(`the navigation offers no link to ${href}`);
    await userEvent.click(link);
  };
  await to("/notifications");
  await waitFor(() => expect(heading()).not.toBe(home));
  await to("/");
  await waitFor(
    async () => {
      const articles = main().getAllByRole("article");
      await expect(articles.some((a) => a.textContent.includes("Sam Rivera"))).toBe(true);
      await expect(articles.some((a) => a.textContent.includes(remotePost))).toBe(false);
    },
    { timeout: 5000 },
  );
}
