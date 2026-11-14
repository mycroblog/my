// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { NetworkError } from "@mycroblog/mykit";
import { cachePostedStatus, findCachedStatus, postStatus, refusal, saveDraft, uploadAttachment } from "./compose";
import { fakeAppData, json, page, status } from "./test/fake";
import { statusContextQuery, type StatusContext } from "./thread";
import { homeTimelineQuery } from "./timeline";

const ids = (data: ReturnType<typeof fakeAppData>["data"]) =>
  data.queryClient.getQueryData(homeTimelineQuery(data).queryKey)?.pages[0]?.items.map((s) => s.id);

test("a posted status tops the home timeline at once, and its parent counts one more reply", async () => {
  const keys: (string | null)[] = [];
  const { data } = fakeAppData((request) => {
    if (request.method === "POST") {
      keys.push(request.headers.get("Idempotency-Key"));
      return json(201, status("new", { in_reply_to_id: "1" }));
    }
    return json(200, page(["1", "2"]));
  });
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  data.queryClient.setQueryData(
    homeTimelineQuery(data).queryKey,
    (cached) => cached && { ...cached, pages: [{ items: [status("1", { replies_count: 2 }), status("2")] }] },
  );
  await postStatus(data, { text: "hi", visibility: "public" }, "key-1");
  expect(keys).toEqual(["key-1"]);
  expect(ids(data)).toEqual(["new", "1", "2"]);
  expect(findCachedStatus(data.queryClient, "1")?.replies_count).toBe(3);
});

test("a posted reply shows in a cached thread of its parent, after the replies already under it", () => {
  const { data } = fakeAppData(() => new Response(null, { status: 500 }));
  const context = statusContextQuery(data, "0").queryKey;
  data.queryClient.setQueryData(context, {
    ancestors: [],
    descendants: [
      status("1", { in_reply_to_id: "0" }),
      status("2", { in_reply_to_id: "1" }),
      status("3", { in_reply_to_id: "0" }),
    ],
  });
  cachePostedStatus(data, status("new", { in_reply_to_id: "1" }));
  expect(data.queryClient.getQueryData<StatusContext>(context)?.descendants.map((s) => s.id)).toEqual([
    "1",
    "2",
    "new",
    "3",
  ]);
});

test("a refused post says the server's words; one without an answer says nothing of its own", async () => {
  const { data } = fakeAppData(() =>
    json(422, { type: "/api/v1/problems/status-rejected", title: "Refused", status: 422, detail: "Too long." }),
  );
  const refused = await postStatus(data, { text: "hi", visibility: "public" }, "k").catch((e: unknown) => e);
  expect(refusal(refused)).toBe("Too long.");
  expect(refusal(new NetworkError("POST", "/statuses", new Error("offline")))).toBeUndefined();
});

test("an upload waits until the server has processed the file, and a file it could not process fails", async () => {
  let looks = 0;
  const { data } = fakeAppData((request) => {
    if (request.method === "POST") return json(202, { id: "m", type: "image", state: "processing", meta: {} });
    looks += 1;
    return json(200, { id: "m", type: "image", state: looks < 2 ? "processing" : "ready", meta: {}, url: "u" });
  });
  let processing = false;
  const media = await uploadAttachment(data, new File(["x"], "a.png", { type: "image/png" }), {
    onProcessing: () => {
      processing = true;
    },
  });
  expect(processing).toBe(true);
  expect(media.state).toBe("ready");

  const failing = fakeAppData(() => json(200, { id: "m", type: "unknown", state: "failed", meta: {} }));
  const error = await uploadAttachment(failing.data, new File(["x"], "a.png")).catch((e: unknown) => e);
  expect((error as Error).name).toBe("MediaFailedError");
});

test("a draft saves over the one saved before, and as a new one when another device saved over it", async () => {
  const seen: string[] = [];
  const { data } = fakeAppData((request) => {
    seen.push(`${request.method} ${request.headers.get("If-Match") ?? ""}`);
    if (request.method === "PUT") return json(412, { type: "about:blank", title: "Precondition Failed", status: 412 });
    return new Response(JSON.stringify({ id: "d2" }), {
      status: 201,
      headers: { "Content-Type": "application/json", ETag: '"2"' },
    });
  });
  expect(await saveDraft(data, { visibility: "public" }, { id: "d1", etag: '"1"' })).toEqual({ id: "d2", etag: '"2"' });
  expect(seen).toEqual(['PUT "1"', "POST "]);
});
