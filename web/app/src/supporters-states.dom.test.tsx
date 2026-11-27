// SPDX-License-Identifier: AGPL-3.0-only

// The supporter's side in each state, on the fake API (ADR-0072 §6–§9, §13, §14, ADR-0162 §1, §6, §13; FR-911, FR-914,
// FR-918): beside the presses of supporters-routes.dom.test.tsx, what each state says and offers. The subscription's
// line names its plan, its period and its amount in every state, and Cancel… comes last, asking first what cancelling
// does; Choose says when the perks start, how to cancel and where the commerce disclosure is, and sends nothing until
// the reader goes on; the return from the provider syncs once, takes its query off the address, and while the payment
// settles reads again 2 s apart for the first 15 reads, then slower, and says so only while it is pending; a lost
// answer reads the subscription again. Subscribing is a local reader's with a confirmed address, said in one sentence
// otherwise; the tips are the operator's amounts, with what a tip is; the list
// holds the listed readers under their plans, and the reader's switch shows while they are a member and the list is
// shown; the menu leads to the supporters beside the funding where the page has something to show.

import { expect, onTestFinished, test } from "bun:test";
import type { FakeOptions, FakeSupporterOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp, instanceOf } from "./test/server";
import { onAPhone, tabBar } from "./test/viewport";

/** The program with its two plans, the tips, and the list shown to everyone, unless a test says otherwise. */
const all: FakeSupporterOptions = { tips: {}, list: "everyone" };

/** A day as the subscription's lines write it. */
const aDay = String.raw`[A-Z][a-z]{2} \d{1,2}, \d{4}`;

/** The section of the reader's subscription in the settings. */
const subscriptionSection = () => main().findByRole("region", { name: "Your subscription" });

/** The names of the buttons inside `element`, in their order. */
const buttonsOf = (element: HTMLElement) =>
  within(element)
    .queryAllByRole("button")
    .map((button) => button.getAttribute("aria-label") ?? words(button));

/** Waits until nothing the app's data asked for is on its way, and a turn more. */
async function atRest(app: TestApp) {
  await waitFor(() => {
    expect(app.data.queryClient.isFetching()).toBe(0);
  });
  await new Promise((resolve) => setTimeout(resolve, 0));
}

// ---- The subscription's line ---------------------------------------------------------------------------------------

const lines: readonly (readonly [
  NonNullable<FakeSupporterOptions["subscription"]>,
  RegExp,
  string,
  readonly string[],
])[] = [
  ["active", new RegExp(`^Renews on ${aDay}$`), "Supporter, monthly · ¥500 a month", []],
  ["yearly", new RegExp(`^Renews on ${aDay}$`), "Supporter, yearly · ¥5,000 a year", []],
  ["kept", new RegExp(`^Renews on ${aDay}$`), "Supporter, monthly · ¥400 a month", []],
  ["hidden", new RegExp(`^Renews on ${aDay}$`), "Founder, monthly · ¥3,000 a month", []],
  ["canceling", new RegExp(`^Ends on ${aDay}$`), "Supporter, monthly · ¥500 a month", ["Resume"]],
  [
    "scheduled",
    new RegExp(`^Changes to Supporter, monthly, on ${aDay}$`),
    "Patron, monthly · ¥1,500 a month until then, ¥500 a month after",
    ["Keep my plan"],
  ],
  [
    "past-due",
    new RegExp(`^Your payment failed: update your payment method before ${aDay}, or your perks end then$`),
    "Supporter, monthly · ¥500 a month",
    ["Update payment"],
  ],
  [
    "lapsed",
    /^Your payment failed, and your perks have ended$/,
    "Supporter, monthly · ¥500 a month",
    ["Update payment"],
  ],
];

test.each(lines)(
  "the subscription's line, %s, names its state, its plan, its period and its amount",
  async (subscription, state, terms, offers) => {
    await renderApp("/settings/supporter", { supporters: { ...all, subscription } });
    const section = await subscriptionSection();
    await within(section).findByText(state);
    expect(words(section)).toContain(terms);
    const offered = buttonsOf(section);
    for (const name of offers) expect(offered).toContain(name);
  },
);

