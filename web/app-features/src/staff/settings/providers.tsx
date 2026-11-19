// SPDX-License-Identifier: AGPL-3.0-only

// The translation providers of the server as a field of the translation's form (ADR-0148): each provider the server
// has, whether it is used, and what it may receive, by audience; the providers used are tried in their order, the
// first first, and a provider used after another is moved to the front by Try first. What it holds is written as the
// two settings hold it: `translation_providers`, the IDs used in their order, and `translation_provider_visibilities`,
// `provider=audience|audience` for each provider the reader chose the audiences of (one not named receives the public,
// local and unlisted posts; `private` and `direct` go only to one named with them).

import { useTranslationProviders, type TranslationProvider } from "@my/app-data";
import { Box, Button, CheckField, FieldError, Row, Stack } from "@my/design-system";
import type { ReactNode } from "react";
import { useStaffStrings } from "../parts";

/** The two settings the field holds, as the server writes them. */
export type ProvidersValue = { readonly list: string; readonly visibilities: string };

/** The audiences a provider may receive, from the widest. */
const audiences = ["public", "local", "unlisted", "private", "direct"] as const;
type Audience = (typeof audiences)[number];
/** What a provider not named receives. */
const unnamed: readonly Audience[] = ["public", "local", "unlisted"];

/** The IDs of `list`, in order. */
const idsOf = (list: string): string[] =>
  list
    .split(",")
    .map((p) => p.trim())
    .filter((p) => p !== "");

/** The audiences named for each provider. */
function namedOf(visibilities: string): Map<string, Audience[]> {
  const named = new Map<string, Audience[]>();
  for (const pair of visibilities.split(",")) {
    const at = pair.indexOf("=");
    if (at <= 0) continue;
    const provider = pair.slice(0, at).trim();
    named.set(
      provider,
      pair
        .slice(at + 1)
        .split("|")
        .map((v) => v.trim())
        .flatMap((v) => audiences.filter((a) => a === v)),
    );
  }
  return named;
}

/** `named` written as the setting holds it. */
const visibilitiesOf = (named: ReadonlyMap<string, readonly Audience[]>): string =>
  [...named].map(([provider, list]) => `${provider}=${audiences.filter((a) => list.includes(a)).join("|")}`).join(",");

/** The providers of the server, the ones used first in their order, then the others in the server's. */
function ordered(providers: readonly TranslationProvider[], used: readonly string[]): TranslationProvider[] {
  return [
    ...used.flatMap((id) => providers.filter((p) => p.id === id)),
    ...providers.filter((p) => !used.includes(p.id)),
  ];
}

export function ProvidersField({
  id,
  value,
  onChange,
  error,
  disabled,
}: {
  /** The id of the first checkbox, which takes the focus when the field is refused; its error is `<id>-error`. */
  id: string;
  value: ProvidersValue;
  onChange: (value: ProvidersValue) => void;
  error?: string | undefined;
  disabled: boolean;
}): ReactNode {
  const { t } = useStaffStrings();
  const providers = useTranslationProviders();
  const used = idsOf(value.list);
  const named = namedOf(value.visibilities);
  if (providers === undefined) return <Box as="p">{t("staff_settings.providers.loading")}</Box>;
  if (providers.length === 0)
    return (
      <Box as="p" breaks="phrase">
        {t("staff_settings.providers.none")}
      </Box>
    );
  const shown = ordered(providers, used);
  const set = (list: readonly string[], next = named) => {
    onChange({ list: list.join(","), visibilities: visibilitiesOf(next) });
  };
  return (
    <Stack gap="screen-row-gap">
      {shown.map((p, i) => {
        const on = used.includes(p.id);
        const receives = named.get(p.id) ?? unnamed;
        const place = used.indexOf(p.id);
        return (
          <Stack key={p.id} gap="hair">
            <CheckField
              id={i === 0 ? id : `${id}-${p.id}`}
              name="translation_providers"
              value={p.id}
              label={t("staff_settings.providers.use", { name: p.name })}
              checked={on}
              describedBy={i === 0 && error !== undefined ? `${id}-error` : undefined}
              onChange={(checked) => {
                set(checked ? [...used, p.id] : used.filter((u) => u !== p.id));
              }}
              touch
              disabled={disabled}
            />
            {on && (
              <Stack gap="hair">
                <Box as="p" text="caption" tone="muted">
                  {t("staff_settings.providers.receives", { name: p.name })}
                </Box>
                <Row wrap gap="screen-tabs-gap">
                  {audiences.map((a) => (
                    <CheckField
                      key={a}
                      id={`${id}-${p.id}-${a}`}
                      name={`translation_provider_visibilities.${p.id}`}
                      value={a}
                      label={t("staff_settings.providers.audience", { audience: a })}
                      checked={receives.includes(a)}
                      onChange={(checked) => {
                        const next = new Map(named);
                        next.set(p.id, checked ? [...receives, a] : receives.filter((r) => r !== a));
                        set(used, next);
                      }}
                      touch
                      disabled={disabled}
                    />
                  ))}
                </Row>
                {place > 0 && (
                  <Row>
                    <Button
                      size="timeline"
                      touch
                      isDisabled={disabled}
                      onPress={() => {
                        set([p.id, ...used.filter((u) => u !== p.id)]);
                      }}
                    >
                      {t("staff_settings.providers.first", { name: p.name })}
                    </Button>
                  </Row>
                )}
              </Stack>
            )}
          </Stack>
        );
      })}
      {error !== undefined && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}
