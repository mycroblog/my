// SPDX-License-Identifier: AGPL-3.0-only

// What each payment page shows of the data the server gave it, and what its forms post: the fake's choice of an outcome,
// and PAY.JP's card form, which checks the card holder's name before PAY.JP sees it and posts the signed state and the
// token that payjp.js gave, never a field of the card.

import { expect, onTestFinished, test } from "bun:test";
import { I18nProvider } from "@my/design-system";
import { act, cleanup, render, screen, within } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import type { PayPage } from "./data";
import { loadPayjp, payjpScript, type PayjpFactory } from "./payjp";
import { PayScreen } from "./screen";
import { moneyIn } from "./strings";

/** payjp.js, which mounts its card element where asked and gives `answer` for any card. */
function fakePayjp(answer: { id: string } | { error: { message?: string } }) {
  const calls: { publicKey: string; mounted: string[]; tokens: unknown[] } = { publicKey: "", mounted: [], tokens: [] };
  const factory: PayjpFactory = (publicKey) => {
    calls.publicKey = publicKey;
    return {
      elements: () => ({
        create: () => ({ mount: (target) => calls.mounted.push(typeof target === "string" ? target : target.id) }),
      }),
      createToken: (_element, data) => {
        calls.tokens.push(data);
        return Promise.resolve(answer as never);
      },
    };
  };
  return { load: () => Promise.resolve(factory), calls };
}

async function shown(page: PayPage, payjp: { load: () => Promise<PayjpFactory> } = fakePayjp({ id: "tok_1" })) {
  cleanup();
  await act(async () => {
    render(
      <I18nProvider locale="en">
        <PayScreen page={page} wordmark={{ light: "/light.svg", dark: "/dark.svg" }} payjp={payjp.load} />
      </I18nProvider>,
    );
    await Promise.resolve();
  });
  return within(screen.getByRole("main"));
}

const server = { name: "Hanami Town" };
const monthly = { amount: 1200, currency: "JPY", interval: "month" } as const;
const card = {
  action: "/payments/pages/payjp/checkout",
  state: "signed",
  publicKey: "pk_test",
  email: "alice@example.com",
};

test("money is written with its currency's symbol and decimals", () => {
  expect(moneyIn("en", 1200, "JPY")).toBe("¥1,200");
  expect(moneyIn("en", 1250, "USD")).toBe("$12.50");
});

test("the fake's checkout names the server, says it is a test build, and offers to pay, to fail and to cancel", async () => {
  const main = await shown({
    screen: "checkout",
    server,
    title: "Subscribe to Supporter",
    price: { amount: 500, currency: "JPY", interval: "month" },
    fake: true,
    back: "/settings/supporter",
    choices: { action: "/payments/pages/fake/checkout/cs_1" },
  });
  expect(screen.getAllByText("Hanami Town").length).toBeGreaterThan(0);
  expect(screen.getByRole("note").textContent).toMatch(/no money moves/);
  expect(main.getByRole("heading", { level: 1 }).textContent).toBe("Subscribe to Supporter");
  expect(main.getByText("¥500 a month")).toBeTruthy();
  for (const [name, value] of [
    ["Pay", "pay"],
    ["Make the payment fail", "fail"],
    ["Cancel", "cancel"],
  ] as const) {
    const button = main.getByRole<HTMLButtonElement>("button", { name });
    expect([button.name, button.value, button.form?.getAttribute("action")]).toEqual([
      "action",
      value,
      "/payments/pages/fake/checkout/cs_1",
    ]);
  }
  // Cancel is the one way out.
  expect(main.queryByRole("link")).toBeNull();
});

