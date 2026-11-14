// SPDX-License-Identifier: AGPL-3.0-only

// The screen of a signed-in account that cannot use the app yet: a registration waiting for a moderator or refused, an
// email address to confirm, moderation actions that keep it out (each with its appeal), or an account its owner
// deactivated (to reactivate, or to delete now, which sits last and apart). Signing out ends it.

import { Button, Form, FormActions, FormBadge, FormHead, FormSection, FormText, TextField } from "@my/design-system";
import type { AuthPage, Status as StatusData, StatusAction } from "./data";
import { AuthFrame, FormState, Hidden, paths, Refused, useAuthStrings } from "./frame";

/** The form that signs the browser out. */
export function SignOut({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  return (
    <Form action={paths.signOut}>
      <FormState page={page} returnTo={false} />
      <FormActions>
        <Button kind="secondary" type="submit">
          {t("auth.sign_out.submit")}
        </Button>
      </FormActions>
    </Form>
  );
}

/** A moderation action in effect, with its appeal, the form to appeal it, or why it cannot be appealed. */
function Action({ page, action }: { page: AuthPage; action: StatusAction }) {
  const t = useAuthStrings();
  const field = `appeal-${action.id}`;
  return (
    <FormSection
      id={`action-${action.id}`}
      heading={
        <>
          {action.kind}
          <FormBadge>{t("auth.status.action_active")}</FormBadge>
        </>
      }
    >
      <FormText>{action.endsAt ? <time dateTime={action.endsAt}>{action.ends}</time> : action.ends}</FormText>
      <FormText muted>{action.category}</FormText>
      {action.reason && <FormText>{action.reason}</FormText>}
      {action.appeal ? (
        <>
          <FormText>
            <FormBadge>{action.appeal.stateLabel}</FormBadge>
          </FormText>
          <FormText>{action.appeal.state}</FormText>
          <FormText>{action.appeal.text}</FormText>
          {action.appeal.answer && (
            <>
              <FormText muted>{t("auth.status.appeal_answer")}</FormText>
              <FormText>{action.appeal.answer}</FormText>
            </>
          )}
        </>
      ) : action.canAppeal ? (
        <Form action={paths.appeal}>
          <FormState page={page} returnTo={false} />
          <Hidden name="action_id" value={action.id} />
          <TextField
            id={field}
            name="text"
            label={t("auth.status.appeal_label")}
            hint={action.appealHint}
            error={action.appealError}
            defaultValue={action.appealText}
            maxLength={action.appealMax}
            required
            multiline
          />
          <FormActions>
            <Button kind="primary" type="submit">
              {t("auth.status.appeal_submit")}
            </Button>
          </FormActions>
        </Form>
      ) : (
        <FormText muted>{action.noAppeal}</FormText>
      )}
    </FormSection>
  );
}

/** A deactivated account: when it goes, its reactivation, and its deletion now, last and apart. */
function Deactivated({ page, status }: { page: AuthPage; status: StatusData }) {
  const t = useAuthStrings();
  const deletion = t("auth.status.deactivated_deletion", { date: status.deletionDate ?? "" });
  return (
    <>
      <FormText>{t("auth.status.deactivated")}</FormText>
      {status.deletionDate && (
        <FormText>{status.deletionAt ? <time dateTime={status.deletionAt}>{deletion}</time> : deletion}</FormText>
      )}
      <Form action={paths.reactivate}>
        <FormState page={page} returnTo={false} />
        <FormText muted>{t("auth.status.deactivated_reactivate_hint")}</FormText>
        <FormActions>
          <Button kind="primary" type="submit">
            {t("auth.status.deactivated_reactivate")}
          </Button>
        </FormActions>
      </Form>
      <SignOut page={page} />
      <FormSection id="delete-heading" heading={t("auth.status.deactivated_delete_heading")} apart>
        <Form action={paths.deleteDeactivated}>
          <FormState page={page} returnTo={false} />
          <FormText muted id="delete-hint">
            {t("auth.status.deactivated_delete_hint")}
          </FormText>
          {status.deleteAsksForPassword && (
            <TextField
              id="delete-password"
              name="password"
              type="password"
              label={t("auth.status.deactivated_password_label")}
              autoComplete="current-password"
              describedBy="delete-hint"
              required
            />
          )}
          <FormActions>
            <Button kind="secondary" type="submit">
              {t("auth.status.deactivated_delete")}
            </Button>
          </FormActions>
        </Form>
      </FormSection>
    </>
  );
}

export function Status({ page }: { page: AuthPage }) {
  const t = useAuthStrings();
  const status = page.status ?? {};
  const title = status.deactivated
    ? t("auth.status.deactivated_title")
    : status.restricted
      ? t("auth.status.restricted_title")
      : t("auth.status.title");
  return (
    <AuthFrame page={page} title={title}>
      <FormHead title={title}>{page.notice && <FormText role="status">{page.notice}</FormText>}</FormHead>
      {/* The screen holds several forms, so what the server refused sits under the title and takes the focus. */}
      <Refused message={page.error} />
      {status.deactivated ? (
        <Deactivated page={page} status={status} />
      ) : (
        <>
          {status.restricted && (
            <>
              <FormText>{t("auth.status.restricted")}</FormText>
              {status.actions?.map((action) => (
                <Action key={action.id} page={page} action={action} />
              ))}
            </>
          )}
          {status.rejected ? (
            <FormText>{t("auth.status.rejected")}</FormText>
          ) : (
            status.pending && <FormText>{t("auth.status.pending")}</FormText>
          )}
          {status.verifyEmail && (
            <Form action={paths.resendVerification}>
              <FormState page={page} returnTo={false} />
              <FormText>{status.verifyEmail}</FormText>
              <FormActions>
                <Button kind="primary" type="submit">
                  {t("auth.status.resend_submit")}
                </Button>
              </FormActions>
            </Form>
          )}
          <SignOut page={page} />
        </>
      )}
    </AuthFrame>
  );
}
