// SPDX-License-Identifier: AGPL-3.0-only
// The supporters screen through the whole app (web-client/README.md T4, ADR-0072): an administrator with manage_supporters
// sees the program, the plans and the subscriptions; enables the program or sees why the server refuses; creates a
// plan or sees the field the server refuses; retires a plan and ends a subscription after asking; narrows the
// subscriptions by state in the URL; sees the payment provider's state with how to fix its failure and its warnings,
// reconciles now or sees why it is too soon, and sees no provider section without a configured provider. A viewer without the permission has neither the screen nor its menu entry; with
// MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import {
  paymentStatusFixture,
  supporterPlanFixture,
  supporterProgramFixture,
  supporterSubscriptionFixture,
} from "./test/handlers";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

/** A problem document with a localized detail, as the server answers a refusal. */
function refusal(status: number, type: string, title: string, detail: string, extra: Record<string, unknown> = {}) {
  return HttpResponse.json(
    { type, title, status, detail, ...extra },
    { status, headers: { "Content-Type": "application/problem+json" } },
  );
}

const planId = "0199f0e0-0000-7000-8000-0000000000c1";

describe("the supporters screen", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_supporters"]));
  });

  test("shows the program, the plans by rank with the retired ones apart, and the subscriptions", async () => {
    const {
      account_id: _account,
      username: _username,
      ...erasedSubscription
    } = supporterSubscriptionFixture("0199f0e0-0000-7000-8000-0000000000e2", {
      state: "expired",
    });
    server.use(
      http.get(`${apiBase}/admin/supporter_program`, () =>
        HttpResponse.json(
          supporterProgramFixture({
            enabled: true,
            provider: "stripe",
            live_subscriptions: 2,
            commerce_disclosure: false,
            warnings: ["no_commerce_disclosure"],
          }),
        ),
      ),
      http.get(`${apiBase}/admin/supporter_plans`, () =>
        HttpResponse.json({
          items: [
            supporterPlanFixture(planId, { members: 2 }),
            supporterPlanFixture("0199f0e0-0000-7000-8000-0000000000c2", {
              name: "Gold",
              rank: 2,
              prices_ready: false,
            }),
            supporterPlanFixture("0199f0e0-0000-7000-8000-0000000000c3", {
              name: "Old",
              rank: 0,
              retired: true,
              retired_at: "2026-10-21T00:00:00Z",
            }),
          ],
        } satisfies Schemas["AdminSupporterPlanList"]),
      ),
      http.get(`${apiBase}/admin/supporter_subscriptions`, () =>
        HttpResponse.json({
          items: [supporterSubscriptionFixture("0199f0e0-0000-7000-8000-0000000000e1"), erasedSubscription],
        }),
      ),
    );
    await renderApp({ path: "/admin/supporters" });

    expect(await screen.findByRole("heading", { level: 1, name: "Supporters" })).toBeInTheDocument();
    const program = screen.getByRole("region", { name: "Supporter program" });
    expect(within(program).getByRole("switch", { name: "Enable the supporter program" })).toBeChecked();
    expect(within(program).getByText("2 subscriptions")).toBeInTheDocument();
    expect(within(program).getByRole("link", { name: "Write them in the terms and policies" })).toHaveAttribute(
      "href",
      "/admin/documents",
    );

    const plans = screen.getByRole("region", { name: "Plans" });
    const [supporter, gold] = within(plans).getAllByRole("listitem");
    if (supporter === undefined || gold === undefined) throw new Error("two plans expected");
    expect(within(supporter).getByRole("heading", { level: 3, name: "Supporter" })).toBeInTheDocument();
    expect(within(supporter).getByText("¥500")).toBeInTheDocument();
    expect(within(supporter).getByText("¥5,400 (10% off)")).toBeInTheDocument();
    expect(within(gold).getByText("Preparing prices")).toBeInTheDocument();
    const retired = screen.getByRole("region", { name: "Retired plans" });
    expect(within(retired).getByRole("heading", { level: 3, name: "Old" })).toBeInTheDocument();
    expect(within(retired).queryByRole("button", { name: /Retire/u })).not.toBeInTheDocument();

    const subscriptions = screen.getByRole("region", { name: "Subscriptions" });
    const [active, ended] = within(subscriptions).getAllByRole("row").slice(1);
    if (active === undefined || ended === undefined) throw new Error("two subscriptions expected");
    expect(within(active).getByRole("rowheader", { name: "@bob" })).toBeInTheDocument();
    expect(within(active).getByText("¥500 a month")).toBeInTheDocument();
    expect(within(active).getByRole("button", { name: "End the subscription of ⁦@bob⁩ now" })).toBeInTheDocument();
    expect(within(ended).getByRole("rowheader", { name: "Deleted account" })).toBeInTheDocument();
    expect(within(ended).queryByRole("button")).not.toBeInTheDocument();
  });

  test("enables the program, and shows why the server refuses it", async () => {
    const { user } = await renderApp({ path: "/admin/supporters" });
    const program = await screen.findByRole("region", { name: "Supporter program" });

    server.use(
      http.put(`${apiBase}/admin/supporter_program`, () =>
        refusal(
          409,
          "/api/v1/problems/supporters-unavailable",
          "The supporter program is not available on this server.",
          "Configure a payment provider first.",
        ),
      ),
    );
    await user.click(within(program).getByRole("switch", { name: "Enable the supporter program" }));
    expect(await within(program).findByRole("alert")).toHaveTextContent(
      "Could not change the supporter program: Configure a payment provider first.",
    );

    let enabled = false;
    server.use(
      http.put(`${apiBase}/admin/supporter_program`, () => {
        enabled = true;
        return HttpResponse.json(supporterProgramFixture({ enabled: true, provider: "stripe" }));
      }),
      http.get(`${apiBase}/admin/supporter_program`, () =>
        HttpResponse.json(supporterProgramFixture(enabled ? { enabled: true, provider: "stripe" } : {})),
      ),
    );
    await user.click(within(program).getByRole("switch", { name: "Enable the supporter program" }));
    expect(await within(program).findByRole("status")).toHaveTextContent("The supporter program is enabled.");
    expect(within(program).getByRole("switch", { name: "Enable the supporter program" })).toBeChecked();
  });

  test("keeps the switch off when the program cannot be enabled", async () => {
    const { configured_provider: _provider, ...noProvider } = supporterProgramFixture({
      can_enable: false,
      warnings: ["no_provider"],
    });
    server.use(http.get(`${apiBase}/admin/supporter_program`, () => HttpResponse.json(noProvider)));
    await renderApp({ path: "/admin/supporters" });
    const program = await screen.findByRole("region", { name: "Supporter program" });

    expect(within(program).getByRole("switch", { name: "Enable the supporter program" })).toBeDisabled();
    expect(
      within(program).getByText("No payment provider is configured, so the program cannot be enabled."),
    ).toBeInTheDocument();
  });

  test("shows the payment provider's state, how to fix the last reconciliation, and the webhook warnings", async () => {
    server.use(
      http.get(`${apiBase}/admin/payment_status`, () => {
        const { last_webhook_at: _webhook, ...status } = paymentStatusFixture({
          mode: "live",
          last_reconcile: {
            outcome: "failed",
            error_kind: "permission",
            requests: 3,
            checked_subscriptions: 12,
            corrected_subscriptions: 2,
            occurred_at: "2026-10-22T00:00:00Z",
          },
          warnings: ["no_webhook", "reconcile_corrected"],
        });
        return HttpResponse.json(status);
      }),
    );
    await renderApp({ path: "/admin/supporters" });
    const payment = await screen.findByRole("region", { name: "Payment provider" });

    expect(within(payment).getByText("stripe")).toBeInTheDocument();
    expect(within(payment).getByText("Live")).toBeInTheDocument();
    expect(within(payment).getByText("None in the last 45 days")).toBeInTheDocument();
    expect(within(payment).getByText("Failed")).toBeInTheDocument();
    expect(within(payment).getByText("12")).toBeInTheDocument();
    expect(within(payment).getByText(/The provider refused the API key/u)).toBeInTheDocument();
    expect(within(payment).getByText(/No webhook delivery is recorded in the last 45 days\./u)).toBeInTheDocument();
    expect(within(payment).getByText(/The last reconciliation corrected subscriptions/u)).toBeInTheDocument();
  });

  test("reconciles now, and explains a second one within the hour", async () => {
    let queued = 0;
    server.use(
      http.post(`${apiBase}/admin/payment_reconciles`, () => {
        queued += 1;
        if (queued > 1) return refusal(429, "/api/v1/problems/rate-limited", "Too many requests.", "Try again later.");
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: "/admin/supporters" });
    const payment = await screen.findByRole("region", { name: "Payment provider" });
    expect(within(payment).getByText("Test: nothing is charged")).toBeInTheDocument();
    expect(within(payment).getByText("Never")).toBeInTheDocument();

    await user.click(within(payment).getByRole("button", { name: "Reconcile now" }));
    expect(await within(payment).findByRole("status")).toHaveTextContent("The reconciliation is queued.");

    await user.click(within(payment).getByRole("button", { name: "Reconcile now" }));
    expect(await within(payment).findByRole("alert")).toHaveTextContent(
      "A reconciliation already ran in this hour (UTC).",
    );
    expect(queued).toBe(2);
  });

  test("shows no payment provider section without a configured provider", async () => {
    const { configured_provider: _provider, ...noProvider } = supporterProgramFixture({
      can_enable: false,
      warnings: ["no_provider"],
    });
    let read = false;
    server.use(
      http.get(`${apiBase}/admin/supporter_program`, () => HttpResponse.json(noProvider)),
      http.get(`${apiBase}/admin/payment_status`, () => {
        read = true;
        return refusal(404, "/api/v1/problems/not-found", "Not found.", "No payment provider is configured.");
      }),
    );
    await renderApp({ path: "/admin/supporters" });

    expect(await screen.findByRole("region", { name: "Supporter program" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Payment provider" })).not.toBeInTheDocument();
    expect(read).toBe(false);
  });

  test("shows why disabling is refused while subscriptions go on", async () => {
    server.use(
      http.get(`${apiBase}/admin/supporter_program`, () =>
        HttpResponse.json(supporterProgramFixture({ enabled: true, provider: "stripe", live_subscriptions: 1 })),
      ),
      http.put(`${apiBase}/admin/supporter_program`, () =>
        refusal(
          409,
          "/api/v1/problems/live-subscriptions",
          "Subscriptions to the supporter plans go on.",
          "Retire the plans first.",
        ),
      ),
    );
    const { user } = await renderApp({ path: "/admin/supporters" });
    const program = await screen.findByRole("region", { name: "Supporter program" });

    await user.click(within(program).getByRole("switch", { name: "Enable the supporter program" }));
    expect(await within(program).findByRole("alert")).toHaveTextContent("Retire the plans first.");
  });

  test("creates a plan, and keeps the server's refusal next to its field", async () => {
    const sent: Schemas["AdminSupporterPlanCreateRequest"][] = [];
    server.use(
      http.post(`${apiBase}/admin/supporter_plans`, async ({ request }) => {
        const body = (await request.json()) as Schemas["AdminSupporterPlanCreateRequest"];
        sent.push(body);
        if (sent.length === 1) {
          return refusal(
            422,
            "/api/v1/problems/invalid-supporter-plan",
            "The supporter plan cannot be saved.",
            "Choose a role that nobody holds.",
            {
              reason: "role_held",
            },
          );
        }
        return HttpResponse.json(supporterPlanFixture(planId, { name: body.name }));
      }),
    );
    const { user } = await renderApp({ path: "/admin/supporters" });
    const form = await screen.findByRole("region", { name: "Create a plan" });

    await user.click(within(form).getByRole("button", { name: "Create the plan" }));
    expect(within(form).getByText("Enter the key of a role.")).toBeInTheDocument();
    expect(sent).toEqual([]);

    await user.type(within(form).getByRole("textbox", { name: "Role" }), "fan");
    await user.type(within(form).getByRole("textbox", { name: "Name" }), "Supporter");
    await user.type(within(form).getByRole("textbox", { name: "Monthly price" }), "500");
    await user.click(within(form).getByRole("button", { name: "Create the plan" }));
    expect(await within(form).findByText("Choose a role that nobody holds.")).toBeInTheDocument();
    expect(within(form).getByRole("textbox", { name: "Role" })).toBeInvalid();
    expect(sent[0]).toEqual({
      role: "fan",
      name: "Supporter",
      monthly_price: { amount: "500", currency: "JPY" },
      annual_discount_percent: 0,
      emoji_limit: 0,
      rank: 0,
      visibility: "public",
    });

    await user.click(within(form).getByRole("button", { name: "Create the plan" }));
    expect(await within(form).findByText(/Created the plan ⁨Supporter⁩\./u)).toBeInTheDocument();
    expect(within(form).getByRole("textbox", { name: "Name" })).toHaveValue("");
  });

  test("retires a plan after asking", async () => {
    let retired = false;
    server.use(
      http.get(`${apiBase}/admin/supporter_plans`, () =>
        HttpResponse.json({
          items: [supporterPlanFixture(planId, { retired, ...(retired && { retired_at: "2026-10-22T00:00:00Z" }) })],
        }),
      ),
      http.post(`${apiBase}/admin/supporter_plans/:id/retire`, () => {
        retired = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: "/admin/supporters" });
    const plans = await screen.findByRole("region", { name: "Plans" });

    await user.click(within(plans).getByRole("button", { name: "Retire ⁨Supporter⁩" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Retire ⁨Supporter⁩?" });
    expect(dialog).toHaveTextContent("every subscription of it ends at the end of its period");
    await user.click(within(dialog).getByRole("button", { name: "Retire" }));

    expect(await within(plans).findByRole("status")).toHaveTextContent("Retired the plan ⁨Supporter⁩.");
    expect(await screen.findByRole("region", { name: "Retired plans" })).toBeInTheDocument();
  });

  test("ends a subscription at once after asking, and narrows the list by state", async () => {
    const queries: string[] = [];
    let canceled = false;
    server.use(
      http.get(`${apiBase}/admin/supporter_subscriptions`, ({ request }) => {
        queries.push(new URL(request.url).search);
        return HttpResponse.json({
          items: [
            supporterSubscriptionFixture("0199f0e0-0000-7000-8000-0000000000e1", canceled ? { state: "expired" } : {}),
          ],
        });
      }),
      http.post(`${apiBase}/admin/supporter_subscriptions/:id/cancel`, () => {
        canceled = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user, router } = await renderApp({ path: "/admin/supporters" });
    const list = await screen.findByRole("region", { name: "Subscriptions" });

    await user.click(within(list).getByRole("button", { name: "End the subscription of ⁦@bob⁩ now" }));
    const dialog = await screen.findByRole("alertdialog", { name: "End the subscription of ⁦@bob⁩ now?" });
    expect(dialog).toHaveTextContent("Nothing is refunded");
    await user.click(within(dialog).getByRole("button", { name: "End now" }));
    expect(await within(list).findByRole("status")).toHaveTextContent("Ended a subscription to ⁨Supporter⁩.");
    await waitFor(() => {
      expect(within(list).queryByRole("button", { name: /End the subscription/u })).not.toBeInTheDocument();
    });

    await user.click(within(list).getByRole("button", { name: /State/u }));
    await user.click(await screen.findByRole("option", { name: "Payment failed" }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ state: "past_due" });
    });
    await waitFor(() => {
      expect(queries.at(-1)).toContain("state=past_due");
    });
  });
});

describe("the supporters screen without the permission", () => {
  test("says why it shows nothing", async () => {
    server.use(signedInHandler, operatorHandler(["manage_finance"]));
    await renderApp({ path: "/admin/supporters" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Supporter program" })).not.toBeInTheDocument();
  });

  test("the administration menu has no entry for a viewer without manage_supporters", async () => {
    server.use(signedInHandler, operatorHandler(["manage_finance"]));
    await renderApp({ path: "/admin" });

    expect(
      await within(await screen.findByRole("navigation", { name: "Admin and moderation" })).findByRole("link", {
        name: /Costs/u,
      }),
    ).toBeInTheDocument();
    expect(
      within(screen.getByRole("navigation", { name: "Admin and moderation" })).queryByRole("link", {
        name: /Supporters/u,
      }),
    ).not.toBeInTheDocument();
  });

  test("the administration menu links to the screen for manage_supporters", async () => {
    server.use(signedInHandler, operatorHandler(["manage_supporters"]));
    await renderApp({ path: "/admin" });

    expect(
      await within(await screen.findByRole("navigation", { name: "Admin and moderation" })).findByRole("link", {
        name: /Supporters/u,
      }),
    ).toHaveAttribute("href", "/admin/supporters");
  });
});
