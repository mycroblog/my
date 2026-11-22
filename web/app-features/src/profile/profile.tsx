// SPDX-License-Identifier: AGPL-3.0-only

// A profile (FR-006, the rough of #3047): the account of a handle, from useAccount() of @my/app-data, in its state
// for the reader: their own, another local account, a remote one, a locked one, one they block or mute, one whose
// server they mute. Its head: the banner, the avatar with the reader's actions at the other end of its row (on their
// own, Edit profile, which leads to the form of the settings; on another's, the bell
// of its new posts while the reader follows it, follow in its state, more: mute, block and remove a follower, each of
// the last two after a confirmation, report it, and mute its server), the name with a locked account's lock and a bot's badge, the
// handle with "Follows you" and the role badges, the bio, the fields, when it joined and its counts (each opening its
// list, follows.tsx), the reader's private note on it, the lines of its state (blocked, muted, its server muted, locked,
// remote), and the featured hashtags. A moved account (ADR-0089 §4) says where it moved, its main action leading
// there, with neither follow nor "Follows you"; an account that is not there (§15) shows back and a line, whatever
// the reason. A failed action is told at the bottom of the page (usePostNotice). Under
// it, the tabs of its posts (posts, posts and replies, media, articles), the pinned posts first under the first two. A blocked
// account shows its head and no posts. A post's actions are answered as on every screen (post-actions.tsx). A visitor
// reads it too, without what is the reader's own (the relationship, the note, the muted server, the rooms and the
// events): Follow, the menu's Report and their posts' actions that need an account lead them to sign in, coming back.

import {
  AccountName,
  accountPath,
  EventListLine,
  FieldList,
  handleOf,
  ListColumn,
  MetaCount,
  MetaLine,
  nameOf,
  NoteEditor,
  NoteLine,
  PostContent,
  type PostStatus,
  ProfileHead,
  sentenceHandleOf,
  statusPath,
  TagList,
  VoiceRoomEntryLine,
} from "@my/app-components";
import { formatParts } from "../slots";
import {
  accountNoteLimit,
  loadAccount,
  useAccount,
  useAccountLists,
  useAccountStatuses,
  useAccountVoiceRoom,
  useEvents,
  useInstance,
  useChangeRelationship,
  useLists,
  useListWrites,
  useFeaturedHashtags,
  useMutedDomain,
  useMuteDomain,
  useRelationship,
  useViewer,
  useWriteAccountNote,
  viewerQuery,
  type Account,
  type AccountPosts,
  type AppData,
  type Relationship,
  type Status,
} from "@my/app-data";
import {
  Avatar,
  BackButton,
  Badge,
  Button,
  CaptionLine,
  ConfirmDialog,
  dateFormat,
  FeedNotice,
  Icon,
  IconButton,
  MenuButton,
  MenuGroup,
  MenuItem,
  ModalPresence,
  ScreenStack,
  TabRow,
  TextButton,
  TextLink,
  LinkButton,
  RowItem,
  RowList,
  useLocale,
  useTimeZone,
} from "@my/design-system";
import { useId, useState } from "react";
import { stringsIn } from "../locale";
import { usePostActions, usePostNotice, usePostOwn, usePostPicker, usePostVoting } from "../post-actions";
import { profileEditPath } from "../profile-identity";
import { ReportDialog } from "../report";
import { TranslatablePost as Post } from "../post-translation";

type T = ReturnType<typeof stringsIn>["t"];

type Tab = Exclude<AccountPosts, "pinned">;

/**
 * What the route's loader waits for: the account of the handle, at once from a copy a post carries, and the viewer's
 * own, from the cache unless a write marked them stale. A failure is left to the screen.
 */
