// SPDX-License-Identifier: AGPL-3.0-only
// Prepares a picked file for upload in the browser. An image is drawn again without its metadata (prepareImage). A
// video or audio file is sent as it is: the server removes its metadata without decoding it (ADR-0005 §9), so for a
// video the client also draws a frame of it on a canvas and uploads it as its `thumbnail` (the uploadMedia operation),
// which the server processes as the poster. An audio file takes the cover in it, if any, as its poster on the server.
//
// It uses an HTMLVideoElement that never joins the document, and OffscreenCanvas.convertToBlob (MDN).
import { mediaKind } from "./policy";
import { fitWithin, prepareImage } from "./prepareImage";

/** A file ready to upload: the file, the poster of a video (null without one), and the image to preview it with. */
export type PreparedMedia = { file: File; poster: Blob | null; preview: Blob | null };

/** Prepares a picked image, video, or audio file for upload. */
export async function prepareMedia(file: File): Promise<PreparedMedia> {
  switch (mediaKind(file.type)) {
    case "image": {
      const prepared = await prepareImage(file);
      return { file: prepared, poster: null, preview: prepared };
    }
    case "video": {
      const poster = await preparePoster(file);
      return { file, poster, preview: poster };
    }
    default:
      return { file, poster: null, preview: null };
  }
}

/** The largest area of a poster: the medium rendition of an attachment (MediaUpload.purpose), 1920 × 1080 pixels. */
const maxPosterPixels = 1920 * 1080;

/** How long the browser may take to load a frame before the video goes without a poster. */
const posterTimeoutMs = 10_000;

/** The moment the poster shows: a little after the start, past a black first frame, within a short video. */
const posterTime = 0.5;

/** Resolves when `target` fires `event`, rejects on `error` or after the timeout. */
function once(target: HTMLVideoElement, event: string, timeoutMs: number): Promise<void> {
  return new Promise((resolve, reject) => {
    const done = () => {
      clearTimeout(timer);
      target.removeEventListener(event, onEvent);
      target.removeEventListener("error", onError);
    };
    const onEvent = () => {
      done();
      resolve();
    };
    const onError = () => {
      done();
      reject(new Error(`the video could not be read (${event})`));
    };
    const timer = setTimeout(() => {
      done();
      reject(new Error(`the video took too long to load (${event})`));
    }, timeoutMs);
    target.addEventListener(event, onEvent);
    target.addEventListener("error", onError);
  });
}

/**
 * Returns a JPEG of a frame of the video `file`, scaled down to the medium size, or null when the browser cannot
 * decode the video (an HEVC video in Firefox, for example). Drawing the frame leaves the file's metadata behind.
 */
async function preparePoster(file: File): Promise<Blob | null> {
  const url = URL.createObjectURL(file);
  const video = document.createElement("video");
  video.muted = true;
  video.playsInline = true;
  video.preload = "auto";
  try {
    const loaded = once(video, "loadeddata", posterTimeoutMs);
    video.src = url;
    await loaded;
    const seeked = once(video, "seeked", posterTimeoutMs);
    video.currentTime = Number.isFinite(video.duration) ? Math.min(posterTime, video.duration / 2) : 0;
    await seeked;
    if (video.videoWidth === 0 || video.videoHeight === 0) return null;
    const size = fitWithin(video.videoWidth, video.videoHeight, maxPosterPixels);
    const canvas = new OffscreenCanvas(size.width, size.height);
    const context = canvas.getContext("2d");
    if (context === null) return null;
    context.drawImage(video, 0, 0, size.width, size.height);
    return await canvas.convertToBlob({ type: "image/jpeg", quality: 0.85 });
  } catch {
    return null;
  } finally {
    video.removeAttribute("src");
    video.load();
    URL.revokeObjectURL(url);
  }
}
