// SPDX-License-Identifier: AGPL-3.0-only

// The consent to a third-party application (ADR-0090 §4): the application as it names itself, what this server knows
// of it, the permissions it asks for by their weight, the account that grants them, and the choice: Deny at the start
// and Allow at the end, both waiting a few seconds after the page opens (consent-wait.ts).

import {
  Button,
  Form,
  FormAccount,
  FormCaution,
  FormChoice,
  FormHead,
  FormList,
  FormSection,
  FormText,
} from "@my/design-system";
import { holdWhileWaiting, useConsentWait } from "./consent-wait";
import type { AuthPage, Consent } from "./data";
import { AuthFrame, Hidden, paths, useAuthStrings } from "./frame";

/** The authorization request that the form posts back with the choice. */
function Request({ consent }: { consent: Consent }) {
  return (
    <>
      <Hidden name="response_type" value={consent.responseType} />
      <Hidden name="client_id" value={consent.clientId} />
      <Hidden name="redirect_uri" value={consent.redirectUri} />
      <input type="hidden" name="scope" value={consent.scope} />
      <Hidden name="state" value={consent.state} />
      <Hidden name="code_challenge" value={consent.codeChallenge} />
      <Hidden name="code_challenge_method" value={consent.codeChallengeMethod} />
    </>
  );
}

export function Authorize({ page }: { page: AuthPage & { consent: Consent } }) {
  const t = useAuthStrings();
  const { consent } = page;
  const left = useConsentWait();
  const waiting = left > 0;
  const title = t("auth.authorize.title", { app: consent.clientName });
  return (
    <AuthFrame page={page} title={title}>
      <FormHead title={title}>
        <FormCaution>{consent.unverified}</FormCaution>
        <FormText muted>{consent.origin}</FormText>
        {consent.website && <FormText muted>{consent.website}</FormText>}
      </FormHead>
      {consent.impersonation && <FormCaution strong>{consent.impersonation}</FormCaution>}
      <FormText>{t("auth.authorize.lead", { app: consent.clientName })}</FormText>
      {consent.scopeGroups.map((group) => (
        <FormSection key={group.risk} id={`scopes-${group.risk}`} heading={group.title}>
          <FormList
            items={group.scopes.map((scope) => (
              <>
                {scope.text}
                {scope.cautions?.map((caution) => (
                  <FormText key={caution} muted>
                    {caution}
                  </FormText>
                ))}
              </>
            ))}
          />
        </FormSection>
      ))}
      <Form
        action={paths.authorize}
        onSubmit={(event) => {
          holdWhileWaiting(left, event);
        }}
      >
        <Hidden name="csrf_token" value={page.csrfToken} />
        <Request consent={consent} />
        <FormText muted>{t("auth.authorize.redirect_notice", { host: consent.redirectHost })}</FormText>
        {consent.account && (
          <FormAccount name={consent.account.displayName || consent.account.handle}>
            {t("auth.authorize.allowing_as", { account: consent.account.handle })}
          </FormAccount>
        )}
        {waiting && <FormText muted>{t("auth.authorize.wait", { seconds: left })}</FormText>}
        {/* Deny at the start, Allow at the end: the row follows the direction of the text. */}
        <FormChoice>
          <Button kind="secondary" type="submit" name="decision" value="deny" isDisabled={waiting}>
            {t("auth.authorize.deny")}
          </Button>
          <Button kind="primary" type="submit" name="decision" value="approve" isDisabled={waiting}>
            {t("auth.authorize.approve")}
          </Button>
        </FormChoice>
      </Form>
    </AuthFrame>
  );
}
