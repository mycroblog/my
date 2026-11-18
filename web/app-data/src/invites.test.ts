// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { AppData } from "./app-data";
import { instanceQuery } from "./instance";
import {
  createOwnInvite,
  inviteStateNow,
  inviteValueRefused,
  invitesNotAllowed,
  invitesNotAllowedReason,
  loadInviteRight,
  ownInvitesQuery,
  revokeOwnInvite,
  useInviteRightKnown,
  useMayInvite,
  useOwnInviteRead,
  useOwnInvites,
  useOwnInviteWrites,
} from "./invites";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { kindOf, refusal } from "./test/settings";

const day = 24 * 3600;

/** The reads of the list of invites among `requests`. */
const listReads = (requests: readonly string[]) => requests.filter((request) => request.startsWith("GET /invites"));

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

// #3116: the cause reaches a screen from the list, and from the refusal of a new invite.
describe("why an account cannot create invites", () => {
  for (const [named, reason] of [
    ["not_permitted", "not_permitted"],
    ["not_approved", "not_approved"],
    ["restricted", "restricted"],
    // A refusal that names no cause: the account still cannot, and no cause is made up.
    ["unknown", undefined],
  ] as const)
    test(`the list and the refusal of a new invite both say ${reason ?? "no cause where the server names none"}`, async () => {
      const { data } = await read({ access: { invites: "not-allowed", invitesRefusal: named } });
      const listed = probe(data, useOwnInvites);
      expect([listed.canCreate, listed.cannotCreateReason, listed.registrationClosed]).toEqual([false, reason, false]);
      const refused = await refusal(createOwnInvite(data));
      expect([invitesNotAllowed(refused), invitesNotAllowedReason(refused)]).toEqual([true, reason]);
    });

  test("on a server that takes no new account, both say that the registration is closed", async () => {
    const { data } = await read({ registrations: "closed" });
    // That the registration is closed is the instance's to say, which the shell read before any screen.
    await data.queryClient.query(instanceQuery(data));
    const listed = probe(data, useOwnInvites);
    expect([listed.canCreate, listed.cannotCreateReason, listed.registrationClosed]).toEqual([
      false,
      "registration_closed",
      true,
    ]);
    const refused = await refusal(createOwnInvite(data));
    expect([invitesNotAllowed(refused), invitesNotAllowedReason(refused)]).toEqual([true, "registration_closed"]);
  });

  test("an account that may create invites has no cause, and its server's registration is not closed", async () => {
    const { data } = await read();
    const listed = probe(data, useOwnInvites);
    expect([listed.canCreate, listed.cannotCreateReason, listed.registrationClosed]).toEqual([true, undefined, false]);
  });

  test("another refusal, and an error that is no refusal, are not that the account may not create invites", async () => {
    const { data } = await read({ access: { invites: "at-cap" } });
    const atCap = await refusal(createOwnInvite(data));
    const lost = new Error("the request got no answer");
    expect([atCap, lost].map((error) => [invitesNotAllowed(error), invitesNotAllowedReason(error)])).toEqual([
      [false, undefined],
      [false, undefined],
    ]);
  });

  test("a cause this app has no word for (a newer server's) is no cause", async () => {
    const { fake, data } = await read();
    const refused = await refusal(createOwnInvite(fakeData({ access: { invites: "not-allowed" } }).data));
    // The specification names four causes: a fifth is what a server newer than this app would send.
    fake.script("POST /invites", { fail: { ...refused.problem, reason: "quota" } as typeof refused.problem });
    const newer = await refusal(createOwnInvite(data));
    expect([invitesNotAllowed(newer), invitesNotAllowedReason(newer)]).toEqual([true, undefined]);
  });
});

describe("inviteValueRefused", () => {
  test("names the value of a new invite that the server refused, and nothing for another refusal", async () => {
    const { fake, data } = await read();
    const uses = await refusal(createOwnInvite(data, { max_uses: 11 }));
    const lifetime = await refusal(createOwnInvite(data, { expires_in: 31 * day }));
    expect([uses, lifetime].map(inviteValueRefused)).toEqual(["max_uses", "expires_in"]);

    // A 422 about a member the form does not have, the limit of usable invites, and no refusal at all.
    fake.script("POST /invites", { fail: { type: "about:blank", title: "Invalid", status: 422, field: "note" } });
    const other = await refusal(createOwnInvite(data));
    const atCap = await refusal(createOwnInvite(fakeData({ access: { invites: "at-cap" } }).data));
    expect([other, atCap, new Error("the request got no answer")].map(inviteValueRefused)).toEqual([
      undefined,
      undefined,
      undefined,
    ]);
  });
});

describe("inviteStateNow", () => {
  test("is the invite's state as the server says it, but an invite that would work does not while the registration is closed", async () => {
    const { data } = await read();
    const { items } = probe(data, useOwnInvites);
    expect(items.map((invite) => inviteStateNow(invite, false))).toEqual([
      "usable",
      "usable",
      "revoked",
      "expired",
      "exhausted",
      "creator_ineligible",
    ]);
    expect(items.map((invite) => inviteStateNow(invite, true))).toEqual([
      "registration_closed",
      "registration_closed",
      "revoked",
      "expired",
      "exhausted",
      "creator_ineligible",
    ]);
  });
});

