// SPDX-License-Identifier: AGPL-3.0-only

// Helpers of the tests of the data of the account settings: how a write is refused, what a screen shown again shows,
// and a confirmation the fake accepts.

import { ProblemError } from "@mycroblog/mykit";
import type { QueryKey } from "@tanstack/react-query";
import type { AppData } from "../app-data";
import type { Confirmation } from "../confirmation";

/** The problem a refused write rejects with; a write that resolves, or rejects otherwise, fails the test. */
export async function refusal(write: Promise<unknown>): Promise<ProblemError> {
  const error = await write.then(
    () => undefined,
    (e: unknown) => e,
  );
  if (!(error instanceof ProblemError)) throw new Error(`expected a ProblemError, got ${String(error)}`);
  return error;
}

/** The last part of a problem's type: `wrong-password` for `/api/v1/problems/wrong-password`. */
export const kindOf = (error: ProblemError) => error.problem.type.split("/").at(-1);

/** Whether a read of the cache is marked for a new read. */
export const marked = (data: AppData, key: QueryKey) => data.queryClient.getQueryState(key)?.isInvalidated;

/** Resolves once `reads` reads of the cache are on their way, none by default: every read answered. */
export async function settled(data: AppData, reads = 0): Promise<void> {
  // A turn first: a read asked for in this turn starts in the next.
  do await new Promise((turn) => setTimeout(turn, 0));
  while (data.queryClient.isFetching() !== reads);
}

/** A confirmation with a passkey, which the fake accepts once. */
export async function passkeyConfirmation(data: AppData): Promise<Extract<Confirmation, { kind: "passkey" }>> {
  const { data: began } = await data.api.POST("/passkeys/confirmation");
  if (began === undefined) throw new Error("the fake did not begin a confirmation");
  return {
    kind: "passkey",
    passkey: { confirmation_token: began.confirmation_token, credential: { id: "credential-1" } },
  };
}
