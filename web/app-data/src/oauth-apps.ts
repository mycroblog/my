// SPDX-License-Identifier: AGPL-3.0-only
// The OAuth apps the reader registers for a bot, a script or a client of their own (ADR-0090): the list, one app, making
// one (a confidential app's secret comes in that answer only), changing it, deleting it, making a new secret and issuing
// a token of it for the reader's own account (the token is in that answer only). Each write puts what it changed in the
// cache.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

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

/**
 * Registers an app, and puts it in the cache. It rejects with the server's problem: a setting that breaks a rule (422,
 * `field` names it), 25 apps already (409), a server that does not let users register apps (403), too many an hour (429).
 */
export async function createOAuthApp(data: AppData, request: CreateOAuthApp): Promise<OAuthAppCreated> {
  const { data: created } = await data.api.POST("/oauth_apps", { body: request });
  if (created === undefined) throw new Error("app-data: /oauth_apps answered without the app");
  const { queryClient } = data;
  queryClient.setQueryData<OAuthApp>(oAuthAppQuery(data, created.app.id).queryKey, created.app);
  queryClient.setQueryData<OAuthAppList>(oAuthAppsQuery(data).queryKey, (now) =>
    now === undefined ? now : { ...now, items: [...now.items, created.app] },
  );
  return created;
}

/** Changes an app, and puts it in the cache as the server left it. It rejects with the problem (422 names the setting). */
export async function updateOAuthApp(data: AppData, id: string, changes: UpdateOAuthApp): Promise<OAuthApp> {
  const { data: app } = await data.api.PATCH("/oauth_apps/{id}", { params: { path: { id } }, body: changes });
  if (app === undefined) throw new Error("app-data: /oauth_apps/{id} answered without the app");
  data.queryClient.setQueryData<OAuthApp>(oAuthAppQuery(data, id).queryKey, app);
  data.queryClient.setQueryData<OAuthAppList>(oAuthAppsQuery(data).queryKey, (now) =>
    now === undefined ? now : { ...now, items: now.items.map((a) => (a.id === id ? app : a)) },
  );
  return app;
}

/** Deletes an app for good, with every token of it, and takes it from the cache. It rejects with the problem. */
export async function deleteOAuthApp(data: AppData, id: string): Promise<void> {
  await data.api.DELETE("/oauth_apps/{id}", { params: { path: { id } } });
  data.queryClient.removeQueries({ queryKey: oAuthAppQuery(data, id).queryKey });
  data.queryClient.setQueryData<OAuthAppList>(oAuthAppsQuery(data).queryKey, (now) =>
    now === undefined ? now : { ...now, items: now.items.filter((a) => a.id !== id) },
  );
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
 * returns it, shown only this once. It rejects with the server's problem (422 for a scope beyond the app's, 409 for an
 * app an administrator disabled).
 */
export async function issueOAuthAppToken(data: AppData, id: string, scope?: string): Promise<OAuthAppToken> {
  const { data: token } = await data.api.POST("/oauth_apps/{id}/tokens", {
    params: { path: { id } },
    body: scope === undefined ? {} : { scope },
  });
  if (token === undefined) throw new Error("app-data: /oauth_apps/{id}/tokens answered without a token");
  return token;
}

/** The reader's apps; undefined until the server answered. */
export function useOAuthApps(): readonly OAuthApp[] | undefined {
  return useQuery(oAuthAppsQuery(useAppData())).data?.items;
}

/** The reader's app of `id`; undefined until the server answered, and when it has none (`useQuery`'s error says why). */
export function useOAuthApp(id: string): OAuthApp | undefined {
  return useQuery(oAuthAppQuery(useAppData(), id)).data;
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
