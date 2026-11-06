// SPDX-License-Identifier: AGPL-3.0-only
// The pages of the supporters screen through the whole app (web-client/README.md T4, ADR-0072): an administrator with
// manage_supporters moves between the program, the plans and the subscriptions with the screen's LocalNav; on the
// program's page enables the program and sets who may read the supporter list in one save, or sees why the server
// refuses, and sees the payment provider's state with how to fix its failure and its warnings, reconciles now or sees
// why it is too soon, and sees no provider section without a configured provider; lists the plans narrowed to a state,
// creates a plan on its page or sees the field the server refuses, and changes and retires a plan on the plan's page;
// lists the subscriptions narrowed by state in the URL and ends one at once on its page after asking. A viewer without
// the permission has neither the screen nor its menu entry; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { sectionHeadings } from "../../test/sections";
import {
  instanceSettingsFixture,
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
const subscriptionId = "0199f0e0-0000-7000-8000-0000000000e1";

/** The plans of a server: Supporter, Gold (preparing its prices) and Old (retired). */
const threePlans = (): Schemas["AdminSupporterPlanList"] => ({
  items: [
    supporterPlanFixture(planId, { members: 2 }),
    supporterPlanFixture("0199f0e0-0000-7000-8000-0000000000c2", { name: "Gold", rank: 2, prices_ready: false }),
    supporterPlanFixture("0199f0e0-0000-7000-8000-0000000000c3", {
      name: "Old",
      rank: 0,
      retired: true,
      retired_at: "2026-10-21T00:00:00Z",
    }),
  ],
});

describe("the program page of the supporters screen", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_supporters"]));
  });

  test("shows the program and the payment provider, under the screen's pages", async () => {
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
    );
    await renderApp({ path: "/admin/supporters" });

    expect(await screen.findByRole("heading", { level: 1, name: "Supporters" })).toBeInTheDocument();
    const pages = screen.getByRole("navigation", { name: "Supporter pages" });
    expect(within(pages).getByRole("link", { name: "Program" })).toHaveAttribute("aria-current", "page");
    expect(within(pages).getByRole("link", { name: "Plans" })).toHaveAttribute("href", "/admin/supporters/plans");
    expect(within(pages).getByRole("link", { name: "Subscriptions" })).toHaveAttribute(
      "href",
      "/admin/supporters/subscriptions",
    );
    // docs/architecture/web-client/screens/admin-supporters.md §3
    expect(sectionHeadings()).toEqual(["Supporter program", "Payment provider"]);
    const program = screen.getByRole("region", { name: "Supporter program" });
    expect(within(program).getByRole("checkbox", { name: "Enable the supporter program" })).toBeChecked();
    expect(within(program).getByText("2 subscriptions")).toBeInTheDocument();
    expect(within(program).getByRole("link", { name: "Write them in the terms and policies" })).toHaveAttribute(
      "href",
      "/admin/documents/commerce-disclosure",
    );
    expect(within(program).getAllByRole("button", { name: "Save" })).toHaveLength(1);
  });

  test("enables the program with the form's save, and shows why the server refuses it", async () => {
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
    await user.click(within(program).getByRole("checkbox", { name: "Enable the supporter program" }));
    await user.click(within(program).getByRole("button", { name: "Save" }));
    expect(await within(program).findByText("Could not save: Configure a payment provider first.")).toBeInTheDocument();
    expect(within(program).getByRole("checkbox", { name: "Enable the supporter program" })).toBeChecked();

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
    await user.click(within(program).getByRole("button", { name: "Save" }));
    expect(await within(program).findByText(/^Saved\./u)).toBeInTheDocument();
    expect(enabled).toBe(true);
    expect(within(program).getByRole("checkbox", { name: "Enable the supporter program" })).toBeChecked();
  });

  test("keeps the checkbox off when the program cannot be enabled", async () => {
    const { configured_provider: _provider, ...noProvider } = supporterProgramFixture({
      can_enable: false,
      warnings: ["no_provider"],
    });
    server.use(http.get(`${apiBase}/admin/supporter_program`, () => HttpResponse.json(noProvider)));
    await renderApp({ path: "/admin/supporters" });
    const program = await screen.findByRole("region", { name: "Supporter program" });

    expect(within(program).getByRole("checkbox", { name: "Enable the supporter program" })).toBeDisabled();
    expect(
      within(program).getByText("No payment provider is configured, so the program cannot be enabled."),
    ).toBeInTheDocument();
  });

  test("shows the payment provider's state, how to fix the last reconciliation, and the webhook warnings", async () => {
    server.use(
      http.get(`${apiBase}/admin/payment_status`, () => {
        const { last_webhook_at: _webhook, ...status } = paymentStatusFixture({
          mode: "live",
          last_reconciliation: {
            outcome: "failed",
            error_kind: "permission",
            requests: 3,
            checked_subscriptions: 12,
            corrected_subscriptions: 2,
            occurred_at: "2026-10-22T00:00:00Z",
          },
          warnings: ["no_webhook", "reconciliation_corrected"],
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
      http.post(`${apiBase}/admin/payment_reconciliation/runs`, () => {
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

    await user.click(within(program).getByRole("checkbox", { name: "Enable the supporter program" }));
    await user.click(within(program).getByRole("button", { name: "Save" }));
    expect(await within(program).findByText(/Retire the plans first\./u)).toBeInTheDocument();
  });
});

describe("the supporter list in the program's form", () => {
  test("publishes the supporter list with the same save, to nobody by default", async () => {
    server.use(signedInHandler, operatorHandler(["manage_supporters", "manage_settings"]));
    const patches: Schemas["UpdateInstanceSettingsRequest"][] = [];
    let programPuts = 0;
    server.use(
      http.patch(`${apiBase}/admin/settings`, async ({ request }) => {
        const body = (await request.json()) as Schemas["UpdateInstanceSettingsRequest"];
        patches.push(body);
        return HttpResponse.json(instanceSettingsFixture(body.values));
      }),
      http.put(`${apiBase}/admin/supporter_program`, () => {
        programPuts += 1;
        return HttpResponse.json(supporterProgramFixture());
      }),
    );
    const { user } = await renderApp({ path: "/admin/supporters" });
    const program = await screen.findByRole("region", { name: "Supporter program" });
    const list = within(program).getByRole("region", { name: "Supporter list" });
    const choice = await within(list).findByRole("radiogroup", { name: "Who may read the supporter list" });
    expect(within(choice).getByRole("radio", { name: "Nobody" })).toBeChecked();
    const signedIn = within(choice).getByRole("radio", { name: "Signed-in users" });
    expect(signedIn).toHaveAccessibleDescription(/Only people signed in to this server/u);

    await user.click(signedIn);
    await user.click(within(program).getByRole("button", { name: "Save" }));

    expect(await within(program).findByText(/^Saved\./u)).toBeInTheDocument();
    expect(patches).toEqual([{ values: { supporter_list_visibility: "signed_in" } }]);
    // The program did not change, so it is not sent.
    expect(programPuts).toBe(0);
  });

  test("says who may read the list, without its choices, to a viewer who may not change it", async () => {
    server.use(signedInHandler, operatorHandler(["manage_supporters"]));
    await renderApp({ path: "/admin/supporters" });
    const list = await screen.findByRole("region", { name: "Supporter list" });

    expect(await within(list).findByText(/may read the supporter list\./u)).toBeInTheDocument();
    expect(within(list).queryByRole("radiogroup")).not.toBeInTheDocument();
  });
});

describe("the plans of the supporters screen", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_supporters"]));
  });

  test("lists the plans in effect by rank, then the retired ones, and narrows them to a state", async () => {
    server.use(http.get(`${apiBase}/admin/supporter_plans`, () => HttpResponse.json(threePlans())));
    const { user, router } = await renderApp({ path: "/admin/supporters/plans" });

    expect(await screen.findByRole("heading", { level: 1, name: "Supporter plans" })).toBeInTheDocument();
    expect(
      within(screen.getByRole("navigation", { name: "Supporter pages" })).getByRole("link", { name: "Plans" }),
    ).toHaveAttribute("aria-current", "page");
    expect(screen.getAllByRole("link", { name: "Create a plan" })).toHaveLength(1);
    expect(screen.getByRole("link", { name: "Create a plan" })).toHaveAttribute("href", "/admin/supporters/plans/new");
    const table = screen.getByRole("table", { name: "Plans" });
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((header) => header.textContent),
    ).toEqual(["Name", "State", "Created"]);
    const [supporter, gold, old] = within(table).getAllByRole("row").slice(1);
    if (supporter === undefined || gold === undefined || old === undefined) throw new Error("three plans expected");
    expect(within(supporter).getByRole("link", { name: "Supporter" })).toHaveAttribute(
      "href",
      `/admin/supporters/plans/${planId}`,
    );
    expect(within(supporter).getByText("In effect")).toBeInTheDocument();
    expect(within(gold).getByText("Preparing prices")).toBeInTheDocument();
    expect(within(old).getByText("Retired")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /State/u }));
    await user.click(await screen.findByRole("option", { name: "Retired" }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ state: "retired" });
    });
    const retired = await screen.findByRole("table", { name: "Plans" });
    await waitFor(() => {
      expect(within(retired).getAllByRole("row")).toHaveLength(2);
    });
    expect(within(retired).getByRole("link", { name: "Old" })).toBeInTheDocument();
  });

  test("creates a plan on its page, keeps the server's refusal next to its field, and opens the plan", async () => {
    const sent: Schemas["AdminSupporterPlanCreateRequest"][] = [];
    let created = false;
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
            { reason: "role_held" },
          );
        }
        created = true;
        return HttpResponse.json(supporterPlanFixture(planId, { name: body.name }));
      }),
      http.get(`${apiBase}/admin/supporter_plans`, () =>
        HttpResponse.json({ items: created ? [supporterPlanFixture(planId)] : [] }),
      ),
    );
    const { user, router } = await renderApp({ path: "/admin/supporters/plans/new" });
    expect(await screen.findByRole("heading", { level: 1, name: "Create a plan" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Create the plan" }));
    expect(screen.getByText("Enter the key of a role.")).toBeInTheDocument();
    expect(sent).toEqual([]);

    await user.type(screen.getByRole("textbox", { name: "Role" }), "fan");
    await user.type(screen.getByRole("textbox", { name: "Name" }), "Supporter");
    await user.type(screen.getByRole("textbox", { name: "Monthly price" }), "500");
    await user.click(screen.getByRole("button", { name: "Create the plan" }));
    expect(await screen.findByText("Choose a role that nobody holds.")).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Role" })).toBeInvalid();
    expect(sent[0]).toEqual({
      role: "fan",
      name: "Supporter",
      monthly_price: { amount: "500", currency: "JPY" },
      annual_discount_percent: 0,
      emoji_limit: 0,
      rank: 0,
      visibility: "public",
    });

    await user.click(screen.getByRole("button", { name: "Create the plan" }));
    await waitFor(() => {
      expect(router.state.location.pathname).toBe(`/admin/supporters/plans/${planId}`);
    });
    expect(await screen.findByRole("heading", { level: 1, name: "Supporter plan" })).toBeInTheDocument();
    expect(await screen.findByText(/Created the plan ⁨Supporter⁩\./u)).toBeInTheDocument();
  });

  test("shows a plan's facts and saves a change of its terms", async () => {
    const patches: Schemas["AdminSupporterPlanUpdateRequest"][] = [];
    server.use(
      http.get(`${apiBase}/admin/supporter_plans`, () => HttpResponse.json(threePlans())),
      http.patch(`${apiBase}/admin/supporter_plans/:id`, async ({ request }) => {
        const change = (await request.json()) as Schemas["AdminSupporterPlanUpdateRequest"];
        patches.push(change);
        return HttpResponse.json(supporterPlanFixture(planId, { name: change.name ?? "Supporter" }));
      }),
    );
    const { user } = await renderApp({ path: `/admin/supporters/plans/${planId}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Supporter plan" })).toBeInTheDocument();
    // docs/architecture/web-client/screens/admin-supporters-plan.md §3
    expect(sectionHeadings()).toEqual(["Supporter", "Edit the plan", "Retire the plan"]);
    const facts = screen.getByRole("region", { name: "Supporter" });
    expect(within(facts).getByText("¥500")).toBeInTheDocument();
    expect(within(facts).getByText("¥5,400 (10% off)")).toBeInTheDocument();
    expect(within(facts).getByText("2 subscriptions")).toBeInTheDocument();

    const edit = screen.getByRole("region", { name: "Edit the plan" });
    await user.click(within(edit).getByRole("button", { name: "Save" }));
    expect(await within(edit).findByText("Nothing has changed.")).toBeInTheDocument();
    expect(patches).toEqual([]);

    const name = within(edit).getByRole("textbox", { name: "Name" });
    await user.clear(name);
    await user.type(name, "Fan");
    await user.click(within(edit).getByRole("button", { name: "Save" }));
    expect(await within(edit).findByText(/Saved the plan ⁨Fan⁩\./u)).toBeInTheDocument();
    expect(patches).toEqual([{ name: "Fan" }]);
  });

  test("retires a plan on its page after asking", async () => {
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
    const { user } = await renderApp({ path: `/admin/supporters/plans/${planId}` });
    const section = await screen.findByRole("region", { name: "Retire the plan" });

    await user.click(within(section).getByRole("button", { name: "Retire" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Retire ⁨Supporter⁩?" });
    expect(dialog).toHaveTextContent("every subscription of it ends at the end of its period");
    await user.click(within(dialog).getByRole("button", { name: "Retire" }));

    const facts = screen.getByRole("region", { name: "Supporter" });
    expect(await within(facts).findByRole("status")).toHaveTextContent("Retired the plan ⁨Supporter⁩.");
    expect(within(facts).getByText(/This plan is retired/u)).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Edit the plan" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Retire the plan" })).not.toBeInTheDocument();
  });
});

describe("the subscriptions of the supporters screen", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_supporters"]));
  });

  test("lists the subscriptions in four columns, and narrows them by state in the URL", async () => {
    const queries: string[] = [];
    const {
      account_id: _account,
      username: _username,
      ...erasedSubscription
    } = supporterSubscriptionFixture("0199f0e0-0000-7000-8000-0000000000e2", { state: "expired" });
    server.use(
      http.get(`${apiBase}/admin/supporter_subscriptions`, ({ request }) => {
        queries.push(new URL(request.url).search);
        return HttpResponse.json({ items: [supporterSubscriptionFixture(subscriptionId), erasedSubscription] });
      }),
    );
    const { user, router } = await renderApp({ path: "/admin/supporters/subscriptions" });

    expect(await screen.findByRole("heading", { level: 1, name: "Supporter subscriptions" })).toBeInTheDocument();
    const table = screen.getByRole("table", { name: "Subscriptions" });
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((header) => header.textContent),
    ).toEqual(["Account", "Plan", "State", "Period ends"]);
    const [active, ended] = within(table).getAllByRole("row").slice(1);
    if (active === undefined || ended === undefined) throw new Error("two subscriptions expected");
    expect(within(active).getByRole("link", { name: "@bob" })).toHaveAttribute(
      "href",
      `/admin/supporters/subscriptions/${subscriptionId}`,
    );
    expect(within(active).getByText("Active")).toBeInTheDocument();
    expect(within(ended).getByRole("link", { name: "Deleted account" })).toBeInTheDocument();
    expect(within(table).queryByRole("button")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /State/u }));
    await user.click(await screen.findByRole("option", { name: "Payment failed" }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ state: "past_due" });
    });
    await waitFor(() => {
      expect(queries.at(-1)).toContain("state=past_due");
    });
  });

  test("shows a subscription and ends it at once after asking", async () => {
    let canceled = false;
    server.use(
      http.get(`${apiBase}/admin/supporter_subscriptions/:id`, ({ params }) =>
        HttpResponse.json(supporterSubscriptionFixture(String(params["id"]), canceled ? { state: "expired" } : {})),
      ),
      http.post(`${apiBase}/admin/supporter_subscriptions/:id/end`, ({ params }) => {
        canceled = true;
        return HttpResponse.json(supporterSubscriptionFixture(String(params["id"]), { state: "expired" }));
      }),
    );
    const { user } = await renderApp({ path: `/admin/supporters/subscriptions/${subscriptionId}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Supporter subscription" })).toBeInTheDocument();
    const facts = screen.getByRole("region", { name: "@bob" });
    expect(within(facts).getByText("¥500 a month")).toBeInTheDocument();
    expect(within(facts).getByRole("link", { name: "Supporter" })).toHaveAttribute(
      "href",
      `/admin/supporters/plans/${planId}`,
    );

    const end = screen.getByRole("region", { name: "End the subscription" });
    await user.click(within(end).getByRole("button", { name: "End now" }));
    const dialog = await screen.findByRole("alertdialog", { name: "End the subscription of ⁦@bob⁩ now?" });
    expect(dialog).toHaveTextContent("Nothing is refunded");
    await user.click(within(dialog).getByRole("button", { name: "End now" }));
    expect(await within(facts).findByRole("status")).toHaveTextContent("Ended a subscription to ⁨Supporter⁩.");
    await waitFor(() => {
      expect(screen.queryByRole("region", { name: "End the subscription" })).not.toBeInTheDocument();
    });
    expect(within(facts).getByText("Ended")).toBeInTheDocument();
  });
});

describe("the supporters screen without the permission", () => {
  test("says why it shows nothing", async () => {
    server.use(signedInHandler, operatorHandler(["manage_finance"]));
    await renderApp({ path: "/admin/supporters" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Supporter program" })).not.toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Supporter pages" })).not.toBeInTheDocument();
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
