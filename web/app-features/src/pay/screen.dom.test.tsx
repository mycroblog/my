// SPDX-License-Identifier: AGPL-3.0-only

// What each payment page shows of the data the server gave it, and what its forms post: the fake's choice of an outcome,
// and PAY.JP's card form, which posts the signed state and the token that payjp.js gave, never a field of the card.

import { expect, test } from "bun:test";
import { I18nProvider } from "@my/design-system";
import { act, cleanup, render, screen, within } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import type { PayPage } from "./data";
import type { PayjpFactory } from "./payjp";
import { PayScreen } from "./screen";

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

async function shown(page: PayPage, payjp = fakePayjp({ id: "tok_1" })) {
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

const card = {
  action: "/payments/pages/payjp/checkout",
  state: "signed",
  publicKey: "pk_test",
  email: "alice@example.com",
};

test("the fake's checkout offers to pay, to fail and to cancel, each posting its action", async () => {
  const main = await shown({
    screen: "checkout",
    title: "Subscribe to Supporter",
    price: "500 JPY a month",
    fake: true,
    back: "/settings/supporter",
    choices: { action: "/payments/pages/fake/checkout/cs_1" },
  });
  expect(main.getByRole("heading", { level: 1 }).textContent).toBe("Subscribe to Supporter");
  expect(main.getByText("500 JPY a month")).toBeTruthy();
  expect(main.getByText(/no money moves/)).toBeTruthy();
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
  expect(main.getByRole("link", { name: "Back" }).getAttribute("href")).toBe("/settings/supporter");
});

test("PAY.JP's checkout posts the state and the token of the card, and no field of the card itself", async () => {
  const payjp = fakePayjp({ id: "tok_1" });
  const main = await shown(
    { screen: "checkout", title: "Subscribe", price: "1,200 yen a month", testMode: true, card },
    payjp,
  );
  expect(main.getByText(/Test mode/)).toBeTruthy();
  expect(payjp.calls.publicKey).toBe("pk_test");
  expect(payjp.calls.mounted).toEqual([".pay-card-element"]);
  const submit = main.getByRole<HTMLButtonElement>("button", { name: "Subscribe" });
  const form = submit.form;
  if (form === null) throw new Error("test: Subscribe submits no form");
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

test("a card that PAY.JP refuses is said before the button, and nothing is posted", async () => {
  const main = await shown(
    { screen: "checkout", title: "Subscribe", card: { ...card, name: "ALICE EXAMPLE" } },
    fakePayjp({ error: { message: "Your card was declined." } }),
  );
  const submit = main.getByRole<HTMLButtonElement>("button", { name: "Subscribe" });
  let posted = false;
  if (submit.form) submit.form.submit = () => (posted = true);
  await userEvent.click(submit);
  expect(main.getByRole("alert").textContent).toBe("Your card was declined.");
  expect(posted).toBe(false);
});

test("without payjp.js, the card form says so and cannot be sent", async () => {
  cleanup();
  await act(async () => {
    render(
      <I18nProvider locale="en">
        <PayScreen
          page={{ screen: "checkout", title: "Subscribe", card }}
          wordmark={{ light: "", dark: "" }}
          payjp={() => Promise.reject(new Error("blocked"))}
        />
      </I18nProvider>,
    );
    await Promise.resolve();
  });
  expect(screen.getByRole("alert").textContent).toBe(
    "The card form could not load. Reload the page, or try again in a while.",
  );
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Subscribe" }).disabled).toBe(true);
});

test("the portal shows the card, a paused subscription, a card change and the recent payments", async () => {
  const main = await shown({
    screen: "portal",
    title: "Payment method and history",
    notice: "The card was changed.",
    card: { ...card, action: "/payments/pages/payjp/portal", change: true },
    portal: {
      card: "Visa ending in 4242, expires 12/2030",
      paused: true,
      payments: [
        { date: "2026-11-01", amount: "1,200 yen" },
        { date: "2026-10-01", amount: "1,200 yen", refunded: "200 yen" },
      ],
    },
  });
  expect(main.getByRole("status").textContent).toBe("The card was changed.");
  expect(main.getByText("Visa ending in 4242, expires 12/2030")).toBeTruthy();
  expect(main.getByText(/subscription is paused/)).toBeTruthy();
  expect(main.getByRole("button", { name: "Change the card" })).toBeTruthy();
  expect(main.getAllByRole("listitem").map((item) => item.textContent)).toEqual([
    "2026-11-01: 1,200 yen",
    "2026-10-01: 1,200 yen, of which 200 yen refunded",
  ]);
});

test("the error page says what the server could not do, and takes the focus to it", async () => {
  const main = await shown({ screen: "error", title: "Payment", error: "This page has expired.", back: "/settings" });
  const alert = main.getByRole("alert");
  expect(alert.textContent).toBe("This page has expired.");
  expect(alert.hasAttribute("data-focus-on-load")).toBe(true);
  expect(main.getByRole("link", { name: "Back" }).getAttribute("href")).toBe("/settings");
});
