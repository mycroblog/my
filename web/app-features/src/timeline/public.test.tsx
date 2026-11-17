// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { publicTimelineQuery } from "@my/app-data";
import { PostActions } from "../post-actions";
import { press } from "../test/actions";
import { fakeData, markupOf } from "../test/feature";
import { loadPublicTimeline, PublicTimeline } from "./public";

test("a signed-in reader's vote in a post's poll on the local timeline reaches the server", async () => {
  // In the scenario sam asks which loaf to bake, in a poll the reader has not voted in.
  const { fake, data } = fakeData();
  const id = fake.idOf("sam-poll");
  const sent = fake.script("POST /statuses/{id}/poll/votes");
  await loadPublicTimeline(data, "local");
  const local = data.queryClient.getQueryData(publicTimelineQuery(data, "local").queryKey);
  const poll = local?.pages.flatMap((p) => p.items).find((s) => s.id === id)?.poll?.id ?? "";
  expect(poll).not.toBe("");
  const html = markupOf(
    data,
    <PostActions onCompose={() => undefined}>
      <PublicTimeline kind="local" />
    </PostActions>,
  );
  expect(html).toContain(">Vote<");
  press({ type: "vote", id, poll, choices: [0] });
  expect((await sent.arrived()).body).toEqual({ poll_id: poll, choices: [0] });
});
