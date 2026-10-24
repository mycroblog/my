// SPDX-License-Identifier: AGPL-3.0-only
// The user's own OAuth apps (ADR-0090 §1, §2): the list, the form that registers one, and what a row does: change it,
// delete it, give it a new secret, and issue tokens of it to the user. A secret and the tokens are kept on the screen
// only until the user leaves or dismisses them (web-client/README.md E2, web-client/16-look-states-and-wording.md G16).
import { useAnnouncer, useToast } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  createOAuthAppMutation,
  deleteOAuthAppMutation,
  issueOAuthAppTokenMutation,
  type OAuthApp,
  type OAuthAppClientType,
  type OAuthAppToken,
  oauthAppsQueryOptions,
  refreshOAuthApps,
  rotateOAuthAppSecretMutation,
  updateOAuthAppMutation,
} from "../api/oauthApps";

/** The user's apps, oldest first, which the route has loaded. */
export function useOAuthAppList(): readonly OAuthApp[] {
  const api = useApi();
  return useSuspenseQuery(oauthAppsQueryOptions(api)).data.items;
}

/** What the fields of an app hold while the user types. */
export type AppDraft = {
  name: string;
  website: string;
  /** One redirect URI per line. */
  redirectUris: string;
  /** Space-separated scopes. */
  scope: string;
  clientType: OAuthAppClientType;
};

/** A new app: the scopes most bots need, public. */
const newAppDraft: AppDraft = { name: "", website: "", redirectUris: "", scope: "read write", clientType: "public" };

/** The draft that edits `app`. */
export function draftOf(app: OAuthApp): AppDraft {
  return {
    name: app.name,
    website: app.website ?? "",
    redirectUris: app.redirect_uris.join("\n"),
    scope: app.scope,
    clientType: app.client_type,
  };
}

/** The redirect URIs typed, one per line, without the blank lines. */
function redirectUrisOf(text: string): string[] {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "");
}

/** The scopes typed, separated by white space, as the single-spaced list the API takes. */
function scopeOf(text: string): string {
  return text.split(/\s+/).filter((scope) => scope !== "").join(" ");
}

/** Copies a value to the clipboard and says whether that worked in a toast. */
export function useCopyValue(): (value: string) => void {
  const t = useT();
  const toast = useToast();
  return (value) => {
    navigator.clipboard.writeText(value).then(
      () => {
        // The toast is announced too.
        toast.show({ message: t("client:settings.apps.copied"), tone: "success" });
      },
      () => {
        toast.show({ message: t("client:settings.apps.copy_failed"), tone: "danger" });
      },
    );
  };
}

/** A secret kept to show once: the app it is for and the secret (absent for an app without one). */
export type ShownSecret = { app: OAuthApp; secret: string | undefined };

/** What the form that registers an app needs. */
export type RegisterAppForm = {
  draft: AppDraft;
  change: (change: Partial<AppDraft>) => void;
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** Why registering failed, localized, for an alert; the draft stays. */
  error: string | undefined;
  /** What the last registration did, for the form's StatusMessage; undefined before the first one. */
  status: string | undefined;
};

/**
 * The form that registers an app. A success empties the form, hands the app and its secret to `onRegistered` to show
 * once, and says which app was registered once the list is fetched again.
 */
export function useRegisterAppForm(onRegistered: (shown: ShownSecret) => void): RegisterAppForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<AppDraft>(newAppDraft);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const register = useMutation({
    ...createOAuthAppMutation(api),
    onSuccess: async ({ app, client_secret }) => {
      await refreshOAuthApps(api, queryClient);
      setDraft(newAppDraft);
      setStatus(t("client:settings.apps.registered", { name: app.name }));
      onRegistered({ app, secret: client_secret });
    },
  });
  return {
    draft,
    change: (change) => {
      setDraft((previous) => ({ ...previous, ...change }));
      if (register.isError) register.reset();
    },
    submit: (event) => {
      event.preventDefault();
      const name = draft.name.trim();
      const redirectUris = redirectUrisOf(draft.redirectUris);
      const scope = scopeOf(draft.scope);
      if (register.isPending || name === "" || redirectUris.length === 0 || scope === "") return;
      register.reset();
      setStatus(undefined);
      const website = draft.website.trim();
      register.mutate({
        name,
        redirect_uris: redirectUris,
        scope,
        client_type: draft.clientType,
        ...(website !== "" && { website }),
      });
    },
    isPending: register.isPending,
    error: register.isError ? t("client:settings.apps.register_failed", { reason: errorMessage(register.error) }) : undefined,
    status,
  };
}

