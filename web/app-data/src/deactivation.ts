// SPDX-License-Identifier: AGPL-3.0-only
// Deactivating and deleting the viewer's own account (ADR-0089): what deactivating would do now and whether it
// is refused, deactivating (which ends the sessions and tokens, and deletes the account after a time unless the viewer
// signs in again), and deleting it at once. Both need the viewer's confirmation. Once either answers, the session is over:
// leaving the screens to the sign-in page is the caller's, since nothing the cache holds is the viewer's any more.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQueries, useQuery } from "@tanstack/react-query";
import type { Reauthentication } from "./account-move";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { readOf, type ScreenRead } from "./reads";
import { confirmationQueries, loadConfirmation } from "./security";

/** What deactivating the account would do now: when it would be deleted, or why it is refused. */
export type DeactivationPreview = components["schemas"]["DeactivationPreview"];

/** Why the account cannot be deactivated now. */
export type DeactivationRefusal = NonNullable<DeactivationPreview["refusal"]>;

/** The deactivation done: when the account is deleted unless the viewer reactivates it first. */
export type Deactivation = components["schemas"]["Deactivation"];

/** What deactivating the viewer's account would do now. */
export function deactivationPreviewQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/accounts/deactivation");
}

/** What deactivating would do, undefined until the server answered. */
export function useDeactivationPreview(): DeactivationPreview | undefined {
  return useQuery(deactivationPreviewQuery(useAppData())).data;
}

/** How many days after its deletion what the server still holds of an account is erased. */
export const deletionEraseDays = 7;

/**
 * Reads what the screen that deactivates the account shows, from the cache where it holds it: what deactivating
 * would do now, and what the account confirms with. A read that fails is the screen's to say.
 */
export async function loadDeactivation(data: AppData): Promise<void> {
  await Promise.all([
    data.queryClient.query({ ...deactivationPreviewQuery(data), staleTime: Infinity }).catch(() => undefined),
    loadConfirmation(data),
  ]);
}

/**
 * Where the reads of the screen that deactivates the account are, and what deactivating would do now once read:
 * `read` once the server answered it and what the account confirms with, `failed` when it did not answer one, with
 * `retry`, which reads again those that failed, and `loading` until then.
 */
export function useDeactivationRead(): ScreenRead & { preview: DeactivationPreview | undefined } {
  const data = useAppData();
  const preview = useQuery(deactivationPreviewQuery(data));
  const ways = useQueries({ queries: [...confirmationQueries(data)] });
  return { ...readOf([preview, ...ways]), preview: preview.data };
}

/** A confirmation of a deactivation: the viewer's proof, and a code of their authenticator app when they have one. */
export type DeactivationProof = Reauthentication & { readonly two_factor_code?: string };

/**
 * Whether the server refused a deactivation because the account cannot be deactivated now (a 409 of the type
 * `/api/v1/problems/deactivation-refused`), which what deactivating would do then says why (`refusal`).
 */
export function deactivationRefused(error: unknown): boolean {
  return error instanceof ProblemError && error.type === "/api/v1/problems/deactivation-refused";
}

/**
 * Deactivates the viewer's account, and resolves with when it would be deleted. It rejects with the server's problem;
 * for an account that cannot be deactivated now (`deactivationRefused`), once what deactivating would do is read
 * again, which says why.
 */
export async function deactivateOwnAccount(data: AppData, proof: DeactivationProof): Promise<Deactivation> {
  try {
    const { data: done } = await data.api.POST("/accounts/deactivation", { body: proof });
    if (done === undefined) throw new Error("app-data: the deactivation came without an answer");
    return done;
  } catch (error) {
    if (deactivationRefused(error))
      await data.queryClient.invalidateQueries({ queryKey: deactivationPreviewQuery(data).queryKey });
    throw error;
  }
}

/** Deletes the viewer's account at once. It rejects with the server's problem (a wrong confirmation). */
export async function deleteOwnAccount(data: AppData, proof: Reauthentication): Promise<void> {
  await data.api.POST("/accounts/delete", { body: proof });
}

/** Deactivates and deletes the account, from a screen (`deactivateOwnAccount`, `deleteOwnAccount`). */
export function useAccountEnd(): {
  readonly deactivate: (proof: DeactivationProof) => Promise<Deactivation>;
  readonly remove: (proof: Reauthentication) => Promise<void>;
} {
  const data = useAppData();
  return { deactivate: (proof) => deactivateOwnAccount(data, proof), remove: (proof) => deleteOwnAccount(data, proof) };
}
