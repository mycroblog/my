// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  browserSessionsQuery,
  revokeBrowserSession,
  revokeOtherBrowserSessions,
  useBrowserSessions,
  useBrowserSessionWrites,
} from "./browser-sessions";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { kindOf, refusal } from "./test/settings";

/** The data after the list of browsers was read; the browser that asks, and the others. */
async function read(options: Parameters<typeof fakeData>[0] = {}) {
  const given = fakeData(options);
  const { items: sessions } = await given.data.queryClient.query(browserSessionsQuery(given.data));
  const current = sessions.find((s) => s.current);
  if (current === undefined) throw new Error("the fake lists no browser that asks");
  return { ...given, current, others: sessions.filter((s) => !s.current) };
}

describe("useBrowserSessions", () => {
  test("is undefined until the list is read, then the browsers, the one that asks marked", async () => {
    const { data } = fakeData();
    expect(probe(data, useBrowserSessions)).toBeUndefined();
    await data.queryClient.query(browserSessionsQuery(data));
    const listed = probe(data, useBrowserSessions);
    expect([listed?.length, listed?.filter((s) => s.current).length]).toEqual([4, 1]);
  });
});

describe("revokeBrowserSession", () => {
  test("signs it out, and the list holds the others", async () => {
    const { data, current, others } = await read();
    const [gone, ...kept] = others;
    await revokeBrowserSession(data, gone?.id ?? "");
    expect(probe(data, useBrowserSessions)?.map((s) => s.id)).toEqual([current.id, ...kept.map((s) => s.id)]);
  });

  test("rejects with the current-session problem for the browser that asks, and the list keeps it", async () => {
    const { data, current } = await read();
    const refused = await refusal(revokeBrowserSession(data, current.id));
    expect(kindOf(refused)).toBe("current-session");
    expect(probe(data, useBrowserSessions)?.length).toBe(4);
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data, others } = await read();
    await probe(data, useBrowserSessionWrites).revoke(others[0]?.id ?? "");
    expect(probe(data, useBrowserSessions)?.length).toBe(3);
  });
});

describe("revokeOtherBrowserSessions", () => {
  test("signs out all the others, and the list holds the browser that asks", async () => {
    const { data, current } = await read();
    await revokeOtherBrowserSessions(data);
    expect(probe(data, useBrowserSessions)?.map((s) => s.id)).toEqual([current.id]);
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data } = await read();
    await probe(data, useBrowserSessionWrites).revokeOthers();
    expect(probe(data, useBrowserSessions)?.length).toBe(1);
  });
});
