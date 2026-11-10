// SPDX-License-Identifier: AGPL-3.0-only

/** How long the URL of a downloaded file stays usable, in ms: long enough for the browser to start saving it. */
const downloadUrlLifetime = 60_000;

/** Saves `blob` as a file named `name` through a temporary link, as the browser's download does. */
export function saveFile(blob: Blob, name: string): void {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  link.click();
  setTimeout(() => {
    URL.revokeObjectURL(url);
  }, downloadUrlLifetime);
}
