// SPDX-License-Identifier: AGPL-3.0-only
// Reading articles through the whole app (web-client/README.md T4, ADR-0160): their cards in a timeline, and their
// page with the title as its heading and the body in full. The real router and QueryClient, MSW for the client API.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { apiBase, resumableSession } from "../../test/msw/http";
import { mediaFixture, remoteStatusFixture, statusPage, timelineStatusFixture } from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Status } from "./api/status";

const cover = mediaFixture("0192f0c4-7a8e-7000-8000-0000000000c0", "ready", "Tomatoes on a balcony");

/** A local article as lists carry it: its title, summary and cover, and no body. */
const localArticle: Status = {
  ...timelineStatusFixture,
  content: "",
  article: { title: "Growing tomatoes", summary: "Notes from three summers.", cover },
};

/** The same article as GET /statuses/{id} answers it, with its body. */
const localArticleInFull: Status = {
  ...localArticle,
  content: "<h2>Soil</h2><p>Use a deep pot.</p><table><thead><tr><th>Month</th></tr></thead></table>",
};

/** An article of another server without a title, in Japanese. */
const remoteArticle: Status = {
  ...remoteStatusFixture,
  id: "0192f0c4-7a8e-7000-8000-0000000003c9",
  content: "",
  spoiler_text: "",
  media_attachments: [],
  language: "ja",
  article: { title: "", summary: "トマトの話です。" },
};

beforeEach(() => {
  sessionStorage.clear();
  server.use(resumableSession());
});

describe("an article in a timeline", () => {
  beforeEach(() => {
    server.use(
      http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([localArticle, remoteArticle]))),
    );
  });

  test("is a card with its title linking to its page, its summary and cover, and no body", async () => {
    await renderApp({ path: "/local" });

    const feed = await screen.findByRole("feed", { name: "Local timeline" });
    const [local] = within(feed).getAllByRole("article");
    if (local === undefined) throw new Error("no article");
    const link = within(local).getByRole("link", { name: "Growing tomatoes" });
    expect(link).toHaveAttribute("href", `/@bob/status/${localArticle.id}`);
    expect(within(local).getByText("Article")).toBeInTheDocument();
    // The title and summary are user content (ADR-0009 §5).
    expect(within(local).getByText("Growing tomatoes")).toHaveAttribute("dir", "auto");
    expect(within(local).getByText("Notes from three summers.")).toHaveAttribute("dir", "auto");
    expect(within(local).getByRole("img", { name: "Tomatoes on a balcony" })).toBeInTheDocument();
    // The actions of a post, as for any post.
    expect(await within(local).findByRole("button", { name: "Favourite" })).toBeInTheDocument();
    // An article is not translated in a list, which carries no body.
    expect(within(local).queryByRole("button", { name: "Translate" })).not.toBeInTheDocument();
  });

  test("reads the same from another server, and names one without a title in the app's words", async () => {
    await renderApp({ path: "/local" });

    const feed = await screen.findByRole("feed", { name: "Local timeline" });
    const remote = within(feed).getAllByRole("article")[1];
    if (remote === undefined) throw new Error("no article");
    expect(within(remote).getByRole("link", { name: "Untitled article" })).toHaveAttribute(
      "href",
      `/@${remoteArticle.account.acct}/status/${remoteArticle.id}`,
    );
    const summary = within(remote).getByText("トマトの話です。");
    expect(summary).toHaveAttribute("lang", "ja");
    expect(summary).toHaveAttribute("dir", "auto");
  });

  test("folds its cover while it is sensitive, and shows its title and summary", async () => {
    server.use(
      http.get(`${apiBase}/timelines/local`, () =>
        HttpResponse.json(statusPage([{ ...localArticle, sensitive: true }])),
      ),
    );
    const user = userEvent.setup();
    await renderApp({ path: "/local" });

    const feed = await screen.findByRole("feed", { name: "Local timeline" });
    const article = within(feed).getByRole("article");
    expect(within(article).getByRole("link", { name: "Growing tomatoes" })).toBeInTheDocument();
    expect(within(article).getByText("Notes from three summers.")).toBeInTheDocument();
    expect(within(article).queryByRole("img")).not.toBeInTheDocument();
    await user.click(within(article).getByRole("button", { name: "Show sensitive media" }));
    expect(within(article).getByRole("img", { name: "Tomatoes on a balcony" })).toBeInTheDocument();
  });
});

describe("the page of an article", () => {
  function serve(status: Status) {
    server.use(http.get(`${apiBase}/statuses/:id`, () => HttpResponse.json(status)));
    return `/@${status.account.acct}/status/${status.id}`;
  }

  test("is headed by its title and shows its summary, cover and body", async () => {
    await renderApp({ path: serve(localArticleInFull) });

    expect(await screen.findByRole("heading", { level: 1, name: "Growing tomatoes" })).toBeInTheDocument();
    expect(screen.getByText("Notes from three summers.")).toHaveAttribute("dir", "auto");
    expect(screen.getByRole("img", { name: "Tomatoes on a balcony" })).toBeInTheDocument();
    // The body's headings start under the page's title.
    expect(screen.getByRole("heading", { level: 2, name: "Soil" })).toBeInTheDocument();
    expect(screen.getByText("Use a deep pot.")).toBeInTheDocument();
    expect(screen.getByRole("table")).toBeInTheDocument();
    // Replies, boosts and favourites, as for a post.
    expect(screen.getByRole("heading", { level: 2, name: "Replies" })).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Boost" })).toBeInTheDocument();
  });

  test("shows the title and summary of a sensitive article and folds its cover and body", async () => {
    const user = userEvent.setup();
    await renderApp({ path: serve({ ...localArticleInFull, sensitive: true }) });

    expect(await screen.findByRole("heading", { level: 1, name: "Growing tomatoes" })).toBeInTheDocument();
    expect(screen.getByText("Notes from three summers.")).toBeVisible();
    expect(screen.getByText("The author marked this article as sensitive.")).toBeInTheDocument();
    expect(screen.getByText("Use a deep pot.")).not.toBeVisible();
    const toggle = screen.getByRole("button", { name: "Show the article" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("Use a deep pot.")).toBeVisible();
    expect(screen.getByRole("img", { name: "Tomatoes on a balcony" })).toBeInTheDocument();
  });

  test("of another server without a title is headed in the app's words", async () => {
    await renderApp({ path: serve({ ...remoteArticle, content: "本文です。" }) });

    expect(await screen.findByRole("heading", { level: 1, name: "Untitled article" })).toBeInTheDocument();
    // The body is user content in the article's language (ADR-0009 §5); the body's text is right in its element.
    const body = screen.getByText("本文です。");
    expect(body).toHaveAttribute("lang", "ja");
    expect(body).toHaveAttribute("dir", "auto");
  });
});
