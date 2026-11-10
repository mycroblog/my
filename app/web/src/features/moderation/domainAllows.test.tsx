// SPDX-License-Identifier: AGPL-3.0-only
// The allowlist of federation through the whole app (web-client/README.md T4, ADR-0159 §1): an operator with
// manage_domains reads the allowed domains, allows one (and one already allowed), and removes one after its alert
// dialog, with MSW playing the admin API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { domainAllowFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_domains"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type Allow = Schemas["DomainAllow"];

/** Plays the allowlist of the server; records the domains added and removed. */
function allowsServer(initial: Allow[]) {
  let allows = initial;
  const added: Schemas["CreateDomainAllowRequest"][] = [];
  const removed: string[] = [];
  server.use(
    http.get(`${apiBase}/admin/domain_allows`, () => HttpResponse.json({ items: allows })),
    http.post(`${apiBase}/admin/domain_allows`, async ({ request }) => {
      const body = (await request.json()) as Schemas["CreateDomainAllowRequest"];
      added.push(body);
      const existing = allows.find((allow) => allow.domain === body.domain);
      if (existing !== undefined) return HttpResponse.json(existing, { status: 200 });
      if (body.domain === "my.example")
        return problem(422, "about:blank", "This server's own domain cannot be allowed");
      const allow = domainAllowFixture({ domain: body.domain, comment: body.comment ?? "" });
      allows = [...allows, allow].toSorted((a, b) => a.domain.localeCompare(b.domain));
      return HttpResponse.json(allow, { status: 201 });
    }),
    http.delete(`${apiBase}/admin/domain_allows/:domain`, ({ params }) => {
      const domain = String(params["domain"]);
      removed.push(domain);
      allows = allows.filter((allow) => allow.domain !== domain);
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return { added, removed };
}

/** The rows of the table of allows, without its header row. */
function rows() {
  return within(screen.getByRole("table")).queryAllByRole("row").slice(1);
}

describe("the allowlist", () => {
  test("lists the allowed domains with their comments, in the domain actions screen's pages", async () => {
    allowsServer([
      domainAllowFixture({ domain: "friend.example", comment: "Partner server" }),
      domainAllowFixture({ domain: "xn--r8jz45g.jp" }),
    ]);
    await renderApp({ path: "/moderation/domain-actions/allowed" });

    expect(await screen.findByRole("heading", { level: 1, name: "Domain actions" })).toBeInTheDocument();
    const pages = screen.getByRole("navigation", { name: "Domain action pages" });
    expect(within(pages).getByRole("link", { name: "Allowed domains" })).toHaveAttribute("aria-current", "page");
    expect(within(pages).getByRole("link", { name: "Blocklist subscriptions" })).toHaveAttribute(
      "href",
      "/moderation/domain-actions/subscriptions",
    );
    expect(
      within(await screen.findByRole("table"))
        .getAllByRole("rowheader")
        .map((header) => header.textContent),
    ).toEqual(["friend.example", "例え.jp (xn--r8jz45g.jp)"]);
    const [first, second] = rows();
    if (first === undefined || second === undefined) throw new Error("two rows expected");
    expect(within(first).getByText("Partner server")).toHaveAttribute("dir", "auto");
    expect(within(second).getByText("None")).toBeInTheDocument();
    // The scope is a setting of manage_settings, which this viewer does not have.
    expect(screen.queryByRole("link", { name: "Change the federation scope" })).not.toBeInTheDocument();
  });

  test("allows a domain, and says so for one already allowed", async () => {
    const api = allowsServer([domainAllowFixture()]);
    const { user } = await renderApp({ path: "/moderation/domain-actions/allowed" });
    const form = await screen.findByRole("region", { name: "Allow a domain" });
    await user.type(within(form).getByRole("textbox", { name: "Domain" }), "partner.example");
    await user.type(within(form).getByRole("textbox", { name: "Comment" }), "Our sister server");
    await user.click(within(form).getByRole("button", { name: "Allow" }));

    expect(await screen.findByText("⁦partner.example⁩ is allowed.")).toBeInTheDocument();
    expect(api.added).toEqual([{ domain: "partner.example", comment: "Our sister server" }]);
    expect(within(form).getByRole("textbox", { name: "Domain" })).toHaveValue("");
    expect(rows().map((row) => within(row).getByRole("rowheader").textContent)).toEqual([
      "friend.example",
      "partner.example",
    ]);

    await user.type(within(form).getByRole("textbox", { name: "Domain" }), "friend.example");
    await user.click(within(form).getByRole("button", { name: "Allow" }));
    await waitFor(() => {
      expect(
        screen.getByText("⁦friend.example⁩ was already allowed; its allow and comment did not change."),
      ).toBeInTheDocument();
    });
    expect(api.added.at(-1)).toEqual({ domain: "friend.example" });
  });

  test("a refused domain stays in the form with the reason", async () => {
    allowsServer([]);
    const { user } = await renderApp({ path: "/moderation/domain-actions/allowed" });
    const form = await screen.findByRole("region", { name: "Allow a domain" });
    await user.type(within(form).getByRole("textbox", { name: "Domain" }), "my.example");
    await user.click(within(form).getByRole("button", { name: "Allow" }));

    expect(await within(form).findByRole("alert")).toHaveTextContent(/could not be allowed/);
    expect(within(form).getByRole("textbox", { name: "Domain" })).toHaveValue("my.example");
  });

  test("removes an allow after its alert dialog, which says what it cuts off, and keeps focus in the table", async () => {
    const api = allowsServer([
      domainAllowFixture({ domain: "a.example" }),
      domainAllowFixture({ domain: "b.example" }),
    ]);
    const { user } = await renderApp({ path: "/moderation/domain-actions/allowed" });
    await user.click(await screen.findByRole("button", { name: "Remove allow of ⁦a.example⁩" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Remove the allow of ⁦a.example⁩?" });
    expect(dialog).toHaveTextContent(/the follows between people here and their accounts end for good/);
    expect(api.removed).toEqual([]);
    await user.click(within(dialog).getByRole("button", { name: "Remove allow" }));

    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(api.removed).toEqual(["a.example"]);
    expect(await screen.findByText("The allow of ⁦a.example⁩ is removed.")).toBeInTheDocument();
    expect(rows()).toHaveLength(1);
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Remove allow of ⁦b.example⁩" })).toHaveFocus();
    });
  });
});
