// SPDX-License-Identifier: AGPL-3.0-only
// The OAuth apps the reader registers for a bot, a script or a client of their own (ADR-0090): the list, one app, making
// one (a confidential app's secret comes in that answer only), changing it, deleting it, making a new secret and issuing
// a token of it for the reader's own account (the token is in that answer only). Each write puts what it changed in the
// cache, and what changes the apps the account authorized (authorized-apps.ts) has that list read again. Whether the
// server lets users register apps, the list says (`can_create`), and a registration refused because it does not
// corrects the list in the cache: the administrators turned registration off since it was read.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import { viewerQuery } from "./accounts";
import type { AppData } from "./app-data";
import { authorizedAppsQuery } from "./authorized-apps";
import { useAppData } from "./provider";
import { removeCached, writeCached } from "./written";

/** An OAuth app of the reader's. */
export type OAuthApp = components["schemas"]["OAuthApp"];

/** What registers an app: its name, redirect URIs, scope, and whether it keeps a secret (`confidential`). */
export type CreateOAuthApp = components["schemas"]["CreateOAuthAppRequest"];

/** What changes an app; what is left out stays. The type never changes. */
export type UpdateOAuthApp = components["schemas"]["UpdateOAuthAppRequest"];

/** A new app, with its secret when it is confidential: the only time the secret is shown. */
export type OAuthAppCreated = components["schemas"]["OAuthAppCreated"];

/** An access token and a refresh token of an app for the reader's own account: shown only once. */
export type OAuthAppToken = components["schemas"]["OAuthAppToken"];

type OAuthAppList = components["schemas"]["OAuthAppList"];

/** The reader's apps. */
export function oAuthAppsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/oauth_apps");
}

/** The reader's app of `id`. */
export function oAuthAppQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/oauth_apps/{id}", { params: { path: { id } } });
}

/** Whether `error` is the server's refusal of a registration because its users may not register apps (the 403). */
export function oAuthAppRegistrationOff(error: unknown): boolean {
  return error instanceof ProblemError && error.type === "/api/v1/problems/oauth-app-registration-disabled";
}

/**
 * Registers an app, and puts it in the cache. It rejects with the server's problem: a setting that breaks a rule (422,
 * `field` names it), 25 apps already (409), a server that does not let users register apps (403: the list in the cache
 * then says so too, as `useOAuthAppRegistrationOff` reads it), too many an hour (429).
 */
export async function createOAuthApp(data: AppData, request: CreateOAuthApp): Promise<OAuthAppCreated> {
  const { queryClient } = data;
  const { data: created } = await data.api.POST("/oauth_apps", { body: request }).catch(async (error: unknown) => {
    // Turned off since the list was read: the list is corrected, for the screens that read it.
    if (oAuthAppRegistrationOff(error))
      await writeCached<OAuthAppList>(data, oAuthAppsQuery(data).queryKey, (now) => ({ ...now, can_create: false }));
    throw error;
  });
  if (created === undefined) throw new Error("app-data: /oauth_apps answered without the app");
  queryClient.setQueryData<OAuthApp>(oAuthAppQuery(data, created.app.id).queryKey, created.app);
  await writeCached<OAuthAppList>(data, oAuthAppsQuery(data).queryKey, (now) => ({
    ...now,
    items: [...now.items, created.app],
  }));
  return created;
}

/**
 * What a write on one of the reader's apps changed of the apps the account authorized, which the server derives from
 * the working tokens (a token issued is a grant, a deleted app has none left, a rename is the name shown): that list
 * is stale, read again by the screen that shows it now, or by the next that does.
 */
function grantsChanged(data: AppData): void {
  void data.queryClient.invalidateQueries({ queryKey: authorizedAppsQuery(data).queryKey });
}

/**
 * Changes an app, and puts it in the cache as the server left it, the reads of it and of the list on their way
 * cancelled first; the apps the account authorized are read again. It rejects with the problem (422 names the
 * setting).
 */
export async function updateOAuthApp(data: AppData, id: string, changes: UpdateOAuthApp): Promise<OAuthApp> {
  const { data: app } = await data.api.PATCH("/oauth_apps/{id}", { params: { path: { id } }, body: changes });
  if (app === undefined) throw new Error("app-data: /oauth_apps/{id} answered without the app");
  const one = oAuthAppQuery(data, id).queryKey;
  await data.queryClient.cancelQueries({ queryKey: one });
  data.queryClient.setQueryData<OAuthApp>(one, app);
  await writeCached<OAuthAppList>(data, oAuthAppsQuery(data).queryKey, (now) => ({
    ...now,
    items: now.items.map((a) => (a.id === id ? app : a)),
  }));
  grantsChanged(data);
  return app;
}

/**
 * Deletes an app for good, with every token of it, and takes it from the cache, the reads of the list on their way
 * cancelled first; the apps the account authorized are read again. An app the server no longer has (404) is deleted
 * already: it resolves all the same, and the list is read again. It rejects with the problem.
 */
export async function deleteOAuthApp(data: AppData, id: string): Promise<void> {
  await removeCached<OAuthAppList>(
    data,
    oAuthAppsQuery(data).queryKey,
    data.api.DELETE("/oauth_apps/{id}", { params: { path: { id } } }),
    (now) => ({ ...now, items: now.items.filter((a) => a.id !== id) }),
  );
  data.queryClient.removeQueries({ queryKey: oAuthAppQuery(data, id).queryKey });
  grantsChanged(data);
}

