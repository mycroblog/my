// SPDX-License-Identifier: AGPL-3.0-only

// The payment pages as the payment entry shows them (ADR-0074 §11), for their stories: a page of @my/app-features in one
// of its states, rendered from page data as the server's shell carries it (internal/interface/payments), a full page.
// A story per state, a file per page (pay-*.stories.tsx); the language is the toolbar's locale (ja, else en), in which
// the samples word what the server words (the title, the price, an outcome, the card and the payments). PAY.JP's script
// is a stand-in that draws nothing into the card element's place and takes no card, so that no story reaches PAY.JP.

import { PayScreen, type PayjpFactory, type PayPage } from "@my/app-features/pay";
import { useLocale } from "@my/design-system";
import { fakeHost, scenarioInstance } from "@my/e2e/fake";
import type { StoryObj } from "@storybook/react-vite";
import wordmarkDark from "../../brand/Wordmark-DarkBg.svg";
import wordmarkLight from "../../brand/Wordmark-Primary-Color.svg";

type Lang = "en" | "ja";

/** payjp.js as a story has it: a card element that draws nothing, and no token for any card. */
const payjp: () => Promise<PayjpFactory> = () =>
  Promise.resolve(() => ({
    elements: () => ({ create: () => ({ mount: () => undefined }) }),
    createToken: () => Promise.resolve({ error: {} }),
  }));

const checkoutAction = "/payments/pages/payjp/checkout";
const portalAction = "/payments/pages/payjp/portal";
const back = "/settings/supporter";

/** The server that is paid: the scenario's, as @my/e2e's fake server has it. */
const server = {
  name: scenarioInstance["name"] ?? fakeHost,
  description: scenarioInstance["short_description"] ?? "",
};

const monthly = { amount: 1200, currency: "JPY", interval: "month" } as const;

const words = {
  checkoutTitle: { en: "Subscribe to the supporter plan “Supporter”", ja: "サポーターのプラン「Supporter」に加入する" },
  tipTitle: { en: "Tip for my.example", ja: "my.example へのチップ" },
  threeDSecure: {
    en: "The card could not be verified (3-D Secure), and nothing was charged. Try again or use another card.",
    ja: "カードの本人認証 (3D セキュア) ができませんでした。料金は請求されていません。もう一度試すか、別のカードを使ってください。",
  },
  fakeFailed: {
    en: "The payment failed, as asked. Pay or cancel.",
    ja: "指示どおり支払いを失敗させました。支払うかキャンセルしてください。",
  },
  portalTitle: { en: "Payment method and history", ja: "支払い方法と履歴" },
  card: { en: "Visa ending in 4242, expires 12/2030", ja: "Visa (末尾 4242、有効期限 12/2030)" },
  changed: { en: "The card was changed.", ja: "カードを変更しました。" },
  resumeFailed: {
    en: "The card was changed, but the payment that resumes the subscription failed, so it is still paused. Try another card.",
    ja: "カードを変更しましたが、加入を再開する支払いに失敗したため、加入は一時停止されたままです。別のカードを試してください。",
  },
  errorTitle: { en: "Payment", ja: "支払い" },
  expired: {
    en: "This page has expired. Start again from the screen you came from.",
    ja: "このページの有効期限が切れています。元の画面からやり直してください。",
  },
  unavailable: {
    en: "PAY.JP is not answering. Try again in a while. If anything was charged, the subscription shows on the screen you came from.",
    ja: "PAY.JP が応答しません。しばらくしてからもう一度試してください。料金が請求されていれば、加入は元の画面に表示されます。",
  },
} satisfies Record<string, Record<Lang, string>>;

/** The states of each payment page, by the page's id and the state's, in `lang`. */
function payPages(lang: Lang): Record<string, Record<string, PayPage>> {
  const w = (key: keyof typeof words) => words[key][lang];
  const card = {
    action: checkoutAction,
    state: "signed-state",
    publicKey: "pk_test_sample",
    email: "yuki@example.com",
  };
  const portalCard = { ...card, action: portalAction, change: true };
  const plan = { name: "Supporter", price: monthly, next: "2026-12-01" };
  const payments = [
    { date: "2026-11-01", amount: 1200, currency: "JPY" },
    { date: "2026-10-01", amount: 1200, refunded: 200, currency: "JPY" },
  ];
  const fakePrice = { amount: 500, currency: "JPY", interval: "month" } as const;
  return {
    checkout: {
      payjp: { screen: "checkout", server, title: w("checkoutTitle"), price: monthly, back, card },
      "test-mode": {
        screen: "checkout",
        server,
        title: w("checkoutTitle"),
        price: monthly,
        testMode: true,
        back,
        card,
      },
      refused: {
        screen: "checkout",
        server,
        title: w("checkoutTitle"),
        price: monthly,
        back,
        error: w("threeDSecure"),
        card: { ...card, name: "YUKI SATO" },
      },
      fake: {
        screen: "checkout",
        server,
        title: w("checkoutTitle"),
        price: fakePrice,
        fake: true,
        back,
        choices: { action: "/payments/pages/fake/checkout/cs_sample" },
      },
      "fake-failed": {
        screen: "checkout",
        server,
        title: w("checkoutTitle"),
        price: fakePrice,
        fake: true,
        back,
        error: w("fakeFailed"),
        choices: { action: "/payments/pages/fake/checkout/cs_sample" },
      },
      tip: {
        screen: "checkout",
        server,
        title: w("tipTitle"),
        price: { amount: 500, currency: "JPY", interval: "once" },
        fake: true,
        back,
        choices: { action: "/payments/pages/fake/tip/cs_sample" },
      },
    },
    portal: {
      payjp: {
        screen: "portal",
        server,
        title: w("portalTitle"),
        back,
        card: portalCard,
        portal: { plan, card: w("card"), payments },
      },
      changed: {
        screen: "portal",
        server,
        title: w("portalTitle"),
        back,
        notice: w("changed"),
        card: portalCard,
        portal: { plan, card: w("card"), payments },
      },
      paused: {
        screen: "portal",
        server,
        title: w("portalTitle"),
        back,
        error: w("resumeFailed"),
        card: portalCard,
        portal: { plan: { name: plan.name, price: monthly }, card: w("card"), paused: true, payments },
      },
      empty: {
        screen: "portal",
        server,
        title: w("portalTitle"),
        back,
        card: portalCard,
        portal: { plan, payments: [] },
      },
      fake: { screen: "portal", server, title: w("portalTitle"), fake: true, back },
    },
    error: {
      expired: { screen: "error", server, title: w("errorTitle"), error: w("expired") },
      unavailable: { screen: "error", server, title: w("errorTitle"), error: w("unavailable"), back },
    },
  };
}

/** One state of one payment page, by their ids in payPages. */
function PayState({ page, state }: { page: string; state: string }) {
  const { locale } = useLocale();
  const data = payPages(locale.startsWith("ja") ? "ja" : "en")[page]?.[state];
  if (data === undefined) throw new Error(`no state ${state} of the payment page ${page}`);
  return <PayScreen page={data} wordmark={{ light: wordmarkLight, dark: wordmarkDark }} preview payjp={payjp} />;
}

/** What every payment page's story file shares: a full page. Tagged `themes`, the story tests run it in dark too. */
export const payMeta = { tags: ["themes"], parameters: { screen: true, layout: "fullscreen" } };

/** The story of a state of a payment page. */
export const payStory = (page: string, state: string): StoryObj => ({
  render: () => <PayState page={page} state={state} />,
});
