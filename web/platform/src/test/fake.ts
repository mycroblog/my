// SPDX-License-Identifier: AGPL-3.0-only
// A browser for the tests: Bun has no WebAuthn, so these stand in for PublicKeyCredential and navigator.credentials
// in the globals until `restore`. The credentials answer with fixed bytes, or as the test sets with `answer`.

import { mock } from "bun:test";
import { creationOptions, requestOptions } from "../webauthn-json";

export const bytes = (...values: number[]) => new Uint8Array(values).buffer;

/** A credential of a registration (with an attestation) or of a sign-in (with an assertion). */
class FakeCredential {
  readonly id = "AQID";
  readonly rawId = bytes(1, 2, 3);
  readonly type = "public-key";
  readonly authenticatorAttachment = "platform";
  readonly response: Record<string, unknown>;
  constructor(kind: "create" | "get", toJSON?: () => unknown) {
    this.response =
      kind === "create"
        ? { clientDataJSON: bytes(1, 2, 3), attestationObject: bytes(4, 5, 6), getTransports: () => ["internal"] }
        : {
            clientDataJSON: bytes(1, 2, 3),
            authenticatorData: bytes(7, 8),
            signature: bytes(9),
            userHandle: bytes(0x75),
          };
    if (toJSON !== undefined) Object.assign(this, { toJSON });
  }
}

/** How the fake browser answers a prompt: with a credential, an error, or not until it is aborted. */
type Answer = "credential" | "hang" | DOMException;

export interface FakeBrowserOptions {
  /** Whether PublicKeyCredential has parseCreationOptionsFromJSON, parseRequestOptionsFromJSON and toJSON. */
  native?: boolean;
  /** What isConditionalMediationAvailable answers; undefined leaves it out. */
  autofill?: boolean | undefined;
}

export interface FakeBrowser {
  create: ReturnType<typeof mock<(options: CredentialCreationOptions) => Promise<unknown>>>;
  get: ReturnType<typeof mock<(options: CredentialRequestOptions) => Promise<unknown>>>;
  parsed: string[];
  answer: Answer;
  restore(): void;
}

function prompt(browser: { answer: Answer }, signal: AbortSignal | null | undefined, credential: () => unknown) {
  return new Promise<unknown>((resolve, reject) => {
    const aborted = () => {
      reject(new DOMException("aborted", "AbortError"));
    };
    if (signal?.aborted === true) aborted();
    signal?.addEventListener("abort", aborted);
    if (browser.answer === "credential") resolve(credential());
    else if (browser.answer instanceof DOMException) reject(browser.answer);
  });
}

/** Makes the browser of the test one with passkeys, and returns its prompts. */
export function installBrowser({ native = false, autofill }: FakeBrowserOptions = {}): FakeBrowser {
  const toJSON = native ? () => ({ native: true }) : undefined;
  const browser: FakeBrowser = {
    parsed: [],
    answer: "credential",
    create: mock((options: CredentialCreationOptions) =>
      prompt(browser, options.signal, () => new FakeCredential("create", toJSON)),
    ),
    get: mock((options: CredentialRequestOptions) =>
      prompt(browser, options.signal, () => new FakeCredential("get", toJSON)),
    ),
    restore: () => {
      Reflect.deleteProperty(globalThis, "PublicKeyCredential");
      Reflect.deleteProperty(navigator, "credentials");
    },
  };
  const statics: Record<string, unknown> = {};
  if (native) {
    statics["parseCreationOptionsFromJSON"] = (json: unknown) => {
      browser.parsed.push("creation");
      return creationOptions(json);
    };
    statics["parseRequestOptionsFromJSON"] = (json: unknown) => {
      browser.parsed.push("request");
      return requestOptions(json);
    };
  }
  if (autofill !== undefined) statics["isConditionalMediationAvailable"] = () => Promise.resolve(autofill);
  Object.defineProperty(globalThis, "PublicKeyCredential", { configurable: true, value: statics });
  Object.defineProperty(navigator, "credentials", {
    configurable: true,
    value: { create: browser.create, get: browser.get },
  });
  return browser;
}
