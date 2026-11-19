// SPDX-License-Identifier: AGPL-3.0-only

// The invitations of an open voice chat room, at /voice/{id}/invitations, for its host and co-hosts (ADR-0100 §7,
// FR-927): inviting this server's people, found by their name or handle, as a listener or as a speaker who speaks
// without asking; and the invitations not revoked, oldest first, each with as what, and Revoke. What came of a press
// is said in a polite status of the screen, the server's refusals in words of their own (invited before, someone who
// cannot be invited — a block between them and a host, removed from the room —, too many invitations this hour).
// Another reader is told that only the host and the co-hosts invite.

import { AccountLine, nameOf } from "@my/app-components";
import {
  inviteToVoiceRoom,
  revokeVoiceInvitation,
  useAccountSearch,
  useViewer,
  useVoiceInvitations,
  useVoiceRoom,
  voiceInviteRefusal,
  type Account,
  type AppData,
  type VoiceRole,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  ChoiceList,
  FeedNotice,
  GroupHeading,
  RowItem,
  RowList,
  ScreenStack,
  ScreenTitle,
  SearchField,
  Stack,
  StateText,
  TextButton,
  useLocale,
} from "@my/design-system";
import { ProblemError } from "@mycroblog/mykit";
import { useId, useState } from "react";
import { stringsIn } from "../locale";

type T = ReturnType<typeof stringsIn>["t"];

const roles: readonly VoiceRole[] = ["listener", "speaker"];

/** Whether `account` is of this server: only its people are invited. */
const isLocal = (account: Account) => !account.acct.includes("@");

/** What came of the last press, said in the screen's status: one new element each time. */
type Said = { readonly text: string; readonly n: number };

/** The people found for what the reader wrote, each with Invite, or what came of inviting them. */
function Found({
  data,
  roomId,
  q,
  role,
  invited,
  onSaid,
  t,
}: {
  data: AppData;
  roomId: string;
  q: string;
  role: VoiceRole;
  /** The accounts invited already, by ID. */
  invited: ReadonlySet<string>;
  onSaid: (text: string) => void;
  t: T;
}) {
  const viewer = useViewer();
  const { items, failed } = useAccountSearch(q === "" ? undefined : q);
  const [sending, setSending] = useState<string | null>(null);
  if (q === "") return null;
  if (failed) return <FeedNotice alert>{t("voice.invite.search_failed")}</FeedNotice>;
  if (items === undefined) return <FeedNotice>{t("voice.invite.searching")}</FeedNotice>;
  const found = items.filter((a) => isLocal(a) && a.id !== viewer?.id);
  if (found.length === 0) return <FeedNotice>{t("voice.invite.none_found")}</FeedNotice>;
  const invite = (account: Account) => {
    if (sending !== null) return;
    setSending(account.id);
    const name = nameOf(account);
    inviteToVoiceRoom(data, roomId, account.id, role).then(
      () => {
        setSending(null);
        onSaid(t("voice.invite.done", { name, role }));
      },
      (error: unknown) => {
        setSending(null);
        onSaid(t("voice.invite.refused", { name, kind: (voiceInviteRefusal(error) ?? "other").replaceAll("-", "_") }));
      },
    );
  };
  return (
    <RowList label={t("voice.invite.found")}>
      {found.map((account) => (
        <RowItem key={account.id}>
          <AccountLine
            account={account}
            end={
              invited.has(account.id) ? (
                <StateText>{t("voice.invite.invited")}</StateText>
              ) : (
                <Button
                  size="timeline"
                  touch
                  isDisabled={sending !== null}
                  onPress={() => {
                    invite(account);
                  }}
                >
                  {t("voice.invite.invite")}
                </Button>
              )
            }
          />
        </RowItem>
      ))}
    </RowList>
  );
}

