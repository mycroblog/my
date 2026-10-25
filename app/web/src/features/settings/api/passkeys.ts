// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the user's passkeys (FR-004): the list, registering one (the server checks the password
// or another passkey and answers the options, the browser creates the passkey, and the server verifies and stores it),
// removing one, and confirming an operation with a passkey in place of the password. Signing in with a passkey is the
// server's own sign-in page (web-client/08-server-rendered-pages.md §8).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { createPasskey, getPasskey, passkeyPromptFailure } from "../../../lib/webauthn";

export type Passkey = components["schemas"]["Passkey"];
/** A passkey's answer that confirms an operation in place of the password. */
export type PasskeyAssertion = components["schemas"]["PasskeyAssertion"];

/**
 * Confirms an operation with one of the user's passkeys: the options (POST /passkeys/confirmation), then the browser's
 * prompt. The answer goes in the `passkey` of the operation's request. An error is an ApiError, a NetworkError, the
 * DOMException of the prompt, or a PasskeyOptionsError (lib/webauthn).
 */
export async function confirmWithPasskey(api: Api): Promise<PasskeyAssertion> {
  const begun = await api.fetch.POST("/passkeys/confirmation");
  // The middleware throws an ApiError for every error status; this narrows the type.
  if (begun.data === undefined) throw new ApiError(begun.error);
  const credential = await getPasskey(begun.data.options);
  return { confirmation_token: begun.data.confirmation_token, credential };
}

/** The most characters of a passkey's name that the API takes (PasskeyRegistrationRequest.name). */
export const passkeyNameLimit = 64;

/** The user's passkeys, oldest first (GET /passkeys). */
export function passkeysQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/passkeys");
}

/**
 * Registers a passkey called `name`, confirmed by the password or, for a user without one (`password` undefined), by
 * another passkey (confirmWithPasskey), as the options of useMutation: the registration's options
 * (POST /passkeys/registration), the browser's prompt, and the new passkey (POST /passkeys). An error is an ApiError, a
 * NetworkError, the DOMException of a prompt, or a PasskeyOptionsError (lib/webauthn).
 */
export function registerPasskeyMutation(api: Api) {
  return {
    mutationKey: ["post", "/passkeys"] as const,
    mutationFn: async ({ name, password }: { name: string; password: string | undefined }): Promise<Passkey> => {
      const body = password === undefined ? { name, passkey: await confirmWithPasskey(api) } : { name, password };
      const begun = await api.fetch.POST("/passkeys/registration", { body });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (begun.data === undefined) throw new ApiError(begun.error);
      const credential = await createPasskey(begun.data.options);
      const { data, error } = await api.fetch.POST("/passkeys", {
        body: { registration_token: begun.data.registration_token, credential },
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Removes a passkey (DELETE /passkeys/{id}), as the options of useMutation. */
export function removePasskeyMutation(api: Api) {
  return {
    mutationKey: ["delete", "/passkeys/{id}"] as const,
    mutationFn: async (id: string): Promise<string> => {
      const { error } = await api.fetch.DELETE("/passkeys/{id}", { params: { path: { id } } });
      if (error !== undefined) throw new ApiError(error);
      return id;
    },
  };
}

/**
 * What registering or removing a passkey changes (web-client/README.md S1, ADR-0045): the list. Resolves once it is fetched
 * again.
 */
export async function refreshPasskeys(api: Api, queryClient: QueryClient): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: passkeysQueryOptions(api).queryKey });
}

/** The problem types of passkeys that the screen tells apart (api/openapi.yaml, Problem). */
const wrongPasswordType = "/api/v1/problems/wrong-password";
const invalidPasskeyType = "/api/v1/problems/invalid-passkey";
const expiredType = "/api/v1/problems/passkey-registration-expired";
const confirmationExpiredType = "/api/v1/problems/passkey-confirmation-expired";
const tooManyType = "/api/v1/problems/too-many-passkeys";

/** Why a registration failed, by what the user can do about it. */
export type PasskeyFailure =
  | "wrong-password"
  | "cancelled"
  | "exists"
  | "unavailable"
  | "invalid"
  | "expired"
  | "too-many"
  | "rate-limited"
  | "other";

/** Classifies an error of a registration (web-client/README.md E2). */
export function passkeyFailure(error: unknown): PasskeyFailure {
  if (error instanceof DOMException) return passkeyPromptFailure(error);
  if (!(error instanceof ApiError)) return "other";
  if (error.status === 429) return "rate-limited";
  switch (error.type) {
    case wrongPasswordType:
      return "wrong-password";
    case invalidPasskeyType:
      return "invalid";
    case expiredType:
    case confirmationExpiredType:
      return "expired";
    case tooManyType:
      return "too-many";
    default:
      return "other";
  }
}
