// SPDX-License-Identifier: AGPL-3.0-only

// A software authenticator for the seed (WebAuthn Level 3): it creates a discoverable ES256 credential for the options
// of a passkey registration and answers them as `PublicKeyCredential.toJSON()` would, with no attestation ("none").
// The credential's private key goes to the seed's state, so that an E2E test can hand it to Chromium's virtual
// authenticator (the DevTools protocol's WebAuthn.addCredential) and sign in with the passkey.

import { createHash, generateKeyPairSync, randomBytes } from "node:crypto";

/** A credential made for a person, in the encodings of WebAuthn.addCredential (standard base64). */
export type PasskeyCredential = {
  credentialId: string;
  rpId: string;
  /** The private key, PKCS #8 DER. */
  privateKey: string;
  userHandle: string;
};

type Cbor = number | string | Uint8Array | readonly Cbor[] | Map<Cbor, Cbor>;

function head(major: number, n: number): number[] {
  if (n < 24) return [(major << 5) | n];
  if (n < 0x100) return [(major << 5) | 24, n];
  if (n < 0x10000) return [(major << 5) | 25, n >> 8, n & 0xff];
  return [(major << 5) | 26, (n >>> 24) & 0xff, (n >> 16) & 0xff, (n >> 8) & 0xff, n & 0xff];
}

/** The CBOR (RFC 8949) of the few kinds a credential needs: integers, strings, byte strings, arrays, maps. */
function cbor(v: Cbor): number[] {
  if (typeof v === "number") return v >= 0 ? head(0, v) : head(1, -1 - v);
  if (typeof v === "string") {
    const bytes = [...new TextEncoder().encode(v)];
    return [...head(3, bytes.length), ...bytes];
  }
  if (v instanceof Uint8Array) return [...head(2, v.length), ...v];
  if (v instanceof Map) return [...head(5, v.size), ...[...v].flatMap(([k, x]) => [...cbor(k), ...cbor(x)])];
  return [...head(4, v.length), ...v.flatMap(cbor)];
}

const b64url = (b: Uint8Array) => Buffer.from(b).toString("base64url");
const b64 = (b: Uint8Array) => Buffer.from(b).toString("base64");

/** The credential and the browser's answer to `options` (PublicKeyCredentialCreationOptionsJSON) at `origin`. */
export function createPasskey(
  options: Record<string, unknown>,
  origin: string,
): { credential: PasskeyCredential; response: Record<string, unknown> } {
  const rp = options["rp"] as { id?: string } | undefined;
  const user = options["user"] as { id: string };
  const rpId = rp?.id ?? new URL(origin).hostname;
  const { publicKey, privateKey } = generateKeyPairSync("ec", { namedCurve: "P-256" });
  const jwk = publicKey.export({ format: "jwk" });
  const coseKey = new Map<Cbor, Cbor>([
    [1, 2],
    [3, -7],
    [-1, 1],
    [-2, Buffer.from(jwk.x ?? "", "base64url")],
    [-3, Buffer.from(jwk.y ?? "", "base64url")],
  ]);
  const id = new Uint8Array(randomBytes(16));
  const authData = new Uint8Array([
    ...createHash("sha256").update(rpId).digest(),
    // User present, user verified, attested credential data.
    0x45,
    ...[0, 0, 0, 0],
    ...new Uint8Array(16),
    id.length >> 8,
    id.length & 0xff,
    ...id,
    ...cbor(coseKey),
  ]);
  const attestationObject = new Uint8Array(
    cbor(
      new Map<Cbor, Cbor>([
        ["fmt", "none"],
        ["attStmt", new Map()],
        ["authData", authData],
      ]),
    ),
  );
  const clientData = new TextEncoder().encode(
    JSON.stringify({ type: "webauthn.create", challenge: options["challenge"], origin, crossOrigin: false }),
  );
  const der = new Uint8Array(privateKey.export({ format: "der", type: "pkcs8" }));
  const spki = new Uint8Array(publicKey.export({ format: "der", type: "spki" }));
  return {
    credential: {
      credentialId: b64(id),
      rpId,
      privateKey: b64(der),
      userHandle: b64(Buffer.from(user.id, "base64url")),
    },
    response: {
      id: b64url(id),
      rawId: b64url(id),
      type: "public-key",
      authenticatorAttachment: "platform",
      clientExtensionResults: {},
      response: {
        clientDataJSON: b64url(clientData),
        attestationObject: b64url(attestationObject),
        authenticatorData: b64url(authData),
        publicKey: b64url(spki),
        publicKeyAlgorithm: -7,
        transports: ["internal"],
      },
    },
  };
}
