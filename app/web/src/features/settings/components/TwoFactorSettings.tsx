// SPDX-License-Identifier: AGPL-3.0-only
// The two-factor authentication page below its heading (FR-005): where it stands; while it is off, enrolling an
// authenticator app (the secret as a QR code and as text to type, then the app's first code); while it is on,
// replacing the recovery codes and turning it off, each confirmed by a code. The recovery codes that turning it
// on or replacing them answers are listed once. What a change did stays on the page in a StatusMessage
// (role="status"); a wrong code is the code field's error and any other failure an alert next to its form
// (web-client/README.md E2, web-client/16-look-states-and-wording.md G16).
import {
  Box,
  Button,
  CodeList,
  DescriptionItem,
  DescriptionList,
  Hint,
  InlineError,
  QrCode,
  Section,
  Stack,
  StatusMessage,
  Text,
  TextField,
} from "@my/ui";
import { useId } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { twoFactorCodeLimit, type TotpSetup } from "../api/twoFactor";
import { type CodeForm, useTwoFactor } from "../hooks/useTwoFactor";

export function TwoFactorSettings() {
  const t = useT();
  const intl = useIntl();
  const settings = useTwoFactor();
  const { status } = settings;
  return (
    <Stack container gap="8">
      <StatusMessage>{settings.result}</StatusMessage>
      <Section title={t("client:settings.two_factor.status.title")}>
        {status.enabled ? (
          <DescriptionList columns={2}>
            <DescriptionItem term={t("client:settings.two_factor.status")}>
              {t("client:settings.two_factor.status.on")}
            </DescriptionItem>
            {status.enrolled_at !== undefined && (
              <DescriptionItem term={t("client:settings.two_factor.enrolled_at")}>
                <time dateTime={status.enrolled_at}>
                  {intl.formatDate(status.enrolled_at, { dateStyle: "medium", timeStyle: "short" })}
                </time>
              </DescriptionItem>
            )}
            {status.recovery_codes_remaining !== undefined && (
              <DescriptionItem term={t("client:settings.two_factor.recovery_codes.remaining")}>
                <Text numeric>{intl.formatNumber(status.recovery_codes_remaining)}</Text>
              </DescriptionItem>
            )}
          </DescriptionList>
        ) : (
          <p>{t("client:settings.two_factor.status.off")}</p>
        )}
      </Section>
      {settings.recoveryCodes !== undefined && <RecoveryCodeList codes={settings.recoveryCodes} />}
      {status.enabled ? (
        <>
          <Section title={t("client:settings.two_factor.recovery_codes.title")}>
            <Hint>{t("client:settings.two_factor.recovery_codes.description")}</Hint>
            <CodeFormView
              form={settings.regenerate}
              submitLabel={t("client:settings.two_factor.recovery_codes.replace")}
            />
          </Section>
          <Section title={t("client:settings.two_factor.disable.title")}>
            <Hint>{t("client:settings.two_factor.disable.description")}</Hint>
            <CodeFormView form={settings.disable} submitLabel={t("client:settings.two_factor.disable.submit")} />
          </Section>
        </>
      ) : (
        <Section title={t("client:settings.two_factor.setup.title")}>
          {settings.setup === null ? (
            <Stack align="start" gap="3">
              <Hint>{t("client:settings.two_factor.setup.description")}</Hint>
              {settings.setupError !== undefined && (
                <InlineError alert size="inherit">
                  {settings.setupError}
                </InlineError>
              )}
              <Button variant="primary" onPress={settings.setUp} isPending={settings.isSettingUp}>
                {t("client:settings.two_factor.setup.start")}
              </Button>
            </Stack>
          ) : (
            <SetupSteps setup={settings.setup} form={settings.enable} />
          )}
        </Section>
      )}
    </Stack>
  );
}

/** Enrolling the app: scan the code or type the secret, then confirm with the app's first code. */
function SetupSteps({ setup, form }: { setup: TotpSetup; form: CodeForm }) {
  const t = useT();
  const secretId = useId();
  return (
    <Stack gap="6">
      <Stack as="ol" marker="decimalInside" gap="1">
        <li>{t("client:settings.two_factor.setup.step.scan")}</li>
        <li>{t("client:settings.two_factor.setup.step.confirm")}</li>
      </Stack>
      <QrCode
        size={setup.qr_code.size}
        path={setup.qr_code.path}
        label={t("client:settings.two_factor.setup.qr_code")}
      />
      <Stack gap="1">
        <Text id={secretId} size="sm" weight="medium">
          {t("client:settings.two_factor.setup.secret")}
        </Text>
        {/* Base32, typed into the app as it is: left to right in every locale, in groups that read easily. */}
        <Text as="code" aria-labelledby={secretId} dir="ltr" mono size="base" wrap="anywhere">
          {setup.secret.match(/.{1,4}/g)?.join(" ") ?? setup.secret}
        </Text>
      </Stack>
      <CodeFormView
        form={form}
        submitLabel={t("client:settings.two_factor.setup.submit")}
        description={t("client:settings.two_factor.setup.code.description")}
      />
    </Stack>
  );
}

/** A form that sends a code of the app (or a recovery code) to confirm what its button says. */
function CodeFormView({
  form,
  submitLabel,
  description,
}: {
  form: CodeForm;
  submitLabel: string;
  description?: string;
}) {
  const t = useT();
  return (
    <Stack as="form" onSubmit={form.submit} align="start" gap="3">
      <Box width="xs">
        <TextField
          label={t("client:settings.two_factor.code")}
          description={description ?? t("client:settings.two_factor.code.description")}
          value={form.code}
          onChange={form.changeCode}
          isRequired
          isInvalid={form.codeError !== undefined}
          maxLength={twoFactorCodeLimit}
          autoComplete="one-time-code"
          spellCheck="false"
          errorMessage={form.codeError ?? t("client:settings.two_factor.code.required")}
        />
      </Box>
      {form.error !== undefined && (
        <InlineError alert size="inherit">
          {form.error}
        </InlineError>
      )}
      <Button type="submit" variant="primary" isPending={form.isPending}>
        {submitLabel}
      </Button>
    </Stack>
  );
}

/** The recovery codes as the server answered them, shown this once. */
function RecoveryCodeList({ codes }: { codes: readonly string[] }) {
  const t = useT();
  return (
    <Section title={t("client:settings.two_factor.recovery_codes.new.title")}>
      <p>{t("client:settings.two_factor.recovery_codes.new.description")}</p>
      <CodeList codes={codes} />
    </Section>
  );
}
