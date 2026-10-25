// SPDX-License-Identifier: MIT
import type { Api } from "./client.js";
import type { components } from "./schema.gen.js";

/** What the server makes of an uploaded file, which picks the sizes of its variants. */
export type MediaUploadPurpose = components["schemas"]["MediaUpload"]["purpose"];

/** A media file as the server describes it. */
export type MediaAttachment = components["schemas"]["MediaAttachment"];

/** What uploadMedia uploads. */
export type MediaUploadInput = {
  /** The image, video, or audio file. Its type is the part's Content-Type, which the server checks against the content. */
  file: Blob;
  /** The file name, such as "photo.jpg"; the name of a File, or "file", by default. */
  fileName?: string;
  /** The poster of a video or audio file. */
  thumbnail?: Blob;
  /** The file name of the thumbnail; the name of a File, or "thumbnail", by default. */
  thumbnailName?: string;
  /** The alternative text. */
  description?: string;
  /** What the file is for; attachment by default. */
  purpose?: MediaUploadPurpose;
};

function nameOf(blob: Blob, name: string | undefined, fallback: string): string {
  if (name !== undefined) return name;
  return typeof File !== "undefined" && blob instanceof File && blob.name !== "" ? blob.name : fallback;
}

/**
 * Uploads a media file (uploadMedia) as multipart/form-data. The answer is the file in its current state, usually
 * processing: poll `GET /media/{id}` until it is ready or failed. A Blob can be read again, so the upload is sent
 * again after a 401 like any other request; it is not retried after a 429 or 503, as POST /media takes no
 * Idempotency-Key.
 */
export async function uploadMedia(api: Api, input: MediaUploadInput, init?: { signal?: AbortSignal }): Promise<MediaAttachment> {
  const form = new FormData();
  form.append("file", input.file, nameOf(input.file, input.fileName, "file"));
  if (input.thumbnail !== undefined) form.append("thumbnail", input.thumbnail, nameOf(input.thumbnail, input.thumbnailName, "thumbnail"));
  if (input.description !== undefined && input.description !== "") form.append("description", input.description);
  if (input.purpose !== undefined) form.append("purpose", input.purpose);
  const { data } = await api.POST("/media", {
    // The body sent is the FormData of bodySerializer; the typed body only satisfies the spec's shape.
    body: { file: "", purpose: input.purpose ?? "attachment" },
    bodySerializer: () => form,
    ...(init?.signal === undefined ? {} : { signal: init.signal }),
  });
  if (data === undefined) throw new Error("mykit: uploadMedia: the answer has no body");
  return data;
}
