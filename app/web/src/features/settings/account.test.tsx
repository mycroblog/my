// SPDX-License-Identifier: AGPL-3.0-only
// The account page through the whole app (web-client/README.md T4): asking for an archive of the user's data,
// downloading it, and deleting the account with the password, with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, oauthPaths, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { installPasskeys, removePasskeys } from "../../test/webauthn";
import { dataExportFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

afterEach(() => {
  vi.restoreAllMocks();
  removePasskeys();
});

type DataExport = Schemas["DataExport"];

const ready = dataExportFixture({
  state: "ready",
  completed_at: "2026-10-16T09:35:00Z",
  expires_at: "2026-10-23T09:35:00Z",
  file_size: 2_500_000,
});

/** Plays the user's archives: the list, and asking for one, which it records. */
function exportsServer(initial: DataExport[]) {
  let items = initial;
  const requests: number[] = [];
  server.use(
    http.get(`${apiBase}/exports`, () => HttpResponse.json({ items })),
    http.post(`${apiBase}/exports`, () => {
      requests.push(1);
      const requested = dataExportFixture({
        id: "0192f0c4-7a8e-7000-8000-0000000008e2",
        requested_at: "2026-10-17T10:00:00Z",
      });
      items = [requested, ...items];
      return HttpResponse.json(requested, { status: 202 });
    }),
  );
  return { requests };
}

describe("the archives of the user's data", () => {
  test("opens from the settings, asks for an archive, and shows it being prepared", async () => {
    const api = exportsServer([]);
    const { user } = await renderApp({ path: "/settings" });

    await user.click(await screen.findByRole("link", { name: "Archive and deletion" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Archive and deletion" })).toBeInTheDocument();
    const section = screen.getByRole("region", { name: "Download your data" });
    expect(within(section).getByRole("heading", { level: 3, name: "You have no archives" })).toBeInTheDocument();

    await user.click(within(section).getByRole("button", { name: "Prepare an archive" }));

    const item = await within(section).findByRole("article", { name: "Archive of Oct 17, 2026, 10:00 AM" });
    expect(within(item).getByText("Being prepared")).toBeInTheDocument();
    expect(within(section).getByRole("status")).toHaveTextContent(
      "The archive is being prepared. It appears below when it is ready.",
    );
    // Another archive waits for this one.
    expect(within(section).getByRole("button", { name: "Prepare an archive" })).toBeDisabled();
    expect(api.requests).toHaveLength(1);
  });

  test("downloads a ready archive as a file", async () => {
    exportsServer([
      ready,
      dataExportFixture({
        id: "0192f0c4-7a8e-7000-8000-0000000008e0",
        state: "failed",
        requested_at: "2026-10-01T00:00:00Z",
      }),
    ]);
    Object.defineProperty(URL, "createObjectURL", { configurable: true, value: () => "blob:archive" });
    Object.defineProperty(URL, "revokeObjectURL", { configurable: true, value: () => undefined });
    const saved: { href: string; download: string }[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
      saved.push({ href: this.href, download: this.download });
    });
    const { user } = await renderApp({ path: "/settings/account" });

    const item = await screen.findByRole("article", { name: "Archive of Oct 16, 2026, 9:30 AM" });
    expect(within(item).getByText("Ready")).toBeInTheDocument();
    expect(within(item).getByText("2.5 MB")).toBeInTheDocument();
    expect(within(item).getByText("Oct 23, 2026, 9:35 AM")).toHaveAttribute("dateTime", "2026-10-23T09:35:00Z");
    const failed = screen.getByRole("article", { name: "Archive of Oct 1, 2026, 12:00 AM" });
    expect(within(failed).getByText("Failed. Prepare another one.")).toBeInTheDocument();
    expect(within(failed).queryByRole("button")).not.toBeInTheDocument();

    await user.click(within(item).getByRole("button", { name: "Download the archive of Oct 16, 2026, 9:30 AM" }));

    await waitFor(() => {
      expect(saved).toEqual([{ href: "blob:archive", download: `export-${ready.id}.zip` }]);
    });
  });
});

describe("deleting the account", () => {
  test("asks for the password, says when it is wrong, then deletes the account and signs out", async () => {
    const passwords: string[] = [];
    let revoked = 0;
    server.use(
      http.post(`${apiBase}/accounts/delete`, async ({ request }) => {
        const body = (await request.json()) as Schemas["DeleteOwnAccountRequest"];
        passwords.push(body.password ?? "");
        return body.password === "secret"
          ? new HttpResponse(null, { status: 204 })
          : problem(422, "/api/v1/problems/wrong-password", "The password is wrong.");
      }),
      http.post(oauthPaths.revoke, () => {
        revoked++;
        return new HttpResponse(null, { status: 200 });
      }),
    );
    const { user } = await renderApp({ path: "/settings/account" });
    const section = await screen.findByRole("region", { name: "Delete your account" });

    await user.click(within(section).getByRole("button", { name: "Delete account…" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Delete your account?" });
    expect(dialog).toHaveAccessibleDescription(
      "Enter your password to confirm. Your account is deleted at once, and you are signed out. This cannot be undone.",
    );
    const password = within(dialog).getByLabelText(/Password/);
    expect(password).toHaveAttribute("type", "password");
    expect(password).toHaveAttribute("autocomplete", "current-password");

    await user.type(password, "wrong");
    await user.click(within(dialog).getByRole("button", { name: "Delete account" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("The password is wrong.");

    await user.clear(password);
    await user.type(password, "secret");
    await user.click(within(dialog).getByRole("button", { name: "Delete account" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Sign in" })).toBeInTheDocument();
    expect(passwords).toEqual(["wrong", "secret"]);
    expect(revoked).toBe(1);
  });

  test("asks a user without a password for a passkey", async () => {
    const credentials = installPasskeys();
    const bodies: Schemas["DeleteOwnAccountRequest"][] = [];
    server.use(
      http.get(`${apiBase}/accounts/password`, () => HttpResponse.json({ has_password: false })),
      http.post(`${apiBase}/accounts/delete`, async ({ request }) => {
        bodies.push((await request.json()) as Schemas["DeleteOwnAccountRequest"]);
        return bodies.length === 1
          ? problem(422, "/api/v1/problems/invalid-passkey", "The passkey could not be verified.")
          : new HttpResponse(null, { status: 204 });
      }),
      http.post(oauthPaths.revoke, () => new HttpResponse(null, { status: 200 })),
    );
    const { user } = await renderApp({ path: "/settings/account" });
    const section = await screen.findByRole("region", { name: "Delete your account" });

    await user.click(within(section).getByRole("button", { name: "Delete account…" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Delete your account?" });
    expect(dialog).toHaveAccessibleDescription(
      "Your browser asks for one of your passkeys to confirm. Your account is deleted at once, and you are signed out. This cannot be undone.",
    );
    expect(within(dialog).queryByLabelText(/Password/)).not.toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Delete account" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "The passkey could not be verified. Use a passkey of this account.",
    );
    await user.click(within(dialog).getByRole("button", { name: "Delete account" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Sign in" })).toBeInTheDocument();
    expect(credentials.get).toHaveBeenCalledTimes(2);
    expect(bodies[1]?.passkey?.confirmation_token).toBe("conf-1");
    expect(bodies[1]?.password).toBeUndefined();
  });
});
