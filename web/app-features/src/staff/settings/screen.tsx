// SPDX-License-Identifier: AGPL-3.0-only

// Server settings, at /admin/settings (ADR-0058 §1): its pages in groups, each a door with a line of what it holds;
// and a page of it, at /admin/settings/<topic>, a form saved as a whole once the settings and the texts are read
// (form.tsx), the brand images' page its own (branding.tsx). An address that names no page says so, with the way back
// to the list.

import { loadAdminBrands, loadAdminSettings, useAdminSettings, type AppData } from "@my/app-data";
import {
  BackButton,
  ScreenStack,
  ScreenTitle,
  SettingsGroup,
  SettingsItem,
  SettingsMenu,
  SettingsScreen,
} from "@my/design-system";
import type { ReactNode } from "react";
import { useStaffStrings } from "../parts";
import { BrandingForm, type BuiltInBrands } from "./branding";
import type { MessageId } from "../../messages.gen";
import { TopicForm } from "./form";
import { ScreenNotice, type AdminSubProps } from "./parts";
import { topicAt } from "./topics";
import { groupHeadings } from "./words";

/** The address of Server settings. */
const adminSettingsPath = "/admin/settings";

/** The address of the page of `topic`. */
const adminSettingsTopicPath = (topic: string): string => `${adminSettingsPath}/${topic}`;

/** The name of each page of Server settings, in the list and as its title. */
const topicWords = {
  general: "staff_settings.topic.general",
  branding: "staff_settings.topic.branding",
  registration: "staff_settings.topic.registration",
  content: "staff_settings.topic.content",
  voice: "staff_settings.topic.voice",
  "auto-delete": "staff_settings.topic.auto_delete",
  federation: "staff_settings.topic.federation",
  discovery: "staff_settings.topic.discovery",
  hooks: "staff_settings.topic.hooks",
  records: "staff_settings.topic.records",
} as const satisfies Readonly<Record<string, MessageId>>;
type TopicPath = keyof typeof topicWords;

/** The words of the page at `path`; undefined for an address that names none. */
const wordsAt = (path: string) => (Object.hasOwn(topicWords, path) ? topicWords[path as TopicPath] : undefined);

/** The pages of Server settings in their groups, in order: each group's heading is `staff_settings.group.<id>`. */
const groups = [
  { id: "server", topics: ["general", "branding", "registration"] },
  { id: "content", topics: ["content", "voice", "auto-delete"] },
  { id: "reach", topics: ["federation", "discovery"] },
  { id: "moderation", topics: ["hooks", "records"] },
] as const;

/** The glyph of a page: its topic's, the brand images' their own. */
const iconOf = (path: string) => (path === "branding" ? "picture" : (topicAt(path)?.icon ?? "settings"));

/** Server settings: its pages, each a door. */
export function AdminSettingsScreen(): ReactNode {
  const { t } = useStaffStrings();
  return (
    <ScreenStack>
      <ScreenTitle glyph="settings">{t("staff.entry.settings")}</ScreenTitle>
      <SettingsMenu label={t("staff.entry.settings")}>
        {groups.map((group) => (
          <SettingsGroup key={group.id} heading={t(groupHeadings[group.id])}>
            {group.topics.map((path) => (
              <SettingsItem
                key={path}
                icon={iconOf(path)}
                label={t(topicWords[path])}
                href={adminSettingsTopicPath(path)}
                count={undefined}
              />
            ))}
          </SettingsGroup>
        ))}
      </SettingsMenu>
    </ScreenStack>
  );
}

/** Reads what the page of `topic` shows, for its route's loader. */
export async function loadAdminSettingsTopic(data: AppData, topic: string): Promise<void> {
  await (topic === "branding" ? loadAdminBrands(data) : loadAdminSettings(data));
}

/** The title of the page of `topic`; undefined for an address that names none. */
export function useAdminSettingsTopicTitle(topic: string): string | undefined {
  const { t } = useStaffStrings();
  const words = wordsAt(topic);
  return words === undefined ? undefined : t(words);
}

/** A page of Server settings: its form once what it shows is read, until then that it loads or failed. */
export function AdminSettingsTopicScreen({
  topic,
  builtIn,
  ...sub
}: AdminSubProps & { topic: string; builtIn: BuiltInBrands }): ReactNode {
  const { t } = useStaffStrings();
  const found = topicAt(topic);
  const read = useAdminSettings();
  const back = { label: t("staff.entry.settings"), href: adminSettingsPath };
  const backButton = (
    <BackButton
      onPress={() => {
        sub.go(back.href);
      }}
    >
      {back.label}
    </BackButton>
  );
  if (topic === "branding") return <BrandingForm back={back} builtIn={builtIn} {...sub} />;
  if (found === undefined)
    return (
      <SettingsScreen
        sub
        title={t("staff.entry.settings")}
        back={backButton}
        notice={
          <ScreenNotice
            status="missing"
            retry={read.retry}
            words={{
              loading: t("staff_settings.loading"),
              failed: t("staff_settings.failed"),
              missing: t("staff_settings.missing"),
            }}
          />
        }
      />
    );
  const title = t(wordsAt(topic) ?? "staff.entry.settings");
  if (read.settings === undefined || read.texts === undefined)
    return (
      <SettingsScreen
        sub
        title={title}
        back={backButton}
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
    <TopicForm
      key={topic}
      topic={found}
      title={title}
      back={back}
      settings={read.settings}
      texts={read.texts}
      {...sub}
    />
  );
}
