// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { settingsEntryAt, signOutHref } from "./settings";

test("an address under /settings names an entry, whether the settings have it or not; the menu and the other screens name none", () => {
  expect(settingsEntryAt("/settings/display")).toBe("display");
  expect(settingsEntryAt("/settings/nothing")).toBe("nothing");
  for (const at of ["/settings", "/", "/lists", "/@yuki", "/settingsx/display"])
    expect([at, settingsEntryAt(at)]).toEqual([at, undefined]);
});

test("the confirmation of signing out opens over the screen the reader is at: its address with sign_out=true", () => {
  expect(signOutHref("/settings")).toBe("/settings?sign_out=true");
  expect(signOutHref("/settings/display")).toBe("/settings/display?sign_out=true");
});
