// SPDX-License-Identifier: AGPL-3.0-only
// The supporter plans through the whole app (web-client/README.md T4, ADR-0071 §4, ADR-0072 §6, §8, §13, §14): MSW plays
// the server, the user subscribes through the final confirmation to the provider's checkout, comes back, changes,
// cancels and resumes the subscription, opens the portal, reads the supporter list and chooses to be on it; the Tauri
// app shows no way to subscribe.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test } from "vitest";
import { accountFixture, instanceFixture, type Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler, withViewer } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { checkoutUrl, ownSubscriptionFixture, portalUrl, supporterPlanFixture } from "./test/handlers";

const basicId = "0192f0c4-7a8e-7000-8000-0000000000c1";
const goldId = "0192f0c4-7a8e-7000-8000-0000000000c2";
const basic = supporterPlanFixture(basicId, { name: "Basic", rank: 1 });
const gold = supporterPlanFixture(goldId, {
  name: "ゴールド",
  rank: 2,
  description: "Thanks in the about page",
  monthly_price: { amount: "1000", currency: "JPY" },
  yearly_price: { amount: "10800", currency: "JPY" },
});

/** The instance with the program enabled, the list published to `list`, and its commerce disclosures. */
function enabledInstance(list: Schemas["SupporterListVisibility"] = "off") {
  return http.get(`${apiBase}/instance`, () =>
    HttpResponse.json({
      ...instanceFixture,
      supporters_enabled: true,
      supporter_list_visibility: list,
      commerce_disclosure_url: "http://localhost:3000/about/commerce",
    }),
  );
}

function plansHandler(items: Schemas["SupporterPlan"][] = [basic, gold]) {
  return http.get(`${apiBase}/instance/supporter_plans`, () => HttpResponse.json({ items }));
}

/** The server's subscription of the viewer, which the commands change, with the requests it received. */
function subscriptionServer(initial: Schemas["OwnSupporterSubscription"]) {
  let own = initial;
  const requests: { path: string; body: unknown }[] = [];
  const answer = (path: string, next: (s: Schemas["SupporterSubscription"]) => Schemas["SupporterSubscription"]) =>
    http.post(`${apiBase}${path}`, async ({ request }) => {
      const text = await request.text();
      requests.push({ path, body: text === "" ? undefined : JSON.parse(text) });
      return withViewer(request, () => {
        if (own.subscription === undefined) return problem(409, "/api/v1/problems/subscription-ended", "Ended");
        own = { subscription: next(own.subscription) };
        return HttpResponse.json(own.subscription);
      });
    });
  server.use(
    http.get(`${apiBase}/supporter/subscription`, ({ request }) => withViewer(request, () => HttpResponse.json(own))),
    answer("/supporter/subscription/cancel", (s) => ({ ...s, state: "canceling" })),
    answer("/supporter/subscription/resume", (s) => ({ ...s, state: "active" })),
    answer("/supporter/subscription/cancel_scheduled_change", (s) => {
      const rest = { ...s };
      delete rest.scheduled_change;
      return rest;
    }),
    answer("/supporter/subscription/change", (s) => ({
      ...s,
      plan_id: goldId,
      plan_name: gold.name,
      price: gold.monthly_price,
    })),
    http.post(`${apiBase}/supporter/checkout`, async ({ request }) => {
      requests.push({ path: "/supporter/checkout", body: await request.json() });
      return withViewer(request, () => HttpResponse.json({ subscription_id: "sub", redirect_url: checkoutUrl }));
    }),
    http.post(`${apiBase}/supporter/subscription/sync`, ({ request }) => {
      requests.push({ path: "/supporter/subscription/sync", body: undefined });
      return withViewer(request, () => HttpResponse.json(own));
    }),
  );
  return { requests };
}

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

afterEach(() => {
  delete (globalThis as { isTauri?: unknown }).isTauri;
});

