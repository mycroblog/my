// SPDX-License-Identifier: AGPL-3.0-only

// The instance theme, at /admin/theme (FR-605, ADR-0059): what can be done now first, the draft's facts and its
// actions: Publish, the preview of the draft over every screen of this tab before it is published (the app's own
// screens under the draft's stylesheet, as visitors would see them), and the way to edit it; then the theme in effect,
// with the restore of the one published before it; and, apart and quiet, the way to the icon and the logo, which are
// a page of Server settings. A publication reaches every visitor's screens a few minutes after, and a page already
// open once it loads again, which what came of it says.

import {
  loadAdminTheme,
  themeErrorsOf,
  themeNothingThere,
  useAdminTheme,
  useAdminThemeWrites,
  useThemePreview,
  type AppData,
  type ThemeSet,
} from "@my/app-data";
import {
  Button,
  FactList,
  LinkButton,
  Row,
  SettingsList,
  SettingsListRow,
  SettingsNote,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  TextLink,
  type Fact,
} from "@my/design-system";
import { useState, type ReactNode } from "react";
import { useMoment, useStaffStrings, type T } from "../parts";
import { ScreenNotice } from "../settings/parts";
import { tokenNames } from "./tokens";

/** The address of the draft's form. */
const themeDraftPath = "/admin/theme/draft";
/** The address of the icon and the logo, a page of Server settings. */
const brandingPath = "/admin/settings/branding";

/** Reads the theme, for the route's loader. */
export const loadTheme = (data: AppData): Promise<void> => loadAdminTheme(data);

/** Whether two sets override the same tokens with the same values. */
const sameSet = (a: ThemeSet | undefined, b: ThemeSet | undefined) =>
  JSON.stringify(a?.tokens ?? []) === JSON.stringify(b?.tokens ?? []);

/** The facts of a set: when it was saved or published, how many tokens it changes, and what no longer applies. */
function setFacts(t: T, list: (items: string[]) => string, set: ThemeSet, when: Fact): Fact[] {
  return [
    when,
    { term: t("staff_theme.fact.changes"), value: t("staff_theme.changes", { count: set.tokens.length }) },
    ...(set.stale_tokens.length === 0
      ? []
      : [
          {
            term: t("staff_theme.fact.stale"),
            value: t("staff_theme.stale", {
              count: set.stale_tokens.length,
              // A token the app knows by its name; one it no longer knows (renamed, retired) by its code name.
              tokens: list(
                set.stale_tokens.map((s) => {
                  const name = tokenNames[s.token];
                  return name === undefined ? s.token : t(name);
                }),
              ),
            }),
          },
        ]),
  ];
}

/** What came of the last press of an action of the screen. */
type Said =
  | {
      readonly section: "draft" | "published";
      readonly words: string;
      /** The draft's form, where what the server refused of a set is put right. */
      readonly fix?: boolean | undefined;
    }
  | undefined;

/** What a press of the screen said, with the way to the draft where the draft is where to put it right. */
function SaidWords({ said }: { said: NonNullable<Said> }): ReactNode {
  const { t } = useStaffStrings();
  if (said.fix !== true) return said.words;
  return (
    <>
      {said.words} <TextLink href={themeDraftPath}>{t("staff_theme.edit")}</TextLink>
    </>
  );
}

