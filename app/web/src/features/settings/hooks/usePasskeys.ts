// SPDX-License-Identifier: AGPL-3.0-only
// The user's passkeys (FR-004): the list, the form that registers one (its name and the password that confirms it, or
// for a user without a password another passkey, then the browser's prompt), and removing one from its row. A wrong
// password is the password field's error; any other failure an alert next to the form (web-client/README.md E2, web-client/16-look-states-and-wording.md G16).
import { useAnnouncer } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { supportsPasskeys } from "../../../lib/webauthn";
import {
  type Passkey,
  type PasskeyFailure,
  passkeyFailure,
  passkeysQueryOptions,
  refreshPasskeys,
  registerPasskeyMutation,
  removePasskeyMutation,
} from "../api/passkeys";
import type { Confirmation } from "../api/linkedIdentities";
import { type ExternalConfirmation, useExternalConfirmation } from "./useExternalConfirmation";
import { useHasPassword } from "./usePasswordChange";

/** The user's passkeys, oldest first, which the route has loaded. */
export function usePasskeyList(): readonly Passkey[] {
  const api = useApi();
  return useSuspenseQuery(passkeysQueryOptions(api)).data.items;
}

/** What the form that registers a passkey needs. */
export type PasskeyForm = {
  /** Whether the browser can create passkeys; the form is left out when it cannot. */
  isSupported: boolean;
  /** Whether the user has a password, which confirms the registration; without one, another passkey confirms it. */
  hasPassword: boolean;
  /** The confirmation with a linked provider, for a user with neither a password nor a passkey. */
  external: ExternalConfirmation;
  name: string;
  changeName: (name: string) => void;
  password: string;
  changePassword: (password: string) => void;
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** The password field's error when the server refused the password, localized; undefined otherwise. */
  passwordError: string | undefined;
  /** Why the registration failed otherwise, localized, for an alert next to the form; undefined when it did not. */
  error: string | undefined;
  /** What the last registration did, for the form's StatusMessage; undefined before the first one. */
  status: string | undefined;
};

/** The alert of a failure that is not the password's. */
function failureMessage(t: ReturnType<typeof useT>, failure: PasskeyFailure, error: unknown): string {
  switch (failure) {
    case "cancelled":
      return t("client:settings.passkeys.add.cancelled");
    case "exists":
      return t("client:settings.passkeys.add.exists");
    case "unavailable":
      return t("client:settings.passkeys.add.unavailable");
    case "invalid":
      return t("client:settings.passkeys.add.invalid");
    case "expired":
      return t("client:settings.passkeys.add.expired");
    case "too-many":
      return t("client:settings.passkeys.add.too_many");
    case "rate-limited":
      return t("client:settings.passkeys.add.rate_limited", { reason: errorMessage(error) });
    default:
      return t("client:settings.passkeys.add.failed", { reason: errorMessage(error) });
  }
}

/**
 * The form that registers a passkey. A success empties the form and says which passkey was added, once the list is
 * fetched again (the form stays pending until then).
 */
export function usePasskeyForm(): PasskeyForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const hasPassword = useHasPassword();
  const external = useExternalConfirmation("passkey_registration", hasPassword);
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [status, setStatus] = useState<string | undefined>(undefined);
  const register = useMutation({
    ...registerPasskeyMutation(api),
    onSuccess: async (added, { confirmation }) => {
      if (confirmation.kind === "external") external.markUsed();
      await refreshPasskeys(api, queryClient);
      setName("");
      setPassword("");
      setStatus(t("client:settings.passkeys.added", { name: added.name }));
    },
    onError: async (error) => {
      // A full list explains the refusal; show it as it is.
      if (passkeyFailure(error) === "too-many") await refreshPasskeys(api, queryClient);
    },
  });
  const failure = register.isError ? passkeyFailure(register.error) : undefined;
  return {
    isSupported: supportsPasskeys(),
    hasPassword,
    external,
    name,
    changeName: (next) => {
      setName(next);
      if (register.isError) register.reset();
    },
    password,
    changePassword: (next) => {
      setPassword(next);
      // A refused password marks the field invalid, which stops the form (native validation) until it changes.
      if (register.isError) register.reset();
    },
    submit: (event) => {
      event.preventDefault();
      const trimmed = name.trim();
      if (register.isPending || trimmed === "" || (hasPassword && password === "")) return;
      let confirmation: Confirmation;
      if (hasPassword) confirmation = { kind: "password", password };
      else if (!external.isPreferred) confirmation = { kind: "passkey" };
      else if (external.confirmed !== undefined) confirmation = { kind: "external", token: external.confirmed.token };
      else return;
      register.reset();
      setStatus(undefined);
      register.mutate({ name: trimmed, confirmation });
    },
    isPending: register.isPending,
    passwordError: failure === "wrong-password" ? t("client:settings.passkeys.add.wrong_password") : undefined,
    error:
      failure === undefined || failure === "wrong-password" ? undefined : failureMessage(t, failure, register.error),
    status,
  };
}

/** Removing a passkey from its row. */
export type RemovePasskey = {
  /** Removes the passkey; `onDone` is called once it is off the list. */
  run: (passkey: Passkey, onDone: () => void) => void;
  /** The passkey being removed. */
  pendingId: string | undefined;
  /** Why the last removal failed, localized, for its dialog; undefined when it did not. */
  error: string | undefined;
  reset: () => void;
};

/**
 * Removing a passkey: the row leaves the list once it is fetched again (the dialog stays pending until then), then the
 * caller closes the dialog and moves focus, and the result is announced. A failure keeps the dialog open and says why.
 */
export function useRemovePasskey(): RemovePasskey {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const remove = useMutation({
    ...removePasskeyMutation(api),
    onSuccess: async () => {
      await refreshPasskeys(api, queryClient);
    },
  });
  return {
    pendingId: remove.isPending ? remove.variables : undefined,
    error: remove.isError
      ? t("client:settings.passkeys.remove.failed", { reason: errorMessage(remove.error) })
      : undefined,
    reset: remove.reset,
    run: (passkey, onDone) => {
      remove.mutate(passkey.id, {
        onSuccess: () => {
          onDone();
          announce(t("client:settings.passkeys.removed", { name: passkey.name }));
        },
      });
    },
  };
}
