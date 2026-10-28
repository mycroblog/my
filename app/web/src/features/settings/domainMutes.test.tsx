// SPDX-License-Identifier: AGPL-3.0-only
// The muted domains page through the whole app (web-client/README.md T4): MSW plays the server's list of muted domains.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

beforeEach(() => {
  server.use(signedInHandler);
});

/** Plays a server that keeps the muted domains in order, and records what was muted and unmuted. */
function useMutedDomains(initial: string[]) {
  let domains = [...initial];
  const changes: string[] = [];
  server.use(
    http.get(`${apiBase}/domain_mutes`, () =>
      HttpResponse.json({
        domain_mutes: domains.map((domain): Schemas["DomainMute"] => ({ domain, created_at: "2026-10-21T09:30:00Z" })),
      } satisfies Schemas["DomainMutePage"]),
    ),
    http.post(`${apiBase}/domain_mutes`, async ({ request }) => {
      const domain = ((await request.json()) as Schemas["MuteDomainRequest"]).domain.toLowerCase();
      changes.push(`mute ${domain}`);
      domains = [...new Set([...domains, domain])].toSorted();
      return HttpResponse.json({ domain, created_at: "2026-10-21T09:30:00Z" } satisfies Schemas["DomainMute"]);
    }),
    http.delete(`${apiBase}/domain_mutes/:domain`, ({ params }) => {
      const domain = String(params["domain"]);
      changes.push(`unmute ${domain}`);
      domains = domains.filter((item) => item !== domain);
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return changes;
}

describe("muted domains", () => {
  test("opens from the settings, mutes a domain, and lists it in order", async () => {
    const changes = useMutedDomains(["xn--r8jz45g.xn--zckzah"]);
    const { user } = await renderApp({ path: "/settings" });

    await user.click(await screen.findByRole("link", { name: "Muted domains" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Muted domains" })).toBeInTheDocument();
    // A domain in IDNA A-labels shows the U-labels it stands for, and the A-labels too.
    const list = screen.getByRole("region", { name: "Domains you mute" });
    expect(within(list).getByText("例え.テスト")).toBeInTheDocument();
    expect(within(list).getByText("xn--r8jz45g.xn--zckzah")).toBeInTheDocument();

    const form = screen.getByRole("region", { name: "Mute a domain" });
    await user.type(within(form).getByRole("textbox", { name: "Domain" }), "Spam.Example");
    await user.click(within(form).getByRole("button", { name: "Mute" }));

    expect(await within(form).findByText("You muted ⁦spam.example⁩.")).toBeInTheDocument();
    expect(within(form).getByRole("textbox", { name: "Domain" })).toHaveValue("");
    expect(changes).toEqual(["mute spam.example"]);
    const rows = await within(list).findAllByRole("listitem");
    expect(rows.map((row) => row.getAttribute("data-row-id"))).toEqual(["spam.example", "xn--r8jz45g.xn--zckzah"]);
  });

  test("unmuting takes the domain off the list and keeps focus in the list", async () => {
    const changes = useMutedDomains(["a.example", "b.example"]);
    const { user } = await renderApp({ path: "/settings/domain-mutes" });

    await user.click(await screen.findByRole("button", { name: "Unmute: a.example" }));

    await waitFor(() => {
      expect(screen.queryByText("a.example")).not.toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Unmute: b.example" })).toHaveFocus();
    expect(await screen.findByText("You unmuted ⁦a.example⁩.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Unmute: b.example" }));
    expect(await screen.findByRole("heading", { level: 2, name: "You do not mute any domain" })).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByRole("heading", { level: 1 })).toHaveFocus();
    });
    expect(changes).toEqual(["unmute a.example", "unmute b.example"]);
  });

  test("a domain the server refuses says why and stays in the field", async () => {
    server.use(
      http.get(`${apiBase}/domain_mutes`, () => HttpResponse.json({ domain_mutes: [] })),
      http.post(`${apiBase}/domain_mutes`, () =>
        HttpResponse.json(
          { type: "about:blank", title: "Unprocessable", status: 422, detail: "domain is not a valid value." },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/settings/domain-mutes" });

    const field = await screen.findByRole("textbox", { name: "Domain" });
    await user.type(field, "not a domain");
    await user.click(screen.getByRole("button", { name: "Mute" }));

    expect(await screen.findByText("Could not mute the domain: domain is not a valid value.")).toHaveAttribute(
      "role",
      "alert",
    );
    expect(field).toHaveValue("not a domain");
  });
});
