// SPDX-License-Identifier: AGPL-3.0-only
// The landing page through the whole app (web-client/README.md T4): what the root address shows people who are not
// signed in, and that it sends signed-in users to their home timeline.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import type { Instance } from "./api/landing";
import { instanceFixture, statusPage, timelineStatusFixture } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { newTabName, visibleText } from "../../test/links";

function serveInstance(overrides: Partial<Instance>) {
  server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, ...overrides })));
}

/** Three recent posts: the hero shows two, the section below lists the third. */
const threePosts = [
  timelineStatusFixture,
  { ...timelineStatusFixture, id: "0192f0c4-7a8e-7000-8000-0000000000p2" },
  { ...timelineStatusFixture, id: "0192f0c4-7a8e-7000-8000-0000000000p3" },
];

describe("the landing page", () => {
  test("welcomes people who are not signed in with the server's description, rules, recent posts and documents", async () => {
    const limits: (string | null)[] = [];
    server.use(
      http.get(`${apiBase}/timelines/local`, ({ request }) => {
        limits.push(new URL(request.url).searchParams.get("limit"));
        return HttpResponse.json(statusPage(threePosts));
      }),
    );
    serveInstance({
      short_description: "A small server for gardeners.",
      description: "We grow vegetables.\n\nEveryone is welcome.",
      rules: [{ id: "0192f0c4-7a8e-7000-8000-0000000000r1", text: "Be kind", hint: "No insults." }],
      terms_url: "http://localhost:3000/about/terms",
      privacy_policy_url: "http://localhost:3000/about/privacy",
    });
    const { router } = await renderApp({ path: "/" });

    expect(await screen.findByRole("heading", { level: 1, name: "Welcome to my example" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/");
    expect(await screen.findByText("A small server for gardeners.")).toHaveAttribute("dir", "auto");
    expect(screen.getByRole("button", { name: "Sign in" })).toBeInTheDocument();

    const about = screen.getByRole("region", { name: "About this server" });
    expect(
      within(about)
        .getAllByText(/./u, { selector: "p" })
        .map((p) => p.textContent),
    ).toEqual(["We grow vegetables.", "Everyone is welcome."]);

    const rules = screen.getByRole("region", { name: "Rules" });
    const rule = within(rules).getByRole("listitem");
    expect(rule).toHaveAttribute("lang", "");
    expect(rule).toHaveTextContent("Be kindNo insults.");

    const posts = screen.getByRole("region", { name: "Recent posts" });
    const feed = await within(posts).findByRole("feed", { name: "Recent posts" });
    // The hero shows the first two posts; the section lists the rest.
    const article = within(feed).getByRole("article");
    // A sample to read: no actions on the posts before signing in.
    expect(within(article).queryByRole("button", { name: /Boost|Favourite|Reply/u })).not.toBeInTheDocument();
    expect(limits).toEqual(["4"]);
    expect(within(posts).getByRole("link", { name: "See the local timeline" })).toHaveAttribute("href", "/local");

    expect(screen.getByRole("link", { name: "More about this server" })).toHaveAttribute("href", "/about");
    const documents = screen.getByRole("navigation", { name: "Terms and policies" });
    expect(within(documents).getAllByRole("link").map(visibleText)).toEqual(["Terms of service", "Privacy policy"]);
  });

  test("orders its sections as the information page does, with the links to it and to the source code last", async () => {
    serveInstance({
      description: "We grow vegetables.",
      rules: [{ id: "0192f0c4-7a8e-7000-8000-0000000000r1", text: "Be kind" }],
      terms_url: "http://localhost:3000/about/terms",
      privacy_policy_url: "http://localhost:3000/about/privacy",
    });
    await renderApp({ path: "/" });

    await screen.findByRole("region", { name: "Terms and policies" });
    const main = screen.getByRole("main");
    expect(
      within(main)
        .getAllByRole("region")
        .map((region) => within(region).getByRole("heading", { level: 2 }).textContent),
    ).toEqual(["What you can do with my", "About this server", "Recent posts", "Rules", "Terms and policies"]);
    const documents = within(main).getByRole("region", { name: "Terms and policies" });
    const more = within(main).getByRole("link", { name: "More about this server" });
    const source = within(main).getByRole("link", { name: newTabName("View the source code") });
    expect(documents.compareDocumentPosition(more) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(more.compareDocumentPosition(source) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  test("links to the source code the server runs, or to an email that requests it", async () => {
    serveInstance({ source_url: "https://git.example.org/fork/my" });
    const { unmount } = await renderApp({ path: "/" });
    const view = await screen.findByRole("link", { name: newTabName("View the source code") });
    expect(view).toHaveAttribute("href", "https://git.example.org/fork/my");
    expect(view).toHaveAttribute("target", "_blank");
    expect(view).toHaveAttribute("rel", "noopener noreferrer");
    unmount();

    serveInstance({ source_url: "mailto:source@my.example" });
    await renderApp({ path: "/" });
    expect(await screen.findByRole("link", { name: "Request the source code by email" })).toHaveAttribute(
      "href",
      "mailto:source@my.example",
    );
  });

  test("shows the server's thumbnail above its summary", async () => {
    serveInstance({
      short_description: "A small server for gardeners.",
      thumbnail: {
        url: "https://my.example/media/beach/r/large.webp",
        width: 1600,
        height: 900,
        alt: "A beach at dawn",
      },
    });
    await renderApp({ path: "/" });
    expect(await screen.findByRole("img", { name: "A beach at dawn" })).toHaveAttribute(
      "src",
      "https://my.example/media/beach/r/large.webp",
    );
  });

  test("links to the server's registration page, which loads in place of the app", async () => {
    serveInstance({ registrations: { mode: "approval" } });
    const { user, leftTo } = await renderApp({ path: "/" });
    const apply = await screen.findByRole("link", { name: "Apply for an account" });
    expect(apply).toHaveAttribute("href", "http://localhost:3000/auth/sign_up");
    expect(apply).toHaveAccessibleDescription("Moderators review each new account before it can be used.");
    await user.click(apply);
    expect(leftTo.map(String)).toEqual(["http://localhost:3000/auth/sign_up"]);
  });

  test.each([
    { mode: "open", name: "Create an account", description: "" },
    {
      mode: "invite",
      name: "Create an account with an invite",
      description: "Only people with an invite can create an account.",
    },
  ] as const)("names the registration link after the $mode mode", async ({ mode, name, description }) => {
    serveInstance({ registrations: { mode } });
    await renderApp({ path: "/" });
    expect(await screen.findByRole("link", { name })).toHaveAccessibleDescription(description);
  });

  test("says that a closed server takes no new accounts, without a registration link", async () => {
    serveInstance({ registrations: { mode: "closed" } });
    await renderApp({ path: "/" });
    expect(await screen.findByText("This server is not accepting new accounts.")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /account/u })).not.toBeInTheDocument();
  });

  test("leaves out the recent posts when the server does not serve the local timeline, and its empty sections", async () => {
    serveInstance({ timelines: { local: "off", federated: "everyone" } });
    await renderApp({ path: "/" });
    await screen.findByRole("link", { name: "Apply for an account" });
    expect(screen.queryByRole("region", { name: "Recent posts" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "About this server" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Rules" })).not.toBeInTheDocument();
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
    await screen.findByRole("link", { name: "Apply for an account" });
    expect(screen.queryByRole("region", { name: "Recent posts" })).not.toBeInTheDocument();
    expect(asked).toBe(false);
  });

  test("leaves the section to its heading and link when the hero shows every recent post", async () => {
    server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([timelineStatusFixture]))));
    await renderApp({ path: "/" });
    const posts = await screen.findByRole("region", { name: "Recent posts" });
    expect(within(posts).queryByRole("feed")).not.toBeInTheDocument();
    expect(within(posts).getByRole("link", { name: "See the local timeline" })).toBeInTheDocument();
  });

  test("shows the failure of the recent posts in their section and tries again", async () => {
    let fail = true;
    server.use(
      http.get(`${apiBase}/timelines/local`, () =>
        fail ? problem(500, "about:blank", "Internal Server Error") : HttpResponse.json(statusPage(threePosts)),
      ),
    );
    const { user } = await renderApp({ path: "/" });
    const posts = await screen.findByRole("region", { name: "Recent posts" });
    expect(
      await within(posts).findByRole("heading", { level: 3, name: "Couldn't load the recent posts" }),
    ).toBeInTheDocument();
    fail = false;
    await user.click(within(posts).getByRole("button", { name: "Try again" }));
    expect(await within(posts).findByRole("feed", { name: "Recent posts" })).toBeInTheDocument();
  });

  test("still offers to sign in when the instance cannot be loaded", async () => {
    server.use(http.get(`${apiBase}/instance`, () => problem(500, "about:blank", "Internal Server Error")));
    await renderApp({ path: "/" });
    expect(await screen.findByRole("heading", { level: 1, name: "Welcome to my example" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign in" })).toBeInTheDocument();
  });

  test("sends signed-in users to their home timeline", async () => {
    server.use(signedInHandler);
    const { router } = await renderApp({ path: "/" });
    expect(await screen.findByRole("heading", { level: 1, name: "Home" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/home");
  });
});
