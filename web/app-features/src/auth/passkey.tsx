// SPDX-License-Identifier: AGPL-3.0-only

// Signing in with a passkey (FR-004) through @my/platform: when the page opens, the browser offers the server's
// passkeys in the autofill of the email field (conditional mediation) where it can, and the button asks for one with
// the browser's prompt, which ends the autofill's request. The browser's answer goes to the server in a form with the
// sign-in's challenge, as a password does; a refusal comes back on the sign-in screen. A browser without passkeys gets
// a line that says so in place of the button.

import { Button, FormAlert, FormText, GlyphLabel, Form } from "@my/design-system";
import { getPasskey, PasskeyError, supportsPasskeys, type PasskeyFailure } from "@my/platform";
import { use, useEffect, useRef, useState } from "react";
import type { AuthPage, Passkey } from "./data";
import { FocusOnLoadContext, FormState, paths, useAuthStrings } from "./frame";

/** The passkey's answer once the browser gave one, and why the last request gave none. */
function usePasskeySignIn(passkey: Passkey) {
  const [credential, setCredential] = useState<string>();
  const [failure, setFailure] = useState<PasskeyFailure>();
  const [busy, setBusy] = useState(false);
  const form = useRef<HTMLFormElement>(null);
  useEffect(() => {
    if (credential !== undefined) form.current?.submit();
  }, [credential]);
  useEffect(() => {
    const ended = new AbortController();
    getPasskey(passkey.options, { autofill: true, signal: ended.signal }).then(
      (answer) => {
        setCredential(JSON.stringify(answer));
      },
      // No autofill, or the person chose none before the page closed: the button stays.
      () => undefined,
    );
    return () => {
      ended.abort();
    };
  }, [passkey.options]);
  const ask = () => {
    setBusy(true);
    setFailure(undefined);
    getPasskey(passkey.options).then(
      (answer) => {
        setCredential(JSON.stringify(answer));
      },
      (error: unknown) => {
        setBusy(false);
        const reason = error instanceof PasskeyError ? error.reason : "failed";
        if (reason !== "aborted") setFailure(reason);
      },
    );
  };
  return { form, credential, failure, busy, ask };
}

/** The passkey's button, with the server's refusal or the browser's failure right before it. */
export function PasskeySignIn({ page, passkey }: { page: AuthPage; passkey: Passkey }) {
  const t = useAuthStrings();
  const focusOnLoad = use(FocusOnLoadContext);
  const { form, credential, failure, busy, ask } = usePasskeySignIn(passkey);
  if (!supportsPasskeys()) return <FormText muted>{t("auth.sign_in.passkey_unsupported")}</FormText>;
  const message =
    failure === "unsupported"
      ? t("auth.sign_in.passkey_unsupported")
      : failure
        ? t("auth.sign_in.passkey_cancelled")
        : passkey.error;
  return (
    <Form action={paths.passkey} ref={form}>
      <FormState page={page} />
      <input type="hidden" name="challenge" value={passkey.challenge} />
      <input type="hidden" name="credential" value={credential ?? ""} />
      {/* The server's refusal takes the focus when the page opens; the browser's failure is announced. */}
      {message && <FormAlert focus={focusOnLoad && failure === undefined}>{message}</FormAlert>}
      <Button kind="secondary" type="button" busy={busy || credential !== undefined} onPress={ask}>
        <GlyphLabel icon="passkey">{t("auth.sign_in.passkey")}</GlyphLabel>
      </Button>
    </Form>
  );
}
