// SPDX-License-Identifier: AGPL-3.0-only
// The passkey form of the server's sign-in page (internal/interface/authpages/templates/sign_in.html) in a browser
// with passkeys and in one without.
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { installPasskeys, removePasskeys } from "../test/webauthn";
import { startPasskeySignIn } from "./passkey-sign-in";

/** The passkey form as the server renders it, with the escaping of html/template undone by the parser. */
function showSignInPage(): void {
  document.body.innerHTML = `
    <main>
      <form class="form" method="post" action="/auth/sign_in/passkey" data-passkey-options='{"challenge":"AQI","rpId":"social.example","userVerification":"required"}' hidden>
        <input type="hidden" name="csrf_token" value="csrf">
        <input type="hidden" name="challenge" value="sealed">
        <input type="hidden" name="credential" value="">
        <p id="passkey-error" class="alert" role="alert" data-passkey-error="The passkey was not used." hidden></p>
        <div class="actions"><button class="button" type="submit">Sign in with a passkey</button></div>
      </form>
    </main>`;
}

const passkeyButton = () => screen.getByRole("button", { name: "Sign in with a passkey", hidden: true });

/** The credential field of the form that was sent. */
function sentCredential(): unknown {
  const form: unknown = submit.mock.contexts[0];
  const value = form instanceof HTMLFormElement ? new FormData(form).get("credential") : null;
  return typeof value === "string" ? JSON.parse(value) : undefined;
}

let submit: ReturnType<typeof vi.spyOn<HTMLFormElement, "submit">>;

beforeEach(() => {
  // jsdom does not navigate; the test sees what the form would send.
  submit = vi.spyOn(HTMLFormElement.prototype, "submit").mockImplementation(() => undefined);
});

afterEach(() => {
  removePasskeys();
  document.body.innerHTML = "";
});

test("stays hidden in a browser without passkeys", () => {
  showSignInPage();
  startPasskeySignIn(document);
  expect(passkeyButton()).not.toBeVisible();
});

test("asks the browser for a passkey and sends its answer", async () => {
  const credentials = installPasskeys();
  showSignInPage();
  startPasskeySignIn(document);
  expect(passkeyButton()).toBeVisible();

  await userEvent.click(passkeyButton());
  await waitFor(() => {
    expect(submit).toHaveBeenCalledOnce();
  });
  const options = credentials.get.mock.calls[0]?.[0].publicKey;
  expect(options?.rpId).toBe("social.example");
  expect(options?.userVerification).toBe("required");
  expect(sentCredential()).toMatchObject({
    id: "AQID",
    type: "public-key",
    response: { userHandle: "dTE", signature: "CQ" },
  });
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

test("says so when the prompt is dismissed, and lets the user try again", async () => {
  const credentials = installPasskeys();
  credentials.get.mockRejectedValueOnce(new DOMException("dismissed", "NotAllowedError"));
  showSignInPage();
  startPasskeySignIn(document);

  const button = passkeyButton();
  await userEvent.click(button);
  expect(await screen.findByRole("alert")).toHaveTextContent("The passkey was not used.");
  expect(button).toBeEnabled();
  expect(submit).not.toHaveBeenCalled();

  await userEvent.click(button);
  await waitFor(() => {
    expect(submit).toHaveBeenCalledOnce();
  });
});
