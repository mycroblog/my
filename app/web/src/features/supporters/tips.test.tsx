// SPDX-License-Identifier: AGPL-3.0-only
// The tips through the whole app (web-client/README.md T4, ADR-0162 §1, §3, §6, §7, §13, §17, §19): MSW plays the
// server; a visitor on the information page learns who may tip and signs in; a user chooses an amount, reads the final
// confirmation and goes to the provider's checkout, comes back and reads only the thanks once the tip is paid; a user
// whose address is not verified reads how to become able to tip instead of the form; the words never speak of a
// donation or a deduction; the Tauri app shows none of it.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test } from "vitest";
import { instanceFixture } from "../../test/fixtures";
import { apiBase, problem, signedInHandler, withViewer } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { checkoutUrl, ownTipFixture, tipAmounts, tipId } from "./test/handlers";

/** The instance with the program enabled and the tips taken, with its commerce disclosures. */
const tipsInstance = http.get(`${apiBase}/instance`, () =>
  HttpResponse.json({
    ...instanceFixture,
    supporters_enabled: true,
    tips_enabled: true,
    commerce_disclosure_url: "http://localhost:3000/about/commerce",
  }),
);

const amountsHandler = http.get(`${apiBase}/instance/tip_amounts`, () => HttpResponse.json(tipAmounts));

const signedOutHandler = http.post("/oauth/token/refresh", () =>
  HttpResponse.json({ error: "invalid_grant" }, { status: 400 }),
);

/** Words that a tip's text never uses: it is a tip for running the server, not a donation, and nothing is deductible. */
const donationWords = /donat|deduct|charity|fundrais/iu;

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

afterEach(() => {
  delete (globalThis as { isTauri?: unknown }).isTauri;
});