export async function loadProfile(data: AppData, acct: string): Promise<void> {
  await Promise.all([
    loadAccount(data, acct),
    data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/** What the loader of the navigation's profile waits for: the viewer's own account, then its profile. */
export async function loadOwnProfile(data: AppData): Promise<void> {
  const viewer = await data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined);
  if (viewer !== undefined) await loadProfile(data, viewer.acct);
}

/**
 * The title of the profile of a handle (the viewer's own without one), once its account is known: its name and its
 * handle; for a reader without a session (`signedIn` false), of an account the server knows.
 */
export function useProfileTitle(acct: string | undefined, signedIn = true): string | undefined {
  const { t } = stringsIn(useLocale().locale);
  // The viewer's own is read only for their own profile, which a visitor has none of.
  const viewer = useViewer(acct === undefined);
  const { account } = useAccount(acct ?? viewer?.acct, signedIn);
  return account === undefined ? undefined : t("profile.title", { name: nameOf(account), handle: handleOf(account) });
}

/** The viewer's own profile, which the navigation's profile opens. */
export function OwnProfile() {
  const { t } = stringsIn(useLocale().locale);
  const viewer = useViewer();
  return viewer === undefined ? <FeedNotice>{t("profile.loading")}</FeedNotice> : <Profile acct={viewer.acct} />;
}

/** A press that waits for the server, busy until it answered. */
function usePress(): [boolean, (run: () => Promise<void>) => void] {
  const [busy, setBusy] = useState(false);
  return [
    busy,
    (run) => {
      setBusy(true);
      run()
        .catch(() => undefined)
        .finally(() => {
          setBusy(false);
        });
    },
  ];
}

/** Opens the account's page on its own server, in a new tab. */
const openOriginal = (account: Account) => () => {
  window.open(account.url, "_blank", "noopener");
};

/** The main action of another account's profile: the follow in its state, or unblock. */
function Follow({ account, relationship, t }: { account: Account; relationship: Relationship; t: T }) {
  const change = useChangeRelationship();
  const write = useAccountWrite(t);
  const [busy, press] = usePress();
  const on = (c: "follow" | "unfollow" | "unblock") => () => {
    press(() => write(c, () => change(account.id, c)).then(() => undefined));
  };
  if (relationship.blocking)
    return (
      <Button size="timeline" busy={busy} onPress={on("unblock")}>
        {t("profile.unblock")}
      </Button>
    );
  if (relationship.following || relationship.requested)
    return (
      <Button size="timeline" busy={busy} onPress={on("unfollow")}>
        {t(relationship.following ? "profile.following" : "profile.requested")}
      </Button>
    );
  return (
    <Button size="timeline" kind="primary" busy={busy} onPress={on("follow")}>
      {t(
        account.locked ? "profile.request_follow" : relationship.followed_by ? "profile.follow_back" : "profile.follow",
      )}
    </Button>
  );
}

/** Which action on an account failed, as profile.action_failed names it. */
type Failure =
  | "follow"
  | "unfollow"
  | "mute"
  | "unmute"
  | "block"
  | "unblock"
  | "remove_follower"
  | "notify"
  | "note"
  | "mute_server"
  | "unmute_server"
  | "list_add"
  | "list_remove";

/**
 * Runs a write on the account, and tells its failure at the bottom of the page, in the server's words when it refused,
 * with Try again.
 */
function useAccountWrite(t: T): (failure: Failure, write: () => Promise<void>) => Promise<boolean> {
  const failed = usePostNotice();
  const run = (failure: Failure, write: () => Promise<void>): Promise<boolean> =>
    write().then(
      () => true,
      (e: unknown) => {
        failed(e, t("profile.action_failed", { action: failure }), () => {
          void run(failure, write);
        });
        return false;
      },
    );
  return run;
}

/** The bell of the account's new posts, beside follow while the reader follows it: an outline off, filled on. */
function Bell({ account, relationship, t }: { account: Account; relationship: Relationship; t: T }) {
  const change = useChangeRelationship();
  const write = useAccountWrite(t);
  const [busy, setBusy] = useState(false);
  const on = relationship.notify ?? false;
  return (
    <IconButton
      icon="notifications"
      label={t("profile.notify")}
      pressed={on}
      isDisabled={busy}
      onPress={() => {
        setBusy(true);
        void write("notify", () => change(account.id, on ? "unsubscribe" : "subscribe")).finally(() => {
          setBusy(false);
        });
      }}
    />
  );
}

/** A confirmation the profile's menu asks before it acts: of blocking the account, or of removing a follower. */
type Asking = { readonly kind: "block" | "remove"; readonly busy: boolean };

/**
 * The menu of what else the reader may do: open the original page of a remote account; mute the account, block it
 * after a confirmation, remove it from their followers (while it follows them) after another, report it (report.tsx);
 * mute its server, or stop muting it (`mutedDomain`, the domain muted that the account's falls under).
 */
function More({
  account,
  relationship,
  mutedDomain,
  t,
}: {
  account: Account;
  relationship: Relationship;
  mutedDomain: string | undefined;
  t: T;
}) {
  const change = useChangeRelationship();
  const muteServer = useMuteDomain();
  const write = useAccountWrite(t);
  const [asking, setAsking] = useState<Asking | undefined>(undefined);
  const [reporting, setReporting] = useState(false);
  const [open, setOpen] = useState(false);
  const [panel, setPanel] = useState<"more" | "lists">("more");
  const following = relationship.following ?? false;
  const lists = useLists(open && panel === "lists");
  const held = useAccountLists(account.id, open && panel === "lists");
  const listWrites = useListWrites();
  const muting = relationship.muting ?? false;
  const blocking = relationship.blocking ?? false;
  const remote = account.acct.includes("@");
  const handle = sentenceHandleOf(account);
  const confirm = () => {
    if (asking === undefined || asking.busy) return;
    setAsking({ ...asking, busy: true });
    void write(asking.kind === "block" ? "block" : "remove_follower", () =>
      change(account.id, asking.kind === "block" ? "block" : "remove-follower"),
    ).finally(() => {
      setAsking(undefined);
    });
  };
  // Add to list, a submenu in the menu's place: each of the reader's lists a toggle, on while it holds the account,
  // then New list, which opens the index with a new list's name.
  const toLists =
    panel === "lists" ? (
      <>
        <MenuGroup
          checked={held === undefined ? [] : [...held]}
          stay
          onCheck={(listId, on) => {
            void write(on ? "list_add" : "list_remove", () => listWrites.member(listId, account, on));
          }}
        >
          {lists.lists.map((l) => (
            <MenuItem key={l.id} id={l.id} icon="included" disabled={held === undefined}>
              {l.name}
            </MenuItem>
          ))}
        </MenuGroup>
        <MenuGroup>
          <MenuItem id="new-list" icon="add" href="/lists?create=1">
            {t("timeline.list_new")}
          </MenuItem>
        </MenuGroup>
      </>
    ) : undefined;
  return (
    <>
      <MenuButton
        icon="more"
        label={t("profile.more")}
        open={open}
        onOpenChange={(o) => {
          setOpen(o);
          if (!o) setPanel("more");
        }}
        sub={
          panel === "lists"
            ? {
                title: t("profile.add_to_list"),
                onBack: () => {
                  setPanel("more");
                },
              }
            : undefined
        }
      >
        {toLists ?? (
          <>
            {remote && (
              <MenuGroup>
                <MenuItem id="open" icon="open" onAction={openOriginal(account)}>
                  {t("profile.open_original")}
                </MenuItem>
              </MenuGroup>
            )}
            <MenuGroup title={handle}>
              <MenuItem
                id="add-to-list"
                icon="list"
                opens=""
                disabled={!following}
                about={following ? undefined : t("profile.add_to_list_followed_only")}
                onAction={() => {
                  setPanel("lists");
                }}
              >
                {t("profile.add_to_list")}
              </MenuItem>
              <MenuItem
                id="mute"
                icon="mute"
                on={muting}
                onAction={() => {
                  void write(muting ? "unmute" : "mute", () => change(account.id, muting ? "unmute" : "mute"));
                }}
              >
                {t(muting ? "profile.unmute" : "profile.mute")}
              </MenuItem>
              <MenuItem
                id="block"
                icon="block"
                on={blocking}
                onAction={() => {
                  if (blocking) void write("unblock", () => change(account.id, "unblock"));
                  else setAsking({ kind: "block", busy: false });
                }}
              >
                {t(blocking ? "profile.unblock" : "profile.block")}
              </MenuItem>
              {relationship.followed_by === true && (
                <MenuItem
                  id="remove-follower"
                  icon="removeFollower"
                  onAction={() => {
                    setAsking({ kind: "remove", busy: false });
                  }}
                >
                  {t("profile.remove_follower")}
                </MenuItem>
              )}
              <MenuItem
                id="report"
                icon="report"
                onAction={() => {
                  setReporting(true);
                }}
              >
                {t("report.menu")}
              </MenuItem>
            </MenuGroup>
            {remote && (
              <MenuGroup title={account.domain}>
                <MenuItem
                  id="mute-server"
                  icon="mute"
                  on={mutedDomain !== undefined}
                  onAction={() => {
                    void (mutedDomain === undefined
                      ? write("mute_server", () => muteServer(account.domain, true))
                      : write("unmute_server", () => muteServer(mutedDomain, false)));
                  }}
                >
                  {t(mutedDomain === undefined ? "profile.mute_server" : "profile.unmute_server")}
                </MenuItem>
              </MenuGroup>
            )}
          </>
        )}
      </MenuButton>
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            key={asking.kind}
            title={t(asking.kind === "block" ? "profile.block_confirm_title" : "profile.remove_follower_title", {
              handle,
            })}
            cancel={t("profile.cancel")}
            confirm={t(asking.kind === "block" ? "profile.block_confirm" : "profile.remove_follower_confirm")}
            busy={asking.busy}
            onCancel={() => {
              if (!asking.busy) setAsking(undefined);
            }}
            onConfirm={confirm}
          >
            {t(asking.kind === "block" ? "profile.block_confirm_text" : "profile.remove_follower_text")}
          </ConfirmDialog>
        )}
        {reporting && (
          <ReportDialog
            key="report"
            subject={{ kind: "account", account }}
            onClose={() => {
              setReporting(false);
            }}
          />
        )}
      </ModalPresence>
    </>
  );
}