test("a lapsed subscription says the perks ended, and nowhere that the reader supports the server", async () => {
  const app = await renderApp("/settings/supporter", { supporters: { ...all, subscription: "lapsed" } });
  await within(await subscriptionSection()).findByText(/^Your payment failed, and your perks have ended$/);
  await atRest(app);
  expect(words(screen.getByRole("main"))).not.toMatch(/you support/i);
  expect(buttonsOf(await subscriptionSection())).not.toContain("Cancel subscription…");
});

test.each(["active", "yearly", "kept", "hidden", "scheduled"] as const)(
  "Cancel… is the last of the subscription's section, %s",
  async (subscription) => {
    await renderApp("/settings/supporter", { supporters: { ...all, subscription } });
    const section = await subscriptionSection();
    await within(section).findByRole("button", { name: "Cancel subscription…" });
    expect(buttonsOf(section).at(-1)).toBe("Cancel subscription…");
  },
);

test("Cancel… asks first: not charged again, the day it ends; Keep subscription cancels nothing", async () => {
  const on = fakeApp({ supporters: { ...all, subscription: "active" } });
  const cancels = on.fake.script("POST /supporter/subscription/cancel", {});
  const app = await renderApp("/settings/supporter", { on });
  await app.user.click(await main().findByRole("button", { name: "Cancel subscription…" }));
  const dialog = await screen.findByRole("alertdialog", { name: "Cancel your subscription?" });
  const said = words(dialog);
  expect(said).toContain("You are not charged again.");
  expect(said).toMatch(new RegExp(`It ends on ${aDay}, and your perks stay until then\\.`));
  expect(cancels.seen).toHaveLength(0);
  await app.user.click(within(dialog).getByRole("button", { name: "Keep subscription" }));
  await waitFor(() => {
    expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
  });
  expect(cancels.seen).toHaveLength(0);
  expect(app.fake.supporters.subscription()?.state).toBe("active");
});

// ---- Choosing a plan -------------------------------------------------------------------------------------------------

test("Choose says when the perks start and how to cancel, and sends nothing until Go to payment", async () => {
  const on = fakeApp({ supporters: all });
  const checkouts = on.fake.script("POST /supporter/checkout", {});
  const app = await renderApp("/about/supporters", { on });
  // The yearly radio says what a year saves beside its name.
  await app.user.click(await main().findByRole("radio", { name: /^Yearly(\s.*)?$/ }));
  await app.user.click(await main().findByRole("button", { name: "Choose Patron" }));
  const dialog = await screen.findByRole("alertdialog", { name: "Subscribe to Patron, yearly?" });
  const said = words(dialog);
  expect(said).toContain("You pay ¥15,000 now, then ¥15,000 every year until you cancel.");
  expect(said).toContain("Your perks start once the payment goes through.");
  expect(said).toContain(
    "You can cancel any time under “Supporter plans and tips” in the settings, and your perks stay until the end",
  );
  // No commerce disclosure on this server: no link to one.
  expect(within(dialog).queryAllByRole("link")).toHaveLength(0);
  await app.user.click(within(dialog).getByRole("button", { name: "Not now" }));
  await waitFor(() => {
    expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
  });
  expect(checkouts.seen).toHaveLength(0);
  expect(app.left()).toHaveLength(0);
});

test("Choose links to the commerce disclosure where the server has one", async () => {
  const on = fakeApp({ supporters: all });
  on.fake.script("GET /instance", {
    change: (instance) => ({ ...instance, commerce_disclosure_url: "https://demo.test/about/commerce" }),
  });
  const app = await renderApp("/about/supporters", { on });
  await app.user.click(await main().findByRole("button", { name: "Choose Supporter" }));
  const dialog = await screen.findByRole("alertdialog", { name: "Subscribe to Supporter, monthly?" });
  const link = within(dialog).getByRole("link", { name: "Commerce disclosure" });
  expect(new URL(link.getAttribute("href") ?? "", "https://demo.test").pathname).toBe("/about/commerce");
});

