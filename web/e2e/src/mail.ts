// SPDX-License-Identifier: AGPL-3.0-only

// The mail of the demo server. It runs with `mail = fake` (server/demo.ts), the mailer of the test build: the server
// counts on it as on one that delivers, so it asks a new person to confirm their email address before the account can
// be used, lets an open registration in once the address is confirmed, and mails password reset links
// (`/auth/forgot_password`). The mailer sends nothing: it writes each message whole to the server's log, one JSON line
// that says "mail not sent (mail = fake)" with the recipient (`to`), the subject and the text. A test reads the mail of
// its own person there, as the person would open it.

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
