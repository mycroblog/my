// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import {
  deactivateOwnAccount,
  deactivationPreviewQuery,
  deactivationRefused,
  deleteOwnAccount,
  lastAdministratorRefused,
  loadDeletion,
  useDeactivationPreview,
  useDeletionRead,
} from "./deactivation";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { refusedWith } from "./test/refused";

/** The password of the reader (yuki), which confirms a deactivation and a deletion. */
const password = scenario.people.find((p) => p.username === "yuki")?.password ?? "";

describe("useDeactivationPreview", () => {
  test("is undefined until read, then when the account would be deleted, with no refusal", async () => {
    const { data } = fakeData();
    expect(probe(data, useDeactivationPreview)).toBeUndefined();
    await data.queryClient.query(deactivationPreviewQuery(data));
    const preview = probe(data, useDeactivationPreview);
    expect(Date.parse(preview?.deletion_due_at ?? "")).toBeGreaterThan(Date.now());
    expect(preview?.refusal).toBeUndefined();
  });

  test("names why the account cannot be deactivated", async () => {
    const { data } = fakeData({ settings: { deactivationRefusal: "last_administrator" } });
    await data.queryClient.query(deactivationPreviewQuery(data));
    expect(probe(data, useDeactivationPreview)?.refusal).toBe("last_administrator");
  });
});

describe("useDeletionRead", () => {
  test("is loading until the deletion's screen is read, then the account and what deactivating would do", async () => {
    const { fake, data } = fakeData();
    expect(probe(data, useDeletionRead).status).toBe("loading");
    await loadDeletion(data);
    const read = probe(data, useDeletionRead);
    expect(read.status).toBe("read");
    expect(read.account?.id).toBe(fake.accountId("yuki"));
    expect(Date.parse(read.preview?.deletion_due_at ?? "")).toBeGreaterThan(Date.now());
    expect(read.preview?.refusal).toBeUndefined();
  });

  test("names the last administrator where deactivating would be refused for it", async () => {
    const { data } = fakeData({ settings: { deactivationRefusal: "last_administrator" } });
    await loadDeletion(data);
    const read = probe(data, useDeletionRead);
    expect(read.status).toBe("read");
    expect(read.preview?.refusal).toBe("last_administrator");
  });
});

describe("deactivateOwnAccount", () => {
  test("returns when the account will be deleted, for a password, a passkey or an external confirmation", async () => {
    for (const proof of [{ password }, { passkey: { id: "key" } }, { external_confirmation: "confirmed" }] as const) {
      const { fake, data, requests } = fakeData();
      const sent = fake.script("POST /accounts/deactivation");
      const deactivation = await deactivateOwnAccount(data, proof as never);
      expect(Date.parse(deactivation.deletion_due_at)).toBeGreaterThan(Date.now());
      expect(requests).toEqual(["POST /accounts/deactivation"]);
      expect(sent.seen).toHaveLength(1);
    }
  });

  test("a wrong password is refused with the problem a screen tells apart from the others", async () => {
    const { data } = fakeData();
    const refused = await refusedWith(deactivateOwnAccount(data, { password: "wrong" }));
    expect([refused.status, refused.type]).toEqual([422, "/api/v1/problems/wrong-password"]);
    expect(deactivationRefused(refused)).toBe(false);
  });

  test("an account that cannot be deactivated is refused with the problem that says so, its detail saying why", async () => {
    const { data } = fakeData({ settings: { deactivationRefusal: "paid_subscription" } });
    const refused = await refusedWith(deactivateOwnAccount(data, { password }));
    expect([refused.status, refused.type]).toEqual([409, "/api/v1/problems/deactivation-refused"]);
    // The detail says why, in the server's words.
    expect(refused.problem.detail?.trim() ?? "").not.toBe("");
    expect(deactivationRefused(refused)).toBe(true);
  });
});

describe("deleteOwnAccount", () => {
  test("deletes with a confirmation", async () => {
    const { data, requests } = fakeData();
    await deleteOwnAccount(data, { password });
    expect(requests).toEqual(["POST /accounts/delete"]);
  });

  test("a wrong password is refused with the problem a screen tells apart from the others", async () => {
    const { data } = fakeData();
    const refused = await refusedWith(deleteOwnAccount(data, { password: "wrong" }));
    expect([refused.status, refused.type]).toEqual([422, "/api/v1/problems/wrong-password"]);
    expect(lastAdministratorRefused(refused)).toBe(false);
  });

  test("the only administrator is refused once the confirmation holds, with the problem that says so", async () => {
    for (const settings of [
      { deletionRefusal: "last_administrator" },
      { deactivationRefusal: "last_administrator" },
    ] as const) {
      const { data, requests } = fakeData({ settings });
      const refused = await refusedWith(deleteOwnAccount(data, { password }));
      expect([refused.status, refused.type]).toEqual([409, "/api/v1/problems/last-administrator"]);
      expect(lastAdministratorRefused(refused)).toBe(true);
      expect(requests).toEqual(["POST /accounts/delete"]);
    }
  });

  test("a confirmation that does not hold is told before the last administrator", async () => {
    const { data } = fakeData({ settings: { deactivationRefusal: "last_administrator" } });
    const refused = await refusedWith(deleteOwnAccount(data, { password: "wrong" }));
    expect(refused.type).toBe("/api/v1/problems/wrong-password");
  });

  test("lastAdministratorRefused is false for what is no problem of the server", () => {
    expect(lastAdministratorRefused(new TypeError("Failed to fetch"))).toBe(false);
    expect(lastAdministratorRefused(undefined)).toBe(false);
  });
});
