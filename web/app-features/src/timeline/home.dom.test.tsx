// SPDX-License-Identifier: AGPL-3.0-only

// The home timeline as a feature: what its loader waits for, the column it shows under the timeline's name, and what
// a reader's press on a post asks of the server or of the screen. Sam reads a server that holds two posts of Yuki's.

import { expect, test } from "bun:test";
import { homeTimelineQuery, preferencesQuery, receiveHomeStatuses } from "@my/app-data";
import { scenario, type FakeOptions } from "@my/e2e/fake";
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import { PostActions, type ComposeRequest } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
import { HomeTimeline, loadHomeTimeline } from "./home";

const yuki = "Yuki 雪";

const server: FakeOptions = {
  viewer: "sam",
  seed: {
    people: scenario.people,
    follows: [{ follower: "sam", followee: "yuki" }],
    posts: [
      { key: "bread", author: "yuki", text: "Bread at ten on Saturday", language: "en", visibility: "public" },
      {
        key: "poll",
        author: "yuki",
        text: "Which bread for Saturday?",
        language: "en",
        visibility: "public",
        poll: { options: ["Sourdough", "Rye"], days: 1 },
      },
    ],
  },
};

/** The post of Yuki's whose text is `text`, in the feed named `name`. */
const post = (text: string, name = "Home timeline") =>
  within(screen.getByRole("feed", { name }))
    .getAllByRole("article", { name: yuki })
    .find((article) => within(article).queryAllByText(text).length > 0);

test("the loader waits for the first page, and the column shows it at once under the timeline's name", async () => {
  const { fake, data } = fakeData(server);
  const read = fake.script("GET /timelines/home");

  await loadHomeTimeline(data);
  expect(read.seen).toHaveLength(1);
  renderFeature(data, <HomeTimeline />, "ja");

  // No wait: the first page is there when the column is drawn.
  expect(post("Bread at ten on Saturday", "ホームタイムライン")).toBeDefined();
  expect(post("Which bread for Saturday?", "ホームタイムライン")).toBeDefined();
});

test("a failed first page does not fail the loader: the column fetches it again and shows how that goes", async () => {
  const { fake, data } = fakeData(server);
  const read = fake.script("GET /timelines/home", { times: 1, fail: 500 });

  await loadHomeTimeline(data);
  expect(data.queryClient.getQueryState(homeTimelineQuery(data).queryKey)?.status).toBe("error");
  renderFeature(data, <HomeTimeline />);

  expect(screen.queryAllByRole("feed")).toHaveLength(0);
  expect(screen.getByText("Loading posts…").getAttribute("role")).toBe("status");
  await waitFor(() => {
    if (post("Bread at ten on Saturday") === undefined) throw new Error("the column shows no post yet");
  });
  expect(read.seen).toHaveLength(2);
});

test("a post answers its actions: a boost reaches the server, and Quote opens the composer", async () => {
  const { fake, data } = fakeData(server);
  await loadHomeTimeline(data);
  const composed: ComposeRequest[] = [];
  const user = renderFeature(
    data,
    <PostActions onCompose={(to) => composed.push(to)}>
      <HomeTimeline />
    </PostActions>,
  );
  const bread = post("Bread at ten on Saturday");
  if (bread === undefined) throw new Error("test: the timeline shows no post of Yuki's");
  const boosts = fake.script("POST /statuses/{id}/boost");

  await user.click(within(bread).getByRole("button", { name: /^Boost/ }));
  await user.click(await screen.findByRole("menuitem", { name: "Boost" }));
  const { url } = await boosts.arrived();
  expect(url.pathname).toEndWith(`/statuses/${fake.idOf("bread")}/boost`);
  await waitFor(() => {
    if (within(bread).queryAllByRole("button", { name: /^Boost/, pressed: true }).length === 0)
      throw new Error("the post does not say it is boosted");
  });

  await user.click(within(bread).getByRole("button", { name: /^Boost/ }));
  await user.click(await screen.findByRole("menuitem", { name: "Quote" }));
  expect(composed).toEqual([{ kind: "quote", id: fake.idOf("bread") }]);
});

test("a vote in a post's poll reaches the server", async () => {
  const { fake, data } = fakeData(server);
  await loadHomeTimeline(data);
  const user = renderFeature(
    data,
    <PostActions onCompose={() => undefined}>
      <HomeTimeline />
    </PostActions>,
  );
  const polled = post("Which bread for Saturday?");
  if (polled === undefined) throw new Error("test: the timeline shows no poll of Yuki's");
  const poll = within(within(polled).getByRole("group", { name: "Poll" }));
  const votes = fake.script("POST /statuses/{id}/poll/votes");

  await user.click(poll.getByRole("radio", { name: "Rye" }));
  await user.click(poll.getByRole("button", { name: "Vote" }));

  const { url, body } = await votes.arrived();
  expect(url.pathname).toEndWith(`/statuses/${fake.idOf("poll")}/poll/votes`);
  expect(body.choices).toEqual([1]);
  await waitFor(() => {
    if (poll.queryAllByText(/your vote/).length !== 1) throw new Error("the poll shows no vote of the reader's");
  });
});

