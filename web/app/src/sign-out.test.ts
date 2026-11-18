// SPDX-License-Identifier: AGPL-3.0-only

// What a tab of the app does once the reader signed out in another tab of the browser (#3198): it empties what it
// read of the account and leaves for the start by a load of the page, where the app opens without a session. That a
// tab hears of it, and of nothing less, is the data's (app-data, session.test.ts); that the app's entry listens, a test
// in a browser with two tabs (web/e2e/specs/settings.spec.ts); what the tab that signs out does, settings.dom.test.tsx.

import { expect, test } from "bun:test";
import { preferencesQuery } from "@my/app-data";
import { followSignOut } from "./sign-out";
import { fakeApp } from "./test/server";

test("following a sign-out made in another tab empties what this tab read of the account and leaves for the start", async () => {
  const { data } = fakeApp();
  await data.queryClient.query(preferencesQuery(data));
  expect(data.queryClient.getQueryCache().getAll().length).toBeGreaterThan(0);
  const left: string[] = [];

  followSignOut({
    data,
    leave: (href) => {
      left.push(href);
    },
  });

  expect(left).toEqual(["/"]);
  expect(data.queryClient.getQueryCache().getAll()).toHaveLength(0);
});