// ---- Who may subscribe -----------------------------------------------------------------------------------------------

test.each([
  ["email_unverified", "Confirm your email address to subscribe."],
  ["not_local", "Only people with an account on this server can subscribe."],
  ["restricted", "Your account cannot subscribe while it is restricted."],
] as const)(
  "a reader who may not subscribe (%s) is told why in one sentence, and offered no Choose",
  async (ineligible, why) => {
    const app = await renderApp("/about/supporters", { supporters: { ...all, ineligible } });
    const plans = await main().findByRole("region", { name: "Plans" });
    await within(plans).findByText(why);
    await atRest(app);
    expect(within(plans).queryAllByRole("button", { name: /^Choose / })).toHaveLength(0);
    expect(within(plans).getAllByText(why)).toHaveLength(1);
  },
);

// ---- The return from the provider ------------------------------------------------------------------------------------

test("back from a checkout left unpaid, the settings sync once and take the query off the address", async () => {
  const on = fakeApp({ supporters: { ...all, subscription: "pending" } });
  const syncs = on.fake.script("POST /supporter/subscription/sync", {});
  const app = await renderApp("/settings/supporter?checkout=canceled", { on });
  await waitFor(() => {
    expect(app.path()).toBe("/settings/supporter");
  });
  await main().findByText("You left the payment page. Nothing was charged.");
  await atRest(app);
  expect(syncs.seen).toHaveLength(1);
});

/**
 * Keeps the app's repeating waits of `ms` or longer from now until the test ends, for the test to run one at a time:
 * the reads again of a payment that settles, which the data cache times with `setInterval`.
 */
function heldIntervals(ms: number) {
  const every = globalThis.setInterval;
  const stop = globalThis.clearInterval;
  const live = new Map<unknown, { ms: number; run: () => void }>();
  globalThis.setInterval = ((handler: TimerHandler, timeout?: number, ...rest: unknown[]) => {
    if (typeof handler !== "function" || timeout === undefined || timeout < ms) return every(handler, timeout, ...rest);
    // A timer that never comes stands for the held one: what stops it stops the held wait.
    const id = every(() => undefined, 2 ** 31 - 1);
    live.set(id, {
      ms: timeout,
      run: () => {
        (handler as (...given: unknown[]) => void)(...rest);
      },
    });
    return id;
  }) as typeof setInterval;
  globalThis.clearInterval = ((id?: Parameters<typeof clearInterval>[0]) => {
    live.delete(id);
    stop(id);
  }) as typeof clearInterval;
  onTestFinished(() => {
    for (const id of live.keys()) stop(id as Parameters<typeof clearInterval>[0]);
    globalThis.setInterval = every;
    globalThis.clearInterval = stop;
  });
  return {
    /** The lengths of the waits that repeat now. */
    live: () => [...live.values()].map((wait) => wait.ms),
    /** Runs the one wait that repeats now. */
    run: () => {
      const [only, ...more] = [...live.values()];
      if (only === undefined || more.length > 0) throw new Error(`test: ${String(live.size)} repeating waits`);
      only.run();
    },
  };
}

