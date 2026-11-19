// SPDX-License-Identifier: AGPL-3.0-only
// The external login providers as a holder of `manage_login_providers` keeps them (FR-010, ADR-0106): the providers
// not deleted, newest first; setting one up (enabled at once, its client secret given once and never shown), changing
// one, enabling and disabling it, giving it a new secret, and deleting it for good. Whether limiting sign-in to the
// providers would start at once is the impact of `login_providers_only` (admin-settings.ts), asked before. Every write
// reads the list again, and the instance, whose sign-in it changes.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { instanceQuery } from "./instance";
import { useAppData } from "./provider";
import { readOf, type ScreenRead } from "./reads";
import { removeCached } from "./written";

/** An external login provider, as the administrators see it. */
export type AdminLoginProvider = components["schemas"]["AdminLoginProvider"];
/** The kind of an external login provider. */
export type LoginProviderKind = components["schemas"]["LoginProviderKind"];
/** A new login provider. */
export type CreateLoginProvider = components["schemas"]["CreateLoginProviderRequest"];
/** The changes to a login provider. */
export type UpdateLoginProvider = components["schemas"]["UpdateLoginProviderRequest"];
/** A condition on a claim of an OpenID Connect provider's ID token. */
export type LoginProviderClaimCondition = components["schemas"]["LoginProviderClaimCondition"];
/** Why the server refused a login provider. */
export type LoginProviderRefusal = components["schemas"]["LoginProviderProblemReason"];

/** The kinds of a login provider, in the order a form offers them. */
export const loginProviderKinds = [
  "oidc",
  "github",
  "mastodon",
  "discord",
] as const satisfies readonly LoginProviderKind[];

/** The most of a provider's lists, as the server takes them. */
export const loginProviderLimits = {
  displayName: 100,
  scopes: 20,
  claims: 10,
  claimValues: 50,
  organizations: 50,
  guilds: 50,
} as const;

/** The providers, newest first. */
export function adminLoginProvidersQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/login_providers");
}

/** Reads the providers, for a route's loader. */
export async function loadAdminLoginProviders(data: AppData): Promise<void> {
  await data.queryClient.query({ ...adminLoginProvidersQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The providers and where their read is. */
export function useAdminLoginProviders(): ScreenRead & {
  readonly providers: readonly AdminLoginProvider[] | undefined;
} {
  const query = useQuery(adminLoginProvidersQuery(useAppData()));
  return { ...readOf([query]), providers: query.data?.items };
}

/** The provider of `id`, found in the list; `missing` once the list is read without it. */
export function useAdminLoginProvider(
  id: string,
): ScreenRead & { readonly provider: AdminLoginProvider | undefined; missing: boolean } {
  const query = useQuery(adminLoginProvidersQuery(useAppData()));
  const provider = query.data?.items.find((p) => p.id === id);
  return { ...readOf([query]), provider, missing: query.data !== undefined && provider === undefined };
}

/** Why the server refused a provider (`reason` of `/api/v1/problems/invalid-login-provider`); undefined otherwise. */
export function loginProviderRefusalOf(error: unknown): LoginProviderRefusal | undefined {
  if (!(error instanceof ProblemError) || error.type !== "/api/v1/problems/invalid-login-provider") return undefined;
  const reason = error.problem["reason"];
  return typeof reason === "string" ? (reason as LoginProviderRefusal) : undefined;
}

/** The writes of the providers; each rejects with the server's problem (`loginProviderRefusalOf`). */
export type AdminLoginProviderWrites = {
  readonly create: (request: CreateLoginProvider) => Promise<AdminLoginProvider>;
  readonly update: (id: string, changes: UpdateLoginProvider) => Promise<AdminLoginProvider>;
  readonly enable: (id: string) => Promise<AdminLoginProvider>;
  readonly disable: (id: string) => Promise<AdminLoginProvider>;
  /** Replaces the client secret with `secret`, which the provider issued. */
  readonly rotate: (id: string, secret: string) => Promise<AdminLoginProvider>;
  /** Deletes the provider for good. */
  readonly remove: (id: string) => Promise<void>;
};

/** The writes of the providers, for a screen. */
export function useAdminLoginProviderWrites(): AdminLoginProviderWrites {
  const data = useAppData();
  const key = adminLoginProvidersQuery(data).queryKey;
  /** The list, the instance and the impact of the limit are read again: a write changes who signs in with what. */
  const changed = async () => {
    void data.queryClient.invalidateQueries({ queryKey: instanceQuery(data).queryKey });
    void data.queryClient.invalidateQueries({
      predicate: ({ queryKey }) => queryKey[1] === "/admin/settings/login_providers_only/impact",
    });
    await data.queryClient.invalidateQueries({ queryKey: key });
  };
  const written = async (provider: AdminLoginProvider | undefined): Promise<AdminLoginProvider> => {
    if (provider === undefined) throw new Error("app-data: a write of a login provider answered without it");
    await changed();
    return provider;
  };
  const path = (id: string) => ({ params: { path: { id } } });
  return {
    create: async (request) => written((await data.api.POST("/admin/login_providers", { body: request })).data),
    update: async (id, changes) =>
      written((await data.api.PATCH("/admin/login_providers/{id}", { ...path(id), body: changes })).data),
    enable: async (id) => written((await data.api.POST("/admin/login_providers/{id}/enable", path(id))).data),
    disable: async (id) => written((await data.api.POST("/admin/login_providers/{id}/disable", path(id))).data),
    rotate: async (id, secret) =>
      written(
        (
          await data.api.POST("/admin/login_providers/{id}/rotate_secret", {
            ...path(id),
            body: { client_secret: secret },
          })
        ).data,
      ),
    remove: async (id) => {
      await removeCached<components["schemas"]["AdminLoginProviderList"]>(
        data,
        key,
        data.api.DELETE("/admin/login_providers/{id}", path(id)),
        (now) => ({ items: now.items.filter((p) => p.id !== id) }),
      );
      void data.queryClient.invalidateQueries({ queryKey: instanceQuery(data).queryKey });
    },
  };
}
