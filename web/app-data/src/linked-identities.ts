// SPDX-License-Identifier: AGPL-3.0-only
// The accounts at external login providers linked to the account (FR-010, ADR-0106 §3.5–§3.7): the links, the
// providers one may link an account at, unlinking, and the two operations that leave the app for a provider: linking an
// account, and confirming an operation by signing in again at a provider that holds a link. Both give the address the
// browser then loads (a full page load, `location.assign`), and the provider's page ends on the server, which sends
// the browser back to `return_to`, `/settings/linked-accounts` unless a screen names another, with the outcome in the
// fragment of the address: read it with `readExternalOutcome`.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { confirmationFields, type Confirmation, type ConfirmedOperation } from "./confirmation";
import { useAppData } from "./provider";
import { removeCached } from "./written";

/** An account at a login provider linked to the account. */
export type LinkedIdentity = components["schemas"]["LinkedIdentity"];

/** A login provider the reader may link an account at (`allow_link`) or confirm with. */
export type LinkableLoginProvider = components["schemas"]["LinkableLoginProvider"];

/** The links of the account, oldest first, and the login providers; both empty on a server without providers. */
export type LinkedIdentities = components["schemas"]["LinkedIdentityList"];

/** Where the browser goes to sign in at a provider. */
export type ExternalStart = { readonly url: string };

/** The links of the account and the login providers. */
export function linkedIdentitiesQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/linked_identities");
}

/**
 * Asks where the browser goes to link an account at the provider of `providerId`, after `confirmation` of the `link`
 * operation. It rejects with the server's problem: a provider that is unavailable or does not allow links (422), one the
 * account links already (409), a wrong confirmation.
 */
export async function beginLinkIdentity(
  data: AppData,
  providerId: string,
  confirmation: Confirmation,
  returnTo?: string,
): Promise<ExternalStart> {
  const { data: start } = await data.api.POST("/linked_identities", {
    body: {
      provider_id: providerId,
      ...confirmationFields(confirmation),
      ...(returnTo === undefined ? {} : { return_to: returnTo }),
    },
  });
  if (start === undefined) throw new Error("app-data: /linked_identities answered without an address");
  return { url: start.start_url };
}

/**
 * Asks where the browser goes to sign in again at the provider of `providerId`, to confirm `operation` in place of the
 * password or a passkey. It rejects with the server's problem (422: the account links nothing at the provider).
 */
export async function beginExternalConfirmation(
  data: AppData,
  providerId: string,
  operation: ConfirmedOperation,
  returnTo?: string,
): Promise<ExternalStart> {
  const { data: start } = await data.api.POST("/linked_identities/confirmation", {
    body: { provider_id: providerId, operation, ...(returnTo === undefined ? {} : { return_to: returnTo }) },
  });
  if (start === undefined) throw new Error("app-data: /linked_identities/confirmation answered without an address");
  return { url: start.start_url };
}

/**
 * Unlinks the account of the link of `id`, and takes it from the cache once the server did, the reads of the links on
 * their way cancelled first. A link the server no longer has (404) is unlinked already: it resolves all the same, and
 * the links are read again. It rejects with the server's problem; a 409 of the type
 * `/api/v1/problems/last-sign-in-method` says it is the last way to sign in.
 */
export async function unlinkIdentity(data: AppData, id: string): Promise<void> {
  await removeCached<LinkedIdentities>(
    data,
    linkedIdentitiesQuery(data).queryKey,
    data.api.DELETE("/linked_identities/{id}", { params: { path: { id } } }),
    (now) => ({ ...now, items: now.items.filter((l) => l.id !== id) }),
  );
}

/** Why a link or a confirmation at a provider did not happen (the `external_error` of the fragment). */
export type ExternalErrorReason =
  "rate_limited" | "unavailable" | "expired" | "denied" | "mismatch" | "linked_elsewhere" | "already_linked" | "failed";

const externalErrors: ReadonlySet<string> = new Set<ExternalErrorReason>([
  "rate_limited",
  "unavailable",
  "expired",
  "denied",
  "mismatch",
  "linked_elsewhere",
  "already_linked",
  "failed",
]);

const operations: ReadonlySet<string> = new Set<ConfirmedOperation>([
  "passkey_registration",
  "password_change",
  "password_removal",
  "account_deletion",
  "account_deactivation",
  "account_move",
  "link",
]);

/** How a link or a confirmation at a provider ended; `provider` is the provider's ID and `operation` the one confirmed. */
export type ExternalOutcome =
  | { readonly kind: "linked"; readonly provider: string }
  | {
      readonly kind: "confirmation";
      /** The token the operation takes once, as its `external_confirmation`, within 10 minutes. */
      readonly token: string;
      readonly provider: string | undefined;
      readonly operation: ConfirmedOperation | undefined;
    }
  | {
      readonly kind: "error";
      readonly reason: ExternalErrorReason;
      readonly provider: string | undefined;
      readonly operation: ConfirmedOperation | undefined;
    };

/**
 * What the server's return from a provider says, from the fragment of the address (`location.hash`, with or without the
 * `#`): `linked=<provider>`, `external_confirmation=<token>` or `external_error=<reason>`, next to `provider` and, for
 * a confirmation, `operation`. A reason this client does not know reads as `failed`. It is null for a fragment that holds
 * none of the three, which is every visit but a return from a provider.
 */
export function readExternalOutcome(fragment: string): ExternalOutcome | null {
  const params = new URLSearchParams(fragment.startsWith("#") ? fragment.slice(1) : fragment);
  const provider = params.get("provider") || undefined;
  const operation = ((value) => (value !== null && operations.has(value) ? (value as ConfirmedOperation) : undefined))(
    params.get("operation"),
  );
  const error = params.get("external_error");
  if (error !== null)
    return {
      kind: "error",
      reason: externalErrors.has(error) ? (error as ExternalErrorReason) : "failed",
      provider,
      operation,
    };
  const token = params.get("external_confirmation");
  if (token !== null && token !== "") return { kind: "confirmation", token, provider, operation };
  const linked = params.get("linked");
  if (linked !== null && linked !== "") return { kind: "linked", provider: linked };
  return null;
}

/** The links of the account and the login providers; undefined until the server answered. */
export function useLinkedIdentities(): LinkedIdentities | undefined {
  return useQuery(linkedIdentitiesQuery(useAppData())).data;
}

export type LinkedIdentityWrites = {
  readonly beginLink: (providerId: string, confirmation: Confirmation, returnTo?: string) => Promise<ExternalStart>;
  readonly beginConfirmation: (
    providerId: string,
    operation: ConfirmedOperation,
    returnTo?: string,
  ) => Promise<ExternalStart>;
  readonly unlink: (id: string) => Promise<void>;
};

/** Linking, confirming at a provider and unlinking, from a screen. */
export function useLinkedIdentityWrites(): LinkedIdentityWrites {
  const data = useAppData();
  return {
    beginLink: (providerId, confirmation, returnTo) => beginLinkIdentity(data, providerId, confirmation, returnTo),
    beginConfirmation: (providerId, operation, returnTo) =>
      beginExternalConfirmation(data, providerId, operation, returnTo),
    unlink: (id) => unlinkIdentity(data, id),
  };
}
