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

/** A passkey of the account. */
export type Passkey = components["schemas"]["Passkey"];

/** The account's passkeys, oldest first. */
export function passkeysQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/passkeys");
}

/**
 * Adds a passkey named `name`, confirmed by `confirmation`, and puts it in the cache. It rejects with the server's
 * problem (a wrong password, an expired confirmation, 20 passkeys already) or with the PasskeyError of the browser's
 * ceremony (`reason`: the reader closed the prompt, the authenticator holds a passkey of the account, no passkeys).
 */
export async function addPasskey(
  data: AppData,
  name: string,
  confirmation: Confirmation,
  { signal }: { signal?: AbortSignal | undefined } = {},
): Promise<Passkey> {
  const { data: begun } = await data.api.POST("/passkeys/registration", {
    body: { name, ...confirmationFields(confirmation) },
    ...(signal === undefined ? {} : { signal }),
  });
  if (begun === undefined) throw new Error("app-data: /passkeys/registration answered without options");
  const credential = await createPasskey(begun.options, { signal });
  const { data: made } = await data.api.POST("/passkeys", {
    body: { registration_token: begun.registration_token, credential },
    ...(signal === undefined ? {} : { signal }),
  });
  if (made === undefined) throw new Error("app-data: /passkeys answered without the passkey");
  data.queryClient.setQueryData<components["schemas"]["PasskeyList"]>(passkeysQuery(data).queryKey, (now) =>
    now === undefined ? now : { ...now, items: [...now.items, made] },
  );
  return made;
}

/**
 * Deletes the passkey of `id`, and takes it from the cache once the server did. It rejects with the server's problem; a
 * 409 of the type `/api/v1/problems/last-sign-in-method` says it is the last way to sign in.
 */
export async function deletePasskey(data: AppData, id: string): Promise<void> {
  await data.api.DELETE("/passkeys/{id}", { params: { path: { id } } });
  data.queryClient.setQueryData<components["schemas"]["PasskeyList"]>(passkeysQuery(data).queryKey, (now) =>
    now === undefined ? now : { ...now, items: now.items.filter((p) => p.id !== id) },
  );
}

/** The account's passkeys, oldest first; undefined until the server answered. */
export function usePasskeys(): readonly Passkey[] | undefined {
  return useQuery(passkeysQuery(useAppData())).data?.items;
}

export type PasskeyWrites = {
  readonly add: (
    name: string,
    confirmation: Confirmation,
    options?: { signal?: AbortSignal | undefined },
  ) => Promise<Passkey>;
  readonly remove: (id: string) => Promise<void>;
};

/** Adding and deleting passkeys, from a screen. */
export function usePasskeyWrites(): PasskeyWrites {
  const data = useAppData();
  return {
    add: (name, confirmation, options) => addPasskey(data, name, confirmation, options),
    remove: (id) => deletePasskey(data, id),
  };
}
