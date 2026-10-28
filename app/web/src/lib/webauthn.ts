// SPDX-License-Identifier: AGPL-3.0-only
// Passkeys in the browser (WebAuthn Level 3, FR-004): the server's options, JSON with binary values in base64url
// (PublicKeyCredentialCreationOptionsJSON / PublicKeyCredentialRequestOptionsJSON, §5.1.8 and §5.1.9), become the
// objects that navigator.credentials takes, and the credential it returns becomes the JSON that the server reads
// (PublicKeyCredential.toJSON, §5.1). Browsers that have parseCreationOptionsFromJSON and toJSON do the same; this
// covers those that do not yet. Only the members that the server sends are read; anything malformed throws a
// PasskeyOptionsError. Both the settings (registration) and the server's sign-in page (src/server-pages) use it.

/** Options from the server that are not WebAuthn options. */
export class PasskeyOptionsError extends Error {
  constructor(member: string) {
    super(`invalid passkey options: ${member}`);
    this.name = "PasskeyOptionsError";
  }
}

type Json = Record<string, unknown>;

function isRecord(value: unknown): value is Json {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function record(value: unknown, member: string): Json {
  if (!isRecord(value)) throw new PasskeyOptionsError(member);
  return value;
}

function text(o: Json, member: string): string {
  const value = o[member];
  if (typeof value !== "string") throw new PasskeyOptionsError(member);
  return value;
}

function optionalText(o: Json, member: string): string | undefined {
  const value = o[member];
  if (value === undefined) return undefined;
  if (typeof value !== "string") throw new PasskeyOptionsError(member);
  return value;
}

function optionalNumber(o: Json, member: string): number | undefined {
  const value = o[member];
  if (value === undefined) return undefined;
  if (typeof value !== "number") throw new PasskeyOptionsError(member);
  return value;
}

/** The value of `member` when it is one of `allowed`, or undefined when it is absent. */
function oneOf<T extends string>(o: Json, member: string, allowed: readonly T[]): T | undefined {
  const value = optionalText(o, member);
  if (value === undefined) return undefined;
  const match = allowed.find((a) => a === value);
  if (match === undefined) throw new PasskeyOptionsError(member);
  return match;
}

/** Decodes base64url (RFC 4648 §5) with or without padding. */
export function decodeBase64url(value: string): ArrayBuffer {
  const base64 = value.replaceAll("-", "+").replaceAll("_", "/");
  const binary = atob(base64.padEnd(Math.ceil(base64.length / 4) * 4, "="));
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes.buffer;
}

/** Encodes bytes as base64url without padding, as WebAuthn's JSON does. */
export function encodeBase64url(value: ArrayBuffer | ArrayBufferView): string {
  const bytes =
    value instanceof ArrayBuffer
      ? new Uint8Array(value)
      : new Uint8Array(value.buffer, value.byteOffset, value.byteLength);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

function bytes(o: Json, member: string): ArrayBuffer {
  try {
    return decodeBase64url(text(o, member));
  } catch {
    throw new PasskeyOptionsError(member);
  }
}

const userVerifications = ["required", "preferred", "discouraged"] as const;
const residentKeys = ["required", "preferred", "discouraged"] as const;
const attachments = ["platform", "cross-platform"] as const;
const attestations = ["none", "indirect", "direct", "enterprise"] as const;
const transports = ["ble", "hybrid", "internal", "nfc", "usb"] as const;

function descriptors(value: unknown, member: string): PublicKeyCredentialDescriptor[] | undefined {
  if (value === undefined || value === null) return undefined;
  if (!Array.isArray(value)) throw new PasskeyOptionsError(member);
  return value.map((item: unknown) => {
    const d = record(item, member);
    if (d["type"] !== "public-key") throw new PasskeyOptionsError(`${member}.type`);
    const listed: unknown = d["transports"];
    const known = Array.isArray(listed) ? transports.filter((t) => listed.includes(t)) : undefined;
    return { type: "public-key", id: bytes(d, "id"), ...(known === undefined ? {} : { transports: known }) };
  });
}

/** The options of navigator.credentials.create() for the server's registration options. */
export function creationOptions(json: unknown): PublicKeyCredentialCreationOptions {
  const o = record(json, "options");
  const rp = record(o["rp"], "rp");
  const user = record(o["user"], "user");
  const rpID = optionalText(rp, "id");
  const params: unknown = o["pubKeyCredParams"];
  if (!Array.isArray(params)) throw new PasskeyOptionsError("pubKeyCredParams");
  const timeout = optionalNumber(o, "timeout");
  const exclude = descriptors(o["excludeCredentials"], "excludeCredentials");
  const attestation = oneOf(o, "attestation", attestations);
  const selection =
    o["authenticatorSelection"] === undefined
      ? undefined
      : record(o["authenticatorSelection"], "authenticatorSelection");
  return {
    rp: { name: text(rp, "name"), ...(rpID === undefined ? {} : { id: rpID }) },
    user: { id: bytes(user, "id"), name: text(user, "name"), displayName: text(user, "displayName") },
    challenge: bytes(o, "challenge"),
    pubKeyCredParams: params.map((item: unknown) => {
      const p = record(item, "pubKeyCredParams");
      const alg = optionalNumber(p, "alg");
      if (p["type"] !== "public-key" || alg === undefined) throw new PasskeyOptionsError("pubKeyCredParams");
      return { type: "public-key", alg };
    }),
    ...(timeout === undefined ? {} : { timeout }),
    ...(exclude === undefined ? {} : { excludeCredentials: exclude }),
    ...(attestation === undefined ? {} : { attestation }),
    ...(selection === undefined ? {} : { authenticatorSelection: authenticatorSelection(selection) }),
  };
}

function authenticatorSelection(s: Json): AuthenticatorSelectionCriteria {
  const attachment = oneOf(s, "authenticatorAttachment", attachments);
  const residentKey = oneOf(s, "residentKey", residentKeys);
  const userVerification = oneOf(s, "userVerification", userVerifications);
  const require = s["requireResidentKey"];
  return {
    ...(attachment === undefined ? {} : { authenticatorAttachment: attachment }),
    ...(residentKey === undefined ? {} : { residentKey }),
    ...(typeof require === "boolean" ? { requireResidentKey: require } : {}),
    ...(userVerification === undefined ? {} : { userVerification }),
  };
}

/** The options of navigator.credentials.get() for the server's sign-in options. */
export function requestOptions(json: unknown): PublicKeyCredentialRequestOptions {
  const o = record(json, "options");
  const rpID = optionalText(o, "rpId");
  const timeout = optionalNumber(o, "timeout");
  const allow = descriptors(o["allowCredentials"], "allowCredentials");
  const userVerification = oneOf(o, "userVerification", userVerifications);
  return {
    challenge: bytes(o, "challenge"),
    ...(rpID === undefined ? {} : { rpId: rpID }),
    ...(timeout === undefined ? {} : { timeout }),
    ...(allow === undefined ? {} : { allowCredentials: allow }),
    ...(userVerification === undefined ? {} : { userVerification }),
  };
}

/** A credential as the JSON that the server reads: what PublicKeyCredential.toJSON() returns. */
export function credentialJSON(credential: PublicKeyCredential): Json {
  const { response } = credential;
  const common = {
    id: credential.id,
    rawId: encodeBase64url(credential.rawId),
    type: credential.type,
    // The extension outputs that the server did not ask for are never read (they may hold buffers).
    clientExtensionResults: {},
    ...(credential.authenticatorAttachment === null
      ? {}
      : { authenticatorAttachment: credential.authenticatorAttachment }),
  };
  if (response instanceof AuthenticatorAttestationResponse) {
    return {
      ...common,
      response: {
        clientDataJSON: encodeBase64url(response.clientDataJSON),
        attestationObject: encodeBase64url(response.attestationObject),
        transports: response.getTransports(),
      },
    };
  }
  if (response instanceof AuthenticatorAssertionResponse) {
    return {
      ...common,
      response: {
        clientDataJSON: encodeBase64url(response.clientDataJSON),
        authenticatorData: encodeBase64url(response.authenticatorData),
        signature: encodeBase64url(response.signature),
        ...(response.userHandle === null ? {} : { userHandle: encodeBase64url(response.userHandle) }),
      },
    };
  }
  throw new PasskeyOptionsError("response");
}

/** Whether the browser can create and use passkeys. */
export function supportsPasskeys(): boolean {
  return "PublicKeyCredential" in window && "credentials" in navigator;
}

/** Why the browser did not give a credential, by what the user can do about it. */
export type PasskeyPromptFailure =
  /** The user dismissed the prompt, it timed out, or the browser refused without saying why (NotAllowedError). */
  | "cancelled"
  /** The authenticator holds a passkey of the account already (InvalidStateError, excludeCredentials). */
  | "exists"
  /** The browser refused for another reason: an origin outside the relying party, or no support. */
  | "unavailable";

/** Classifies an error of navigator.credentials. */
export function passkeyPromptFailure(error: unknown): PasskeyPromptFailure {
  if (error instanceof DOMException) {
    if (error.name === "NotAllowedError" || error.name === "AbortError") return "cancelled";
    if (error.name === "InvalidStateError") return "exists";
  }
  return "unavailable";
}

/** Creates a passkey with the server's registration options and returns it as JSON. */
export async function createPasskey(options: unknown): Promise<Json> {
  const credential = await navigator.credentials.create({ publicKey: creationOptions(options) });
  if (!(credential instanceof PublicKeyCredential)) throw new DOMException("no passkey was created", "NotAllowedError");
  return credentialJSON(credential);
}

/** Asks for a passkey with the server's sign-in options and returns it as JSON. */
export async function getPasskey(options: unknown): Promise<Json> {
  const credential = await navigator.credentials.get({ publicKey: requestOptions(options) });
  if (!(credential instanceof PublicKeyCredential)) throw new DOMException("no passkey was given", "NotAllowedError");
  return credentialJSON(credential);
}
