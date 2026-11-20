// SPDX-License-Identifier: AGPL-3.0-only

// The server's about page and its documents, on the fake API, signed in and signed out: each section shows only where
// the server sets or shows it; the moderated servers under each of who may read them; a document published, not
// published, and failing; the link card crawler's page; the ways back to the about page, and the ways to it from the shell's
// footer, the landing page and a document. What each state looks like is the stories' (about.stories.tsx).

import { expect, test } from "bun:test";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, within } from "@testing-library/react";
import { main, renderApp } from "./test/app";
import { fakeApp } from "./test/server";

/** Every section the page can show. */
const everything: FakeOptions = {
  about: { says: "full", domains: "everyone", rationale: "everyone" },
  thumbnail: true,
  supporters: { tips: {}, list: "everyone" },
  finance: { costs: "totals", income: "totals", meter: "amounts" },
};

/** The names of the sections of the main column, in their order, once the server's head is in. */
async function sections(): Promise<string[]> {
  await main().findByRole("region", { name: "Server" });
  return main()
    .getAllByRole("region")
    .map((r) => r.getAttribute("aria-label") ?? "")
    .filter((name) => name !== "");
}

/** The section of the moderated servers, or null. */
const domains = () => main().queryByRole("region", { name: "Moderated servers" });

test("the about page shows every section the server sets, in order, and names the page after the server", async () => {
  const app = await renderApp("/about", everything);
  expect(await sections()).toEqual([
    "Contact",
    "Rules",
    "Terms and policies",
    "Moderated servers",
    "External review",
    "Automatic deletion",
    "Search engines and AI",
    "Costs and funding",
    "Support",
    "Server",
  ]);
  expect(main().getByRole("heading", { level: 1 }).textContent).toBe("About mycroblog demo");
  expect(app.title()).toBe("About mycroblog demo");
});

test("a server that says almost nothing shows only what it states to crawlers and its facts", async () => {
  await renderApp("/about", { about: { says: "bare" }, finance: { meter: "none" } });
  expect(await sections()).toEqual(["Search engines and AI", "Server"]);
  expect(main().queryByRole("link", { name: "Terms of service" })).toBeNull();
  expect(main().queryByRole("link", { name: "Privacy policy" })).toBeNull();
});

test("the about page reads the same for a visitor", async () => {
  await renderApp("/about", { ...everything, visitor: true });
  expect(await sections()).toContain("Moderated servers");
  expect(await sections()).toContain("Support");
});

for (const visitor of [false, true]) {
  test(`a supporters list for signed-in people is named ${visitor ? "to no visitor" : "to a signed-in reader"}`, async () => {
    await renderApp("/about", { supporters: { tips: {}, list: "signed_in" }, visitor });
    const support = await main().findByRole("region", { name: "Support" });
    expect(support.textContent.includes("With the supporters list")).toBe(!visitor);
  });
}

test("a supporters list for everyone is named to a visitor", async () => {
  await renderApp("/about", { supporters: { tips: {}, list: "everyone" }, visitor: true });
  const support = await main().findByRole("region", { name: "Support" });
  expect(support.textContent).toContain("With the supporters list");
});

for (const visitor of [false, true]) {
  const reader = visitor ? "a visitor" : "a signed-in reader";

  test(`a server that lists its moderated servers to nobody shows ${reader} no section of them`, async () => {
    await renderApp("/about", { about: { domains: "off" }, visitor });
    await sections();
    expect(domains()).toBeNull();
  });

  test(`a server that lists its moderated servers to everyone lists them to ${reader}, with the comments it shows them`, async () => {
    // The comments go to signed-in people alone: a visitor reads each domain and what was done, without why.
    await renderApp("/about", { about: { domains: "everyone", rationale: "signed_in" }, visitor });
    const list = await within(await main().findByRole("region", { name: "Moderated servers" })).findByRole("list", {
      name: "Moderated servers",
    });
    const items = within(list).getAllByRole("listitem");
    expect(items[0]?.textContent).toContain("spam.example");
    expect(items[0]?.textContent).toContain("Suspended");
    if (visitor) expect(items[0]?.textContent).not.toContain("Spam from new accounts every day.");
    else expect(items[0]?.textContent).toContain("Spam from new accounts every day.");
    // A domain moderated without a public comment shows what was done alone.
    const bots = items.find((item) => item.textContent.includes("bots.example"));
    expect(bots?.textContent.endsWith("Limited")).toBe(true);
  });
}

