// SPDX-License-Identifier: AGPL-3.0-only

// The code an authenticator app shows for a TOTP secret (RFC 6238 with RFC 4226: HMAC-SHA-1, six digits, 30-second
// steps), which the seed enters to enable two-factor authentication and to sign in, and the E2E tests to sign in.

import { createHmac } from "node:crypto";

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";

/** The bytes of an RFC 4648 base32 string, without padding. */
function base32(text: string): Buffer {
  let bits = 0;
  let value = 0;
  const out: number[] = [];
  for (const c of text.replace(/=+$/, "").toUpperCase()) {
    const v = alphabet.indexOf(c);
    if (v < 0) throw new Error(`totp: ${c} is not base32`);
    value = (value << 5) | v;
    bits += 5;
    if (bits >= 8) {
      out.push((value >>> (bits - 8)) & 0xff);
      bits -= 8;
    }
  }
  return Buffer.from(out);
}

/** The six-digit code of the base32 `secret` at the time `at` (ms since the epoch). */
export function totp(secret: string, at = Date.now()): string {
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(at / 30_000)));
  const mac = createHmac("sha1", base32(secret)).update(counter).digest();
  const offset = (mac[mac.length - 1] ?? 0) & 0x0f;
  const code = (mac.readUInt32BE(offset) & 0x7fffffff) % 1_000_000;
  return code.toString().padStart(6, "0");
}
