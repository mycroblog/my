// SPDX-License-Identifier: AGPL-3.0-only
// Moving the user's own account to another account, and cancelling the move (FR-009, ADR-0089). The account that the
// user moves to is found with the lookup that resolves an account of another server; the move is confirmed with the
// password, or a passkey for a user without one. The moved state is the viewer's `moved` (the viewer query).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "../../../lib/api/schema.gen";
import { ApiError } from "../../../lib/errors";
import { confirmWithPasskey } from "./passkeys";

export type MoveTarget = components["schemas"]["Account"];

/** The longest handle or address the lookup takes (lookupAccount `acct`). */
export const targetInputLimit = 2048;

/** Finds the account that `acct` names, fetching it from its server when this one does not know it (GET /accounts/lookup). */
export async function lookupMoveTarget(api: Api, acct: string): Promise<MoveTarget> {
  const { data, error } = await api.fetch.GET("/accounts/lookup", { params: { query: { acct, resolve: true } } });
  // The middleware throws an ApiError for every error status; this narrows the type.
  if (data === undefined) throw new ApiError(error);
  return data;
}

/**
 * Moves the account to `targetId` after checking the password, or a passkey when `password` is undefined (a user
 * without a password), (POST /accounts/move), as the options of useMutation.
 */
export function moveAccountMutation(api: Api) {
  return {
    mutationKey: ["post", "/accounts/move"] as const,
    mutationFn: async ({ targetId, password }: { targetId: string; password: string | undefined }): Promise<void> => {
      const body =
        password === undefined
          ? { target_account_id: targetId, passkey: await confirmWithPasskey(api) }
          : { target_account_id: targetId, password };
      await api.fetch.POST("/accounts/move", { body });
    },
  };
}

/** Cancels the move (POST /accounts/move/cancel), as the options of useMutation. */
export function cancelMoveMutation(api: Api) {
  return {
    mutationKey: ["post", "/accounts/move/cancel"] as const,
    mutationFn: async (): Promise<void> => {
      await api.fetch.POST("/accounts/move/cancel");
    },
  };
}

/**
 * What a move or its cancellation changes: the viewer's account (its `moved`), and the profiles, which name the
 * target. Resolves once the viewer is fetched again.
 */
export async function refreshAfterMove(queryClient: QueryClient): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: ["get", "/accounts/verify_credentials"] });
  await queryClient.invalidateQueries({ queryKey: ["get", "/accounts/lookup"] });
}
