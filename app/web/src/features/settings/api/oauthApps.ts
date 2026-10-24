// SPDX-License-Identifier: AGPL-3.0-only
// The OAuth apps that the user registers (ADR-0090 §1, §2): the list, registering one, changing it, deleting it, giving
// a confidential one a new secret, and issuing tokens of one to the user's own account. A secret and the tokens are in
// the answer of the operation only, so the screen shows them once.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "../../../lib/api/schema.gen";
import { ApiError } from "../../../lib/errors";

export type OAuthApp = components["schemas"]["OAuthApp"];
export type OAuthAppCreated = components["schemas"]["OAuthAppCreated"];
export type OAuthAppToken = components["schemas"]["OAuthAppToken"];
export type OAuthAppClientType = components["schemas"]["OAuthAppClientType"];
export type CreateOAuthAppRequest = components["schemas"]["CreateOAuthAppRequest"];
export type UpdateOAuthAppRequest = components["schemas"]["UpdateOAuthAppRequest"];

/** The limits of the API (CreateOAuthAppRequest). */
export const appNameLimit = 100;
export const appWebsiteLimit = 2048;

/** The user's apps, oldest first (GET /oauth_apps). */
export function oauthAppsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/oauth_apps");
}

/** Registers an app (POST /oauth_apps), as the options of useMutation. The secret of a confidential app is in the answer only. */
export function createOAuthAppMutation(api: Api) {
  return {
    mutationKey: ["post", "/oauth_apps"] as const,
    mutationFn: async (body: CreateOAuthAppRequest): Promise<OAuthAppCreated> => {
      const { data, error } = await api.fetch.POST("/oauth_apps", { body });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Changes an app (PATCH /oauth_apps/{id}), as the options of useMutation. */
export function updateOAuthAppMutation(api: Api) {
  return {
    mutationKey: ["patch", "/oauth_apps/{id}"] as const,
    mutationFn: async ({ id, body }: { id: string; body: UpdateOAuthAppRequest }): Promise<OAuthApp> => {
      const { data, error } = await api.fetch.PATCH("/oauth_apps/{id}", { params: { path: { id } }, body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Deletes an app and revokes its tokens (DELETE /oauth_apps/{id}), as the options of useMutation. */
export function deleteOAuthAppMutation(api: Api) {
  return {
    mutationKey: ["delete", "/oauth_apps/{id}"] as const,
    mutationFn: async (id: string): Promise<string> => {
      const { error } = await api.fetch.DELETE("/oauth_apps/{id}", { params: { path: { id } } });
      if (error !== undefined) throw new ApiError(error);
      return id;
    },
  };
}

/** Gives a confidential app a new secret (POST /oauth_apps/{id}/secret), as the options of useMutation. Returns the secret. */
export function rotateOAuthAppSecretMutation(api: Api) {
  return {
    mutationKey: ["post", "/oauth_apps/{id}/secret"] as const,
    mutationFn: async (id: string): Promise<string> => {
      const { data, error } = await api.fetch.POST("/oauth_apps/{id}/secret", { params: { path: { id } } });
      if (data === undefined) throw new ApiError(error);
      return data.client_secret;
    },
  };
}

/** Issues tokens of an app to the user (POST /oauth_apps/{id}/tokens), as the options of useMutation. */
export function issueOAuthAppTokenMutation(api: Api) {
  return {
    mutationKey: ["post", "/oauth_apps/{id}/tokens"] as const,
    mutationFn: async ({ id, scope }: { id: string; scope: string }): Promise<OAuthAppToken> => {
      const { data, error } = await api.fetch.POST("/oauth_apps/{id}/tokens", {
        params: { path: { id } },
        body: scope === "" ? {} : { scope },
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** What registering, changing or deleting an app changes (web-client/README.md S1, ADR-0045): the list. Resolves once it is fetched again. */
export async function refreshOAuthApps(api: Api, queryClient: QueryClient): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: oauthAppsQueryOptions(api).queryKey });
}
