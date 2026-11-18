// SPDX-License-Identifier: AGPL-3.0-only

// How the two lists of the settings' Apps entry stand together (#3206): the apps the account authorized and the apps
// it registered are one screen, which waits for both. That a read failed, and that `retry` reads it again, is pinned
// on a client's render (app-features, settings/apps/apps.dom.test.tsx): to a hook rendered once on the server, as
// the probe renders it, a failed read is one that has not answered, and its `retry` asks for nothing.

import { describe, expect, test } from "bun:test";
import { useAppsRead } from "./apps";
import { authorizedAppsQuery } from "./authorized-apps";
import { oAuthAppsQuery } from "./oauth-apps";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { settled } from "./test/settings";

const lists = [
  ["the authorized apps", "GET /authorized_apps"],
  ["the reader's own apps", "GET /oauth_apps"],
] as const;

/** Reads both lists as the screen does, each failing by itself. */
async function readBoth({ data }: ReturnType<typeof fakeData>) {
  await Promise.allSettled([
    data.queryClient.query(authorizedAppsQuery(data)),
    data.queryClient.query(oAuthAppsQuery(data)),
  ]);
}

describe("useAppsRead", () => {
  test("is loading before anything is read, and read once both lists are", async () => {
    const given = fakeData();
    expect(probe(given.data, useAppsRead).status).toBe("loading");

    await readBoth(given);

    expect(probe(given.data, useAppsRead).status).toBe("read");
  });

  for (const [what, route] of lists)
    test(`is still loading while ${what} have no answer, the other list read`, async () => {
      const given = fakeData();
      const held = given.fake.script(route, { hold: "request" });
      const both = readBoth(given);
      await held.arrived();
      // The other list answered: only the held one is on its way.
      await settled(given.data, 1);

      expect(probe(given.data, useAppsRead).status).toBe("loading");

      held.release();
      await both;
      expect(probe(given.data, useAppsRead).status).toBe("read");
    });

  for (const [what, route] of lists)
    test(`is not read when ${what} could not be read, the other list read`, async () => {
      const given = fakeData();
      given.fake.script(route, { fail: 500, times: 1 });

      await readBoth(given);

      expect(probe(given.data, useAppsRead).status).not.toBe("read");
    });
});
