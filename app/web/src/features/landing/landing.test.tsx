// SPDX-License-Identifier: AGPL-3.0-only
// The landing page through the whole app (web-client/README.md T4; screens/landing.md): what the root address shows
// people who are not signed in, and that it sends signed-in users to their home timeline.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test, vi } from "vitest";
import type { Instance, InstanceOverview } from "./api/landing";
import { instanceFixture, statusPage, timelineStatusFixture } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { newTabName, visibleText } from "../../test/links";
import { overviewFixture } from "./test/handlers";

function serveInstance(overrides: Partial<Instance>) {
  server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, ...overrides })));
}

function serveOverview(overview: InstanceOverview) {
  server.use(http.get(`${apiBase}/instance/overview`, () => HttpResponse.json(overview)));
}

/** The names of the level-two headings of the page's main region, in order. */
function sectionTitles(): string[] {
  return within(screen.getByRole("main"))
    .getAllByRole("heading", { level: 2 })
    .map((heading) => heading.textContent);
}

/** Three recent posts, as many as the page asks for. */
const threePosts = [
  timelineStatusFixture,
  { ...timelineStatusFixture, id: "0192f0c4-7a8e-7000-8000-0000000000p2" },
  { ...timelineStatusFixture, id: "0192f0c4-7a8e-7000-8000-0000000000p3" },
];

