// SPDX-License-Identifier: AGPL-3.0-only

// The browser and the system a signed-in browser's row names (ADR-0090 §11), read from the User-Agent it signed in
// with: the browser a reader would name, not the engines its User-Agent also lists, and nothing where the string
// names no browser or no system the app knows, for the row to show the string as it is.

import { expect, test } from "bun:test";
import { browserOf } from "./user-agent";

const webkit = "AppleWebKit/605.1.15 (KHTML, like Gecko)";
const blink = "AppleWebKit/537.36 (KHTML, like Gecko)";
const windows = "Mozilla/5.0 (Windows NT 10.0; Win64; x64)";
const mac = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)";
const iphone = "Mozilla/5.0 (iPhone; CPU iPhone OS 19_1 like Mac OS X)";

const known: readonly (readonly [string, string, string])[] = [
  ["Firefox", "Linux", "Mozilla/5.0 (X11; Linux x86_64; rv:146.0) Gecko/20100101 Firefox/146.0"],
  ["Firefox", "Android", "Mozilla/5.0 (Android 16; Mobile; rv:146.0) Gecko/146.0 Firefox/146.0"],
  ["Chrome", "Windows", `${windows} ${blink} Chrome/140.0.0.0 Safari/537.36`],
  ["Chrome", "macOS", `${mac} ${blink} Chrome/140.0.0.0 Safari/537.36`],
  ["Chrome", "ChromeOS", `Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) ${blink} Chrome/140.0.0.0 Safari/537.36`],
  // Android says Linux too, and an iPhone or an iPad "like Mac OS X".
  ["Chrome", "Android", `Mozilla/5.0 (Linux; Android 16; Pixel 10) ${blink} Chrome/140.0.0.0 Mobile Safari/537.36`],
  ["Safari", "macOS", `${mac} ${webkit} Version/19.0 Safari/605.1.15`],
  ["Safari", "iPhone", `${iphone} ${webkit} Version/19.1 Mobile/15E148 Safari/604.1`],
  ["Safari", "iPad", `Mozilla/5.0 (iPad; CPU OS 19_1 like Mac OS X) ${webkit} Version/19.1 Mobile/15E148 Safari/604.1`],
  // On an iPhone every browser is WebKit and says Safari; its own token names it.
  ["Chrome", "iPhone", `${iphone} ${webkit} CriOS/140.0.0.0 Mobile/15E148 Safari/604.1`],
  ["Firefox", "iPhone", `${iphone} ${webkit} FxiOS/146.0 Mobile/15E148 Safari/605.1.15`],
  // A browser built on Chromium says Chrome and Safari too; its own token names it.
  ["Edge", "Windows", `${windows} ${blink} Chrome/140.0.0.0 Safari/537.36 Edg/140.0.0.0`],
  ["Opera", "Windows", `${windows} ${blink} Chrome/140.0.0.0 Safari/537.36 OPR/125.0.0.0`],
  ["Vivaldi", "Linux", `Mozilla/5.0 (X11; Linux x86_64) ${blink} Chrome/140.0.0.0 Safari/537.36 Vivaldi/7.6`],
  [
    "Samsung Internet",
    "Android",
    `Mozilla/5.0 (Linux; Android 16; SAMSUNG SM-S938B) ${blink} SamsungBrowser/29.0 Chrome/136.0.0.0 Mobile Safari/537.36`,
  ],
];

for (const [browser, os, userAgent] of known)
  test(`${browser} on ${os} is read from "${userAgent}"`, () => {
    expect(browserOf(userAgent)).toEqual({ browser, os });
  });

for (const [what, userAgent] of [
  ["a tool that is no browser", "curl/8.5.0"],
  ["an app that names a system and no browser", "Tuskr/2.1 (Android 16)"],
  ["a browser that names no system", "Firefox/146.0"],
  ["a system with no browser", "Mozilla/5.0 (X11; Linux x86_64)"],
  ["an empty string", ""],
] as const)
  test(`${what} ("${userAgent}") reads as nothing, for the row to show the string`, () => {
    expect(browserOf(userAgent)).toBeUndefined();
  });
