// SPDX-License-Identifier: AGPL-3.0-only

// The mail of the demo server. It runs with `mail = none` (server/demo.ts): its mailer sends nothing, and in the
// `development` profile it writes each message whole to the server's log, for a developer to take a link from. A
// message is one JSON line of the log that says "mail not sent (mail = none)" with the recipient, the subject and the
// text. A test reads the mail of its own person there, as the person would open it.
//
// With no mailer that delivers, the server asks nobody to confirm an email address before the account can be used,
// treats an open registration as one that waits for approval, and offers no password reset by email: the flows that
// need a delivered mail are not the demo server's.

import { readFileSync } from "node:fs";

/** The texts of the messages to `address` in the server's log `file`, oldest first. */
function mailTo(file: string, address: string): string[] {
  const found: string[] = [];
  for (const line of readFileSync(file, "utf8").split("\n")) {
    if (!line.includes("mail not sent")) continue;
    let entry: Record<string, unknown>;
    try {
      entry = JSON.parse(line) as Record<string, unknown>;
    } catch {
      // The line the server is writing now.
      continue;
    }
    const { msg, to, text } = entry;
    if (typeof msg === "string" && msg.startsWith("mail not sent") && to === address && typeof text === "string")
      found.push(text);
  }
  return found;
}

/** The newest link to the server's page at `path` among the messages to `address`, when one came. */
export function mailedLinkTo(file: string, address: string, path: string): string | undefined {
  const links = mailTo(file, address).flatMap((text) => text.match(/https?:\/\/\S+/g) ?? []);
  return links.findLast((link) => URL.canParse(link) && new URL(link).pathname === path);
}
