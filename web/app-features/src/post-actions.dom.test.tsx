// SPDX-License-Identifier: AGPL-3.0-only

// What a post's actions do under PostActions, pressed on a screen that shows posts (the home timeline): which open
// the composer, which reach the server and show at once, which say what they did once the server answered, and which
// write nothing.

import { describe, expect, onTestFinished, test } from "bun:test";
import type { PostAction } from "@my/app-components";
import { preferencesQuery, updatePreferences, type LinkCardSize } from "@my/app-data";
import { fakeOrigin, scenario, type FakeApi } from "@my/e2e/fake";
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import { PostActions, usePostActions, usePostCardSize, type ComposeRequest } from "./post-actions";
import { fakeData, renderFeature } from "./test/feature";
import { HomeTimeline, loadHomeTimeline } from "./timeline/home";

/**
 * A server where the reader follows sam: the reader's own post and poll, sam's first loaf, and sam's poll of several
 * choices.
 * `before` acts on the server before the home timeline is loaded and drawn under PostActions, in `locale`; `post(text)`
 * is the post that says `text`.
 */
async function timeline({
  locale = "en",
  before,
}: { locale?: string | undefined; before?: ((fake: FakeApi) => Promise<void>) | undefined } = {}) {
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
  await before?.(made.fake);
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
    locale,
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
  expect(await menuOf("The first loaf.")).toEqual([
    "Copy link",
    "Share",
    "Bookmark",
    "Mute thread",
    "Mute",
    "Block…",
    "Report…",
  ]);
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

// What the More menu's writes that leave no mark on the post say (#3367): Bookmark, Remove bookmark, Mute thread and
// Unmute thread each tell the reader in the page's live region, once the server has answered, and a failure offers
// Try again.

/** What the page's live regions say now: the words of each status that says something. */
const notices = () =>
  screen
    .queryAllByRole("status")
    .map((region) => region.textContent)
    .filter((words) => words !== "");

/** The status that says `words`, beside what it offers, or nothing. */
const noticeOf = (words: string) =>
  screen.queryAllByRole("status").find((region) => region.textContent.includes(words));

/**
 * Keeps every notice the live regions say from now until the test ends, each once as it comes: what a wait for the
 * end state cannot see, a notice that showed and went.
 */
function heard() {
  const said: string[] = [];
  const listen = () => {
    for (const words of notices()) if (!said.includes(words)) said.push(words);
  };
  const observer = new MutationObserver(listen);
  observer.observe(document.body, { childList: true, subtree: true, characterData: true });
  onTestFinished(() => {
    observer.disconnect();
  });
  return () => {
    listen();
    return said;
  };
}

/**
 * Keeps what the page waits about six seconds for (from 5 s to 7 s) until the test lets it come, from now until the
 * test ends: `asked()` is how many such waits began, and `pass()` lets them come at once. What Testing Library waits
 * for is shorter, what the data's cache waits for longer, and both keep their time.
 */
function sixSecondWaits() {
  const wait = globalThis.setTimeout;
  const held: (() => void)[] = [];
  const holding = ((handler: TimerHandler, timeout?: number, ...rest: unknown[]) => {
    if (typeof handler !== "function" || timeout === undefined || timeout < 5000 || timeout > 7000)
      return wait(handler, timeout, ...rest);
    held.push(() => {
      (handler as (...given: unknown[]) => void)(...rest);
    });
    return wait(() => undefined, 0);
  }) as typeof setTimeout;
  globalThis.setTimeout = Object.assign(holding, wait);
  onTestFinished(() => {
    globalThis.setTimeout = wait;
  });
  return {
    asked: () => held.length,
    pass: () => {
      act(() => {
        for (const come of held.splice(0)) come();
      });
    },
  };
}

/** Bookmarks the post `key` and mutes its thread on the server, past the app, as another device of the reader's does. */
const bookmarkedAndMuted = (key: string) => async (fake: FakeApi) => {
  for (const what of ["bookmark", "mute"]) {
    const answer = await fake.serve(
      new Request(`${fakeOrigin}/api/v1/statuses/${fake.idOf(key)}/${what}`, { method: "POST" }),
    );
    if (!answer.ok) throw new Error(`test: the fake answered ${String(answer.status)} to POST …/${what}`);
  }
};

type Write = {
  /** The item of the More menu. */
  item: string;
  route:
    | "POST /statuses/{id}/bookmark"
    | "DELETE /statuses/{id}/bookmark"
    | "POST /statuses/{id}/mute"
    | "DELETE /statuses/{id}/mute";
  /** What the live region says once the server did it. */
  done: string;
  /** What it says when the write did not finish. */
  failed: string;
  /** Whether the post is bookmarked and its thread muted before. */
  undoes: boolean;
};

const writes: readonly Write[] = [
  {
    item: "Bookmark",
    route: "POST /statuses/{id}/bookmark",
    done: "Post bookmarked",
    failed: "The bookmark did not finish.",
    undoes: false,
  },
  {
    item: "Remove bookmark",
    route: "DELETE /statuses/{id}/bookmark",
    done: "Bookmark removed",
    failed: "The bookmark did not finish.",
    undoes: true,
  },
  {
    item: "Mute thread",
    route: "POST /statuses/{id}/mute",
    done: "Thread muted",
    failed: "The thread's mute did not finish.",
    undoes: false,
  },
  {
    item: "Unmute thread",
    route: "DELETE /statuses/{id}/mute",
    done: "Thread unmuted",
    failed: "The thread's mute did not finish.",
    undoes: true,
  },
];

/** The home timeline, where sam's first loaf is bookmarked and its thread muted when `undoes`; `choose` presses an item of its More menu. */
async function loafMenu({ undoes, locale }: { undoes: boolean; locale?: string }) {
  const drawn = await timeline({ locale, before: undoes ? bookmarkedAndMuted("loaf") : undefined });
  const { user, post } = drawn;
  const more = locale === "ja" ? "その他" : "More";
  const choose = async (name: string) => {
    await user.click(post("The first loaf.").getByRole("button", { name: more }));
    await user.click(within(await screen.findByRole("menu", { name: more })).getByRole("menuitem", { name }));
    await waitFor(() => {
      if (screen.queryAllByRole("menu").length > 0) throw new Error("the menu is still open");
    });
  };
  return { ...drawn, choose };
}

describe.each(writes)("$item from a post's More menu", ({ item, route, done, failed, undoes }) => {
  test(`says "${done}" in the live region once the server has answered, with no action, for six seconds`, async () => {
    const { fake, choose } = await loafMenu({ undoes });
    const said = heard();
    const waits = sixSecondWaits();
    const sent = fake.script(route, { hold: "request" });

    await choose(item);
    const { url } = await sent.arrived();
    expect(url.pathname).toEndWith(
      `/statuses/${fake.idOf("loaf")}/${route.endsWith("bookmark") ? "bookmark" : "mute"}`,
    );
    // Not before the server has answered.
    expect(said()).toEqual([]);

    sent.release();
    await waitFor(() => {
      if (noticeOf(done) === undefined) throw new Error(`the live region does not say "${done}"`);
    });
    expect(said()).toEqual([done]);
    expect(within(noticeOf(done) ?? document.body).queryAllByRole("button")).toHaveLength(0);
    expect(screen.queryAllByRole("button", { name: "Try again" })).toHaveLength(0);
    // It stays for as long as the page waits, six seconds, and no longer.
    expect(waits.asked()).toBe(1);

    waits.pass();
    await waitFor(() => {
      if (noticeOf(done) !== undefined) throw new Error(`the live region still says "${done}"`);
    });
  });

  test(`a write that gets no answer says "${failed}", never "${done}", and Try again sends it again`, async () => {
    const { fake, user, choose } = await loafMenu({ undoes });
    const said = heard();
    const every = fake.script(route);
    const lost = fake.script(route, { times: 1, hold: "request", fail: "network" });

    await choose(item);
    await lost.arrived();
    expect(said()).toEqual([]);
    lost.release();
    await waitFor(() => {
      if (noticeOf(failed) === undefined) throw new Error(`the live region does not say "${failed}"`);
    });
    expect(said().filter((words) => words.includes(done))).toEqual([]);
    expect(within(noticeOf(failed) ?? document.body).getAllByRole("button", { name: "Try again" })).toHaveLength(1);

    await user.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => {
      if (noticeOf(done) === undefined) throw new Error(`the live region does not say "${done}" after Try again`);
    });
    // The same write, of the same post, twice.
    const path = `/api/v1/statuses/${fake.idOf("loaf")}/${route.endsWith("bookmark") ? "bookmark" : "mute"}`;
    expect(every.seen.map((request) => request.url.pathname)).toEqual([path, path]);
  });

  test("a write the server refuses says the server's own words, and offers Try again", async () => {
    const { fake, choose } = await loafMenu({ undoes });
    const said = heard();
    const title = "This post cannot be changed right now.";
    fake.script(route, { times: 1, fail: { type: "about:blank", title, status: 422 } });

    await choose(item);
    await waitFor(() => {
      if (noticeOf(title) === undefined) throw new Error("the live region does not say the server's refusal");
    });
    expect(said().filter((words) => words.includes(done))).toEqual([]);
    expect(within(noticeOf(title) ?? document.body).getAllByRole("button", { name: "Try again" })).toHaveLength(1);
  });
});

test("the notices are in the reader's language: ja", async () => {
  const { choose } = await loafMenu({ undoes: false, locale: "ja" });
  const said = heard();
  /** Chooses `item` and waits until the live region says `words`. */
  const says = async (item: string, words: string) => {
    await choose(item);
    await waitFor(() => {
      if (noticeOf(words) === undefined) throw new Error(`the live region does not say "${words}"`);
    });
  };

  await says("ブックマーク", "投稿をブックマークしました");
  await says("ブックマークを外す", "ブックマークを外しました");
  await says("スレッドをミュート", "スレッドをミュートしました");
  await says("スレッドのミュートを解除", "スレッドのミュートを解除しました");

  expect(said()).toEqual([
    "投稿をブックマークしました",
    "ブックマークを外しました",
    "スレッドをミュートしました",
    "スレッドのミュートを解除しました",
  ]);
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
