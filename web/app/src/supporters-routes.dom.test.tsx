// SPDX-License-Identifier: AGPL-3.0-only

// The supporter's side (ADR-0072 §6–§8, §13, ADR-0162 §13; FR-911, FR-914, FR-918), on the fake API: what a story
// cannot reach, as it follows a press. Choosing a plan asks first, saying what subscribing does, then leaves for the
// provider's checkout, whose return the settings say (paid, or left unpaid) and take off the address; a switch to a
// higher plan applies now and one to the monthly price at the period's end, each said; cancelling asks, keeps the
// perks to the period's end and can be resumed; keeping the plan cancels the scheduled change; the listing saves at
// once; a tip of the amount chosen leaves for its checkout and its return says thanks, and the rate limit says how long
// to wait. What each state looks like is the stories' (supporters.stories.tsx).

import { expect, test } from "bun:test";
import type { FakeSupporterOptions } from "@my/e2e/fake";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

/** The path and query of the last address the app left for. */
const leftTo = (app: TestApp) => {
  const href = app.left().at(-1);
  if (href === undefined) throw new Error("test: the app left for nothing");
  const url = new URL(href);
  return `${url.pathname}${url.search}`;
};

/**
 * Renders the app at `path` on a fake whose supporters are `supporters`, with the tips and the list on; `comeBack` loads
 * the app anew at the address it last left for, on the same server, as the provider's return does.
 */
async function at(path: string, supporters: FakeSupporterOptions = {}) {
  const on = fakeApp({ supporters: { tips: {}, list: "everyone", ...supporters } });
  const app = await renderApp(path, { on });
  const comeBack = async () => {
    await waitFor(() => {
      expect(app.left()).toHaveLength(1);
    });
    const href = leftTo(app);
    cleanup();
    return { app: await renderApp(href, { on }), href };
  };
  return Object.assign(app, { comeBack });
}

/** Waits until the page says `sentence` in its main or in one of its live regions. */
const says = (sentence: string, timeout = 1000) =>
  waitFor(
    () => {
      const said = [words(screen.getByRole("main")), ...screen.queryAllByRole("status").map((s) => words(s))];
      if (!said.some((text) => text.includes(sentence))) throw new Error(`test: the page says ${JSON.stringify(said)}`);
    },
    { timeout },
  );

/** Presses `name` in the question over the page, once it is there. */
async function confirm(app: TestApp, name: string) {
  const dialog = await screen.findByRole("alertdialog");
  await app.user.click(within(dialog).getByRole("button", { name }));
}

test("Choose asks what subscribing does, then leaves for the checkout, whose return the settings say and take off", async () => {
  const app = await at("/about/supporters");
  await app.user.click(await main().findByRole("button", { name: "Choose Supporter" }));
  const dialog = await screen.findByRole("alertdialog", { name: "Subscribe to Supporter, monthly?" });
  expect(words(dialog)).toContain("You pay ¥500 now, then ¥500 every month until you cancel.");
  await confirm(app, "Go to payment");
  const { app: returned, href } = await app.comeBack();
  expect(href.startsWith("/settings/supporter?checkout=success")).toBe(true);
  // The server asked the provider as the screen synced: the screen reads it again 2 s later, while it settles.
  await says("Your payment is being confirmed.");
  await says("Thank you. You support this server on Supporter, monthly.", 5000);
  await waitFor(() => {
    expect(returned.path()).toBe("/settings/supporter");
  });
  expect(app.fake.supporters.subscription()?.state).toBe("active");
});

test("a checkout left unpaid is said, and nothing is subscribed", async () => {
  const app = await at("/about/supporters", { checkout: "canceled" });
  await app.user.click(await main().findByRole("button", { name: "Choose Patron" }));
  await confirm(app, "Go to payment");
  await app.comeBack();
  await says("You left the payment page. Nothing was charged.");
  expect(app.fake.supporters.subscription()?.state).toBe("pending");
});

test("a switch to a higher plan applies now, and is said where the focus goes", async () => {
  const app = await at("/about/supporters", { subscription: "active" });
  await app.user.click(await main().findByRole("button", { name: "Switch Patron" }));
  await screen.findByRole("alertdialog", { name: "Change to Patron, monthly, now?" });
  await confirm(app, "Change now");
  await says("Your plan is now Patron, monthly.");
  expect(app.fake.supporters.subscription()?.plan_id).toBe(app.fake.supporters.planId("patron"));
  await waitFor(() => {
    const focused = app.focused();
    expect(focused === null ? "" : words(focused as HTMLElement)).toBe("Your plan is now Patron, monthly.");
  });
});

test("a switch to the monthly price waits for the period's end", async () => {
  const app = await at("/about/supporters", { subscription: "yearly" });
  // The reader pays yearly: the yearly prices show, theirs marked; Monthly shows the switch to it.
  await app.user.click(await main().findByRole("radio", { name: "Monthly" }));
  await app.user.click(await main().findByRole("button", { name: "Switch Supporter" }));
  await confirm(app, "Schedule the change");
  await says("Your plan changes to Supporter, monthly, on");
  expect(app.fake.supporters.subscription()?.scheduled_change?.interval).toBe("month");
});

