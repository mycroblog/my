// SPDX-License-Identifier: AGPL-3.0-only
// The apps the user authorized (ADR-0090 §5): the list and revoking one. Revoking the server's own web client ends
// the session of every browser, this one included: the app then signs out here and opens the sign-in page.
import { useAnnouncer } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useApi, useSession } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  type AuthorizedApp,
  authorizedAppsQueryOptions,
  refreshAuthorizedApps,
  revokeAuthorizedAppMutation,
} from "../api/authorizedApps";

/** The authorized apps, the most recently used first, which the route has loaded. */
export function useAuthorizedAppList(): readonly AuthorizedApp[] {
  const api = useApi();
  return useSuspenseQuery(authorizedAppsQueryOptions(api)).data.items;
}

/** Revoking an app from its row. */
export type RevokeApp = {
  /** Revokes the app; `onDone` is called once it is off the list. */
  run: (app: AuthorizedApp, onDone: () => void) => void;
  /** The app being revoked. */
  pendingId: string | undefined;
  /** Why the last revocation failed, localized, for its dialog; undefined when it did not. */
  error: string | undefined;
  reset: () => void;
};

/**
 * Revoking an app: the row leaves the list once it is fetched again (the dialog stays pending until then), then the
 * caller closes the dialog and moves focus, and the result is announced. A failure keeps the dialog open and says why.
 * Revoking the web client forgets the session in every tab and opens the sign-in page instead.
 */
export function useRevokeApp(): RevokeApp {
  const api = useApi();
  const session = useSession();
  const navigate = useNavigate();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const revoke = useMutation({
    mutationKey: revokeAuthorizedAppMutation(api).mutationKey,
    mutationFn: async (app: AuthorizedApp) => {
      await revokeAuthorizedAppMutation(api).mutationFn(app.id);
      return app;
    },
    onSuccess: async (app) => {
      if (app.first_party) {
        // The server ended the session of every browser; this forgets it in every tab. The token is already revoked,
        // so the server's answer to signing out does not matter.
        await session.signOut().catch(() => undefined);
        await navigate({ to: "/login" });
        return;
      }
      await refreshAuthorizedApps(api, queryClient);
    },
  });
  return {
    pendingId: revoke.isPending ? revoke.variables.id : undefined,
    error: revoke.isError ? t("client:settings.authorized_apps.revoke.failed", { reason: errorMessage(revoke.error) }) : undefined,
    reset: revoke.reset,
    run: (app, onDone) => {
      revoke.mutate(app, {
        onSuccess: () => {
          onDone();
          announce(t("client:settings.authorized_apps.revoked", { name: app.name }));
        },
      });
    },
  };
}
