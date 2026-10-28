// SPDX-License-Identifier: AGPL-3.0-only
// The quote of a post through the whole app (web-client/README.md T4, FR-114): the real router and QueryClient, MSW for
// the client API.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { apiBase } from "../../test/msw/http";
import { remoteStatusFixture, type Schemas, statusFixture } from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Status } from "./api/status";

const postId = "0192f0c4-7a8e-7000-8000-0000000000f0";

const quoted: Status = {
  ...remoteStatusFixture,
  content: "<p>The sun rose again.</p>",
  spoiler_text: "",
  media_attachments: [],
};

/** Answers GET /statuses/{id} with a post that has `quote`, and returns the path of its page. */
function serveQuote(quote: Schemas["StatusQuote"]) {
  const post: Status = { ...statusFixture(postId), content: "<p>Look at this</p>", quote };
  server.use(http.get(`${apiBase}/statuses/:id`, () => HttpResponse.json(post)));
  return `/@${post.account.acct}/status/${post.id}`;
}

beforeEach(() => {
  sessionStorage.clear();
});

describe("the quote of a post", () => {
  test("shows the quoted post with its author, and its time linking to its page", async () => {
    await renderApp({ path: serveQuote({ state: "legacy", quoted_status: quoted }) });

    const group = await screen.findByRole("group", { name: "Quoted post" });
    expect(group).toHaveTextContent("The sun rose again.");
    expect(within(group).getByRole("link", { name: /Carol/ })).toHaveAttribute("href", `/@${quoted.account.acct}`);
    const links = within(group).getAllByRole("link");
    expect(links.some((link) => link.getAttribute("href") === `/@${quoted.account.acct}/status/${quoted.id}`)).toBe(
      true,
    );
  });

  test("tells why the quoted post is not shown", async () => {
    await renderApp({ path: serveQuote({ state: "deleted" }) });
    expect(await screen.findByRole("group", { name: "Quoted post" })).toHaveTextContent("The quoted post was deleted.");
  });

  test("tells that a quote awaits its approval", async () => {
    await renderApp({ path: serveQuote({ state: "pending" }) });
    expect(await screen.findByRole("group", { name: "Quoted post" })).toHaveTextContent(
      "The quoted post is not available yet.",
    );
  });

  test("keeps the post of a muted author behind the notice until it is shown", async () => {
    const user = userEvent.setup();
    await renderApp({ path: serveQuote({ state: "muted_account", quoted_status: quoted }) });

    const group = await screen.findByRole("group", { name: "Quoted post" });
    expect(group).toHaveTextContent("You muted the author of the quoted post.");
    expect(within(group).getByText("The sun rose again.")).not.toBeVisible();
    await user.click(within(group).getByRole("button", { name: "Show the quoted post" }));
    expect(within(group).getByText("The sun rose again.")).toBeVisible();
  });
});
