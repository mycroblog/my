// SPDX-License-Identifier: AGPL-3.0-only
// The translation screen below its heading (web-client/screens/admin-translation.md, ADR-0148, manage_settings): the
// month's characters and cache hit rate, then one form with the providers, what each may receive, the browser's
// translation, the limits, and how long translations are kept. The form says first that the privacy policy must change
// before a server-side provider is enabled, since text sent to one cannot be recalled. "Move up" and "Move down" set the
// order with the keyboard as well as the pointer (no dragging).
import {
  Button,
  Callout,
  Card,
  Checkbox,
  Cluster,
  EmptyState,
  Fieldset,
  FormActionBar,
  type FormActionResult,
  Hint,
  Link,
  RegionError,
  Spinner,
  Stack,
  StatTile,
  StatTileGrid,
  TextField,
} from "@my/ui";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { RequirePermission, Section } from "../../moderation";
import {
  type TranslationOverview,
  type TranslationProvider,
  translationVisibilities,
  type Visibility,
} from "../api/translation";
import { type TranslationNumberKey, useTranslationSettings, useTranslationUse } from "../hooks/useTranslationSettings";

const visibilityLabels: Readonly<Record<Visibility, ClientMessageKey>> = {
  public: "client:admin.translation.visibility.public",
  local: "client:admin.translation.visibility.local",
  unlisted: "client:admin.translation.visibility.unlisted",
  private: "client:admin.translation.visibility.private",
  direct: "client:admin.translation.visibility.direct",
};

export function TranslationSettings() {
  const t = useT();
  return (
    <RequirePermission permission="manage_settings">
      <Stack gap="8">
        <Section surface="card" title={t("client:admin.translation.use.title")}>
          <TranslationUse />
        </Section>
        <TranslationForm />
      </Stack>
    </RequirePermission>
  );
}

function TranslationUse() {
  const t = useT();
  const use = useTranslationUse();
  switch (use.status) {
    case "loading":
      return (
        <div aria-busy="true">
          <Spinner label={t("client:admin.translation.use.loading")} />
        </div>
      );
    case "error":
      return (
        <RegionError
          heading={t("client:admin.translation.use.failed")}
          headingLevel={3}
          reason={use.reason}
          retryLabel={t("client:admin.translation.use.retry")}
          onRetry={use.retry}
          isRetrying={use.isRetrying}
        />
      );
    case "ready":
      return <TranslationNumbers overview={use.overview} />;
  }
}

function TranslationNumbers({ overview }: { overview: TranslationOverview }) {
  const t = useT();
  const intl = useIntl();
  const rate = overview.cache_hit_rate;
  return (
    <Stack gap="4">
      <StatTileGrid>
        <StatTile
          as="li"
          label={t("client:admin.translation.use.characters", {
            month: intl.formatDate(`${overview.month}-01T00:00:00Z`, {
              year: "numeric",
              month: "long",
              timeZone: "UTC",
            }),
          })}
          value={intl.formatNumber(overview.characters)}
        />
        <StatTile
          as="li"
          label={t("client:admin.translation.use.hit_rate")}
          value={
            rate === undefined
              ? t("client:admin.translation.use.hit_rate.none")
              : intl.formatNumber(rate, { style: "percent", maximumFractionDigits: 1 })
          }
        />
      </StatTileGrid>
      <Hint size="inherit">{t("client:admin.translation.use.description")}</Hint>
    </Stack>
  );
}

type Form = ReturnType<typeof useTranslationSettings>;

function saveResult(form: Form, failed: (reason: string) => string): FormActionResult | undefined {
  if (form.saveError !== undefined) return { tone: "danger", message: failed(form.saveError) };
  if (form.status !== undefined) return { tone: "success", message: form.status };
  return undefined;
}

