// SPDX-License-Identifier: AGPL-3.0-only

// The short screens of the flow: the browser is signed in, something went wrong, an email address to confirm and
// confirmed, a forgotten password and a new one, an account deleted, and the emails of a notification stopped.

import {
  Button,
  Form,
  FormActions,
  FormHead,
  FormLink,
  FormLinks,
  FormText,
  LinkButton,
  TextField,
} from "@my/design-system";
import type { AuthPage } from "./data";
import { AuthFrame, FormState, Hidden, paths, Refused, useAuthStrings } from "./frame";
import { HumanCheck } from "./human-check";
import { SignOut } from "./status";

function SignInLink({ label }: { label?: string }) {
  const t = useAuthStrings();
  return (
    <FormLinks>
      <FormLink href={paths.signIn}>{label ?? t("auth.sign_in.title")}</FormLink>
    </FormLinks>
  );
}

/** A browser with a session: on to where it was going, or sign out. */
export function SignedIn({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  const title = t("auth.signed_in.title");
  return (
    <AuthFrame page={page} title={title}>
      <FormHead title={title} />
      <FormActions>
        <LinkButton kind="primary" href={page.returnTo ?? "/"}>
          {t("auth.signed_in.continue")}
        </LinkButton>
      </FormActions>
      <SignOut page={page} />
    </AuthFrame>
  );
}

/** What the server could not do, in its words. */
export function ErrorScreen({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  const title = t("auth.error.title");
  return (
    <AuthFrame page={page} title={title}>
      <FormHead title={title} />
      <Refused message={page.error} />
      <SignInLink />
    </AuthFrame>
  );
}

/** The link of a verification email: a button confirms the address, so that a mail scanner does not. */
export function VerifyEmail({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  const title = t("auth.verify_email.title");
  return (
    <AuthFrame page={page} title={title}>
      <FormHead title={title}>
        <FormText>{t("auth.verify_email.prompt")}</FormText>
      </FormHead>
      <Form action={paths.verifyEmail}>
        <FormState page={page} returnTo={false} />
        <Hidden name="token" value={page.token} />
        <FormActions>
          <Button kind="primary" type="submit">
            {t("auth.verify_email.submit")}
          </Button>
        </FormActions>
      </Form>
    </AuthFrame>
  );
}

export function Verified({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  const title = t("auth.verify_email.done_title");
  return (
    <AuthFrame page={page} title={title}>
      <FormHead title={title}>
        <FormText role="status">{t("auth.verify_email.done")}</FormText>
      </FormHead>
      <FormActions>
        <LinkButton kind="primary" href={page.returnTo ?? "/"}>
          {t("auth.signed_in.continue")}
        </LinkButton>
      </FormActions>
    </AuthFrame>
  );
}

export function AccountDeleted({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  const title = t("auth.status.deleted_title");
  return (
    <AuthFrame page={page} title={title}>
      <FormHead title={title}>
        <FormText role="status">{t("auth.status.deleted")}</FormText>
      </FormHead>
    </AuthFrame>
  );
}

/** The request of a link that sets a new password, or why the server cannot send one. */
export function ForgotPassword({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  const title = t("auth.forgot_password.title");
  const view = page.forgotPassword ?? {};
  const note = view.loginProvidersOnly
    ? t("auth.forgot_password.login_providers_only")
    : view.unavailable
      ? t("auth.forgot_password.unavailable")
      : undefined;
  return (
    <AuthFrame page={page} title={title}>
      {note !== undefined ? (
        <FormHead title={title}>
          <FormText>{note}</FormText>
        </FormHead>
      ) : view.sent ? (
        <FormHead title={title}>
          <FormText role="status">{page.notice}</FormText>
        </FormHead>
      ) : (
        <>
          <FormHead title={title}>
            <FormText id="forgot-password-prompt">{t("auth.forgot_password.prompt")}</FormText>
          </FormHead>
          <Form action={paths.forgotPassword}>
            <FormState page={page} returnTo={false} />
            <TextField
              id="forgot-password-email"
              name="email"
              type="email"
              label={t("auth.forgot_password.email_label")}
              defaultValue={page.email}
              autoComplete="username"
              autoCapitalize="none"
              describedBy="forgot-password-prompt"
              required
            />
            {page.humanCheck && <HumanCheck check={page.humanCheck} />}
            <FormActions>
              <Refused message={page.error} />
              <Button kind="primary" type="submit">
                {t("auth.forgot_password.submit")}
              </Button>
            </FormActions>
          </Form>
        </>
      )}
      <SignInLink />
    </AuthFrame>
  );
}

/** The form of a new password that a reset link opens. */
export function ResetPassword({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  const title = t("auth.reset_password.title");
  const field = page.newPassword;
  return (
    <AuthFrame page={page} title={title}>
      <FormHead title={title}>
        <FormText>{t("auth.reset_password.prompt")}</FormText>
      </FormHead>
      <Form action={paths.resetPassword}>
        <FormState page={page} returnTo={false} />
        <Hidden name="token" value={page.token} />
        {field && (
          <TextField
            id="reset-password-password"
            name="password"
            type="password"
            label={t("auth.reset_password.password_label")}
            hint={field.error ? undefined : field.hint}
            error={field.error}
            autoComplete="new-password"
            minLength={field.min}
            maxLength={field.max}
            required
          />
        )}
        <FormActions>
          <Refused message={page.error} />
          <Button kind="primary" type="submit">
            {t("auth.reset_password.submit")}
          </Button>
        </FormActions>
      </Form>
    </AuthFrame>
  );
}

export function PasswordReset({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  const title = t("auth.reset_password.done_title");
  return (
    <AuthFrame page={page} title={title}>
      <FormHead title={title}>
        <FormText role="status">{t("auth.reset_password.done")}</FormText>
      </FormHead>
      <FormActions>
        <LinkButton kind="primary" href={paths.signIn}>
          {t("auth.sign_in.title")}
        </LinkButton>
      </FormActions>
    </AuthFrame>
  );
}

/** The link of a notification email: a button stops its emails, so that a mail scanner does not. */
export function MailUnsubscribe({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  const title = t("auth.mail_unsubscribe.title");
  const action = `${paths.mailUnsubscribe}?${new URLSearchParams({ token: page.token ?? "" }).toString()}`;
  return (
    <AuthFrame page={page} title={title}>
      <FormHead title={title}>
        <FormText>{page.unsubscribe?.prompt}</FormText>
      </FormHead>
      <Form action={action}>
        <input type="hidden" name="List-Unsubscribe" value="One-Click" />
        <FormActions>
          <Button kind="primary" type="submit">
            {t("auth.mail_unsubscribe.submit")}
          </Button>
        </FormActions>
      </Form>
    </AuthFrame>
  );
}

export function MailUnsubscribed({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  const title = t("auth.mail_unsubscribe.done_title");
  return (
    <AuthFrame page={page} title={title}>
      <FormHead title={title}>
        <FormText role="status">{page.unsubscribe?.done}</FormText>
      </FormHead>
    </AuthFrame>
  );
}
