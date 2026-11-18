// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { changeRelationship } from "./accounts";
import type { AppData } from "./app-data";
import { blocksQuery, mutesQuery } from "./relations";
import { fakeData } from "./test/data";

/** A server where the reader blocks marco and mutes hana, and nobody else. */
function server() {
  return fakeData({
    seed: {
      people: scenario.people,
      restrictions: [
        { kind: "block", by: "yuki", target: "marco" },
        { kind: "mute", by: "yuki", target: "hana" },
      ],
    },
  });
}

/** What a screen of the lists shows again: the cache while it holds the answer and it is not marked, else a read. */
async function shown(data: AppData) {
  const blocks = await data.queryClient.infiniteQuery({ ...blocksQuery(data), staleTime: Infinity });
  const mutes = await data.queryClient.infiniteQuery({ ...mutesQuery(data), staleTime: Infinity });
  return {
    blocked: blocks.pages.flatMap((p) => p.items.map((a) => a.id)),
    muted: mutes.pages.flatMap((p) => p.items.map((m) => m.account.id)),
  };
}

describe("the lists of blocked and muted accounts", () => {
  test("hold the accounts the reader restricted, each in its own list", async () => {
    const { fake, data } = server();
    expect(await shown(data)).toEqual({ blocked: [fake.accountId("marco")], muted: [fake.accountId("hana")] });
  });

  test("a block from a profile appears in the blocked list, and leaves the muted one alone", async () => {
    const { fake, data } = server();
    await shown(data);
    await changeRelationship(data, fake.accountId("sam"), "block");
    const { blocked, muted } = await shown(data);
    expect([...blocked].sort()).toEqual([fake.accountId("marco"), fake.accountId("sam")].sort());
    expect(muted).toEqual([fake.accountId("hana")]);
  });

  test("an unblock from a profile leaves the blocked list", async () => {
    const { fake, data } = server();
    await shown(data);
    await changeRelationship(data, fake.accountId("marco"), "unblock");
    expect((await shown(data)).blocked).toEqual([]);
  });

  test("a mute from a profile appears in the muted list, an unmute leaves it", async () => {
    const { fake, data } = server();
    await shown(data);
    await changeRelationship(data, fake.accountId("sam"), "mute");
    expect([...(await shown(data)).muted].sort()).toEqual([fake.accountId("hana"), fake.accountId("sam")].sort());
    await changeRelationship(data, fake.accountId("hana"), "unmute");
    expect((await shown(data)).muted).toEqual([fake.accountId("sam")]);
  });

  test("a block of an account that is muted moves it between the lists the server keeps", async () => {
    const { fake, data } = server();
    await shown(data);
    await changeRelationship(data, fake.accountId("hana"), "block");
    const { blocked } = await shown(data);
    expect(blocked).toContain(fake.accountId("hana"));
  });

  test("a block the server refused leaves the lists as they were", async () => {
    const { fake, data } = server();
    await shown(data);
    fake.script("POST /accounts/{id}/block", { fail: 500 });
    await changeRelationship(data, fake.accountId("sam"), "block").then(
      () => undefined,
      () => undefined,
    );
    expect(await shown(data)).toEqual({ blocked: [fake.accountId("marco")], muted: [fake.accountId("hana")] });
  });
});