/**
 * A visitor's main action and menu on another account's profile: Follow (Request to follow for a locked account), and
 * a menu of the original page of a remote account and Report, each that needs an account leading to sign in, given
 * the profile's address to come back to.
 */
function VisitorActions({ account, signIn, t }: { account: Account; signIn: (returnTo: string) => void; t: T }) {
  const back = () => {
    signIn(accountPath(account));
  };
  return (
    <>
      <Button size="timeline" kind="primary" onPress={back}>
        {t(account.locked ? "profile.request_follow" : "profile.follow")}
      </Button>
      <MenuButton icon="more" label={t("profile.more")}>
        {account.acct.includes("@") && (
          <MenuGroup>
            <MenuItem id="open" icon="open" onAction={openOriginal(account)}>
              {t("profile.open_original")}
            </MenuItem>
          </MenuGroup>
        )}
        <MenuGroup title={sentenceHandleOf(account)}>
          <MenuItem id="report" icon="report" onAction={back}>
            {t("report.menu")}
          </MenuItem>
        </MenuGroup>
      </MenuButton>
    </>
  );
}

/**
 * The reader's private note on the account, a line under the counts that opens its editor in place: the note, or with
 * none, what adding one does. Saving an empty note removes it; Cancel keeps it as it was.
 */
function Note({ account, note, t }: { account: Account; note: string; t: T }) {
  const save = useWriteAccountNote();
  const write = useAccountWrite(t);
  const [editing, setEditing] = useState<{ text: string; busy: boolean } | undefined>(undefined);
  if (editing === undefined)
    return (
      <NoteLine
        note={note}
        placeholder={t("profile.note_add")}
        label={t("profile.note_edit")}
        onPress={() => {
          setEditing({ text: note, busy: false });
        }}
      />
    );
  return (
    <NoteEditor
      label={t("profile.note")}
      value={editing.text}
      onChange={(text) => {
        setEditing({ ...editing, text });
      }}
      maxLength={accountNoteLimit}
      cancel={t("profile.cancel")}
      save={t("profile.note_save")}
      busy={editing.busy}
      onCancel={() => {
        setEditing(undefined);
      }}
      onSave={() => {
        const text = editing.text.trim();
        setEditing({ ...editing, busy: true });
        void write("note", () => save(account.id, text)).then((saved) => {
          // A note that failed stays in its editor, as the reader wrote it.
          setEditing(saved ? undefined : { ...editing, busy: false });
        });
      }}
    />
  );
}

