// SPDX-License-Identifier: AGPL-3.0-only
// The browsers signed in to the user's account (ADR-0136): the list, signing out one, and signing out the others.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  type BrowserSession,
  browserSessionsQueryOptions,
  refreshBrowserSessions,
  revokeBrowserSessionMutation,
  revokeOtherBrowserSessionsMutation,
} from "../api/browserSessions";

/** The signed-in browsers, this one first, which the route has loaded. */
export function useBrowserSessionList(): readonly BrowserSession[] {
  const api = useApi();
  return useSuspenseQuery(browserSessionsQueryOptions(api)).data.items;
}

/** Signing out a browser from its row. */
export type RevokeBrowserSession = {
  /** Signs the browser out; `onDone` is called once it is off the list. */
  run: (session: BrowserSession, onDone: () => void) => void;
  /** The ID of the browser being signed out. */
  pendingId: string | undefined;
  /** Why the last attempt failed, localized, for its dialog; undefined when it did not. */
  error: string | undefined;
  reset: () => void;
};

/**
 * Signing out a browser: the row leaves the list once it is fetched again (the dialog stays pending until then), then
 * the caller closes the dialog and moves focus, and the result is announced. A failure keeps the dialog open and says
 * why.
 */
export function useRevokeBrowserSession(): RevokeBrowserSession {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const revoke = useMutation({
    ...revokeBrowserSessionMutation(api),
    onSuccess: () => refreshBrowserSessions(api, queryClient),
  });
  return {
    pendingId: revoke.isPending ? revoke.variables : undefined,
    error: revoke.isError
      ? t("client:settings.sessions.revoke.failed", { reason: errorMessage(revoke.error) })
      : undefined,
    reset: revoke.reset,
    run: (session, onDone) => {
      revoke.mutate(session.id, {
        onSuccess: () => {
          onDone();
          notice.inPlace(t("client:settings.sessions.revoked"));
        },
      });
    },
  };
}

/** Signing out every other browser. */
export type RevokeOtherBrowserSessions = {
  /** Signs the others out; `onDone` is called once they are off the list. */
  run: (onDone: () => void) => void;
  isPending: boolean;
  /** Why the last attempt failed, localized, for its dialog; undefined when it did not. */
  error: string | undefined;
  reset: () => void;
};

/** Signing out every other browser, with the same flow as one browser. */
export function useRevokeOtherBrowserSessions(): RevokeOtherBrowserSessions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const revoke = useMutation({
    ...revokeOtherBrowserSessionsMutation(api),
    onSuccess: () => refreshBrowserSessions(api, queryClient),
  });
  return {
    isPending: revoke.isPending,
    error: revoke.isError
      ? t("client:settings.sessions.revoke_others.failed", { reason: errorMessage(revoke.error) })
      : undefined,
    reset: revoke.reset,
    run: (onDone) => {
      revoke.mutate(undefined, {
        onSuccess: () => {
          onDone();
          notice.inPlace(t("client:settings.sessions.others_revoked"));
        },
      });
    },
  };
}
