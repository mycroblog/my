// SPDX-License-Identifier: AGPL-3.0-only

// Handing a link to the reader's other apps: the system's share sheet (the Web Share API) where the device has one,
// else the clipboard. The caller words what happened.

/** What became of a link handed on: shared, copied in place of a share sheet, dismissed by the reader, or refused. */
export type ShareOutcome = "shared" | "copied" | "cancelled" | "failed";

/** Copies `text` to the clipboard; false when the browser refused it (no permission, no clipboard). */
export async function copyText(text: string): Promise<boolean> {
  try {
    if (typeof navigator === "undefined" || typeof navigator.clipboard === "undefined") return false;
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

/** Shares the link `url` through the system's share sheet, else copies it. */
export async function shareLink(url: string, title?: string): Promise<ShareOutcome> {
  const data: ShareData = title === undefined ? { url } : { url, title };
  if (typeof navigator !== "undefined" && typeof navigator.share === "function")
    try {
      await navigator.share(data);
      return "shared";
    } catch (e) {
      // The reader dismissed the sheet: nothing to tell. Any other refusal falls back to the clipboard.
      if (e instanceof DOMException && e.name === "AbortError") return "cancelled";
    }
  return (await copyText(url)) ? "copied" : "failed";
}
