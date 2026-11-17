// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario, type FakeSeed } from "@my/e2e/fake";
import { PostActions, type ComposeRequest } from "../post-actions";
import { press } from "../test/actions";
import { fakeData, markupOf } from "../test/feature";
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
const ancestors = [by("hana", "a1"), ...[2, 3, 4].map((n) => by("hana", `a${String(n)}`, { replyTo: `a${String(n - 1)}` }))];
const focused = by("sam", "f", { replyTo: "a4" });

/** A server that holds `posts`, of which `f` is read as its thread's screen reads it. */
async function shown(posts: Post[]) {
  const { fake, data, requests } = fakeData({
    seed: {
      people: scenario.people,
      posts,
      interactions: [
        ...["hana", "marco", "layla"].map((by) => ({ kind: "favourite" as const, by, post: "f" })),
        ...["hana", "marco"].map((by) => ({ kind: "boost" as const, by, post: "f" })),
      ],
    },
  });
  const id = fake.idOf("f");
  await loadThread(data, id);
  return { fake, data, requests, id, html: markupOf(data, <Thread id={id} onBack={back} />) };
}

test("the loader reads the post and its thread", async () => {
  const { requests, id } = await shown([by("sam", "f")]);
  expect(requests).toEqual([`GET /statuses/${id}`, `GET /statuses/${id}/context`]);
});

test("the focused post says when it was posted and to whom, its counts only on its actions, with no stamp", async () => {
  const { data, html, id } = await shown([by("sam", "f")]);
  const { data: post } = await data.api.GET("/statuses/{id}", { params: { path: { id } } });
  expect(html).toContain(`<time dateTime="${post?.created_at ?? ""}">`);
  expect(html).not.toContain("2 boosts");
  expect(html).not.toContain("3 favourites");
  expect(html).toMatch(/>3</);
  expect(html).not.toContain(`href="/@sam/status/${id}"`);
  expect(html).toContain("No replies yet.");
});

test("between the thread's first post and the two closest, the ancestors fold", async () => {
  const { html } = await shown([...ancestors, focused]);
  expect(html).toContain("Post a1");
  expect(html).toContain("Show 1 earlier post");
  expect(html).not.toContain("Post a2");
  expect(html.indexOf("Post a3")).toBeLessThan(html.indexOf("Post f"));
});

test("the replies follow under their heading, each in the feed's order", async () => {
  const { html } = await shown([by("sam", "f"), by("hana", "r1", { replyTo: "f" }), by("sam", "r2", { replyTo: "r1" })]);
  expect(html).toContain(">Replies</h2>");
  expect(html.indexOf("Post f")).toBeLessThan(html.indexOf("Post r1"));
  expect(html.indexOf("Post r1")).toBeLessThan(html.indexOf("Post r2"));
  expect(html).toMatch(/aria-posinset="3" aria-setsize="3"/);
});

test("a post that could not be read does not fail the loader: the screen reads it again, and says it loads", async () => {
  const { data, requests } = fakeData();
  const none = "00000000-0000-7000-8000-00000000ffff";
  await loadThread(data, none);
  expect(requests).toHaveLength(2);
  const html = markupOf(data, <Thread id={none} onBack={back} />);
  expect(html).toContain("Loading the post…");
  expect(html).toContain(">Back<");
});

test("the focused post and a reply answer their actions: a favourite reaches the server, Reply opens the composer", async () => {
  const { fake, data, requests, id } = await shown([by("sam", "f"), by("hana", "r", { replyTo: "f" })]);
  const favourite = fake.script("POST /statuses/{id}/favourite");
  const composed: ComposeRequest[] = [];
  markupOf(
    data,
    <PostActions onCompose={(to) => composed.push(to)}>
      <Thread id={id} onBack={back} />
    </PostActions>,
  );
  press({ type: "favourite", id });
  press({ type: "reply", id: fake.idOf("r") });
  await favourite.served();
  await Bun.sleep(0);
  expect(requests).toContain(`POST /statuses/${id}/favourite`);
  expect(composed).toEqual([{ kind: "reply", id: fake.idOf("r") }]);
  expect(markupOf(data, <Thread id={id} onBack={back} />)).toMatch(/>4</);
});

test("the focused post shows its poll, and a vote in it reaches the server", async () => {
  const { fake, data, id } = await shown([
    by("sam", "f", { poll: { options: ["Sourdough", "Rye"], days: 1 } }),
  ]);
  const sent = fake.script("POST /statuses/{id}/poll/votes");
  const { data: post } = await data.api.GET("/statuses/{id}", { params: { path: { id } } });
  const poll = post?.poll?.id ?? "";
  const html = markupOf(
    data,
    <PostActions onCompose={() => undefined}>
      <Thread id={id} onBack={back} />
    </PostActions>,
  );
  expect(html).toContain('aria-label="Poll"');
  expect(html).toContain(">Vote<");
  press({ type: "vote", id, poll, choices: [0] });
  expect((await sent.arrived()).body).toEqual({ poll_id: poll, choices: [0] });
});
