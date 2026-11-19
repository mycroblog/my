// SPDX-License-Identifier: AGPL-3.0-only

// The data of a payment page (ADR-0074 §11): the JSON data block <script type="application/json" id="pay"> that the
// server's shell of the payment entry carries (internal/interface/payments, pages.go; its type is supporters.PageData).
// The server decides which screen shows and words what it decides (the title, the price, a form's outcome, what it read
// of the provider); the screens word the rest.

/** The screens of the payment entry, by the server's name for each. */
export type PayScreenName = "checkout" | "portal" | "error";

/** The card form of payjp.js: it posts the token of the card that PAY.JP's iframe took, never the card. */
export type PayCardForm = {
  /** The path the form posts to, with the signed state that it posts back as `s`. */
  action: string;
  state: string;
  /** PAY.JP's public key, which payjp.js takes. */
  publicKey: string;
  email?: string;
  name?: string;
  /** A card change (the portal's) rather than a subscription (the checkout's). */
  change?: boolean;
};

/** A payment of the portal, worded by the server: its date (YYYY-MM-DD), its amount and what of it was refunded. */
type PayPayment = { date: string; amount: string; refunded?: string };

export type PayPage = {
  screen: PayScreenName;
  title: string;
  /** PAY.JP's test keys are in use: no card is charged. */
  testMode?: boolean;
  /** The fake provider of a test build (ADR-0015 §6). */
  fake?: boolean;
  price?: string;
  /** What went wrong, which takes the focus when the page opens; what went right. */
  error?: string;
  notice?: string;
  /** Where the link back goes; none without it. */
  back?: string;
  card?: PayCardForm;
  /** The fake checkout's form, which posts its action (pay, fail, cancel). */
  choices?: { action: string };
  /** What the portal shows of the customer; none on a portal with nothing to show (the fake's). */
  portal?: { card?: string; paused?: boolean; payments: PayPayment[] };
};

const screens = new Set<string>(["checkout", "portal", "error"]);

/**
 * The page of the data block's text. It throws when the text is no data of a page; the server writes it, so a mismatch
 * is a server and an entry of different versions.
 */
export function parsePayPage(block: string | null): PayPage {
  if (block === null) throw new Error("pay: the page has no data block");
  const value: unknown = JSON.parse(block);
  if (typeof value !== "object" || value === null) throw new Error("pay: the data block is not an object");
  const { screen, title } = value as { screen?: unknown; title?: unknown };
  if (typeof screen !== "string" || !screens.has(screen)) throw new Error(`pay: no screen ${String(screen)}`);
  if (typeof title !== "string") throw new Error("pay: the data block has no title");
  return value as PayPage;
}
