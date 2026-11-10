// SPDX-License-Identifier: AGPL-3.0-only
// A notification from the event stream about the viewer's post reads the post again and shows its new counts in every
// cached copy (web-client/README.md S10, FR-207): here the home timeline holds the post and a boost of it.
import { screen, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import { otherAccountFixture, statusFixture, statusPage, viewerStatusFixture } from "../../test/fixtures";
import { apiBase, problem, resumableSession } from "../../test/msw/http";
import { streamingServer } from "../../test/msw/streaming";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Status } from "./api/status";
import { postRefreshWindow } from "./api/live";

const post: Status = viewerStatusFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000000e1",
  content: "<p>My post</p>",
  favourites_count: 2,
  boosts_count: 1,
  replies_count: 1,
});
const boost: Status = { ...statusFixture("0192f0c4-7a8e-7000-8000-0000000000e2"), content: "", boost: post };
/** A post that no cached answer holds. */
const uncachedId = "0192f0c4-7a8e-7000-8000-0000000000e9";

/** How long a test waits for the read that a notification starts, in ms. */
const readTimeout = postRefreshWindow * 4;

let notificationIds = 0;

function notification(type: string, status: Status) {
  notificationIds += 1;
  return {
    id: `0192f0c4-7a8e-7000-8000-${String(notificationIds).padStart(12, "0")}`,
    type,
    created_at: "2026-10-16T10:00:00Z",
    account: otherAccountFixture,
    status,
  };
}

/**
 * The home timeline holds the viewer's post and a boost of it; GET /statuses/{id} answers `answer(id)` and records the
 * IDs it is asked for.
 */
async function render(answer: (id: string) => Response) {
  const streams = streamingServer();
  const reads: string[] = [];
  server.use(
    resumableSession(),
    streams.handler,
    http.get(`${apiBase}/timelines/home`, () => HttpResponse.json(statusPage([post, boost]))),
    http.get(`${apiBase}/statuses/:id`, ({ params }) => {
      const id = String(params["id"]);
      reads.push(id);
      return answer(id);
    }),
  );
  const view = await renderApp({ path: "/home" });
  expect(await screen.findAllByRole("button", { name: "Favourite, 2" })).toHaveLength(2);
  await waitFor(() => {
    expect(streams.connections()).toBe(1);
  });
  return { ...view, stream: streams.current(), reads };
}

function serve(changed: Partial<Status>) {
  return () => HttpResponse.json({ ...post, ...changed });
}

describe("a notification about the viewer's post", () => {
  test("of a favourite shows the post's new count in the timeline and inside the boost", async () => {
    const { stream, reads } = await render(serve({ favourites_count: 3 }));

    stream.send("notification", notification("favourite", post));

    expect(await screen.findAllByRole("button", { name: "Favourite, 3" }, { timeout: readTimeout })).toHaveLength(2);
    expect(reads).toEqual([post.id]);
  });

  test("of a reaction shows the post's new reactions", async () => {
    const { stream } = await render(serve({ reactions: [{ name: "👍", count: 2, me: false }] }));

    stream.send("notification", notification("reaction", post));

    expect(
      await screen.findAllByRole("button", { name: "👍 2, react with this emoji" }, { timeout: readTimeout }),
    ).toHaveLength(2);
  });

  test("of a boost shows the post's new count of boosts", async () => {
    const { stream } = await render(serve({ boosts_count: 4 }));

    stream.send("notification", notification("boost", post));

    expect(await screen.findAllByRole("button", { name: "Boost, 4" }, { timeout: readTimeout })).toHaveLength(2);
  });

  test("of a reply reads the post replied to and shows its new count of replies", async () => {
    const { stream, reads } = await render(serve({ replies_count: 2 }));
    const reply: Status = { ...statusFixture("0192f0c4-7a8e-7000-8000-0000000000e3"), in_reply_to_id: post.id };

    stream.send("notification", notification("mention", reply));

    expect(await screen.findAllByRole("link", { name: "Read 2 replies" }, { timeout: readTimeout })).toHaveLength(2);
    expect(reads).toEqual([post.id]);
  });

  test("of a quote reads the quoted post again", async () => {
    const { stream, reads } = await render(serve({ favourites_count: 3 }));
    const quote: Status = {
      ...statusFixture("0192f0c4-7a8e-7000-8000-0000000000e4"),
      quote: { state: "accepted", quoted_status: post },
    };

    stream.send("notification", notification("quote", quote));

    expect(await screen.findAllByRole("button", { name: "Favourite, 3" }, { timeout: readTimeout })).toHaveLength(2);
    expect(reads).toEqual([post.id]);
  });

  test("reads no post that no cached answer holds", async () => {
    const { stream, reads } = await render(serve({ favourites_count: 3 }));

    stream.send("notification", notification("favourite", { ...post, id: uncachedId }));
    // A later notification about the cached post: once it is read, the first one's window has long passed.
    stream.send("notification", notification("favourite", post));

    expect(await screen.findAllByRole("button", { name: "Favourite, 3" }, { timeout: readTimeout })).toHaveLength(2);
    expect(reads).toEqual([post.id]);
  });

  test("in a burst reads the post once", async () => {
    const { stream, reads } = await render(serve({ favourites_count: 5 }));

    stream.send("notification", notification("favourite", post));
    stream.send("notification", notification("reaction", post));
    stream.send("notification", notification("boost", post));

    expect(await screen.findAllByRole("button", { name: "Favourite, 5" }, { timeout: readTimeout })).toHaveLength(2);
    expect(reads).toEqual([post.id]);
  });

  test("does not undo the viewer's favourite on its way", async () => {
    let release: (() => void) | undefined;
    const answered = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.post(`${apiBase}/statuses/:id/favourite`, async () => {
        await answered;
        return HttpResponse.json({ ...post, favourited: true, favourites_count: 4 });
      }),
    );
    // The server has not seen the viewer's favourite yet, and someone else boosted the post.
    const { stream, user } = await render(serve({ favourited: false, favourites_count: 3, boosts_count: 2 }));
    const [favourite] = screen.getAllByRole("button", { name: "Favourite, 2" });
    if (favourite === undefined) throw new Error("the post has no Favourite button");

    await user.click(favourite);
    expect(await screen.findAllByRole("button", { name: "Favourite, 3", pressed: true })).toHaveLength(2);

    stream.send("notification", notification("boost", post));

    expect(await screen.findAllByRole("button", { name: "Boost, 2" }, { timeout: readTimeout })).toHaveLength(2);
    expect(screen.getAllByRole("button", { name: "Favourite, 3", pressed: true })).toHaveLength(2);
    release?.();
    expect(await screen.findAllByRole("button", { name: "Favourite, 4", pressed: true })).toHaveLength(2);
  });

  test.each([
    ["fails", () => problem(503, "about:blank", "Service Unavailable")],
    ["is gone", () => problem(404, "about:blank", "Not Found")],
  ])("leaves the post as it was, without an error, when the read %s", async (_, answer) => {
    const { stream, reads } = await render(answer);

    stream.send("notification", notification("favourite", post));

    await waitFor(
      () => {
        expect(reads).toEqual([post.id]);
      },
      { timeout: readTimeout },
    );
    expect(await screen.findAllByRole("button", { name: "Favourite, 2" })).toHaveLength(2);
    expect(screen.getAllByText("My post")).toHaveLength(2);
    // The announcer's assertive region stays empty: no toast says anything.
    for (const alert of screen.queryAllByRole("alert")) expect(alert).toBeEmptyDOMElement();
  });
});