/** The theme: the draft and its actions, the theme in effect and its restore, and the way to the brand images. */
export function AdminThemeScreen(): ReactNode {
  const { t, locale } = useStaffStrings();
  const list = (items: string[]) => new Intl.ListFormat(locale, { type: "conjunction" }).format(items);
  const moment = useMoment();
  const read = useAdminTheme();
  const writes = useAdminThemeWrites();
  const preview = useThemePreview();
  const [busy, setBusy] = useState<"publish" | "restore" | undefined>(undefined);
  const [said, setSaid] = useState<Said>(undefined);
  const title = t("staff.entry.theme");
  const theme = read.theme;
  if (theme === undefined)
    return (
      <SettingsScreen
        sub
        icon="theme"
        title={title}
        notice={
          <ScreenNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{ loading: t("staff_theme.loading"), failed: t("staff_theme.failed") }}
          />
        }
      />
    );
  const { draft, published } = theme;
  const act = (what: "publish" | "restore") => {
    setBusy(what);
    setSaid(undefined);
    (what === "publish" ? writes.publish() : writes.restore()).then(
      () => {
        setBusy(undefined);
        // What is in effect is what the screens show now, after a reload: the draft's preview has done its part.
        preview.stop();
        setSaid({ section: what === "publish" ? "draft" : "published", words: t("staff_theme.done", { what }) });
      },
      (error: unknown) => {
        setBusy(undefined);
        setSaid({
          section: what === "publish" ? "draft" : "published",
          // A set that no longer meets a rule (the defaults its pairs are checked with changed in an upgrade) is put
          // right in the draft; trying again would be refused again.
          ...(themeErrorsOf(error) !== undefined
            ? { words: t("staff_theme.invalid", { what }), fix: true }
            : {
                words: t(
                  what === "restore" && themeNothingThere(error)
                    ? "staff_theme.restore.none"
                    : what === "publish" && themeNothingThere(error)
                      ? "staff_theme.publish.none"
                      : "staff_settings.act_failed",
                ),
              }),
        });
      },
    );
  };
  const drafted = draft !== undefined;
  const publishedSame = drafted && published !== undefined && sameSet(draft, published.set);
  return (
    <SettingsScreen sub icon="theme" title={title}>
      <SettingsSection heading={t("staff_theme.draft")}>
        {draft === undefined ? (
          <SettingsNote instead>{t("staff_theme.draft.none")}</SettingsNote>
        ) : (
          <FactList
            label={t("staff_theme.draft")}
            items={[
              ...setFacts(t, list, draft, { term: t("staff_theme.fact.saved"), value: moment(draft.saved_at) }),
              {
                term: t("staff_theme.fact.state"),
                value: t(publishedSame ? "staff_theme.draft.same" : "staff_theme.draft.differs"),
              },
            ]}
          />
        )}
        <Row wrap gap="entry-caption">
          {drafted && (
            <Button
              // The screen's next step while the draft differs from the theme in effect; else only a publication again.
              kind={publishedSame ? "secondary" : "primary"}
              size="timeline"
              touch
              busy={busy === "publish"}
              isDisabled={busy !== undefined}
              onPress={() => {
                act("publish");
              }}
            >
              {t("staff_theme.publish")}
            </Button>
          )}
          {drafted && (
            <Button size="timeline" touch aria-pressed={preview.on} onPress={preview.on ? preview.stop : preview.start}>
              {t("staff_theme.preview")}
            </Button>
          )}
          {/* Without a draft, starting one is the screen's only action, the primary as on every staff screen. */}
          <LinkButton kind={drafted ? "secondary" : "primary"} size="timeline" href={themeDraftPath}>
            {t(drafted ? "staff_theme.edit" : "staff_theme.start")}
          </LinkButton>
        </Row>
        <SettingsResult>
          {said?.section === "draft" ? <SaidWords said={said} /> : preview.on ? t("staff_theme.preview.on") : undefined}
        </SettingsResult>
      </SettingsSection>
      <SettingsSection heading={t("staff_theme.published")}>
        {published === undefined ? (
          <SettingsNote instead>{t("staff_theme.published.none")}</SettingsNote>
        ) : (
          <FactList
            label={t("staff_theme.published")}
            items={setFacts(t, list, published.set, {
              term: t("staff_theme.fact.published"),
              value: moment(published.published_at),
            })}
          />
        )}
        {published !== undefined && (
          <Row wrap gap="entry-caption">
            <Button
              size="timeline"
              touch
              busy={busy === "restore"}
              isDisabled={busy !== undefined}
              onPress={() => {
                act("restore");
              }}
            >
              {t("staff_theme.restore")}
            </Button>
          </Row>
        )}
        <SettingsResult>{said?.section === "published" ? <SaidWords said={said} /> : undefined}</SettingsResult>
      </SettingsSection>
      <SettingsSection heading={t("staff_theme.elsewhere")}>
        <SettingsList label={t("staff_theme.elsewhere")}>
          <SettingsListRow
            icon="picture"
            title={t("staff_settings.topic.branding")}
            caption={t("staff_theme.brand")}
            href={brandingPath}
          />
        </SettingsList>
      </SettingsSection>
    </SettingsScreen>
  );
}
