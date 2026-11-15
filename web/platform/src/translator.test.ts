// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, expect, test } from "bun:test";
import { installTranslator } from "./test/translator";
import {
  browserName,
  browserTranslatorAvailability,
  hasBrowserTranslator,
  needsActivation,
  openBrowserTranslator,
} from "./translator";

afterEach(() => {
  Reflect.deleteProperty(globalThis, "Translator");
  Reflect.deleteProperty(navigator, "userAgentData");
});

/** A browser that states `brands`, as Chromium's navigator.userAgentData does. */
const stating = (brands: string[]) => {
  Object.defineProperty(navigator, "userAgentData", {
    configurable: true,
    value: { brands: brands.map((brand) => ({ brand, version: "142" })) },
  });
};

test("a browser without the Translator API has no translator, and translates no pair", async () => {
  expect(hasBrowserTranslator()).toBe(false);
  expect(await browserTranslatorAvailability("ja", "en")).toBe("unavailable");
  const opened = await openBrowserTranslator("ja", "en").then(
    () => "opened",
    () => "rejected",
  );
  expect(opened).toBe("rejected");
});

test("a browser with it says, pair by pair, whether it translates now, after a download, or not at all", async () => {
  installTranslator({ "ja>en": "available", "de>en": "downloadable", "fr>en": "downloading" });
  expect(hasBrowserTranslator()).toBe(true);
  expect(await browserTranslatorAvailability("ja", "en")).toBe("available");
  expect(await browserTranslatorAvailability("de", "en")).toBe("downloadable");
  expect(await browserTranslatorAvailability("fr", "en")).toBe("downloading");
  expect(await browserTranslatorAvailability("ko", "en")).toBe("unavailable");
});

test("a browser that fails to say whether it translates a pair does not", async () => {
  Object.defineProperty(globalThis, "Translator", {
    configurable: true,
    value: { availability: () => Promise.reject(new DOMException("bad tag", "NotSupportedError")) },
  });
  expect(await browserTranslatorAvailability("ja", "en")).toBe("unavailable");
});

test("a translator between the two languages translates text, and its output stays text", async () => {
  const browser = installTranslator({ "ja>en": "available" });
  const translator = await openBrowserTranslator("ja", "en");
  expect(await translator.translate("<b>小さなサーバー</b>")).toBe("[en] <b>小さなサーバー</b>");
  expect(browser.created).toEqual(["ja>en"]);
  expect(browser.translated).toEqual(["<b>小さなサーバー</b>"]);
});

test("while the browser downloads its model, the progress hears each share that came", async () => {
  const browser = installTranslator({ "ja>en": "downloadable" });
  browser.download = [0, 0.25, 0.5, 1];
  const heard: number[] = [];
  await openBrowserTranslator("ja", "en", (share) => heard.push(share));
  expect(heard).toEqual([0, 0.25, 0.5, 1]);
});

test("a download refused without the reader's press is told apart from other failures", async () => {
  const browser = installTranslator({ "ja>en": "downloadable" });
  browser.refuse = new DOMException("Requires a user gesture", "NotAllowedError");
  const refused: unknown = await openBrowserTranslator("ja", "en").catch((e: unknown) => e);
  expect(needsActivation(refused)).toBe(true);
  expect(needsActivation(new DOMException("No model", "NotSupportedError"))).toBe(false);
  expect(needsActivation(new Error("NotAllowedError"))).toBe(false);
  expect(needsActivation(undefined)).toBe(false);
});

test("the browser's name comes from the brand it states, never from the engine's or a made-up one", () => {
  stating(["Not)A;Brand", "Chromium", "Google Chrome"]);
  expect(browserName()).toBe("Google Chrome");
  stating(["Microsoft Edge", "Chromium", "Not_A Brand"]);
  expect(browserName()).toBe("Microsoft Edge");
  Reflect.deleteProperty(navigator, "userAgentData");
  expect(browserName()).toBeUndefined();
});

test("a browser that states no brand but the engine's has no name", () => {
  stating(["Chromium", "Not/A)Brand"]);
  expect(browserName()).toBeUndefined();
});
