// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, server, status } from "../test/server";
import { loadThread, Thread } from "./thread";

const back = () => undefined;

const focused = status("f", "mika", { in_reply_to_id: "a4", boosts_count: 12, favourites_count: 48 });

async function shown(context: unknown) {
  const { data, requests } = server({ "/statuses/f": focused, "/statuses/f/context": context });
  await loadThread(data, "f");
  return { html: render(data, <Thread id="f" onBack={back} />), requests };
}

test("the loader reads the post and its thread", async () => {
  const { requests } = await shown({ ancestors: [], descendants: [] });
  expect(requests).toEqual(["GET /statuses/f", "GET /statuses/f/context"]);
});

test("the focused post says when it was posted, to whom and its counts inside it, with no stamp", async () => {
  const { html } = await shown({ ancestors: [], descendants: [] });
  expect(html).toContain('<time dateTime="2026-11-14T03:00:00Z">');
  expect(html).toContain("12 boosts");
  expect(html).toContain("48 favourites");
  expect(html).not.toContain('href="/@mika/status/f"');
  expect(html).toContain("No replies yet.");
});

test("between the thread's first post and the two closest, the ancestors fold", async () => {
  const ancestors = ["a1", "a2", "a3", "a4"].map((id) => status(id, "alice"));
  const { html } = await shown({ ancestors, descendants: [] });
  expect(html).toContain("Post a1");
  expect(html).toContain("Show 1 earlier post");
  expect(html).not.toContain("Post a2");
  expect(html.indexOf("Post a3")).toBeLessThan(html.indexOf("Post f"));
});

test("the replies follow under their heading, each in the feed's order", async () => {
  const descendants = [status("r1", "alice", { in_reply_to_id: "f" }), status("r2", "mika", { in_reply_to_id: "r1" })];
  const { html } = await shown({ ancestors: [], descendants });
  expect(html).toContain(">Replies</h2>");
  expect(html.indexOf("Post f")).toBeLessThan(html.indexOf("Post r1"));
  expect(html.indexOf("Post r1")).toBeLessThan(html.indexOf("Post r2"));
  expect(html).toMatch(/aria-posinset="3" aria-setsize="3"/);
});

test("a post that could not be read does not fail the loader: the screen reads it again, and says it loads", async () => {
  const { data, requests } = server({});
  await loadThread(data, "f");
  expect(requests).toHaveLength(2);
  const html = render(data, <Thread id="f" onBack={back} />);
  expect(html).toContain("Loading the post…");
  expect(html).toContain(">Back<");
});