/**
 * The open voice chat room where the account hosts or speaks, which the reader sees (ADR-0100 §8), as a line that
 * leads to it; nothing for none, for a listener, without a session, or while the voice chat is off.
 */
function VoiceEntry({ account, signedIn }: { account: Account; signedIn: boolean }) {
  const on = useInstance()?.voice_enabled === true;
  const entry = useAccountVoiceRoom(account.id, on && signedIn);
  return entry === undefined || entry === null ? null : <VoiceRoomEntryLine entry={entry} />;
}

/** How many of the account's upcoming events its profile shows. */
const eventsShown = 3;

/**
 * The upcoming events the account organizes that the reader sees (ADR-0161 §9, FR-951), the nearest first, as lines
 * that lead to them; nothing for none, without a session, or for an account of another server, which has none here.
 */
function EventsEntry({ account, signedIn, t }: { account: Account; signedIn: boolean; t: T }) {
  const list = useEvents({ organizerId: account.id }, signedIn && !account.acct.includes("@"));
  if (list.items.length === 0) return null;
  const now = Date.now();
  return (
    <RowList label={t("profile.events")}>
      {list.items.slice(0, eventsShown).map((event) => (
        <RowItem key={event.id}>
          <EventListLine event={event} now={now} />
        </RowItem>
      ))}
    </RowList>
  );
}

