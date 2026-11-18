// SPDX-License-Identifier: AGPL-3.0-only

// Invites, the entry of the settings at /settings/invites (FR-011, ADR-0106 §14, §15, §18): the invites the reader
// created, newest first, as the server lists them, each by its code with what it allows now (usable until an hour,
// revoked, expired, used up, not usable while the reader may not create invites) and how often it was used out of how
// many; the code of one that does not work now is muted, so that the ones that work stand out. A row opens the invite's
// screen (invite.tsx), which holds its link to copy and the way to revoke it. Create invite opens the screen that
// creates one (create.tsx).
//
// The menu offers the entry only to a reader who may create invites (entries.tsx). One who may not and opens the
// address reads why in a summary at the screen's top, with no way to create, and still finds the invites they made.
// A reader who has as many usable invites as the server allows reads so in the button's place, in the text's color
// apart from the description, and one who can create a single one more, that it is the last.

import { loadOwnInvites, useOwnInvites, type AppData } from "@my/app-data";
import {
  Button,
  LinkButton,
  Row,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsNote,
  SettingsScreen,
  SettingsSection,
  useLocale,
} from "@my/design-system";
import { stringsIn } from "../../locale";
import type { SettingsEntryProps } from "../entries";
import { numberIn } from "../parts";
import { CannotInvite, invitesPaths, Unread, useInviteFacts } from "./parts";

/** What the loader of a screen of the Invites waits for: the first page of the list, from the cache when it has it. */
export async function loadInvitesSettings(data: AppData): Promise<void> {
  await loadOwnInvites(data);
}

export function InvitesSettings({ back }: SettingsEntryProps) {
  const { locale } = useLocale();
  const { t } = stringsIn(locale);
  const factsOf = useInviteFacts();
  const list = useOwnInvites();
  const { limits, usable, canCreate, items } = list;
  const title = t("settings.entry.invites");
  if (limits === undefined || usable === undefined || canCreate === undefined)
    return (
      <SettingsScreen
        icon="invite"
        title={title}
        back={back}
        notice={<Unread status={list.status === "error" ? "failed" : "loading"} retry={list.refetch} />}
      />
    );

  /** How many more invites the reader can create before they have as many usable ones as the server allows. */
  const left = Math.max(0, limits.max_active - usable);
  const may = canCreate && left > 0;
  /** The next, older invites were asked for and could not be read: said under the ones held, with the way to ask again. */
  const failedMore = list.failedNextPage && list.hasNextPage && !list.isFetchingNextPage;
  const create = (
    <LinkButton size="timeline" href={invitesPaths.create}>
      {t("settings.invites.create")}
    </LinkButton>
  );
  return (
    <SettingsScreen icon="invite" title={title} back={back}>
      {!canCreate && <CannotInvite reason={list.cannotCreateReason} />}
      <SettingsSection
        heading={t("settings.invites.own")}
        // The list is counted once its last page is read.
        count={items.length === 0 || list.hasNextPage ? undefined : numberIn(locale, items.length)}
        about={t("settings.invites.own.about")}
        // The empty list's place is the one that adds while there is none.
        action={may && items.length > 0 ? create : undefined}
      >
        {canCreate && left === 0 && (
          <SettingsNote instead>{t("settings.invites.full", { max: limits.max_active })}</SettingsNote>
        )}
        {canCreate && left === 1 && (
          <SettingsNote>{t("settings.invites.left", { left, max: limits.max_active })}</SettingsNote>
        )}
        {items.length === 0 ? (
          <SettingsEmpty action={may ? create : undefined}>
            {t("settings.invites.none", { may: may ? "yes" : "no" })}
          </SettingsEmpty>
        ) : (
          <SettingsList label={t("settings.invites.own")}>
            {items.map((invite) => {
              const { works, state, more } = factsOf(invite);
              return (
                <SettingsListRow
                  key={invite.id}
                  icon="invite"
                  title={invite.code}
                  inactive={!works}
                  facts={[state, ...more]}
                  href={invitesPaths.invite(invite.id)}
                />
              );
            })}
          </SettingsList>
        )}
        {failedMore && <SettingsEmpty>{t("settings.invites.failed_more")}</SettingsEmpty>}
        {list.hasNextPage && (
          <Row justify="start">
            {/* One button, which stays as its name changes, and the focus with it: Show more, then Try again. */}
            <Button size="timeline" busy={list.isFetchingNextPage} onPress={list.fetchNextPage}>
              {t(failedMore ? "settings.retry" : "settings.invites.more")}
            </Button>
          </Row>
        )}
      </SettingsSection>
    </SettingsScreen>
  );
}
