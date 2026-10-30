// SPDX-License-Identifier: AGPL-3.0-only
// The theme screen below its heading (manage_settings, ADR-0059): the color tokens, one row each with its light and
// dark value side by side, the contrast of their pairs as they are typed, the other tokens (type, spacing, radius,
// elevation, motion), the custom CSS with what the server does not check, and a preview of the overrides. An empty
// input keeps the default, which its description names. The reasons the server refuses a theme stay above the button
// (role="alert", web-client/README.md E2); what a save did stays on the page in a StatusMessage.
import {
  Button,
  Grid,
  Hint,
  InlineError,
  Stack,
  StatusMessage,
  Text,
  TextArea,
  TextField,
  ThemePreview,
  VisuallyHidden,
} from "@my/ui";
import { useT } from "../../../lib/i18n";
import { type ColorField, useThemeSettings, type ValueField } from "../hooks/useThemeSettings";
import { Section, RequirePermission } from "../../moderation";

export function ThemeSettings() {
  return (
    <RequirePermission permission="manage_settings">
      <ThemeSettingsForm />
    </RequirePermission>
  );
}

function ThemeSettingsForm() {
  const t = useT();
  const form = useThemeSettings();
  return (
    <Stack as="form" gap="6" onSubmit={form.submit}>
      <Hint size="inherit">{t("client:admin.theme.description")}</Hint>
      <Section surface="card" title={t("client:admin.theme.colors.title")}>
        <Hint size="inherit">{t("client:admin.theme.colors.description")}</Hint>
        <Stack as="ul" container gap="4">
          {form.colorFields.map((field) => (
            <ColorInputs key={field.token} field={field} />
          ))}
        </Stack>
      </Section>
      <Section surface="card" title={t("client:admin.theme.contrast.title")}>
        {form.failingPairs.length === 0 && form.malformedColors.length === 0 ? (
          <p>{t("client:admin.theme.contrast.ok")}</p>
        ) : (
          <Stack as="ul" gap="1" marker="disc" tone="danger">
            {form.malformedColors.map((text) => (
              <li key={text}>{text}</li>
            ))}
            {form.failingPairs.map((pair) => (
              <li key={pair.key}>{pair.text}</li>
            ))}
          </Stack>
        )}
      </Section>
      <Section surface="card" title={t("client:admin.theme.tokens.title")}>
        <Hint size="inherit">{t("client:admin.theme.tokens.description")}</Hint>
        {form.valueFields.map((field) => (
          <ValueInput key={field.token.name} field={field} />
        ))}
      </Section>
      <Section surface="card" title={t("client:admin.theme.css.title")}>
        <Hint size="inherit">{t("client:admin.theme.css.description")}</Hint>
        <TextArea
          label={t("client:admin.theme.css.label")}
          value={form.customCss.value}
          onChange={form.customCss.onChange}
          rows={8}
          {...(form.customCss.error === undefined ? {} : { isInvalid: true, errorMessage: form.customCss.error })}
        />
      </Section>
      <Section surface="card" title={t("client:admin.theme.preview.title")}>
        <Hint size="inherit">{t("client:admin.theme.preview.description")}</Hint>
        <Preview style={form.previewStyle} />
      </Section>
      {form.saveErrors !== undefined && (
        <Stack gap="1" tone="danger" role="alert">
          <p>{t("client:admin.theme.save_refused")}</p>
          <Stack as="ul" gap="1" marker="disc">
            {form.saveErrors.map((text) => (
              <li key={text}>{text}</li>
            ))}
          </Stack>
        </Stack>
      )}
      {form.saveError !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.theme.save_failed", { reason: form.saveError })}
        </InlineError>
      )}
      <div>
        <Button type="submit" variant="primary" isPending={form.isSaving}>
          {t("client:admin.theme.save")}
        </Button>
      </div>
      <StatusMessage>{form.status}</StatusMessage>
    </Stack>
  );
}

function ColorInputs({ field }: { field: ColorField }) {
  const t = useT();
  // The row shows the token name once; each field's label repeats it only for assistive technology, so the field
  // keeps its token in its name ("color-accent Light") while the screen shows "Light".
  const token = <VisuallyHidden>{field.token}</VisuallyHidden>;
  return (
    <Grid as="li" columns={3} at="md" gap="2">
      <Text as="code" size="sm" weight="semibold" mono dir="ltr">
        {field.token}
      </Text>
      <TextField
        label={
          <>
            {token} {t("client:admin.theme.color.light")}
          </>
        }
        description={t("client:admin.theme.color.default", { value: field.defaults.light })}
        value={field.light.value}
        onChange={field.light.onChange}
        placeholder={field.defaults.light}
        autoComplete="off"
      />
      <TextField
        label={
          <>
            {token} {t("client:admin.theme.color.dark")}
          </>
        }
        description={t("client:admin.theme.color.default", { value: field.defaults.dark })}
        value={field.dark.value}
        onChange={field.dark.onChange}
        placeholder={field.defaults.dark}
        autoComplete="off"
      />
    </Grid>
  );
}

function ValueInput({ field }: { field: ValueField }) {
  const t = useT();
  const { token } = field;
  const value = token.value ?? "";
  let description: string;
  if (token.kind === "length" && token.min !== undefined && token.max !== undefined) {
    description = t("client:admin.theme.bounds.length", { value, min: String(token.min), max: String(token.max) });
  } else if (token.kind === "duration" && token.min !== undefined && token.max !== undefined) {
    description = t("client:admin.theme.bounds.duration", { value, min: String(token.min), max: String(token.max) });
  } else if (token.kind === "number" && token.min !== undefined && token.max !== undefined) {
    description = t("client:admin.theme.bounds.number", { value, min: String(token.min), max: String(token.max) });
  } else {
    description = t("client:admin.theme.bounds.default", { value });
  }
  return (
    <TextField
      label={token.name}
      description={description}
      value={field.input.value}
      onChange={field.input.onChange}
      autoComplete="off"
    />
  );
}

/** A few parts of the app with the overrides applied to the region's custom properties. */
function Preview({ style }: { style: Record<string, string> }) {
  const t = useT();
  return (
    <ThemePreview
      tokens={style}
      heading={t("client:admin.theme.preview.heading")}
      text={t("client:admin.theme.preview.text")}
      muted={t("client:admin.theme.preview.muted")}
      link={t("client:admin.theme.preview.link")}
      button={t("client:admin.theme.preview.button")}
      danger={t("client:admin.theme.preview.danger")}
    />
  );
}
