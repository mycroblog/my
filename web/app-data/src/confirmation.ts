// SPDX-License-Identifier: AGPL-3.0-only
// Confirming an operation on how the account signs in (ADR-0106 §3.7): changing or removing the password, adding a
// passkey, linking an account. The server takes the current password, an assertion of a passkey of the account, or the
// confirmation of a linked login provider. The passkey is asked of the browser here: the server's options for it, the
// browser's prompt, and the assertion the operation then takes once, within 5 minutes. A provider's confirmation is
// made on the provider's page and comes back in the fragment of the address (linked-identities.ts).
import type { components } from "@mycroblog/mykit";
import { getPasskey } from "@my/platform";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** What the server takes as the passkey of an operation: the token of the options and the browser's answer. */
export type PasskeyAssertion = components["schemas"]["PasskeyAssertion"];

/** The operations an external login provider's confirmation is for. */
export type ConfirmedOperation = components["schemas"]["ConfirmedOperation"];

/** How the reader confirms an operation. */
export type Confirmation =
  | { readonly kind: "password"; readonly password: string }
  | { readonly kind: "passkey"; readonly passkey: PasskeyAssertion }
  | { readonly kind: "external"; readonly token: string };

/** The members of a request that carry `confirmation`; the password is named `passwordField` in the request. */
export function confirmationFields(
  confirmation: Confirmation,
  passwordField: "password" | "current_password" = "password",
):
  { password?: string; current_password?: string } | { passkey: PasskeyAssertion } | { external_confirmation: string } {
  switch (confirmation.kind) {
    case "password":
      return passwordField === "password"
        ? { password: confirmation.password }
        : { current_password: confirmation.password };
    case "passkey":
      return { passkey: confirmation.passkey };
    case "external":
      return { external_confirmation: confirmation.token };
  }
}

/**
 * Asks the server for the options of a confirmation with a passkey, has the browser answer them, and returns the
 * confirmation for one operation. It rejects with the server's problem, or with the PasskeyError of the browser's
 * ceremony (`reason` says why: the reader closed the prompt, the browser has no passkeys).
 */
export async function confirmWithPasskey(
  data: AppData,
  { signal }: { signal?: AbortSignal | undefined } = {},
): Promise<Confirmation> {
  const { data: begun } = await data.api.POST("/passkeys/confirmation", signal === undefined ? {} : { signal });
  if (begun === undefined) throw new Error("app-data: /passkeys/confirmation answered without options");
  const credential = await getPasskey(begun.options, { signal });
  return { kind: "passkey", passkey: { confirmation_token: begun.confirmation_token, credential } };
}

/** Confirms with a passkey, from a screen (confirmWithPasskey). */
export function useConfirmWithPasskey(): (options?: { signal?: AbortSignal | undefined }) => Promise<Confirmation> {
  const data = useAppData();
  return (options) => confirmWithPasskey(data, options);
}
