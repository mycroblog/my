// SPDX-License-Identifier: AGPL-3.0-only
// The posts that the viewer's keyword filters fold, through the whole app (web-client/README.md T4, ADR-0157 §2): the
// server marks them in `filtered`, and the cards show only what they matched until the viewer shows them.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { type Schemas, statusFixture, statusPage, timelineStatusFixture } from "../../test/fixtures";
import { apiBase } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

const spoilers: Schemas["FilterResult"] = { filter: { id: "f1", title: "Spoilers" }, keyword_matches: ["finale"] };
const politics: Schemas["FilterResult"] = { filter: { id: "f2", title: "Politics" }, keyword_matches: ["vote"] };

const folded: Schemas["Status"] = {
  ...timelineStatusFixture,
  content: "<p>The finale was a dream.</p>",
  filtered: [spoilers, politics],
};

beforeEach(() => {
  sessionStorage.clear();
});

describe("a post that keyword filters fold", () => {
  test("is an article named by the filters it matched, and nothing of it is read until it is shown", async () => {
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([folded]))));
    const { user } = await renderApp({ path: "/local" });

    const article = await screen.findByRole("article", { name: "Filtered: ⁨Spoilers⁩ and ⁨Politics⁩" });
    expect(article).not.toHaveTextContent("The finale was a dream.");
    expect(within(article).queryByRole("link", { name: /Bob/ })).not.toBeInTheDocument();

    await user.click(within(article).getByRole("button", { name: "Show" }));

    expect(await screen.findByText("The finale was a dream.")).toBeInTheDocument();
    // The post's own card is the feed's only article now, named by its author.
    const [card] = within(screen.getByRole("feed")).getAllByRole("article");
    expect(card).toHaveAccessibleName(/Bob/);
    await waitFor(() => {
      expect(card).toHaveFocus();
    });
    expect(screen.queryByRole("button", { name: "Show" })).not.toBeInTheDocument();
  });

  test("folds only the quote when only the quoted post matched", async () => {
    const quoted: Schemas["Status"] = { ...folded, id: "0192f0c4-7a8e-7000-8000-0000000000b2", filtered: [spoilers] };
    const post: Schemas["Status"] = {
      ...statusFixture("0192f0c4-7a8e-7000-8000-0000000000f0"),
      content: "<p>Look at this</p>",
      quote: { state: "accepted", quoted_status: quoted },
    };
    server.use(http.get(`${apiBase}/statuses/:id`, () => HttpResponse.json(post)));
    const { user } = await renderApp({ path: `/@${post.account.acct}/status/${post.id}` });

    const group = await screen.findByRole("group", { name: "Quoted post" });
    expect(screen.getByText("Look at this")).toBeInTheDocument();
    expect(group).toHaveTextContent("Filtered: ⁨Spoilers⁩");
    // Behind the disclosure, hidden: neither seen nor read (web-client/README.md Y8).
    expect(within(group).getByText("The finale was a dream.")).not.toBeVisible();

    await user.click(within(group).getByRole("button", { name: "Show the quoted post" }));
    expect(within(group).getByText("The finale was a dream.")).toBeVisible();
  });
});
