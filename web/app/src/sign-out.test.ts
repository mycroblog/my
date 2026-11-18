// SPDX-License-Identifier: AGPL-3.0-only

// What a tab of the app does once the reader signed out in another tab of the browser (#3198): it empties what it
// read of the account and leaves for the start by a load of the page, where the app opens without a session. That a
// tab hears of it, and of nothing less, is the data's (app-data, session.test.ts); that the app's entry listens, a test
// in a browser with two tabs (web/e2e/specs/settings.spec.ts); what the tab that signs out does, settings.dom.test.tsx,
// which also holds that each way a session ends leaves nothing a confirmation kept in the tab's storage, which this
// file's runtime has not.

import { expect, test } from "bun:test";
import { preferencesQuery } from "@my/app-data";
import { followSignOut, leaveEndedSession, sessionEnding } from "./sign-out";
import { fakeApp, withoutToken } from "./test/server";

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

// A form with unsaved changes holds a navigation and the page's unload until the session is known to end: the app
// leaves an ended session without asking (web/app/src/settings-profile.dom.test.tsx holds the forms to it).
test("the session is marked as ending before the app leaves, when it follows a sign-out made in another tab", () => {
  const { data } = fakeApp();
  expect(sessionEnding(data)).toBe(false);
  const marked: boolean[] = [];

  followSignOut({
    data,
    leave: () => {
      marked.push(sessionEnding(data));
    },
  });

  expect(marked).toEqual([true]);
});

test("the session is marked as ending before the app leaves a session the server ended", async () => {
  const { data } = withoutToken(fakeApp());
  expect(sessionEnding(data)).toBe(false);
  const marked: boolean[] = [];

  await leaveEndedSession({
    data,
    leave: () => {
      marked.push(sessionEnding(data));
    },
  });

  expect(marked).toEqual([true]);
});

test("no session is ending where there is none, nor in another tab's data", () => {
  const { data } = fakeApp();
  const other = fakeApp().data;
  followSignOut({ data, leave: () => undefined });

  expect(sessionEnding(null)).toBe(false);
  expect(sessionEnding(other)).toBe(false);
});
