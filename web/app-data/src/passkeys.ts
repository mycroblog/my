// SPDX-License-Identifier: AGPL-3.0-only
// The account's passkeys (FR-004, ADR-0106 §26): the list, adding one and deleting one. Adding runs in three steps: the
// reader confirms and names it, the server gives the options of a registration, the browser makes the credential
// (@my/platform), and the server stores it under the name; nothing is stored before the last. Whether the browser has
// passkeys at all is `supportsPasskeys` of @my/platform.
import type { components } from "@mycroblog/mykit";
import { createPasskey } from "@my/platform";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { confirmationFields, type Confirmation } from "./confirmation";
import { useAppData } from "./provider";
import { removeCached, writeCached } from "./written";

/** A passkey of the account. */
export type Passkey = components["schemas"]["Passkey"];

type PasskeyList = components["schemas"]["PasskeyList"];

/** The account's passkeys, oldest first. */
export function passkeysQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/passkeys");
}

/**
 * Adds a passkey named `name`, confirmed by `confirmation`, and puts it in the cache, the reads of the list on their
 * way cancelled first. It rejects with the server's problem (a wrong password, an expired confirmation, 20 passkeys
 * already) or with the PasskeyError of the browser's ceremony (`reason`: the reader closed the prompt, the
 * authenticator holds a passkey of the account, no passkeys).
 */
export async function addPasskey(data: AppData, name: string, confirmation: Confirmation): Promise<Passkey> {
  const { data: begun } = await data.api.POST("/passkeys/registration", {
    body: { name, ...confirmationFields(confirmation) },
  });
  if (begun === undefined) throw new Error("app-data: /passkeys/registration answered without options");
  const credential = await createPasskey(begun.options);
  const { data: made } = await data.api.POST("/passkeys", {
    body: { registration_token: begun.registration_token, credential },
  });
  if (made === undefined) throw new Error("app-data: /passkeys answered without the passkey");
  await writeCached<PasskeyList>(data, passkeysQuery(data).queryKey, (now) => ({
    ...now,
    items: [...now.items, made],
  }));
  return made;
}

/**
 * Deletes the passkey of `id`, and takes it from the cache once the server did, the reads of the list on their way
 * cancelled first. A passkey the server no longer has (404) is deleted already: it resolves all the same, and the list
 * is read again. It rejects with the server's problem; a 409 of the type `/api/v1/problems/last-sign-in-method` says
 * it is the last way to sign in.
 */
export async function deletePasskey(data: AppData, id: string): Promise<void> {
  await removeCached<PasskeyList>(
    data,
    passkeysQuery(data).queryKey,
    data.api.DELETE("/passkeys/{id}", { params: { path: { id } } }),
    (now) => ({ ...now, items: now.items.filter((p) => p.id !== id) }),
  );
}

/** The account's passkeys, oldest first; undefined until the server answered. */
export function usePasskeys(): readonly Passkey[] | undefined {
  return useQuery(passkeysQuery(useAppData())).data?.items;
}

export type PasskeyWrites = {
  readonly add: (name: string, confirmation: Confirmation) => Promise<Passkey>;
  readonly remove: (id: string) => Promise<void>;
};

/** Adding and deleting passkeys, from a screen. */
export function usePasskeyWrites(): PasskeyWrites {
  const data = useAppData();
  return {
    add: (name, confirmation) => addPasskey(data, name, confirmation),
    remove: (id) => deletePasskey(data, id),
  };
}