test("while the payment settles the settings read again 2 s apart for the first 15 reads, then slower, and say it only while it is pending", async () => {
  const waits = heldIntervals(1000);
  const on = fakeApp({ supporters: { ...all, subscription: "pending" } });
  const reads = on.fake.script("GET /supporter/subscription", {});
  const syncs = on.fake.script("POST /supporter/subscription/sync", {});
  const app = await renderApp("/settings/supporter?checkout=success&checkout_id=fake_cs_1", { on });
  const settling = "Your payment is being confirmed. This page updates once it goes through.";
  await main().findByText(settling);
  await waitFor(() => {
    expect(app.path()).toBe("/settings/supporter");
  });
  await atRest(app);
  expect(syncs.seen).toHaveLength(1);
  expect(waits.live()).toEqual([2000]);
  // The first read was the screen's own; each wait of 2 s brings the next, up to the fifteenth.
  while (reads.seen.length < 15) {
    const before = reads.seen.length;
    expect(waits.live()).toEqual([2000]);
    waits.run();
    await waitFor(() => {
      expect(reads.seen.length).toBe(before + 1);
    });
    await atRest(app);
  }
  const [later] = waits.live();
  expect(later).toBeGreaterThan(2000);
  // Read once more, slower, the subscription is active: the settling sentence goes, and no read waits any more.
  on.fake.script("GET /supporter/subscription", {
    change: (own) =>
      own.subscription === undefined
        ? own
        : { ...own, subscription: { ...own.subscription, state: "active" as const } },
  });
  waits.run();
  await main().findByText("Thank you. You support this server on Supporter, monthly.");
  await atRest(app);
  expect(within(screen.getByRole("main")).queryAllByText(settling)).toHaveLength(0);
  expect(waits.live()).toEqual([]);
  expect(syncs.seen).toHaveLength(1);
});

// ---- A change whose answer was lost -------------------------------------------------------------------------------

test("a change whose answer was lost reads the subscription again", async () => {
  const on = fakeApp({ supporters: { ...all, subscription: "active" } });
  const app = await renderApp("/about/supporters", { on });
  await main().findByRole("button", { name: "Switch Patron" });
  await atRest(app);
  const reads = on.fake.script("GET /supporter/subscription", {});
  const lost = on.fake.script("POST /supporter/subscription/change", { times: 1, hold: "response", fail: "network" });
  await app.user.click(main().getByRole("button", { name: "Switch Patron" }));
  const dialog = await screen.findByRole("alertdialog", { name: "Change to Patron, monthly, now?" });
  await app.user.click(within(dialog).getByRole("button", { name: "Change now" }));
  await lost.served();
  lost.release();
  await within(dialog).findByRole("alert");
  await waitFor(() => {
    expect(reads.seen.length).toBeGreaterThan(0);
  });
});

// ---- The tips ------------------------------------------------------------------------------------------------------

