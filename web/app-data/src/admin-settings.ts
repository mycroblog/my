// SPDX-License-Identifier: AGPL-3.0-only
// The instance's settings as a holder of `manage_settings` changes them (FR-001, FR-705, FR-724, FR-725, FR-922,
// ADR-0058): every setting with its value and what it accepts, saved together, all or none; what switching federation
// to the allowlist would end, and who limiting sign-in to the login providers would keep out, both read before saving;
// the instance's texts (its name, descriptions, terms, privacy policy, commerce disclosure, the alternative texts of its
// images), one value for every locale; its brand images (ADR-0059), checked by the server and drawn by the browser; and
// the translation providers the server has (ADR-0148). Every write reads the instance again, whose public information
// it changes.
import { ProblemError, uploadMedia, type components } from "@mycroblog/mykit";
import { drawSvgPictures } from "@my/platform";
import { useQueries, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { instanceQuery } from "./instance";
import { useAppData } from "./provider";
import { readOf, type ScreenRead } from "./reads";
import { writeCached } from "./written";

/** An instance setting: its value and what it accepts. */
export type AdminSetting = components["schemas"]["AdminSetting"];
/** The key of an instance setting. */
export type AdminSettingKey = components["schemas"]["InstanceSettingKey"];
/** The kind of a setting's value. */
export type AdminSettingKind = components["schemas"]["InstanceSettingKind"];
/** What switching federation to the allowlist would end now. */
export type FederationScopeImpact = components["schemas"]["FederationScopeImpact"];
/** Who limiting sign-in to the login providers keeps out. */
export type LoginProvidersOnlyImpact = components["schemas"]["LoginProvidersOnlyImpact"];
/** The key of an instance text. */
export type InstanceTextKey = components["schemas"]["InstanceTextKey"];
/** A brand image of the instance: the icon, the logo, or the logo for dark backgrounds. */
export type BrandKind = components["schemas"]["BrandKind"];
/** A brand image as the administrators see it. */
export type AdminBrandImage = components["schemas"]["AdminBrandImage"];
/** The brand images of the instance; a kind that uses the built-in images is left out. */
export type AdminBrands = components["schemas"]["AdminBrands"];
/** An SVG as the server would save it, with the renditions to draw from it. */
export type BrandSanitized = components["schemas"]["BrandSanitized"];
/** Something the server removed from an uploaded SVG. */
export type BrandDropped = components["schemas"]["BrandDropped"];
/** Why a brand image cannot be saved. */
export type BrandError = components["schemas"]["BrandError"];
/** A PNG rendition of a brand image and its size. */
export type BrandRenditionSize = components["schemas"]["BrandRenditionSize"];
/** How translation is used in the current month (UTC), and the providers the server has. */
export type TranslationOverview = components["schemas"]["AdminTranslationOverview"];
/** A translation provider the server has. */
export type TranslationProvider = components["schemas"]["TranslationProvider"];

/** The keys of the instance texts, in the order the screens show them. */
export const instanceTextKeys = [
  "name",
  "short_description",
  "description",
  "terms",
  "privacy_policy",
  "commerce_disclosure",
  "preview_image_alt",
  "thumbnail_alt",
] as const satisfies readonly InstanceTextKey[];

/** The most characters of each instance text, as the server counts them. */
export const instanceTextLimits: Readonly<Record<InstanceTextKey, number>> = {
  name: 100,
  short_description: 500,
  description: 20_000,
  terms: 100_000,
  privacy_policy: 100_000,
  commerce_disclosure: 100_000,
  preview_image_alt: 1500,
  thumbnail_alt: 1500,
};

/** Every instance setting. */
export function adminSettingsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/settings");
}

/** The current instance texts. */
export function instanceTextsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/instance_texts");
}

/** The brand images. */
export function adminBrandsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/instance/brand");
}

/** How translation is used this month, and the providers the server has. */
export function translationOverviewQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/translation");
}

/** What switching federation to the allowlist would end now. */
export function federationScopeImpactQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/settings/federation_scope/impact");
}

/** Who limiting sign-in to the login providers keeps out, or would with the provider of `providerId` enabled. */
export function loginProvidersOnlyImpactQuery({ queries }: AppData, providerId?: string) {
  return queries.queryOptions("get", "/admin/settings/login_providers_only/impact", {
    params: { query: providerId === undefined ? {} : { provider_id: providerId } },
  });
}

