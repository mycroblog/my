// SPDX-License-Identifier: AGPL-3.0-only

// What a post's actions do under PostActions, pressed on a screen that shows posts (the home timeline): which open
// the composer, which reach the server and show at once, and which write nothing.

import { expect, test } from "bun:test";
import type { PostAction } from "@my/app-components";
import { preferencesQuery, updatePreferences, type LinkCardSize } from "@my/app-data";
import { scenario } from "@my/e2e/fake";
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import { PostActions, usePostActions, usePostCardSize, type ComposeRequest } from "./post-actions";
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

test("Mute thread reaches the server, and the menu then offers Unmute thread, which ends the mute and offers Mute thread again", async () => {
  const { fake, user, post } = await timeline();
  const id = fake.idOf("own");
  const muted = fake.script("POST /statuses/{id}/mute");
  const unmuted = fake.script("DELETE /statuses/{id}/mute");
  /** Opens the More menu of the reader's post, and chooses its item `name`. */
  const choose = async (name: string) => {
    await user.click(post("Post 1").getByRole("button", { name: "More" }));
    await user.click(within(await screen.findByRole("menu", { name: "More" })).getByRole("menuitem", { name }));
    await waitFor(() => {
      if (screen.queryAllByRole("menu").length > 0) throw new Error("the menu is still open");
    });
  };

  await choose("Mute thread");
  expect((await muted.arrived()).url.pathname).toEndWith(`/statuses/${id}/mute`);
  await choose("Unmute thread");
  expect((await unmuted.arrived()).url.pathname).toEndWith(`/statuses/${id}/mute`);

  await user.click(post("Post 1").getByRole("button", { name: "More" }));
  expect(itemsOf()).toContain("Mute thread");
  expect(itemsOf()).not.toContain("Unmute thread");
});

test("a vote that gets no answer holds the choice while it is in flight, shows the poll again, and Try again sends it", async () => {
  const { fake, user, post } = await timeline();
  const sent = fake.script("POST /statuses/{id}/poll/votes", { times: 1, hold: "request", fail: "network" });
  const poll = within(post("Which loaves?").getByRole("group", { name: "Poll" }));
  /** The results of the poll, an item per option. */
  const results = () => poll.queryAllByRole("listitem").map((item) => item.textContent);

  await user.click(poll.getByRole("checkbox", { name: "Rye" }));
  await user.click(poll.getByRole("button", { name: "Vote" }));
  await sent.arrived();
  await waitFor(() => {
    if (poll.getByRole("button", { name: "Vote" }).getAttribute("aria-busy") !== "true")
      throw new Error("the poll does not say the vote is in flight");
  });
  expect(poll.getByRole<HTMLInputElement>("checkbox", { name: "Rye" }).checked).toBe(true);
  expect(poll.getByRole<HTMLInputElement>("checkbox", { name: "Rye" }).disabled).toBe(true);
  expect(results()).toEqual([]);

  sent.release();
  expect(await screen.findByText("The vote did not finish.")).not.toBeNull();
  expect(results()).toEqual([]);
  expect(poll.getByRole<HTMLInputElement>("checkbox", { name: "Rye" }).disabled).toBe(false);

  await user.click(screen.getByRole("button", { name: "Try again" }));
  await waitFor(() => {
    if (poll.queryAllByText(/your vote/).length !== 1) throw new Error("the poll shows no vote of the reader's");
  });
  expect(results()).toEqual(["Sourdough0%", "Rye (your vote)100%", "Focaccia0%"]);
});