/** The invitations not revoked, oldest first, each with as what and Revoke; Show more for the next page. */
function Invited({ data, roomId, onSaid, t }: { data: AppData; roomId: string; onSaid: (text: string) => void; t: T }) {
  const list = useVoiceInvitations(roomId);
  const [revoking, setRevoking] = useState<string | null>(null);
  const retry = (onPress: () => void) => <TextButton onPress={onPress}>{t("voice.retry")}</TextButton>;
  if (list.status === "pending") return <FeedNotice>{t("voice.invitations.loading")}</FeedNotice>;
  if (list.items.length === 0)
    return list.status === "error" ? (
      <FeedNotice alert action={retry(list.refetch)}>
        {t("voice.invitations.failed")}
      </FeedNotice>
    ) : (
      <FeedNotice>{t("voice.invitations.none")}</FeedNotice>
    );
  const revoke = (accountId: string, name: string) => {
    if (revoking !== null) return;
    setRevoking(accountId);
    revokeVoiceInvitation(data, roomId, accountId).then(
      () => {
        setRevoking(null);
        onSaid(t("voice.invitations.revoked", { name }));
      },
      () => {
        setRevoking(null);
        onSaid(t("voice.invitations.revoke_failed", { name }));
      },
    );
  };
  return (
    <Stack gap="entry-room">
      <RowList label={t("voice.invitations.title")}>
        {list.items.map((invitation) => {
          const name = nameOf(invitation.account);
          return (
            <RowItem key={invitation.id}>
              <AccountLine
                account={invitation.account}
                end={
                  <>
                    <StateText>{t("voice.invitations.as", { role: invitation.role })}</StateText>
                    <Button
                      size="timeline"
                      touch
                      isDisabled={revoking !== null}
                      onPress={() => {
                        revoke(invitation.account.id, name);
                      }}
                    >
                      {t("voice.invitations.revoke")}
                    </Button>
                  </>
                }
              />
            </RowItem>
          );
        })}
      </RowList>
      {list.failedNextPage ? (
        <FeedNotice alert action={retry(list.fetchNextPage)}>
          {t("voice.invitations.failed_more")}
        </FeedNotice>
      ) : (
        list.hasNextPage && (
          <Box>
            <Button size="timeline" touch busy={list.isFetchingNextPage} onPress={list.fetchNextPage}>
              {t("voice.rooms_more")}
            </Button>
          </Box>
        )
      )}
    </Stack>
  );
}

/**
 * The invitations of the open room of `id`, for its host and co-hosts: inviting people of this server, and the
 * invitations with Revoke; `onBack` goes back to the room.
 */
export function VoiceInvitations({ data, id, onBack }: { data: AppData; id: string; onBack: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const viewer = useViewer();
  const { room, error, refetch } = useVoiceRoom(id);
  const [q, setQ] = useState("");
  const [role, setRole] = useState<VoiceRole>("listener");
  const [said, setSaid] = useState<Said | null>(null);
  const searchId = useId();
  const invitations = useVoiceInvitations(id, room !== undefined);
  const back = <BackButton onPress={onBack}>{t("voice.room_back")}</BackButton>;
  const head = (
    <ScreenTitle back={back} glyph="invite">
      {t("voice.invitations.title")}
    </ScreenTitle>
  );
  // A room that ended has no invitations: the address says so, with nothing to try again.
  if (room === undefined && error instanceof ProblemError && error.status === 404)
    return (
      <ScreenStack>
        {head}
        <FeedNotice lead>{t("voice.settings.ended")}</FeedNotice>
      </ScreenStack>
    );
  if (room === undefined)
    return (
      <ScreenStack busy={error === null}>
        {head}
        {error === null ? (
          <FeedNotice>{t("voice.loading")}</FeedNotice>
        ) : (
          <FeedNotice alert action={<TextButton onPress={refetch}>{t("voice.retry")}</TextButton>}>
            {t("voice.failed")}
          </FeedNotice>
        )}
      </ScreenStack>
    );
  const self = room.participants.find((p) => p.account.id === viewer?.id);
  if (self === undefined || !(self.hosting || self.cohosting))
    return (
      <ScreenStack>
        {head}
        <FeedNotice lead>{t("voice.invitations.hosts_only")}</FeedNotice>
      </ScreenStack>
    );
  const onSaid = (text: string) => {
    setSaid({ text, n: (said?.n ?? 0) + 1 });
  };
  const invited = new Set(invitations.items.map((i) => i.account.id));
  return (
    <ScreenStack>
      {head}
      <Stack gap="entry-line">
        <GroupHeading>{t("voice.invite.heading")}</GroupHeading>
        <SearchField
          id={searchId}
          label={t("voice.invite.search")}
          hint={t("voice.invite.search_hint")}
          onChange={setQ}
          clearLabel={t("voice.invite.clear")}
        />
        <ChoiceList
          label={t("voice.invite.role")}
          multiple={false}
          items={roles.map((r) => ({ key: r, label: t("voice.invite.role_choice", { role: r }) }))}
          chosen={[roles.indexOf(role)]}
          onChange={([i]) => {
            setRole(roles[i ?? 0] ?? "listener");
          }}
        />
        {/* What came of a press, read where it happened. */}
        <Box as="p" role="status">
          {said !== null && <span key={said.n}>{said.text}</span>}
        </Box>
        <Found
          data={data}
          roomId={id}
          q={q.trim().replace(/^@/u, "")}
          role={role}
          invited={invited}
          onSaid={onSaid}
          t={t}
        />
      </Stack>
      <Stack gap="entry-line">
        <GroupHeading>{t("voice.invitations.title")}</GroupHeading>
        <Invited data={data} roomId={id} onSaid={onSaid} t={t} />
      </Stack>
    </ScreenStack>
  );
}
