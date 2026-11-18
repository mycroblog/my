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
import { kindOf, refusal, settled } from "./test/settings";

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

  // The codes are shown once, and the server has turned it on: nothing the app reads afterwards may keep them from
  // the reader.
  test("answers the codes as soon as the server turned it on, the cache saying on with as many codes left, while the status it reads again in the background, with no screen on it, has no answer", async () => {
    const { data, fake } = await off();
    const { setup_token } = await setUpTotp(data);
    // Eight codes, where the server makes ten: what the cache counts is what the answer holds.
    fake.script("POST /two_factor/totp", {
      change: (body) => ({ ...body, recovery_codes: body.recovery_codes.slice(0, 8) }),
    });
    const reread = fake.script("GET /two_factor", { hold: "request" });

    const codes = await enableTotp(data, setup_token, fakeTotpCode);

    expect(codes).toHaveLength(8);
    expect(probe(data, useTwoFactor)).toMatchObject({ enabled: true, recovery_codes_remaining: 8 });
    await reread.arrived();
    reread.release();
    await settled(data);
    expect(reread.seen).toHaveLength(1);
    // The server's own count, once it answered.
    expect(probe(data, useTwoFactor)).toMatchObject({ enabled: true, recovery_codes_remaining: 10 });
  });

  test("leaves the cache saying on when the status it reads again could not be read", async () => {
    const { data, fake } = await off();
    const { setup_token } = await setUpTotp(data);
    const reread = fake.script("GET /two_factor", { fail: 500 });

    expect(await enableTotp(data, setup_token, fakeTotpCode)).toHaveLength(10);
    await reread.arrived();
    await settled(data);

    expect(reread.seen).toHaveLength(1);
    expect(probe(data, useTwoFactor)).toMatchObject({ enabled: true, recovery_codes_remaining: 10 });
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data } = await off();
    const { setup_token } = await setUpTotp(data);
    expect(await probe(data, useTwoFactorWrites).enable(setup_token, fakeTotpCode)).toHaveLength(10);
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