test("PAY.JP's checkout says what is charged, and posts the state and the token, no field of the card", async () => {
  const payjp = fakePayjp({ id: "tok_1" });
  const main = await shown({ screen: "checkout", title: "Subscribe", price: monthly, testMode: true, card }, payjp);
  expect(screen.getByRole("note").textContent).toMatch(/Test mode/);
  expect(payjp.calls.publicKey).toBe("pk_test");
  expect(payjp.calls.mounted).toEqual([".pay-card-element"]);
  expect(main.getByText("Then ¥1,200 a month.")).toBeTruthy();
  const submit = main.getByRole<HTMLButtonElement>("button", { name: "Pay ¥1,200 and subscribe" });
  const form = submit.form;
  if (form === null) throw new Error("test: the button submits no form");
  expect(form.getAttribute("action")).toBe(card.action);
  let sent: Record<string, FormDataEntryValue> | undefined;
  form.submit = () => {
    sent = Object.fromEntries(new FormData(form));
  };
  await userEvent.type(main.getByLabelText("Name on the card"), "ALICE EXAMPLE");
  await userEvent.click(submit);
  expect(payjp.calls.tokens).toEqual([
    { three_d_secure: true, card: { name: "ALICE EXAMPLE", email: "alice@example.com" } },
  ]);
  expect(sent).toEqual({ s: "signed", token: "tok_1", email: "alice@example.com", name: "ALICE EXAMPLE" });
});

test("a name that PAY.JP would refuse is said under its field before anything is sent", async () => {
  const payjp = fakePayjp({ id: "tok_1" });
  const main = await shown({ screen: "checkout", title: "Subscribe", price: monthly, card }, payjp);
  const submit = main.getByRole<HTMLButtonElement>("button", { name: "Pay ¥1,200 and subscribe" });
  let posted = false;
  if (submit.form) submit.form.submit = () => (posted = true);
  const name = main.getByLabelText<HTMLInputElement>("Name on the card");
  await userEvent.type(name, "山田太郎");
  await userEvent.click(submit);
  expect(name.getAttribute("aria-invalid")).toBe("true");
  expect(document.getElementById("pay-name-error")?.textContent).toBe(
    "Enter the name in Latin letters, as printed on the card.",
  );
  expect(document.activeElement).toBe(name);
  expect([payjp.calls.tokens, posted]).toEqual([[], false]);
});

test("a card that PAY.JP refuses is said in the catalog's words before the button, and nothing is posted", async () => {
  const main = await shown(
    { screen: "checkout", title: "Subscribe", price: monthly, card: { ...card, name: "ALICE EXAMPLE" } },
    fakePayjp({ error: { message: "カードが拒否されました" } }),
  );
  const submit = main.getByRole<HTMLButtonElement>("button", { name: "Pay ¥1,200 and subscribe" });
  let posted = false;
  if (submit.form) submit.form.submit = () => (posted = true);
  await userEvent.click(submit);
  expect(main.getByRole("alert").textContent).toMatch(/^The card could not be checked, and nothing was charged/);
  expect(main.getByRole("alert").nextElementSibling).toBe(submit);
  expect(posted).toBe(false);
});

test("without payjp.js, the card form says so and cannot be sent", async () => {
  await shown(
    { screen: "checkout", title: "Subscribe", price: monthly, card },
    { load: () => Promise.reject(new Error("blocked")) },
  );
  expect(screen.getByRole("alert").textContent).toBe(
    "The card form could not be loaded. Reload the page, or try again in a while.",
  );
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Pay ¥1,200 and subscribe" }).disabled).toBe(true);
});

test("the portal sums the subscription up first, then the card, its change, and the payments as rows", async () => {
  const main = await shown({
    screen: "portal",
    server,
    title: "Payment method and history",
    notice: "The card was changed.",
    card: { ...card, action: "/payments/pages/payjp/portal", change: true },
    back: "/settings/supporter",
    portal: {
      plan: { name: "Supporter", price: monthly, next: "2026-12-01" },
      card: "Visa ending in 4242, expires 12/2030",
      payments: [
        { date: "2026-11-01", amount: 1200, currency: "JPY" },
        { date: "2026-10-01", amount: 1200, refunded: 200, currency: "JPY" },
      ],
    },
  });
  expect(main.getByText("Supporter: ¥1,200 a month")).toBeTruthy();
  expect(main.getByText("Next payment: 2026-12-01")).toBeTruthy();
  expect(main.getByRole("status").textContent).toBe("The card was changed.");
  expect(main.getByText("Visa ending in 4242, expires 12/2030")).toBeTruthy();
  expect(main.getByRole("button", { name: "Change the card" })).toBeTruthy();
  expect(main.getAllByRole("listitem").map((row) => row.textContent)).toEqual([
    "2026-11-01¥1,200",
    "2026-10-01¥1,200¥200 refunded",
  ]);
  expect(main.getByRole("link", { name: "Back" }).getAttribute("href")).toBe("/settings/supporter");
});

