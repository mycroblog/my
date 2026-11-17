// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { PostActions } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
import { loadPublicTimeline, PublicTimeline } from "./public";

test("a signed-in reader's vote in a post's poll on the local timeline reaches the server", async () => {
  // sam asks which loaf to bake, in a poll the reader has not voted in: the one post of the server.
  const { fake, data } = fakeData({
    seed: {
      people: scenario.people,
      posts: [
        {
          key: "sam-poll",
          author: "sam",
          text: "Which loaf this weekend?",
          visibility: "public",
          language: "en",
          poll: { options: ["Sourdough", "Rye"], days: 1 },
        },
      ],
    },
  });
  const sent = fake.script("POST /statuses/{id}/poll/votes");
  await loadPublicTimeline(data, "local");
  const user = renderFeature(
    data,
    <PostActions onCompose={() => undefined}>
      <PublicTimeline kind="local" />
    </PostActions>,
  );
  const [poll] = screen.getAllByRole("group", { name: "Poll" });
  if (poll === undefined) throw new Error("test: the local timeline shows no poll");
  const [first] = within(poll).getAllByRole("radio");
  if (first === undefined) throw new Error("test: the poll has no option");

  await user.click(first);
  await user.click(within(poll).getByRole("button", { name: "Vote" }));

  const { url, body } = await sent.arrived();
  expect(url.pathname).toEndWith(`/statuses/${fake.idOf("sam-poll")}/poll/votes`);
  expect(body.choices).toEqual([0]);
  await waitFor(() => {
    if (within(poll).queryAllByText(/your vote/).length !== 1)
      throw new Error("the poll shows no vote of the reader's");
  });
});
