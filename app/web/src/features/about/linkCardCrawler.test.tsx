// SPDX-License-Identifier: AGPL-3.0-only
// The page about the link card crawler through the whole app (web-client/README.md T4, web-client/screens/about-crawler.md):
// what the operator of a linked website reads at /about/crawler, signed out, by the settings of this server.
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import { instanceFixture, type Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

const heading = "Link card crawler of my example";
const obeys = "On this server, the crawler is on and it obeys robots.txt.";
const ignores = /^On this server, the crawler is on and it does not read robots\.txt/u;
const off = /^On this server, link cards are off, so the crawler fetches nothing now\./u;
const robotsIgnored =
  "This server does not read robots.txt, so this rule has no effect here. It applies on servers that obey it.";

function serveInstance(overrides: Partial<Schemas["Instance"]>) {
  server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, ...overrides })));
}

/** The preformatted text of a code sample, left to right in every locale. */
function codeBlock(text: RegExp) {
  return screen.getByText(text, { selector: 'pre[dir="ltr"] code' });
}

describe("the page about the link card crawler", () => {
  test("tells a signed-out visitor what the crawler is, what it does, and how to opt out", async () => {
    serveInstance({ contact: { email: "admin@my.example" } });
    await renderApp({ path: "/about/crawler" });

    expect(await screen.findByRole("heading", { level: 1, name: heading })).toBeInTheDocument();
    expect(await screen.findByText(obeys)).toBeInTheDocument();
    expect(codeBlock(/^my-linkcard\/0\.0\.0-test \(\+https:\/\/my\.example\/\)$/u)).toBeInTheDocument();

    const trigger = screen.getByRole("region", { name: "What causes a fetch" });
    expect(within(trigger).getByText(/^That includes posts that arrive from other servers/u)).toBeInTheDocument();

    const fetches = screen.getByRole("region", { name: "What it fetches" });
    expect(
      within(fetches)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual([
      expect.stringMatching(/^The page\. It reads only the head/u),
      "One image, the one that the head of the page names.",
      expect.stringMatching(/^robots\.txt of the origin/u),
    ]);

    const sends = screen.getByRole("region", { name: "What it sends" });
    expect(
      within(sends).getByText(
        "It sends no cookies and no Referer, and nothing tells you which account or post linked the page.",
      ),
    ).toBeInTheDocument();

    const often = screen.getByRole("region", { name: "How often" });
    expect(within(often).getByText(/14 days ago or more/u)).toBeInTheDocument();
    expect(within(often).getByText(/^robots\.txt is read about once a day per origin/u)).toBeInTheDocument();
    expect(within(often).getByText(/^It makes one request at a time to a host/u)).toBeInTheDocument();
    expect(within(often).getByText(/^When a host answers 429 or 503/u)).toBeInTheDocument();

    const optOut = screen.getByRole("region", { name: "How to opt out" });
    expect(within(optOut).queryByText(robotsIgnored)).not.toBeInTheDocument();
    expect(codeBlock(/^User-agent: my-linkcard\sDisallow: \/$/u).textContent).toBe(
      "User-agent: my-linkcard\nDisallow: /",
    );
    const rows = within(within(optOut).getByRole("table")).getAllByRole("row");
    expect(rows.map((row) => within(row).queryByRole("rowheader")?.textContent)).toEqual([
      undefined,
      "nosnippetmax-snippet:0",
      "noimageindexmax-image-preview:none",
      "noindexnone",
    ]);
    expect(rows[3]).toHaveTextContent(
      /No change\. These ask to stay out of search results.*To refuse, use robots\.txt\./u,
    );
    expect(within(optOut).getByText("nosnippet")).toHaveAttribute("dir", "ltr");

    const contact = screen.getByRole("region", { name: "Contact this server's operators" });
    expect(within(contact).getByText(/^The operators can stop the crawler on a domain/u)).toBeInTheDocument();
    expect(within(contact).getByRole("link", { name: "admin@my.example" })).toHaveAttribute(
      "href",
      "mailto:admin@my.example",
    );
  });

  test("shows the same to a signed-in user", async () => {
    server.use(signedInHandler);
    await renderApp({ path: "/about/crawler" });
    expect(await screen.findByRole("heading", { level: 1, name: heading })).toBeInTheDocument();
    expect(await screen.findByText(obeys)).toBeInTheDocument();
  });

  test("says that robots.txt does not count here on a server that ignores it", async () => {
    serveInstance({ link_cards_ignore_robots_txt: true });
    await renderApp({ path: "/about/crawler" });

    expect(await screen.findByText(ignores)).toBeInTheDocument();
    expect(screen.queryByText(obeys)).not.toBeInTheDocument();
    const optOut = screen.getByRole("region", { name: "How to opt out" });
    expect(within(optOut).getByText(robotsIgnored)).toBeInTheDocument();
    expect(within(optOut).getByRole("table")).toHaveTextContent(/To refuse on this server, ask its operators\./u);
    expect(within(optOut).getByRole("table")).not.toHaveTextContent(/use robots\.txt/u);
    // The crawler reads no robots.txt here, so the page does not say that it does.
    expect(screen.queryByText(/^robots\.txt of the origin/u)).not.toBeInTheDocument();
    expect(screen.queryByText(/^robots\.txt is read about once a day/u)).not.toBeInTheDocument();
    expect(screen.queryByText(/^The crawler obeys a new rule within a day/u)).not.toBeInTheDocument();
  });

  test("says that the crawler fetches nothing while link cards are off", async () => {
    serveInstance({ link_cards: false });
    await renderApp({ path: "/about/crawler" });

    expect(await screen.findByText(off)).toBeInTheDocument();
    expect(screen.queryByText(obeys)).not.toBeInTheDocument();
    expect(screen.queryByText(ignores)).not.toBeInTheDocument();
  });

  test("while link cards are off, still says that robots.txt would not count when the server ignores it", async () => {
    serveInstance({ link_cards: false, link_cards_ignore_robots_txt: true });
    await renderApp({ path: "/about/crawler" });

    expect(await screen.findByText(off)).toBeInTheDocument();
    expect(screen.getByText(robotsIgnored)).toBeInTheDocument();
  });

  test("says when the server publishes no contact, and links to its information page", async () => {
    await renderApp({ path: "/about/crawler" });
    const contact = await screen.findByRole("region", { name: "Contact this server's operators" });
    expect(within(contact).getByText("This server publishes no contact.")).toBeInTheDocument();
    expect(within(contact).getByRole("link", { name: "More about this server" })).toHaveAttribute("href", "/about");
  });

  test("keeps its heading and offers to try again when the settings cannot be read, and claims nothing meanwhile", async () => {
    let failing = true;
    server.use(
      http.get(`${apiBase}/instance`, () =>
        failing ? problem(500, "about:blank", "Internal Server Error") : HttpResponse.json(instanceFixture),
      ),
    );
    await renderApp({ path: "/about/crawler" });

    expect(
      await screen.findByRole("heading", { level: 2, name: "Could not load what the crawler does on this server" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: /^Link card crawler of / })).toBeInTheDocument();
    expect(screen.queryByText(obeys)).not.toBeInTheDocument();

    failing = false;
    const retry = screen.getByRole("button", { name: "Try again" });
    await userEvent.click(retry);
    await waitFor(() => {
      expect(screen.getByText(obeys)).toBeInTheDocument();
    });
  });

  test("is linked from the crawlers section of the information page", async () => {
    await renderApp({ path: "/about" });
    const crawlers = await screen.findByRole("region", { name: "Search engines and AI" });
    expect(
      within(crawlers).getByRole("link", { name: "What that crawler does, and how a website refuses it" }),
    ).toHaveAttribute("href", "/about/crawler");
  });
});
