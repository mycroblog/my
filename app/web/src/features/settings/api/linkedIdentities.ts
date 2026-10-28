// SPDX-License-Identifier: AGPL-3.0-only
// The user's accounts at the login providers that the administrators set up (FR-010): the list with the providers,
// unlinking one, starting a link after a confirmation, and starting a confirmation with a linked provider in place of
// the password. A link and a confirmation leave the app for the server's start page, which sends the browser to the
// provider; the provider sends it back to the page that started it with the outcome in the fragment
// (parseExternalOutcome).
import type { QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { confirmWithPasskey, type PasskeyAssertion } from "./passkeys";

export type LinkedIdentity = components["schemas"]["LinkedIdentity"];
export type LinkableLoginProvider = components["schemas"]["LinkableLoginProvider"];
/** An operation that the user confirms. */
export type ConfirmedOperation = components["schemas"]["ConfirmedOperation"];

/** The user's linked accounts and the login providers (GET /linked_identities). */
export function linkedIdentitiesQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/linked_identities");
}

/**
 * What confirms an operation: the current password, a passkey of the user (the browser's prompt), or the token of a
 * confirmation with a linked provider.
 */
export type Confirmation =
  { kind: "password"; password: string } | { kind: "passkey" } | { kind: "external"; token: string };

/** The members of an operation's request that carry `confirmation`; a passkey opens the browser's prompt first. */
export async function confirmationBody(
  api: Api,
  confirmation: Confirmation,
): Promise<{ password: string } | { passkey: PasskeyAssertion } | { external_confirmation: string }> {
  switch (confirmation.kind) {
    case "password":
      return { password: confirmation.password };
    case "passkey":
      return { passkey: await confirmWithPasskey(api) };
    case "external":
      return { external_confirmation: confirmation.token };
  }
}

/** Unlinks an account (DELETE /linked_identities/{id}), as the options of useMutation. */
export function unlinkIdentityMutation(api: Api) {
  return {
    mutationKey: ["delete", "/linked_identities/{id}"] as const,
    mutationFn: async (id: string): Promise<string> => {
      const { error } = await api.fetch.DELETE("/linked_identities/{id}", { params: { path: { id } } });
      if (error !== undefined) throw new ApiError(error);
      return id;
    },
  };
}

/**
 * Starts linking an account at `providerId` once `confirmation` confirms it (POST /linked_identities), as the options
 * of useMutation; it resolves to the URL that the browser opens next. The provider sends the browser back to `returnTo`.
 */
export function beginLinkMutation(api: Api) {
  return {
    mutationKey: ["post", "/linked_identities"] as const,
    mutationFn: async ({
      providerId,
      confirmation,
      returnTo,
    }: {
      providerId: string;
      confirmation: Confirmation;
      returnTo: string;
    }): Promise<URL> => {
      const body = { provider_id: providerId, return_to: returnTo, ...(await confirmationBody(api, confirmation)) };
      const { data, error } = await api.fetch.POST("/linked_identities", { body });
      if (data === undefined) throw new ApiError(error);
      return new URL(data.start_url);
    },
  };
}

/**
 * Starts a confirmation of `operation` with the linked provider `providerId` (POST /linked_identities/confirmation),
 * as the options of useMutation; it resolves to the URL that the browser opens next. The provider sends the browser
 * back to `returnTo`.
 */
export function beginExternalConfirmationMutation(api: Api) {
  return {
    mutationKey: ["post", "/linked_identities/confirmation"] as const,
    mutationFn: async ({
      providerId,
      operation,
      returnTo,
    }: {
      providerId: string;
      operation: ConfirmedOperation;
      returnTo: string;
    }): Promise<URL> => {
      const { data, error } = await api.fetch.POST("/linked_identities/confirmation", {
        body: { provider_id: providerId, operation, return_to: returnTo },
      });
      if (data === undefined) throw new ApiError(error);
      return new URL(data.start_url);
    },
  };
}

/**
 * What linking or unlinking changes (web-client/README.md S1, ADR-0045): the list. Resolves once it is fetched again.
 */
export async function refreshLinkedIdentities(api: Api, queryClient: QueryClient): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: linkedIdentitiesQueryOptions(api).queryKey });
}

/** Why a link or a confirmation with a provider failed, as the fragment's `external_error` names it. */
export type ExternalFailure =
  "rate_limited" | "unavailable" | "expired" | "denied" | "mismatch" | "linked_elsewhere" | "already_linked" | "failed";

const externalFailures: readonly ExternalFailure[] = [
  "rate_limited",
  "unavailable",
  "expired",
  "denied",
  "mismatch",
  "linked_elsewhere",
  "already_linked",
  "failed",
];

const confirmedOperations: readonly ConfirmedOperation[] = [
  "passkey_registration",
  "password_change",
  "password_removal",
  "account_deletion",
  "account_move",
  "link",
];

/**
 * What the provider's round trip ended with, as the server wrote it in the fragment of the page it went back to. A
 * failure of a confirmation names its operation; that of a link names none.
 */
export type ExternalOutcome =
  | { kind: "confirmed"; token: string; operation: ConfirmedOperation; providerId: string }
  | { kind: "linked"; providerId: string }
  | { kind: "failed"; failure: ExternalFailure; operation: ConfirmedOperation | undefined };

/** Reads the outcome of a link or a confirmation from a location's fragment (without the `#`); undefined for none. */
export function parseExternalOutcome(hash: string): ExternalOutcome | undefined {
  const values = new URLSearchParams(hash);
  const token = values.get("external_confirmation");
  const operation = confirmedOperations.find((op) => op === values.get("operation"));
  const provider = values.get("provider");
  if (token !== null && token !== "" && operation !== undefined && provider !== null) {
    return { kind: "confirmed", token, operation, providerId: provider };
  }
  const linked = values.get("linked");
  if (linked !== null && linked !== "") return { kind: "linked", providerId: linked };
  const failed = values.get("external_error");
  if (failed === null) return undefined;
  return { kind: "failed", failure: externalFailures.find((f) => f === failed) ?? "failed", operation };
}

/** The problem types of links that the screen tells apart (api/openapi.yaml, Problem). */
const lastSignInMethodType = "/api/v1/problems/last-sign-in-method";
const externalExpiredType = "/api/v1/problems/external-confirmation-expired";

/** Whether an unlink failed because the account is the user's last way to sign in. */
export function isLastSignInMethod(error: unknown): boolean {
  return error instanceof ApiError && error.type === lastSignInMethodType;
}

/** Whether an operation failed because its confirmation with a provider expired or was used. */
export function isExternalConfirmationExpired(error: unknown): boolean {
  return error instanceof ApiError && error.type === externalExpiredType;
}