function Head({
  account,
  relationship,
  own,
  signIn,
  t,
  lang,
}: {
  account: Account;
  relationship: Relationship | undefined;
  own: boolean;
  signIn: ((returnTo: string) => void) | undefined;
  t: T;
  lang: string;
}) {
  const nameId = useId();
  const change = useChangeRelationship();
  const featured = useFeaturedHashtags(account.id);
  const n = (v: number) => new Intl.NumberFormat(lang).format(v);
  const joined = dateFormat(lang, useTimeZone(), { year: "numeric", month: "long" }).format(
    new Date(account.created_at),
  );
  const blocked = relationship?.blocking ?? false;
  const remote = account.acct.includes("@");
  const signedIn = signIn === undefined;
  const mutedDomain = useMutedDomain(remote && !own && signedIn ? account.domain : undefined);
  const muteServer = useMuteDomain();
  const write = useAccountWrite(t);
  const avatar = account.avatar?.medium.url ?? account.avatar?.small.url;
  // A moved account's main action leads to the account it moved to, which offers follow; its followers moved with it.
  const moved = account.moved;
  const goToNew = moved && (
    <LinkButton href={accountPath(moved)} kind="primary" size="timeline">
      {t("profile.go_to_new")}
    </LinkButton>
  );
  return (
    <ProfileHead
      nameId={nameId}
      banner={account.header?.medium.url}
      avatar={<Avatar name={nameOf(account)} src={avatar} size="preview" decorative />}
      actions={
        own ? (
          <LinkButton href={profileEditPath} size="timeline">
            {t("settings.profile.edit")}
          </LinkButton>
        ) : signIn !== undefined ? (
          (goToNew ?? <VisitorActions account={account} signIn={signIn} t={t} />)
        ) : (
          relationship !== undefined && (
            <>
              {moved === undefined && relationship.following === true && !blocked && (
                <Bell account={account} relationship={relationship} t={t} />
              )}
              {goToNew ?? <Follow account={account} relationship={relationship} t={t} />}
              <More account={account} relationship={relationship} mutedDomain={mutedDomain} t={t} />
            </>
          )
        )
      }
      name={<AccountName account={account} />}
      marks={
        <>
          {account.locked && (
            <span role="img" aria-label={t("profile.locked")}>
              <Icon name="followers" place="caption" />
            </span>
          )}
          {account.bot && <Badge>{t("profile.bot")}</Badge>}
        </>
      }
      handle={handleOf(account)}
      badges={
        <>
          {moved === undefined && relationship?.followed_by && <Badge>{t("profile.follows_you")}</Badge>}
          {account.badges.map((b) => (
            <Badge key={b.role_id}>{b.label}</Badge>
          ))}
        </>
      }
    >
      {moved !== undefined && (
        <CaptionLine icon="moved">
          {formatParts(
            t,
            "profile.moved_to",
            {},
            {
              handle: (
                <bdi dir="ltr">
                  <TextLink href={accountPath(moved)}>{handleOf(moved)}</TextLink>
                </bdi>
              ),
            },
          )}
        </CaptionLine>
      )}
      {blocked ? (
        <CaptionLine icon="block">{t("profile.blocked")}</CaptionLine>
      ) : (
        <>
          {relationship?.muting && (
            <CaptionLine
              icon="mute"
              action={
                <TextButton
                  onPress={() => {
                    void write("unmute", () => change(account.id, "unmute"));
                  }}
                >
                  {t("profile.unmute")}
                </TextButton>
              }
            >
              {t("profile.muted")}
            </CaptionLine>
          )}
          {account.note !== "" && <PostContent html={account.note} emojis={account.emojis} lang={undefined} />}
          {account.fields.length > 0 && (
            <FieldList
              items={account.fields.map((f) => ({
                name: f.name,
                value: <PostContent html={f.value} emojis={account.emojis} lang={undefined} />,
              }))}
            />
          )}
        </>
      )}
      <MetaLine>
        <span>{t("profile.joined", { date: joined })}</span>
        {account.following_count !== undefined && (
          <MetaCount
            label={t("profile.following_count")}
            value={n(account.following_count)}
            href={`${accountPath(account)}/following`}
          />
        )}
        {account.followers_count !== undefined && (
          <MetaCount
            label={t("profile.followers_count")}
            value={n(account.followers_count)}
            href={`${accountPath(account)}/followers`}
          />
        )}
      </MetaLine>
      {!blocked && <VoiceEntry account={account} signedIn={signedIn} />}
      {!blocked && <EventsEntry account={account} signedIn={signedIn} t={t} />}
      {!own && relationship?.note !== undefined && <Note account={account} note={relationship.note} t={t} />}
      {mutedDomain !== undefined && (
        <CaptionLine
          icon="mute"
          action={
            <TextButton
              onPress={() => {
                void write("unmute_server", () => muteServer(mutedDomain, false));
              }}
            >
              {t("profile.unmute_server")}
            </TextButton>
          }
        >
          {t("profile.server_muted", { domain: mutedDomain })}
        </CaptionLine>
      )}
      {account.locked && <CaptionLine icon="followers">{t("profile.locked")}</CaptionLine>}
      {remote && (
        <CaptionLine
          icon="open"
          action={<TextButton onPress={openOriginal(account)}>{t("profile.open_original")}</TextButton>}
        >
          {t("profile.remote", { domain: account.domain })}
        </CaptionLine>
      )}
      {!blocked && featured.length > 0 && (
        <TagList
          heading={t("profile.featured")}
          items={featured.map((tag) => ({
            name: tag.name,
            href: `/tags/${encodeURIComponent(tag.name)}`,
            count: t("profile.tag_posts", { count: tag.statuses_count }),
          }))}
        />
      )}
    </ProfileHead>
  );
}

