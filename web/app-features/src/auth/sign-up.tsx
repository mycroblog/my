// SPDX-License-Identifier: AGPL-3.0-only

// The registration screen: the form of the server's registration mode (an invite code, a reason for a moderator), the
// CAPTCHA, and the agreement to the server's documents; the same form without a password for an account at a login
// provider. Each field's error sits under it, and the summary right before the button.

import {
  Button,
  CheckField,
  Form,
  FormActions,
  FormHead,
  FormLink,
  FormLinks,
  FormList,
  FormOthers,
  FormText,
  TextField,
} from "@my/design-system";
import type { AuthPage, SignUpForm } from "./data";
import { AuthFrame, FormState, Hidden, paths, Refused, useAuthStrings } from "./frame";
import { HumanCheck } from "./human-check";
import { ProviderButtons } from "./sign-in";

function SignInLink() {
  const t = useAuthStrings();
  return (
    <FormLinks>
      <FormLink href={paths.signIn}>{t("auth.sign_in.title")}</FormLink>
    </FormLinks>
  );
}

/** The agreement to the server's documents, which it asks for while it has published one. */
function Agreement({ page, form }: { page: AuthPage; form: SignUpForm }) {
  const t = useAuthStrings();
  const documents = page.documents ?? [];
  if (documents.length === 0) return null;
  return (
    <>
      <FormText id="sign-up-agreement">{t("auth.sign_up.agreement")}</FormText>
      <FormList
        items={documents.map((d) => (
          <FormLink key={d.path} href={d.path} newTab={t("auth.link.new_tab")}>
            {d.title}
          </FormLink>
        ))}
      />
      <CheckField
        id="sign-up-agreement-check"
        name="agreement"
        value="yes"
        label={t("auth.sign_up.agreement.label")}
        defaultChecked={form.agreed}
        required
        error={form.errors?.["agreement"]}
        describedBy="sign-up-agreement"
      />
    </>
  );
}

export function SignUp({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  const form = page.signUp;
  const external = form?.external;
  const title = external ? t("auth.external.sign_up.title", { provider: external.provider }) : t("auth.sign_up.title");
  const providers = page.external ?? [];
  if (!form || form.closed)
    return (
      <AuthFrame page={page} title={title}>
        <FormHead title={title}>
          <FormText>{t("auth.sign_up.closed")}</FormText>
        </FormHead>
        <SignInLink />
      </AuthFrame>
    );
  if (page.loginProvidersOnly && !external)
    return (
      <AuthFrame page={page} title={title}>
        <FormHead title={title}>
          {!page.error && <FormText>{t("auth.sign_up.login_providers_only")}</FormText>}
        </FormHead>
        <Refused message={page.error} />
        {providers.length > 0 && (
          <FormOthers heading={t("auth.external.heading_only")}>
            <ProviderButtons external={providers} />
          </FormOthers>
        )}
        <SignInLink />
      </AuthFrame>
    );
  const errors = form.errors ?? {};
  return (
    <AuthFrame page={page} title={title} documents={false}>
      <FormHead title={title} />
      <Form action={external ? paths.externalSignUp : paths.signUp}>
        <FormState page={page} returnTo={false} />
        <Hidden name="ticket" value={external?.ticket} />
        <TextField
          id="sign-up-username"
          name="username"
          label={t("auth.sign_up.username_label")}
          hint={form.usernameHint}
          error={errors["username"]}
          defaultValue={form.username}
          autoComplete="nickname"
          autoCapitalize="none"
          maxLength={form.usernameMax}
          required
        />
        <TextField
          id="sign-up-email"
          name="email"
          type="email"
          label={t("auth.sign_up.email_label")}
          hint={
            external
              ? (external.emailVerifiedHint ?? t("auth.external.sign_up.email_hint"))
              : t("auth.sign_up.email_hint")
          }
          error={errors["email"]}
          defaultValue={form.email}
          autoComplete="username"
          autoCapitalize="none"
          required
        />
        {!external && (
          <TextField
            id="sign-up-password"
            name="password"
            type="password"
            label={t("auth.sign_up.password_label")}
            hint={form.passwordHint}
            error={errors["password"]}
            autoComplete="new-password"
            minLength={form.passwordMin}
            maxLength={form.passwordMax}
            required
          />
        )}
        {form.askInvite && (
          <TextField
            id="sign-up-invite"
            name="invite_code"
            label={t("auth.sign_up.invite_label")}
            hint={form.inviteHint}
            error={errors["invite_code"]}
            defaultValue={form.inviteCode}
            autoComplete="off"
            autoCapitalize="characters"
            required={form.needInvite}
          />
        )}
        {form.askReason && (
          <TextField
            id="sign-up-reason"
            name="reason"
            label={t("auth.sign_up.reason_label")}
            hint={form.reasonHint}
            error={errors["reason"]}
            defaultValue={form.reason}
            maxLength={form.reasonMax}
            multiline
          />
        )}
        {page.humanCheck && <HumanCheck check={page.humanCheck} />}
        <Agreement page={page} form={form} />
        <FormActions>
          <Refused message={page.error} />
          <Button kind="primary" type="submit">
            {t("auth.sign_up.submit")}
          </Button>
        </FormActions>
      </Form>
      {!external && providers.length > 0 && (
        <FormOthers heading={t("auth.external.heading")}>
          <ProviderButtons external={providers} />
        </FormOthers>
      )}
      <SignInLink />
    </AuthFrame>
  );
}
