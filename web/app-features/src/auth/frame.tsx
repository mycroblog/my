// SPDX-License-Identifier: AGPL-3.0-only

// What every auth screen shares: its strings, the page around it (the server's mark or introduction and the footer of
// its documents), the document's title, the paths its forms post to, and the hidden fields they carry.

import { FormAlert, FormPage, FormPageFooter, useLocale } from "@my/design-system";
import { createContext, use, useEffect, type ReactNode } from "react";
import { stringsIn } from "../locale";
import type { AuthPage } from "./data";

/** The auth screens' strings in the reader's locale. */
export const useAuthStrings = () => stringsIn(useLocale().locale).t;

/** The URLs of the built-in wordmark, for light and dark, which a server without a logo of its own shows. */
export type Wordmark = { light: string; dark: string };

/** The wordmark that the app gives the screens (AuthScreen). */
export const WordmarkContext = createContext<Wordmark>({ light: "", dark: "" });

/**
 * Whether what the server refused takes the focus when the screen shows: on the page of a screen, not where many
 * screens show at once (a preview).
 */
export const FocusOnLoadContext = createContext(true);

/** The server's endpoints that the screens' forms post to (internal/interface/authpages). */
export const paths = {
  signIn: "/auth/sign_in",
  passkey: "/auth/sign_in/passkey",
  twoFactor: "/auth/sign_in/two_factor",
  signOut: "/auth/sign_out",
  signUp: "/auth/sign_up",
  externalSignUp: "/auth/external/sign_up",
  forgotPassword: "/auth/forgot_password",
  resetPassword: "/auth/reset_password",
  verifyEmail: "/auth/verify_email",
  resendVerification: "/auth/verify_email/resend",
  appeal: "/auth/appeal",
  reactivate: "/auth/reactivate",
  deleteDeactivated: "/auth/deactivated/delete",
  mailUnsubscribe: "/mail/unsubscribe",
  authorize: "/oauth/authorize",
} as const;

/** `path` with the screen's return target, which a link back into the flow keeps. */
export const withReturnTo = (path: string, returnTo: string | undefined) =>
  returnTo ? `${path}?${new URLSearchParams({ return_to: returnTo }).toString()}` : path;

/** A hidden field of a form, left out without a value. */
export function Hidden({ name, value }: { name: string; value: string | undefined }) {
  return value === undefined || value === "" ? null : <input type="hidden" name={name} value={value} />;
}

/** The anti-forgery token and the return target that every form of the flow posts back. */
export function FormState({ page, returnTo = true }: { page: AuthPage; returnTo?: boolean }) {
  return (
    <>
      <Hidden name="csrf_token" value={page.csrfToken} />
      {returnTo && <Hidden name="return_to" value={page.returnTo} />}
    </>
  );
}

/** What the server refused of the screen's form, right before its button, which takes the focus when the page opens. */
export function Refused({ message }: { message: string | undefined }) {
  const focus = use(FocusOnLoadContext);
  return message ? <FormAlert focus={focus}>{message}</FormAlert> : null;
}

/**
 * The page of a screen titled `title`, which also titles the document with the server's name. `documents` false
 * leaves the server's documents out of the footer, for a screen that links them in its body (the registration).
 */
export function AuthFrame({
  page,
  title,
  documents = true,
  children,
}: {
  page: AuthPage;
  title: string;
  documents?: boolean;
  children: ReactNode;
}) {
  const t = useAuthStrings();
  const instance = page.server.name;
  const documentTitle = instance ? t("auth.document_title", { page: title, instance }) : title;
  useEffect(() => {
    document.title = documentTitle;
  }, [documentTitle]);
  const links = [
    ...(documents ? (page.documents ?? []).map((d) => ({ href: d.path, label: d.title })) : []),
    ...(page.statusPageUrl ? [{ href: page.statusPageUrl, label: t("auth.status_page.link") }] : []),
  ];
  return (
    <FormPage
      server={page.server}
      wordmark={use(WordmarkContext)}
      footer={<FormPageFooter label={t("auth.document.footer_label")} links={links} />}
    >
      {children}
    </FormPage>
  );
}
