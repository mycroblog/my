// SPDX-License-Identifier: AGPL-3.0-only
// Deactivating the user's own account (ADR-0145): what it would do now, the dialog that confirms it with the password,
// a code of the second factor, a passkey or a linked provider, and the page that states the date of the deletion.
import { useMutation, useSuspenseQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useApi, useSession } from "../../../lib/auth/SessionProvider";
import { ApiError } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  type DeactivationConfirmation,
  type DeactivationPreview,
  deactivateOwnAccountMutation,
  deactivationPreviewQueryOptions,
  invalidCodeType,
} from "../api/account";
import { confirmationFailure } from "../api/password";
import { twoFactorQueryOptions } from "../api/twoFactor";
import { confirmationErrorMessage } from "./passkeyConfirmation";
import { type ExternalConfirmation, useExternalConfirmation } from "./useExternalConfirmation";
import { useHasPassword } from "./usePasswordChange";

/** What confirms the deactivation in the dialog: the way the account signs in, or a code of its second factor. */
type DeactivationMethod = "sign-in" | "code";

/** What the deactivation of the account needs. */
export type AccountDeactivation = {
  /** When the account would be deleted if the user deactivated it now. */
  deletionDueAt: string;
  /** Why it cannot be deactivated now; undefined when it can. */
  refusal: DeactivationPreview["refusal"];
  /** When it can be deactivated again, after a recent reactivation. */
  availableAt: string | undefined;
  hasPassword: boolean;
  /** Whether the user has a second factor, whose code may confirm in place of the rest. */
  hasCode: boolean;
  /** The confirmation with a linked provider, for a user with neither a password nor a passkey. */
  external: ExternalConfirmation;
  isOpen: boolean;
  open: () => void;
  cancel: () => void;
  method: DeactivationMethod;
  changeMethod: (method: DeactivationMethod) => void;
  password: string;
  changePassword: (password: string) => void;
  code: string;
  changeCode: (code: string) => void;
  /** Deactivates the account; on success the user is signed out and sent to the page that states the date. */
  submit: () => void;
  isPending: boolean;
  /** Why the deactivation failed, localized, for the dialog (which stays open). */
  error: string | undefined;
};

export function useAccountDeactivation(): AccountDeactivation {
  const api = useApi();
  const session = useSession();
  const navigate = useNavigate();
  const t = useT();
  const hasPassword = useHasPassword();
  const external = useExternalConfirmation("account_deactivation", hasPassword);
  const preview = useSuspenseQuery(deactivationPreviewQueryOptions(api));
  const twoFactor = useSuspenseQuery(twoFactorQueryOptions(api));
  const [isOpen, setIsOpen] = useState(false);
  const [method, setMethod] = useState<DeactivationMethod>("sign-in");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const mutation = useMutation({
    ...deactivateOwnAccountMutation(api),
    onSuccess: async ({ deletion_due_at }) => {
      setIsOpen(false);
      setPassword("");
      setCode("");
      // The server has ended every session and token of the account; this forgets them in every tab.
      await session.signOut().catch(() => undefined);
      await navigate({ to: "/deactivated", search: { deletion_due_at } });
    },
  });
  const confirmation = (): DeactivationConfirmation | undefined => {
    if (method === "code") return code === "" ? undefined : { kind: "code", code };
    if (hasPassword) return password === "" ? undefined : { kind: "password", password };
    if (!external.isPreferred) return { kind: "passkey" };
    return external.confirmed === undefined ? undefined : { kind: "external", token: external.confirmed.token };
  };
  const failure = mutation.isError ? confirmationFailure(mutation.error) : undefined;
  const wrongCode = mutation.error instanceof ApiError && mutation.error.type === invalidCodeType;
  return {
    deletionDueAt: preview.data.deletion_due_at,
    refusal: preview.data.refusal,
    availableAt: preview.data.available_at,
    hasPassword,
    hasCode: twoFactor.data.enabled,
    external,
    isOpen,
    open: () => {
      mutation.reset();
      setMethod("sign-in");
      setPassword("");
      setCode("");
      setIsOpen(true);
    },
    cancel: () => {
      setIsOpen(false);
      mutation.reset();
    },
    method,
    changeMethod: (next) => {
      mutation.reset();
      setMethod(next);
    },
    password,
    changePassword: setPassword,
    code,
    changeCode: setCode,
    submit: () => {
      if (mutation.isPending) return;
      const c = confirmation();
      if (c !== undefined) mutation.mutate(c);
    },
    isPending: mutation.isPending,
    error:
      failure === undefined
        ? undefined
        : wrongCode
          ? t("client:settings.account.deactivate.wrong_code")
          : failure === "wrong-password"
            ? t("client:settings.account.deactivate.wrong_password")
            : confirmationErrorMessage(t, failure, mutation.error, (reason) => reason),
  };
}
