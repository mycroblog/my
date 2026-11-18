// SPDX-License-Identifier: AGPL-3.0-only
// The account's password (FR-002, ADR-0106): whether it has one, changing or setting it, and removing it where a
// passkey or a linked account remains to sign in with. Both writes end every other browser and app of the account,
// so those lists are read again. A change is confirmed with the current password or a passkey; a setting by an account
// without a password, and a removal, with a passkey or a linked provider's confirmation (confirmation.ts).
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { authorizedAppsQuery } from "./authorized-apps";
import { browserSessionsQuery } from "./browser-sessions";
import { confirmationFields, type Confirmation } from "./confirmation";
import { useAppData } from "./provider";

/** Whether the account signs in with a password. */
export type PasswordStatus = components["schemas"]["PasswordStatus"];

/** Whether the account has a password. */
export function passwordStatusQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/accounts/password");
}

/** What a write of the password changed: the status is as the server left it, and the lists it ended are read again. */
async function passwordWritten(data: AppData, hasPassword: boolean): Promise<void> {
  const { queryClient } = data;
  queryClient.setQueryData<PasswordStatus>(passwordStatusQuery(data).queryKey, { has_password: hasPassword });
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: browserSessionsQuery(data).queryKey }),
    queryClient.invalidateQueries({ queryKey: authorizedAppsQuery(data).queryKey }),
  ]);
}

/**
 * Changes the password to `newPassword` (8 to 128 characters), or sets it for an account without one, confirmed by the
 * current password or a passkey or a provider's confirmation. It rejects with the server's problem: a wrong password
 * or passkey, an expired confirmation, a new password of the wrong length (`field` names `new_password`).
 */
export async function changePassword(data: AppData, newPassword: string, confirmation: Confirmation): Promise<void> {
  await data.api.POST("/accounts/password", {
    body: { new_password: newPassword, ...confirmationFields(confirmation, "current_password") },
  });
  await passwordWritten(data, true);
}

/** What takes the place of the password when it is removed: a passkey, or a linked provider's confirmation. */
export type PasswordRemoval = Exclude<Confirmation, { readonly kind: "password" }>;

/**
 * Removes the password, confirmed by a passkey or a provider's confirmation. It rejects with the server's problem; a
 * 409 of the type `/api/v1/problems/last-sign-in-method` says no passkey and no linked account remains.
 */
export async function removePassword(data: AppData, confirmation: PasswordRemoval): Promise<void> {
  await data.api.POST("/accounts/password/remove", { body: confirmationFields(confirmation) });
  await passwordWritten(data, false);
}

/** Whether the account has a password; undefined until the server answered. */
export function useHasPassword(): boolean | undefined {
  return useQuery(passwordStatusQuery(useAppData())).data?.has_password;
}

export type PasswordWrites = {
  readonly change: (newPassword: string, confirmation: Confirmation) => Promise<void>;
  readonly remove: (confirmation: PasswordRemoval) => Promise<void>;
};

/** Changing, setting and removing the password, from a screen. */
export function usePasswordWrites(): PasswordWrites {
  const data = useAppData();
  return {
    change: (newPassword, confirmation) => changePassword(data, newPassword, confirmation),
    remove: (confirmation) => removePassword(data, confirmation),
  };
}
