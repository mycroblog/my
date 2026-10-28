// SPDX-License-Identifier: AGPL-3.0-only
// The user's password (FR-002, FR-004): whether the user has one, changing it with the current one, setting one with a
// passkey when the user has none, and removing it with a passkey. A change or a removal ends every other session and
// grant and keeps this one. Resetting a forgotten password is the server's own sign-in pages (web-client/08-server-rendered-pages.md §8).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { passkeyPromptFailure } from "../../../lib/webauthn";
import { confirmWithPasskey } from "./passkeys";
import { type Confirmation, confirmationBody } from "./linkedIdentities";

/** The lengths of a password that the server takes, in characters (identity.MinPasswordLength / MaxPasswordLength). */
export const passwordMinLength = 8;
export const passwordMaxLength = 128;

/** Whether the user has a password (GET /accounts/password). */
export function passwordStatusQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/accounts/password");
}

/**
 * Changes the password after the current one confirms it, or sets one after a passkey or a linked provider confirms
 * it (POST /accounts/password), as the options of useMutation.
 */
export function changePasswordMutation(api: Api) {
  return {
    mutationKey: ["post", "/accounts/password"] as const,
    mutationFn: async ({
      confirmation,
      newPassword,
    }: {
      confirmation: Confirmation;
      newPassword: string;
    }): Promise<void> => {
      const body =
        confirmation.kind === "password"
          ? { current_password: confirmation.password, new_password: newPassword }
          : { ...(await confirmationBody(api, confirmation)), new_password: newPassword };
      await api.fetch.POST("/accounts/password", { body });
    },
  };
}

/**
 * Removes the password after a passkey or a linked provider confirms it (POST /accounts/password/remove), as the
 * options of useMutation.
 */
export function removePasswordMutation(api: Api) {
  return {
    mutationKey: ["post", "/accounts/password/remove"] as const,
    mutationFn: async (confirmation: Exclude<Confirmation, { kind: "password" }>): Promise<void> => {
      const body =
        confirmation.kind === "external"
          ? { external_confirmation: confirmation.token }
          : { passkey: await confirmWithPasskey(api) };
      await api.fetch.POST("/accounts/password/remove", { body });
    },
  };
}

/**
 * What setting or removing the password changes (web-client/README.md S1, ADR-0045): whether the user has one. Resolves once
 * it is fetched again.
 */
export async function refreshPasswordStatus(api: Api, queryClient: QueryClient): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: passwordStatusQueryOptions(api).queryKey });
}

/** The problem types that the screen tells apart (api/openapi.yaml, Problem). */
const wrongPasswordType = "/api/v1/problems/wrong-password";
const invalidPasskeyType = "/api/v1/problems/invalid-passkey";
const confirmationExpiredType = "/api/v1/problems/passkey-confirmation-expired";

/** Why a change, a removal, or another confirmed operation failed, by what the user can do about it. */
export type ConfirmationFailure =
  "wrong-password" | "cancelled" | "unavailable" | "invalid-passkey" | "expired" | "rate-limited" | "other";

/** Classifies an error of an operation confirmed with the password or a passkey (web-client/README.md E2). */
export function confirmationFailure(error: unknown): ConfirmationFailure {
  if (error instanceof DOMException) return passkeyPromptFailure(error) === "cancelled" ? "cancelled" : "unavailable";
  if (!(error instanceof ApiError)) return "other";
  if (error.status === 429) return "rate-limited";
  switch (error.type) {
    case wrongPasswordType:
      return "wrong-password";
    case invalidPasskeyType:
      return "invalid-passkey";
    case confirmationExpiredType:
      return "expired";
    default:
      return "other";
  }
}
