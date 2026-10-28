// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the server's own icon and logos (ADR-0116, manage_settings): the images as the
// administrators see them (GET /admin/instance/brand), checking an SVG without saving it (POST .../sanitize), setting
// one with its PNG renditions (PUT .../{kind}), and going back to the built-in image (DELETE .../{kind}). An image that
// breaks a rule is a 422 `invalid-brand` problem whose `brand_error` says why.
import type { QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { refreshAuditLog } from "../../moderation";

type Schemas = components["schemas"];

export type BrandKind = Schemas["BrandKind"];
export type BrandRendition = Schemas["BrandRendition"];
export type BrandRenditionSize = Schemas["BrandRenditionSize"];
export type BrandSanitized = Schemas["BrandSanitized"];
export type BrandDropped = Schemas["BrandDropped"];
export type BrandError = Schemas["BrandError"];
type AdminBrands = Schemas["AdminBrands"];
export type AdminBrandImage = Schemas["AdminBrandImage"];

/** The problem type of a brand image that breaks a rule. */
const invalidBrandType = "/api/v1/problems/invalid-brand";

/** The most bytes of an uploaded SVG (internal/domain/instance, brand.go). */
export const brandSvgLimit = 256 * 1024;

/** The brand images as their administrators see them. */
export function brandsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/instance/brand");
}

/** Checks and sanitizes an SVG for `kind` without saving it, as the options of useMutation. */
export function sanitizeBrandMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/instance/brand/sanitize"] as const,
    mutationFn: async ({ kind, file }: { kind: BrandKind; file: Blob }): Promise<BrandSanitized> => {
      const { data, error } = await api.fetch.POST("/admin/instance/brand/sanitize", {
        // The generated type of a binary property is string; the serializer sends the file itself.
        body: { kind, svg: "" },
        bodySerializer: () => {
          const form = new FormData();
          form.append("kind", kind);
          form.append("svg", file);
          return form;
        },
      });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** A brand image to set: the sanitized SVG, its PNG renditions, and the background of an icon. */
export type BrandUpload = {
  kind: BrandKind;
  svg: Blob;
  renditions: readonly { rendition: BrandRendition; png: Blob }[];
  /** The `#rrggbb` color behind the icon's opaque renditions; only for the icon. */
  background?: string | undefined;
};

/** Sets a brand image, as the options of useMutation. Returns the image as recorded. */
export function setBrandMutation(api: Api) {
  return {
    mutationKey: ["put", "/admin/instance/brand/{kind}"] as const,
    mutationFn: async (upload: BrandUpload): Promise<AdminBrandImage> => {
      const { data, error } = await api.fetch.PUT("/admin/instance/brand/{kind}", {
        params: { path: { kind: upload.kind } },
        body: { svg: "" },
        bodySerializer: () => {
          const form = new FormData();
          form.append("svg", upload.svg);
          if (upload.background !== undefined) form.append("background", upload.background);
          // Each rendition goes in the part named after it with `_` for `-` (SetBrandRequest).
          for (const { rendition, png } of upload.renditions) {
            form.append(rendition.replaceAll("-", "_"), png);
          }
          return form;
        },
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Brings a brand image back to the built-in one, as the options of useMutation. */
export function clearBrandMutation(api: Api) {
  return {
    mutationKey: ["delete", "/admin/instance/brand/{kind}"] as const,
    mutationFn: async (kind: BrandKind): Promise<void> => {
      await api.fetch.DELETE("/admin/instance/brand/{kind}", { params: { path: { kind } } });
    },
  };
}

/** Why an `invalid-brand` problem refused the image, or undefined for any other error. */
export function brandErrorOf(error: unknown): BrandError | undefined {
  if (!(error instanceof ApiError) || error.type !== invalidBrandType) return undefined;
  const brandError = error.document["brand_error"];
  if (typeof brandError !== "object" || brandError === null || !("code" in brandError)) return undefined;
  return brandError as BrandError;
}

/**
 * Marks what a change of a brand image changes (ADR-0045): the admin's brand images, GET /instance (its `icon`, `logo`,
 * and `logo_dark`, which the app's header and the landing page show), and the audit log, which records the change.
 */
export async function cacheSavedBrand(
  queryClient: QueryClient,
  api: Api,
  kind: BrandKind,
  image: AdminBrandImage | undefined,
): Promise<void> {
  queryClient.setQueryData(brandsQueryOptions(api).queryKey, (brands: AdminBrands | undefined) => {
    const { [kind]: _, ...rest }: AdminBrands = brands ?? {};
    return image === undefined ? rest : { ...rest, [kind]: image };
  });
  await Promise.all([queryClient.invalidateQueries({ queryKey: ["get", "/instance"] }), refreshAuditLog(queryClient)]);
}
