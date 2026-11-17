// SPDX-License-Identifier: AGPL-3.0-only

// The plays of the Profile stories that press something before the story shows its state, in the story's locale: the
// note's editor opened, the profile's menu open, a confirmation of it open, a post's menu open on another's post, and that post
// once the reader muted or blocked its author or muted its server from it. They find what to press by the features'
// words, in the locale the story renders, and assert nothing: what each state shows is held by the features' tests.

import { featureStringsIn, type FeatureMessageId as MessageId } from "@my/app-features";
import type { StoryContext } from "@storybook/react-vite";
import { screen, userEvent, waitFor, within } from "storybook/test";
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
  await main.findByRole("textbox", { name: t("profile.note") });
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

/**
 * The profile's menu left open (a sheet on a phone, a popover from a tablet), once it has risen into place: no box
 * around it fading in or moving.
 */
export async function menuOpen(context: StoryContext): Promise<void> {
  const { t, main } = await settled(context);
  const [more] = await main.findAllByRole("button", { name: t("profile.more") });
  if (more === undefined) throw new Error("the profile offers no menu");
  await userEvent.click(more);
  await risen(await screen.findByRole("menu", { name: t("profile.more") }));
}

/** Waits until no box around the menu is fading in or moving (a sheet rising on a phone), as it will stay. */
async function risen(menu: HTMLElement): Promise<void> {
  await waitFor(() => {
    for (let e: HTMLElement | null = menu; e !== null; e = e.parentElement) {
      const style = getComputedStyle(e);
      if (Number(style.opacity) < 1 || !["none", "matrix(1, 0, 0, 1, 0, 0)"].includes(style.transform))
        throw new Error("the menu is still coming in");
    }
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
  await postMenu(context);
  await risen(await screen.findByRole("menu"));
}

/** The post of another server once the reader did `item` from its menu: a line in its place, with its undoing. */
async function done(context: StoryContext, item: MessageId, undo: MessageId): Promise<void> {
  const { t, main } = await postMenu(context);
  await userEvent.click(await screen.findByRole("menuitem", { name: t(item) }));
  if (item === "profile.block")
    await userEvent.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: t("profile.block_confirm") }),
    );
  await main.findByRole("button", { name: t(undo) });
  // The confirmation and the menu have left, so that the story shows the line alone.
  await waitFor(() => {
    if (screen.queryByRole("alertdialog") !== null || screen.queryByRole("menu") !== null)
      throw new Error("the confirmation or the menu is still open");
  });
}

/** The post once the reader muted its author from it. */
export const mutedFromPost = (context: StoryContext) => done(context, "profile.mute", "profile.undo");

/** The post once the reader blocked its author from it, after the confirmation. */
export const blockedFromPost = (context: StoryContext) => done(context, "profile.block", "profile.unblock");

/** The post once the reader muted its author's server from it. */
export const serverMutedFromPost = (context: StoryContext) => done(context, "profile.mute_server", "profile.undo");