test("cancelling asks, keeps the perks to the period's end, and Resume takes it back", async () => {
  const app = await at("/settings/supporter", { subscription: "active" });
  await app.user.click(await main().findByRole("button", { name: "Cancel subscription…" }));
  await screen.findByRole("alertdialog", { name: "Cancel your subscription?" });
  await confirm(app, "Cancel subscription");
  await says("Your subscription ends on");
  expect(app.fake.supporters.subscription()?.state).toBe("canceling");
  await app.user.click(await main().findByRole("button", { name: "Resume" }));
  await says("Your subscription goes on.");
  expect(app.fake.supporters.subscription()?.state).toBe("active");
});

test("Keep my plan cancels the change scheduled for the period's end", async () => {
  const app = await at("/settings/supporter", { subscription: "scheduled" });
  await app.user.click(await main().findByRole("button", { name: "Keep my plan" }));
  await says("You keep Patron, monthly.");
  expect(app.fake.supporters.subscription()?.scheduled_change).toBeUndefined();
});

test("the reader's choice to be listed saves at once, and lists them under their plan", async () => {
  const app = await at("/settings/supporter", { subscription: "active" });
  const listed = (await main().findByRole("switch", { name: "List me among the supporters" })) as HTMLInputElement;
  expect(listed.checked).toBe(false);
  await app.user.click(listed);
  await waitFor(() => {
    expect(listed.checked).toBe(true);
  });
  await app.navigate("/about/supporters");
  await waitFor(() => {
    expect(words(main().getByRole("list", { name: "Supporter" }))).toContain("yuki");
  });
});

test("a tip of the amount chosen leaves for its checkout, and its return says thanks", async () => {
  const app = await at("/settings/supporter/tip");
  await app.user.click(await main().findByRole("radio", { name: "¥1,000" }));
  await app.user.click(screen.getByRole("button", { name: "Tip ¥1,000" }));
  const { app: returned } = await app.comeBack();
  // An open tip is read again 2 s later, the server asking the provider at each read.
  await says("Thank you for your tip of ¥1,000", 5000);
  await waitFor(() => {
    expect(returned.path()).toBe("/settings/supporter/tip");
  });
});

test("the rate limit of the tips says how long to wait", async () => {
  const app = await at("/settings/supporter/tip", { tips: { limited: true } });
  await app.user.click(await screen.findByRole("button", { name: "Tip ¥500" }));
  await says("You started too many tips. Try again in 30 minutes.");
  expect(app.left()).toHaveLength(0);
});

test("a change from a hidden plan to a lower public one is said as at the period's end, by the subscription's rank", async () => {
  const app = await at("/about/supporters", { subscription: "hidden" });
  // Founder (rank 3) is not among the public plans: Patron (rank 2) is lower, whatever the list holds.
  await app.user.click(await main().findByRole("button", { name: "Switch Patron" }));
  expect(words(await screen.findByRole("alertdialog"))).toContain("Change to Patron, monthly, on");
  await confirm(app, "Schedule the change");
  await says("Your plan changes to Patron, monthly, on");
  expect(app.fake.supporters.subscription()?.plan_id).not.toBe(app.fake.supporters.planId("patron"));
  expect(app.fake.supporters.subscription()?.scheduled_change?.plan_id).toBe(app.fake.supporters.planId("patron"));
});

test("a change whose answer was lost reads the subscription again, and asked again says it is the plan already", async () => {
  const app = await at("/about/supporters", { subscription: "active" });
  const lost = app.fake.script("POST /supporter/subscription/change", { times: 1, hold: "response", fail: "network" });
  await app.user.click(await main().findByRole("button", { name: "Switch Patron" }));
  await confirm(app, "Change now");
  await lost.served();
  lost.release();
  // The question says it could not be done; the change was made, and asked again the server says it is the plan.
  await within(await screen.findByRole("alertdialog")).findByRole("alert");
  await confirm(app, "Change now");
  await says("Patron, monthly, is your plan already.");
  await waitFor(() => {
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });
  await waitFor(() => {
    expect(main().getByRole("listitem", { name: "Patron" }).getAttribute("aria-current")).toBe("true");
  });
});

test("the change of a plan renewing at an earlier price says that price is lost", async () => {
  const app = await at("/about/supporters", { subscription: "kept" });
  await app.user.click(await main().findByRole("button", { name: "Switch Patron" }));
  expect(words(await screen.findByRole("alertdialog"))).toContain("You now pay ¥400, an earlier price kept for you.");
});

test("a payment past due offers Update payment and no cancel", async () => {
  await at("/settings/supporter", { subscription: "past-due" });
  await main().findByRole("button", { name: "Update payment" });
  expect(main().queryByRole("button", { name: "Cancel subscription…" })).toBeNull();
});

test("cancelling a subscription whose change is scheduled says the change goes too", async () => {
  const app = await at("/settings/supporter", { subscription: "scheduled" });
  await app.user.click(await main().findByRole("button", { name: "Cancel subscription…" }));
  expect(words(await screen.findByRole("alertdialog"))).toContain(
    "The change to Supporter, monthly, scheduled for then is cancelled too.",
  );
});

test("a tip refused as the reader may not says the server's reason", async () => {
  const app = await at("/settings/supporter/tip", { program: false, tips: {} });
  app.fake.script("POST /supporter/tips", {
    fail: {
      type: "/api/v1/problems/supporter-ineligible",
      title: "You cannot support this server",
      status: 422,
      detail: "Confirm your email address first.",
    },
  });
  await app.user.click(await screen.findByRole("button", { name: "Tip ¥500" }));
  await says("Confirm your email address first.");
});
