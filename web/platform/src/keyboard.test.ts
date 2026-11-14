// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { namesCommandKey } from "./keyboard";

test("Apple's platforms name the modifier ⌘, the others Ctrl", () => {
  const platform = Object.getOwnPropertyDescriptor(navigator, "platform");
  try {
    for (const [name, command] of [
      ["MacIntel", true],
      ["iPhone", true],
      ["Win32", false],
      ["Linux x86_64", false],
    ] as const) {
      Object.defineProperty(navigator, "platform", { configurable: true, value: name });
      expect(namesCommandKey()).toBe(command);
    }
  } finally {
    if (platform === undefined) Reflect.deleteProperty(navigator, "platform");
    else Object.defineProperty(navigator, "platform", platform);
  }
});
