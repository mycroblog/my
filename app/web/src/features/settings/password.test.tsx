// SPDX-License-Identifier: AGPL-3.0-only
// The password page through the whole app (web-client/README.md T4): changing the password with the current one, what a
// wrong current password and too many of them say, removing it with a passkey (which test/webauthn plays), and setting
// one again with a passkey, with MSW playing the server.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { installPasskeys, removePasskeys } from "../../test/webauthn";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

afterEach(() => {
  removePasskeys();
});

const phone: Schemas["Passkey"] = {
  id: "019a0000-0000-7000-8000-0000000000a1",
  name: "Phone",
  created_at: "2026-10-01T09:00:00Z",
};

/** The server's password of a viewer with a passkey: whether it is set, and the removals and settings it received. */
function passwordlessServer(initial: boolean) {
  let hasPassword = initial;
  const removals: Schemas["RemovePasswordRequest"][] = [];
  const changes: Schemas["ChangePasswordRequest"][] = [];
  server.use(
    http.get(`${apiBase}/passkeys`, () => HttpResponse.json({ items: [phone] })),
    http.get(`${apiBase}/accounts/password`, () => HttpResponse.json({ has_password: hasPassword })),
    http.post(`${apiBase}/accounts/password/remove`, async ({ request }) => {
      removals.push((await request.json()) as Schemas["RemovePasswordRequest"]);
      if (removals.length === 1)
        return problem(422, "/api/v1/problems/passkey-confirmation-expired", "The confirmation expired.");
      hasPassword = false;
      return new HttpResponse(null, { status: 204 });
    }),
    http.post(`${apiBase}/accounts/password`, async ({ request }) => {
      changes.push((await request.json()) as Schemas["ChangePasswordRequest"]);
      hasPassword = true;
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return { removals, changes };
}

/** The server's password of the viewer, "correct horse", with the changes it received. */
function passwordServer() {
  const changes: Schemas["ChangePasswordRequest"][] = [];
  server.use(
    http.post(`${apiBase}/accounts/password`, async ({ request }) => {
      const body = (await request.json()) as Schemas["ChangePasswordRequest"];
      changes.push(body);
      if (body.current_password === "limited") {
        return problem(429, "about:blank", "Too many requests.", { headers: { "Retry-After": "60" } });
      }
      if (body.current_password !== "correct horse")
        return problem(422, "/api/v1/problems/wrong-password", "The password is wrong.");
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return changes;
}

describe("password", () => {
  test("is in the settings menu", async () => {
    await renderApp({ path: "/settings" });
    const link = await screen.findByRole("link", { name: "Password" });
    expect(link).toHaveAttribute("href", "/settings/password");
  });

  test("changes the password after the current one: a wrong one is the field's error", async () => {
    const changes = passwordServer();
    const { user } = await renderApp({ path: "/settings/password" });

    expect(await screen.findByRole("heading", { level: 1, name: "Password" })).toBeInTheDocument();
    const current = screen.getByLabelText(/^Current password/);
    const next = screen.getByLabelText(/^New password/);
    expect(current).toHaveAttribute("autocomplete", "current-password");
    expect(next).toHaveAttribute("autocomplete", "new-password");
    expect(next).toHaveAttribute("minlength", "8");
    expect(next).toHaveAccessibleDescription("8 to 128 characters.");

    await user.type(current, "wrong");
    await user.type(next, "a new password");
    await user.click(screen.getByRole("button", { name: "Change password" }));
    expect(await screen.findByText("The password is wrong.")).toBeInTheDocument();
    expect(current).toHaveAttribute("aria-invalid", "true");

    await user.clear(current);
    await user.type(current, "correct horse");
    await user.click(screen.getByRole("button", { name: "Change password" }));
    expect(
      await screen.findByText("Your password is changed. Other browsers and apps are signed out."),
    ).toBeInTheDocument();
    expect(changes).toEqual([
      { current_password: "wrong", new_password: "a new password" },
      { current_password: "correct horse", new_password: "a new password" },
    ]);
    expect(current).toHaveValue("");
    expect(next).toHaveValue("");
  });

  test("says why when too many current passwords were wrong", async () => {
    passwordServer();
    const { user } = await renderApp({ path: "/settings/password" });
    await user.type(await screen.findByLabelText(/^Current password/), "limited");
    await user.type(screen.getByLabelText(/^New password/), "a new password");
    await user.click(screen.getByRole("button", { name: "Change password" }));
    expect(await screen.findByText(/^Too many incorrect passwords:/)).toHaveAttribute("role", "alert");
  });

  test("removes the password after a passkey, then sets one again with a passkey", async () => {
    const credentials = installPasskeys();
    const api = passwordlessServer(true);
    const { user } = await renderApp({ path: "/settings/password" });

    const section = await screen.findByRole("region", { name: "Remove the password" });
    await user.click(within(section).getByRole("button", { name: "Remove password…" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Remove your password?" });
    await user.click(within(dialog).getByRole("button", { name: "Remove password" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "Confirming with the passkey took too long. Try again.",
    );
    await user.click(within(dialog).getByRole("button", { name: "Remove password" }));

    expect(
      await screen.findByText(
        "Your password is removed. Sign in with your passkeys from now on. Other browsers and apps are signed out.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(api.removals[1]?.passkey?.confirmation_token).toBe("conf-1");
    expect(credentials.get).toHaveBeenCalledTimes(2);

    // Without a password, the page sets one; a passkey confirms it.
    const set = screen.getByRole("region", { name: "Set a password" });
    expect(within(set).queryByLabelText(/^Current password/)).not.toBeInTheDocument();
    await user.type(within(set).getByLabelText(/^New password/), "a new password");
    await user.click(within(set).getByRole("button", { name: "Set password" }));
    expect(
      await screen.findByText("Your password is set. Other browsers and apps are signed out."),
    ).toBeInTheDocument();
    expect(api.changes).toHaveLength(1);
    expect(api.changes[0]?.new_password).toBe("a new password");
    expect(api.changes[0]?.passkey?.confirmation_token).toBe("conf-1");
    expect(api.changes[0]?.current_password).toBeUndefined();
    expect(await screen.findByRole("region", { name: "Change the password" })).toBeInTheDocument();
  });

  test("tells a user without a passkey to add one before removing the password", async () => {
    installPasskeys();
    await renderApp({ path: "/settings/password" });
    const section = await screen.findByRole("region", { name: "Remove the password" });
    expect(
      within(section).getByText("Add a passkey before you remove the password, so that you can still sign in."),
    ).toBeInTheDocument();
    expect(within(section).queryByRole("button")).not.toBeInTheDocument();
  });

  test("says why the prompt of the passkey did not confirm the new password", async () => {
    const credentials = installPasskeys();
    credentials.get.mockRejectedValueOnce(new DOMException("dismissed", "NotAllowedError"));
    passwordlessServer(false);
    const { user } = await renderApp({ path: "/settings/password" });
    const set = await screen.findByRole("region", { name: "Set a password" });
    await user.type(within(set).getByLabelText(/^New password/), "a new password");
    await user.click(within(set).getByRole("button", { name: "Set password" }));
    expect(await within(set).findByRole("alert")).toHaveTextContent(
      "Your passkey was not used: the browser's prompt was closed or timed out.",
    );
  });
});
