// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the server settings screen (web-client/README.md S1): the instance's texts (its display
// name and its descriptions, PUT /admin/instance_texts/{key}), one value each that every locale shows, and the
// instance settings (GET and PATCH /admin/settings): the registration mode, the default language, the public
// timelines, the federation and the publication of the domain actions, all for manage_settings. GET /instance
// reads the name, the descriptions, the registration mode and the timelines. The texts and the instance are read
// with the queries of api/documents.ts.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { refreshAuditLog } from "../../moderation";
import { cacheSavedDocument, type InstanceText } from "./documents";

type Schemas = components["schemas"];

export type InstanceSettingKey = Schemas["InstanceSettingKey"];
export type InstanceSettingList = Schemas["AdminSettingList"];

/** The most characters of the instance's name (InstanceTextKey `name`), counted in code points. */
export const instanceNameLimit = 100;

/** The most characters of the short and full descriptions (internal/domain/instance, text.go). */
export const shortDescriptionLimit = 500;
export const descriptionLimit = 20_000;

/** The most characters of the thumbnail's alt text (internal/domain/instance, text.go). */
export const thumbnailAltLimit = 1500;

/** The texts of the server settings screen. */
type SettingsTextKey = Extract<
  Schemas["InstanceTextKey"],
  "name" | "short_description" | "description" | "thumbnail_alt"
>;

/** A new value of a text; an empty `body` clears it. */
export type InstanceTextChange = { key: SettingsTextKey; body: string };

/**
 * Sets each text of the changes (one audited change each), as the options of useMutation. Returns the texts as
 * recorded.
 */
export function setInstanceTextsMutation(api: Api) {
  return {
    mutationKey: ["put", "/admin/instance_texts/{key}"] as const,
    mutationFn: (changes: readonly InstanceTextChange[]): Promise<InstanceText[]> =>
      Promise.all(
        changes.map(async ({ key, body }) => {
          const { data, error } = await api.fetch.PUT("/admin/instance_texts/{key}", {
            params: { path: { key } },
            body: { body },
          });
          // The middleware throws an ApiError for every error status; this narrows the type.
          if (data === undefined) throw new ApiError(error);
          return data;
        }),
      ),
  };
}

/**
 * Marks what new texts change (ADR-0045): the texts, GET /instance (the navigation, the tab title, the sign-in page
 * and the about page show the name and the descriptions) and the audit log, the same queries as a new document.
 */
export function cacheSavedTexts(queryClient: QueryClient): Promise<void> {
  return cacheSavedDocument(queryClient);
}

/** Every instance setting with what it accepts (GET /admin/settings). */
export function instanceSettingsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/settings");
}

/**
 * Sets some instance settings together, all or none (PATCH /admin/settings, one audited change each), as the
 * options of useMutation. Returns every setting as it is now.
 */
export function updateInstanceSettingsMutation(api: Api) {
  return {
    mutationKey: ["patch", "/admin/settings"] as const,
    mutationFn: async (values: Readonly<Partial<Record<InstanceSettingKey, string>>>): Promise<InstanceSettingList> => {
      const { data, error } = await api.fetch.PATCH("/admin/settings", { body: { values } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Writes the settings from the answer of a change (ADR-0045), and marks what they change: GET /instance (the
 * registration mode and the timelines that the navigation offers) and the audit log, fetched again and awaited
 * while shown.
 */
export async function cacheSavedSettings(
  queryClient: QueryClient,
  api: Api,
  settings: InstanceSettingList,
): Promise<void> {
  queryClient.setQueryData(instanceSettingsQueryOptions(api).queryKey, settings);
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/instance"] }),
    // What finance_visibility publishes (FR-903), and what the finance_viewer_* settings show (ADR-0070 §5).
    queryClient.invalidateQueries({ queryKey: ["get", "/instance/cost_totals"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/finance/cost_totals"] }),
    // What income_visibility publishes, and with it the balance and the plans of every income total (ADR-0073 §7).
    queryClient.invalidateQueries({ queryKey: ["get", "/instance/income_totals"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/finance/income_totals"] }),
    // Who may read the supporter list (ADR-0071 §4).
    queryClient.invalidateQueries({ queryKey: ["get", "/instance/supporters"] }),
    // What the landing page shows of the counts and the operators (public_counts, public_operators).
    queryClient.invalidateQueries({ queryKey: ["get", "/instance/overview"] }),
    refreshAuditLog(queryClient),
  ]);
}

/** A change of the server's contact and thumbnail: the settings that changed, and the alt text when it changed. */
export type ContactChange = {
  values: Readonly<Partial<Record<InstanceSettingKey, string>>>;
  texts: readonly InstanceTextChange[];
};

/**
 * Saves the server's contact and thumbnail, as the options of useMutation: the settings together, all or none
 * (PATCH /admin/settings), then the alt text when it changed (PUT /admin/instance_texts/thumbnail_alt). Returns
 * every setting as it is now, or undefined when no setting changed.
 */
export function saveContactMutation(api: Api) {
  const setTexts = setInstanceTextsMutation(api).mutationFn;
  return {
    mutationKey: ["patch", "/admin/settings", "contact"] as const,
    mutationFn: async ({ values, texts }: ContactChange): Promise<InstanceSettingList | undefined> => {
      let settings: InstanceSettingList | undefined;
      if (Object.keys(values).length > 0) {
        const { data, error } = await api.fetch.PATCH("/admin/settings", { body: { values } });
        // The middleware throws an ApiError for every error status; this narrows the type.
        if (data === undefined) throw new ApiError(error);
        settings = data;
      }
      if (texts.length > 0) await setTexts(texts);
      return settings;
    },
  };
}
