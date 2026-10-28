// SPDX-License-Identifier: AGPL-3.0-only
// The authorized apps page through the whole app (web-client/README.md T4): the list with where each app comes from,
// its scopes and times (the last use to the hour), revoking one after its dialog, a failure that keeps the dialog
// open, and revoking the web client, which signs this browser out; with MSW playing the server.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

const web: Schemas["AuthorizedApp"] = {
  id: "019a0000-0000-7000-8000-0000000000c1",
  client_id: "my-web",
  name: "Web",
  registration: "first_party",
  first_party: true,
  scope: "read write follow",
  authorized_at: "2026-10-01T09:00:00Z",
  last_used_at: "2026-10-21T09:00:00Z",
};
const reader: Schemas["AuthorizedApp"] = {
  id: "019a0000-0000-7000-8000-0000000000c2",
  client_id: "https://reader.example/client.json",
  name: "Reader",
  registration: "metadata_document",
  first_party: false,
  scope: "read:statuses",
  authorized_at: "2026-10-10T12:00:00Z",
};
const bot: Schemas["AuthorizedApp"] = {
  id: "019a0000-0000-7000-8000-0000000000c3",
  client_id: "my_ci_bot",
  name: "Bot",
  registration: "user",
  registered_by: "alice",
  first_party: false,
  scope: "write:statuses",
  authorized_at: "2026-10-12T12:00:00Z",
  last_used_at: "2026-10-20T08:00:00Z",
};
const scribe: Schemas["AuthorizedApp"] = {
  id: "019a0000-0000-7000-8000-0000000000c4",
  client_id: "my_ci_scribe",
  name: "Scribe",
  registration: "dynamic",
  first_party: false,
  scope: "read",
  authorized_at: "2026-10-13T12:00:00Z",
};

/** The server's authorized apps of the viewer, which revoking changes, with the IDs it was asked to revoke. */
function appServer(initial: Schemas["AuthorizedApp"][], fail = false) {
  let items = initial;
  const revoked: string[] = [];
  server.use(
    http.get(`${apiBase}/authorized_apps`, () => HttpResponse.json({ items })),
    http.delete(`${apiBase}/authorized_apps/:id`, ({ params }) => {
      if (fail) return problem(500, "about:blank", "Internal Server Error");
      revoked.push(String(params["id"]));
      items = items.filter((app) => app.id !== params["id"]);
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return revoked;
}

describe("authorized apps", () => {
  test("are in the settings menu", async () => {
    await renderApp({ path: "/settings" });
    const link = await screen.findByRole("link", { name: "Authorized apps" });
    expect(link).toHaveAttribute("href", "/settings/authorized-apps");
  });

  test("lists each app with its origin, scopes, first authorization and last use to the hour", async () => {
    appServer([web, reader, bot, scribe]);
    await renderApp({ path: "/settings/authorized-apps" });

    expect(await screen.findByRole("heading", { level: 1, name: "Authorized apps" })).toBeInTheDocument();
    const rows = (await screen.findAllByRole("listitem")).filter((item) => item.hasAttribute("data-row-id"));
    expect(rows).toHaveLength(4);

    const [webRow, readerRow, botRow, scribeRow] = rows as [HTMLElement, HTMLElement, HTMLElement, HTMLElement];
    expect(within(webRow).getByRole("heading", { level: 3, name: "Web" })).toBeInTheDocument();
    expect(within(webRow).getByText("This server's own app")).toBeInTheDocument();
    expect(within(webRow).queryByText("This server has not verified this app.")).not.toBeInTheDocument();
    expect(within(webRow).getByText("read write follow")).toBeInTheDocument();
    expect(within(webRow).getByText(/^Oct 21, 2026, 9 AM \(to the hour\)$/)).toBeInTheDocument();

    expect(within(readerRow).getByText("From reader.example")).toBeInTheDocument();
    expect(within(readerRow).getByText("This server has not verified this app.")).toBeInTheDocument();
    expect(within(readerRow).getByText("No use recorded")).toBeInTheDocument();
    expect(within(readerRow).getByText("First authorized")).toBeInTheDocument();

    expect(within(botRow).getByText("Registered by @alice")).toBeInTheDocument();
    expect(within(scribeRow).getByText("Registered by the app itself")).toBeInTheDocument();
  });

  test("says when no app is authorized", async () => {
    appServer([]);
    await renderApp({ path: "/settings/authorized-apps" });
    expect(await screen.findByRole("heading", { level: 2, name: "No authorized apps" })).toBeInTheDocument();
  });

  test("revokes an app after its dialog and moves focus to the next row", async () => {
    const revoked = appServer([reader, bot]);
    const { user } = await renderApp({ path: "/settings/authorized-apps" });

    await user.click(await screen.findByRole("button", { name: "Revoke: Reader" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Revoke “Reader”?" });
    expect(dialog).toHaveAccessibleDescription(
      "The app loses access to your account at once. To use it again, you have to allow it again.",
    );
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    await waitFor(() => {
      expect(screen.queryByRole("heading", { level: 3, name: "Reader" })).not.toBeInTheDocument();
    });
    expect(revoked).toEqual([reader.id]);
    expect(screen.getByRole("heading", { level: 3, name: "Bot" })).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Revoke: Bot" })).toHaveFocus();
  });

  test("does not revoke when the dialog is cancelled", async () => {
    const revoked = appServer([reader]);
    const { user } = await renderApp({ path: "/settings/authorized-apps" });

    await user.click(await screen.findByRole("button", { name: "Revoke: Reader" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(revoked).toEqual([]);
    expect(screen.getByRole("heading", { level: 3, name: "Reader" })).toBeInTheDocument();
  });

  test("keeps the dialog open and says why when revoking fails", async () => {
    appServer([reader], true);
    const { user } = await renderApp({ path: "/settings/authorized-apps" });

    await user.click(await screen.findByRole("button", { name: "Revoke: Reader" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    expect(await within(dialog).findByText(/^Could not revoke the app:/)).toBeInTheDocument();
    // Behind the modal dialog, so hidden from the accessibility tree.
    expect(screen.getByRole("heading", { level: 3, name: "Reader", hidden: true })).toBeInTheDocument();
  });

  test("warns that revoking the web app signs out every browser, then signs this one out", async () => {
    const revoked = appServer([web, reader]);
    const { user } = await renderApp({ path: "/settings/authorized-apps" });

    await user.click(await screen.findByRole("button", { name: "Revoke: Web" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Revoke “Web”?" });
    expect(dialog).toHaveAccessibleDescription(
      "This is the web app you are using. Revoking it signs you out of every browser, including this one. You have to sign in again.",
    );
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    // Signed out, the landing page opens (/login would start signing in again).
    expect(await screen.findByRole("heading", { level: 1, name: "my example" })).toBeInTheDocument();
    expect(revoked).toEqual([web.id]);
  });
});
