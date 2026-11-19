// SPDX-License-Identifier: AGPL-3.0-only

// payjp.js, PAY.JP's script (ADR-0074 §11): the one script of another site that a payment page loads, and only on a page
// with a card form. It draws the card element in PAY.JP's own iframe and turns the card into a token there, after the
// card's 3-D Secure in an iframe, so that the card never passes through this page or the server. The page's
// Content-Security-Policy allows its origin (internal/interface/payments), and the build's inventory of the payment
// entry's scripts lists it (web/app/scripts/payment-scripts.ts).

/** Where payjp.js loads from: its v2, which PAY.JP keeps up to date at this one address. */
export const payjpScript = "https://js.pay.jp/v2/pay.js";

/** The card element of payjp.js. */
export type PayjpCardElement = { mount(target: string | HTMLElement): void; unmount?: () => void };

/** What payjp.js answers to a card: its token, or why it took none. */
type PayjpTokenAnswer = { id: string; error?: undefined } | { id?: undefined; error: { message?: string } };

/** The part of a payjp.js instance that the card form uses. */
export type Payjp = {
  elements(): { create(type: "card"): PayjpCardElement };
  createToken(
    element: PayjpCardElement,
    data: { three_d_secure: boolean; card: { name: string; email: string } },
  ): Promise<PayjpTokenAnswer>;
};

/** The global function that payjp.js defines, which starts an instance on a public key. */
export type PayjpFactory = (publicKey: string, options: { threeDSecureWorkflow: "iframe" }) => Payjp;

const factoryOf = () => (window as { Payjp?: PayjpFactory }).Payjp;

let loading: Promise<PayjpFactory> | undefined;

/** Loads payjp.js into the page once, and gives its factory; it fails when the script does not load. */
export function loadPayjp(): Promise<PayjpFactory> {
  const loaded = factoryOf();
  if (loaded !== undefined) return Promise.resolve(loaded);
  loading ??= new Promise<PayjpFactory>((resolve, reject) => {
    const script = document.createElement("script");
    script.src = payjpScript;
    script.async = true;
    script.addEventListener("load", () => {
      const factory = factoryOf();
      if (factory === undefined) reject(new Error("payjp.js defines no Payjp"));
      else resolve(factory);
    });
    script.addEventListener("error", () => {
      loading = undefined;
      script.remove();
      reject(new Error("payjp.js did not load"));
    });
    document.head.append(script);
  });
  return loading;
}
