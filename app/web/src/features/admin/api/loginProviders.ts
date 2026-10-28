// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the login providers screen (web-client/README.md S1, ADR-0106, manage_login_providers):
// the external login providers (GET /admin/login_providers), newest first; setting one up, changing, enabling,
// disabling and deleting it, and giving it a new client secret. A provider that breaks a rule is a 422
// `invalid-login-provider` problem whose `reason` names the field. The client secret is sent on setup and rotation
// only and never answered. Every change is recorded in the audit log.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { refreshAuditLog } from "../../moderation";

type Schemas = components["schemas"];

export type LoginProvider = Schemas["AdminLoginProvider"];
export type LoginProviderKind = Schemas["LoginProviderKind"];
export type LoginProviderClaimCondition = Schemas["LoginProviderClaimCondition"];
export type NewLoginProvider = Schemas["CreateLoginProviderRequest"];
export type LoginProviderChange = Schemas["UpdateLoginProviderRequest"];
export type LoginProviderProblemReason = Schemas["LoginProviderProblemReason"];

/** The problem type of a login provider that breaks a rule. */
const invalidLoginProviderProblem = "/api/v1/problems/invalid-login-provider";

/** The limits of the settings (CreateLoginProviderRequest). */
export const loginProviderLimits = {
  displayName: 100,
  issuer: 2048,
  clientId: 512,
  clientSecret: 1024,
  scopes: 20,
  claims: 10,
  claimName: 100,
  claimValues: 50,
  organizations: 50,
  guilds: 50,
} as const;

/** The kinds of provider, in the order of the form (LoginProviderKind). */
export const loginProviderKinds = [
  "oidc",
  "github",
  "discord",
  "mastodon",
] as const satisfies readonly LoginProviderKind[];

/** The login providers that are not deleted, newest first. */
export function loginProvidersQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/login_providers");
}

/** The reason of an `invalid-login-provider` problem, or undefined for another error. */
export function loginProviderProblem(error: unknown): { reason: LoginProviderProblemReason | undefined } | undefined {
  if (!(error instanceof ApiError) || error.type !== invalidLoginProviderProblem) return undefined;
  const reason = error.document["reason"];
  return { reason: typeof reason === "string" ? (reason as LoginProviderProblemReason) : undefined };
}

/** Sets up a login provider (POST /admin/login_providers), as the options of useMutation. */
export function createLoginProviderMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/login_providers"] as const,
    mutationFn: async (body: NewLoginProvider): Promise<LoginProvider> => {
      const { data, error } = await api.fetch.POST("/admin/login_providers", { body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Changes a login provider (PATCH /admin/login_providers/{id}), as the options of useMutation. */
export function updateLoginProviderMutation(api: Api) {
  return {
    mutationKey: ["patch", "/admin/login_providers/{id}"] as const,
    mutationFn: async ({ id, change }: { id: string; change: LoginProviderChange }): Promise<LoginProvider> => {
      const { data, error } = await api.fetch.PATCH("/admin/login_providers/{id}", {
        params: { path: { id } },
        body: change,
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Enables or disables a login provider, as the options of useMutation. */
export function setLoginProviderEnabledMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/login_providers/{id}/enable"] as const,
    mutationFn: async ({ id, enabled }: { id: string; enabled: boolean }): Promise<LoginProvider> => {
      const { data, error } = enabled
        ? await api.fetch.POST("/admin/login_providers/{id}/enable", { params: { path: { id } } })
        : await api.fetch.POST("/admin/login_providers/{id}/disable", { params: { path: { id } } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Deletes a login provider, as the options of useMutation. */
export function deleteLoginProviderMutation(api: Api) {
  return {
    mutationKey: ["delete", "/admin/login_providers/{id}"] as const,
    mutationFn: async (id: string): Promise<void> => {
      // The middleware throws for every error status; a 204 has no body to read.
      await api.fetch.DELETE("/admin/login_providers/{id}", { params: { path: { id } } });
    },
  };
}

/** Gives a login provider the client secret that the provider issued, as the options of useMutation. */
export function rotateLoginProviderSecretMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/login_providers/{id}/rotate_secret"] as const,
    mutationFn: async ({ id, secret }: { id: string; secret: string }): Promise<LoginProvider> => {
      const { data, error } = await api.fetch.POST("/admin/login_providers/{id}/rotate_secret", {
        params: { path: { id } },
        body: { client_secret: secret },
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Marks what a change of a login provider changes (ADR-0045): the list, whose row the answer shows, and the audit
 * log; the queries shown now are fetched again and awaited.
 */
export async function refreshLoginProviders(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/login_providers"] }),
    refreshAuditLog(queryClient),
  ]);
}
