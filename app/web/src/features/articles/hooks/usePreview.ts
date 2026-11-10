// SPDX-License-Identifier: AGPL-3.0-only
// The preview of the body (ADR-0160 §4): the server renders it as publishing would store it (POST /articles/preview),
// so the preview is the published article's HTML. It is asked for once the text has stayed the same for a moment, and
// again when an image of the body finishes processing (the server shows an image only once it is ready). The last
// rendering stays on screen while the next one is on its way.
import { serverSanitizedHtml, type SanitizedHtml } from "@my/ui";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { articlePreviewQueryOptions, type ArticlePreviewRequest } from "../api/articles";

/** How long the text stays the same before it is rendered, in ms. */
const previewDelay = 500;

/** The preview: nothing to render yet, the first rendering on its way, a failure, or the rendered body. */
export type Preview =
  | { status: "empty" }
  | { status: "loading" }
  /** `message` is localized by the server or the network layer. */
  | { status: "error"; message: string }
  | {
      status: "ready";
      html: SanitizedHtml;
      summary: string;
      summaryFromBody: boolean;
      bytes: number;
      maxBytes: number;
      images: number;
      maxImages: number;
      /** Whether publishing would refuse the body: over the bytes or the images of the instance. */
      isOverLimit: boolean;
      /** Whether a newer rendering is on its way. */
      isUpdating: boolean;
    };

/** `value` once it has stayed the same for `delay` ms; the timer is the external system (R8). */
function useSettled(value: string, delay: number): string {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => {
      setSettled(value);
    }, delay);
    return () => {
      clearTimeout(timer);
    };
  }, [value, delay]);
  return settled;
}

/** The request of a settled preview, kept as its JSON so that it settles as one value. */
function requestOf(json: string): ArticlePreviewRequest {
  const [text, summary] = JSON.parse(json) as [string, string];
  return { text, summary };
}

/**
 * The preview of the body `text` with the summary the author wrote; `images` names the uploads of the body that are
 * ready, so that the body is rendered again once one is.
 */
export function usePreview(text: string, summary: string, images: string): Preview {
  const api = useApi();
  const settled = requestOf(useSettled(JSON.stringify([text, summary.trim()]), previewDelay));
  const query = useQuery({
    ...articlePreviewQueryOptions(api, settled, images),
    enabled: settled.text.trim() !== "",
    placeholderData: keepPreviousData,
  });
  if (text.trim() === "") return { status: "empty" };
  if (query.isError) return { status: "error", message: errorMessage(query.error) };
  const data = query.data;
  if (data === undefined) return { status: "loading" };
  return {
    status: "ready",
    html: serverSanitizedHtml(data.content),
    summary: data.summary,
    summaryFromBody: data.summary_from_body,
    bytes: data.bytes,
    maxBytes: data.max_bytes,
    images: data.images,
    maxImages: data.max_images,
    isOverLimit: data.bytes > data.max_bytes || data.images > data.max_images,
    isUpdating: query.isPlaceholderData || query.isFetching || settled.text !== text,
  };
}
