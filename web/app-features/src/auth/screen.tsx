// SPDX-License-Identifier: AGPL-3.0-only

// The auth screens (ADR-0090 §16): the screen that the server's data names, which the auth entry renders on its own
// page. Each screen's forms post to the server, which answers with the next screen or a redirect.

import { Authorize } from "./authorize";
import type { AuthPage } from "./data";
import { FocusOnLoadContext, WordmarkContext, type Wordmark } from "./frame";
import {
  AccountDeleted,
  ErrorScreen,
  ForgotPassword,
  MailUnsubscribe,
  MailUnsubscribed,
  PasswordReset,
  ResetPassword,
  SignedIn,
  Verified,
  VerifyEmail,
} from "./messages";
import { SignIn, TwoFactor } from "./sign-in";
import { SignUp } from "./sign-up";
import { Status } from "./status";

/**
 * The screen of `page`, with the built-in wordmark that the app gives (the brand is the app's to place). `preview`
 * shows it among others, where no refusal takes the focus.
 */
export function AuthScreen({
  page,
  wordmark,
  preview = false,
}: {
  page: AuthPage;
  wordmark: Wordmark;
  preview?: boolean;
}) {
  return (
    <WordmarkContext value={wordmark}>
      <FocusOnLoadContext value={!preview}>
        <Screen page={page} />
      </FocusOnLoadContext>
    </WordmarkContext>
  );
}

function Screen({ page }: { page: AuthPage }) {
  switch (page.screen) {
    case "sign_in":
      return <SignIn page={page} />;
    case "two_factor":
      return <TwoFactor page={page} />;
    case "signed_in":
      return <SignedIn page={page} />;
    case "sign_up":
      return <SignUp page={page} />;
    case "status":
      return <Status page={page} />;
    case "verify_email":
      return <VerifyEmail page={page} />;
    case "verified":
      return <Verified page={page} />;
    case "account_deleted":
      return <AccountDeleted page={page} />;
    case "authorize":
      return page.consent ? <Authorize page={{ ...page, consent: page.consent }} /> : <ErrorScreen page={page} />;
    case "forgot_password":
      return <ForgotPassword page={page} />;
    case "reset_password":
      return <ResetPassword page={page} />;
    case "password_reset":
      return <PasswordReset page={page} />;
    case "mail_unsubscribe":
      return <MailUnsubscribe page={page} />;
    case "mail_unsubscribed":
      return <MailUnsubscribed page={page} />;
    case "error":
      return <ErrorScreen page={page} />;
  }
}