// Whether new posts go in by themselves is the reader's choice (#3199), which the shell read before any screen.

/** The home timeline of a reader whose preferences are `preferences`, read as the shell reads them, and drawn. */
async function homeWith(preferences: NonNullable<FakeOptions["preferences"]>) {
  cleanup();
  const made = fakeData({ ...server, preferences });
  await made.data.queryClient.query(preferencesQuery(made.data));
  await loadHomeTimeline(made.data);
  const user = renderFeature(
    made.data,
    <PostActions onCompose={() => undefined}>
      <HomeTimeline />
    </PostActions>,
  );
  return { ...made, user, feed: within(screen.getByRole("feed", { name: "Home timeline" })) };
}

/** Two posts arrive on the reader's stream, as the app's stream hands them to the home timeline. */
function twoArrive({ fake, data }: ReturnType<typeof fakeData>) {
  const arrived = fake.newPosts(2);
  act(() => {
    receiveHomeStatuses(data, arrived);
  });
  return arrived;
}

test("for a reader who chose to hold new posts back, they wait behind the button, and go in when it is pressed", async () => {
  const shown = await homeWith({ timeline_auto_insert: false });
  expect(shown.feed.getAllByRole("article")).toHaveLength(2);

  twoArrive(shown);

  const button = await screen.findByRole("button", { name: "2 new posts" });
  expect(shown.feed.getAllByRole("article")).toHaveLength(2);
  await shown.user.click(button);
  await waitFor(() => {
    if (shown.feed.getAllByRole("article").length !== 4) throw new Error("the new posts did not go in");
  });
  expect(screen.queryAllByRole("button", { name: /new posts?$/ })).toHaveLength(0);
});

test("until the reader's preferences are read, new posts are held back; once read, they go in or stay as the reader chose", async () => {
  /** The home timeline of a reader who set `timeline_auto_insert` so, drawn while their preferences are on their way. */
  async function unread(autoInsert: boolean) {
    cleanup();
    const made = fakeData({ ...server, preferences: { timeline_auto_insert: autoInsert } });
    const read = made.fake.script("GET /preferences", { hold: "request" });
    await loadHomeTimeline(made.data);
    renderFeature(
      made.data,
      <PostActions onCompose={() => undefined}>
        <HomeTimeline />
      </PostActions>,
    );
    await read.arrived();
    const feed = within(screen.getByRole("feed", { name: "Home timeline" }));
    twoArrive(made);
    // Nothing says yet whether the reader wants them in: they wait.
    await screen.findByRole("button", { name: "2 new posts" });
    expect(feed.getAllByRole("article")).toHaveLength(2);
    read.release();
    await waitFor(() => {
      if (made.data.queryClient.getQueryData(preferencesQuery(made.data).queryKey) === undefined)
        throw new Error("the preferences are not read yet");
    });
    return feed;
  }

  const inserts = await unread(true);
  await waitFor(() => {
    if (inserts.getAllByRole("article").length !== 4) throw new Error("the new posts did not go in");
  });
  expect(screen.queryAllByRole("button", { name: /new posts?$/ })).toHaveLength(0);

  const holds = await unread(false);
  // Read, and the reader chose to be asked: a turn later the posts still wait.
  await act(async () => {
    await Promise.resolve();
  });
  expect(screen.getByRole("button", { name: "2 new posts" })).not.toBeNull();
  expect(holds.getAllByRole("article")).toHaveLength(2);
});

test("for a reader who did not, new posts go in by themselves, with no button", async () => {
  const shown = await homeWith({ timeline_auto_insert: true });

  twoArrive(shown);

  await waitFor(() => {
    if (shown.feed.getAllByRole("article").length !== 4) throw new Error("the new posts did not go in");
  });
  expect(screen.queryAllByRole("button", { name: /new posts?$/ })).toHaveLength(0);
});

/** The people suggested over the home timeline, and the feed under them, as the screen shows them. */
const suggested = () => screen.queryByRole("list", { name: "Suggested for you" });
const feed = () => screen.queryByRole("feed", { name: "Home timeline" });

/** Quickdeals follows no one: the server suggests people to them, and their home holds one post, their own. */
const newcomer: FakeOptions = { viewer: "quickdeals" };

/** Waits until the home timeline's first page is read, so that a test looks at the screen when it could have it. */
const timelineRead = (data: ReturnType<typeof fakeData>["data"]) =>
  waitFor(() => {
    if (data.queryClient.getQueryState(homeTimelineQuery(data).queryKey)?.status !== "success")
      throw new Error("the first page of the timeline is not read yet");
  });