test("the tip offers the operator's amounts alone, describes them by what a tip is, and Tip names the one chosen", async () => {
  const options: FakeOptions = { supporters: all };
  const { name } = await instanceOf(options);
  const app = await renderApp("/settings/supporter/tip", options);
  const amounts = await main().findByRole("radiogroup", { name: "Amount" });
  expect(within(amounts).getAllByRole("radio")).toHaveLength(3);
  for (const amount of ["¥500", "¥1,000", "¥3,000"])
    expect(within(amounts).getByRole("radio", { name: amount })).toBeDefined();
  const described = (amounts.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .map((id) => document.getElementById(id))
    .map((element) => (element === null ? "" : words(element)))
    .join(" ");
  expect(described).toContain(
    `A tip is a one-time thank-you to the people who run ${name} for its service. Nothing comes with it in return.`,
  );
  await app.user.click(within(amounts).getByRole("radio", { name: "¥3,000" }));
  expect(await screen.findByRole("button", { name: "Tip ¥3,000" })).toBeDefined();
  expect(screen.queryAllByRole("button", { name: "Tip ¥500" })).toHaveLength(0);
});

test("one tip at a time: the screen says a new one ends the page left open, and a second press while it starts sends nothing", async () => {
  const on = fakeApp({ supporters: { ...all, tips: { open: true } } });
  const app = await renderApp("/settings/supporter/tip", { on });
  await main().findByText("Starting a tip ends any tip payment page you left open, so you pay only once.");
  const starts = on.fake.script("POST /supporter/tips", { hold: "request" });
  const tip = await screen.findByRole("button", { name: "Tip ¥500" });
  await app.user.click(tip);
  await starts.arrived();
  await app.user.click(tip);
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(starts.seen).toHaveLength(1);
  starts.release();
  await waitFor(() => {
    expect(app.left()).toHaveLength(1);
  });
});

// ---- The supporters list ---------------------------------------------------------------------------------------------

/** The names each list of the supporters' section holds, by the list's name. */
async function listed(): Promise<Record<string, string[]>> {
  const section = await main().findByRole("region", { name: "Supporters" });
  await within(section).findAllByRole("list");
  return Object.fromEntries(
    within(section)
      .getAllByRole("list")
      .map((list) => [
        list.getAttribute("aria-label") ?? "",
        within(list)
          .getAllByRole("listitem")
          .map((item) => words(within(item).getByRole("link"))),
      ]),
  );
}

test.each([
  ["everyone", false],
  ["signed_in", false],
  ["everyone", true],
] as const)(
  "the list shown to %s (a visitor: %p) holds the listed readers alone, under their plans",
  async (list, visitor) => {
    await renderApp("/about/supporters", { visitor, supporters: { ...all, list } });
    // marco is a Patron who did not choose to be listed.
    expect(await listed()).toEqual({ Supporter: ["Sam Rivera", "Hana 花"], Patron: ["Mio 澪"] });
  },
);

test.each(["signed_in", "everyone"] as const)(
  "a visitor's page, the list shown to %s, asks for an account once",
  async (list) => {
    const app = await renderApp("/about/supporters", { visitor: true, supporters: { ...all, list } });
    await main().findByRole("region", { name: "Supporters" });
    await atRest(app);
    expect(main().getAllByRole("link", { name: "Sign in" })).toHaveLength(1);
    if (list === "signed_in") {
      expect(main().getByText("Only people signed in to this server can see the supporters list.")).toBeDefined();
      expect(within(main().getByRole("region", { name: "Supporters" })).queryAllByRole("list")).toHaveLength(0);
    }
  },
);

test.each([
  ["active", "everyone", true],
  ["past-due", "signed_in", true],
  ["canceling", "everyone", true],
  ["active", "off", false],
  ["none", "everyone", false],
  ["pending", "everyone", false],
  ["lapsed", "everyone", false],
] as const)("the listing's switch, %s with the list shown to %s: offered %p", async (subscription, list, offered) => {
  const app = await renderApp("/settings/supporter", { supporters: { ...all, subscription, list } });
  await subscriptionSection();
  await atRest(app);
  expect(main().queryAllByRole("switch", { name: "List me among the supporters" })).toHaveLength(offered ? 1 : 0);
});

// ---- The menu ----------------------------------------------------------------------------------------------------------

/** The places of the tab bar's Menu, each its name and where it leads, in their order. */
async function menuPlaces(app: TestApp): Promise<string[]> {
  await app.user.click(within(tabBar()).getByRole("button", { name: /^Menu(?: \d+ unread in direct messages)?$/ }));
  const menu = await screen.findByRole("menu", { name: "Menu" });
  return within(menu)
    .getAllByRole("menuitem")
    .map((item) => `${item.getAttribute("aria-label") ?? words(item)} → ${item.getAttribute("href") ?? ""}`);
}

test.each([
  [{ supporters: all }, true],
  [{ supporters: { list: "off" } }, true],
  [{ supporters: { program: false, tips: {} } }, true],
  [{}, false],
] as const)(
  "the menu, of a server whose supporters are %j, leads to the funding and beside it to the supporters: %p",
  async (options, shown) => {
    onAPhone();
    const app = await renderApp("/lists", options);
    const places = await menuPlaces(app);
    const funding = places.indexOf("Funding → /about/costs");
    expect(funding).toBeGreaterThan(-1);
    if (shown) expect(places[funding - 1]).toBe("Supporters → /about/supporters");
    else expect(places.filter((place) => place.startsWith("Supporters"))).toHaveLength(0);
  },
);
