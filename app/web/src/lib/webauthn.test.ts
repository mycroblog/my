// SPDX-License-Identifier: AGPL-3.0-only
import { afterEach, describe, expect, test } from "vitest";
import {
  FakeAssertionResponse,
  FakeAttestationResponse,
  FakePublicKeyCredential,
  installPasskeys,
  removePasskeys,
} from "../test/webauthn";
import {
  creationOptions,
  credentialJSON,
  decodeBase64url,
  encodeBase64url,
  PasskeyOptionsError,
  passkeyPromptFailure,
  requestOptions,
  supportsPasskeys,
} from "./webauthn";

afterEach(() => {
  removePasskeys();
});

const asBytes = (value: BufferSource | undefined) =>
  value === undefined ? [] : [...new Uint8Array(value instanceof ArrayBuffer ? value : value.buffer)];

/** Registration options as the server sends them (go-webauthn's PublicKeyCredentialCreationOptions). */
const registration = {
  rp: { name: "social.example", id: "social.example" },
  user: { name: "alice@social.example", displayName: "alice@social.example", id: "dTE" },
  challenge: "AQID-_8",
  pubKeyCredParams: [
    { type: "public-key", alg: -7 },
    { type: "public-key", alg: -257 },
  ],
  timeout: 300000,
  excludeCredentials: [{ type: "public-key", id: "BAU", transports: ["internal", "made-up"] }],
  authenticatorSelection: { requireResidentKey: true, residentKey: "required", userVerification: "required" },
  attestation: "none",
};

describe("base64url", () => {
  test("round-trips every byte value, without padding", () => {
    const all = new Uint8Array(256).map((_, i) => i);
    const text = encodeBase64url(all);
    expect(text).not.toMatch(/[+/=]/);
    expect([...new Uint8Array(decodeBase64url(text))]).toEqual([...all]);
    expect(encodeBase64url(new Uint8Array([0xfb, 0xff]))).toBe("-_8");
    expect([...new Uint8Array(decodeBase64url("-_8="))]).toEqual([0xfb, 0xff]);
  });
});

describe("creationOptions", () => {
  test("decodes the binary members and keeps the rest", () => {
    const o = creationOptions(registration);
    expect(asBytes(o.challenge)).toEqual([1, 2, 3, 0xfb, 0xff]);
    expect(asBytes(o.user.id)).toEqual([0x75, 0x31]);
    expect(o.user.name).toBe("alice@social.example");
    expect(o.rp).toEqual({ name: "social.example", id: "social.example" });
    expect(o.pubKeyCredParams).toEqual([
      { type: "public-key", alg: -7 },
      { type: "public-key", alg: -257 },
    ]);
    expect(o.timeout).toBe(300000);
    expect(o.attestation).toBe("none");
    expect(o.authenticatorSelection).toEqual({
      requireResidentKey: true,
      residentKey: "required",
      userVerification: "required",
    });
    expect(o.excludeCredentials?.map((d) => ({ ...d, id: asBytes(d.id) }))).toEqual([
      { type: "public-key", id: [4, 5], transports: ["internal"] },
    ]);
  });

  test("throws on options that are not WebAuthn's", () => {
    expect(() => creationOptions(null)).toThrow(PasskeyOptionsError);
    expect(() => creationOptions({ ...registration, challenge: 3 })).toThrow(PasskeyOptionsError);
    expect(() => creationOptions({ ...registration, attestation: "maybe" })).toThrow(PasskeyOptionsError);
    expect(() => creationOptions({ ...registration, pubKeyCredParams: [{ type: "other", alg: -7 }] })).toThrow(
      PasskeyOptionsError,
    );
    expect(() => creationOptions({ ...registration, user: { ...registration.user, id: "!!" } })).toThrow(
      PasskeyOptionsError,
    );
  });
});

describe("requestOptions", () => {
  test("decodes the challenge of a sign-in with a discoverable credential", () => {
    const o = requestOptions({
      challenge: "AQI",
      rpId: "social.example",
      timeout: 300000,
      userVerification: "required",
    });
    expect(asBytes(o.challenge)).toEqual([1, 2]);
    expect(o).toMatchObject({ rpId: "social.example", timeout: 300000, userVerification: "required" });
    expect(o.allowCredentials).toBeUndefined();
  });
});

describe("credentialJSON", () => {
  test("encodes a new credential as toJSON() does", () => {
    installPasskeys();
    const credential = new FakePublicKeyCredential(new FakeAttestationResponse());
    expect(credentialJSON(credential as unknown as PublicKeyCredential)).toEqual({
      id: "AQID",
      rawId: "AQID",
      type: "public-key",
      authenticatorAttachment: "platform",
      clientExtensionResults: {},
      response: { clientDataJSON: "AQID", attestationObject: "BAUG", transports: ["internal"] },
    });
  });

  test("encodes an assertion with its user handle", () => {
    installPasskeys();
    const credential = new FakePublicKeyCredential(new FakeAssertionResponse());
    expect(credentialJSON(credential as unknown as PublicKeyCredential)).toMatchObject({
      response: { clientDataJSON: "AQID", authenticatorData: "Bwg", signature: "CQ", userHandle: "dTE" },
    });
  });
});

describe("passkeyPromptFailure", () => {
  test("tells a dismissed prompt from a passkey that exists and from the rest", () => {
    expect(passkeyPromptFailure(new DOMException("", "NotAllowedError"))).toBe("cancelled");
    expect(passkeyPromptFailure(new DOMException("", "AbortError"))).toBe("cancelled");
    expect(passkeyPromptFailure(new DOMException("", "InvalidStateError"))).toBe("exists");
    expect(passkeyPromptFailure(new DOMException("", "SecurityError"))).toBe("unavailable");
    expect(passkeyPromptFailure(new Error("x"))).toBe("unavailable");
  });
});

describe("supportsPasskeys", () => {
  test("needs PublicKeyCredential and navigator.credentials", () => {
    expect(supportsPasskeys()).toBe(false);
    installPasskeys();
    expect(supportsPasskeys()).toBe(true);
  });
});