/** Changing an app in its dialog. */
export type UpdateApp = {
  /** Saves `draft`; `onDone` is called once the list shows it. */
  run: (app: OAuthApp, draft: AppDraft, onDone: () => void) => void;
  isPending: boolean;
  error: string | undefined;
};

/** Changing an app: the dialog stays open and says why when the server refuses a setting. */
export function useUpdateApp(): UpdateApp {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const update = useMutation({
    ...updateOAuthAppMutation(api),
    onSuccess: async () => {
      await refreshOAuthApps(api, queryClient);
    },
  });
  return {
    isPending: update.isPending,
    error: update.isError ? t("client:settings.apps.save_failed", { reason: errorMessage(update.error) }) : undefined,
    run: (app, draft, onDone) => {
      update.mutate(
        {
          id: app.id,
          body: {
            name: draft.name.trim(),
            // An empty website removes it (UpdateOAuthAppRequest).
            website: draft.website.trim(),
            redirect_uris: redirectUrisOf(draft.redirectUris),
            scope: scopeOf(draft.scope),
          },
        },
        {
          onSuccess: (saved) => {
            onDone();
            announce(t("client:settings.apps.saved", { name: saved.name }));
          },
        },
      );
    },
  };
}

/** Deleting an app from its row. */
export type DeleteApp = {
  /** Deletes the app; `onDone` is called once it is off the list. */
  run: (app: OAuthApp, onDone: () => void) => void;
  /** The app being deleted. */
  pendingId: string | undefined;
  error: string | undefined;
  reset: () => void;
};

/** Deleting an app: the row leaves the list once it is fetched again, then the caller closes the dialog and moves focus. */
export function useDeleteApp(): DeleteApp {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const remove = useMutation({
    ...deleteOAuthAppMutation(api),
    onSuccess: async () => {
      await refreshOAuthApps(api, queryClient);
    },
  });
  return {
    pendingId: remove.isPending ? remove.variables : undefined,
    error: remove.isError ? t("client:settings.apps.delete.failed", { reason: errorMessage(remove.error) }) : undefined,
    reset: remove.reset,
    run: (app, onDone) => {
      remove.mutate(app.id, {
        onSuccess: () => {
          onDone();
          announce(t("client:settings.apps.deleted", { name: app.name }));
        },
      });
    },
  };
}

/** Giving a confidential app a new secret. */
export type RotateSecret = {
  /** Replaces the secret; `onDone` gets the new one. */
  run: (app: OAuthApp, onDone: (secret: string) => void) => void;
  pendingId: string | undefined;
  error: string | undefined;
  reset: () => void;
};

/** Giving an app a new secret: the old one stops working at once; the new one is handed on to be shown once. */
export function useRotateSecret(): RotateSecret {
  const api = useApi();
  const t = useT();
  const rotate = useMutation(rotateOAuthAppSecretMutation(api));
  return {
    pendingId: rotate.isPending ? rotate.variables : undefined,
    error: rotate.isError ? t("client:settings.apps.rotate.failed", { reason: errorMessage(rotate.error) }) : undefined,
    reset: rotate.reset,
    run: (app, onDone) => {
      rotate.mutate(app.id, { onSuccess: onDone });
    },
  };
}

/** Issuing tokens of an app to the user. */
export type IssueToken = {
  run: (app: OAuthApp, scope: string, onDone: (tokens: OAuthAppToken) => void) => void;
  isPending: boolean;
  error: string | undefined;
};

/** Issuing tokens: the tokens are handed on to be shown once; a refusal (a scope beyond the app's) is shown in the dialog. */
export function useIssueToken(): IssueToken {
  const api = useApi();
  const t = useT();
  const issue = useMutation(issueOAuthAppTokenMutation(api));
  return {
    isPending: issue.isPending,
    error: issue.isError ? t("client:settings.apps.token.failed", { reason: errorMessage(issue.error) }) : undefined,
    run: (app, scope, onDone) => {
      issue.mutate({ id: app.id, scope: scopeOf(scope) }, { onSuccess: onDone });
    },
  };
}
