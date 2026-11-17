// SPDX-License-Identifier: AGPL-3.0-only

// The home timeline as a feature: what its loader waits for, the column it shows under the timeline's name, and what
// a reader's press on a post asks of the server or of the screen. Sam reads a server that holds two posts of Yuki's.

import { expect, test } from "bun:test";
import { homeTimelineQuery } from "@my/app-data";
import { scenario, type FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
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
