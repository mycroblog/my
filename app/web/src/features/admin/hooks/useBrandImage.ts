// SPDX-License-Identifier: AGPL-3.0-only
// One of the server's own brand images (ADR-0116 §3): the icon, the logo, or the logo for dark backgrounds. Choosing an
// SVG sends it to the server to check and sanitize (nothing is saved), draws every PNG rendition the server asks for
// from the sanitized SVG in this browser, and previews them; saving sends the SVG and the PNGs together, and the
// built-in image comes back when the image is cleared. What the server refuses shows under the file's button, with
// how to fix it; the result of saving and clearing shows next to the buttons (FormActionBar).
import { type FormActionResult, useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useEffect, useRef, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  type AdminBrandImage,
  type BrandDropped,
  type BrandError,
  type BrandKind,
  brandErrorOf,
  brandsQueryOptions,
  brandSvgLimit,
  type BrandSanitized,
  cacheSavedBrand,
  clearBrandMutation,
  sanitizeBrandMutation,
  setBrandMutation,
} from "../api/brand";
import {
  type DrawnRendition,
  drawRenditions,
  hexColor,
  lightSurfaceColor,
  RasterizeError,
  revokeRenditions,
} from "./brandRenditions";

/** An SVG chosen and checked, with its renditions drawn, before it is saved. */
export type BrandDraft = {
  /** The sanitized SVG, as the server will save it. */
  sanitized: BrandSanitized;
  /** A blob: URL of the sanitized SVG, for its preview. */
  svgUrl: string;
  /** The PNG renditions, drawn on the background below. */
  renditions: readonly DrawnRendition[];
  /** The background the opaque renditions were drawn on. */
  background: string;
};

/** Why the chosen file cannot be used, in words that say how to fix it. */
const brandErrorMessages: Record<BrandError["code"], ClientMessageKey> = {
  malformed: "client:admin.brand.error.malformed",
  doctype: "client:admin.brand.error.doctype",
  not_svg: "client:admin.brand.error.not_svg",
  no_viewbox: "client:admin.brand.error.no_viewbox",
  too_large: "client:admin.brand.error.too_large",
  too_many_elements: "client:admin.brand.error.too_complex",
  too_deep: "client:admin.brand.error.too_complex",
  too_many_uses: "client:admin.brand.error.too_complex",
  aspect_ratio: "client:admin.brand.error.aspect_ratio.logo",
  text: "client:admin.brand.error.text",
  unsafe_css: "client:admin.brand.error.unsafe_css",
  missing_rendition: "client:admin.brand.error.rendition",
  unexpected_rendition: "client:admin.brand.error.rendition",
  invalid_png: "client:admin.brand.error.rendition",
  dimensions: "client:admin.brand.error.rendition",
  not_opaque: "client:admin.brand.error.rendition",
  invalid_color: "client:admin.brand.error.invalid_color",
};

/** The aspect ratio a kind needs, for the message of `aspect_ratio`. */
const aspectMessages: Record<BrandKind, ClientMessageKey> = {
  icon: "client:admin.brand.error.aspect_ratio.icon",
  logo: "client:admin.brand.error.aspect_ratio.logo",
  logo_dark: "client:admin.brand.error.aspect_ratio.logo",
};

const savedMessages: Record<BrandKind, ClientMessageKey> = {
  icon: "client:admin.brand.icon.saved",
  logo: "client:admin.brand.logo.saved",
  logo_dark: "client:admin.brand.logo_dark.saved",
};

const clearedMessages: Record<BrandKind, ClientMessageKey> = {
  icon: "client:admin.brand.icon.cleared",
  logo: "client:admin.brand.logo.cleared",
  logo_dark: "client:admin.brand.logo_dark.cleared",
};

/** The chosen file is over the size the server takes; checked here before it is sent. */
class FileTooLargeError extends Error {
  override name = "FileTooLargeError";
}

