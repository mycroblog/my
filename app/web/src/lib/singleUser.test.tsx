// SPDX-License-Identifier: AGPL-3.0-only
// A one-person server through the whole app (web-client/README.md T4, ADR-0152): the navigation and the menus offer
// nothing that the server does not, the app never asks for it, and the addresses of those screens are not found.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test } from "vitest";
import { instanceFixture, type Schemas, viewerFixture } from "../test/fixtures";
import { apiBase, operatorHandler, signedInHandler, withViewer } from "../test/msw/http";
import { server } from "../test/msw/server";
import { renderApp } from "../test/render";

const oneUser: Schemas["Instance"] = {
  ...instanceFixture,
  single_user: true,
  registrations: { mode: "closed" },
  timelines: { local: "off", federated: "off" },
};

/** The paths of the API that the app asked for. */
let asked: string[] = [];

beforeEach(() => {
  asked = [];
  server.events.on("request:start", ({ request }) => {
    asked.push(new URL(request.url).pathname.slice(apiBase.length));
  });
  server.use(
    signedInHandler,
    http.get(`${apiBase}/instance`, () => HttpResponse.json(oneUser)),
  );
});

afterEach(() => {
  server.events.removeAllListeners();
});

/** The paths of what a one-person server answers with a 404 that the app asked for. */
function askedCommunity(): string[] {
  return asked.filter((path) =>
    ["/invites", "/accounts/suggestions", "/admin/invites", "/admin/registrations", "/admin/signup_blocks"].some(
      (community) => path.startsWith(community),
    ),
  );
}

describe("a one-person server", () => {
  test("offers only the home timeline of the timelines in the navigation", async () => {
    await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });
    const navigation = screen.getByRole("navigation", { name: "Main" });
    expect(await within(navigation).findByRole("link", { name: "Home" })).toBeInTheDocument();
    expect(within(navigation).queryByRole("link", { name: "Local" })).not.toBeInTheDocument();
    expect(within(navigation).queryByRole("link", { name: "Federated" })).not.toBeInTheDocument();
  });

  test("suggests no accounts to a new user, and does not ask for them", async () => {
    server.use(
      http.get(`${apiBase}/accounts/verify_credentials`, ({ request }) =>
        withViewer(request, () => HttpResponse.json({ ...viewerFixture, following_count: 0 })),
      ),
    );
    await renderApp({ path: "/home" });
    expect(await screen.findByRole("heading", { name: "Get started" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Suggested accounts" })).not.toBeInTheDocument();
    // The checklist's follow step searches instead.
    expect(screen.getByRole("link", { name: "Find people" })).toHaveAttribute("href", "/search");
    expect(askedCommunity()).toEqual([]);
  });

  test("lists no invites in the settings, and does not ask for them", async () => {
    await renderApp({ path: "/settings" });
    expect(await screen.findByRole("link", { name: "Import and export" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Your invites" })).not.toBeInTheDocument();
    expect(askedCommunity()).toEqual([]);
  });

  test("lists no registrations, sign-up blocks or invites in the moderation menu", async () => {
    server.use(operatorHandler(["manage_accounts", "manage_invites", "manage_reports"]));
    await renderApp({ path: "/moderation" });
    const menu = await screen.findByRole("navigation", { name: "Admin and moderation" });
    expect(await within(menu).findByRole("link", { name: "Reports" })).toBeInTheDocument();
    expect(within(menu).getByRole("link", { name: "Accounts" })).toBeInTheDocument();
    for (const name of ["Registrations", "Sign-up blocks", "Invites"]) {
      expect(within(menu).queryByRole("link", { name })).not.toBeInTheDocument();
    }
    expect(askedCommunity()).toEqual([]);
  });

  test.each(["/settings/invites", "/moderation/invites", "/moderation/registrations", "/moderation/signup-blocks"])(
    "does not find %s, and does not ask the server for it",
    async (path) => {
      server.use(operatorHandler(["manage_accounts", "manage_invites"]));
      await renderApp({ path });
      expect(await screen.findByRole("heading", { level: 1, name: "Page not found" })).toBeInTheDocument();
      await waitFor(() => {
        expect(askedCommunity()).toEqual([]);
      });
    },
  );
});
