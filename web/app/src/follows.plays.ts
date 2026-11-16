// SPDX-License-Identifier: AGPL-3.0-only

// The plays of the Follows stories, which check what each state shows once the screen settled, in the story's locale:
// the list's tab chosen, a row per account with its follow, the owner's refusal in place of a hidden list, the failure
// with its retry, and the line of an empty list. They press nothing, so that each story shows its state as it opened.

import { featureStringsIn } from "@my/app-features";
import type { StoryContext } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { screenMeta } from "./app.samples";

/** The text a reader reads, without the marks that isolate a name's direction (U+2068, U+2069). */
const plain = (s: string | null) => (s ?? "").replaceAll(/[⁨⁩]/gu, "").trim();

/** What the story shows once the screen settled, and the features' strings in its locale. */
async function settled(context: StoryContext) {
  await screenMeta.play(context);
  const { t } = featureStringsIn(String(context.globals["locale"] ?? "en"));
  return { t, main: within(within(context.canvasElement).getByRole("main")) };
}

/** The tab of `list` is the one chosen of the two, and the open list's panel. */
async function chosen(context: StoryContext, list: "following" | "followers") {
  const { t, main } = await settled(context);
  const tabs = within(main.getByRole("tablist", { name: t("profile.follows_tabs") })).getAllByRole("tab");
  await expect(tabs).toHaveLength(2);
  const label = t(list === "following" ? "profile.following_count" : "profile.followers_count");
  const selected = tabs.filter((tab) => tab.getAttribute("aria-selected") === "true");
  await expect(selected.map((tab) => plain(tab.textContent).startsWith(label))).toEqual([true]);
  return { t, panel: within(main.getByRole("tabpanel")) };
}

/**
 * sam's followers as yuki reads them: a row per follower, each a link to its profile, with Follow for marco alone, whom
 * yuki does not follow: none for yuki, and none for those yuki follows.
 */
export async function followers(context: StoryContext): Promise<void> {
  const { t, panel } = await chosen(context, "followers");
  const profiles = panel.getAllByRole("link").map((a) => a.getAttribute("href"));
  await expect(profiles).toEqual(["/@marco", "/@layla", "/@yuki", "/@hana"]);
  const follows = panel.getAllByRole("button", { name: (n: string) => n === t("profile.follow") });
  await expect(follows).toHaveLength(1);
}

/** sam's follows, on the tab of whom sam follows. */
export async function following(context: StoryContext): Promise<void> {
  const { panel } = await chosen(context, "following");
  await expect(panel.getAllByRole("link").length).toBeGreaterThan(0);
}

/** A list the owner keeps from yuki says so, with the owner's name, and shows no account. */
async function hidden(context: StoryContext, name: string, list: "following" | "followers"): Promise<void> {
  const { t, panel } = await chosen(context, list);
  const said = t("profile.follows_hidden_private", { name });
  await expect(panel.getAllByRole("status").map((s) => plain(s.textContent))).toContain(said);
  await expect(panel.queryAllByRole("link")).toHaveLength(0);
}

/** mio's followers, shown to mio's followers only, whom yuki is not one of. */
export const followersOnly = (context: StoryContext) => hidden(context, "Mio 澪", "followers");

/** kai's follows, shown to nobody. */
export const privateList = (context: StoryContext) => hidden(context, "Kai", "following");

/** A list that failed says so, with the retry. */
export async function listFailed(context: StoryContext): Promise<void> {
  const { t, panel } = await chosen(context, "followers");
  const alert = panel.getByRole("alert");
  await expect(plain(alert.textContent)).toContain(t("profile.followers_failed"));
  await expect(within(alert).getByRole("button", { name: t("profile.retry") })).toBeEnabled();
}

/** A list with no account says so. */
export async function empty(context: StoryContext): Promise<void> {
  const { t, panel } = await chosen(context, "followers");
  await expect(panel.getAllByRole("status").map((s) => plain(s.textContent))).toContain(t("profile.followers_empty"));
  await expect(panel.queryAllByRole("link")).toHaveLength(0);
}
