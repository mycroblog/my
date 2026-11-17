// SPDX-License-Identifier: AGPL-3.0-only

// What a reader does with a post's author and their server from the post's More menu, in the home timeline: the menu
// names both, and a mute, a block (after its confirmation) and a mute of the server each put a line in the post's
// place that says what was done, with its undoing. The profile's own menu is in profile.dom.test.tsx.

import { expect, test } from "bun:test";
import { scenario, type Route } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { PostActions } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
import { HomeTimeline, loadHomeTimeline } from "../timeline/home";

const remote = "Developed the first roll from the coast.";

/**
 * The home timeline of yuki, who follows sam alone, loaded and drawn, with the More menu open on its one post: sora's,
 * of another server (photos.example), which sam boosted.
 */
async function menuOpen() {
  const made = fakeData({
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: [{ key: "film", author: "sora@photos.example", text: remote, visibility: "public", language: "en" }],
      interactions: [{ kind: "boost", by: "sam", post: "film" }],
    },
  });
  await loadHomeTimeline(made.data);
  const user = renderFeature(
    made.data,
    <PostActions onCompose={() => undefined}>
      <HomeTimeline />
    </PostActions>,
  );
  const post = screen.getAllByRole("article").find((article) => article.textContent.includes(remote));
  if (post === undefined) throw new Error("test: the home timeline shows no post of another server");
  const first = screen.getAllByRole("article").indexOf(post);
  await user.click(within(post).getByRole("button", { name: "More" }));
  const menu = within(await screen.findByRole("menu", { name: "More" }));
  /** What stands in the post's place: its words, and its buttons by their names. */
  const place = () => {
    const article = screen.getAllByRole("article")[first];
    if (article === undefined) throw new Error("test: nothing stands in the post's place");
    return {
      said: article.textContent,
      buttons: within(article)
        .queryAllByRole("button")
        .map((button) => button.textContent),
    };
  };
  return { ...made, user, menu, place };
}

test("the More menu of a post of another server names its author and their server: Mute and Block… under the one, Mute server under the other", async () => {
  const { menu } = await menuOpen();
  const items = (group: string) =>
    within(menu.getByRole("group", { name: group }))
      .getAllByRole("menuitem")
      .map((item) => item.textContent);

  expect(items("@sora")).toEqual(["Mute", "Block…"]);
  expect(items("photos.example")).toEqual(["Mute server"]);
});

for (const [item, route, line, undo] of [
  ["Mute", "POST /accounts/{id}/mute", /^You muted ⁨?@sora⁩?\.\s?Undo$/, "Undo"],
  ["Block…", "POST /accounts/{id}/block", /^You blocked ⁨?@sora⁩?\.\s?Unblock$/, "Unblock"],
  ["Mute server", "POST /domain_mutes", /^You muted the server ⁨?photos\.example⁩?\.\s?Undo$/, "Undo"],
] as const satisfies readonly (readonly [string, Route, RegExp, string])[])
  test(`${item} from a post's menu${item === "Block…" ? ", once confirmed," : ""} reaches the server and puts a line in the post's place that says so, with ${undo}`, async () => {
    const { fake, user, menu, place } = await menuOpen();
    const sent = fake.script(route);

    await user.click(menu.getByRole("menuitem", { name: item }));
    if (item === "Block…") {
      const confirmation = within(await screen.findByRole("alertdialog", { name: /^Block ⁨?@sora⁩?\?$/ }));
      expect(sent.seen).toHaveLength(0);
      await user.click(confirmation.getByRole("button", { name: "Block" }));
    }

    await sent.arrived();
    await waitFor(() => {
      if (!place().buttons.includes(undo)) throw new Error(`the post's place says "${place().said}"`);
    });
    expect(place().said).toMatch(line);
    expect(place().buttons).toEqual([undo]);
    // The menu and the confirmation have left: the line stands alone.
    await waitFor(() => {
      if (screen.queryAllByRole("menu").length + screen.queryAllByRole("alertdialog").length > 0)
        throw new Error("the menu or the confirmation is still open");
    });
  });