/** A post of the list: the pinned ones first, keyed apart from the same post further down. */
type Item = { readonly status: Status; readonly pinned: boolean };

function Posts({
  account,
  tab,
  closed,
  signIn,
  t,
}: {
  account: Account;
  tab: Tab;
  closed: boolean;
  signIn: ((returnTo: string) => void) | undefined;
  t: T;
}) {
  const posts = useAccountStatuses(account.id, tab);
  // The pinned posts lead the lists of all posts, not the filters of some (media, articles).
  const withPinned = tab === "posts" || tab === "replies";
  const pinned = useAccountStatuses(account.id, "pinned", withPinned);
  const onAction = usePostActions();
  const votingOf = usePostVoting();
  const ownOf = usePostOwn();
  const picker = usePostPicker();
  const items: Item[] = [
    ...(withPinned ? pinned.items.map((status) => ({ status, pinned: true })) : []),
    ...posts.items.map((status) => ({ status, pinned: false })),
  ];
  return (
    <ListColumn
      {...posts}
      label={t("profile.tabs")}
      items={items}
      keyOf={(item) => `${item.pinned ? "pinned:" : ""}${item.status.id}`}
      empty={closed ? <FeedNotice>{t("profile.locked_posts")}</FeedNotice> : undefined}
    >
      {(item) => (
        <Post
          status={item.status}
          context={item.pinned ? <CaptionLine icon="pin">{t("profile.pinned")}</CaptionLine> : undefined}
          onAction={onAction}
          {...(signIn === undefined
            ? { voting: votingOf(item.status), own: ownOf(item.status), picker }
            : {
                signIn: (status: PostStatus) => {
                  signIn(statusPath(status));
                },
              })}
        />
      )}
    </ListColumn>
  );
}