function TranslationForm() {
  const t = useT();
  const form = useTranslationSettings();
  const use = useTranslationUse();
  if (!form.ready) return null;
  const providers = use.status === "ready" ? use.overview.providers : [];
  // The enabled providers in their order, then the others. An enabled ID that this server has no provider for has no
  // card, and stays in the setting.
  const enabled = form.enabled.flatMap((id) => providers.filter((provider) => provider.id === id));
  const others = providers.filter((provider) => !form.enabled.includes(provider.id));
  return (
    <Stack as="form" gap="8" onSubmit={form.submit}>
      <Section surface="card" title={t("client:admin.translation.providers.title")}>
        <Stack gap="4">
          <Callout>
            {t("client:admin.translation.providers.privacy")}{" "}
            <Link href="/admin/documents">{t("client:admin.translation.providers.privacy.link")}</Link>
          </Callout>
          {use.status === "ready" && providers.length === 0 ? (
            <EmptyState
              headingLevel={3}
              heading={t("client:admin.translation.providers.none.heading")}
              description={t("client:admin.translation.providers.none.description")}
            />
          ) : (
            [...enabled, ...others].map((provider) => (
              <ProviderCard
                key={provider.id}
                form={form}
                provider={provider}
                before={enabled[enabled.indexOf(provider) - 1]?.id}
                after={enabled[enabled.indexOf(provider) + 1]?.id}
              />
            ))
          )}
          <Checkbox
            isSelected={form.browser}
            onChange={form.setBrowser}
            description={t("client:admin.translation.browser.description")}
          >
            {t("client:admin.translation.browser")}
          </Checkbox>
        </Stack>
      </Section>
      <Section surface="card" title={t("client:admin.translation.limits.title")}>
        <Stack gap="4">
          <Hint size="inherit">{t("client:admin.translation.limits.description")}</Hint>
          <NumberField
            form={form}
            setting="translation_user_monthly_characters"
            label={t("client:admin.translation.limits.user")}
          />
          <NumberField
            form={form}
            setting="translation_server_monthly_characters"
            label={t("client:admin.translation.limits.server")}
          />
          <NumberField
            form={form}
            setting="translation_retention_days"
            label={t("client:admin.translation.retention")}
          />
        </Stack>
      </Section>
      <FormActionBar
        sticky
        result={saveResult(form, (reason) => t("client:admin.translation.save_failed", { reason }))}
      >
        <Button type="submit" variant="primary" isPending={form.isSaving}>
          {t("client:admin.translation.save")}
        </Button>
      </FormActionBar>
    </Stack>
  );
}

/** A provider of this server: whether it is enabled and, if so, its place in the order and what it may receive. */
function ProviderCard({
  form,
  provider,
  before,
  after,
}: {
  form: Form;
  provider: TranslationProvider;
  /** The enabled provider just before this one, and just after: none at the edges of the order. */
  before: string | undefined;
  after: string | undefined;
}) {
  const t = useT();
  const on = form.enabled.includes(provider.id);
  const allowed = form.visibilities(provider.id);
  const name = provider.name;
  return (
    <Card>
      <Stack gap="4">
        <Checkbox
          isSelected={on}
          onChange={(selected) => {
            form.setEnabled(provider.id, selected);
          }}
        >
          <span dir="auto">{name}</span>
        </Checkbox>
        {on && (
          <>
            <Cluster gap="2">
              {/* The first provider cannot move up nor the last down: those buttons stay out rather than doing nothing. */}
              {before !== undefined && (
                <Button
                  variant="secondary"
                  aria-label={t("client:admin.translation.move_up_provider", { provider: name })}
                  onPress={() => {
                    form.swap(provider.id, before);
                  }}
                >
                  {t("client:admin.translation.move_up")}
                </Button>
              )}
              {after !== undefined && (
                <Button
                  variant="secondary"
                  aria-label={t("client:admin.translation.move_down_provider", { provider: name })}
                  onPress={() => {
                    form.swap(provider.id, after);
                  }}
                >
                  {t("client:admin.translation.move_down")}
                </Button>
              )}
            </Cluster>
            <Fieldset legend={t("client:admin.translation.visibilities", { provider: name })}>
              <Stack gap="2">
                <Hint size="inherit">{t("client:admin.translation.visibilities.description")}</Hint>
                {translationVisibilities.map((visibility) => (
                  <Checkbox
                    key={visibility}
                    isSelected={allowed.includes(visibility)}
                    onChange={(selected) => {
                      form.setVisibilities(
                        provider.id,
                        selected ? [...allowed, visibility] : allowed.filter((v) => v !== visibility),
                      );
                    }}
                  >
                    {t(visibilityLabels[visibility])}
                  </Checkbox>
                ))}
              </Stack>
            </Fieldset>
          </>
        )}
      </Stack>
    </Card>
  );
}

function NumberField({ form, setting, label }: { form: Form; setting: TranslationNumberKey; label: string }) {
  const t = useT();
  const field = form.numberField(setting);
  return (
    <TextField
      label={label}
      description={t("client:admin.translation.number.range", field.range)}
      value={form.value(setting)}
      onChange={(value) => {
        form.change(setting, value);
      }}
      inputMode="numeric"
      autoComplete="off"
      // Only an invalid value sets isInvalid: React Aria ignores `validate` once isInvalid is given at all.
      {...(field.error === undefined ? {} : { isInvalid: true, errorMessage: field.error })}
    />
  );
}
