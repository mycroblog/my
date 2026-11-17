// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, expect, test } from "bun:test";
import {
  createPasskey,
  getPasskey,
  PasskeyError,
  supportsPasskeyAutofill,
  supportsPasskeys,
  type PasskeyFailure,
} from "./passkeys";
import { bytes, installBrowser, type FakeBrowser } from "./test/fake";

// The JSON the server sends (internal/adapter/webauthn): binary members in base64url without padding.
const registration = {
  rp: { name: "my", id: "example.com" },
  user: { id: "dTE", name: "alice@example.com", displayName: "alice" },
  challenge: "AAEC",
  pubKeyCredParams: [{ type: "public-key", alg: -7 }],
  timeout: 300000,
  excludeCredentials: [{ type: "public-key", id: "BAU", transports: ["internal", "carrier-pigeon"] }],
  authenticatorSelection: { residentKey: "required", userVerification: "required" },
  attestation: "none",
};
const signIn = { challenge: "AAEC", rpId: "example.com", timeout: 300000, userVerification: "required" };

let browser: FakeBrowser | undefined;
afterEach(() => {
  browser?.restore();
  browser = undefined;
});

/** The reason of the PasskeyError that `promise` rejects with. */
async function reasonOf(promise: Promise<unknown>): Promise<PasskeyFailure> {
  try {
    await promise;
  } catch (error) {
    if (error instanceof PasskeyError) return error.reason;
    throw error;
  }
  throw new Error("no PasskeyError");
}

test("without WebAuthn there are no passkeys, no autofill, and a ceremony fails as unsupported", async () => {
  expect(supportsPasskeys()).toBe(false);
  expect(await supportsPasskeyAutofill()).toBe(false);
  expect(await reasonOf(getPasskey(signIn))).toBe("unsupported");
});

test("the autofill is there only where the browser says conditional mediation is available", async () => {
  browser = installBrowser();
  expect(supportsPasskeys()).toBe(true);
  expect(await supportsPasskeyAutofill()).toBe(false);
  browser.restore();
  browser = installBrowser({ autofill: false });
  expect(await supportsPasskeyAutofill()).toBe(false);
  browser.restore();
  browser = installBrowser({ autofill: true });
  expect(await supportsPasskeyAutofill()).toBe(true);
});

test("a sign-in decodes the options and encodes the credential where the browser lacks the JSON methods", async () => {
  browser = installBrowser();
  const json = await getPasskey(signIn);
  const publicKey = browser.get.mock.calls[0]?.[0].publicKey;
  expect(publicKey?.challenge).toEqual(bytes(0, 1, 2));
  expect(publicKey?.rpId).toBe("example.com");
  expect(publicKey?.userVerification).toBe("required");
  expect(json).toEqual({
    id: "AQID",
    rawId: "AQID",
    type: "public-key",
    clientExtensionResults: {},
    authenticatorAttachment: "platform",
    response: { clientDataJSON: "AQID", authenticatorData: "Bwg", signature: "CQ", userHandle: "dQ" },
  });
});

test("a registration decodes the options and encodes the credential where the browser lacks the JSON methods", async () => {
  browser = installBrowser();
  const json = await createPasskey(registration);
  const publicKey = browser.create.mock.calls[0]?.[0].publicKey;
  expect(publicKey?.user.id).toEqual(bytes(0x75, 0x31));
  expect(publicKey?.excludeCredentials).toEqual([{ type: "public-key", id: bytes(4, 5), transports: ["internal"] }]);
  expect(publicKey?.authenticatorSelection).toEqual({ residentKey: "required", userVerification: "required" });
  expect(json["response"]).toEqual({ clientDataJSON: "AQID", attestationObject: "BAUG", transports: ["internal"] });
});

test("the browser's own JSON methods are used where it has them", async () => {
  browser = installBrowser({ native: true });
  expect(await createPasskey(registration)).toEqual({ native: true });
  expect(await getPasskey(signIn)).toEqual({ native: true });
  expect(browser.parsed).toEqual(["creation", "request"]);
});

test("the autofill asks with conditional mediation, and fails as unsupported where the browser has none", async () => {
  browser = installBrowser({ autofill: true });
  await getPasskey(signIn, { autofill: true });
  expect(browser.get.mock.calls[0]?.[0].mediation).toBe("conditional");
  browser.restore();
  browser = installBrowser({ autofill: false });
  expect(await reasonOf(getPasskey(signIn, { autofill: true }))).toBe("unsupported");
  expect(browser.get).not.toHaveBeenCalled();
});

test("the browser's errors become the reasons a screen words", async () => {
  browser = installBrowser();
  const cases: [string, PasskeyFailure][] = [
    ["NotAllowedError", "cancelled"],
    ["InvalidStateError", "exists"],
    ["NotSupportedError", "unsupported"],
    ["SecurityError", "failed"],
    ["UnknownError", "failed"],
  ];
  for (const [name, reason] of cases) {
    browser.answer = new DOMException("refused", name);
    expect(await reasonOf(createPasskey(registration))).toBe(reason);
  }
});

test("options the server got wrong fail before any prompt", async () => {
  browser = installBrowser();
  expect(await reasonOf(getPasskey({ challenge: 3 }))).toBe("failed");
  expect(await reasonOf(createPasskey({ ...registration, user: undefined }))).toBe("failed");
  expect(browser.get).not.toHaveBeenCalled();
  expect(browser.create).not.toHaveBeenCalled();
});

test("the caller's signal aborts a ceremony", async () => {
  browser = installBrowser();
  browser.answer = "hang";
  const controller = new AbortController();
  const waiting = reasonOf(getPasskey(signIn, { signal: controller.signal }));
  controller.abort();
  expect(await waiting).toBe("aborted");
  expect(await reasonOf(getPasskey(signIn, { signal: controller.signal }))).toBe("aborted");
});

test("a new ceremony aborts the one before it, as a press of the button ends the autofill's request", async () => {
  browser = installBrowser({ autofill: true });
  browser.answer = "hang";
  const autofill = reasonOf(getPasskey(signIn, { autofill: true }));
  await Promise.resolve();
  browser.answer = "credential";
  const pressed = getPasskey(signIn);
  expect(await autofill).toBe("aborted");
  expect((await pressed)["id"]).toBe("AQID");
});
