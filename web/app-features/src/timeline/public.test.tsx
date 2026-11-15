// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { PostActions } from "../post-actions";
import { press } from "../test/actions";
import { poll, render, server, status } from "../test/server";
import { loadPublicTimeline, PublicTimeline } from "./public";

test("a signed-in reader's vote in a post's poll on the local timeline reaches the server", async () => {
  const { data, received } = server({
    "/timelines/local": { items: [status("v", "sam", { poll: poll("p") })] },
    "/statuses/v/poll/votes": status("v", "sam", { poll: poll("p", { voted: true, own_votes: [0] }) }),
  });
  await loadPublicTimeline(data, "local");
  const html = render(
    data,
    <PostActions onCompose={() => undefined}>
      <PublicTimeline kind="local" />
    </PostActions>,
  );
  expect(html).toContain(">Vote<");
  press({ type: "vote", id: "v", poll: "p", choices: [0] });
  expect(await received("POST /statuses/v/poll/votes")).toEqual({ poll_id: "p", choices: [0] });
});
