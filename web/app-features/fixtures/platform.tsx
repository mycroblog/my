// SPDX-License-Identifier: AGPL-3.0-only
// A feature asks @my/platform what the browser can do, and checks nothing itself (ADR-0028 §17): the platform's
// globals, a feature test on a global object and a media query of the platform are refused; the viewport, the
// reader's preferences, a binding of the file and a type stay open.

import { getPasskey, supportsPasskeys } from "@my/platform";

export const asked = [getPasskey, supportsPasskeys];

export const direct = [
  navigator.credentials, // error: platform navigator
  navigator.userAgent, // error: platform navigator
  typeof PublicKeyCredential, // error: platform PublicKeyCredential
  window.navigator.maxTouchPoints, // error: platform .navigator
  globalThis["navigator"], // error: platform .navigator
  "ontouchstart" in window, // error: platform "ontouchstart" in
  "PublicKeyCredential" in globalThis, // error: platform "PublicKeyCredential" in
  "ontouchstart" in document.documentElement, // error: platform "ontouchstart" in
  window.matchMedia("(display-mode: standalone)"), // error: platform matchMedia(display-mode)
  matchMedia(`(pointer: coarse)`), // error: platform matchMedia(pointer)
  document.defaultView?.matchMedia("(any-hover: hover)"), // error: platform matchMedia(any-hover)
  new Notification("hi"), // error: platform Notification
];

export function open(navigator: { credentials: string }, query: string) {
  const options = { navigator: 1, PublicKeyCredential: 2 };
  return [
    navigator.credentials,
    options,
    "key" in options,
    window.matchMedia("(min-width: 40rem)"),
    window.matchMedia("(prefers-reduced-motion: reduce)"),
    window.matchMedia(query),
  ];
}

export type Prompt = typeof PublicKeyCredential | PublicKeyCredential | Notification;
