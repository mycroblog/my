// SPDX-License-Identifier: AGPL-3.0-only
// The translation screen's data (web-client/screens/admin-translation.md, manage_settings): the use of the current
// month and the providers this server has (GET /admin/translation). Its settings are instance settings
// (api/settings.ts). A setting's value is one string, which the functions here read and write.
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";

type Schemas = components["schemas"];

export type TranslationOverview = Schemas["AdminTranslationOverview"];
export type TranslationProvider = Schemas["TranslationProvider"];
export type Visibility = Schemas["Visibility"];

/** Every visibility, in the order the server stores and the form shows them. */
export const translationVisibilities = [
  "public",
  "local",
  "unlisted",
  "private",
  "direct",
] as const satisfies readonly Visibility[];

/** What a provider that translation_provider_visibilities does not name may receive (ADR-0148 §5). */
const defaultVisibilities: readonly Visibility[] = ["public", "local", "unlisted"];

/** The use of the current month and the providers this server has. */
export function translationOverviewQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/translation");
}

/** The IDs of translation_providers, the first tried first. */
export function parseProviderList(value: string): string[] {
  return value
    .split(",")
    .map((id) => id.trim())
    .filter((id) => id !== "");
}

/** The pairs of translation_provider_visibilities, by provider. A provider not named is absent. */
export function parseProviderVisibilities(value: string): ReadonlyMap<string, readonly Visibility[]> {
  const out = new Map<string, readonly Visibility[]>();
  for (const pair of parseProviderList(value)) {
    const [id = "", list = ""] = pair.split("=");
    const named = list.split("|").map((name) => name.trim());
    out.set(
      id.trim(),
      translationVisibilities.filter((visibility) => named.includes(visibility)),
    );
  }
  return out;
}

/** The visibilities that a provider may receive under a value of translation_provider_visibilities. */
export function visibilitiesOf(value: string, provider: string): readonly Visibility[] {
  return parseProviderVisibilities(value).get(provider) ?? defaultVisibilities;
}

/**
 * The value of translation_provider_visibilities with what a provider may receive changed. A provider left at the
 * default stops being named, and the other pairs stay, those of a provider this server does not have too.
 */
export function withVisibilities(value: string, provider: string, visibilities: readonly Visibility[]): string {
  const pairs = new Map(parseProviderVisibilities(value));
  const atDefault =
    visibilities.length === defaultVisibilities.length && defaultVisibilities.every((v) => visibilities.includes(v));
  if (atDefault) pairs.delete(provider);
  else pairs.set(provider, visibilities);
  return [...pairs.keys()]
    .toSorted()
    .map((id) => {
      const allowed = pairs.get(id) ?? [];
      return `${id}=${translationVisibilities.filter((v) => allowed.includes(v)).join("|")}`;
    })
    .join(",");
}