describe("the tips on the about page", () => {
  test("tell a visitor who is not signed in who may tip, and the way to sign in", async () => {
    server.use(signedOutHandler, tipsInstance);
    await renderApp({ path: "/about" });

    const section = await screen.findByRole("region", { name: "Tips" });
    const tips = within(section);
    expect(tips.getByText("Users of this server with a verified email address can tip.")).toBeInTheDocument();
    expect(tips.getByRole("link", { name: "Sign in to leave a tip" })).toHaveAttribute(
      "href",
      "/login?return_to=%2Fsettings%2Fsupporter%2Ftip",
    );
    expect(tips.queryByRole("link", { name: "Leave a tip" })).not.toBeInTheDocument();
    expect(section).not.toHaveTextContent(donationWords);
  });

  test("link a signed-in user to the page of the tips", async () => {
    server.use(signedInHandler, tipsInstance);
    await renderApp({ path: "/about" });

    expect(await screen.findByRole("link", { name: "Leave a tip" })).toHaveAttribute("href", "/settings/supporter/tip");
  });

  test("are absent while the server takes no tips", async () => {
    server.use(signedOutHandler);
    await renderApp({ path: "/about" });

    expect(await screen.findByRole("heading", { name: /About/u, level: 1 })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Tips" })).not.toBeInTheDocument();
  });
});

describe("the page of the tips", () => {
  beforeEach(() => {
    server.use(signedInHandler);
  });

  test("tips an amount through the final confirmation to the provider's checkout", async () => {
    const sent: unknown[] = [];
    server.use(
      tipsInstance,
      amountsHandler,
      http.post(`${apiBase}/supporter/tips`, async ({ request }) => {
        sent.push(await request.json());
        return withViewer(request, () => HttpResponse.json({ tip_id: tipId, redirect_url: checkoutUrl }));
      }),
    );
    const { user, leftTo } = await renderApp({ path: "/settings/supporter/tip" });

    const amounts = await screen.findByRole("radiogroup", { name: "Amount" });
    expect(within(amounts).getByRole("radio", { name: "¥300" })).toBeChecked();
    expect(screen.getByText(/Nothing is given in return/u)).toBeInTheDocument();
    await user.click(within(amounts).getByRole("radio", { name: "¥1,000" }));
    await user.click(screen.getByRole("button", { name: "Leave a tip" }));

    const dialog = await screen.findByRole("alertdialog", { name: /Tip ¥1,000 for running/u });
    expect(within(dialog).getByText("Once, right now. It does not renew.")).toBeInTheDocument();
    expect(within(dialog).getByText("Nothing. A tip gives no benefit, role, or badge.")).toBeInTheDocument();
    expect(within(dialog).getByRole("link", { name: /Commerce disclosures/u })).toHaveAttribute(
      "href",
      "/about/commerce",
    );
    expect(dialog.textContent).not.toMatch(donationWords);
    expect(leftTo).toEqual([]);
    await user.click(within(dialog).getByRole("button", { name: "Go to payment" }));

    await waitFor(() => {
      expect(leftTo.map(String)).toEqual([checkoutUrl]);
    });
    expect(sent).toEqual([{ amount: { amount: "1000", currency: "JPY" } }]);
  });

  test("tells a user with an unverified address how to become able to tip, instead of the form", async () => {
    server.use(
      tipsInstance,
      amountsHandler,
      http.get(`${apiBase}/supporter/subscription`, ({ request }) =>
        withViewer(request, () => HttpResponse.json({ ineligible_reason: "email_unverified" })),
      ),
    );
    await renderApp({ path: "/settings/supporter/tip" });

    expect(
      await screen.findByText(
        "To tip, first verify your email address with the link in the email the server sent you.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("radiogroup", { name: "Amount" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Leave a tip" })).not.toBeInTheDocument();
  });

  test("shows only the thanks once the tip that the checkout returned from is paid", async () => {
    let reads = 0;
    server.use(
      tipsInstance,
      amountsHandler,
      http.get(`${apiBase}/supporter/tips/:id`, ({ request }) =>
        withViewer(request, () => {
          reads += 1;
          return HttpResponse.json(
            reads === 1 ? ownTipFixture() : ownTipFixture({ state: "paid", paid_at: "2026-10-22T00:01:00Z" }),
          );
        }),
      ),
    );
    await renderApp({ path: `/settings/supporter/tip?tip=${tipId}&checkout=success` });

    expect(await screen.findByText(/Confirming your payment with the payment provider/u)).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "Thank you" }, { timeout: 5000 })).toBeInTheDocument();
    expect(screen.getByText(/Your tip of ¥500 goes to running/u)).toBeInTheDocument();
    expect(screen.queryByRole("radiogroup", { name: "Amount" })).not.toBeInTheDocument();
    expect(screen.getByRole("main").textContent).not.toMatch(donationWords);
  });

  test("says that nothing was charged when the user left the checkout, and offers the amounts again", async () => {
    server.use(tipsInstance, amountsHandler);
    await renderApp({ path: `/settings/supporter/tip?tip=${tipId}&checkout=canceled` });

    expect(await screen.findByText("You left the payment page. Nothing was charged.")).toBeInTheDocument();
    expect(screen.getByRole("radiogroup", { name: "Amount" })).toBeInTheDocument();
  });

  test("says when the user may start again after the server's limit", async () => {
    server.use(
      tipsInstance,
      amountsHandler,
      http.post(`${apiBase}/supporter/tips`, ({ request }) =>
        withViewer(request, () =>
          problem(429, "/api/v1/problems/rate-limited", "Too Many Requests", { headers: { "Retry-After": "600" } }),
        ),
      ),
    );
    const { user, leftTo } = await renderApp({ path: "/settings/supporter/tip" });

    await user.click(await screen.findByRole("button", { name: "Leave a tip" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Go to payment" }));

    expect(await within(dialog).findByText(/Too many tips were started\. Try again after/u)).toBeInTheDocument();
    expect(leftTo).toEqual([]);
  });

  test("says that the server takes no tips", async () => {
    server.use(
      http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, supporters_enabled: true })),
    );
    await renderApp({ path: "/settings/supporter/tip" });

    expect(await screen.findByRole("heading", { name: "This server does not take tips." })).toBeInTheDocument();
  });

  test("is linked from the page of the supporter plans", async () => {
    server.use(tipsInstance);
    const { user } = await renderApp({ path: "/settings/supporter" });

    await user.click(await screen.findByRole("link", { name: "Leave a tip" }));
    expect(await screen.findByRole("heading", { name: "Tip", level: 1 })).toBeInTheDocument();
  });
});

describe("the tips in the Tauri app", () => {
  test("show that the page is not available, and no entry on the about page", async () => {
    (globalThis as { isTauri?: unknown }).isTauri = true;
    server.use(signedInHandler, tipsInstance, amountsHandler);
    await renderApp({ path: "/settings/supporter/tip" });

    expect(await screen.findByRole("heading", { name: "This page is not available in the app." })).toBeInTheDocument();
    expect(screen.queryByRole("radiogroup", { name: "Amount" })).not.toBeInTheDocument();
  });
});
