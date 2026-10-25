// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the user's two-factor authentication with an authenticator app (FR-005): where it
// stands, a new secret to enroll (nothing is stored until a code confirms it), turning it on, replacing the
// recovery codes, and turning it off. Signing in with the code is the server's own sign-in page (web-client/08-server-rendered-pages.md §8).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { components } from "@mycroblog/mykit";

type Schemas = components["schemas"];

export type TwoFactor = Schemas["TwoFactor"];
export type TotpSetup = Schemas["TotpSetup"];
export type RecoveryCodes = Schemas["RecoveryCodes"];

/** The most characters of a code that the API takes (a TOTP code or a recovery code). */
export const twoFactorCodeLimit = 64;

/** Where the user's two-factor authentication stands (GET /two_factor). */
export function twoFactorQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/two_factor");
}

/** Generates a new secret to add to an authenticator app (POST /two_factor/totp/setup), as the options of useMutation. */
export function setUpTotpMutation(api: Api) {
  return {
    mutationKey: ["post", "/two_factor/totp/setup"] as const,
    mutationFn: async (): Promise<TotpSetup> => {
      const { data, error } = await api.fetch.POST("/two_factor/totp/setup");
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Turns two-factor authentication on with the secret of a setup and a code of the app (POST /two_factor/totp). */
export function enableTotpMutation(api: Api) {
  return {
    mutationKey: ["post", "/two_factor/totp"] as const,
    mutationFn: async (body: { setup_token: string; code: string }): Promise<RecoveryCodes> => {
      const { data, error } = await api.fetch.POST("/two_factor/totp", { body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Replaces the recovery codes, confirmed by a code (POST /two_factor/recovery_codes). */
export function regenerateRecoveryCodesMutation(api: Api) {
  return {
    mutationKey: ["post", "/two_factor/recovery_codes"] as const,
    mutationFn: async (code: string): Promise<RecoveryCodes> => {
      const { data, error } = await api.fetch.POST("/two_factor/recovery_codes", { body: { code } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Turns two-factor authentication off, confirmed by a code (POST /two_factor/totp/disable). */
export function disableTotpMutation(api: Api) {
  return {
    mutationKey: ["post", "/two_factor/totp/disable"] as const,
    mutationFn: async (code: string): Promise<TwoFactor> => {
      const { data, error } = await api.fetch.POST("/two_factor/totp/disable", { body: { code } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * What turning two-factor authentication on or off, or replacing the recovery codes, changes (web-client/README.md S1,
 * ADR-0045): where it stands (on since when, how many codes are left). Turning it off answers the new state, which
 * goes into the cache; the others mark it stale, and this resolves once it is fetched again.
 */
export async function cacheChangedTwoFactor(api: Api, queryClient: QueryClient, answered?: TwoFactor): Promise<void> {
  const { queryKey } = twoFactorQueryOptions(api);
  if (answered !== undefined) {
    queryClient.setQueryData(queryKey, answered);
    return;
  }
  await queryClient.invalidateQueries({ queryKey });
}

/** The problem types of two-factor authentication that the screen tells apart (api/openapi.yaml, Problem). */
const invalidCodeType = "/api/v1/problems/invalid-two-factor-code";
const setupExpiredType = "/api/v1/problems/totp-setup-expired";
const alreadyEnabledType = "/api/v1/problems/two-factor-enabled";
const notEnabledType = "/api/v1/problems/two-factor-not-enabled";

/** Why a code was refused, by what the user can do about it. */
export type TwoFactorFailure = "invalid-code" | "setup-expired" | "state-changed" | "rate-limited" | "other";

/** Classifies an error of a two-factor operation (web-client/README.md E2). */
export function twoFactorFailure(error: unknown): TwoFactorFailure {
  if (!(error instanceof ApiError)) return "other";
  if (error.status === 429) return "rate-limited";
  switch (error.type) {
    case invalidCodeType:
      return "invalid-code";
    case setupExpiredType:
      return "setup-expired";
    case alreadyEnabledType:
    case notEnabledType:
      return "state-changed";
    default:
      return "other";
  }
}