describe("the landing page", () => {
  beforeEach(() => {
    // The progress line marks the section in view; jsdom has no layout, so nothing is marked.
    Element.prototype.scrollIntoView = vi.fn();
  });

  test("shows the server's masthead: its name, domain, summary, counts and who runs it, with signing in at the top", async () => {
    serveInstance({ short_description: "A small server for gardeners." });
    const { router } = await renderApp({ path: "/" });

    expect(await screen.findByRole("heading", { level: 1, name: "my example" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/");
    expect(await screen.findByText("A small server for gardeners.")).toHaveAttribute("dir", "auto");
    expect(screen.getByText("my.example")).toBeInTheDocument();
    expect(await screen.findByText("120 members")).toBeInTheDocument();
    expect(screen.getByText("Since March 2026")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Run by ⁨Olive⁩" })).toHaveAttribute("href", "#landing-operators");
    expect(screen.getByRole("button", { name: "Sign in" })).toBeInTheDocument();
    // The software is named only in the footer.
    expect(screen.queryByText(/What you can do/u)).not.toBeInTheDocument();
  });

  test("names the document after the server once, as its heading is the server's name", async () => {
    await renderApp({ path: "/" });

    await screen.findByRole("heading", { level: 1, name: "my example" });
    await waitFor(() => {
      expect(document.title).toBe("my example");
    });
  });

  test("leads through three sections under a progress line that links to each", async () => {
    serveInstance({
      description: "We grow vegetables.\n\nEveryone is welcome.",
      rules: [{ id: "0192f0c4-7a8e-7000-8000-0000000000r1", text: "Be kind", hint: "No insults." }],
    });
    const { user } = await renderApp({ path: "/" });
    await screen.findByText("We grow vegetables.");
    expect(sectionTitles()).toEqual(["What kind of place is this?", "What do we agree to?", "How do I join?"]);

    const progress = screen.getByRole("navigation", { name: "Parts of this page" });
    expect(within(progress).getAllByRole("link").map(visibleText)).toEqual([
      "01What kind of place is this?",
      "02What do we agree to?",
      "03How do I join?",
    ]);
    await user.click(within(progress).getByRole("link", { name: "What do we agree to?" }));
    const promise = screen.getByRole("region", { name: "What do we agree to?" });
    expect(within(promise).getByRole("heading", { level: 2 })).toHaveFocus();
    const rule = within(promise).getByRole("listitem");
    expect(rule).toHaveAttribute("lang", "");
    expect(rule).toHaveTextContent("Be kindNo insults.");

    const place = screen.getByRole("region", { name: "What kind of place is this?" });
    expect(
      within(place)
        .getAllByText(/./u, { selector: "p[dir=auto]" })
        .map((p) => p.textContent),
    ).toEqual(["We grow vegetables.", "Everyone is welcome."]);
  });

  test("lists who runs the server with their badges, and what people post with the numbers of the week", async () => {
    const limits: (string | null)[] = [];
    server.use(
      http.get(`${apiBase}/timelines/local`, ({ request }) => {
        limits.push(new URL(request.url).searchParams.get("limit"));
        return HttpResponse.json(statusPage(threePosts));
      }),
    );
    await renderApp({ path: "/" });
    const place = await screen.findByRole("region", { name: "What kind of place is this?" });

    const operators = await within(place).findByRole("list", { name: "Who runs it" });
    const [olive] = within(operators).getAllByRole("listitem");
    if (olive === undefined) throw new Error("no operator");
    expect(within(olive).getByRole("link", { name: "Olive @olive" })).toHaveAttribute("href", "/@olive");
    expect(within(within(olive).getByRole("list", { name: "Badges" })).getByText("Admin")).toBeInTheDocument();

    expect(within(place).getByText("Public posts in the last 7 days")).toBeInTheDocument();
    expect(within(place).getByText("35")).toBeInTheDocument();
    const feed = await within(place).findByRole("feed", { name: "What people post" });
    const articles = within(feed).getAllByRole("article");
    expect(articles).toHaveLength(3);
    // A sample to read: no actions on the posts before signing in.
    expect(within(articles[0] as HTMLElement).queryByRole("button", { name: /Boost|Favourite|Reply/u })).toBeNull();
    expect(limits).toEqual(["3"]);
    expect(within(place).getByRole("link", { name: "See the local timeline" })).toHaveAttribute("href", "/local");
  });

  test("shows a quiet server's description and operators without an empty feed or numbers", async () => {
    serveInstance({ description: "Just started." });
    serveOverview({
      ...overviewFixture,
      counts: {
        members: 1,
        daily_posts: [0, 0, 0, 0, 0, 0, 0],
        started_at: "2026-10-27T00:00:00Z",
        counted_at: "2026-10-28T00:00:00Z",
      },
    });
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([]))));
    await renderApp({ path: "/" });
    const place = await screen.findByRole("region", { name: "What kind of place is this?" });
    await within(place).findByRole("list", { name: "Who runs it" });
    expect(within(place).getByText("Just started.")).toBeInTheDocument();
    expect(within(place).queryByRole("heading", { name: "What people post" })).not.toBeInTheDocument();
    expect(within(place).queryByRole("feed")).not.toBeInTheDocument();
  });

  test("leaves out the counts and the operators that the server hides", async () => {
    serveOverview({});
    await renderApp({ path: "/" });
    await screen.findByRole("heading", { level: 1, name: "my example" });
    await screen.findByRole("feed", { name: "What people post" });
    expect(screen.queryByText(/members/u)).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Run by/u })).not.toBeInTheDocument();
    expect(screen.queryByRole("list", { name: "Who runs it" })).not.toBeInTheDocument();
    expect(screen.queryByText("Public posts in the last 7 days")).not.toBeInTheDocument();
  });

  test("joins through the steps of the approval mode, past the rules, to the server's registration page", async () => {
    serveInstance({ registrations: { mode: "approval" } });
    const { user, leftTo } = await renderApp({ path: "/" });
    // The masthead's primary action moves down to the section on how to join, not to the form.
    const join = await screen.findByRole("link", { name: "Join" });
    expect(join).toHaveAttribute("href", "#landing-join");
    await user.click(join);
    const section = screen.getByRole("region", { name: "How do I join?" });
    expect(within(section).getByRole("heading", { level: 2 })).toHaveFocus();

    const steps = within(section).getAllByRole("list")[0] as HTMLElement;
    expect(within(steps).getAllByRole("listitem").map(visibleText)).toEqual([
      "1Apply, saying why you want to join",
      "2A moderator reviews your account",
      "3You get an email once you are approved",
    ]);
    const apply = within(section).getByRole("link", { name: "Apply for an account" });
    expect(apply).toHaveAttribute("href", "http://localhost:3000/auth/sign_up");
    await user.click(apply);
    expect(leftTo.map(String)).toEqual(["http://localhost:3000/auth/sign_up"]);
  });

  test.each([
    {
      mode: "open",
      sendsEmail: true,
      action: "Create an account",
      steps: [
        "Enter a username, an email address and a password",
        "Confirm your email address from the mail you receive",
      ],
    },
    {
      mode: "open",
      sendsEmail: false,
      action: "Create an account",
      steps: ["Enter a username, an email address and a password", "A moderator reviews your account"],
    },
    {
      mode: "approval",
      sendsEmail: false,
      action: "Apply for an account",
      steps: ["Apply, saying why you want to join", "A moderator reviews your account"],
    },
    {
      mode: "invite",
      sendsEmail: true,
      action: "Create an account with an invite",
      steps: ["Get an invite link from someone on this server", "Open the link and create your account"],
    },
  ] as const)(
    "tells what happens after joining in the $mode mode (email: $sendsEmail)",
    async ({ mode, sendsEmail, action, steps }) => {
      serveInstance({ registrations: { mode }, sends_email: sendsEmail });
      await renderApp({ path: "/" });
      const section = await screen.findByRole("region", { name: "How do I join?" });
      await within(section).findByRole("link", { name: action });
      const list = within(section).getAllByRole("list")[0] as HTMLElement;
      const items = within(list).getAllByRole("listitem").map(visibleText);
      expect(items.slice(0, 2)).toEqual(steps.map((step, index) => `${String(index + 1)}${step}`));
    },
  );

  test("says that a closed server takes no new accounts, with signing in as the primary action", async () => {
    serveInstance({ registrations: { mode: "closed" } });
    await renderApp({ path: "/" });
    expect(await screen.findAllByText("This server is not accepting new accounts.")).toHaveLength(2);
    expect(screen.queryByRole("link", { name: "Join" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Create an account|Apply for an account/u })).not.toBeInTheDocument();
  });

  test("shows a visitor from another server how to follow from there, with a handle of this server", async () => {
    const { user } = await renderApp({ path: "/" });
    const link = await screen.findByRole("link", { name: "Have an account on another server? Follow from there" });
    await user.click(link);
    const heading = screen.getByRole("heading", { level: 3, name: "With an account on another server" });
    expect(heading).toHaveFocus();
    const steps = screen.getByRole("list", { name: "With an account on another server" });
    expect(await within(steps).findByText("Copy their handle, such as ⁦@olive@my.example⁩")).toBeInTheDocument();
    expect(within(steps).getByRole("link", { name: "See the local timeline" })).toHaveAttribute("href", "/local");
  });

  test("names the software only in the footer, with its source code, the information page and the documents", async () => {
    serveInstance({
      source_url: "https://git.example.org/fork/my",
      terms_url: "http://localhost:3000/about/terms",
      privacy_policy_url: "http://localhost:3000/about/privacy",
    });
    await renderApp({ path: "/" });
    const view = await screen.findByRole("link", { name: newTabName("View the source code") });
    expect(view).toHaveAttribute("href", "https://git.example.org/fork/my");
    const powered = screen.getByText("Powered by my, free software under the AGPL-3.0");
    const about = screen.getByRole("link", { name: "More about this server" });
    expect(about).toHaveAttribute("href", "/about");
    const documents = screen.getByRole("navigation", { name: "Terms and policies" });
    expect(within(documents).getAllByRole("link").map(visibleText)).toEqual(["Terms of service", "Privacy policy"]);
    // All of them come after the last section.
    const join = screen.getByRole("region", { name: "How do I join?" });
    for (const element of [about, documents, powered, view]) {
      expect(join.compareDocumentPosition(element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    }
  });

  test("links to an email that requests the source code", async () => {
    serveInstance({ source_url: "mailto:source@my.example" });
    await renderApp({ path: "/" });
    expect(await screen.findByRole("link", { name: "Request the source code by email" })).toHaveAttribute(
      "href",
      "mailto:source@my.example",
    );
  });

  test("shows the server's thumbnail in the masthead", async () => {
    serveInstance({
      thumbnail: { url: "https://my.example/media/beach/r/large.webp", width: 1600, height: 900, alt: "A beach" },
    });
    await renderApp({ path: "/" });
    await screen.findByRole("heading", { level: 1, name: "my example" });
    // The banner decorates, as a profile's does; the information page shows the image with its description.
    const banner = screen
      .getAllByRole("presentation")
      .find((image) => image.getAttribute("src") === "https://my.example/media/beach/r/large.webp");
    expect(banner).toHaveAttribute("alt", "");
  });

  test("does not ask for the recent posts when the local timeline is only for signed-in users", async () => {
    let asked = false;
    serveInstance({ timelines: { local: "logged_in", federated: "everyone" } });
    server.use(
      http.get(`${apiBase}/timelines/local`, () => {
        asked = true;
        return problem(401, "/api/v1/problems/unauthenticated", "Unauthenticated");
      }),
    );
    await renderApp({ path: "/" });
    await screen.findByText("Public posts in the last 7 days");
    expect(screen.queryByRole("feed")).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "See the local timeline" })).not.toBeInTheDocument();
    expect(asked).toBe(false);
  });

  test("shows the failure of the recent posts in their part and tries again", async () => {
    let fail = true;
    server.use(
      http.get(`${apiBase}/timelines/local`, () =>
        fail ? problem(500, "about:blank", "Internal Server Error") : HttpResponse.json(statusPage(threePosts)),
      ),
    );
    const { user } = await renderApp({ path: "/" });
    const place = await screen.findByRole("region", { name: "What kind of place is this?" });
    expect(
      await within(place).findByRole("heading", { level: 4, name: "Couldn't load the recent posts" }),
    ).toBeInTheDocument();
    fail = false;
    await user.click(within(place).getByRole("button", { name: "Try again" }));
    expect(await within(place).findByRole("feed", { name: "What people post" })).toBeInTheDocument();
  });

  test("still offers to sign in when the instance and the overview cannot be loaded", async () => {
    server.use(
      http.get(`${apiBase}/instance`, () => problem(500, "about:blank", "Internal Server Error")),
      http.get(`${apiBase}/instance/overview`, () => problem(500, "about:blank", "Internal Server Error")),
    );
    await renderApp({ path: "/" });
    expect(await screen.findByRole("heading", { level: 1, name: "my example" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign in" })).toBeInTheDocument();
  });

  test("sends signed-in users to their home timeline", async () => {
    server.use(signedInHandler);
    const { router } = await renderApp({ path: "/" });
    expect(await screen.findByRole("heading", { level: 1, name: "Home" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/home");
  });
});
