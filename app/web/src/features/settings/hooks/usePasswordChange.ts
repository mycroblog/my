// SPDX-License-Identifier: AGPL-3.0-only
// The user's password (FR-002, FR-004): the form that changes it with the current one, or sets one with a passkey for a
// user who removed theirs, and removing it with a passkey. A wrong current password is that field's error, a new
// password of the wrong length the new field's (the browser's constraint validation catches it first), a failed
// confirmation with a passkey and any other failure an alert next to the form (web-client/README.md E2, web-client/16-look-states-and-wording.md G16).
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { supportsPasskeys } from "../../../lib/webauthn";
import type { Confirmation } from "../api/linkedIdentities";
import { passkeysQueryOptions } from "../api/passkeys";
import {
  changePasswordMutation,
  confirmationFailure,
  passwordStatusQueryOptions,
  refreshPasswordStatus,
  removePasswordMutation,
} from "../api/password";
import { confirmationErrorMessage } from "./passkeyConfirmation";
import { type ExternalConfirmation, useExternalConfirmation } from "./useExternalConfirmation";

/** Whether the user has a password, which the route has loaded. */
export function useHasPassword(): boolean {
  const api = useApi();
  return useSuspenseQuery(passwordStatusQueryOptions(api)).data.has_password;
}

/** What the form that changes or sets the password needs. */
export type PasswordChangeForm = {
  /**
   * Whether the user has a password: the form changes it after the current one. Without one, the form sets one after a
   * passkey confirms it.
   */
  hasPassword: boolean;
  /** Whether the browser can use passkeys, which confirm setting a password. */
  isPasskeySupported: boolean;
  /** The confirmation with a linked provider, which sets a password for a user without passkeys. */
  external: ExternalConfirmation;
  currentPassword: string;
  changeCurrentPassword: (password: string) => void;
  newPassword: string;
  changeNewPassword: (password: string) => void;
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** The current password field's error when the server refused it, localized; undefined otherwise. */
  currentPasswordError: string | undefined;
  /** Why the change failed otherwise, localized, for an alert next to the form; undefined when it did not. */
  error: string | undefined;
};

/**
 * The form that changes the password, or sets one with a passkey. A success empties the form and passes what it did to
 * `onStatus` (the page's StatusMessage, which a new submission empties), once whether the user has a password is
 * fetched again.
 */
export function usePasswordChangeForm(onStatus: (status: string | undefined) => void): PasswordChangeForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const hasPassword = useHasPassword();
  const external = useExternalConfirmation("password_change", hasPassword);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const change = useMutation({
    ...changePasswordMutation(api),
    onSuccess: async (_, { confirmation }) => {
      if (confirmation.kind === "external") external.markUsed();
      await refreshPasswordStatus(api, queryClient);
      setCurrentPassword("");
      setNewPassword("");
      onStatus(t(confirmation.kind === "password" ? "client:settings.password.changed" : "client:settings.password.set.done"));
    },
  });
  const failure = change.isError ? confirmationFailure(change.error) : undefined;
  let error: string | undefined;
  if (failure === "rate-limited") error = t("client:settings.password.rate_limited", { reason: errorMessage(change.error) });
  else if (failure !== undefined && failure !== "wrong-password") {
    error = confirmationErrorMessage(t, failure, change.error, (reason) => t("client:settings.password.failed", { reason }));
  }
  return {
    hasPassword,
    isPasskeySupported: supportsPasskeys(),
    external,
    currentPassword,
    changeCurrentPassword: (next) => {
      setCurrentPassword(next);
      // A refused password marks the field invalid, which stops the form (native validation) until it changes.
      if (change.isError) change.reset();
    },
    newPassword,
    changeNewPassword: (next) => {
      setNewPassword(next);
      if (change.isError) change.reset();
    },
    submit: (event) => {
      event.preventDefault();
      if (change.isPending || newPassword === "" || (hasPassword && currentPassword === "")) return;
      let confirmation: Confirmation;
      if (hasPassword) confirmation = { kind: "password", password: currentPassword };
      else if (!external.isPreferred) confirmation = { kind: "passkey" };
      else if (external.confirmed !== undefined) {
        confirmation = { kind: "external", token: external.confirmed.token };
      } else return;
      change.reset();
      onStatus(undefined);
      change.mutate({ confirmation, newPassword });
    },
    isPending: change.isPending,
    currentPasswordError: failure === "wrong-password" ? t("client:settings.password.wrong_password") : undefined,
    error,
  };
}

/** What removing the password needs. */
export type PasswordRemoval = {
  /** Whether the browser can use passkeys, which confirm the removal. */
  isSupported: boolean;
  /** Whether the user has a passkey, which confirms the removal. */
  hasPasskey: boolean;
  /** The confirmation with a linked provider, which removes the password of a user without passkeys. */
  external: ExternalConfirmation;
  isOpen: boolean;
  open: () => void;
  cancel: () => void;
  /** Asks the browser for a passkey and removes the password; the dialog closes once it is gone. */
  confirm: () => void;
  isPending: boolean;
  /** Why the removal failed, localized, for the dialog (which stays open); undefined when it did not. */
  error: string | undefined;
};

/**
 * Removing the password after a passkey confirms it, from an alert dialog. Once whether the user has a password is
 * fetched again, the dialog closes and the page's StatusMessage (`onStatus`) says what happened.
 */
export function usePasswordRemoval(onStatus: (status: string | undefined) => void): PasswordRemoval {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const hasPasskey = useSuspenseQuery(passkeysQueryOptions(api)).data.items.length > 0;
  // Only a passkey or a linked provider confirms the removal of the password.
  const external = useExternalConfirmation("password_removal", false);
  const [isOpen, setIsOpen] = useState(false);
  const remove = useMutation({
    ...removePasswordMutation(api),
    onSuccess: async (_, confirmation) => {
      if (confirmation.kind === "external") external.markUsed();
      await refreshPasswordStatus(api, queryClient);
      setIsOpen(false);
      onStatus(t("client:settings.password.removed"));
    },
  });
  const failure = remove.isError ? confirmationFailure(remove.error) : undefined;
  return {
    isSupported: supportsPasskeys(),
    hasPasskey,
    external,
    isOpen,
    open: () => {
      remove.reset();
      onStatus(undefined);
      setIsOpen(true);
    },
    cancel: () => {
      setIsOpen(false);
      remove.reset();
    },
    confirm: () => {
      if (remove.isPending) return;
      if (!external.isPreferred) {
        remove.mutate({ kind: "passkey" });
      } else if (external.confirmed !== undefined) {
        remove.mutate({ kind: "external", token: external.confirmed.token });
      }
    },
    isPending: remove.isPending,
    error:
      failure === undefined
        ? undefined
        : confirmationErrorMessage(t, failure, remove.error, (reason) => t("client:settings.password.remove.failed", { reason })),
  };
}
