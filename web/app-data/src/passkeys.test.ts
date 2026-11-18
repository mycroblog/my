// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { addPasskey, deletePasskey, passkeysQuery, usePasskeys, usePasskeyWrites } from "./passkeys";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { kindOf, passkeyConfirmation, refusal } from "./test/settings";

describe("usePasskeys", () => {
  test("is undefined until the list is read, then the passkeys the server holds", async () => {
    const { data } = fakeData({ access: { passkeys: 2 } });
    expect(probe(data, usePasskeys)).toBeUndefined();
    await data.queryClient.query(passkeysQuery(data));
    expect(probe(data, usePasskeys)?.length).toBe(2);
  });
});

describe("deletePasskey", () => {
  test("deletes it, and the list no longer holds it", async () => {
    const { data } = fakeData();
    const [first, ...others] = (await data.queryClient.query(passkeysQuery(data))).items;
    await deletePasskey(data, first?.id ?? "");
    expect(probe(data, usePasskeys)?.map((p) => p.id)).toEqual(others.map((p) => p.id));
  });

  test("rejects with the last-sign-in-method problem for the only way to sign in, and the list keeps it", async () => {
    const { data } = fakeData({ access: { password: false, passkeys: 1, linked: "none" } });
    const [only] = (await data.queryClient.query(passkeysQuery(data))).items;
    const refused = await refusal(deletePasskey(data, only?.id ?? ""));
    expect(kindOf(refused)).toBe("last-sign-in-method");
    expect(probe(data, usePasskeys)?.map((p) => p.id)).toEqual([only?.id ?? ""]);
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data } = fakeData();
    const [first] = (await data.queryClient.query(passkeysQuery(data))).items;
    await probe(data, usePasskeyWrites).remove(first?.id ?? "");
    expect(probe(data, usePasskeys)?.some((p) => p.id === first?.id)).toBe(false);
  });
});

describe("addPasskey", () => {
  test("rejects with the too-many-passkeys problem at the limit, and the list is as it was", async () => {
    const { data } = fakeData({ access: { passkeys: 20 } });
    await data.queryClient.query(passkeysQuery(data));
    const refused = await refusal(addPasskey(data, "One more", await passkeyConfirmation(data)));
    expect(kindOf(refused)).toBe("too-many-passkeys");
    expect(probe(data, usePasskeys)?.length).toBe(20);
  });

  test("rejects with the wrong-password problem for a password that is not the account's, naming no passkey", async () => {
    const { data } = fakeData();
    await data.queryClient.query(passkeysQuery(data));
    const refused = await refusal(addPasskey(data, "Laptop", { kind: "password", password: "not the password" }));
    expect([kindOf(refused), refused.problem.field]).toEqual(["wrong-password", "password"]);
    expect(probe(data, usePasskeys)?.length).toBe(3);
  });

  test.todo(
    "appends the new passkey to the list: the platform's createPasskey has no seam for a unit test on Bun (a finding)",
  );
});
