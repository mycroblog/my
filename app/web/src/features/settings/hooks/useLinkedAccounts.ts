// SPDX-License-Identifier: AGPL-3.0-only
// The user's linked accounts at the login providers (FR-010): the list, unlinking one from its row, and linking one
// after the password, a passkey, or (for a user with neither) a linked provider confirms it. Linking leaves the app
// for the provider, which sends the browser back to this page with the outcome; the page tells it in its
// StatusMessage or an alert (web-client/README.md E2).
import { useAnnouncer } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useLocation } from "@tanstack/react-router";
import { useState } from "react";
import { useApi, useLeaveApp } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { supportsPasskeys } from "../../../lib/webauthn";
import {
  beginLinkMutation,
  type Confirmation,
  type ExternalFailure,
  isExternalConfirmationExpired,
  isLastSignInMethod,
  type LinkableLoginProvider,
  type LinkedIdentity,
  linkedIdentitiesQueryOptions,
  refreshLinkedIdentities,
  unlinkIdentityMutation,
} from "../api/linkedIdentities";
import { passkeysQueryOptions } from "../api/passkeys";
import { confirmationFailure } from "../api/password";
import { confirmationErrorMessage } from "./passkeyConfirmation";
import { type ExternalConfirmation, useExternalConfirmation, useExternalOutcome } from "./useExternalConfirmation";
import { useHasPassword } from "./usePasswordChange";

/** The message of a failed link, as the provider's round trip ended. */
const linkFailureMessages: Record<ExternalFailure, ClientMessageKey> = {
  linked_elsewhere: "client:settings.linked_accounts.failed.linked_elsewhere",
  already_linked: "client:settings.linked_accounts.failed.already_linked",
  denied: "client:settings.linked_accounts.failed.denied",
  unavailable: "client:settings.linked_accounts.failed.unavailable",
  expired: "client:settings.linked_accounts.failed.expired",
  rate_limited: "client:settings.linked_accounts.failed.rate_limited",
  mismatch: "client:settings.linked_accounts.failed.failed",
  failed: "client:settings.linked_accounts.failed.failed",
};

/** The user's linked accounts, oldest first, which the route has loaded. */
export function useLinkedAccountList(): readonly LinkedIdentity[] {
  const api = useApi();
  return useSuspenseQuery(linkedIdentitiesQueryOptions(api)).data.items;
}

/** What the page tells of the link that brought the user back: the status of a success, or the alert of a failure. */
export function useLinkOutcome(): { status: string | undefined; error: string | undefined } {
  const api = useApi();
  const t = useT();
  const outcome = useExternalOutcome();
  const list = useSuspenseQuery(linkedIdentitiesQueryOptions(api)).data;
  if (outcome?.kind === "linked") {
    const name = list.providers.find((p) => p.id === outcome.providerId)?.display_name ?? "";
    return { status: t("client:settings.linked_accounts.linked", { provider: name }), error: undefined };
  }
  if (outcome?.kind === "failed" && outcome.operation === undefined) {
    return { status: undefined, error: t(linkFailureMessages[outcome.failure]) };
  }
  return { status: undefined, error: undefined };
}

/** Unlinking an account from its row. */
export type UnlinkAccount = {
  /** Unlinks the account; `onDone` is called once it is off the list. */
  run: (link: LinkedIdentity, onDone: () => void) => void;
  pendingId: string | undefined;
  /** Why the last unlink failed, localized, for its dialog; undefined when it did not. */
  error: string | undefined;
  reset: () => void;
};

/**
 * Unlinking an account: the row leaves the list once it is fetched again (the dialog stays pending until then), then
 * the caller closes the dialog and moves focus, and the result is announced. A failure keeps the dialog open and says
 * why: the last way to sign in has its own message.
 */
