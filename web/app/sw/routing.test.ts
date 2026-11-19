// SPDX-License-Identifier: AGPL-3.0-only
/// <reference types="bun" />

// The Service Worker never serves a payment page: each is a document of its own with its own CSP, which the server
// gives (ADR-0074 §11), so a navigation under /payments goes to the network as it is, and the app's shell is never
// put in its place.

import { expect, test } from "bun:test";
import { classify, type RequestFacts } from "./routing";

const origin = "https://my.example";

/** The facts of a navigation of the page to `path` of this origin. */
const navigation = (path: string, method = "GET"): RequestFacts => ({
  url: `${origin}${path}`,
  method,
  mode: "navigate",
  destination: "document",
  authorized: false,
});

test("a navigation to a payment page passes through, whatever the provider and the method", () => {
  for (const [path, method] of [
    ["/payments", "GET"],
    ["/payments/pages/payjp/checkout?s=signed", "GET"],
    ["/payments/pages/payjp/portal?s=signed", "GET"],
    ["/payments/pages/fake/checkout/cs_1", "GET"],
    ["/payments/pages/payjp/checkout", "POST"],
    ["/payments/pages/fake/checkout/cs_1", "POST"],
  ])
    expect([path, method, classify(navigation(path, method), origin)]).toEqual([path, method, "network"]);
});

test("a navigation of the app is the app's, beside a path that only begins with the same letters", () => {
  // The same facts as above lead elsewhere: the pass-through is the path's, not the facts' shape.
  expect(classify(navigation("/home"), origin)).toBe("navigation");
  expect(classify(navigation("/paymentsx"), origin)).toBe("navigation");
});
