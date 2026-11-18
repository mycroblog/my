// SPDX-License-Identifier: AGPL-3.0-only
// Two-factor authentication with an authenticator app (FR-005): whether it is on and how many recovery codes are left,
// turning it on in two steps (the server gives a secret and a QR code, the reader enters a code the app makes from it),
// turning it off, and making the recovery codes again. The recovery codes are returned by the writes that make them
// and kept nowhere: the screen shows them once.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { writeCached } from "./written";

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
 * the recovery codes, which are shown only this once, as soon as the server gave them: the cache says at once that it
 * is on, with as many recovery codes left, and the state is read again behind, with no screen on it too, for what the
 * answer does not hold (since when). A reader who leaves before that read ends has seen the codes. It rejects with the
 * server's problem: a wrong code (`field` names `code`), an expired setup, too many wrong codes.
 */
export async function enableTotp(data: AppData, setupToken: string, code: string): Promise<readonly string[]> {
  const { data: codes } = await data.api.POST("/two_factor/totp", { body: { setup_token: setupToken, code } });
  const made = codes?.recovery_codes ?? [];
  const { queryKey } = twoFactorQuery(data);
  await writeCached<TwoFactor>(data, queryKey, (now) => ({
    ...now,
    enabled: true,
    recovery_codes_remaining: made.length,
  }));
  void data.queryClient.invalidateQueries({ queryKey, refetchType: "all" });
  return made;
}

/**
 * Turns two-factor authentication off once `code`, one of the app or an unused recovery code, confirms it, and puts
 * the state the server answered in the cache, the reads of it on their way cancelled first. It rejects with the
 * server's problem (a wrong code, too many wrong codes).
 */
export async function disableTotp(data: AppData, code: string): Promise<void> {
  const { data: state } = await data.api.POST("/two_factor/totp/disable", { body: { code } });
  if (state === undefined) return;
  const { queryKey } = twoFactorQuery(data);
  await data.queryClient.cancelQueries({ queryKey });
  data.queryClient.setQueryData<TwoFactor>(queryKey, state);
}

/**
 * Makes new recovery codes once `code` confirms it, and returns them, shown only this once; the old ones stop working.
 * It rejects as `disableTotp` does.
 */
export async function regenerateRecoveryCodes(data: AppData, code: string): Promise<readonly string[]> {
  const { data: codes } = await data.api.POST("/two_factor/recovery_codes", { body: { code } });
  const made = codes?.recovery_codes ?? [];
  await writeCached<TwoFactor>(data, twoFactorQuery(data).queryKey, (now) => ({
    ...now,
    recovery_codes_remaining: made.length,
  }));
  return made;
}

/**
 * The setup the screen that turns two-factor authentication on shows: a new secret, asked for as the screen opens and
 * kept while it stays open, never longer, so that the next visit gets another.
 */
export function totpSetupQuery(data: AppData) {
  return {
    queryKey: ["post", "/two_factor/totp/setup"] as const,
    queryFn: () => setUpTotp(data),
    staleTime: Infinity,
    gcTime: 0,
  };
}

/**
 * The setup of the screen that turns two-factor authentication on: `loading` until the server gave a secret, `failed`
 * when it did not, and `restart`, which asks for a new one (after a failure, or once the setup expired). A restart
 * first drops the setup it had, whose secret works no more: until the new one is read there is none, and the screen
 * says that it loads, or that it failed, never the old secret.
 */
export function useTotpSetup(): {
  setup: TotpSetup | undefined;
  status: "loading" | "failed" | "ready";
  restart: () => void;
} {
  const appData = useAppData();
  const query = totpSetupQuery(appData);
  const { data, isError, isFetching } = useQuery(query);
  return {
    setup: data,
    status: data !== undefined ? "ready" : isError && !isFetching ? "failed" : "loading",
    restart: () => {
      void appData.queryClient.resetQueries({ queryKey: query.queryKey });
    },
  };
}

/** Whether `error` says the setup of `enableTotp` is unknown or expired: the reader starts a new one (`setUpTotp`). */
export function totpSetupExpired(error: unknown): boolean {
  return error instanceof ProblemError && error.type === "/api/v1/problems/totp-setup-expired";
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
