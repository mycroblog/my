// SPDX-License-Identifier: AGPL-3.0-only

// The browser and the operating system a `User-Agent` names, for the list of signed-in browsers (ADR-0090 §11): the
// few that say who they are in a way that has not changed for years, by their product names, which no language
// translates. A string it cannot read gives nothing, and the list shows the string itself. No UI here.

/** The browsers, the first whose token the string holds: each also carries the tokens of those after it. */
const browsers: readonly (readonly [RegExp, string])[] = [
  [/\bEdg(?:e|A|iOS)?\//, "Edge"],
  [/\bOPR\//, "Opera"],
  [/\bSamsungBrowser\//, "Samsung Internet"],
  [/\bVivaldi\//, "Vivaldi"],
  [/\b(?:Firefox|FxiOS)\//, "Firefox"],
  [/\b(?:Chrome|CriOS|Chromium)\//, "Chrome"],
  [/\bVersion\/[\d.]+.*\bSafari\//, "Safari"],
];

/** The operating systems, the first whose token the string holds: Android and ChromeOS also say Linux, an iPad Mac. */
const systems: readonly (readonly [RegExp, string])[] = [
  [/\bWindows\b/, "Windows"],
  [/\biPhone\b/, "iPhone"],
  [/\biPad\b/, "iPad"],
  [/\bAndroid\b/, "Android"],
  [/\bCrOS\b/, "ChromeOS"],
  [/\bMac OS X\b|\bMacintosh\b/, "macOS"],
  [/\bLinux\b/, "Linux"],
];

/** The browser and the operating system `userAgent` names; undefined unless it names both. */
export function browserOf(userAgent: string): { browser: string; os: string } | undefined {
  const browser = browsers.find(([token]) => token.test(userAgent))?.[1];
  const os = systems.find(([token]) => token.test(userAgent))?.[1];
  return browser === undefined || os === undefined ? undefined : { browser, os };
}