export function useUnlinkAccount(): UnlinkAccount {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const unlink = useMutation({
    ...unlinkIdentityMutation(api),
    onSuccess: async () => {
      await refreshLinkedIdentities(api, queryClient);
    },
  });
  let error: string | undefined;
  if (unlink.isError) {
    error = isLastSignInMethod(unlink.error)
      ? t("client:settings.linked_accounts.unlink.last")
      : t("client:settings.linked_accounts.unlink.failed", { reason: errorMessage(unlink.error) });
  }
  return {
    pendingId: unlink.isPending ? unlink.variables : undefined,
    error,
    reset: unlink.reset,
    run: (link, onDone) => {
      unlink.mutate(link.id, {
        onSuccess: () => {
          onDone();
          announce(t("client:settings.linked_accounts.unlinked", { provider: link.provider_name }));
        },
      });
    },
  };
}

/** How the user confirms a link: the password, a passkey, or a linked provider. */
type LinkConfirmationMethod = "password" | "passkey" | "external";

/** What linking an account needs: the providers, the dialog with its confirmation, and leaving for the provider. */
export type LinkAccount = {
  /** The providers that allow links and that the user links no account at. */
  providers: readonly LinkableLoginProvider[];
  method: LinkConfirmationMethod;
  /** The confirmation with a linked provider, which a user without a password and passkeys gives first. */
  external: ExternalConfirmation;
  /** The provider whose dialog is open. */
  linking: LinkableLoginProvider | undefined;
  open: (provider: LinkableLoginProvider) => void;
  cancel: () => void;
  password: string;
  changePassword: (password: string) => void;
  /** Confirms and leaves for the provider. */
  submit: () => void;
  isPending: boolean;
  /** Why the start failed, localized, for the dialog (which stays open); undefined when it did not. */
  error: string | undefined;
};

export function useLinkAccount(): LinkAccount {
  const api = useApi();
  const t = useT();
  const leaveApp = useLeaveApp();
  const pathname = useLocation({ select: (location) => location.pathname });
  const list = useSuspenseQuery(linkedIdentitiesQueryOptions(api)).data;
  const hasPasskey = useSuspenseQuery(passkeysQueryOptions(api)).data.items.length > 0;
  const hasPassword = useHasPassword();
  const external = useExternalConfirmation("link", hasPassword);
  const [linking, setLinking] = useState<LinkableLoginProvider | undefined>(undefined);
  const [password, setPassword] = useState("");
  const begin = useMutation({
    ...beginLinkMutation(api),
    onSuccess: (url, { confirmation }) => {
      if (confirmation.kind === "external") external.markUsed();
      leaveApp(url);
    },
  });
  const method: LinkConfirmationMethod = hasPassword
    ? "password"
    : hasPasskey && supportsPasskeys()
      ? "passkey"
      : "external";
  let error: string | undefined;
  if (begin.isError) {
    const failure = confirmationFailure(begin.error);
    if (failure === "wrong-password") error = t("client:settings.linked_accounts.link.wrong_password");
    else if (isExternalConfirmationExpired(begin.error))
      error = t("client:settings.external_confirmation.failed.expired");
    else
      error = confirmationErrorMessage(t, failure, begin.error, (reason) =>
        t("client:settings.linked_accounts.link.failed", { reason }),
      );
  }
  return {
    providers: list.providers.filter((p) => p.allow_link && !list.items.some((link) => link.provider_id === p.id)),
    method,
    external,
    linking,
    open: (provider) => {
      begin.reset();
      setPassword("");
      setLinking(provider);
    },
    cancel: () => {
      setLinking(undefined);
      begin.reset();
    },
    password,
    changePassword: setPassword,
    submit: () => {
      if (linking === undefined || begin.isPending) return;
      let confirmation: Confirmation;
      if (method === "password") {
        if (password === "") return;
        confirmation = { kind: "password", password };
      } else if (method === "passkey") {
        confirmation = { kind: "passkey" };
      } else {
        if (external.confirmed === undefined) return;
        confirmation = { kind: "external", token: external.confirmed.token };
      }
      begin.mutate({ providerId: linking.id, confirmation, returnTo: pathname });
    },
    // The browser leaves for the provider after a success, so the dialog stays pending until then.
    isPending: begin.isPending || begin.isSuccess,
    error,
  };
}
