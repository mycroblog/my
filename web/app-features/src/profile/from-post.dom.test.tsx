// SPDX-License-Identifier: AGPL-3.0-only

// What a reader does with a post's author and their server from the post's More menu, in the home timeline: the menu
// names both, and a mute, a block (after its confirmation) and a mute of the server each put a line in the post's
// place that says what was done, with its undoing. Each sentence names an author of another server with their server
// and one of this server without (#3174; the rule itself: web/app-components/src/account.test.ts). The profile's own
// menu is in profile.dom.test.tsx.

import { expect, test } from "bun:test";
import { scenario, type Route } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { PostActions } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
import { HomeTimeline, loadHomeTimeline } from "../timeline/home";

/** The authors of the timeline's two posts: sora, of another server (photos.example), and sam, of this one. */
const authors = {
  remote: { handle: "@sora@photos.example", text: "Developed the first roll from the coast." },
  local: { handle: "@sam", text: "The first loaf." },
} as const;

type Author = keyof typeof authors;

/** `handle` as a sentence holds it: whole between the marks that isolate its direction (U+2068…U+2069). */
const isolated = (handle: string) => `⁨?${handle.replaceAll(".", "\\.")}⁩?`;

/**
 * The home timeline of yuki, who follows sam alone, loaded and drawn, with the More menu open on the post of
 * `author`: sam's own, or sora's, which sam boosted.
 */
async function menuOpen(author: Author) {
  const made = fakeData({
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: [
        { key: "loaf", author: "sam", text: authors.local.text, visibility: "public", language: "en" },
        { key: "film", author: "sora@photos.example", text: authors.remote.text, visibility: "public", language: "en" },
      ],
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
  const post = screen.getAllByRole("article").find((article) => article.textContent.includes(authors[author].text));
  if (post === undefined) throw new Error(`test: the home timeline shows no post of the ${author} author`);
  const at = screen.getAllByRole("article").indexOf(post);
  await user.click(within(post).getByRole("button", { name: "More" }));
  const menu = within(await screen.findByRole("menu", { name: "More" }));
  /** What stands in the post's place: its words, and its buttons by their names. */
  const place = () => {
    const article = screen.getAllByRole("article")[at];
    if (article === undefined) throw new Error("test: nothing stands in the post's place");
    return {
      said: article.textContent,
      buttons: within(article)
        .queryAllByRole("button")
        .map((button) => button.textContent),
    };
  };
  /** The items of the menu's group named `group`, by their words. */
  const items = (group: string) =>
    within(menu.getByRole("group", { name: group }))
      .getAllByRole("menuitem")
      .map((item) => item.textContent);
  return { ...made, user, menu, place, items };
}

test("the More menu of a post of another server names its author, with their server, and the server: Mute and Block… under the one, Mute server under the other", async () => {
  const { items } = await menuOpen("remote");

  expect(items("@sora@photos.example")).toEqual(["Mute", "Block…"]);
  expect(items("photos.example")).toEqual(["Mute server"]);
});

test("the More menu of a post of this server names its author without a server, and offers no mute of a server", async () => {
  const { menu, items } = await menuOpen("local");

  expect(items("@sam")).toEqual(["Mute", "Block…"]);
  expect(menu.queryAllByRole("menuitem", { name: "Mute server" })).toHaveLength(0);
});

const cases = [
  ...(["remote", "local"] as const).flatMap((author) => {
    const handle = isolated(authors[author].handle);
    return [
      [author, "Mute", "POST /accounts/{id}/mute", `^You muted ${handle}\\.\\s?Undo$`, "Undo"],
      [author, "Block…", "POST /accounts/{id}/block", `^You blocked ${handle}\\.\\s?Unblock$`, "Unblock"],
    ] as const;
  }),
  [
    "remote",
    "Mute server",
    "POST /domain_mutes",
    `^You muted the server ${isolated("photos.example")}\\.\\s?Undo$`,
    "Undo",
  ],
] as const satisfies readonly (readonly [Author, string, Route, string, string])[];

for (const [author, item, route, line, undo] of cases)
  test(`${item} from the menu of a post of ${author === "remote" ? "another" : "this"} server${item === "Block…" ? ", once confirmed," : ""} reaches the server and puts a line in the post's place that says so, with ${undo}`, async () => {
    const { fake, user, menu, place } = await menuOpen(author);
    const sent = fake.script(route);

    await user.click(menu.getByRole("menuitem", { name: item }));
    if (item === "Block…") {
      const question = new RegExp(`^Block ${isolated(authors[author].handle)}\\?$`);
      const confirmation = within(await screen.findByRole("alertdialog", { name: question }));
      expect(sent.seen).toHaveLength(0);
      await user.click(confirmation.getByRole("button", { name: "Block" }));
    }

    await sent.arrived();
    await waitFor(() => {
      if (!place().buttons.includes(undo)) throw new Error(`the post's place says "${place().said}"`);
    });
    expect(place().said).toMatch(new RegExp(line));
    expect(place().buttons).toEqual([undo]);
    // The menu and the confirmation have left: the line stands alone.
    await waitFor(() => {
      if (screen.queryAllByRole("menu").length + screen.queryAllByRole("alertdialog").length > 0)
        throw new Error("the menu or the confirmation is still open");
    });
  });