export function useBrandImage(kind: BrandKind) {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const notice = useNotice();
  const { data: brands } = useSuspenseQuery(brandsQueryOptions(api));
  const saved: AdminBrandImage | undefined = brands[kind];

  const [draft, setDraft] = useState<BrandDraft>();
  const [backgroundDraft, setBackgroundDraft] = useState<string>();
  const [defaultBackground] = useState(lightSurfaceColor);
  const [result, setResult] = useState<FormActionResult>();
  // The blob: URLs on show, released when they are replaced and when the form goes away.
  const shown = useRef<BrandDraft | undefined>(undefined);
  useEffect(
    () => () => {
      if (shown.current !== undefined) release(shown.current);
    },
    [],
  );

  const background = backgroundDraft ?? saved?.background ?? defaultBackground;
  const backgroundValid = hexColor.test(background);

  const show = (next: BrandDraft | undefined) => {
    if (shown.current !== undefined) release(shown.current);
    shown.current = next;
    setDraft(next);
  };

  const prepare = useMutation({
    mutationFn: async ({ file, on }: { file: Blob; on: string }): Promise<BrandDraft> => {
      if (file.size > brandSvgLimit) throw new FileTooLargeError();
      const sanitized = await sanitizeBrandMutation(api).mutationFn({ kind, file });
      const viewBox = { width: sanitized.view_box_width, height: sanitized.view_box_height };
      const renditions = await drawRenditions(sanitized.svg, viewBox, sanitized.renditions, on);
      const svgUrl = URL.createObjectURL(new Blob([sanitized.svg], { type: "image/svg+xml" }));
      return { sanitized, svgUrl, renditions, background: on };
    },
    onMutate: () => {
      setResult(undefined);
    },
    onSuccess: (next) => {
      show(next);
      notice.inPlace(
        next.sanitized.dropped.length === 0
          ? t("client:admin.brand.checked")
          : t("client:admin.brand.checked_dropped", { count: next.sanitized.dropped.length }),
      );
    },
  });

  // A new background draws the opaque renditions again from the same SVG.
  const redraw = useMutation({
    mutationFn: async ({ from, on }: { from: BrandDraft; on: string }): Promise<BrandDraft> => {
      const { sanitized } = from;
      const viewBox = { width: sanitized.view_box_width, height: sanitized.view_box_height };
      const renditions = await drawRenditions(sanitized.svg, viewBox, sanitized.renditions, on);
      const svgUrl = URL.createObjectURL(new Blob([sanitized.svg], { type: "image/svg+xml" }));
      return { sanitized, svgUrl, renditions, background: on };
    },
    onSuccess: (next) => {
      show(next);
    },
  });

  const save = useMutation({
    ...setBrandMutation(api),
    onMutate: () => {
      setResult(undefined);
    },
    onSuccess: async (image) => {
      // The header, the landing page, and the server's pages show the new image once GET /instance is fetched again
      // (the button stays pending until then).
      await cacheSavedBrand(queryClient, api, kind, image);
      show(undefined);
      setBackgroundDraft(undefined);
      setResult({ tone: "success", message: t(savedMessages[kind]) });
    },
    onError: (error) => {
      // A refused file shows under its button; anything else next to the buttons.
      if (brandErrorOf(error) === undefined) {
        setResult({ tone: "danger", message: t("client:admin.brand.save_failed", { reason: errorMessage(error) }) });
      }
    },
  });

  const clear = useMutation({
    ...clearBrandMutation(api),
    onMutate: () => {
      setResult(undefined);
    },
    onSuccess: async () => {
      await cacheSavedBrand(queryClient, api, kind, undefined);
      show(undefined);
      setBackgroundDraft(undefined);
      setResult({ tone: "success", message: t(clearedMessages[kind]) });
    },
    onError: (error) => {
      setResult({ tone: "danger", message: t("client:admin.brand.clear_failed", { reason: errorMessage(error) }) });
    },
  });

  /** The message under the file's button: why the file cannot be used, or that one is needed to save. */
  const fileErrorOf = (error: unknown): string | undefined => {
    if (error === null || error === undefined) return undefined;
    if (error instanceof FileTooLargeError) return t("client:admin.brand.error.too_large");
    if (error instanceof RasterizeError) return t("client:admin.brand.error.draw");
    const brandError = brandErrorOf(error);
    if (brandError === undefined) return t("client:admin.brand.check_failed", { reason: errorMessage(error) });
    return brandError.code === "aspect_ratio" ? t(aspectMessages[kind]) : t(brandErrorMessages[brandError.code]);
  };
  const [missingFile, setMissingFile] = useState(false);
  const fileError = missingFile
    ? t("client:admin.brand.error.no_file")
    : (fileErrorOf(prepare.error) ??
      fileErrorOf(redraw.error) ??
      (brandErrorOf(save.error) === undefined ? undefined : fileErrorOf(save.error)));

  const busy = prepare.isPending || redraw.isPending || save.isPending || clear.isPending;

  return {
    kind,
    /** The image saved now, or undefined while the built-in one is in use. */
    saved,
    /** The SVG chosen and its renditions, before saving. */
    draft,
    /** What the server removed from the chosen SVG. */
    dropped: draft?.sanitized.dropped ?? ([] as readonly BrandDropped[]),
    choose: (file: File) => {
      setMissingFile(false);
      save.reset();
      prepare.mutate({ file, on: backgroundValid ? background : defaultBackground });
    },
    isChecking: prepare.isPending || redraw.isPending,
    /** Why the chosen file cannot be used, localized; undefined when it can. */
    fileError,
    /** The color behind the icon's opaque renditions; only the icon has one. */
    background: {
      value: background,
      onChange: (value: string) => {
        setBackgroundDraft(value);
        setResult(undefined);
        const color = value.trim().toLowerCase();
        if (draft !== undefined && hexColor.test(color) && color !== draft.background && !redraw.isPending) {
          redraw.mutate({ from: draft, on: color });
        }
      },
      error: backgroundValid ? undefined : t("client:admin.brand.background.invalid"),
    },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (busy) return;
      if (draft === undefined) {
        setMissingFile(true);
        return;
      }
      if (kind === "icon" && !backgroundValid) return;
      save.mutate({
        kind,
        svg: new File([draft.sanitized.svg], `${kind}.svg`, { type: "image/svg+xml" }),
        renditions: draft.renditions.map(({ rendition, png }) => ({ rendition, png })),
        ...(kind === "icon" && { background: draft.background }),
      });
    },
    isSaving: save.isPending,
    clear: () => {
      if (busy) return;
      clear.mutate(kind);
    },
    isClearing: clear.isPending,
    /** The result of the last save or clear, for the FormActionBar. */
    result,
  };
}

function release(draft: BrandDraft) {
  URL.revokeObjectURL(draft.svgUrl);
  revokeRenditions(draft.renditions);
}
