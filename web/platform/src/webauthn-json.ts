// SPDX-License-Identifier: AGPL-3.0-only

// The WebAuthn JSON of the server (Web Authentication Level 3, §5.1.8, §5.1.9), for the browsers of the floor that
// lack PublicKeyCredential.parseCreationOptionsFromJSON, parseRequestOptionsFromJSON and toJSON: the options with
// their binary members in base64url become the objects navigator.credentials takes, and the credential it returns
// becomes the JSON that toJSON gives. Only the members the server sends are read; anything malformed throws a
// TypeError, as the browser's own parsers do.

type Json = Record<string, unknown>;

function isRecord(value: unknown): value is Json {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function malformed(member: string): TypeError {
  return new TypeError(`invalid WebAuthn options: ${member}`);
}

function record(value: unknown, member: string): Json {
  if (!isRecord(value)) throw malformed(member);
  return value;
}

function text(o: Json, member: string): string {
  const value = o[member];
  if (typeof value !== "string") throw malformed(member);
  return value;
}

function optionalText(o: Json, member: string): string | undefined {
  const value = o[member];
  if (value === undefined) return undefined;
  if (typeof value !== "string") throw malformed(member);
  return value;
}

function optionalNumber(o: Json, member: string): number | undefined {
  const value = o[member];
  if (value === undefined) return undefined;
  if (typeof value !== "number") throw malformed(member);
  return value;
}

/** The value of `member` when it is one of `allowed`, or undefined when it is absent. */
function oneOf<T extends string>(o: Json, member: string, allowed: readonly T[]): T | undefined {
  const value = optionalText(o, member);
  if (value === undefined) return undefined;
  const match = allowed.find((a) => a === value);
  if (match === undefined) throw malformed(member);
  return match;
}

/** Decodes base64url (RFC 4648 §5), with or without padding. */
function decodeBase64url(value: string): ArrayBuffer {
  const base64 = value.replaceAll("-", "+").replaceAll("_", "/");
  const binary = atob(base64.padEnd(Math.ceil(base64.length / 4) * 4, "="));
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes.buffer;
}

/** Encodes bytes as base64url without padding, as WebAuthn's JSON does. */
function encodeBase64url(value: ArrayBuffer): string {
  let binary = "";
  for (const byte of new Uint8Array(value)) binary += String.fromCharCode(byte);
  return btoa(binary).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

function bytes(o: Json, member: string): ArrayBuffer {
  const value = text(o, member);
  try {
    return decodeBase64url(value);
  } catch {
    throw malformed(member);
  }
}

const requirements = ["required", "preferred", "discouraged"] as const;
const attachments = ["platform", "cross-platform"] as const;
const attestations = ["none", "indirect", "direct", "enterprise"] as const;
const transports = ["ble", "hybrid", "internal", "nfc", "usb"] as const;

function descriptors(value: unknown, member: string): PublicKeyCredentialDescriptor[] | undefined {
  if (value === undefined || value === null) return undefined;
  if (!Array.isArray(value)) throw malformed(member);
  return value.map((item: unknown) => {
    const d = record(item, member);
    if (d["type"] !== "public-key") throw malformed(`${member}.type`);
    const listed: unknown = d["transports"];
    const known = Array.isArray(listed) ? transports.filter((t) => listed.includes(t)) : undefined;
    return { type: "public-key", id: bytes(d, "id"), ...(known === undefined ? {} : { transports: known }) };
  });
}

function authenticatorSelection(s: Json): AuthenticatorSelectionCriteria {
  const attachment = oneOf(s, "authenticatorAttachment", attachments);
  const residentKey = oneOf(s, "residentKey", requirements);
  const userVerification = oneOf(s, "userVerification", requirements);
  const requireResidentKey = s["requireResidentKey"];
  return {
    ...(attachment === undefined ? {} : { authenticatorAttachment: attachment }),
    ...(residentKey === undefined ? {} : { residentKey }),
    ...(typeof requireResidentKey === "boolean" ? { requireResidentKey } : {}),
    ...(userVerification === undefined ? {} : { userVerification }),
  };
}

/** What parseCreationOptionsFromJSON gives for the server's registration options. */
export function creationOptions(json: unknown): PublicKeyCredentialCreationOptions {
  const o = record(json, "options");
  const rp = record(o["rp"], "rp");
  const user = record(o["user"], "user");
  const rpId = optionalText(rp, "id");
  const params: unknown = o["pubKeyCredParams"];
  if (!Array.isArray(params)) throw malformed("pubKeyCredParams");
  const timeout = optionalNumber(o, "timeout");
  const exclude = descriptors(o["excludeCredentials"], "excludeCredentials");
  const attestation = oneOf(o, "attestation", attestations);
  const selection = o["authenticatorSelection"];
  return {
    rp: { name: text(rp, "name"), ...(rpId === undefined ? {} : { id: rpId }) },
    user: { id: bytes(user, "id"), name: text(user, "name"), displayName: text(user, "displayName") },
    challenge: bytes(o, "challenge"),
    pubKeyCredParams: params.map((item: unknown) => {
      const p = record(item, "pubKeyCredParams");
      const alg = optionalNumber(p, "alg");
      if (p["type"] !== "public-key" || alg === undefined) throw malformed("pubKeyCredParams");
      return { type: "public-key", alg };
    }),
    ...(timeout === undefined ? {} : { timeout }),
    ...(exclude === undefined ? {} : { excludeCredentials: exclude }),
    ...(attestation === undefined ? {} : { attestation }),
    ...(selection === undefined
      ? {}
      : { authenticatorSelection: authenticatorSelection(record(selection, "authenticatorSelection")) }),
  };
}

/** What parseRequestOptionsFromJSON gives for the server's sign-in options. */
export function requestOptions(json: unknown): PublicKeyCredentialRequestOptions {
  const o = record(json, "options");
  const rpId = optionalText(o, "rpId");
  const timeout = optionalNumber(o, "timeout");
  const allow = descriptors(o["allowCredentials"], "allowCredentials");
  const userVerification = oneOf(o, "userVerification", requirements);
  return {
    challenge: bytes(o, "challenge"),
    ...(rpId === undefined ? {} : { rpId }),
    ...(timeout === undefined ? {} : { timeout }),
    ...(allow === undefined ? {} : { allowCredentials: allow }),
    ...(userVerification === undefined ? {} : { userVerification }),
  };
}

/** The members of a credential that its JSON reads: those of a PublicKeyCredential and of its response. */
export interface CredentialLike {
  readonly id: string;
  readonly rawId: ArrayBuffer;
  readonly type: string;
  readonly authenticatorAttachment: string | null;
  readonly response: {
    readonly clientDataJSON: ArrayBuffer;
    // A registration's (AuthenticatorAttestationResponse).
    readonly attestationObject?: ArrayBuffer;
    getTransports?(): string[];
    // A sign-in's (AuthenticatorAssertionResponse).
    readonly authenticatorData?: ArrayBuffer;
    readonly signature?: ArrayBuffer;
    readonly userHandle?: ArrayBuffer | null;
  };
}

/** What toJSON gives for a credential of navigator.credentials. */
export function credentialJSON(credential: CredentialLike): Json {
  const { response } = credential;
  const common = {
    id: credential.id,
    rawId: encodeBase64url(credential.rawId),
    type: credential.type,
    // The server asks for no extension, and the outputs of the others may hold buffers.
    clientExtensionResults: {},
    ...(credential.authenticatorAttachment === null
      ? {}
      : { authenticatorAttachment: credential.authenticatorAttachment }),
  };
  if (response.attestationObject !== undefined) {
    return {
      ...common,
      response: {
        clientDataJSON: encodeBase64url(response.clientDataJSON),
        attestationObject: encodeBase64url(response.attestationObject),
        transports: response.getTransports?.() ?? [],
      },
    };
  }
  if (response.authenticatorData !== undefined && response.signature !== undefined) {
    const { userHandle } = response;
    return {
      ...common,
      response: {
        clientDataJSON: encodeBase64url(response.clientDataJSON),
        authenticatorData: encodeBase64url(response.authenticatorData),
        signature: encodeBase64url(response.signature),
        ...(userHandle === null || userHandle === undefined ? {} : { userHandle: encodeBase64url(userHandle) }),
      },
    };
  }
  throw new TypeError("the credential has neither an attestation nor an assertion");
}
