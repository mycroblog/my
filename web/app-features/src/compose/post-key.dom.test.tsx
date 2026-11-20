// SPDX-License-Identifier: AGPL-3.0-only

// The Idempotency-Key of a post (#2892): Post sends a post that got no answer again with the same key, so that it is
// posted once, and one the server answered with a new key, so that the post corrected is not taken for the one
// refused.

import { expect, test } from "bun:test";
import { defaultComposeSettings } from "@my/app-data";
import { ProblemError } from "@mycroblog/mykit";
import { screen, waitFor, within } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { ComposerView } from "./composer";
import { emptyDraft, postKey } from "./model";
import { isAnswer, useComposer } from "./use-composer";

const none = () => undefined;

/** The composer of a new post on a fake server, and the requests of its posts. */
function composer() {
  const on = fakeData();
  const initial = { ...emptyDraft({ kind: "new" }, defaultComposeSettings, "k"), text: "when?" };
  function Composing() {
    const c = useComposer(initial, { onPosted: none, onKept: none });
    return <ComposerView c={c} layer={false} onClose={none} onDismiss={none} />;
  }
  const user = renderFeature(on.data, <Composing />);
  return { on, user, form: within(screen.getByRole("dialog")) };
}

/** Presses Post and waits for the alert of its failure. */
async function postFails({ user, form }: ReturnType<typeof composer>): Promise<void> {
  await form.findAllByRole("textbox", { name: "Text" });
  await user.click(form.getByRole("button", { name: "Post" }));
  await waitFor(() => {
    expect(form.queryAllByRole("alert")).toHaveLength(1);
  });
}

test("a post the server refused goes again with a new key", async () => {
  const c = composer();
  const posts = c.on.fake.script("POST /statuses", { times: 1, fail: 422 });
  await postFails(c);
  await c.user.click(c.form.getByRole("button", { name: "Post" }));
  await posts.arrived(2);

  const [first, second] = posts.seen.map((s) => s.headers.get("Idempotency-Key"));
  expect(first).toBe("k");
  expect(second).not.toBe(first ?? "");
  expect(second).not.toBeNull();
});

test("a post that got no answer goes again with the same key", async () => {
  const c = composer();
  // The client of the test sends a request once: the composer's Post is the retry.
  const posts = c.on.fake.script("POST /statuses", { times: 1, fail: "network" });
  await postFails(c);
  await c.user.click(c.form.getByRole("button", { name: "Post" }));
  await posts.arrived(2);

  expect(new Set(posts.seen.map((s) => s.headers.get("Idempotency-Key")))).toEqual(new Set(["k"]));
});

test("the key is renewed after an answer, but not after the one that the first request with it still runs", () => {
  const problem = (type: string, status: number) => new ProblemError({ type, title: "", status }, new Headers());
  expect(isAnswer(problem("about:blank", 422))).toBe(true);
  expect(isAnswer(problem("about:blank", 503))).toBe(true);
  expect(isAnswer(problem("/api/v1/problems/idempotency-key-in-use", 409))).toBe(false);
  expect(isAnswer(new TypeError("Failed to fetch"))).toBe(false);
  expect(postKey({ key: "k" })).toBe("k");
  expect(postKey({ key: "k", answers: 2 })).not.toBe(postKey({ key: "k", answers: 1 }));
});