test("a paused subscription says so in place of its next payment, and a customer without a card adds one", async () => {
  const main = await shown({
    screen: "portal",
    title: "Payment method and history",
    card: { ...card, action: "/payments/pages/payjp/portal", change: true },
    portal: { plan: { name: "Supporter", price: monthly }, paused: true, payments: [] },
  });
  expect(main.getByText(/subscription is paused/)).toBeTruthy();
  expect(main.queryByText(/Next payment/)).toBeNull();
  expect(main.getByRole("heading", { name: "Add a card" })).toBeTruthy();
  expect(main.getByRole("button", { name: "Add a card" })).toBeTruthy();
  expect(main.getByText("No payments yet.")).toBeTruthy();
});

test("the error page says what the server could not do, takes the focus to it, and leads back to the app", async () => {
  const main = await shown({ screen: "error", title: "Payment", error: "This page has expired." });
  const alert = main.getByRole("alert");
  expect(alert.textContent).toBe("This page has expired.");
  expect(alert.hasAttribute("data-focus-on-load")).toBe(true);
  expect(main.getByRole("link", { name: "Back to the app" }).getAttribute("href")).toBe("/");
});

/** Whether `first` stands before `second` in the document. */
const before = (first: Node, second: Node) =>
  (first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;

const yearly = { amount: 12000, currency: "JPY", interval: "year" } as const;

test("PAY.JP's checkout names the server, the plan and its period, and says what follows right above the button", async () => {
  const main = await shown({ screen: "checkout", server, title: "Subscribe to Supporter", price: yearly, card });
  expect(screen.getByRole("complementary", { name: "Hanami Town" })).toBeTruthy();
  expect(within(screen.getByRole("banner")).getByText("Hanami Town")).toBeTruthy();
  expect(main.getByRole("heading", { level: 1 }).textContent).toBe("Subscribe to Supporter");
  expect(main.getByText("¥12,000 a year")).toBeTruthy();
  const then = main.getByText("Then ¥12,000 a year.");
  const submit = main.getByRole("button", { name: "Pay ¥12,000 and subscribe" });
  expect(before(then, submit)).toBe(true);
  // Nothing a reader meets stands between the line and the button.
  expect(then.nextElementSibling).toBe(submit);
});

test("the fake's checkout of a tip says it is paid once, and nothing follows it", async () => {
  const main = await shown({
    screen: "checkout",
    server,
    title: "Tip Hanami Town",
    price: { amount: 500, currency: "JPY", interval: "once" },
    fake: true,
    choices: { action: "/payments/pages/fake/checkout/cs_2" },
  });
  expect(main.getByRole("heading", { level: 1 }).textContent).toBe("Tip Hanami Town");
  expect(main.getByText("¥500, once")).toBeTruthy();
  expect(main.queryAllByText(/^Then /)).toHaveLength(0);
  expect(main.queryAllByText(/subscribe/i)).toHaveLength(0);
  expect(main.getByRole("button", { name: "Pay" })).toBeTruthy();
});

test("test mode and the fake build are said once, ahead of the page's header and main; a live page says neither", async () => {
  for (const [page, says] of [
    [
      { screen: "checkout", server, title: "Subscribe", price: monthly, testMode: true, card },
      "Test mode: nothing is charged.",
    ],
    [
      { screen: "checkout", server, title: "Subscribe", price: monthly, fake: true, choices: { action: "/f" } },
      "Test build: this page stands in for a payment provider, and no money moves.",
    ],
    [
      { screen: "portal", server, title: "Payments", fake: true, portal: { payments: [] } },
      "Test build: this page stands in for a payment provider, and no money moves.",
    ],
  ] as const satisfies readonly (readonly [PayPage, string])[]) {
    await shown(page);
    const notes = screen.getAllByRole("note");
    expect([page.screen, notes.map((note) => note.textContent)]).toEqual([page.screen, [says]]);
    const [note] = notes;
    if (note === undefined) throw new Error("test: no note");
    expect([before(note, screen.getByRole("banner")), before(note, screen.getByRole("main"))]).toEqual([true, true]);
  }
  await shown({ screen: "checkout", server, title: "Subscribe", price: monthly, card });
  expect(screen.queryAllByRole("note")).toHaveLength(0);
});

test("a payment the server refused is said before the button, and takes the focus when the page loads", async () => {
  for (const [page, button] of [
    [
      { screen: "checkout", server, title: "Subscribe", price: yearly, card, error: "The card was declined." },
      "Pay ¥12,000 and subscribe",
    ],
    [
      {
        screen: "checkout",
        server,
        title: "Subscribe",
        price: monthly,
        fake: true,
        error: "The card was declined.",
        choices: { action: "/f" },
      },
      "Pay",
    ],
  ] as const satisfies readonly (readonly [PayPage, string])[]) {
    const main = await shown(page);
    const alert = main.getByRole("alert");
    expect([button, alert.textContent, alert.hasAttribute("data-focus-on-load")]).toEqual([
      button,
      "The card was declined.",
      true,
    ]);
    expect([button, before(alert, main.getByRole("button", { name: button }))]).toEqual([button, true]);
  }
});

test("only a page with a card form asks for payjp.js", async () => {
  for (const [page, asks] of [
    [{ screen: "checkout", title: "Subscribe", price: monthly, card }, 1],
    [
      {
        screen: "portal",
        title: "Payments",
        card: { ...card, action: "/payments/pages/payjp/portal", change: true },
        portal: { payments: [] },
      },
      1,
    ],
    [{ screen: "checkout", title: "Subscribe", price: monthly, fake: true, choices: { action: "/f" } }, 0],
    [{ screen: "portal", title: "Payments", fake: true, portal: { payments: [] } }, 0],
    [{ screen: "error", title: "Payment", error: "This page has expired." }, 0],
  ] as const satisfies readonly (readonly [PayPage, number])[]) {
    let asked = 0;
    const payjp = fakePayjp({ id: "tok_1" });
    await shown(page, {
      load: () => {
        asked += 1;
        return payjp.load();
      },
    });
    expect([page.screen, page.card === undefined ? "no card form" : "card form", asked]).toEqual([
      page.screen,
      page.card === undefined ? "no card form" : "card form",
      asks,
    ]);
  }
});

test("payjp.js is loaded from PAY.JP's address and no other, once for the page, and gives PAY.JP's factory", async () => {
  // The head keeps the scripts it is given instead of connecting them, so the simulation fetches nothing; the test
  // then says the script loaded, as a browser would once PAY.JP's file ran.
  const head = document.head;
  const asked: HTMLScriptElement[] = [];
  const keep = (...nodes: (Node | string)[]) => {
    for (const node of nodes) if (node instanceof HTMLScriptElement) asked.push(node);
  };
  Object.assign(head, {
    append: keep,
    appendChild: <T extends Node>(node: T) => {
      keep(node);
      return node;
    },
  });
  const factory: PayjpFactory = () => {
    throw new Error("test: not called");
  };
  onTestFinished(() => {
    Reflect.deleteProperty(head, "append");
    Reflect.deleteProperty(head, "appendChild");
    Reflect.deleteProperty(window, "Payjp");
  });
  const first = loadPayjp();
  const second = loadPayjp();
  expect(payjpScript).toBe("https://js.pay.jp/v2/pay.js");
  expect(asked.map((script) => script.src)).toEqual(["https://js.pay.jp/v2/pay.js"]);
  expect(second).toBe(first);
  Reflect.set(window, "Payjp", factory);
  for (const script of asked) script.dispatchEvent(new Event("load"));
  expect(await first).toBe(factory);
});

/** The card form of a checkout given `name`: what was asked of PAY.JP, what was posted, and the field. */
async function submitted(name: string) {
  const payjp = fakePayjp({ id: "tok_1" });
  const main = await shown({ screen: "checkout", title: "Subscribe", price: monthly, card }, payjp);
  const submit = main.getByRole<HTMLButtonElement>("button", { name: "Pay ¥1,200 and subscribe" });
  const form = submit.form;
  if (form === null) throw new Error("test: the button submits no form");
  let sent: string | undefined;
  form.submit = () => {
    const value = new FormData(form).get("name");
    sent = typeof value === "string" ? value : "a file";
  };
  const field = main.getByLabelText<HTMLInputElement>("Name on the card");
  await userEvent.type(field, name);
  await userEvent.click(submit);
  const said = (field.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .map((id) => document.getElementById(id)?.textContent ?? "");
  return { tokens: payjp.calls.tokens.length, sent, said, focused: document.activeElement === field };
}

test("a name of 2 to 45 Latin letters, digits, spaces, dots and hyphens goes to PAY.JP as typed", async () => {
  for (const name of ["AB", "ab1", "Mary-Jane O.", "A".repeat(45)]) {
    const { tokens, sent, said } = await submitted(name);
    expect([name, tokens, sent, said]).toEqual([name, 1, name, ["As printed on the card, in Latin letters."]]);
  }
});

test("any other name is said under its field, which takes the focus, and no token is asked for", async () => {
  for (const name of ["AB_C", "ÉLISE", "O'Brien", "Ann, Lee"]) {
    const result = await submitted(name);
    expect([name, result]).toEqual([
      name,
      { tokens: 0, sent: undefined, said: ["Enter the name in Latin letters, as printed on the card."], focused: true },
    ]);
  }
});

test("PAY.JP's own words for a refused card never reach the page", async () => {
  const main = await shown(
    { screen: "checkout", title: "Subscribe", price: monthly, card: { ...card, name: "ALICE EXAMPLE" } },
    fakePayjp({ error: { message: "Your card was declined by PAY.JP." } }),
  );
  await userEvent.click(main.getByRole("button", { name: "Pay ¥1,200 and subscribe" }));
  expect(main.getByRole("alert").textContent).toBe(
    "The card could not be checked, and nothing was charged. Check the email address, the name on the card, and the card details, then try again.",
  );
  expect(document.body.textContent).not.toContain("declined by PAY.JP");
});

test("the portal's card change posts the state, the token, the email and the name, and nothing else", async () => {
  const main = await shown({
    screen: "portal",
    title: "Payment method and history",
    card: { ...card, action: "/payments/pages/payjp/portal", change: true },
    portal: { card: "Visa ending in 4242, expires 12/2030", payments: [] },
  });
  const submit = main.getByRole<HTMLButtonElement>("button", { name: "Change the card" });
  const form = submit.form;
  if (form === null) throw new Error("test: the button submits no form");
  let sent: Record<string, FormDataEntryValue> | undefined;
  form.submit = () => {
    sent = Object.fromEntries(new FormData(form));
  };
  await userEvent.type(main.getByLabelText("Name on the card"), "ALICE EXAMPLE");
  await userEvent.click(submit);
  expect([form.getAttribute("action"), sent]).toEqual([
    "/payments/pages/payjp/portal",
    { s: "signed", token: "tok_1", email: "alice@example.com", name: "ALICE EXAMPLE" },
  ]);
});

test("the error page leads back to the screen the server names", async () => {
  const main = await shown({
    screen: "error",
    title: "Payment",
    error: "This page has expired.",
    back: "/settings/supporter",
  });
  expect(main.getAllByRole("link").map((link) => [link.textContent, link.getAttribute("href")])).toEqual([
    ["Back to the app", "/settings/supporter"],
  ]);
});
