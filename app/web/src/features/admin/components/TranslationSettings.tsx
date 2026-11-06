// SPDX-License-Identifier: AGPL-3.0-only
// The translation screen below its heading (web-client/screens/admin-translation.md, ADR-0148, manage_settings): the
// month's characters and cache hit rate, then one form with the providers, what each may receive, whether reports may
// go to them, the browser's translation, the limits, and how long translations are kept. The form says first that the privacy policy must change
// before a server-side provider is enabled, since text sent to one cannot be recalled. "Move up" and "Move down" set the
// order with the keyboard as well as the pointer (no dragging), as the WAI-ARIA APG's rearrangeable list does: the focus
// stays with the moved provider, and a button that cannot move it is disabled rather than gone.
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
import { useEffect, useId, useRef } from "react";
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
  const baseId = useId();
  /** The button to focus once the cards have rendered a move. */
  const focusAfterMoveRef = useRef<string | null>(null);
  // Runs after every render: focus is moved in the DOM, outside React, once the moved card is in its new place (moving a
  // focused element in the DOM takes the focus off it).
  useEffect(() => {
    const id = focusAfterMoveRef.current;
    if (id === null) return;
    focusAfterMoveRef.current = null;
    document.getElementById(id)?.focus();
  });
  if (!form.ready) {
    return (
      <div aria-busy="true">
        <Spinner label={t("client:admin.translation.settings.loading")} />
      </div>
    );
  }
  const providers = use.status === "ready" ? use.overview.providers : [];
  // The enabled providers in their order, then the others. An enabled ID that this server has no provider for has no
  // card, and stays in the setting.
  const enabled = form.enabled.flatMap((id) => providers.filter((provider) => provider.id === id));
  const others = providers.filter((provider) => !form.enabled.includes(provider.id));
  const buttonId = (id: string, step: -1 | 1) => `${baseId}-${id}-${step === -1 ? "up" : "down"}`;
  const move = (id: string, step: -1 | 1) => {
    const index = enabled.findIndex((provider) => provider.id === id);
    const other = enabled[index + step];
    if (other === undefined) return;
    // The focus stays on the moved provider: on the same button, or on the other one where this one is now disabled.
    const to = index + step;
    const atEnd = step === -1 ? to === 0 : to === enabled.length - 1;
    focusAfterMoveRef.current = buttonId(id, atEnd ? (step === -1 ? 1 : -1) : step);
    form.swap(id, other.id);
  };
  return (
    <Stack as="form" gap="8" onSubmit={form.submit}>
      <Section surface="card" title={t("client:admin.translation.providers.title")}>
        <Stack gap="4">
          <Callout>
            <Stack gap="2">
              <span>{t("client:admin.translation.providers.privacy")}</span>
              <Link href="/admin/documents" variant="standalone">
                {t("client:admin.translation.providers.privacy.link")}
              </Link>
            </Stack>
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
                isFirst={enabled.indexOf(provider) === 0}
                isLast={enabled.indexOf(provider) === enabled.length - 1}
                buttonId={(step) => buttonId(provider.id, step)}
                onMove={(step) => {
                  move(provider.id, step);
                }}
              />
            ))
          )}
          <Checkbox
            isSelected={form.reportsToServer}
            onChange={form.setReportsToServer}
            description={t("client:admin.translation.reports.description")}
          >
            {t("client:admin.translation.reports")}
          </Checkbox>
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
  isFirst,
  isLast,
  buttonId,
  onMove,
}: {
  form: Form;
  provider: TranslationProvider;
  /** Whether the provider is the first or the last of the enabled ones, which cannot move up or down. */
  isFirst: boolean;
  isLast: boolean;
  /** The ID of the button that moves the provider up (-1) or down (1). */
  buttonId: (step: -1 | 1) => string;
  onMove: (step: -1 | 1) => void;
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
              {/* The first provider cannot move up nor the last down: those buttons are disabled, so that the buttons
                  keep their places as the order changes. The names start with the words on the buttons (WCAG 2.5.3). */}
              <Button
                id={buttonId(-1)}
                variant="secondary"
                aria-label={t("client:admin.translation.move_up_provider", { provider: name })}
                isDisabled={isFirst}
                onPress={() => {
                  onMove(-1);
                }}
              >
                {t("client:admin.translation.move_up")}
              </Button>
              <Button
                id={buttonId(1)}
                variant="secondary"
                aria-label={t("client:admin.translation.move_down_provider", { provider: name })}
                isDisabled={isLast}
                onPress={() => {
                  onMove(1);
                }}
              >
                {t("client:admin.translation.move_down")}
              </Button>
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
