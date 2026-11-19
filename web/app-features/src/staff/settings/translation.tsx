// SPDX-License-Identifier: AGPL-3.0-only

// Translation, at /admin/translation (ADR-0148, ADR-0058 §1): the providers the server sends posts to, in the order it
// tries them, and what each may receive; whether readers may use their browser's own translation; how many characters
// a user and the whole server may send in a month; how long a translation is kept; and whether the moderators may
// send a report's text. A form saved as a whole (form.tsx), read with the settings and the providers the server has,
// under how translation is used in the current month (UTC): the characters sent to providers, which the limits count,
// and how many requests a translation the server kept answered.

import {
  loadAdminSettings,
  loadTranslationProviders,
  useAdminSettings,
  useTranslationOverview,
  type AppData,
} from "@my/app-data";
import { SettingsFacts, SettingsScreen, SettingsSection, useLocale } from "@my/design-system";
import type { ReactNode } from "react";
import { useStaffStrings } from "../parts";
import { TopicForm } from "./form";
import { ScreenNotice } from "./parts";
import { translationTopic } from "./topics";

/** Reads what the screen shows, for the route's loader. */
export async function loadTranslationSettings(data: AppData): Promise<void> {
  await Promise.all([loadAdminSettings(data), loadTranslationProviders(data)]);
}

/** How translation is used this month: the characters sent and the share answered from kept translations. */
function Usage(): ReactNode {
  const { t } = useStaffStrings();
  const { locale } = useLocale();
  const { overview, status, retry } = useTranslationOverview();
  const heading = t("staff_settings.translation_usage");
  if (overview === undefined)
    return (
      <SettingsSection heading={heading}>
        <ScreenNotice
          status={status === "failed" ? "failed" : "loading"}
          retry={retry}
          words={{
            loading: t("staff_settings.translation_usage.loading"),
            failed: t("staff_settings.translation_usage.failed"),
          }}
        />
      </SettingsSection>
    );
  const percent = new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 0 });
  const requests = overview.cache_hits + overview.cache_misses;
  return (
    <SettingsSection heading={heading} about={t("staff_settings.translation_usage.about", { month: overview.month })}>
      <SettingsFacts
        label={heading}
        items={[
          { text: t("staff_settings.translation_usage.characters", { count: overview.characters }) },
          {
            text:
              overview.cache_hit_rate === undefined
                ? t("staff_settings.translation_usage.no_requests")
                : t("staff_settings.translation_usage.kept", {
                    share: percent.format(overview.cache_hit_rate),
                    count: requests,
                  }),
          },
        ]}
      />
    </SettingsSection>
  );
}

/** Translation's settings, once they are read. */
export function TranslationSettingsScreen(): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminSettings();
  const title = t("staff.entry.translation");
  if (read.settings === undefined || read.texts === undefined)
    return (
      <SettingsScreen
        icon={translationTopic.icon}
        title={title}
        notice={
          <ScreenNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{ loading: t("staff_settings.loading"), failed: t("staff_settings.failed") }}
          />
        }
      />
    );
  return (
    <TopicForm topic={translationTopic} title={title} settings={read.settings} texts={read.texts} before={<Usage />} />
  );
}
