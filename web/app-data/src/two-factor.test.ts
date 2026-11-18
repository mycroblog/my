// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { fakeTotpCode } from "@my/e2e/fake";
import type { AppData } from "./app-data";
import {
  disableTotp,
  enableTotp,
  regenerateRecoveryCodes,
  setUpTotp,
  twoFactorQuery,
  useTwoFactor,
  useTwoFactorWrites,
} from "./two-factor";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { kindOf, refusal } from "./test/settings";

/** What a screen shown again shows: the cache while it holds the answer and it is not marked, else a read. */
const status = (data: AppData) => data.queryClient.query({ ...twoFactorQuery(data), staleTime: Infinity });

/** The data of an account without two-factor authentication, which has read its status. */
async function off() {
  const given = fakeData();
  await given.data.queryClient.query(twoFactorQuery(given.data));
  return given;
}

describe("useTwoFactor", () => {
  test("is undefined until the status is read, then the server's", async () => {
    const { data } = fakeData({ access: { twoFactor: true } });
    expect(probe(data, useTwoFactor)).toBeUndefined();
    await data.queryClient.query(twoFactorQuery(data));
    expect(probe(data, useTwoFactor)).toMatchObject({ enabled: true, recovery_codes_remaining: 10 });
  });
});

describe("enableTotp", () => {
  test("turns it on with the code of the authenticator, answers the recovery codes, and the cache says on", async () => {
    const { data } = await off();
    const setup = await setUpTotp(data);
    expect(setup.setup_token).not.toBe("");
    const codes = await enableTotp(data, setup.setup_token, fakeTotpCode);
    expect(codes).toHaveLength(10);
    expect(new Set(codes).size).toBe(10);
    expect(await status(data)).toMatchObject({ enabled: true, recovery_codes_remaining: 10 });
  });

  test("rejects with the field `code` for a wrong code, and the cache says off still", async () => {
    const { data } = await off();
    const { setup_token } = await setUpTotp(data);
    const refused = await refusal(enableTotp(data, setup_token, "000000"));
    expect([kindOf(refused), refused.problem.field]).toEqual(["invalid-two-factor-code", "code"]);
    expect(probe(data, useTwoFactor)).toMatchObject({ enabled: false });
  });

  test("rejects with the setup-expired problem for a setup the server does not know", async () => {
    const { data } = await off();
    const refused = await refusal(enableTotp(data, "no-such-setup", fakeTotpCode));
    expect(kindOf(refused)).toBe("totp-setup-expired");
    expect(probe(data, useTwoFactor)).toMatchObject({ enabled: false });
  });

  test("through the hook, are the same writes bound to the session", async () => {
    const { data } = await off();
    const writes = probe(data, useTwoFactorWrites);
    const { setup_token } = await writes.setUp();
    expect(await writes.enable(setup_token, fakeTotpCode)).toHaveLength(10);
    expect(await status(data)).toMatchObject({ enabled: true });
  });
});

describe("disableTotp", () => {
  test("turns it off with a code, and the cache says off", async () => {
    const { data } = fakeData({ access: { twoFactor: true } });
    await data.queryClient.query(twoFactorQuery(data));
    await disableTotp(data, fakeTotpCode);
    expect(await status(data)).toMatchObject({ enabled: false });
  });

  test("rejects with the field `code` for a wrong code, and the cache says on still", async () => {
    const { data } = fakeData({ access: { twoFactor: true } });
    await data.queryClient.query(twoFactorQuery(data));
    const refused = await refusal(disableTotp(data, "000000"));
    expect([kindOf(refused), refused.problem.field]).toEqual(["invalid-two-factor-code", "code"]);
    expect(probe(data, useTwoFactor)).toMatchObject({ enabled: true });
  });
});

describe("regenerateRecoveryCodes", () => {
  test("answers ten new codes in place of the old, and the cache counts ten", async () => {
    const { data } = fakeData({ access: { twoFactor: true } });
    await data.queryClient.query(twoFactorQuery(data));
    const first = await regenerateRecoveryCodes(data, fakeTotpCode);
    const again = await regenerateRecoveryCodes(data, fakeTotpCode);
    expect(again).toHaveLength(10);
    expect(again.filter((c) => first.includes(c))).toEqual([]);
    expect(await status(data)).toMatchObject({ enabled: true, recovery_codes_remaining: 10 });
  });

  test("rejects with the field `code` for a wrong code", async () => {
    const { data } = fakeData({ access: { twoFactor: true } });
    const refused = await refusal(regenerateRecoveryCodes(data, "000000"));
    expect([kindOf(refused), refused.problem.field]).toEqual(["invalid-two-factor-code", "code"]);
  });
});