test("for a reader who follows no one, no post shows while the suggestions are on their way, then the people and the feed show together, the people first", async () => {
  const { fake, data } = fakeData(newcomer);
  const suggestions = fake.script("GET /accounts/suggestions", { hold: "response" });
  const loading = loadHomeTimeline(data);
  renderFeature(data, <HomeTimeline />);

  await suggestions.served();
  await timelineRead(data);
  // The timeline is read and the suggestions are not: the column is loading, and nothing of the feed is drawn.
  expect(screen.getByText("Loading posts…").getAttribute("role")).toBe("status");
  expect(feed()).toBeNull();
  expect(screen.queryAllByRole("article")).toHaveLength(0);
  expect(suggested()).toBeNull();

  suggestions.release();
  await loading;
  const people = await screen.findByRole("list", { name: "Suggested for you" });
  const [first] = within(screen.getByRole("feed", { name: "Home timeline" })).getAllByRole("article");
  if (first === undefined) throw new Error("test: the home timeline shows no post");
  expect(people.compareDocumentPosition(first) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  expect(screen.queryByText("Loading posts…")).toBeNull();
});

// The first paint does not wait for a retry of the suggestions after a failed answer: the feed shows at once, with
// one request seen.
test("for a reader who follows no one, suggestions that fail leave the feed alone, and the block is not added later", async () => {
  const { fake, data } = fakeData(newcomer);
  const suggestions = fake.script("GET /accounts/suggestions", { fail: 500 });
  await loadHomeTimeline(data);
  renderFeature(data, <HomeTimeline />);

  expect(screen.getAllByRole("article").length).toBeGreaterThan(0);
  expect(suggested()).toBeNull();
  await waitFor(() => {
    if (data.queryClient.isFetching() > 0) throw new Error("the server's answers are still on their way");
  });
  expect(suggestions.seen).toHaveLength(1);
  expect(suggested()).toBeNull();
  expect(screen.queryAllByRole("alert")).toHaveLength(0);
});

test("for a reader who follows no one, suggestions that name no one leave the feed alone", async () => {
  const { fake, data } = fakeData(newcomer);
  fake.script("GET /accounts/suggestions", { change: (page) => ({ ...page, items: [] }) });
  await loadHomeTimeline(data);
  renderFeature(data, <HomeTimeline />);

  expect(screen.getAllByRole("article").length).toBeGreaterThan(0);
  await waitFor(() => {
    if (data.queryClient.isFetching() > 0) throw new Error("the server's answers are still on their way");
  });
  expect(suggested()).toBeNull();
  expect(screen.queryAllByRole("link", { name: "Find more people" })).toHaveLength(0);
});

test("for a reader who follows someone, no suggestions are asked for, and the feed shows without them", async () => {
  const { fake, data } = fakeData(server);
  const suggestions = fake.script("GET /accounts/suggestions");
  await loadHomeTimeline(data);
  renderFeature(data, <HomeTimeline />);

  expect(post("Bread at ten on Saturday")).toBeDefined();
  await waitFor(() => {
    if (data.queryClient.isFetching() > 0) throw new Error("the server's answers are still on their way");
  });
  expect(suggestions.seen).toHaveLength(0);
  expect(suggested()).toBeNull();
});

/** Follows the first person the block suggests, as the viewer's reading says they now follow one, and waits for it. */
async function followFromTheBlock({ fake, data }: ReturnType<typeof fakeData>, user: ReturnType<typeof renderFeature>) {
  const people = await screen.findByRole("list", { name: "Suggested for you" });
  const viewer = fake.script("GET /accounts/verify_credentials", {
    change: (account) => ({ ...account, following_count: 1 }),
  });
  const follow = fake.script("POST /accounts/{id}/follow");
  const [row] = within(people).getAllByRole("listitem");
  if (row === undefined) throw new Error("test: the block suggests no one");
  await user.click(within(row).getByRole("button", { name: "Follow" }));
  await follow.served();
  // Nothing in the block reads the viewer again by itself; whatever does (a refocus, the stream) is this.
  await data.queryClient.invalidateQueries({ queryKey: ["get", "/accounts/verify_credentials"] });
  await viewer.served();
  await waitFor(() => {
    if (data.queryClient.isFetching() > 0) throw new Error("the server's answers are still on their way");
  });
}

test("the block stays while the reader stays on Home, though they now follow someone, and the feed stays after it", async () => {
  const made = fakeData(newcomer);
  await loadHomeTimeline(made.data);
  const user = renderFeature(made.data, <HomeTimeline />);

  await followFromTheBlock(made, user);

  const people = screen.getByRole("list", { name: "Suggested for you" });
  const [first] = within(screen.getByRole("feed", { name: "Home timeline" })).getAllByRole("article");
  if (first === undefined) throw new Error("test: the home timeline shows no post");
  expect(people.compareDocumentPosition(first) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
});

test("leaving Home and coming back decides again: after a follow, the feed shows without the block", async () => {
  const made = fakeData(newcomer);
  const suggestions = made.fake.script("GET /accounts/suggestions");
  await loadHomeTimeline(made.data);
  const user = renderFeature(made.data, <HomeTimeline />);
  await followFromTheBlock(made, user);
  const asked = suggestions.seen.length;

  cleanup();
  await loadHomeTimeline(made.data);
  renderFeature(made.data, <HomeTimeline />);

  expect(screen.getAllByRole("article").length).toBeGreaterThan(0);
  expect(suggested()).toBeNull();
  await waitFor(() => {
    if (made.data.queryClient.isFetching() > 0) throw new Error("the server's answers are still on their way");
  });
  expect(suggested()).toBeNull();
  expect(suggestions.seen).toHaveLength(asked);
});
