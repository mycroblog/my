// SPDX-License-Identifier: AGPL-3.0-only

// What a post's actions do under PostActions, pressed on a screen that shows posts (the home timeline): which open
// the composer, which reach the server and show at once, and which write nothing.

import { expect, test } from "bun:test";
import type { PostAction } from "@my/app-components";
import { scenario } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { PostActions, usePostActions, type ComposeRequest } from "./post-actions";
import { fakeData, renderFeature } from "./test/feature";
import { HomeTimeline, loadHomeTimeline } from "./timeline/home";

/**
 * A server where the reader follows sam: the reader's own post and poll, sam's first loaf, and sam's poll of several
 * choices.
 * The home timeline is loaded and drawn under PostActions; `post(text)` is the post that says `text`.
 */
async function timeline() {
  const made = fakeData({
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: [
        { key: "own", author: "yuki", text: "Post 1", visibility: "public", language: "en" },
        {
          key: "ownPoll",
          author: "yuki",
          text: "Which loaf shall I bake?",
          visibility: "public",
          language: "en",
          poll: { options: ["Sourdough", "Rye"], days: 1 },
        },
        { key: "loaf", author: "sam", text: "The first loaf.", visibility: "public", language: "en" },
        {
          key: "asks",
          author: "sam",
          text: "Which loaves?",
          visibility: "public",
          language: "en",
          poll: { options: ["Sourdough", "Rye", "Focaccia"], multiple: true, days: 1 },
        },
      ],
    },
  });
  await loadHomeTimeline(made.data);
  made.requests.length = 0;
  const composed: ComposeRequest[] = [];
  let act: (action: PostAction) => void = () => undefined;
  /** The handler as a screen under PostActions gets it. */
  function Screen() {
    act = usePostActions();
    return <HomeTimeline />;
  }
  const user = renderFeature(
    made.data,
    <PostActions onCompose={(to) => composed.push(to)}>
      <Screen />
    </PostActions>,
  );
  const post = (text: string) => {
    const found = screen.getAllByRole("article").find((article) => within(article).queryAllByText(text).length > 0);
    if (found === undefined) throw new Error(`test: the timeline shows no post that says ${text}`);
    return within(found);
  };
  /** The requests that write: every one but a read. */
  const written = () => made.requests.filter((request) => !request.startsWith("GET "));
  return {
    ...made,
    user,
    composed,
    post,
    written,
    act: (action: PostAction) => {
      act(action);
    },
  };
}

test("Reply and Quote open the composer for the post, and write nothing", async () => {
  const { fake, user, composed, post, written } = await timeline();
  const id = fake.idOf("loaf");

  await user.click(post("The first loaf.").getByRole("button", { name: /^Reply/ }));
  await user.click(post("The first loaf.").getByRole("button", { name: /^Boost/ }));
  await user.click(await screen.findByRole("menuitem", { name: "Quote" }));

  expect(composed).toEqual([
    { kind: "reply", id },
    { kind: "quote", id },
  ]);
  expect(written()).toEqual([]);
});

test("a favourite shows at once, before the server answers, and reaches the server", async () => {
  const { fake, user, post, written } = await timeline();
  const sent = fake.script("POST /statuses/{id}/favourite", { hold: "request" });

  await user.click(post("Which loaves?").getByRole("button", { name: "Favourite", pressed: false }));
  await sent.arrived();

  expect(post("Which loaves?").getByRole("button", { name: "Favourite, 1", pressed: true })).not.toBeNull();
  expect(written()).toEqual([`POST /statuses/${fake.idOf("asks")}/favourite`]);
  sent.release();
  await waitFor(() => {
    if (post("Which loaves?").queryAllByRole("button", { name: "Favourite, 1", pressed: true }).length !== 1)
      throw new Error("the post lost the reader's favourite");
  });
});

test("a vote reaches the server with every choice, and opens no composer", async () => {
  const { fake, user, composed, post } = await timeline();
  const sent = fake.script("POST /statuses/{id}/poll/votes");
  const poll = within(post("Which loaves?").getByRole("group", { name: "Poll" }));

  await user.click(poll.getByRole("checkbox", { name: "Sourdough" }));
  await user.click(poll.getByRole("checkbox", { name: "Focaccia" }));
  await user.click(poll.getByRole("button", { name: "Vote" }));

  const { url, body } = await sent.arrived();
  expect(url.pathname).toEndWith(`/statuses/${fake.idOf("asks")}/poll/votes`);
  expect(body.choices).toEqual([0, 2]);
  expect(composed).toEqual([]);
  await waitFor(() => {
    if (poll.queryAllByText(/your vote/).length !== 2) throw new Error("the poll shows no votes of the reader's");
  });
});

test("Edit reads the post's source and opens the composer with it, and writes nothing", async () => {
  const { fake, user, composed, post, requests, written } = await timeline();
  const id = fake.idOf("own");

  await user.click(post("Post 1").getByRole("button", { name: "More" }));
  await user.click(await screen.findByRole("menuitem", { name: "Edit" }));

  await waitFor(() => {
    if (composed.length === 0) throw new Error("no composer was asked for yet");
  });
  expect(composed).toMatchObject([{ kind: "edit", id, source: { id, text: "Post 1", visibility: "public" } }]);
  expect(requests).toContain(`GET /statuses/${id}/source`);
  expect(written()).toEqual([]);
});

test("an action that is another feature's, or on a post the cache does not hold, writes nothing", async () => {
  const { fake, act, written, composed } = await timeline();

  act({ type: "translate", id: fake.idOf("own") });
  act({ type: "favourite", id: "00000000-0000-7000-8000-00000000ffff" });
  await Promise.resolve();

  expect(written()).toEqual([]);
  expect(composed).toEqual([]);
});

/** The words of the items of the menu that is open, in their order. */
const itemsOf = () =>
  within(screen.getByRole("menu", { name: "More" }))
    .getAllByRole("menuitem")
    .map((item) => item.textContent);

test("the More menu offers a reader what they may do with the post: their own, their own poll, another's", async () => {
  const { user, post } = await timeline();
  /** Opens the More menu of the post that says `text`, and gives the words of its items. */
  const menuOf = async (text: string) => {
    await user.click(post(text).getByRole("button", { name: "More" }));
    const items = itemsOf();
    await user.keyboard("{Escape}");
    return items;
  };

  expect(await menuOf("Post 1")).toEqual([
    "Copy link",
    "Share",
    "Bookmark",
    "Mute thread",
    "Edit",
    "Delete and redraft…",
    "Delete…",
  ]);
  // The server refuses an edit of a post with a poll.
  expect(await menuOf("Which loaf shall I bake?")).toEqual([
    "Copy link",
    "Share",
    "Bookmark",
    "Mute thread",
    "Delete and redraft…",
    "Delete…",
  ]);
  expect(await menuOf("The first loaf.")).toEqual(["Copy link", "Share", "Bookmark", "Mute thread", "Mute", "Block…"]);
});
