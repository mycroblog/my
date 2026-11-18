// SPDX-License-Identifier: AGPL-3.0-only
// Two-factor authentication with an authenticator app (FR-005): whether it is on and how many recovery codes are left,
// turning it on in two steps (the server gives a secret and a QR code, the reader enters a code the app makes from it),
// turning it off, and making the recovery codes again. The recovery codes are returned by the writes that make them
// and kept nowhere: the screen shows them once.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** Whether two-factor authentication is on, since when, and how many recovery codes are unused. */
export type TwoFactor = components["schemas"]["TwoFactor"];

/** The secret of a setup, as text, as an `otpauth://` URI and as a QR code, with the token that carries it. */
export type TotpSetup = components["schemas"]["TotpSetup"];

/** The state of the account's two-factor authentication. */
export function twoFactorQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/two_factor");
}

/** Asks for a new secret to add to an authenticator app; nothing is stored until `enableTotp`. It rejects with the problem. */
export async function setUpTotp(data: AppData): Promise<TotpSetup> {
  const { data: setup } = await data.api.POST("/two_factor/totp/setup");
  if (setup === undefined) throw new Error("app-data: /two_factor/totp/setup answered without a secret");
  return setup;
}

/**
 * Turns two-factor authentication on with the secret of `setupToken`, once `code` shows that the app has it, and returns
 * the recovery codes, which are shown only this once. It rejects with the server's problem: a wrong code (`field`
 * names `code`), an expired setup, too many wrong codes.
 */
export async function enableTotp(data: AppData, setupToken: string, code: string): Promise<readonly string[]> {
  const { data: codes } = await data.api.POST("/two_factor/totp", { body: { setup_token: setupToken, code } });
  // Read again even with no screen on it: a later write patches the cached state, which must not be the old one.
  await data.queryClient.invalidateQueries({ queryKey: twoFactorQuery(data).queryKey, refetchType: "all" });
  return codes?.recovery_codes ?? [];
}

/**
 * Turns two-factor authentication off once `code`, one of the app or an unused recovery code, confirms it. It rejects
 * with the server's problem (a wrong code, too many wrong codes).
 */
export async function disableTotp(data: AppData, code: string): Promise<void> {
  const { data: state } = await data.api.POST("/two_factor/totp/disable", { body: { code } });
  if (state !== undefined) data.queryClient.setQueryData<TwoFactor>(twoFactorQuery(data).queryKey, state);
}

/**
 * Makes new recovery codes once `code` confirms it, and returns them, shown only this once; the old ones stop working.
 * It rejects as `disableTotp` does.
 */
export async function regenerateRecoveryCodes(data: AppData, code: string): Promise<readonly string[]> {
  const { data: codes } = await data.api.POST("/two_factor/recovery_codes", { body: { code } });
  const made = codes?.recovery_codes ?? [];
  data.queryClient.setQueryData<TwoFactor>(twoFactorQuery(data).queryKey, (now) =>
    now === undefined ? now : { ...now, recovery_codes_remaining: made.length },
  );
  return made;
}

/** The state of two-factor authentication; undefined until the server answered. */
export function useTwoFactor(): TwoFactor | undefined {
  return useQuery(twoFactorQuery(useAppData())).data;
}

export type TwoFactorWrites = {
  readonly setUp: () => Promise<TotpSetup>;
  readonly enable: (setupToken: string, code: string) => Promise<readonly string[]>;
  readonly disable: (code: string) => Promise<void>;
  readonly regenerateCodes: (code: string) => Promise<readonly string[]>;
};

/** Turning two-factor authentication on and off and making recovery codes, from a screen. */
export function useTwoFactorWrites(): TwoFactorWrites {
  const data = useAppData();
  return {
    setUp: () => setUpTotp(data),
    enable: (setupToken, code) => enableTotp(data, setupToken, code),
    disable: (code) => disableTotp(data, code),
    regenerateCodes: (code) => regenerateRecoveryCodes(data, code),
  };
}
