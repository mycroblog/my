// SPDX-License-Identifier: AGPL-3.0-only
// The browser and the operating system in a User-Agent, for the list of signed-in browsers (ADR-0136 §2). It tells the
// common browsers apart and nothing more: what it does not know, the caller shows as the original string.

export type DescribedUserAgent = { browser: string | undefined; os: string | undefined };

/** The first match wins, so a browser comes before the engines its User-Agent also names. */
const browsers: readonly (readonly [RegExp, string])[] = [
  [/\bEdg(?:e|A|iOS)?\//, "Edge"],
  [/\b(?:OPR|Opera)\//, "Opera"],
  [/\bVivaldi\//, "Vivaldi"],
  [/\bSamsungBrowser\//, "Samsung Internet"],
  [/\b(?:Firefox|FxiOS)\//, "Firefox"],
  [/\b(?:Chrome|Chromium|CriOS)\//, "Chrome"],
  [/\bSafari\//, "Safari"],
];

/** The first match wins, so iOS and Android come before the systems their User-Agents also name. */
const systems: readonly (readonly [RegExp, string])[] = [
  [/\b(?:iPhone|iPad|iPod)\b/, "iOS"],
  [/\bAndroid\b/, "Android"],
  [/\bWindows\b/, "Windows"],
  [/\bCrOS\b/, "ChromeOS"],
  [/\bMac OS X\b|\bMacintosh\b/, "macOS"],
  [/\bLinux\b/, "Linux"],
];

function firstMatch(table: readonly (readonly [RegExp, string])[], userAgent: string): string | undefined {
  return table.find(([pattern]) => pattern.test(userAgent))?.[1];
}

/** The browser and the operating system of a User-Agent, each undefined when it is not one this knows. */
export function describeUserAgent(userAgent: string): DescribedUserAgent {
  return { browser: firstMatch(browsers, userAgent), os: firstMatch(systems, userAgent) };
}
