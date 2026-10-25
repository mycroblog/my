// SPDX-License-Identifier: AGPL-3.0-only
// Uploading the images of a profile (the avatar and the header), and of the custom emoji and the server's thumbnail
// that the admin screens set: the server accepts a file, processes it in the background, and is polled until the file is ready
// (api/openapi.yaml, uploadMedia and getMedia).
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";

export type MediaAttachment = components["schemas"]["MediaAttachment"];

/** The image types the server accepts (uploadMedia). */
export const acceptedImageTypes = ["image/jpeg", "image/png", "image/gif", "image/webp"] as const;

/** How long to wait between two polls of a file still processing, and how many polls to make at most. */
const pollInterval = 1000;
const maxPolls = 60;

/** The server could not process the image, or did not finish in time. */
export class MediaProcessingError extends Error {
  override name = "MediaProcessingError";
}

/** What an image is for, which picks the sizes the server makes of it (MediaUpload.purpose). */
export type ImagePurpose = "avatar" | "header" | "emoji" | "attachment";

/** Uploads `file` as a `purpose` image and resolves with it once the server has processed it. */
export async function uploadImage(api: Api, file: File, purpose: ImagePurpose, signal?: AbortSignal): Promise<MediaAttachment> {
  const { data: uploaded } = await api.fetch.POST("/media", {
    // The generated type of a binary property is string; the serializer sends the File itself.
    body: { file: "", purpose },
    bodySerializer: () => {
      const form = new FormData();
      form.append("file", file);
      form.append("purpose", purpose);
      return form;
    },
    ...(signal === undefined ? {} : { signal }),
  });
  let media = uploaded;
  for (let poll = 0; media?.state === "processing"; poll++) {
    if (poll >= maxPolls) throw new MediaProcessingError(`media ${media.id} is still processing`);
    await delay(pollInterval, signal);
    const { data } = await api.fetch.GET("/media/{id}", { params: { path: { id: media.id } }, ...(signal === undefined ? {} : { signal }) });
    media = data;
  }
  if (media?.state !== "ready") throw new MediaProcessingError(`media ${media?.id ?? "?"} failed`);
  return media;
}

function delay(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(resolve, ms);
    signal?.addEventListener(
      "abort",
      () => {
        clearTimeout(timer);
        reject(signal.reason as Error);
      },
      { once: true },
    );
  });
}