describe("createOwnInvite, refused", () => {
  test("as an account that may not create invites, leaves the list saying so with the cause at once, before any new read of it", async () => {
    const { problem } = await refusal(
      createOwnInvite(fakeData({ access: { invites: "not-allowed", invitesRefusal: "restricted" } }).data),
    );
    const { fake, data, requests } = await read();
    fake.script("POST /invites", { fail: problem, times: 1 });
    // A new read of the list would not answer: what the list says now is what the refusal left in it.
    const reread = fake.script("GET /invites", { hold: "request" });

    const refused = await refusal(createOwnInvite(data));

    expect(invitesNotAllowedReason(refused)).toBe("restricted");
    const listed = probe(data, useOwnInvites);
    expect([listed.canCreate, listed.cannotCreateReason, listed.items.length]).toEqual([false, "restricted", 6]);
    reread.end();
    // A screen shown again reads the list: on this server, the account may create invites after all.
    expect((await again(data)).canCreate).toBe(true);
    expect(listReads(requests)).toHaveLength(2);
  });

  for (const [what, options, request] of [
    ["a value over the server's most", {}, { max_uses: 11 }],
    ["the limit of usable invites", { access: { invites: "at-cap" } }, {}],
    ["an account that may not create invites", { access: { invites: "not-allowed" } }, {}],
  ] as const)
    test(`for ${what}, has the list read again by a screen that shows it`, async () => {
      const { data, requests } = await read(options);
      await refusal(createOwnInvite(data, request));
      await again(data);
      expect(listReads(requests)).toHaveLength(2);
    });
});

describe("useOwnInviteRead", () => {
  test("is loading until the list is read, then holds the invite of the ID; an ID of none is missing once no page is left", async () => {
    const { fake, data } = fakeData();
    const before = probe(data, () => useOwnInviteRead(fake.inviteId(3)));
    expect([before.status, before.invite]).toEqual(["loading", undefined]);

    await data.queryClient.infiniteQuery(ownInvitesQuery(data));

    const found = probe(data, () => useOwnInviteRead(fake.inviteId(3)));
    expect([found.status, found.invite?.id, found.invite?.state]).toEqual(["read", fake.inviteId(3), "expired"]);
    const none = probe(data, () => useOwnInviteRead(fake.inviteId(99)));
    expect([none.status, none.invite]).toEqual(["missing", undefined]);
  });

  test("while a page of the list is left, an ID not found yet is still loading: found, or missing, once the list is read whole", async () => {
    const { fake, data } = await read({ access: { invites: "many" } });
    const statuses = () =>
      [fake.inviteId(0), fake.inviteId(25), fake.inviteId(99)].map(
        (id) => probe(data, () => useOwnInviteRead(id)).status,
      );
    expect(statuses()).toEqual(["read", "loading", "loading"]);

    await data.queryClient.infiniteQuery({ ...ownInvitesQuery(data), pages: 2, staleTime: 0 });

    expect(statuses()).toEqual(["read", "read", "missing"]);
  });
});

// What the settings' menu waits for, and lists Invites by (ADR-0106 §18).
describe("whether the reader may create invites", () => {
  /** Whether it is known, and what it is. */
  const right = (data: AppData) => [probe(data, useInviteRightKnown), probe(data, useMayInvite)];

  test("is not known until the instance and the first page of the invites are read, and is then what the list says", async () => {
    const { data, requests } = fakeData();
    expect(probe(data, useInviteRightKnown)).toBe(false);
    await data.queryClient.query(instanceQuery(data));
    expect(probe(data, useInviteRightKnown)).toBe(false);

    await loadInviteRight(data);

    expect(right(data)).toEqual([true, true]);
    expect(listReads(requests)).toHaveLength(1);
  });

  test("an account that may not create invites may not", async () => {
    const { data } = fakeData({ access: { invites: "not-allowed" } });
    await loadInviteRight(data);
    expect(right(data)).toEqual([true, false]);
  });

  for (const [what, options] of [
    ["a server that takes no new account", { registrations: "closed" }],
    ["a one-person server", { singleUser: true, viewer: "hana" }],
  ] as const)
    test(`on ${what} nobody may, which is known from the instance: the invites are not asked for`, async () => {
      const { data, requests } = fakeData(options);
      await loadInviteRight(data);
      expect(right(data)).toEqual([true, false]);
      expect(requests).toEqual(["GET /instance"]);
    });

  // What the menu then lists needs a client's render (the app's test): a render on the server takes a read that
  // failed for one on its way.
  test("the read of it does not fail when the invites could not be read: the screen that waits for it still opens", async () => {
    const { fake, data, requests } = fakeData();
    fake.script("GET /invites", { fail: 500 });
    await loadInviteRight(data);
    expect(listReads(requests)).toHaveLength(1);
  });
});
