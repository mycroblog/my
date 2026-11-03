// SPDX-License-Identifier: AGPL-3.0-only
// Deactivating the account through the whole app (web-client/README.md T4): the date before and after confirming,
// confirming with the password or a code, what a wrong one says, and the refusals; MSW plays the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

const due = "2026-12-03T09:00:00Z";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

/** Plays what the page reads and records the confirmations of the deactivation. */
function deactivationServer(preview: Schemas["DeactivationPreview"], twoFactorEnabled = false) {
  const bodies: Schemas["DeactivateOwnAccountRequest"][] = [];
  server.use(
    http.get(`${apiBase}/exports`, () => HttpResponse.json({ items: [] })),
    http.get(`${apiBase}/accounts/deactivation`, () => HttpResponse.json(preview)),
    http.get(`${apiBase}/two_factor`, () => HttpResponse.json({ enabled: twoFactorEnabled })),
    http.post(`${apiBase}/accounts/deactivation`, async ({ request }) => {
      const body = (await request.json()) as Schemas["DeactivateOwnAccountRequest"];
      bodies.push(body);
      if (body.password === "wrong") {
        return problem(422, "/api/v1/problems/wrong-password", "The password is wrong.");
      }
      if (body.two_factor_code === "000000") {
        return problem(422, "/api/v1/problems/invalid-two-factor-code", "The code is wrong.");
      }
      return HttpResponse.json({ deletion_due_at: due } satisfies Schemas["Deactivation"]);
    }),
  );
  return bodies;
}

describe("/settings/account, deactivating", () => {
  test("states the date, confirms with the password, and ends on the page that states the date", async () => {
    const bodies = deactivationServer({ deletion_due_at: due });
    const { user, router } = await renderApp({ path: "/settings/account" });
    await user.click(await screen.findByRole("button", { name: "Deactivate account…" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Deactivate your account?" });
    expect(within(dialog).getByText(/It will be deleted on .*2026/u)).toBeInTheDocument();

    await user.type(within(dialog).getByLabelText(/Current password/u), "wrong");
    await user.click(within(dialog).getByRole("button", { name: "Deactivate" }));
    expect(await within(dialog).findByText("The password is not correct.")).toBeInTheDocument();

    await user.clear(within(dialog).getByLabelText(/Current password/u));
    await user.type(within(dialog).getByLabelText(/Current password/u), "right");
    await user.click(within(dialog).getByRole("button", { name: "Deactivate" }));
    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/deactivated");
    });
    expect(bodies.at(-1)).toEqual({ password: "right" });
    expect(await screen.findByRole("heading", { name: "Your account is deactivated" })).toBeInTheDocument();
    expect(screen.getByText(/It will be deleted on .*2026/u)).toBeInTheDocument();
    expect(within(screen.getByRole("main")).getByRole("link", { name: "Sign in" })).toHaveAttribute("href", "/login");
  });

  test("a code of the second factor confirms in place of the password", async () => {
    const bodies = deactivationServer({ deletion_due_at: due }, true);
    const { user, router } = await renderApp({ path: "/settings/account" });
    await user.click(await screen.findByRole("button", { name: "Deactivate account…" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Use an authentication code instead" }));

    await user.type(within(dialog).getByLabelText(/Authentication code/u), "000000");
    await user.click(within(dialog).getByRole("button", { name: "Deactivate" }));
    expect(await within(dialog).findByText("The code is not correct.")).toBeInTheDocument();

    await user.clear(within(dialog).getByLabelText(/Authentication code/u));
    await user.type(within(dialog).getByLabelText(/Authentication code/u), "123456");
    await user.click(within(dialog).getByRole("button", { name: "Deactivate" }));
    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/deactivated");
    });
    expect(bodies.at(-1)).toEqual({ two_factor_code: "123456" });
  });

  test("cancelling sends nothing", async () => {
    const bodies = deactivationServer({ deletion_due_at: due });
    const { user } = await renderApp({ path: "/settings/account" });
    await user.click(await screen.findByRole("button", { name: "Deactivate account…" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Keep my account active" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(bodies).toEqual([]);
  });

  test("a paid subscription refuses it and leads to the subscription", async () => {
    deactivationServer({ deletion_due_at: due, refusal: "paid_subscription" });
    await renderApp({ path: "/settings/account" });
    expect(await screen.findByText(/Your supporter subscription is still running/u)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Manage your subscription" })).toHaveAttribute(
      "href",
      "/settings/supporter",
    );
    expect(screen.queryByRole("button", { name: "Deactivate account…" })).not.toBeInTheDocument();
  });

  test("a recent reactivation refuses it and says from when it can be done", async () => {
    deactivationServer({ deletion_due_at: due, refusal: "recently_reactivated", available_at: "2026-11-04T09:00:00Z" });
    await renderApp({ path: "/settings/account" });
    expect(await screen.findByText(/You can deactivate it again from .*2026/u)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Deactivate account…" })).not.toBeInTheDocument();
  });
});
