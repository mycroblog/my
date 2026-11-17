// SPDX-License-Identifier: AGPL-3.0-only

// A thread as a feature: what its loader reads, the focused post between what it answers and its replies, and what a
// press on one of its posts asks of the server or of the screen.

import { expect, test } from "bun:test";
import { scenario, type FakeSeed } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { PostActions, type ComposeRequest } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
import { loadThread, Thread } from "./thread";

const back = () => undefined;

type Post = FakeSeed["posts"][number];

const by = (author: string, key: string, more: Partial<Post> = {}): Post => ({
  key,
  author,
  text: `Post ${key}`,
  visibility: "public",
  language: "en",
  ...more,
});

/** The thread of sam's post `f`, under four posts of hana's, which three people favourited and two boosted. */
const ancestors = [
  by("hana", "a1"),
  ...[2, 3, 4].map((n) => by("hana", `a${String(n)}`, { replyTo: `a${String(n - 1)}` })),
];
const focused = by("sam", "f", { replyTo: "a4" });

/** A server that holds `posts`, of which `f` is read as its thread's screen reads it, and drawn. */
async function shown(posts: Post[]) {
  const made = fakeData({
    seed: {
      people: scenario.people,
      posts,
      interactions: [
        ...["hana", "marco", "layla"].map((by) => ({ kind: "favourite" as const, by, post: "f" })),
        ...["hana", "marco"].map((by) => ({ kind: "boost" as const, by, post: "f" })),
      ],
    },
  });
  const id = made.fake.idOf("f");
  await loadThread(made.data, id);
  const loaded = [...made.requests];
  const composed: ComposeRequest[] = [];
  const user = renderFeature(
    made.data,
    <PostActions onCompose={(to) => composed.push(to)}>
      <Thread id={id} onBack={back} />
    </PostActions>,
  );
  return { ...made, id, loaded, user, composed };
}

/** The posts the screen shows, each by its text, in the order a reader meets them. */
const posts = () => screen.getAllByRole("article");
const texts = () => posts().map((post) => within(post).queryAllByText(/^Post \w+$/)[0]?.textContent);
const post = (key: string) => {
  const found = posts().find((article) => within(article).queryAllByText(`Post ${key}`).length > 0);
  if (found === undefined) throw new Error(`test: the thread shows no post ${key}`);
  return found;
};

test("the loader reads the post and its thread", async () => {
  const { loaded, id } = await shown([by("sam", "f")]);

  expect(loaded).toEqual([`GET /statuses/${id}`, `GET /statuses/${id}/context`]);
});

test("the focused post says when it was posted, its counts only on its actions, with no stamp that leads to itself", async () => {
  const { data, id } = await shown([by("sam", "f")]);
  const { data: read } = await data.api.GET("/statuses/{id}", { params: { path: { id } } });
  const shownPost = within(post("f"));

  expect(shownPost.getAllByRole<HTMLTimeElement>("time").map((time) => time.dateTime)).toEqual([
    read?.created_at ?? "",
  ]);
  expect(shownPost.getByRole("button", { name: "Favourite, 3" })).not.toBeNull();
  expect(shownPost.getByRole("button", { name: "Boost, 2" })).not.toBeNull();
  expect(shownPost.queryAllByText(/2 boosts|3 favourites/)).toHaveLength(0);
  expect(shownPost.getAllByRole("link").map((link) => link.getAttribute("href"))).not.toContain(`/@sam/status/${id}`);
  expect(screen.getByText("No replies yet.")).not.toBeNull();
});

test("between the thread's first post and the two closest, the ancestors fold, and a press shows them", async () => {
  const { user } = await shown([...ancestors, focused]);

  expect(texts()).toEqual(["Post a1", "Post a3", "Post a4", "Post f"]);
  await user.click(screen.getByRole("button", { name: "Show 1 earlier post" }));

  expect(texts()).toEqual(["Post a1", "Post a2", "Post a3", "Post a4", "Post f"]);
});

test("the replies follow under their heading, each in the feed's order, with its position in it", async () => {
  await shown([by("sam", "f"), by("hana", "r1", { replyTo: "f" }), by("sam", "r2", { replyTo: "r1" })]);
  const heading = screen.getByRole("heading", { level: 2, name: "Replies" });

  expect(texts()).toEqual(["Post f", "Post r1", "Post r2"]);
  // A feed holds articles alone: the heading stands at the head of the first reply, after the focused post.
  expect(post("f").compareDocumentPosition(heading) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  expect(heading.compareDocumentPosition(screen.getByText("Post r1")) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  expect(
    posts().map((article) => [article.getAttribute("aria-posinset"), article.getAttribute("aria-setsize")]),
  ).toEqual([
    ["1", "3"],
    ["2", "3"],
    ["3", "3"],
  ]);
});

test("a post that could not be read does not fail the loader: the screen says it loads, with its Back", async () => {
  const { data, requests } = fakeData();
  const none = "00000000-0000-7000-8000-00000000ffff";

  await loadThread(data, none);
  expect(requests).toHaveLength(2);
  renderFeature(data, <Thread id={none} onBack={back} />);

  expect(screen.getByText("Loading the post…")).not.toBeNull();
  expect(screen.getByRole("button", { name: "Back" })).not.toBeNull();
});

test("the focused post and a reply answer their actions: a favourite reaches the server, Reply opens the composer", async () => {
  const { fake, user, composed, id } = await shown([by("sam", "f"), by("hana", "r", { replyTo: "f" })]);
  const favourite = fake.script("POST /statuses/{id}/favourite");

  await user.click(within(post("f")).getByRole("button", { name: "Favourite, 3" }));
  expect((await favourite.arrived()).url.pathname).toEndWith(`/statuses/${id}/favourite`);
  await waitFor(() => {
    if (within(post("f")).queryAllByRole("button", { name: "Favourite, 4", pressed: true }).length === 0)
      throw new Error("the focused post does not count the reader's favourite");
  });

  await user.click(within(post("r")).getByRole("button", { name: /^Reply/ }));
  expect(composed).toEqual([{ kind: "reply", id: fake.idOf("r") }]);
});

test("the focused post shows its poll, and a vote in it reaches the server", async () => {
  const { fake, user, id } = await shown([by("sam", "f", { poll: { options: ["Sourdough", "Rye"], days: 1 } })]);
  const sent = fake.script("POST /statuses/{id}/poll/votes");
  const poll = within(within(post("f")).getByRole("group", { name: "Poll" }));

  await user.click(poll.getByRole("radio", { name: "Sourdough" }));
  await user.click(poll.getByRole("button", { name: "Vote" }));

  const { url, body } = await sent.arrived();
  expect(url.pathname).toEndWith(`/statuses/${id}/poll/votes`);
  expect(body.choices).toEqual([0]);
  await waitFor(() => {
    if (poll.queryAllByText(/your vote/).length !== 1) throw new Error("the poll shows no vote of the reader's");
  });
});
