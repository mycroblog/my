// SPDX-License-Identifier: AGPL-3.0-only
// The queries and the mutation of the theme screen (web-client/README.md S1, ADR-0059): the instance theme
// (GET /admin/theme) and replacing it (PUT /admin/theme, manage_settings). A theme that breaks a rule is a 422
// `invalid-theme` problem whose `theme_errors` say why, one by one.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { refreshAuditLog } from "../../moderation";

type Schemas = components["schemas"];

export type AdminTheme = Schemas["AdminTheme"];
export type ThemeTokenValue = Schemas["ThemeTokenValue"];
export type ThemeError = Schemas["ThemeError"];

/** The problem type of a theme that breaks a rule. */
const invalidThemeType = "/api/v1/problems/invalid-theme";

/** The most bytes of the custom CSS (internal/domain/instance, theme.go). */
export const customCssLimit = 64 * 1024;

/** The instance theme as its administrators edit it. */
export function themeQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/theme");
}

/** Replaces the theme, as the options of useMutation. Returns the theme as recorded. */
export function setThemeMutation(api: Api) {
  return {
    mutationKey: ["put", "/admin/theme"] as const,
    mutationFn: async (body: Schemas["SetThemeRequest"]): Promise<AdminTheme> => {
      const { data, error } = await api.fetch.PUT("/admin/theme", { body });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** The reasons of an `invalid-theme` problem, or undefined for any other error. */
export function themeErrorsOf(error: unknown): readonly ThemeError[] | undefined {
  if (!(error instanceof ApiError) || error.type !== invalidThemeType) return undefined;
  const errors = error.document["theme_errors"];
  return Array.isArray(errors) ? (errors as ThemeError[]) : [];
}

/**
 * Marks what a new theme changes (ADR-0045): the theme, GET /instance (its `theme`), and the audit log, which
 * records the change. Pages load the theme's stylesheet from the HTML shell, so the open page keeps its look
 * until it is loaded again.
 */
export async function cacheSavedTheme(queryClient: QueryClient, api: Api, theme: AdminTheme): Promise<void> {
  queryClient.setQueryData(themeQueryOptions(api).queryKey, theme);
  await Promise.all([queryClient.invalidateQueries({ queryKey: ["get", "/instance"] }), refreshAuditLog(queryClient)]);
}
