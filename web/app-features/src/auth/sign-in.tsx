// SPDX-License-Identifier: AGPL-3.0-only

// The sign-in screen and the code step of two-factor authentication. The password form posts to the server, with the
// CAPTCHA when the server has one; a passkey and the login providers are the other ways in. While only the login
// providers sign in, the screen offers them alone.

import {
  Button,
  Form,
  FormActions,
  FormHead,
  FormLink,
  FormLinks,
  FormOthers,
  FormText,
  GlyphLabel,
  LinkButton,
  TextField,
  type IconName,
} from "@my/design-system";
import { oneTimeCodeField, passkeyUsernameField } from "@my/platform";
import type { AuthPage, ExternalButton } from "./data";
import { AuthFrame, FormState, paths, Refused, useAuthStrings, withReturnTo } from "./frame";
import { HumanCheck } from "./human-check";
import { PasskeySignIn } from "./passkey";

/** The mark of a login provider of a kind the design system has a glyph for. */
const providerGlyphs: Readonly<Record<string, IconName>> = { github: "github", discord: "discord" };

/** The links to the login providers' starts, as buttons. */
export function ProviderButtons({ external }: { external: ExternalButton[] }) {
  return external.map((provider) => {
    const glyph = providerGlyphs[provider.kind];
    return (
      <LinkButton key={provider.url} href={provider.url}>
        {glyph ? <GlyphLabel icon={glyph}>{provider.label}</GlyphLabel> : provider.label}
      </LinkButton>
    );
  });
}

export function SignIn({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  const title = t("auth.sign_in.title");
  const external = page.external ?? [];
  if (page.loginProvidersOnly)
    return (
      <AuthFrame page={page} title={title}>
        <FormHead title={title}>
          {!page.error && <FormText>{t("auth.sign_in.login_providers_only")}</FormText>}
        </FormHead>
        <Refused message={page.error} />
        {external.length > 0 && (
          <FormOthers heading={t("auth.external.heading_only")}>
            <ProviderButtons external={external} />
          </FormOthers>
        )}
      </AuthFrame>
    );
  return (
    <AuthFrame page={page} title={title}>
      <FormHead title={title} />
      <Form action={paths.signIn}>
        <FormState page={page} />
        <TextField
          id="sign-in-email"
          name="email"
          type="email"
          label={t("auth.sign_in.email_label")}
          defaultValue={page.email}
          autoComplete={page.passkey ? passkeyUsernameField.autoComplete : "username"}
          autoCapitalize="none"
          required
        />
        <TextField
          id="sign-in-password"
          name="password"
          type="password"
          label={t("auth.sign_in.password_label")}
          autoComplete="current-password"
          required
          end={
            page.canResetPassword && (
              <FormLink href={paths.forgotPassword}>{t("auth.sign_in.forgot_password_link")}</FormLink>
            )
          }
        />
        {page.humanCheck && <HumanCheck check={page.humanCheck} />}
        <FormActions>
          <Refused message={page.error} />
          <Button kind="primary" type="submit">
            {t("auth.sign_in.submit")}
          </Button>
        </FormActions>
      </Form>
      {(page.passkey || external.length > 0) && (
        <FormOthers heading={t("auth.external.heading")}>
          {page.passkey && <PasskeySignIn page={page} passkey={page.passkey} />}
          <ProviderButtons external={external} />
        </FormOthers>
      )}
      {page.canSignUp && (
        <FormLinks>
          <FormLink href={paths.signUp}>{t("auth.sign_in.sign_up_link")}</FormLink>
        </FormLinks>
      )}
    </AuthFrame>
  );
}

export function TwoFactor({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  const title = t("auth.two_factor.title");
  return (
    <AuthFrame page={page} title={title}>
      <FormHead title={title}>
        <FormText id="two-factor-prompt">{t("auth.two_factor.prompt")}</FormText>
      </FormHead>
      <Form action={paths.twoFactor}>
        <FormState page={page} />
        <TextField
          id="two-factor-code"
          name="code"
          label={t("auth.two_factor.code_label")}
          autoComplete={oneTimeCodeField.autoComplete}
          inputMode="text"
          autoCapitalize="none"
          maxLength={64}
          required
          describedBy="two-factor-prompt"
        />
        <FormActions>
          <Refused message={page.error} />
          <Button kind="primary" type="submit">
            {t("auth.two_factor.submit")}
          </Button>
        </FormActions>
      </Form>
      <FormLinks>
        <FormLink href={withReturnTo(paths.signIn, page.returnTo)}>{t("auth.two_factor.restart_link")}</FormLink>
      </FormLinks>
    </AuthFrame>
  );
}