/** Reads the settings and the texts, for a route's loader; a read that fails is the screen's to say. */
export async function loadAdminSettings(data: AppData): Promise<void> {
  const { queryClient } = data;
  await Promise.all([
    queryClient.query({ ...adminSettingsQuery(data), staleTime: Infinity }),
    queryClient.query({ ...instanceTextsQuery(data), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/** What a screen of the settings reads: the settings by key and the texts by key, once the server answered both. */
export type AdminSettingsRead = ScreenRead & {
  readonly settings: ReadonlyMap<AdminSettingKey, AdminSetting> | undefined;
  readonly texts: Readonly<Partial<Record<InstanceTextKey, string>>> | undefined;
};

/** The settings and the texts, with where their reads are. */
export function useAdminSettings(): AdminSettingsRead {
  const data = useAppData();
  const reads = useQueries({ queries: [adminSettingsQuery(data), instanceTextsQuery(data)] });
  const [settings, texts] = reads;
  return {
    ...readOf(reads),
    settings: settings.data === undefined ? undefined : new Map(settings.data.items.map((s) => [s.key, s] as const)),
    texts:
      texts.data === undefined ? undefined : Object.fromEntries(texts.data.items.map((t) => [t.key, t.body] as const)),
  };
}

/** Reads the instance again, whose public information a write of the settings changes. */
function instanceChanged(data: AppData): void {
  void data.queryClient.invalidateQueries({ queryKey: instanceQuery(data).queryKey });
}

/**
 * Saves `values`, the settings to change by key, all of them or none; the cache then holds every setting as the server
 * answered. A value a key refuses is a 422 naming the key (`refusedField`).
 */
async function saveAdminSettings(data: AppData, values: Readonly<Record<string, string>>): Promise<void> {
  const { data: list } = await data.api.PATCH("/admin/settings", { body: { values } });
  if (list === undefined) throw new Error("app-data: the settings answered without the settings");
  await writeCached(data, adminSettingsQuery(data).queryKey, () => list);
  void data.queryClient.invalidateQueries({
    predicate: ({ queryKey }) =>
      queryKey[1] === "/admin/settings/federation_scope/impact" ||
      queryKey[1] === "/admin/settings/login_providers_only/impact",
  });
  instanceChanged(data);
}

/**
 * Records `body` as the text of `key`, the same in every locale; an empty body clears it. A body the key refuses is a
 * 422 naming `body`.
 */
async function setInstanceText(data: AppData, key: InstanceTextKey, body: string): Promise<void> {
  const { data: text } = await data.api.PUT("/admin/instance_texts/{key}", {
    params: { path: { key } },
    body: { body },
  });
  if (text === undefined) throw new Error("app-data: the text answered without the text");
  await writeCached<components["schemas"]["AdminInstanceTextList"]>(data, instanceTextsQuery(data).queryKey, (now) => ({
    items: [...now.items.filter((t) => t.key !== key), ...(text.body === "" ? [] : [text])],
  }));
  instanceChanged(data);
}

/** Uploads the image of a `media` setting and resolves with the ID the setting takes. */
async function uploadSettingImage(data: AppData, file: File): Promise<string> {
  return (await uploadMedia(data.api, { file })).id;
}

/** The writes of the settings, for a screen. */
export type AdminSettingsWrites = {
  readonly save: (values: Readonly<Record<string, string>>) => Promise<void>;
  readonly setText: (key: InstanceTextKey, body: string) => Promise<void>;
  readonly upload: (file: File) => Promise<string>;
};

/** The writes of the settings (`saveAdminSettings`, `setInstanceText`, `uploadSettingImage`). */
export function useAdminSettingsWrites(): AdminSettingsWrites {
  const data = useAppData();
  return {
    save: (values) => saveAdminSettings(data, values),
    setText: (key, body) => setInstanceText(data, key, body),
    upload: (file) => uploadSettingImage(data, file),
  };
}

/** What switching federation to the allowlist would end, asked only while `enabled` (the reader chose it). */
export function useFederationScopeImpact(enabled: boolean): FederationScopeImpact | undefined {
  return useQuery({ ...federationScopeImpactQuery(useAppData()), enabled }).data;
}

/**
 * Who limiting sign-in to the login providers keeps out, or would with the provider of `providerId` enabled, asked
 * only while `enabled`; and whether the answer failed.
 */
export function useLoginProvidersOnlyImpact(
  enabled: boolean,
  providerId?: string,
): { readonly impact: LoginProvidersOnlyImpact | undefined; readonly failed: boolean } {
  const query = useQuery({ ...loginProvidersOnlyImpactQuery(useAppData(), providerId), enabled });
  return { impact: query.data, failed: query.status === "error" };
}

/** The translation providers the server has, enabled or not; undefined until the server said. */
export function useTranslationProviders(): readonly TranslationProvider[] | undefined {
  return useQuery(translationOverviewQuery(useAppData())).data?.providers;
}

/** How translation is used this month, and where its read is. */
export function useTranslationOverview(): ScreenRead & { readonly overview: TranslationOverview | undefined } {
  const query = useQuery(translationOverviewQuery(useAppData()));
  return { ...readOf([query]), overview: query.data };
}

/** Reads the providers the server has, for a route's loader. */
export async function loadTranslationProviders(data: AppData): Promise<void> {
  await data.queryClient.query({ ...translationOverviewQuery(data), staleTime: Infinity }).catch(() => undefined);
}

// ---- The brand images ------------------------------------------------------------------------------------------------

/** The brand images and where their read is. */
export function useAdminBrands(): ScreenRead & { readonly brands: AdminBrands | undefined } {
  const query = useQuery(adminBrandsQuery(useAppData()));
  return { ...readOf([query]), brands: query.data };
}

/** Reads the brand images, for a route's loader. */
export async function loadAdminBrands(data: AppData): Promise<void> {
  await data.queryClient.query({ ...adminBrandsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** Why the server refused a brand image (`BrandError`); undefined for any other failure. */
export function brandErrorOf(error: unknown): BrandError | undefined {
  if (!(error instanceof ProblemError) || error.type !== "/api/v1/problems/invalid-brand") return undefined;
  const brand = error.problem["brand_error"];
  return typeof brand === "object" && brand !== null && "code" in brand ? (brand as BrandError) : undefined;
}

/** The part of a rendition in the request that sets a brand image: its name with `_` for `-`. */
const partOf = (rendition: string) => rendition.replaceAll("-", "_");

/** Checks and sanitizes an SVG as setting it would, saving nothing; rejects with the server's problem. */
async function sanitizeBrand(data: AppData, kind: BrandKind, svg: Blob): Promise<BrandSanitized> {
  const form = new FormData();
  form.append("kind", kind);
  form.append("svg", svg, "brand.svg");
  const { data: sanitized } = await data.api.POST("/admin/instance/brand/sanitize", {
    // The body sent is the FormData of bodySerializer; the typed body only satisfies the spec's shape.
    body: { kind, svg: "" },
    bodySerializer: () => form,
  });
  if (sanitized === undefined) throw new Error("app-data: the check of the image answered without the image");
  return sanitized;
}

/** The opaque renditions, drawn on the background (ADR-0059 §10). */
const opaque = new Set(["apple-touch-180", "maskable-512", "preview-1200x630"]);

/** The share of a rendition the SVG fills: the middle 80% of a maskable icon, 8 tenths by 5 of a link preview. */
const fitOf = (rendition: string): readonly [number, number] | undefined =>
  rendition === "maskable-512" ? [0.8, 0.8] : rendition === "preview-1200x630" ? [0.8, 0.5] : undefined;

/**
 * Sets the brand image of `kind` to the sanitized SVG: the browser draws every PNG rendition the server asked for from
 * it, the opaque ones on `background` (`#rrggbb`: the icon's own color, which the server keeps with it, or the light
 * surface a logo's link preview is drawn on), and the server checks them all. The cache then
 * holds the image, and the instance, which shows it, is read again. Rejects with the server's problem (`brandErrorOf`),
 * or where the browser cannot draw the SVG.
 */
async function setBrand(
  data: AppData,
  kind: BrandKind,
  sanitized: BrandSanitized,
  background: string,
): Promise<AdminBrandImage> {
  const pictures = await drawSvgPictures(
    sanitized.svg,
    { width: sanitized.view_box_width, height: sanitized.view_box_height },
    sanitized.renditions.map((r) => ({
      width: r.width,
      height: r.height,
      background: opaque.has(r.rendition) ? background : undefined,
      fit: fitOf(r.rendition),
    })),
  );
  const form = new FormData();
  form.append("svg", new Blob([sanitized.svg], { type: "image/svg+xml" }), "brand.svg");
  if (kind === "icon") form.append("background", background);
  sanitized.renditions.forEach((r, i) => {
    const png = pictures[i];
    if (png !== undefined) form.append(partOf(r.rendition), png, `${r.rendition}.png`);
  });
  const { data: image } = await data.api.PUT("/admin/instance/brand/{kind}", {
    params: { path: { kind } },
    // The body sent is the FormData of bodySerializer; the typed body only satisfies the spec's shape.
    body: { svg: "" },
    bodySerializer: () => form,
  });
  if (image === undefined) throw new Error("app-data: the brand image answered without the image");
  await writeCached<AdminBrands>(data, adminBrandsQuery(data).queryKey, (now) => ({ ...now, [kind]: image }));
  instanceChanged(data);
  return image;
}

/** Brings the brand image of `kind` back to the built-in one. */
async function clearBrand(data: AppData, kind: BrandKind): Promise<void> {
  await data.api.DELETE("/admin/instance/brand/{kind}", { params: { path: { kind } } });
  await writeCached<AdminBrands>(data, adminBrandsQuery(data).queryKey, (now) =>
    Object.fromEntries(Object.entries(now).filter(([k]) => k !== kind)),
  );
  instanceChanged(data);
}

/** The writes of the brand images, for a screen. */
export function useBrandWrites(): {
  readonly sanitize: (kind: BrandKind, svg: Blob) => Promise<BrandSanitized>;
  readonly set: (kind: BrandKind, sanitized: BrandSanitized, background: string) => Promise<AdminBrandImage>;
  readonly clear: (kind: BrandKind) => Promise<void>;
} {
  const data = useAppData();
  return {
    sanitize: (kind, svg) => sanitizeBrand(data, kind, svg),
    set: (kind, sanitized, background) => setBrand(data, kind, sanitized, background),
    clear: (kind) => clearBrand(data, kind),
  };
}