/**
 * Makes a new secret of a confidential app and returns it, shown only this once; the old one stops working. It rejects
 * with the server's problem (409 for a public app).
 */
export async function rotateOAuthAppSecret(data: AppData, id: string): Promise<string> {
  const { data: secret } = await data.api.POST("/oauth_apps/{id}/rotate_secret", { params: { path: { id } } });
  if (secret === undefined) throw new Error("app-data: /oauth_apps/{id}/rotate_secret answered without a secret");
  return secret.client_secret;
}

/**
 * Issues a token of the app for the reader's own account, within `scope` (the app's whole scope when left out), and
 * returns it, shown only this once; the app is then one the account authorized, and that list is read again. It
 * rejects with the server's problem (422 for a scope beyond the app's, 409 for an app an administrator disabled).
 */
export async function issueOAuthAppToken(data: AppData, id: string, scope?: string): Promise<OAuthAppToken> {
  const { data: token } = await data.api.POST("/oauth_apps/{id}/tokens", {
    params: { path: { id } },
    body: scope === undefined ? {} : { scope },
  });
  if (token === undefined) throw new Error("app-data: /oauth_apps/{id}/tokens answered without a token");
  grantsChanged(data);
  return token;
}

/** The reader's apps; undefined until the server answered. */
export function useOAuthApps(): readonly OAuthApp[] | undefined {
  return useQuery(oAuthAppsQuery(useAppData())).data?.items;
}

/** The reader's app of `id`; undefined until the server answered, and when it has none (`useOAuthAppRead` says which). */
export function useOAuthApp(id: string): OAuthApp | undefined {
  return useQuery(oAuthAppQuery(useAppData(), id)).data;
}

/**
 * Where the read of the reader's app of `id` is, for the screens of one app: `read` once the server answered with it,
 * `missing` when the reader has no such app (deleted, another user's, an ID that names none), `failed` when the server
 * did not answer, with `retry`, which reads it again, and `loading` until then.
 */
export function useOAuthAppRead(id: string): {
  status: "loading" | "failed" | "missing" | "read";
  retry: () => void;
} {
  const read = useQuery(oAuthAppQuery(useAppData(), id));
  const settled = read.status === "error" && !read.isFetching;
  return {
    // What was read stays read: a later read of it that fails leaves the screen what it had.
    status:
      read.data !== undefined
        ? "read"
        : !settled
          ? "loading"
          : read.error instanceof ProblemError && read.error.status === 404
            ? "missing"
            : "failed",
    retry: () => {
      void read.refetch();
    },
  };
}

/**
 * Where the reads of the screen that issues a token of the reader's app of `id` are: the app (`useOAuthAppRead`), and
 * the reader's account, whose permissions say which permission scopes the token may carry, so that the form starts
 * from them. `read` once the server answered both, `missing` when the reader has no such app, `failed` when the server
 * did not answer one, with `retry`, which reads again what failed, and `loading` until then.
 */
export function useOAuthAppTokenRead(id: string): {
  status: "loading" | "failed" | "missing" | "read";
  retry: () => void;
} {
  const app = useOAuthAppRead(id);
  const account = useQuery(viewerQuery(useAppData()));
  const failed = account.status === "error" && !account.isFetching;
  return {
    // What was read stays read: a later read of it that fails leaves the screen what it had.
    status: app.status !== "read" ? app.status : account.data !== undefined ? "read" : failed ? "failed" : "loading",
    retry: () => {
      if (app.status === "failed") app.retry();
      if (failed) void account.refetch();
    },
  };
}

/**
 * Where the read of the reader's apps is, for a screen that needs the list alone (registering an app): `read` once the
 * server answered, `failed` when it did not, with `retry`, which reads it again, and `loading` until then.
 */
export function useOAuthAppsRead(): { status: "loading" | "failed" | "read"; retry: () => void } {
  const read = useQuery(oAuthAppsQuery(useAppData()));
  return {
    // What was read stays read: a later read of it that fails leaves the screen what it had.
    status: read.data !== undefined ? "read" : read.status === "error" && !read.isFetching ? "failed" : "loading",
    retry: () => {
      void read.refetch();
    },
  };
}

/**
 * Whether the server's administrators turned registration by users off, as the list of the reader's apps says
 * (`can_create`: the instance's setting alone, not the most apps a user has nor how many an hour); false until the
 * list is read. The apps registered before stay listed and keep working.
 */
export function useOAuthAppRegistrationOff(): boolean {
  return useQuery(oAuthAppsQuery(useAppData())).data?.can_create === false;
}

export type OAuthAppWrites = {
  readonly create: (request: CreateOAuthApp) => Promise<OAuthAppCreated>;
  readonly update: (id: string, changes: UpdateOAuthApp) => Promise<OAuthApp>;
  readonly remove: (id: string) => Promise<void>;
  readonly rotateSecret: (id: string) => Promise<string>;
  readonly issueToken: (id: string, scope?: string) => Promise<OAuthAppToken>;
};

/** Making, changing and deleting the reader's apps, their secrets and tokens, from a screen. */
export function useOAuthAppWrites(): OAuthAppWrites {
  const data = useAppData();
  return {
    create: (request) => createOAuthApp(data, request),
    update: (id, changes) => updateOAuthApp(data, id, changes),
    remove: (id) => deleteOAuthApp(data, id),
    rotateSecret: (id) => rotateOAuthAppSecret(data, id),
    issueToken: (id, scope) => issueOAuthAppToken(data, id, scope),
  };
}
