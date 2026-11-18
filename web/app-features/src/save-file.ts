// SPDX-License-Identifier: AGPL-3.0-only

// Saving a file the server gave (the archive of the reader's data, a CSV file) to the reader's device, as a download
// of the page: what the closed server's screen and the settings' export both do with what app-data read.

/** Saves `blob` as a file named `name`, as a download of the page. */
export function saveFile(blob: Blob, name: string): void {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  link.click();
  URL.revokeObjectURL(url);
}
