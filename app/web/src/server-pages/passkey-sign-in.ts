// SPDX-License-Identifier: AGPL-3.0-only
// The passkey button of the sign-in page that the Go server renders without JavaScript (ADR-0090, FR-004). The page
// carries a hidden form with the options of a sign-in in data-passkey-options; in a browser with passkeys, this
// script shows the form and, when it is sent, asks the browser for a passkey and sends its answer in the credential
// field, so that the server signs in as with the password form. A prompt that the user dismisses or that fails
// shows the form's alert (its text is in data-passkey-error) and leaves the form to try again. The page's
// Content-Security-Policy admits this site's scripts only; the script writes text, never markup.
import { getPasskey, supportsPasskeys } from "../lib/webauthn";

/** Shows the passkey form of `root` and runs it; does nothing without one or without passkeys. */
export function startPasskeySignIn(root: ParentNode): void {
  const form = root.querySelector<HTMLFormElement>("form[data-passkey-options]");
  if (form === null || !supportsPasskeys()) return;
  const credential = form.elements.namedItem("credential");
  const button = form.querySelector<HTMLButtonElement>("button[type=submit]");
  const alert = form.querySelector<HTMLElement>("[data-passkey-error]");
  if (!(credential instanceof HTMLInputElement) || button === null || alert === null) return;
  form.hidden = false;
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    if (button.disabled) return;
    button.disabled = true;
    alert.hidden = true;
    void (async () => {
      try {
        const options: unknown = JSON.parse(form.dataset["passkeyOptions"] ?? "");
        credential.value = JSON.stringify(await getPasskey(options));
        // submit() sends the form without another submit event.
        form.submit();
      } catch {
        alert.textContent = alert.dataset["passkeyError"] ?? "";
        alert.hidden = false;
        button.disabled = false;
      }
    })();
  });
}

startPasskeySignIn(document);
