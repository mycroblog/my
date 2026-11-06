// SPDX-License-Identifier: AGPL-3.0-only
// The link card domains screen through the whole app (web-client/screens/admin-link-card-domains.md): an administrator
// with manage_settings sees the rules a page at a time and searches them, sets one on the form's page, sees why the
// server refused one, changes the mode of a rule on its page and removes one after asking; the first load fails in
// place and tries again; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { linkCardDomainRuleFixture } from "./test/handlers";

const path = "/admin/link-card-domains";
const newPath = "/admin/link-card-domains/new";

function answering(rules: Schemas["LinkCardDomainRule"][]) {
  return http.get(`${apiBase}/admin/link_card_domains`, () =>
    HttpResponse.json({ items: rules } satisfies Schemas["LinkCardDomainRuleList"]),
  );
}

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
  server.use(signedInHandler, operatorHandler(["manage_settings"]));
});

describe("the link card domains screen", () => {
  test("tells a viewer without the permission why it is empty", async () => {
    server.use(operatorHandler(["manage_webhooks"]));
    await renderApp({ path });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
  });

  test("says when there are no rules", async () => {
    await renderApp({ path });

    expect(await screen.findByRole("heading", { level: 3, name: "No rules yet" })).toBeInTheDocument();
  });

  test("lists the rules with their mode, linking to their page", async () => {
    server.use(
      answering([
        linkCardDomainRuleFixture("images.example", { note: "Heavy images" }),
        linkCardDomainRuleFixture("stop.example", { mode: "no_fetch" }),
      ]),
    );
    await renderApp({ path });
    const list = await screen.findByRole("region", { name: "Rules" });

    const [images, stop] = (await within(list).findAllByRole("row")).slice(1);
    if (images === undefined || stop === undefined) throw new Error("two rules expected");
    expect(within(images).getByRole("link", { name: "images.example" })).toHaveAttribute(
      "href",
      `${path}/images.example`,
    );
    expect(within(images).getByText("Stop images only")).toBeInTheDocument();
    expect(within(stop).getByText("Stop fetching")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add a rule" })).toHaveAttribute("href", newPath);
  });

  test("searches the rules by a part of the domain, and keeps the search in the URL", async () => {
    server.use(answering([linkCardDomainRuleFixture("images.example"), linkCardDomainRuleFixture("stop.example")]));
    const { user, router } = await renderApp({ path });
    const list = await screen.findByRole("region", { name: "Rules" });
    await within(list).findByRole("link", { name: "stop.example" });

    const search = within(list).getByRole("search", { name: "Search the rules" });
    await user.type(within(search).getByRole("searchbox", { name: "Domain contains" }), "IMAGES");
    await user.click(within(search).getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(within(list).queryByRole("link", { name: "stop.example" })).not.toBeInTheDocument();
    });
    expect(within(list).getByRole("link", { name: "images.example" })).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ domain: "IMAGES" });

    await user.click(within(list).getByRole("button", { name: "Clear search" }));
    expect(await within(list).findByRole("link", { name: "stop.example" })).toBeInTheDocument();
  });

  test("shows a long list one page of 40 rules at a time", async () => {
    server.use(
      answering(
        Array.from({ length: 1000 }, (_, index) =>
          linkCardDomainRuleFixture(`site-${String(index).padStart(4, "0")}.example`),
        ),
      ),
    );
    const { user } = await renderApp({ path });
    const list = await screen.findByRole("region", { name: "Rules" });

    // The header row and one page of rules.
    expect(await within(list).findAllByRole("row")).toHaveLength(41);
    expect(within(list).getByRole("link", { name: "site-0000.example" })).toBeInTheDocument();
    expect(within(list).queryByRole("link", { name: "site-0040.example" })).not.toBeInTheDocument();

    await user.click(within(list).getByRole("button", { name: "Next" }));
    expect(await within(list).findByRole("link", { name: "site-0040.example" })).toBeInTheDocument();
    expect(within(list).getAllByRole("row")).toHaveLength(41);
    expect(within(list).queryByRole("link", { name: "site-0000.example" })).not.toBeInTheDocument();
  });

  test("shows a failure of the first load in place, and tries again", async () => {
    let fail = true;
    server.use(
      http.get(`${apiBase}/admin/link_card_domains`, () =>
        fail
          ? HttpResponse.json(
              {
                type: "about:blank",
                title: "Server error",
                status: 500,
                detail: "The database is down",
              },
              { status: 500, headers: { "Content-Type": "application/problem+json" } },
            )
          : HttpResponse.json({
              items: [linkCardDomainRuleFixture("back.example")],
            } satisfies Schemas["LinkCardDomainRuleList"]),
      ),
    );
    const { user } = await renderApp({ path });

    expect(
      await screen.findByRole("heading", { level: 3, name: "Could not load the rules" }, { timeout: 5000 }),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Link card domains" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add a rule" })).toBeInTheDocument();

    fail = false;
    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByRole("link", { name: "back.example" })).toBeInTheDocument();
  });

  test("sets a rule, and opens its page", async () => {
    let sent: (Schemas["SetLinkCardDomainRuleRequest"] & { domain: string }) | undefined;
    server.use(
      http.put(`${apiBase}/admin/link_card_domains/:domain`, async ({ request, params }) => {
        const body = (await request.json()) as Schemas["SetLinkCardDomainRuleRequest"];
        sent = { domain: String(params["domain"]), ...body };
        return HttpResponse.json(linkCardDomainRuleFixture(sent.domain, { mode: sent.mode }));
      }),
      http.get(`${apiBase}/admin/link_card_domains`, () =>
        HttpResponse.json({
          items: sent === undefined ? [] : [linkCardDomainRuleFixture(sent.domain, { mode: sent.mode })],
        } satisfies Schemas["LinkCardDomainRuleList"]),
      ),
    );
    const { user, router } = await renderApp({ path: newPath });
    const domain = await screen.findByRole("textbox", { name: /Domain/u });

    await user.type(domain, " news.example ");
    await user.click(screen.getByRole("radio", { name: /Stop fetching/u }));
    await user.type(screen.getByRole("textbox", { name: /Note/u }), "Asked us to stop");
    await user.click(screen.getByRole("button", { name: "Save rule" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe(`${path}/news.example`);
    });
    expect(sent).toEqual({ domain: "news.example", mode: "no_fetch", note: "Asked us to stop" });
    expect(await screen.findByRole("region", { name: "news.example" })).toBeInTheDocument();
  });

  test("refuses a URL before sending it, next to the field", async () => {
    let posts = 0;
    server.use(
      http.put(`${apiBase}/admin/link_card_domains/:domain`, () => {
        posts += 1;
        return new HttpResponse(null, { status: 500 });
      }),
    );
    const { user } = await renderApp({ path: newPath });

    await user.type(await screen.findByRole("textbox", { name: /Domain/u }), "https://news.example/a");
    await user.click(screen.getByRole("button", { name: "Save rule" }));

    expect(await screen.findByText("Enter a host name only, without https:// or a path.")).toBeInTheDocument();
    expect(posts).toBe(0);
  });

  test("shows why the server refused the domain next to the field, and keeps the draft", async () => {
    server.use(
      http.put(`${apiBase}/admin/link_card_domains/:domain`, () =>
        HttpResponse.json(
          {
            type: "about:blank",
            title: "Unprocessable",
            status: 422,
            detail: "domain is not a valid value",
          },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: newPath });
    const domain = await screen.findByRole("textbox", { name: /Domain/u });

    await user.type(domain, "a..example");
    await user.click(screen.getByRole("button", { name: "Save rule" }));

    await waitFor(() => {
      expect(domain).toBeInvalid();
    });
    expect(domain).toHaveAccessibleDescription(expect.stringContaining("domain is not a valid value"));
    expect(domain).toHaveValue("a..example");
  });

  test("changes the mode and the note of a rule on its page", async () => {
    let sent: (Schemas["SetLinkCardDomainRuleRequest"] & { domain: string }) | undefined;
    server.use(
      answering([linkCardDomainRuleFixture("images.example", { note: "Heavy images" })]),
      http.put(`${apiBase}/admin/link_card_domains/:domain`, async ({ request, params }) => {
        const body = (await request.json()) as Schemas["SetLinkCardDomainRuleRequest"];
        sent = { domain: String(params["domain"]), ...body };
        return HttpResponse.json(linkCardDomainRuleFixture(sent.domain, { mode: sent.mode, note: sent.note ?? "" }));
      }),
    );
    const { user } = await renderApp({ path: `${path}/images.example` });

    const rule = await screen.findByRole("region", { name: "images.example" });
    expect(within(rule).getByRole("radio", { name: /Stop images only/u })).toBeChecked();
    await user.click(within(rule).getByRole("radio", { name: /Stop fetching/u }));
    await user.type(within(rule).getByRole("textbox", { name: /Note/u }), " and slow");
    await user.click(within(rule).getByRole("button", { name: "Save" }));

    // The result is next to the button (the note's counter is a status region too).
    expect(await within(rule).findByText(/The rule for .*images\.example.* is saved\./u)).toBeInTheDocument();
    expect(sent).toEqual({
      domain: "images.example",
      mode: "no_fetch",
      note: "Heavy images and slow",
    });
  });

  test("says when a domain has no rule", async () => {
    await renderApp({ path: `${path}/none.example` });

    expect(await screen.findByRole("heading", { name: /none\.example.* has no rule/u })).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: "Add a rule" }).length).toBeGreaterThan(0);
  });

  test("removes a rule after asking, and goes back to the list", async () => {
    let rules = [linkCardDomainRuleFixture("stop.example", { mode: "no_fetch" })];
    let removed: unknown;
    server.use(
      http.get(`${apiBase}/admin/link_card_domains`, () =>
        HttpResponse.json({ items: rules } satisfies Schemas["LinkCardDomainRuleList"]),
      ),
      http.delete(`${apiBase}/admin/link_card_domains/:domain`, ({ params }) => {
        removed = params["domain"];
        rules = [];
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user, router } = await renderApp({ path: `${path}/stop.example` });

    await user.click(await screen.findByRole("button", { name: /Remove the rule for .*stop\.example/u }));
    const dialog = await screen.findByRole("alertdialog");
    expect(removed).toBeUndefined();
    await user.click(within(dialog).getByRole("button", { name: "Remove" }));

    expect(await screen.findByRole("heading", { level: 3, name: "No rules yet" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe(path);
    expect(removed).toBe("stop.example");
    // The toast says what happened.
    expect(screen.getAllByText(/The rule for .*stop\.example.* is removed\./u).length).toBeGreaterThan(0);
  });
});