/**
 * What a profile shows of an account that is not there, whether it never was, was deleted or is deactivated, which
 * the server does not tell apart (ADR-0089 §15): back (`onBack`), then a line that names the handle opened.
 */
function NotFound({ acct, onBack }: { acct: string; onBack: (() => void) | undefined }) {
  const { t } = stringsIn(useLocale().locale);
  return (
    <ScreenStack>
      {onBack !== undefined && <BackButton onPress={onBack}>{t("profile.not_found_back")}</BackButton>}
      <FeedNotice start>{t("profile.not_found", { handle: `@${acct}` })}</FeedNotice>
    </ScreenStack>
  );
}

/**
 * The profile of the account of a handle (`user`, or `user@domain` for a remote one); `onBack` leaves the profile of
 * an account that is not there. With `signIn`, for a visitor, the actions that need an account lead to sign in, given
 * the address to come back to.
 */
export function Profile({
  acct,
  onBack,
  signIn,
}: {
  acct: string;
  onBack?: (() => void) | undefined;
  signIn?: ((returnTo: string) => void) | undefined;
}) {
  const { t, lang } = stringsIn(useLocale().locale);
  const signedIn = signIn === undefined;
  const { account, error, missing } = useAccount(acct, signedIn);
  const viewer = useViewer(signedIn);
  const own = account !== undefined && viewer?.id === account.id;
  const relationship = useRelationship(account === undefined || own || missing || !signedIn ? undefined : account.id);
  const [tab, setTab] = useState<Tab>("posts");
  if (missing) return <NotFound acct={acct} onBack={onBack} />;
  if (account === undefined)
    return error === null ? (
      <ScreenStack busy>
        <FeedNotice>{t("profile.loading")}</FeedNotice>
      </ScreenStack>
    ) : (
      <FeedNotice alert>{t("profile.failed")}</FeedNotice>
    );
  const blocked = relationship?.blocking ?? false;
  const closed = account.locked && !own && !(relationship?.following ?? false);
  return (
    <ScreenStack>
      <Head account={account} relationship={relationship} own={own} signIn={signIn} t={t} lang={lang} />
      {blocked ? (
        <FeedNotice>{t("profile.blocked_posts")}</FeedNotice>
      ) : (
        <TabRow
          label={t("profile.tabs")}
          tabs={[
            { id: "posts", name: t("profile.posts") },
            { id: "replies", name: t("profile.posts_and_replies") },
            { id: "media", name: t("profile.media") },
            { id: "articles", name: t("profile.articles") },
          ]}
          current={tab}
          onChange={setTab}
        >
          <Posts account={account} tab={tab} closed={closed} signIn={signIn} t={t} />
        </TabRow>
      )}
    </ScreenStack>
  );
}
