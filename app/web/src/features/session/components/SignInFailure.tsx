// SPDX-License-Identifier: AGPL-3.0-only
import { Link } from "@my/ui";
import { NetworkError } from "../../../lib/errors";
import { SignInError } from "../../../lib/auth/signIn";
import { type ClientMessageKey, useT } from "../../../lib/i18n";

export type SignInFailureProps = {
  /** What the callback failed with. */
  error: unknown;
};

/** The body of the page shown when the callback could not complete a sign-in: why, and a way to start again. */
export function SignInFailure({ error }: SignInFailureProps) {
  const t = useT();
  return (
    <>
      <p>{t(failureMessage(error))}</p>
      <p>
        <Link href="/login" variant="standalone">
          {t("client:login.failed.try_again")}
        </Link>
      </p>
    </>
  );
}

function failureMessage(error: unknown): ClientMessageKey {
  if (error instanceof NetworkError) return "client:api.error.network";
  if (!(error instanceof SignInError)) return "client:login.failed.rejected";
  switch (error.reason) {
    case "denied":
      return "client:login.failed.denied";
    case "invalid-state":
      return "client:login.failed.invalid_state";
    case "rejected":
      return "client:login.failed.rejected";
  }
}
