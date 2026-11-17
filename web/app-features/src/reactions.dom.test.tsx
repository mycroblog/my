// SPDX-License-Identifier: AGPL-3.0-only

// A reader's emoji reaction to a post (FR-116), picked in the picker a post's React opens on a screen that shows posts
// (the home timeline): a pick from the keyboard reaches the server as one by a pointer does and gives the focus back,
// and the picker's search finds a custom emoji of the server by its alias. The picker's own keys are its tests'
// (web/design-system/src/emoji-picker.dom.test.tsx), a reaction that does not finish is post-actions.dom.test.tsx, and
// that the server keeps a reaction is an E2E test (web/e2e/specs/reactions.spec.ts).

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { act, screen, waitFor, within } from "@testing-library/react";
import { PostActions } from "./post-actions";
import { fakeData, renderFeature } from "./test/feature";
import { HomeTimeline, loadHomeTimeline } from "./timeline/home";

/** A server with its custom emoji where the reader (yuki) follows sam, who posted one post: the home timeline, drawn. */
async function timeline() {
  const made = fakeData({
    seed: {
      people: scenario.people,
      emojis: scenario.emojis,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: [{ key: "loaf", author: "sam", text: "The first loaf.", visibility: "public", language: "en" }],
    },
  });
  await loadHomeTimeline(made.data);
  const user = renderFeature(
    made.data,
    <PostActions onCompose={() => undefined}>
      <HomeTimeline />
    </PostActions>,
  );
  const post = () => within(screen.getByRole("article", { name: "Sam Rivera" }));
  /** The emoji named `name` in the section `section` of the open picker, once its grid shows it. */
  const emoji = async (section: string, name: string) => {
    const dialog = await screen.findByRole("dialog", { name: "React" });
    const grid = await within(dialog).findByRole("grid", { name: "Emoji" });
    return waitFor(() => within(within(grid).getByRole("rowgroup", { name: section })).getByRole("gridcell", { name }));
  };
  /** Waits until the picker is closed. */
  const closed = () =>
    waitFor(() => {
      if (screen.queryAllByRole("dialog", { name: "React" }).length > 0) throw new Error("the picker is still open");
    });
  /** The names of the post's reaction chips; none while it has no reaction. */
  const chips = () =>
    post()
      .queryAllByRole("group", { name: "Reactions" })
      .flatMap((group) => within(group).getAllByRole("button"))
      .map((chip) => chip.getAttribute("aria-label") ?? chip.textContent);
  return { ...made, user, post, emoji, closed, chips };
}

/** Gives `element` the focus, as Tab or the arrow keys bring it there. */
const focus = (element: HTMLElement) => {
  act(() => {
    element.focus();
  });
};

test("a reaction picked with Enter reaches the server, closes the picker and gives React the focus back; Space on it takes it back", async () => {
  const { fake, user, post, emoji, closed, chips } = await timeline();
  const set = fake.script("PUT /statuses/{id}/reaction");
  const takenBack = fake.script("DELETE /statuses/{id}/reaction");
  const react = () => post().getByRole("button", { name: "React" });

  focus(react());
  await user.keyboard("{Enter}");
  focus(await emoji("Custom emoji", ":wave:"));
  await user.keyboard("{Enter}");

  await closed();
  await waitFor(() => {
    if (set.seen.length !== 1) throw new Error("the reaction did not reach the server");
  });
  expect(set.seen[0]?.url.pathname).toEndWith(`/statuses/${fake.idOf("loaf")}/reaction`);
  await waitFor(() => {
    expect(chips()).toEqual([":wave:, 1, including yours"]);
  });
  expect(document.activeElement === react()).toBe(true);

  await user.keyboard(" ");
  focus(await emoji("Custom emoji", ":wave:, your reaction"));
  await user.keyboard(" ");

  await closed();
  await waitFor(() => {
    if (takenBack.seen.length !== 1) throw new Error("the reaction was not taken back on the server");
  });
  await waitFor(() => {
    expect(chips()).toEqual([]);
  });
  expect(set.seen).toHaveLength(1);
  expect(document.activeElement === react()).toBe(true);
});

test("the picker's search finds a custom emoji by its alias, and the reaction with it reaches the server", async () => {
  const { fake, user, post, emoji, chips } = await timeline();
  const set = fake.script("PUT /statuses/{id}/reaction");

  await user.click(post().getByRole("button", { name: "React" }));
  const dialog = within(await screen.findByRole("dialog", { name: "React" }));
  // "ocha" is no part of the shortcode: the server lists it as an alias of :teacup:.
  await user.type(await dialog.findByRole("searchbox", { name: "Search emoji" }), "ocha");
  const found = await emoji("Search results", ":teacup:");
  expect(within(dialog.getByRole("rowgroup", { name: "Search results" })).getAllByRole("gridcell")).toHaveLength(1);
  await user.click(found);

  await waitFor(() => {
    expect(chips()).toEqual([":teacup:, 1, including yours"]);
  });
  expect(set.seen).toHaveLength(1);
});