test("a server that lists its moderated servers to signed-in people lists them to a signed-in reader", async () => {
  await renderApp("/about", { about: { domains: "signed_in" } });
  const section = await main().findByRole("region", { name: "Moderated servers" });
  expect(await within(section).findByRole("list", { name: "Moderated servers" })).toBeDefined();
});

test("a visitor of a server that lists its moderated servers to signed-in people is asked to sign in, back here", async () => {
  const app = await renderApp("/about", { about: { domains: "signed_in" }, visitor: true });
  const section = await main().findByRole("region", { name: "Moderated servers" });
  expect(section.textContent).toContain("Only people signed in to this server can see which servers it moderates.");
  expect(within(section).queryByRole("list")).toBeNull();
  await app.user.click(within(section).getByRole("link", { name: "Sign in" }));
  await app.arrived("/login?return_to=%2Fabout");
});

test("the about page says that it loads until the instance answers", async () => {
  await renderApp("/about", { about: { instance: "waits" } });
  expect(await main().findByText("Loading the server's information…")).toBeDefined();
});

test("the about page says the instance could not be read, and Try again reads it", async () => {
  const on = fakeApp();
  on.fake.script("GET /instance", { fail: 500, times: 1 });
  const app = await renderApp("/about", { on });
  expect(await main().findByText("The server's information could not be loaded.")).toBeDefined();
  await app.user.click(main().getByRole("button", { name: "Try again" }));
  expect(await main().findByRole("region", { name: "Server" })).toBeDefined();
});

test("the crawler's page says the instance could not be read", async () => {
  await renderApp("/about/crawler", { about: { instance: "fails" } });
  expect(await main().findByText("The server's information could not be loaded.")).toBeDefined();
});

test("the about page opens a document, which leads back to it", async () => {
  const app = await renderApp("/about", { visitor: true });
  await sections();
  await app.user.click(main().getByRole("link", { name: "Terms of service" }));
  await app.arrived("/about/terms");
  expect(await main().findByRole("heading", { level: 2, name: "Terms of service" })).toBeDefined();
  expect(app.title()).toBe("Terms of service");
  await app.user.click(main().getByRole("button", { name: "About this server" }));
  await app.arrived("/about");
  expect(await main().findByRole("region", { name: "Server" })).toBeDefined();
});

for (const [path, title, says] of [
  ["/about/terms", "Terms of service", undefined],
  ["/about/privacy", "Privacy policy", undefined],
  ["/about/commerce", "Commerce disclosure", "full"],
] as const) {
  for (const visitor of [false, true]) {
    test(`${path} shows the published document, when it was revised, ${visitor ? "to a visitor" : "signed in"}`, async () => {
      const app = await renderApp(path, { visitor, ...(says === undefined ? {} : { about: { says } }) });
      const heading = await main().findByRole("heading", { level: 2, name: title });
      expect(heading.parentElement?.textContent).toContain("Last revised");
      expect(main().getByRole("heading", { level: 1 }).textContent).toBe(title);
      expect(app.title()).toBe(title);
    });
  }
}

test("the privacy policy keeps the server's table", async () => {
  await renderApp("/about/privacy");
  const table = await main().findByRole("table");
  expect(within(table).getByRole("cell", { name: "Sign-in addresses" })).toBeDefined();
});

test("a document the server has not published says so, with the one way to the about page", async () => {
  const app = await renderApp("/about/terms", { about: { says: "bare" }, visitor: true });
  expect(await main().findByText("This server has not published terms of service.")).toBeDefined();
  expect(main().queryByRole("button", { name: "About this server" })).toBeNull();
  await app.user.click(main().getByRole("link", { name: "About this server" }));
  await app.arrived("/about");
});

test("a commerce disclosure the server has not published says so", async () => {
  await renderApp("/about/commerce");
  expect(await main().findByText("This server has not published a commerce disclosure.")).toBeDefined();
});

