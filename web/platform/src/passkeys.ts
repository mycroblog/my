// SPDX-License-Identifier: AGPL-3.0-only

// Passkeys (FR-004) in the browser: whether it has them and offers them in a field's autofill (conditional mediation),
// and the two ceremonies with the server's JSON. The options come as the JSON that
// PublicKeyCredential.parseCreationOptionsFromJSON and parseRequestOptionsFromJSON read, and the credential goes back
// as the JSON of its toJSON (internal/app/identity, PasskeyCeremonies); webauthn-json.ts stands in for the three
// where the browser lacks them. A page runs one ceremony at a time, as the browser does: a new one aborts the one
// before it, so that a press of "Sign in with a passkey" ends the autofill's request. A failure throws a PasskeyError
// whose reason is one of a few that a screen can word.

import { creationOptions, credentialJSON, requestOptions, type CredentialLike } from "./webauthn-json";

/**
 * Why a ceremony gave no passkey:
 * - `aborted`: the caller's signal, or a newer ceremony, ended it; a screen says nothing.
 * - `cancelled`: the person closed the browser's prompt, it timed out, or the browser chose not to say why
 *   (NotAllowedError, which it also gives when no passkey of the site is there).
 * - `exists`: the authenticator holds a passkey of this account already (a registration's InvalidStateError).
 * - `unsupported`: the browser has no passkeys, or no passkey autofill when it was asked for.
 * - `failed`: anything else: options the browser refused, a site outside the server's relying party, an error of
 *   the authenticator.
 */
export type PasskeyFailure = "aborted" | "cancelled" | "exists" | "unsupported" | "failed";

/** A ceremony that gave no passkey, and why. */
export class PasskeyError extends Error {
  readonly reason: PasskeyFailure;
  constructor(reason: PasskeyFailure, options?: { cause?: unknown }) {
    super(`no passkey: ${reason}`, options);
    this.name = "PasskeyError";
    this.reason = reason;
  }
}

/** A credential as the JSON the server reads (PublicKeyCredential.toJSON). */
export type PasskeyJSON = Record<string, unknown>;

/** The static members of PublicKeyCredential that this reads, each of which a browser of the floor may lack. */
interface Statics {
  parseCreationOptionsFromJSON?: (options: unknown) => PublicKeyCredentialCreationOptions;
  parseRequestOptionsFromJSON?: (options: unknown) => PublicKeyCredentialRequestOptions;
  isConditionalMediationAvailable?: () => Promise<boolean>;
}

/** The browser's WebAuthn, or undefined without it (an insecure context, an old browser, a test). */
function webAuthn(): { credentials: CredentialsContainer; statics: Statics } | undefined {
  const browser = globalThis as unknown as {
    PublicKeyCredential?: Statics;
    navigator?: { credentials?: CredentialsContainer };
  };
  const statics = browser.PublicKeyCredential;
  const credentials = browser.navigator?.credentials;
  return statics === undefined || credentials === undefined ? undefined : { credentials, statics };
}

/** Whether the browser can create passkeys and sign in with them. */
export function supportsPasskeys(): boolean {
  return webAuthn() !== undefined;
}

/**
 * Whether the browser offers passkeys in the autofill of a field marked with `passkeyUsernameField`, so that a sign-in
 * can ask for one when its page opens (`getPasskey(options, { autofill: true })`).
 */
export async function supportsPasskeyAutofill(): Promise<boolean> {
  const statics = webAuthn()?.statics;
  if (statics?.isConditionalMediationAvailable === undefined) return false;
  try {
    return await statics.isConditionalMediationAvailable();
  } catch {
    return false;
  }
}

/** The attributes of the username field of a sign-in whose autofill offers passkeys. */
export const passkeyUsernameField = { autoComplete: "username webauthn" } as const;

/** The ceremony that runs now, which a newer one aborts. */
let current: AbortController | undefined;

/** Why `error` of a ceremony whose signal is `signal` gave no passkey. */
function failureOf(error: unknown, signal: AbortSignal): PasskeyFailure {
  if (error instanceof PasskeyError) return error.reason;
  if (signal.aborted) return "aborted";
  if (error instanceof DOMException) {
    switch (error.name) {
      case "AbortError":
        return "aborted";
      case "NotAllowedError":
        return "cancelled";
      case "InvalidStateError":
        return "exists";
      case "NotSupportedError":
        return "unsupported";
    }
  }
  return "failed";
}

/** The JSON of what the browser answered: its own toJSON where it has one that works, else webauthn-json.ts's. */
function toJSON(credential: Credential | null): PasskeyJSON {
  if (credential?.type !== "public-key") throw new PasskeyError("cancelled");
  const key = credential as Credential & CredentialLike & { toJSON?: () => unknown };
  if (typeof key.toJSON === "function") {
    try {
      const json = key.toJSON();
      if (typeof json === "object" && json !== null) return json as PasskeyJSON;
    } catch {
      // Some credentials of password managers' extensions carry a toJSON that throws; the fallback reads them.
    }
  }
  return credentialJSON(key);
}

/** Runs a ceremony under its own signal, which `signal` and the next ceremony abort, and returns its JSON. */
async function ceremony(
  signal: AbortSignal | undefined,
  run: (
    webAuthn: { credentials: CredentialsContainer; statics: Statics },
    signal: AbortSignal,
  ) => Promise<Credential | null>,
): Promise<PasskeyJSON> {
  current?.abort();
  const controller = new AbortController();
  current = controller;
  const forward = () => {
    controller.abort();
  };
  if (signal?.aborted === true) controller.abort();
  signal?.addEventListener("abort", forward, { once: true });
  try {
    const browser = webAuthn();
    if (browser === undefined) throw new PasskeyError("unsupported");
    if (controller.signal.aborted) throw new PasskeyError("aborted");
    return toJSON(await run(browser, controller.signal));
  } catch (error) {
    throw new PasskeyError(failureOf(error, controller.signal), { cause: error });
  } finally {
    signal?.removeEventListener("abort", forward);
    if (current === controller) current = undefined;
  }
}

/**
 * Creates a passkey with the server's registration options (PublicKeyCredentialCreationOptionsJSON) and returns the
 * credential as the JSON the server reads. It throws a PasskeyError.
 */
export function createPasskey(options: unknown, { signal }: { signal?: AbortSignal | undefined } = {}) {
  return ceremony(signal, ({ credentials, statics }, own) => {
    const publicKey = statics.parseCreationOptionsFromJSON?.(options) ?? creationOptions(options);
    return credentials.create({ publicKey, signal: own });
  });
}

/**
 * Asks for a passkey with the server's sign-in options (PublicKeyCredentialRequestOptionsJSON) and returns the
 * credential as the JSON the server reads. With `autofill` the browser offers the passkeys in the autofill of the
 * field marked with `passkeyUsernameField` instead of a prompt, and the request waits until the person picks one or
 * it is aborted; it fails as `unsupported` where `supportsPasskeyAutofill` is false. It throws a PasskeyError.
 */
export function getPasskey(
  options: unknown,
  { signal, autofill = false }: { signal?: AbortSignal | undefined; autofill?: boolean | undefined } = {},
) {
  return ceremony(signal, async ({ credentials, statics }, own) => {
    if (autofill && !(await supportsPasskeyAutofill())) throw new PasskeyError("unsupported");
    const publicKey = statics.parseRequestOptionsFromJSON?.(options) ?? requestOptions(options);
    return credentials.get({ publicKey, signal: own, ...(autofill ? { mediation: "conditional" as const } : {}) });
  });
}
