// SPDX-License-Identifier: AGPL-3.0-only

// The people of an event its organizers choose (ADR-0161 §4, FR-944), each a screen under the event, both in the same
// order: who is chosen, each with a menu whose last item, apart, ends it, asked first; then the search of this server's
// people, found by their name or handle, each with the press that chooses them.
//
// - Its invitations, at /events/{id}/invitations, for its organizer and co-organizers: the invitations not revoked,
//   oldest first; inviting someone, each once (a revoked invitation too, so that revoking cannot be undone). The
//   server's refusals are said in words of their own: invited before, someone who cannot be invited (a block between
//   them and an organizer, an organizer, an account that is not active), as many invitations as the event sends this
//   hour.
// - Its co-organizers, at /events/{id}/coorganizers, for its organizer alone: up to 3, in the order named; naming
//   another while there are fewer; the refusals said the same way (someone in a block with an organizer, the most an
//   event has).
//
// What came of a press is said in a polite status of the screen, a new element each time. The focus stays on the
// control pressed while it is there; where what it acted on is gone with it (someone invited, named, revoked or
// removed), it goes to the search, or to the list while the search is gone (the most co-organizers named). Another
// reader is told who chooses them, and an event that was cancelled that it has none to choose.

import { AccountLine, isUnavailable, nameOf } from "@my/app-components";
import {
  eventLimits,
  eventRefusal,
  useAccountSearch,
  useEvent,
  useEventInvitations,
  useEventWrites,
  useViewer,
  type Account,
  type ServerEvent,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  ConfirmDialog,
  FeedNotice,
  GroupHeading,
  IconButton,
  MenuGroup,
  MenuItem,
  MenuSurface,
  ModalPresence,
  RowItem,
  RowList,
  ScreenStack,
  ScreenTitle,
  SearchField,
  ShowMore,
  Stack,
  StateText,
  useAddedFocus,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import { useGone } from "./manage";
import { Retry, useEventStrings, type T } from "./parts";

/** Whether `account` is of this server: only its people are invited and named. */
const isLocal = (account: Account) => !account.acct.includes("@");

/** What came of the last press, said in the screen's status: one new element each time. */
type Said = { readonly text: string; readonly n: number };

/** The screen's status, there from the start, and what says into it. */
function useSaid(): { said: Said | null; say: (text: string) => void } {
  const [said, setSaid] = useState<Said | null>(null);
  return {
    said,
    say: (text) => {
      setSaid((before) => ({ text, n: (before?.n ?? 0) + 1 }));
    },
  };
}

/** What came of a press, read where it happened. */
function Status({ said }: { said: Said | null }) {
  return (
    <Box as="p" role="status">
      {said !== null && <span key={said.n}>{said.text}</span>}
    </Box>
  );
}

/** Puts the focus on the element of `id` once the page has drawn what a press changed. */
const focusSoon = (id: string) => {
  requestAnimationFrame(() => {
    document.getElementById(id)?.focus();
  });
};

/**
 * The people found for what the reader wrote, each with the action of `act` (Invite, Name co-organizer), or a word of
 * where they stand (Invited, Co-organizer, Organizer).
 */
function Found({
  q,
  label,
  stand,
  act,
  t,
}: {
  q: string;
  /** The button's words. */
  label: string;
  /** Where the account stands already, in a word; undefined for one the button acts on. */
  stand: (account: Account) => string | undefined;
  /** Acts on the account; resolves once it is done or refused. */
  act: (account: Account) => Promise<void>;
  t: T;
}) {
  const viewer = useViewer();
  const { items, failed } = useAccountSearch(q === "" ? undefined : q);
  const [sending, setSending] = useState<string | null>(null);
  if (q === "") return null;
  if (failed) return <FeedNotice alert>{t("events.people.search_failed")}</FeedNotice>;
  if (items === undefined) return <FeedNotice>{t("events.people.searching")}</FeedNotice>;
  const found = items.filter((a) => isLocal(a) && a.id !== viewer?.id);
  if (found.length === 0) return <FeedNotice>{t("events.people.none_found")}</FeedNotice>;
  return (
    <RowList label={t("events.people.found")}>
      {found.map((account) => {
        const word = stand(account);
        return (
          <RowItem key={account.id}>
            <AccountLine
              account={account}
              end={
                word !== undefined ? (
                  <StateText>{word}</StateText>
                ) : (
                  <Button
                    size="target"
                    busy={sending === account.id}
                    isDisabled={sending !== null && sending !== account.id}
                    onPress={() => {
                      if (sending !== null) return;
                      setSending(account.id);
                      void act(account).finally(() => {
                        setSending(null);
                      });
                    }}
                  >
                    {label}
                  </Button>
                )
              }
            />
          </RowItem>
        );
      })}
    </RowList>
  );
}

/** The search of this server's people, under its heading, the field at `id`. */
function Search({ id, heading, onChange, t }: { id: string; heading: string; onChange: (q: string) => void; t: T }) {
  return (
    <>
      <GroupHeading>{heading}</GroupHeading>
      <SearchField
        id={id}
        label={t("events.people.search")}
        hint={t("events.people.search_hint")}
        onChange={(q) => {
          onChange(q.trim().replace(/^@/u, ""));
        }}
        clearLabel={t("events.people.clear")}
      />
    </>
  );
}

/** The menu of one account chosen: its last item, apart, ends what was chosen (`end`, its words), asked first. */
function PersonMenu({ account, end, onEnd, t }: { account: Account; end: string; onEnd: () => void; t: T }) {
  const label = t("events.people.menu", { name: nameOf(account) });
  return (
    <MenuSurface trigger={<IconButton icon="more" label={label} square />} label={label} placement="bottom end">
      <MenuGroup>
        <MenuItem id="end" icon="leave" onAction={onEnd}>
          {end}
        </MenuItem>
      </MenuGroup>
    </MenuSurface>
  );
}

/** What ending a choice asks: its question, what follows, and the buttons' words. */
type Ask = { readonly title: string; readonly text: string; readonly confirm: string; readonly keep: string };

/**
 * The confirmation of ending what was chosen of `account`: `end` does it and resolves whether it was done, and
 * `onGone` runs once the dialog is out of the document, the focus to go where the row was.
 */
function EndDialog({
  ask,
  end,
  onClose,
  onGone,
}: {
  ask: Ask;
  end: () => Promise<boolean>;
  onClose: () => void;
  onGone: () => void;
}) {
  const [busy, setBusy] = useState(false);
  useGone(onGone);
  return (
    <ConfirmDialog
      title={ask.title}
      cancel={ask.keep}
      closeLabel={ask.keep}
      confirm={ask.confirm}
      busy={busy}
      onCancel={() => {
        if (!busy) onClose();
      }}
      onConfirm={() => {
        setBusy(true);
        void end().then(onClose);
      }}
    >
      {ask.text}
    </ConfirmDialog>
  );
}

/**
 * The head of a screen under the event, and what stands in its place until the event is read, when it is gone, or when
 * the reader is not one who chooses (`may`, with the words that say who does), or the event was cancelled.
 */
function UnderEvent({
  id,
  title,
  glyph,
  onBack,
  may,
  notAllowed,
  children,
}: {
  id: string;
  title: string;
  glyph: "invite" | "members";
  onBack: () => void;
  may: (event: ServerEvent) => boolean;
  notAllowed: (event: ServerEvent) => string;
  children: (event: ServerEvent) => ReactNode;
}) {
  const t = useEventStrings();
  const { event, error, gone, refetch } = useEvent(id);
  const head = (
    <ScreenTitle back={<BackButton onPress={onBack}>{t("events.back_to_event")}</BackButton>} glyph={glyph}>
      {title}
    </ScreenTitle>
  );
  if (event === undefined)
    return (
      <ScreenStack busy={!gone && error === null}>
        {head}
        {gone ? (
          <FeedNotice lead>{t("events.gone")}</FeedNotice>
        ) : error === null ? (
          <FeedNotice>{t("events.event_loading")}</FeedNotice>
        ) : (
          <FeedNotice alert action={<Retry onPress={refetch} t={t} />}>
            {t("events.event_failed")}
          </FeedNotice>
        )}
      </ScreenStack>
    );
  const words = !may(event) ? notAllowed(event) : event.state === "canceled" ? t("events.people.canceled") : undefined;
  return (
    <ScreenStack>
      {head}
      {words === undefined ? children(event) : <FeedNotice lead>{words}</FeedNotice>}
    </ScreenStack>
  );
}

/** The invitations not revoked, oldest first, each with its menu; Show more for the next page. */
function Invited({
  event,
  listId,
  onRevoke,
  t,
}: {
  event: ServerEvent;
  listId: string;
  onRevoke: (account: Account) => void;
  t: T;
}) {
  const list = useEventInvitations(event.id);
  const added = useAddedFocus(list.items.length);
  const heading = t("events.invitations.list");
  return (
    <Stack gap="entry-line" id={listId} tabIndex={-1} role="group" aria-label={heading}>
      <GroupHeading>{heading}</GroupHeading>
      {list.status === "pending" ? (
        <FeedNotice>{t("events.invitations.loading")}</FeedNotice>
      ) : list.items.length === 0 ? (
        list.status === "error" ? (
          <FeedNotice alert action={<Retry onPress={list.refetch} t={t} />}>
            {t("events.invitations.failed")}
          </FeedNotice>
        ) : (
          <FeedNotice>{t("events.invitations.none")}</FeedNotice>
        )
      ) : (
        <Stack gap="entry-room">
          <RowList label={heading}>
            {list.items.map((invitation, i) => (
              <RowItem key={invitation.id} itemRef={added.item(i)}>
                <AccountLine
                  account={invitation.account}
                  end={
                    <PersonMenu
                      account={invitation.account}
                      end={t("events.invitations.revoke")}
                      onEnd={() => {
                        onRevoke(invitation.account);
                      }}
                      t={t}
                    />
                  }
                />
              </RowItem>
            ))}
          </RowList>
          <ShowMore
            more={list.hasNextPage}
            busy={list.isFetchingNextPage}
            failed={list.failedNextPage}
            onPress={added.press(list.fetchNextPage)}
          />
        </Stack>
      )}
    </Stack>
  );
}

/** The invitations of an event once it is read, for its organizers. */
function Invitations({ event, t }: { event: ServerEvent; t: T }) {
  const writes = useEventWrites(event.id);
  const invitations = useEventInvitations(event.id);
  const { said, say } = useSaid();
  const [q, setQ] = useState("");
  const [revoking, setRevoking] = useState<Account | null>(null);
  const searchId = useId();
  const listId = useId();
  const invited = new Set(invitations.items.map((i) => i.account.id));
  const organizers = new Set([event.organizer.id, ...event.coorganizers.map((a) => a.id)]);
  const revokingName = revoking === null ? "" : nameOf(revoking);
  return (
    <>
      <Invited event={event} listId={listId} onRevoke={setRevoking} t={t} />
      <Stack gap="entry-line">
        <Search id={searchId} heading={t("events.invite.heading")} onChange={setQ} t={t} />
        <Status said={said} />
        <Found
          q={q}
          label={t("events.invite.invite")}
          stand={(account) =>
            organizers.has(account.id)
              ? t("events.people.organizer")
              : invited.has(account.id)
                ? t("events.invite.invited")
                : undefined
          }
          act={async (account) => {
            const name = nameOf(account);
            try {
              await writes.invite(account.id);
              say(t("events.invite.done", { name }));
              // Invite gave way to Invited: the search, for the next one.
              focusSoon(searchId);
            } catch (error) {
              const kind = eventRefusal(error);
              say(
                t("events.invite.refused", {
                  name,
                  kind:
                    kind === "already-invited" || kind === "invalid-invitee" || kind === "rate-limited"
                      ? kind.replaceAll("-", "_")
                      : "other",
                  count: eventLimits.invitationsAnHour,
                }),
              );
            }
          }}
          t={t}
        />
      </Stack>
      <ModalPresence>
        {revoking !== null && (
          <EndDialog
            key="revoke"
            ask={{
              title: t("events.invitations.revoke_title", { name: revokingName }),
              text: t("events.invitations.revoke_text", { name: revokingName }),
              confirm: t("events.invitations.revoke_confirm"),
              keep: t("events.invitations.revoke_keep"),
            }}
            end={() =>
              writes.revokeInvitation(revoking.id).then(
                () => {
                  say(t("events.invitations.revoked", { name: revokingName }));
                  return true;
                },
                () => {
                  say(t("events.invitations.revoke_failed", { name: revokingName }));
                  return false;
                },
              )
            }
            onClose={() => {
              setRevoking(null);
            }}
            onGone={() => {
              // The row went with the invitation, or stays where the focus returns to: the list either way.
              focusSoon(listId);
            }}
          />
        )}
      </ModalPresence>
    </>
  );
}

/**
 * The invitations of the event of `id`, for its organizers: the invitations, each with its menu that revokes it, and
 * inviting people of this server; `onBack` goes back to the event.
 */
export function EventInvitationsScreen({ id, onBack }: { id: string; onBack: () => void }) {
  const t = useEventStrings();
  return (
    <UnderEvent
      id={id}
      title={t("events.invitations")}
      glyph="invite"
      onBack={onBack}
      may={(event) => event.viewer.organizer || event.viewer.coorganizer}
      notAllowed={(event) =>
        isUnavailable(event.organizer)
          ? t("events.invitations.organizers_only.unavailable")
          : t("events.invitations.organizers_only", { organizer: nameOf(event.organizer) })
      }
    >
      {(event) => <Invitations event={event} t={t} />}
    </UnderEvent>
  );
}

/** The co-organizers of an event once it is read, for its organizer: each with its menu, and naming another. */
function Coorganizers({ event, t }: { event: ServerEvent; t: T }) {
  const writes = useEventWrites(event.id);
  const { said, say } = useSaid();
  const [q, setQ] = useState("");
  const [removing, setRemoving] = useState<Account | null>(null);
  const searchId = useId();
  const listId = useId();
  const named = new Set(event.coorganizers.map((a) => a.id));
  const full = event.coorganizers.length >= eventLimits.coorganizers;
  const heading = t("events.coorganizers.count", { count: event.coorganizers.length, max: eventLimits.coorganizers });
  const removingName = removing === null ? "" : nameOf(removing);
  return (
    <>
      <Stack gap="entry-line" id={listId} tabIndex={-1} role="group" aria-label={heading}>
        <GroupHeading>{heading}</GroupHeading>
        {event.coorganizers.length === 0 ? (
          <FeedNotice>{t("events.coorganizers.none")}</FeedNotice>
        ) : (
          <RowList label={heading}>
            {event.coorganizers.map((account) => (
              <RowItem key={account.id}>
                <AccountLine
                  account={account}
                  end={
                    <PersonMenu
                      account={account}
                      end={t("events.coorganizers.remove")}
                      onEnd={() => {
                        setRemoving(account);
                      }}
                      t={t}
                    />
                  }
                />
              </RowItem>
            ))}
          </RowList>
        )}
      </Stack>
      <Stack gap="entry-line">
        {full ? (
          <>
            <GroupHeading>{t("events.coorganizers.heading")}</GroupHeading>
            <Box as="p" text="body">
              {t("events.coorganizers.full", { count: eventLimits.coorganizers })}
            </Box>
          </>
        ) : (
          <Search id={searchId} heading={t("events.coorganizers.heading")} onChange={setQ} t={t} />
        )}
        <Status said={said} />
        {!full && (
          <Found
            q={q}
            label={t("events.coorganizers.name")}
            stand={(account) =>
              account.id === event.organizer.id
                ? t("events.people.organizer")
                : named.has(account.id)
                  ? t("events.coorganizers.named")
                  : undefined
            }
            act={async (account) => {
              const name = nameOf(account);
              try {
                const changed = await writes.coorganizer(account.id, true);
                say(t("events.coorganizers.done", { name }));
                // The button gave way to Co-organizer, or the search to the sentence that the event has the most.
                focusSoon(changed.coorganizers.length >= eventLimits.coorganizers ? listId : searchId);
              } catch (error) {
                const kind = eventRefusal(error);
                say(
                  t("events.coorganizers.refused", {
                    name,
                    kind:
                      kind === "invalid-coorganizer" || kind === "too-many-coorganizers"
                        ? kind.replaceAll("-", "_")
                        : "other",
                    count: eventLimits.coorganizers,
                  }),
                );
              }
            }}
            t={t}
          />
        )}
      </Stack>
      <ModalPresence>
        {removing !== null && (
          <EndDialog
            key="remove"
            ask={{
              title: t("events.coorganizers.remove_title", { name: removingName }),
              text: t("events.coorganizers.remove_text"),
              confirm: t("events.coorganizers.remove_confirm"),
              keep: t("events.coorganizers.remove_keep"),
            }}
            end={() =>
              writes.coorganizer(removing.id, false).then(
                () => {
                  say(t("events.coorganizers.removed", { name: removingName }));
                  return true;
                },
                () => {
                  say(t("events.coorganizers.remove_failed", { name: removingName }));
                  return false;
                },
              )
            }
            onClose={() => {
              setRemoving(null);
            }}
            onGone={() => {
              focusSoon(listId);
            }}
          />
        )}
      </ModalPresence>
    </>
  );
}

/**
 * The co-organizers of the event of `id`, for its organizer: each with its menu that removes them, and naming up to
 * 3; `onBack` goes back to the event. Another reader is told that only the organizer names them.
 */
export function EventCoorganizersScreen({ id, onBack }: { id: string; onBack: () => void }) {
  const t = useEventStrings();
  return (
    <UnderEvent
      id={id}
      title={t("events.coorganizers")}
      glyph="members"
      onBack={onBack}
      may={(event) => event.viewer.organizer}
      notAllowed={(event) =>
        isUnavailable(event.organizer)
          ? t("events.coorganizers.organizer_only.unavailable")
          : t("events.coorganizers.organizer_only", { organizer: nameOf(event.organizer) })
      }
    >
      {(event) => <Coorganizers event={event} t={t} />}
    </UnderEvent>
  );
}
