// SPDX-License-Identifier: AGPL-3.0-only
// The link card domains screen through the whole app (web-client/screens/admin-link-card-domains.md): an administrator
// with manage_settings sees the rules, sets one, sees why the server refused one, changes the mode of a rule in its row
// and removes one after asking; the first load fails in place and tries again; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { linkCardDomainRuleFixture } from "./test/handlers";

const path = "/admin/link-card-domains";

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

  test("lists the rules with their mode and note", async () => {
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
    expect(within(images).getByRole("rowheader", { name: /^images\.example/u })).toBeInTheDocument();
    expect(within(images).getByText("Heavy images")).toBeInTheDocument();
    expect(within(images).getAllByText("Stop images only").length).toBeGreaterThan(0);
    expect(within(stop).getAllByText("Stop fetching").length).toBeGreaterThan(0);
  });

  test("shows a failure of the first load in place, and tries again", async () => {
    let fail = true;
    server.use(
      http.get(`${apiBase}/admin/link_card_domains`, () =>
        fail
          ? HttpResponse.json(
              { type: "about:blank", title: "Server error", status: 500, detail: "The database is down" },
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
    expect(screen.getByRole("textbox", { name: /Domain/u })).toBeInTheDocument();

    fail = false;
    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByRole("rowheader", { name: /^back\.example/u })).toBeInTheDocument();
  });

  test("sets a rule, and empties the form", async () => {
    let sent: Schemas["SetLinkCardDomainRuleRequest"] | undefined;
    server.use(
      http.post(`${apiBase}/admin/link_card_domains`, async ({ request }) => {
        sent = (await request.json()) as Schemas["SetLinkCardDomainRuleRequest"];
        return HttpResponse.json(linkCardDomainRuleFixture(sent.domain, { mode: sent.mode }));
      }),
    );
    const { user } = await renderApp({ path });
    const domain = await screen.findByRole("textbox", { name: /Domain/u });

    await user.type(domain, " news.example ");
    await user.click(screen.getByRole("radio", { name: /Stop fetching/u }));
    await user.type(screen.getByRole("textbox", { name: /Note/u }), "Asked us to stop");
    await user.click(screen.getByRole("button", { name: "Save rule" }));

    expect(await screen.findByText(/The rule for .*news\.example.* is saved\./u)).toBeInTheDocument();
    expect(sent).toEqual({ domain: "news.example", mode: "no_fetch", note: "Asked us to stop" });
    expect(domain).toHaveValue("");
  });

  test("refuses a URL before sending it, next to the field", async () => {
    let posts = 0;
    server.use(
      http.post(`${apiBase}/admin/link_card_domains`, () => {
        posts += 1;
        return new HttpResponse(null, { status: 500 });
      }),
    );
    const { user } = await renderApp({ path });

    await user.type(await screen.findByRole("textbox", { name: /Domain/u }), "https://news.example/a");
    await user.click(screen.getByRole("button", { name: "Save rule" }));

    expect(await screen.findByText("Enter a host name only, without https:// or a path.")).toBeInTheDocument();
    expect(posts).toBe(0);
  });

  test("shows why the server refused the domain next to the field, and keeps the draft", async () => {
    server.use(
      http.post(`${apiBase}/admin/link_card_domains`, () =>
        HttpResponse.json(
          { type: "about:blank", title: "Unprocessable", status: 422, detail: "domain is not a valid value" },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path });
    const domain = await screen.findByRole("textbox", { name: /Domain/u });

    await user.type(domain, "a..example");
    await user.click(screen.getByRole("button", { name: "Save rule" }));

    await waitFor(() => {
      expect(domain).toBeInvalid();
    });
    expect(domain).toHaveAccessibleDescription(expect.stringContaining("domain is not a valid value"));
    expect(domain).toHaveValue("a..example");
  });

  test("changes the mode of a rule in its row, keeping its note", async () => {
    let sent: Schemas["SetLinkCardDomainRuleRequest"] | undefined;
    server.use(
      answering([linkCardDomainRuleFixture("images.example", { note: "Heavy images" })]),
      http.post(`${apiBase}/admin/link_card_domains`, async ({ request }) => {
        sent = (await request.json()) as Schemas["SetLinkCardDomainRuleRequest"];
        return HttpResponse.json(linkCardDomainRuleFixture(sent.domain, { mode: sent.mode, note: sent.note ?? "" }));
      }),
    );
    const { user } = await renderApp({ path });

    await user.click(await screen.findByRole("button", { name: /What to stop on .*images\.example/u }));
    await user.click(await screen.findByRole("option", { name: "Stop fetching" }));

    await waitFor(() => {
      expect(sent).toEqual({ domain: "images.example", mode: "no_fetch", note: "Heavy images" });
    });
  });

  test("removes a rule after asking", async () => {
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
    const { user } = await renderApp({ path });

    await user.click(await screen.findByRole("button", { name: /Remove the rule for .*stop\.example/u }));
    const dialog = await screen.findByRole("alertdialog");
    expect(removed).toBeUndefined();
    await user.click(within(dialog).getByRole("button", { name: "Remove" }));

    expect(await screen.findByRole("heading", { level: 3, name: "No rules yet" })).toBeInTheDocument();
    expect(removed).toBe("stop.example");
    expect(screen.getAllByText(/The rule for .*stop\.example.* is removed\./u).length).toBeGreaterThan(0);
  });
});
