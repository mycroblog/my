// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { AppData } from "./app-data";
import { createOwnInvite, ownInvitesQuery, revokeOwnInvite, useOwnInvites, useOwnInviteWrites } from "./invites";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { kindOf, refusal } from "./test/settings";

/** The data after the first page of the invites was read. */
async function read(options: Parameters<typeof fakeData>[0] = {}) {
  const given = fakeData(options);
  await given.data.queryClient.infiniteQuery(ownInvitesQuery(given.data));
  return given;
}

/** What the screen shows after the list is marked and shown again. */
async function again(data: AppData) {
  await data.queryClient.infiniteQuery({ ...ownInvitesQuery(data), staleTime: Infinity });
  return probe(data, useOwnInvites);
}

describe("useOwnInvites", () => {
  test("holds nothing of the server's until the first page is read, then the invites, the right and the limits", async () => {
    const { data } = fakeData();
    const before = probe(data, useOwnInvites);
    expect(before.limits).toBeUndefined();
    expect(before.items).toEqual([]);
    await data.queryClient.infiniteQuery(ownInvitesQuery(data));
    const read = probe(data, useOwnInvites);
    expect(read.canCreate).toBe(true);
    expect(read.usable).toBe(2);
    expect(read.limits).toMatchObject({ max_active: 5, max_uses: 10 });
    expect(new Set(read.items.map((i) => i.state))).toEqual(
      new Set(["usable", "revoked", "expired", "exhausted", "creator_ineligible"]),
    );
  });

  test("says an account that may not create invites cannot, and counts none usable", async () => {
    const { data } = await read({ access: { invites: "not-allowed" } });
    const listed = probe(data, useOwnInvites);
    expect([listed.canCreate, listed.usable]).toEqual([false, 0]);
  });
});

describe("createOwnInvite", () => {
  test("creates one with the server's defaults, and a screen shown again lists it first and counts it usable", async () => {
    const { data } = await read();
    const made = await createOwnInvite(data);
    expect(made.state).toBe("usable");
    expect(made.url).toContain(made.code);
    const listed = await again(data);
    expect(listed.items[0]?.id).toBe(made.id);
    expect(listed.usable).toBe(3);
  });

  test("creates one with the uses and the lifetime asked for", async () => {
    const { data } = await read();
    const made = await createOwnInvite(data, { max_uses: 3, expires_in: 3600 });
    expect(made.max_uses).toBe(3);
    expect(Date.parse(made.expires_at) - Date.parse(made.created_at)).toBeLessThanOrEqual(3600 * 1000 + 1000);
  });

  test("rejects with the field of a value above the limits, and the list is as it was", async () => {
    const { data } = await read();
    const uses = await refusal(createOwnInvite(data, { max_uses: 11 }));
    const lifetime = await refusal(createOwnInvite(data, { expires_in: 31 * 24 * 3600 }));
    expect([uses.problem.field, lifetime.problem.field]).toEqual(["max_uses", "expires_in"]);
    expect((await again(data)).usable).toBe(2);
  });

  test("rejects with the too-many-invites problem at the cap", async () => {
    const { data } = await read({ access: { invites: "at-cap" } });
    expect(probe(data, useOwnInvites).usable).toBe(5);
    expect(kindOf(await refusal(createOwnInvite(data)))).toBe("too-many-invites");
  });

  test("rejects with the not-allowed problem for an account that may not create", async () => {
    const { data } = await read({ access: { invites: "not-allowed" } });
    const refused = await refusal(createOwnInvite(data));
    expect([refused.problem.status, kindOf(refused)]).toEqual([403, "invites-not-allowed"]);
  });

  test("rejects with the single-user-mode problem on a one-person server", async () => {
    const { data } = fakeData({ singleUser: true });
    const refused = await refusal(createOwnInvite(data));
    expect([refused.problem.status, kindOf(refused)]).toEqual([404, "single-user-mode"]);
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data } = await read();
    const made = await probe(data, useOwnInviteWrites).create({});
    expect((await again(data)).items[0]?.id).toBe(made.id);
  });
});

describe("revokeOwnInvite", () => {
  test("revokes it, answers it revoked, and a screen shown again reads it so and counts one usable less", async () => {
    const { data } = await read();
    const usable = probe(data, useOwnInvites).items.find((i) => i.state === "usable");
    const revoked = await revokeOwnInvite(data, usable?.id ?? "");
    expect(revoked.state).toBe("revoked");
    const listed = await again(data);
    expect(listed.items.find((i) => i.id === usable?.id)?.state).toBe("revoked");
    expect(listed.usable).toBe(1);
  });

  test("rejects with the single-user-mode problem on a one-person server", async () => {
    const { data } = fakeData({ singleUser: true });
    expect(kindOf(await refusal(revokeOwnInvite(data, "any")))).toBe("single-user-mode");
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data } = await read();
    const usable = probe(data, useOwnInvites).items.find((i) => i.state === "usable");
    await probe(data, useOwnInviteWrites).revoke(usable?.id ?? "");
    expect((await again(data)).usable).toBe(1);
  });
});
