// SPDX-License-Identifier: AGPL-3.0-only
// The instance's information page through the whole app (web-client/README.md T4, FR-718): what everyone may read about
// the server at /about, and the moderated domains as far as the instance's settings list them.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test, vi } from "vitest";
import type { Instance } from "./api/about";
import { instanceFixture, otherAccountFixture } from "../../test/fixtures";
import { apiBase, problem, signedInHandler, withViewer } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { publicDomainActionsFixture } from "./test/handlers";
import { newTabName } from "../../test/links";

function serveInstance(overrides: Partial<Instance>) {
  server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, ...overrides })));
}

describe("the information page", () => {
  test("tells everyone about the server: its description, rules, moderated servers and documents", async () => {
    serveInstance({
      short_description: "A small server for gardeners.",
      description: "We grow vegetables.\n\nEveryone is welcome.",
      rules: [{ id: "0192f0c4-7a8e-7000-8000-0000000000r1", text: "Be kind", hint: "No insults." }],
      terms_url: "http://localhost:3000/about/terms",
      privacy_policy_url: "http://localhost:3000/about/privacy",
    });
    await renderApp({ path: "/about" });

    expect(await screen.findByRole("heading", { level: 1, name: "About my example" })).toBeInTheDocument();
    expect(await screen.findByText("A small server for gardeners.")).toHaveAttribute("dir", "auto");

    const description = screen.getByRole("region", { name: "Description" });
    expect(
      within(description)
        .getAllByText(/./u, { selector: "p" })
        .map((p) => p.textContent),
    ).toEqual(["We grow vegetables.", "Everyone is welcome."]);

    const rule = within(screen.getByRole("region", { name: "Rules" })).getByRole("listitem");
    expect(rule).toHaveAttribute("lang", "");
    expect(rule).toHaveTextContent("Be kindNo insults.");

    const domains = screen.getByRole("region", { name: "Moderated servers" });
    const items = within(domains).getAllByRole("listitem");
    expect(items.map((item) => item.textContent)).toEqual([
      "spam.exampleSuspended: no federation at allSpam and harassment",
      "bad.*****leLimited: its posts stay off the public timelines",
    ]);
    expect(within(items[0] ?? domains).getByText("Spam and harassment")).toHaveAttribute("dir", "auto");
    // The moderators hid part of a name: the page says so.
    expect(within(domains).getByText("Some server names are partly hidden with *.")).toBeInTheDocument();

    const documents = screen.getByRole("region", { name: "Terms and policies" });
    expect(
      within(documents)
        .getAllByRole("link")
        .map((link) => link.getAttribute("href")),
    ).toEqual(["/about/terms", "/about/privacy"]);
  });

  test("puts the terms and policies right after the rules, then the operators, and the source code and the licenses last", async () => {
    serveInstance({
      description: "We grow vegetables.",
      rules: [{ id: "0192f0c4-7a8e-7000-8000-0000000000r1", text: "Be kind" }],
      contact: { email: "admin@my.example" },
      terms_url: "http://localhost:3000/about/terms",
      privacy_policy_url: "http://localhost:3000/about/privacy",
    });
    await renderApp({ path: "/about" });

    await screen.findByRole("region", { name: "Moderated servers" });
    const main = screen.getByRole("main");
    expect(
      within(main)
        .getAllByRole("region")
        .map((region) => within(region).getByRole("heading", { level: 2 }).textContent),
    ).toEqual([
      "Description",
      "Rules",
      "Terms and policies",
      "Contact",
      "Moderated servers",
      "Search engines and AI",
      "Source code",
      "Licenses",
    ]);
  });

  test("shows the server's thumbnail and how to reach its operators", async () => {
    serveInstance({
      thumbnail: {
        url: "https://my.example/media/beach/r/large.webp",
        width: 1600,
        height: 900,
        alt: "A beach at dawn",
      },
      contact: { account: otherAccountFixture, email: "admin@my.example" },
    });
    await renderApp({ path: "/about" });

    expect(await screen.findByRole("img", { name: "A beach at dawn" })).toHaveAttribute(
      "src",
      "https://my.example/media/beach/r/large.webp",
    );
    const contact = screen.getByRole("region", { name: "Contact" });
    expect(within(contact).getByRole("link", { name: "Bob @bob" })).toHaveAttribute("href", "/@bob");
    expect(within(contact).getByRole("link", { name: "admin@my.example" })).toHaveAttribute(
      "href",
      "mailto:admin@my.example",
    );
  });

  test("leaves out the contact and the thumbnail when the server has none", async () => {
    await renderApp({ path: "/about" });
    expect(await screen.findByRole("heading", { level: 1, name: "About my example" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Contact" })).not.toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });

  test("says where to get the source code the server runs, and why everyone may", async () => {
    serveInstance({ source_url: "https://git.example.org/fork/my" });
    await renderApp({ path: "/about" });
    const source = await screen.findByRole("region", { name: "Source code" });
    expect(within(source).getByText(/GNU Affero General Public License/u)).toBeInTheDocument();
    const link = within(source).getByRole("link", { name: newTabName("View the source code") });
    expect(link).toHaveAttribute("href", "https://git.example.org/fork/my");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    expect(within(source).queryByRole("term")).not.toBeInTheDocument();
  });

  test("links to the server's status page in a new tab, before the source code, when the server has one (FR-810)", async () => {
    serveInstance({ contact: { email: "admin@my.example" }, status_page_url: "https://status.example/" });
    await renderApp({ path: "/about" });
    const status = await screen.findByRole("region", { name: "Server status" });
    const link = within(status).getByRole("link", { name: newTabName("View the status page") });
    expect(link).toHaveAttribute("href", "https://status.example/");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    const headings = within(screen.getByRole("main"))
      .getAllByRole("heading", { level: 2 })
      .map((heading) => heading.textContent);
    expect(headings.slice(headings.indexOf("Server status"), headings.indexOf("Server status") + 2)).toEqual([
      "Server status",
      "Source code",
    ]);
  });

  test("has no status section when the server has no status page", async () => {
    serveInstance({});
    await renderApp({ path: "/about" });
    await screen.findByRole("region", { name: "Source code" });
    expect(screen.queryByRole("region", { name: "Server status" })).not.toBeInTheDocument();
  });

  test("links to the license notices of what the web app bundles, which load in place of the app", async () => {
    const { user, leftTo } = await renderApp({ path: "/about" });
    const licenses = await screen.findByRole("region", { name: "Licenses" });
    expect(
      within(licenses)
        .getAllByRole("link")
        .map((link) => [link.textContent, link.getAttribute("href")]),
    ).toEqual([
      ["npm packages in the app", "/licenses/npm-packages.md"],
      ["npm packages in the offline support (Service Worker)", "/licenses/npm-packages-sw.md"],
      ["Unicode emoji data and CLDR emoji names (Unicode License v3)", "/licenses/LICENSE-Unicode.txt"],
      ["Noto Color Emoji font (SIL Open Font License 1.1)", "/licenses/OFL.txt"],
    ]);
    await user.click(within(licenses).getByRole("link", { name: /Unicode License v3/u }));
    expect(leftTo.map(String)).toEqual(["http://localhost:3000/licenses/LICENSE-Unicode.txt"]);
  });

  test("offers to request the source code by email when the server takes requests at an address", async () => {
    serveInstance({ source_url: "mailto:source%2Bmy@my.example?subject=Source" });
    await renderApp({ path: "/about" });
    const source = await screen.findByRole("region", { name: "Source code" });
    expect(within(source).getByRole("link", { name: "Request the source code by email" })).toHaveAttribute(
      "href",
      "mailto:source%2Bmy@my.example?subject=Source",
    );
    expect(within(source).getByRole("definition")).toHaveTextContent("source+my@my.example");
    expect(within(source).getByRole("definition")).toHaveAttribute("dir", "ltr");
  });

  test("says what the server states to search engines and AI, and what such a statement means", async () => {
    serveInstance({ search_indexing: "allow", ai_usage: "deny_training" });
    await renderApp({ path: "/about" });
    const crawlers = await screen.findByRole("region", { name: "Search engines and AI" });
    expect(
      within(crawlers).getByText(
        "Search engines may index the profiles and public posts of the accounts that allow it.",
      ),
    ).toBeInTheDocument();
    expect(
      within(crawlers).getByText(/^This server reserves its pages against the training of AI models\. AI search/u),
    ).toBeInTheDocument();
    expect(within(crawlers).getByText(/Article 30-4 of the Copyright Act/u)).toBeInTheDocument();
  });

  test("says when the server denies search engines and every use by AI", async () => {
    serveInstance({ search_indexing: "deny", ai_usage: "deny" });
    await renderApp({ path: "/about" });
    const crawlers = await screen.findByRole("region", { name: "Search engines and AI" });
    expect(
      within(crawlers).getByText("This server asks search engines not to index any of its pages."),
    ).toBeInTheDocument();
    expect(
      within(crawlers).getByText(
        "This server reserves its pages against the training of AI models and against their use by AI search, assistants and agents.",
      ),
    ).toBeInTheDocument();
  });

  test("leaves out the moderated servers when the server lists them to nobody", async () => {
    server.use(http.get(`${apiBase}/instance/domain_actions`, () => problem(404, "about:blank", "Not Found")));
    await renderApp({ path: "/about" });
    expect(await screen.findByRole("heading", { level: 1, name: "About my example" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Moderated servers" })).not.toBeInTheDocument();
  });

  test("asks visitors to sign in when the server lists the moderated servers to its users only", async () => {
    let requests = 0;
    server.use(
      http.get(`${apiBase}/instance/domain_actions`, ({ request }) => {
        requests++;
        return withViewer(request, () => HttpResponse.json({ items: publicDomainActionsFixture }));
      }),
    );
    const view = await renderApp({ path: "/about" });
    const domains = await screen.findByRole("region", { name: "Moderated servers" });
    expect(within(domains).getByText("Only signed-in users can see this list.")).toBeInTheDocument();
    expect(within(domains).getByRole("link", { name: "Sign in" })).toHaveAttribute("href", "/login?return_to=%2Fabout");
    expect(within(domains).queryByRole("list")).not.toBeInTheDocument();
    // The answer is the server's setting: it is not asked again.
    expect(requests).toBe(1);
    view.unmount();

    server.use(signedInHandler);
    await renderApp({ path: "/about" });
    const signedIn = await screen.findByRole("region", { name: "Moderated servers" });
    expect(within(signedIn).getAllByRole("listitem")).toHaveLength(2);
  });

  test("says when no servers are moderated, without the note on hidden names", async () => {
    server.use(http.get(`${apiBase}/instance/domain_actions`, () => HttpResponse.json({ items: [] })));
    await renderApp({ path: "/about" });
    const domains = await screen.findByRole("region", { name: "Moderated servers" });
    expect(within(domains).getByRole("heading", { level: 3, name: "No servers are moderated" })).toBeInTheDocument();
    expect(within(domains).queryByText("Some server names are partly hidden with *.")).not.toBeInTheDocument();
  });

  test("loads more moderated servers a page at a time", async () => {
    const cursors: (string | null)[] = [];
    server.use(
      http.get(`${apiBase}/instance/domain_actions`, ({ request }) => {
        const cursor = new URL(request.url).searchParams.get("cursor");
        cursors.push(cursor);
        const [first, second] = publicDomainActionsFixture;
        return HttpResponse.json(cursor === null ? { items: [first], next_cursor: "next" } : { items: [second] });
      }),
    );
    const { user } = await renderApp({ path: "/about" });
    const domains = await screen.findByRole("region", { name: "Moderated servers" });
    expect(within(domains).getAllByRole("listitem")).toHaveLength(1);
    await user.click(within(domains).getByRole("button", { name: "Show more" }));
    expect(await within(domains).findByText("bad.*****le")).toBeInTheDocument();
    expect(within(domains).queryByRole("button", { name: "Show more" })).not.toBeInTheDocument();
    expect(cursors).toEqual([null, "next"]);
  });

  test("shows the failure of the moderated servers in their section and tries again", async () => {
    let fail = true;
    server.use(
      http.get(`${apiBase}/instance/domain_actions`, () =>
        fail
          ? problem(500, "about:blank", "Internal Server Error")
          : HttpResponse.json({ items: publicDomainActionsFixture }),
      ),
    );
    const { user } = await renderApp({ path: "/about" });
    const domains = await screen.findByRole("region", { name: "Moderated servers" });
    expect(
      await within(domains).findByRole("heading", { level: 3, name: "Couldn't load the moderated servers" }),
    ).toBeInTheDocument();
    fail = false;
    await user.click(within(domains).getByRole("button", { name: "Try again" }));
    expect(await within(domains).findByText("spam.example")).toBeInTheDocument();
  });

  test("is linked from the column about the server on wide screens", async () => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query === "(min-width: 80rem)",
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }));
    server.use(signedInHandler);
    await renderApp({ path: "/home" });
    const aside = await screen.findByRole("complementary", { name: "About this server" });
    expect(within(aside).getByRole("link", { name: "More about this server" })).toHaveAttribute("href", "/about");
  });

  test("is linked from the footer of signed-out pages, which have no column about the server at any width", async () => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query === "(min-width: 80rem)",
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }));
    server.use(
      http.get(`${apiBase}/instance`, () =>
        HttpResponse.json({
          ...instanceFixture,
          source_url: "https://git.example.org/fork/my",
          terms_url: "http://localhost:3000/about/terms",
        }),
      ),
    );
    await renderApp({ path: "/local" });
    const footer = await screen.findByRole("contentinfo");
    expect(within(footer).getByRole("link", { name: "More about this server" })).toHaveAttribute("href", "/about");
    expect(await within(footer).findByRole("link", { name: "Terms of service" })).toBeInTheDocument();
    expect(within(footer).getByRole("link", { name: newTabName("View the source code") })).toHaveAttribute(
      "href",
      "https://git.example.org/fork/my",
    );
    expect(screen.queryByRole("complementary")).not.toBeInTheDocument();
  });
});
