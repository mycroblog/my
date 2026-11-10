// SPDX-License-Identifier: AGPL-3.0-only
// The mute of a post's thread through the whole app (web-client/README.md T4, ADR-0158 §8): the real router and
// QueryClient, MSW for the client API. The viewer mutes and unmutes the thread from a post's menu, and the posts of the
// thread on screen take the answer.
import { screen, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { statusFixture, viewerStatusFixture } from "../../test/fixtures";
import { apiBase, problem, resumableSession } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Status } from "./api/status";

const postId = "0192f0c4-7a8e-7000-8000-0000000000d1";
const ancestor: Status = { ...statusFixture("0192f0c4-7a8e-7000-8000-0000000000d0"), content: "<p>The question</p>" };
const post: Status = { ...statusFixture(postId), content: "<p>The post</p>", in_reply_to_id: ancestor.id };
const reply: Status = {
  ...viewerStatusFixture({ id: "0192f0c4-7a8e-7000-8000-0000000000d2", content: "<p>The answer</p>" }),
  in_reply_to_id: postId,
};

/** Serves the thread of `post` and records the mutes and unmutes; `fail` refuses them. */
function serveThread({ fail = false }: { fail?: boolean } = {}) {
  const requests: string[] = [];
  let muted = false;
  const withMute = (status: Status): Status => ({ ...status, muted });
  const answer = (on: boolean, id: string) => {
    requests.push(`${on ? "POST" : "DELETE"} ${id}`);
    if (fail) return problem(503, "/api/v1/problems/unavailable", "Service Unavailable");
    muted = on;
    const found = [post, ancestor, reply].find((status) => status.id === id) ?? post;
    return HttpResponse.json(withMute(found));
  };
  server.use(
    resumableSession(),
    http.get(`${apiBase}/statuses/:id/context`, ({ params }) =>
      HttpResponse.json(
        params["id"] === postId
          ? { ancestors: [withMute(ancestor)], descendants: [withMute(reply)] }
          : { ancestors: [], descendants: [] },
      ),
    ),
    http.get(`${apiBase}/statuses/:id`, ({ params }) =>
      HttpResponse.json(withMute([post, ancestor, reply].find((s) => s.id === params["id"]) ?? post)),
    ),
    http.post(`${apiBase}/statuses/:id/mute`, ({ params }) => answer(true, String(params["id"]))),
    http.delete(`${apiBase}/statuses/:id/mute`, ({ params }) => answer(false, String(params["id"]))),
  );
  return { requests };
}

/** Finds a visible toast by its text (the announcer's live regions say it too). */
async function findToast(text: string | RegExp) {
  return within(await screen.findByRole("region", { name: "Messages" })).findByText(text);
}

/** Opens the "More actions" menu of the post whose text is `text`. */
async function openMenu(user: UserEvent, text: string) {
  const article = await screen.findByRole("article", { description: text });
  // The actions show once the session has resumed.
  await user.click(await within(article).findByRole("button", { name: "More actions" }));
  return screen.findByRole("menu", { name: "More actions" });
}

beforeEach(() => {
  sessionStorage.clear();
});

describe("muting a thread", () => {
  test("mutes the thread from a post's menu, marks every post of the thread, and unmutes it the same way", async () => {
    const { requests } = serveThread();
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    // The replies load after the post: wait for the reply before muting.
    await screen.findByRole("article", { description: "The answer" });

    await user.click(within(await openMenu(user, "The post")).getByRole("menuitem", { name: "Mute this thread" }));
    expect(
      await findToast("Thread muted. Its posts still appear in your timelines, but it no longer notifies you."),
    ).toBeInTheDocument();
    expect(requests).toEqual([`POST ${postId}`]);

    // The viewer's own reply in the same thread offers the unmute; its post is still on screen.
    const menu = await openMenu(user, "The answer");
    expect(within(menu).queryByRole("menuitem", { name: "Mute this thread" })).not.toBeInTheDocument();
    await user.click(within(menu).getByRole("menuitem", { name: "Unmute this thread" }));
    expect(await findToast("Thread unmuted. It notifies you again.")).toBeInTheDocument();
    expect(requests).toEqual([`POST ${postId}`, `DELETE ${reply.id}`]);
    expect(
      within(await openMenu(user, "The question")).getByRole("menuitem", { name: "Mute this thread" }),
    ).toBeInTheDocument();
  });

  test("says why when the server refuses, and changes nothing", async () => {
    serveThread({ fail: true });
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    await screen.findByRole("article", { description: "The answer" });

    await user.click(within(await openMenu(user, "The post")).getByRole("menuitem", { name: "Mute this thread" }));
    expect(await findToast(/^Could not mute the thread: /)).toBeInTheDocument();
    expect(
      within(await openMenu(user, "The post")).getByRole("menuitem", { name: "Mute this thread" }),
    ).toBeInTheDocument();
  });
});