test("a reaction that does not finish puts back the favourite it took, and Try again sends it", async () => {
  const { fake, user, post } = await timeline();
  const loaf = () => post("The first loaf.");
  /** The reader's favourite of the post: the favourite button, pressed, named with its count; null without one. */
  const favourited = () => loaf().queryByRole("button", { name: "Favourite, 1", pressed: true });
  await user.click(loaf().getByRole("button", { name: "Favourite" }));
  await waitFor(() => {
    if (favourited() === null) throw new Error("the post does not say it is favourited");
  });
  const held = fake.script("PUT /statuses/{id}/reaction", { times: 1, hold: "request", fail: "network" });

  /** The reader's reaction: its chip, pressed, named "{emoji}, {count}, including yours". */
  const reaction = () =>
    waitFor(() =>
      within(loaf().getByRole("group", { name: "Reactions" })).getByRole("button", {
        name: "😀, 1, including yours",
        pressed: true,
      }),
    );
  await user.click(loaf().getByRole("button", { name: "React" }));
  const grid = await within(await screen.findByRole("dialog", { name: "React" })).findByRole("grid", { name: "Emoji" });
  await user.click(
    within(within(grid).getByRole("rowgroup", { name: "Smileys & emotion" })).getByRole("gridcell", {
      name: "grinning face",
    }),
  );
  await held.arrived();
  // Shown at once, before the server answers; a reaction takes the favourite's place.
  expect(await reaction()).not.toBeNull();
  expect(favourited()).toBeNull();

  held.release();
  expect(await screen.findByText("The reaction did not finish.")).not.toBeNull();
  expect(loaf().queryByRole("group", { name: "Reactions" })).toBeNull();
  expect(favourited()).not.toBeNull();

  await user.click(screen.getByRole("button", { name: "Try again" }));
  await waitFor(() => {
    if (held.seen.length !== 2) throw new Error("the reaction was not sent again");
  });
  expect(await reaction()).not.toBeNull();
  expect(favourited()).toBeNull();
});

// The size of a post's link card (#3199): the reader's choice, which every screen's post asks PostActions for. A
// simulation tells a small card from a large one by nothing a reader meets, so the size is read where a post reads it;
// what each size looks like is the design system's.

/** The size a post under PostActions draws its link card in, on `made`'s data: `size()` is what it reads now. */
function cardSizeOn({ data }: Pick<ReturnType<typeof fakeData>, "data">) {
  const seen: { size?: LinkCardSize } = {};
  function ReadsTheSize() {
    seen.size = usePostCardSize();
    return null;
  }
  renderFeature(
    data,
    <PostActions onCompose={() => undefined}>
      <ReadsTheSize />
    </PostActions>,
  );
  return () => seen.size;
}

/** Waits until the size read is `size`. */
const becomes = (read: () => LinkCardSize | undefined, size: LinkCardSize) =>
  waitFor(() => {
    if (read() !== size) throw new Error(`a post's card is ${String(read())}, not ${size}`);
  });

test("a post's link card is large until the reader's preferences are read, then of the size they chose", async () => {
  const made = fakeData({ preferences: { link_card_size: "small" } });
  const read = made.fake.script("GET /preferences", { hold: "request" });

  const size = cardSizeOn(made);
  await read.arrived();
  expect(size()).toBe("large");
  read.release();

  await becomes(size, "small");
});

test("a reader who chose no size gets large cards, and so does a reader without an account", async () => {
  const reader = fakeData();
  await reader.data.queryClient.query(preferencesQuery(reader.data));
  expect(cardSizeOn(reader)()).toBe("large");

  cleanup();
  const visitor = fakeData({ visitor: true });
  const size = cardSizeOn(visitor);
  await waitFor(() => {
    if (visitor.data.queryClient.isFetching() > 0) throw new Error("the server's answers are still on their way");
  });
  expect(size()).toBe("large");
});

test("a size chosen while posts show is theirs at once, and a choice the server refuses is taken back", async () => {
  const made = fakeData();
  await made.data.queryClient.query(preferencesQuery(made.data));
  const size = cardSizeOn(made);
  expect(size()).toBe("large");
  const sent = made.fake.script("PATCH /preferences", { hold: "request", fail: 500 });

  let refused: Promise<unknown> = Promise.resolve();
  act(() => {
    refused = updatePreferences(made.data, { link_card_size: "small" }).catch((error: unknown) => error);
  });
  // No turn passed: the request has not reached the server.
  expect(size()).toBe("small");
  await sent.arrived();
  sent.release();
  await act(() => refused);

  expect(size()).toBe("large");
});