describe("the page of the supporter plans", () => {
  test("is in the settings menu only while the program is enabled", async () => {
    await renderApp({ path: "/settings" });
    const menu = await screen.findByRole("navigation", { name: "Settings pages" });
    expect(within(menu).getByRole("link", { name: "Profile" })).toBeInTheDocument();
    expect(within(menu).queryByRole("link", { name: "Supporter plans" })).not.toBeInTheDocument();
  });

  test("subscribes through the final confirmation to the provider's checkout", async () => {
    server.use(enabledInstance(), plansHandler());
    const { requests } = subscriptionServer({});
    const { user, leftTo } = await renderApp({ path: "/settings" });
    await user.click(await screen.findByRole("link", { name: "Supporter plans" }));

    const plans = await screen.findByRole("radiogroup", { name: "Plan" });
    const goldRadio = within(plans).getByRole("radio", { name: "ゴールド" });
    expect(goldRadio).toHaveAccessibleDescription(/¥1,000 a month or ¥10,800 a year/u);
    await user.click(goldRadio);
    await user.click(
      within(screen.getByRole("radiogroup", { name: "Payment" })).getByRole("radio", { name: "Yearly" }),
    );
    await user.click(screen.getByRole("button", { name: "Subscribe" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Subscribe to ⁨ゴールド⁩?" });
    expect(within(dialog).getByText("¥10,800 a year, the same at every renewal")).toBeInTheDocument();
    expect(within(dialog).getByText("Now, then every year")).toBeInTheDocument();
    expect(within(dialog).getByText(/The benefits stay until the end of the period you paid for/u)).toBeInTheDocument();
    expect(within(dialog).getByRole("link", { name: /Commerce disclosures/u })).toHaveAttribute(
      "href",
      "/about/commerce",
    );
    expect(leftTo).toEqual([]);
    await user.click(within(dialog).getByRole("button", { name: "Go to payment" }));

    await waitFor(() => {
      expect(leftTo.map(String)).toEqual([checkoutUrl]);
    });
    expect(requests).toEqual([{ path: "/supporter/checkout", body: { plan_id: goldId, interval: "year" } }]);
  });

  test("tells a user with an unverified address why they cannot subscribe", async () => {
    server.use(enabledInstance(), plansHandler());
    subscriptionServer({ ineligible_reason: "email_unverified" });
    await renderApp({ path: "/settings/supporter" });

    expect(await screen.findByText(/first verify your email address/u)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Subscribe" })).not.toBeInTheDocument();
  });

  test("says that the server offers no plans while the program is disabled", async () => {
    server.use(
      http.get(`${apiBase}/instance/supporter_plans`, () => problem(404, "about:blank", "Not Found")),
      http.get(`${apiBase}/supporter/subscription`, () => problem(404, "about:blank", "Not Found")),
    );
    await renderApp({ path: "/settings/supporter" });

    expect(
      await screen.findByRole("heading", { name: "This server does not offer supporter plans." }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("radiogroup", { name: "Plan" })).not.toBeInTheDocument();
  });

  test("re-reads the subscription when the checkout returns, and waits for its confirmation", async () => {
    server.use(enabledInstance(), plansHandler());
    const { requests } = subscriptionServer({
      subscription: ownSubscriptionFixture(basicId, { state: "pending", plan_name: "Basic" }),
    });
    // The provider adds its checkout's ID under a key of its own.
    await renderApp({ path: "/settings/supporter?checkout=success&checkout_id=cs_test_1" });

    expect(await screen.findByText(/Confirming your payment with the payment provider/u)).toBeInTheDocument();
    expect(requests.map((r) => r.path)).toEqual(["/supporter/subscription/sync"]);
  });

  test("thanks the user once the checkout's subscription is confirmed", async () => {
    server.use(enabledInstance(), plansHandler());
    subscriptionServer({ subscription: ownSubscriptionFixture(basicId, { plan_name: "Basic" }) });
    await renderApp({ path: "/settings/supporter?checkout=success" });

    expect(await screen.findByText("Thank you for your support. Your subscription is confirmed.")).toBeInTheDocument();
  });

  test("says that nothing was charged when the user left the checkout", async () => {
    server.use(enabledInstance(), plansHandler());
    subscriptionServer({});
    await renderApp({ path: "/settings/supporter?checkout=canceled" });

    expect(await screen.findByText("You left the payment page. Nothing was charged.")).toBeInTheDocument();
  });

  test("cancels at the end of the period after a dialog, and resumes", async () => {
    server.use(enabledInstance(), plansHandler());
    const { requests } = subscriptionServer({ subscription: ownSubscriptionFixture(basicId, { plan_name: "Basic" }) });
    const { user } = await renderApp({ path: "/settings/supporter" });

    const section = await screen.findByRole("region", { name: "Your subscription" });
    expect(within(section).getByText("¥500 a month")).toBeInTheDocument();
    expect(within(section).getByText("Renews on")).toBeInTheDocument();
    await user.click(within(section).getByRole("button", { name: "Cancel the subscription" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Cancel ⁨Basic⁩?" });
    expect(dialog).toHaveAccessibleDescription(/the benefits stay until then/u);
    await user.click(within(dialog).getByRole("button", { name: "Cancel the subscription" }));

    expect(
      await within(section).findByText("Cancelled the subscription at the end of the period."),
    ).toBeInTheDocument();
    expect(within(section).getByText("Ends on")).toBeInTheDocument();
    await user.click(within(section).getByRole("button", { name: "Resume the subscription" }));
    expect(await within(section).findByText("Resumed the subscription.")).toBeInTheDocument();
    expect(requests.map((r) => r.path)).toEqual(["/supporter/subscription/cancel", "/supporter/subscription/resume"]);
  });

  test("changes to a higher plan at once after saying so", async () => {
    server.use(enabledInstance(), plansHandler());
    const { requests } = subscriptionServer({ subscription: ownSubscriptionFixture(basicId, { plan_name: "Basic" }) });
    const { user } = await renderApp({ path: "/settings/supporter" });

    const change = await screen.findByRole("region", { name: "Change the plan" });
    expect(within(change).getByRole("button", { name: "Change the plan" })).toBeDisabled();
    await user.click(within(change).getByRole("radio", { name: "ゴールド" }));
    await user.click(within(change).getByRole("button", { name: "Change the plan" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Change to ⁨ゴールド⁩?" });
    expect(dialog).toHaveAccessibleDescription(/The change starts now/u);
    await user.click(within(dialog).getByRole("button", { name: "Change the plan" }));

    expect(await screen.findByText("Changed the plan.")).toBeInTheDocument();
    expect(requests).toEqual([
      { path: "/supporter/subscription/change", body: { plan_id: goldId, interval: "month" } },
    ]);
  });

  test("withdraws a scheduled change", async () => {
    server.use(enabledInstance(), plansHandler());
    const { requests } = subscriptionServer({
      subscription: ownSubscriptionFixture(goldId, {
        plan_name: gold.name,
        scheduled_change: {
          plan_id: basicId,
          plan_name: "Basic",
          interval: "month",
          price: basic.monthly_price,
          effective_at: "2026-11-20T00:00:00Z",
        },
      }),
    });
    const { user } = await renderApp({ path: "/settings/supporter" });

    expect(await screen.findByText(/To ⁨Basic⁩ at ¥500 a month on/u)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Withdraw the scheduled change" }));
    expect(await screen.findByText("Withdrew the scheduled change.")).toBeInTheDocument();
    expect(screen.queryByText(/To ⁨Basic⁩ at/u)).not.toBeInTheDocument();
    expect(requests.map((r) => r.path)).toEqual(["/supporter/subscription/cancel_scheduled_change"]);
  });

  test("asks to fix a failed payment and opens the provider's portal", async () => {
    server.use(enabledInstance(), plansHandler());
    subscriptionServer({
      subscription: ownSubscriptionFixture(basicId, { state: "past_due", grace_ends_at: "2026-10-27T00:00:00Z" }),
    });
    const { user, leftTo } = await renderApp({ path: "/settings/supporter" });

    expect(await screen.findByText(/A payment failed\. Update your payment method/u)).toBeInTheDocument();
    expect(screen.getByText("Benefits end on")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Payment methods and invoices" }));
    await waitFor(() => {
      expect(leftTo.map(String)).toEqual([portalUrl]);
    });
  });

  test("lets a pending subscription continue to the checkout", async () => {
    server.use(enabledInstance(), plansHandler());
    const { requests } = subscriptionServer({
      subscription: ownSubscriptionFixture(basicId, { state: "pending", interval: "year" }),
    });
    const { user, leftTo } = await renderApp({ path: "/settings/supporter" });

    await user.click(await screen.findByRole("button", { name: "Continue to payment" }));
    await waitFor(() => {
      expect(leftTo.map(String)).toEqual([checkoutUrl]);
    });
    expect(requests).toEqual([{ path: "/supporter/checkout", body: { plan_id: basicId, interval: "year" } }]);
  });
});

describe("the Tauri app", () => {
  test("has no entry to the plans in the settings menu", async () => {
    (globalThis as { isTauri?: unknown }).isTauri = true;
    server.use(enabledInstance(), plansHandler());
    await renderApp({ path: "/settings" });

    const menu = await screen.findByRole("navigation", { name: "Settings pages" });
    expect(within(menu).getByRole("link", { name: "Profile" })).toBeInTheDocument();
    expect(within(menu).queryByRole("link", { name: "Supporter plans" })).not.toBeInTheDocument();
  });

  test("shows that the page of the plans is not available, without reading the plans", async () => {
    (globalThis as { isTauri?: unknown }).isTauri = true;
    let planRequests = 0;
    server.use(
      enabledInstance(),
      http.get(`${apiBase}/instance/supporter_plans`, () => {
        planRequests += 1;
        return HttpResponse.json({ items: [basic] });
      }),
    );
    await renderApp({ path: "/settings/supporter" });

    expect(await screen.findByRole("heading", { name: "This page is not available in the app." })).toBeInTheDocument();
    expect(screen.queryByRole("radiogroup", { name: "Plan" })).not.toBeInTheDocument();
    expect(planRequests).toBe(0);
  });

  test("leaves the plans out of the about page, but links to the supporter list", async () => {
    (globalThis as { isTauri?: unknown }).isTauri = true;
    server.use(enabledInstance("public"), plansHandler());
    await renderApp({ path: "/about" });

    const section = await screen.findByRole("region", { name: "Supporters" });
    expect(within(section).getByRole("link", { name: "See the supporter list" })).toHaveAttribute(
      "href",
      "/about/supporters",
    );
    expect(within(section).queryByRole("link", { name: "See the plans and subscribe" })).not.toBeInTheDocument();
    expect(within(section).queryByText("Basic")).not.toBeInTheDocument();
  });
});

describe("the about page", () => {
  test("shows the plans with the way to subscribe while the program is enabled", async () => {
    server.use(enabledInstance(), plansHandler());
    await renderApp({ path: "/about" });

    const section = await screen.findByRole("region", { name: "Supporters" });
    expect(await within(section).findByText("ゴールド")).toHaveAttribute("dir", "auto");
    expect(within(section).getByText("¥1,000 a month or ¥10,800 a year")).toBeInTheDocument();
    expect(within(section).getByRole("link", { name: "See the plans and subscribe" })).toHaveAttribute(
      "href",
      "/settings/supporter",
    );
    expect(within(section).queryByRole("link", { name: "See the supporter list" })).not.toBeInTheDocument();
  });

  test("has no supporter section while the program is disabled and the list unpublished", async () => {
    await renderApp({ path: "/about" });

    expect(await screen.findByRole("heading", { level: 1, name: "About my example" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Supporters" })).not.toBeInTheDocument();
  });
});

describe("the supporter list", () => {
  test("lists the members by plan, with their names and handles only", async () => {
    const carol = accountFixture(3, "Carol", instanceFixture.domain);
    const dave = accountFixture(4, "Dave", instanceFixture.domain);
    server.use(
      enabledInstance("public"),
      http.get(`${apiBase}/instance/supporters`, () =>
        HttpResponse.json({
          plans: [
            { id: basicId, name: "Basic" },
            { id: goldId, name: "ゴールド" },
          ],
          items: [
            { plan_id: goldId, account: carol },
            { plan_id: basicId, account: dave },
          ],
        }),
      ),
    );
    await renderApp({ path: "/about/supporters" });

    const goldGroup = await screen.findByRole("region", { name: "ゴールド" });
    expect(within(goldGroup).getByRole("link", { name: "Carol @carol" })).toHaveAttribute("href", "/@carol");
    const basicGroup = screen.getByRole("region", { name: "Basic" });
    expect(within(basicGroup).getByRole("link", { name: "Dave @dave" })).toBeInTheDocument();
    expect(screen.queryByText(/¥/u)).not.toBeInTheDocument();
  });

  test("says that the list is not published (404)", async () => {
    server.use(http.get(`${apiBase}/instance/supporters`, () => problem(404, "about:blank", "Not Found")));
    await renderApp({ path: "/about/supporters" });

    expect(
      await screen.findByRole("heading", { name: "This server does not publish a supporter list." }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "About this server" })).toHaveAttribute("href", "/about");
  });

  test("asks a visitor without a session to sign in (401)", async () => {
    server.use(
      http.post("/oauth/token/refresh", () => HttpResponse.json({ error: "invalid_grant" }, { status: 400 })),
      http.get(`${apiBase}/instance/supporters`, () =>
        problem(401, "/api/v1/problems/unauthenticated", "Unauthenticated"),
      ),
    );
    await renderApp({ path: "/about/supporters" });

    expect(await screen.findByText("Only signed-in users can read the supporter list.")).toBeInTheDocument();
    expect(within(screen.getByRole("main")).getByRole("link", { name: "Sign in" })).toHaveAttribute(
      "href",
      "/login?return_to=%2Fabout%2Fsupporters",
    );
  });
});

describe("the choice to be on the supporter list", () => {
  test("is a switch on the profile settings of a member while the list is published", async () => {
    const saved: boolean[] = [];
    server.use(
      enabledInstance("signed_in"),
      http.put(`${apiBase}/preferences/supporter_listing`, async ({ request }) => {
        const { listed } = (await request.json()) as Schemas["SupporterListing"];
        saved.push(listed);
        return withViewer(request, () => HttpResponse.json({ listed }));
      }),
    );
    const { user } = await renderApp({ path: "/settings/profile" });

    const section = await screen.findByRole("region", { name: "Supporter list" });
    expect(within(section).getByText(/never an amount or a date/u)).toBeInTheDocument();
    const toggle = within(section).getByRole("switch", { name: "List me on the supporter list" });
    expect(toggle).not.toBeChecked();
    await user.click(toggle);

    await waitFor(() => {
      expect(toggle).toBeChecked();
    });
    expect(saved).toEqual([true]);
  });

  test("is not offered to a user who is not a member", async () => {
    server.use(
      enabledInstance("public"),
      http.get(`${apiBase}/preferences/supporter_listing`, () => problem(404, "about:blank", "Not Found")),
    );
    await renderApp({ path: "/settings/profile" });

    expect(await screen.findByRole("heading", { level: 1, name: "Profile" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Supporter list" })).not.toBeInTheDocument();
  });
});
