// SPDX-License-Identifier: AGPL-3.0-only
// Moving the user's own account to another account, and cancelling the move (FR-009, ADR-0089). The user names the new
// account with a form, which looks it up (fetching it from its server when this one does not know it) and opens the
// confirmation dialog; the dialog asks for the password, or a passkey of the browser for a user without one. A wrong
// password is the dialog's error like any other failure, so the dialog stays open and can be tried again (web-client/README.md E2).
// Whether the account has moved, and to whom, is the viewer's `moved`, which the viewer query refreshes.
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { handleOf } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { isolate } from "../../profile";
import { viewerQueryOptions } from "../../session";
import { confirmationFailure } from "../api/password";
import {
  cancelMoveMutation,
  lookupMoveTarget,
  type MoveTarget,
  moveAccountMutation,
  refreshAfterMove,
} from "../api/move";
import { confirmationErrorMessage } from "./passkeyConfirmation";
import { type ExternalConfirmation, useExternalConfirmation } from "./useExternalConfirmation";
import { useHasPassword } from "./usePasswordChange";

/** The account that the viewer moved to, or undefined while the account has not moved. */
export function useMovedTo(): MoveTarget | undefined {
  const api = useApi();
  return useSuspenseQuery(viewerQueryOptions(api)).data.moved;
}

/** The address of the viewer's own account, which the new account lists as an alias. */
export function useOwnAddress(): string {
  const api = useApi();
  return useSuspenseQuery(viewerQueryOptions(api)).data.url;
}

/** What the form that names the new account, and the dialog that moves the account, need. */
export type AccountMove = {
  /** What the user typed in the field of the new account. */
  input: string;
  setInput: (input: string) => void;
  /** Looks the new account up and opens the dialog once the browser has checked the field. */
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isLookingUp: boolean;
  /** Why the lookup failed, localized, for an alert next to the form; undefined when it did not. */
  lookupError: string | undefined;
  /** The account the dialog asks to move to; undefined while the dialog has not opened. */
  target: MoveTarget | undefined;
  /** The new account's handle, isolated from the sentence around it (Y10); empty while there is no target. */
  targetHandle: string;
  /** Whether the user has a password, which confirms the move; without one, a passkey confirms it. */
  hasPassword: boolean;
  /** The confirmation with a linked provider, for a user with neither a password nor a passkey. */
  external: ExternalConfirmation;
  isOpen: boolean;
  cancel: () => void;
  password: string;
  changePassword: (password: string) => void;
  /** Moves the account; on success the dialog closes and the viewer's account is fetched again. */
  confirm: () => void;
  isPending: boolean;
  /** Why the move failed, localized, for the dialog (which stays open); undefined when it did not. */
  error: string | undefined;
};

/** Moving the account; `onStatus` gets what a success did, for the page's StatusMessage. */
export function useAccountMove(onStatus: (status: string | undefined) => void): AccountMove {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const hasPassword = useHasPassword();
  const external = useExternalConfirmation("account_move", hasPassword);
  const [input, setInput] = useState("");
  const [target, setTarget] = useState<MoveTarget | undefined>(undefined);
  const [isOpen, setIsOpen] = useState(false);
  const [password, setPassword] = useState("");
  const targetHandle = target === undefined ? "" : isolate(handleOf(target.acct));
  const move = useMutation({
    ...moveAccountMutation(api),
    onSuccess: async (_, { confirmation }) => {
      if (confirmation.kind === "external") external.markUsed();
      await refreshAfterMove(queryClient);
      setIsOpen(false);
      setPassword("");
      setInput("");
      onStatus(t("client:account_move.move.done", { handle: targetHandle }));
    },
  });
  const lookup = useMutation({
    mutationKey: ["get", "/accounts/lookup", "move"] as const,
    mutationFn: (acct: string) => lookupMoveTarget(api, acct),
    onSuccess: (found) => {
      move.reset();
      setPassword("");
      setTarget(found);
      setIsOpen(true);
    },
  });
  const failure = move.isError ? confirmationFailure(move.error) : undefined;
  return {
    input,
    setInput: (value) => {
      setInput(value);
      if (lookup.isError) lookup.reset();
    },
    submit: (event) => {
      event.preventDefault();
      const value = input.trim();
      if (lookup.isPending || value === "") return;
      lookup.reset();
      onStatus(undefined);
      lookup.mutate(value);
    },
    isLookingUp: lookup.isPending,
    lookupError: lookup.isError
      ? t("client:account_move.move.lookup_failed", { reason: errorMessage(lookup.error) })
      : undefined,
    target,
    targetHandle,
    hasPassword,
    external,
    isOpen,
    cancel: () => {
      setIsOpen(false);
      move.reset();
    },
    password,
    changePassword: setPassword,
    confirm: () => {
      if (move.isPending || target === undefined) return;
      if (hasPassword) {
        if (password !== "") move.mutate({ targetId: target.id, confirmation: { kind: "password", password } });
      } else if (!external.isPreferred) {
        move.mutate({ targetId: target.id, confirmation: { kind: "passkey" } });
      } else if (external.confirmed !== undefined) {
        move.mutate({ targetId: target.id, confirmation: { kind: "external", token: external.confirmed.token } });
      }
    },
    isPending: move.isPending,
    error:
      failure === undefined
        ? undefined
        : failure === "wrong-password"
          ? t("client:account_move.move.wrong_password")
          : confirmationErrorMessage(t, failure, move.error, (reason) =>
              t("client:account_move.move.failed", { reason }),
            ),
  };
}

/** What the dialog that cancels the move needs. */
export type MoveCancellation = {
  isOpen: boolean;
  open: () => void;
  cancel: () => void;
  /** Cancels the move; on success the dialog closes and the viewer's account is fetched again. */
  confirm: () => void;
  isPending: boolean;
  /** Why cancelling failed, localized, for the dialog (which stays open); undefined when it did not. */
  error: string | undefined;
};

/** Cancelling the move; `onStatus` gets what a success did, for the page's StatusMessage. */
export function useMoveCancellation(onStatus: (status: string | undefined) => void): MoveCancellation {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [isOpen, setIsOpen] = useState(false);
  const cancel = useMutation({
    ...cancelMoveMutation(api),
    onSuccess: async () => {
      await refreshAfterMove(queryClient);
      setIsOpen(false);
      onStatus(t("client:account_move.cancel.done"));
    },
    onError: async (error) => {
      // A move that is no longer there (409): the page should show the account as it is.
      if (error instanceof ApiError && error.status === 409) await refreshAfterMove(queryClient);
    },
  });
  return {
    isOpen,
    open: () => {
      cancel.reset();
      onStatus(undefined);
      setIsOpen(true);
    },
    cancel: () => {
      setIsOpen(false);
      cancel.reset();
    },
    confirm: () => {
      if (!cancel.isPending) cancel.mutate();
    },
    isPending: cancel.isPending,
    error: cancel.isError ? t("client:account_move.cancel.failed", { reason: errorMessage(cancel.error) }) : undefined,
  };
}
