// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import {
  deactivateOwnAccount,
  deactivationPreviewQuery,
  deleteOwnAccount,
  useDeactivationPreview,
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

  test("a wrong confirmation is refused with the problem a screen tells apart from the others", async () => {
    const { data } = fakeData();
    const refused = await refusedWith(deactivateOwnAccount(data, { password: "wrong" }));
    expect([refused.status, refused.type]).toEqual([403, "/api/v1/problems/reauthentication-failed"]);
  });

  test("an account that cannot be deactivated is refused with a problem naming why", async () => {
    const { data } = fakeData({ settings: { deactivationRefusal: "paid_subscription" } });
    const refused = await refusedWith(deactivateOwnAccount(data, { password }));
    expect([refused.status, refused.type]).toEqual([409, "/api/v1/problems/deactivation-paid_subscription"]);
  });
});

describe("deleteOwnAccount", () => {
  test("deletes with a confirmation", async () => {
    const { data, requests } = fakeData();
    await deleteOwnAccount(data, { password });
    expect(requests).toEqual(["POST /accounts/delete"]);
  });

  test("a wrong confirmation is refused with the problem a screen tells apart from the others", async () => {
    const { data } = fakeData();
    const refused = await refusedWith(deleteOwnAccount(data, { password: "wrong" }));
    expect([refused.status, refused.type]).toEqual([403, "/api/v1/problems/reauthentication-failed"]);
  });
});