test("a document that could not be read says so, and Try again reads it", async () => {
  const on = fakeApp();
  on.fake.script("GET /instance/documents/{kind}", { fail: 500, times: 1 });
  const app = await renderApp("/about/privacy", { on });
  expect(await main().findByText("The document could not be loaded.")).toBeDefined();
  await app.user.click(main().getByRole("button", { name: "Try again" }));
  expect(await main().findByRole("heading", { level: 2, name: "Privacy policy" })).toBeDefined();
});

test("the about page says what the server states to search engines and AI, and that it is no legal advice", async () => {
  await renderApp("/about", { crawlers: { search: "allow", ai: "deny_training" } });
  const section = await main().findByRole("region", { name: "Search engines and AI" });
  expect(section.textContent).toContain(
    "Search engines may index the public posts and profiles of people who allow it.",
  );
  expect(section.textContent).toContain("robots.txt");
  expect(section.textContent).toContain("This text is not legal advice.");
});

test("the about page leads to the link card crawler's page, which leads back", async () => {
  const app = await renderApp("/about", { visitor: true });
  await sections();
  await app.user.click(main().getByRole("link", { name: "What that crawler does, and how a website refuses it" }));
  await app.arrived("/about/crawler");
  expect(await main().findByRole("heading", { level: 2, name: "Link card crawler" })).toBeDefined();
  expect(app.title()).toBe("Link card crawler");
  await app.user.click(main().getByRole("button", { name: "About this server" }));
  await app.arrived("/about");
});

test("the crawler's page of a server that obeys robots.txt says so, names the crawler, and gives the contact", async () => {
  await renderApp("/about/crawler", { about: { says: "full", linkCards: "obeys" }, visitor: true });
  expect(await main().findByText("On this server, the crawler is on and it obeys robots.txt.")).toBeDefined();
  expect(main().getByText(/^my-linkcard\/0\.0\.0-demo \(\+https:\/\/.+\/\)$/)).toBeDefined();
  expect(
    main().getByText("User-agent: my-linkcard Disallow: /", { normalizer: (t) => t.replace(/\s+/g, " ") }),
  ).toBeDefined();
  expect(main().getByText(/^robots\.txt of the origin of the page/)).toBeDefined();
  const table = main().getByRole("table", { name: "What each directive changes in a card" });
  expect(within(table).getAllByRole("row")).toHaveLength(4);
  expect(main().getByRole("link", { name: "admin@example.invalid" })).toBeDefined();
  expect(main().queryByText("This server publishes no contact.")).toBeNull();
});

test("the crawler's page of a server that does not read robots.txt says a website cannot refuse it there", async () => {
  await renderApp("/about/crawler", { about: { linkCards: "ignores" } });
  expect(
    await main().findByText(
      /^On this server, the crawler is on and, by its administrators' choice, does not read robots\.txt\. A website cannot refuse it here with robots\.txt\./,
    ),
  ).toBeDefined();
  expect(main().getByText(/^This server does not read robots\.txt, so this rule has no effect here\./)).toBeDefined();
  expect(main().queryByText(/^robots\.txt of the origin of the page/)).toBeNull();
  expect(main().getByText("This server publishes no contact.")).toBeDefined();
});

test("the crawler's page of a server with link cards off says it fetches nothing now", async () => {
  await renderApp("/about/crawler");
  expect(await main().findByText(/^On this server, link cards are off/)).toBeDefined();
});

test("a visitor reaches the about page and the documents from the shell's footer", async () => {
  const app = await renderApp("/local", { visitor: true });
  const footer = () => within(screen.getAllByRole("navigation", { name: "This server" })[0] as HTMLElement);
  expect(footer().getByRole("link", { name: "Terms" }).getAttribute("href")).toBe("/about/terms");
  expect(footer().getByRole("link", { name: "Privacy" }).getAttribute("href")).toBe("/about/privacy");
  await app.user.click(footer().getByRole("link", { name: "About" }));
  await app.arrived("/about");
  expect(await main().findByRole("region", { name: "Server" })).toBeDefined();
  await app.user.click(footer().getByRole("link", { name: "Privacy" }));
  await app.arrived("/about/privacy");
});

test("the landing page leads a visitor to the about page, which the server serves as a page of its own", async () => {
  await renderApp("/", { visitor: true });
  const link = await screen.findByRole("link", { name: "More about this server" });
  expect(link.getAttribute("href")).toBe("/about");
});
