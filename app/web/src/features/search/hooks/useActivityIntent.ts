// SPDX-License-Identifier: AGPL-3.0-only
// The logic of the page that the Activity Intents of WebFinger open (FEP-3b86, web-client/screens/intent.md): a follow
// or reply button of another server sends the viewer here with the address of an account or a post. Nothing is looked
// up until the viewer presses the button, so that another site cannot make this server fetch by sending the browser
// here (FEP-3b86 §6.1, R7). The hook returns no JSX.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { isFederationRestricted } from "../../../lib/federation";
import { type Translate, useT } from "../../../lib/i18n";
import type { Status } from "../../status";
import { lookupAccount, storeFoundAccount } from "../api/lookup";
import { lookupStatus, storeFoundStatus } from "../api/statusLookup";

/** What the button of the other server asks for: following an account, or replying to a post. */
export type ActivityIntentKind = "follow" | "reply";

export type ActivityIntentOptions = {
  kind: ActivityIntentKind;
  /** The address of the account or the post, as the other server sent it. */
  target: string;
  /** Called with the found account's `acct`, to open its profile. */
  onAccount: (acct: string) => void;
  /** Called with the found post, to open it and start the reply. */
  onStatus: (status: Status) => void;
};

export type ActivityIntentState = {
  /** Looks the account or the post up: the button's handler. */
  open: () => void;
  isPending: boolean;
  /** Why the last lookup found nothing (localized), or null. */
  error: string | null;
};

/** The localized reason a lookup failed. 404 and 429 come from the client's catalog (web-client/README.md E2). */
function failureOf(t: Translate, kind: ActivityIntentKind, error: unknown): string {
  if (error instanceof ApiError && error.status === 404)
    return t(kind === "follow" ? "client:intent.follow.not_found" : "client:intent.reply.not_found");
  if (error instanceof ApiError && error.status === 429) return t("client:intent.rate_limited");
  if (isFederationRestricted(error)) return t("client:intent.known_only");
  return t("client:intent.failed", { reason: errorMessage(error) });
}

/** Returns the page's state; a found account or post goes to `onAccount` or `onStatus`. */
export function useActivityIntent({ kind, target, onAccount, onStatus }: ActivityIntentOptions): ActivityIntentState {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const mutation = useMutation({
    mutationFn: async (): Promise<{ acct: string } | { status: Status }> => {
      if (kind === "follow") {
        // The page is for signed-in viewers only (its route), so the account is fetched when this server does not know it.
        const account = await lookupAccount(api, target, true);
        storeFoundAccount(queryClient, api, account);
        return { acct: account.acct };
      }
      const status = await lookupStatus(api, target);
      storeFoundStatus(queryClient, api, status);
      return { status };
    },
    onSuccess: (found) => {
      if ("acct" in found) onAccount(found.acct);
      else onStatus(found.status);
    },
    onError: (error) => {
      // The message is next to the button, which keeps the focus; a screen reader hears it at once.
      notice.inPlace(failureOf(t, kind, error), "assertive");
    },
  });
  return {
    open: () => {
      if (!mutation.isPending) mutation.mutate();
    },
    isPending: mutation.isPending,
    error: mutation.isError ? failureOf(t, kind, mutation.error) : null,
  };
}
