// SPDX-License-Identifier: AGPL-3.0-only

// A profile's relations (#3122, #3123), in the reader's language (the toolbar's locale; ja, else en): the reader's
// private note on the account, a line that opens its editor in place; the bell of its new posts beside follow while the reader follows it; its menu
// (remove a follower, block after a confirmation, mute its server); a post's menu with its author's and its server's
// actions, and what a post turns into once one of them is done; a moved account, whose main action leads to the account
// it moved to; and an account that is not there (deleted, deactivated, or never was). The words are placeholders of the
// feature's (appfeatures:profile). On PC the screens sit in the app's columns; here, in a column of the timeline's width.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId, useState, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { Avatar } from "./avatar";
import { Button, LinkButton, TextButton, TextLink } from "./button";
import { ConfirmDialog } from "./dialog";
import { CaptionLine } from "./entry";
import { FeedNotice } from "./feed";
import { IconButton } from "./icon-button";
import { BackButton, ScreenStack } from "./list";
import { MenuButton, MenuGroup, MenuItem, MenuPanel } from "./menu";
import { Badge, FieldList, MetaCount, MetaLine, NoteEditor, NoteLine, ProfileHead } from "./profile";
import { images } from "./samples";

const rem = (px: number) => `${px / rootPx}rem`;

type Lang = "en" | "ja";
const useLang = (): Lang => (new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en");

type Person = { name: string; handle: string; avatar?: string | undefined };

const owner: Record<Lang, Person> = {
  en: { name: "Sam Rivera", handle: "@sam@social.example", avatar: images.snow },
  ja: { name: "サム・リベラ", handle: "@sam@social.example", avatar: images.snow },
};

const movedTo = "@sam@photos.example";

const words = {
  en: {
    bio: "Landscape photographer. Ridges, coastlines and the light just before it rains.",
    site: "Website",
    joined: "Joined March 2023",
    followingCount: "Following",
    followersCount: "Followers",
    more: "More",
    follow: "Follow",
    followBack: "Follow back",
    following: "Following",
    followsYou: "Follows you",
    openOriginal: "Open original page",
    mute: "Mute",
    unmute: "Unmute",
    block: "Block…",
    removeFollower: "Remove follower…",
    muteServer: "Mute server",
    unmuteServer: "Unmute server",
    notify: "Notify me of new posts",
    tabs: "Following and followers",
    back: "Back to the profile",
    hiddenFollowers: (name: string) => `${name} shows this list only to their followers.`,
    hiddenPrivate: (name: string) => `${name} keeps this list to themselves.`,
    emptyFollowers: "No followers yet.",
    loading: "Loading the followers…",
    failed: "The followers could not be loaded.",
    retry: "Try again",
    showMore: "Show more",
    note: "Private note",
    addNote: "Add a note only you can see",
    editNote: "Edit note",
    save: "Save",
    cancel: "Cancel",
    noteText: "Met at the photo walk in May. Prints on film; ask about the ridge series.",
    blockTitle: (handle: string) => `Block ${handle}?`,
    blockText:
      "Follows between you end, and while the block lasts neither of you can follow the other or see their posts. You can unblock them later, but the follows do not come back.",
    blockConfirm: "Block",
    removeTitle: (handle: string) => `Remove ${handle} from your followers?`,
    removeText: "They stop seeing your followers-only posts. They can follow you again.",
    removeConfirm: "Remove",
    serverMuted: (domain: string) => `You muted the server ${domain}.`,
    serverMutedNow: (domain: string) => `You muted the server ${domain}.`,
    muted: (handle: string) => `You muted ${handle}.`,
    blocked: (handle: string) => `You blocked ${handle}.`,
    undo: "Undo",
    unblock: "Unblock",
    movedTo: "Moved to",
    goToNew: "Go to new account",
    gone: (handle: string) => `${handle} was not found. The account may no longer exist, or the link may be wrong.`,
    goneBack: "Back",
    copyLink: "Copy link",
    share: "Share",
    bookmark: "Bookmark",
  },
  ja: {
    bio: "風景写真を撮っています。尾根、海岸線、雨が降る直前の光。",
    site: "ウェブサイト",
    joined: "2023年3月に登録",
    followingCount: "フォロー",
    followersCount: "フォロワー",
    more: "その他",
    follow: "フォロー",
    followBack: "フォローバック",
    following: "フォロー中",
    followsYou: "フォローされています",
    openOriginal: "元のページを表示",
    mute: "ミュート",
    unmute: "ミュートを解除",
    block: "ブロック…",
    removeFollower: "フォロワーから削除…",
    muteServer: "サーバーをミュート",
    unmuteServer: "サーバーのミュートを解除",
    notify: "新しい投稿を通知",
    tabs: "フォローとフォロワー",
    back: "プロフィールに戻る",
    hiddenFollowers: (name: string) => `${name}さんはこの一覧をフォロワーにだけ公開しています。`,
    hiddenPrivate: (name: string) => `${name}さんはこの一覧を公開していません。`,
    emptyFollowers: "フォロワーはまだいません。",
    loading: "フォロワーを読み込んでいます…",
    failed: "フォロワーを読み込めませんでした。",
    retry: "もう一度試す",
    showMore: "さらに表示",
    note: "非公開のメモ",
    addNote: "自分だけが見られるメモを追加",
    editNote: "メモを編集",
    save: "保存",
    cancel: "キャンセル",
    noteText: "5月の撮影会で会った。フィルムで焼いている。尾根のシリーズについて聞く。",
    blockTitle: (handle: string) => `${handle} をブロックしますか？`,
    blockText:
      "お互いのフォローは解除され、ブロックしている間は、どちらも相手をフォローできず、相手の投稿も見られません。ブロックは後で解除できますが、フォローは元に戻りません。",
    blockConfirm: "ブロック",
    removeTitle: (handle: string) => `${handle} をフォロワーから削除しますか？`,
    removeText: "相手はフォロワー限定の投稿を見られなくなります。もう一度フォローすることはできます。",
    removeConfirm: "削除",
    serverMuted: (domain: string) => `サーバー ${domain} をミュートしています。`,
    serverMutedNow: (domain: string) => `サーバー ${domain} をミュートしました。`,
    muted: (handle: string) => `${handle} をミュートしました。`,
    blocked: (handle: string) => `${handle} をブロックしました。`,
    undo: "元に戻す",
    unblock: "ブロックを解除",
    movedTo: "移転先:",
    goToNew: "新しいアカウントに移動",
    gone: (handle: string) =>
      `${handle} は見つかりませんでした。アカウントがもう存在しないか、リンクが間違っている可能性があります。`,
    goneBack: "戻る",
    copyLink: "リンクをコピー",
    share: "共有",
    bookmark: "ブックマーク",
  },
} as const;

type W = (typeof words)[Lang];

const domainOf = (handle: string) => handle.slice(handle.lastIndexOf("@") + 1);
const userOf = (handle: string) => handle.slice(0, handle.lastIndexOf("@"));

/** A column of the timeline's width, with its side margin. */
const Column = ({ children }: { children: ReactNode }) => (
  <div style={{ maxWidth: rem(600), paddingInline: rem(16), paddingBlock: rem(16) }}>{children}</div>
);

/** A menu shown open, as wide as its items, under the more button at the end of the head's row. */
const OpenMenu = ({ label, children }: { label: string; children: ReactNode }) => (
  <div style={{ display: "flex", justifyContent: "flex-end", marginBlockStart: rem(-8) }}>
    <MenuPanel label={label}>{children}</MenuPanel>
  </div>
);

/** The profile's menu: the remote account's page, the account's actions, its server's. */
function ProfileItems({
  w,
  follower = true,
  serverMuted = false,
}: {
  w: W;
  follower?: boolean;
  serverMuted?: boolean;
}) {
  const p = owner.en;
  return (
    <>
      <MenuGroup>
        <MenuItem id="open" icon="open">
          {w.openOriginal}
        </MenuItem>
      </MenuGroup>
      <MenuGroup title={userOf(p.handle)}>
        <MenuItem id="mute" icon="mute">
          {w.mute}
        </MenuItem>
        <MenuItem id="block" icon="block">
          {w.block}
        </MenuItem>
        {follower && (
          <MenuItem id="remove" icon="removeFollower">
            {w.removeFollower}
          </MenuItem>
        )}
      </MenuGroup>
      <MenuGroup title={domainOf(p.handle)}>
        <MenuItem id="mute-server" icon="mute" on={serverMuted}>
          {serverMuted ? w.unmuteServer : w.muteServer}
        </MenuItem>
      </MenuGroup>
    </>
  );
}

/** More, at the far end of the head's row. */
const More = ({ w, follower }: { w: W; follower: boolean }) => (
  <MenuButton icon="more" label={w.more}>
    <ProfileItems w={w} follower={follower} />
  </MenuButton>
);

/**
 * The head of Sam's profile, a remote account that follows the reader: its counts open its lists; `main` is the main
 * action and what sits beside it (follow back by default), more at the far end; `lines` sit after the counts.
 */
function Head({
  w,
  main,
  lines,
  before,
  followsYou = true,
}: {
  w: W;
  main?: ReactNode;
  lines?: ReactNode;
  before?: ReactNode;
  followsYou?: boolean;
}) {
  const p = owner[useLang()];
  const nameId = useId();
  return (
    <ProfileHead
      nameId={nameId}
      banner={images.dusk}
      avatar={<Avatar name={p.name} src={p.avatar} size="preview" decorative />}
      actions={
        <>
          {main ?? (
            <Button size="timeline" kind="primary">
              {w.followBack}
            </Button>
          )}
          <More w={w} follower={followsYou} />
        </>
      }
      name={p.name}
      handle={p.handle}
      badges={followsYou ? <Badge>{w.followsYou}</Badge> : undefined}
    >
      {before}
      <p className="m-0" dir="auto">
        {w.bio}
      </p>
      <FieldList items={[{ name: w.site, value: "ridgeline.example" }]} />
      <MetaLine>
        <span>{w.joined}</span>
        <MetaCount label={w.followingCount} value="312" href="#following" />
        <MetaCount label={w.followersCount} value="1,284" href="#followers" />
      </MetaLine>
      {lines}
    </ProfileHead>
  );
}

// #3122: the note, new posts, the menu and its confirmations.

/** The note, a line of the head that opens its editor in place: none, written, or open in its editor. */
function Note({ written, editing = false }: { written: boolean; editing?: boolean }) {
  const w = words[useLang()];
  const [open, setOpen] = useState(editing);
  const [text, setText] = useState(written ? w.noteText : "");
  return (
    <Column>
      <Head
        w={w}
        lines={
          open ? (
            <NoteEditor
              label={w.note}
              value={text}
              onChange={setText}
              maxLength={2000}
              cancel={w.cancel}
              save={w.save}
              onCancel={() => {
                setOpen(false);
              }}
              onSave={() => {
                setOpen(false);
              }}
            />
          ) : (
            <NoteLine
              note={text}
              placeholder={w.addNote}
              label={w.editNote}
              onPress={() => {
                setOpen(true);
              }}
            />
          )
        }
      />
    </Column>
  );
}

/** The bell of the account's new posts, beside follow while the reader follows it: an outline off, filled on. */
function Notify({ on }: { on: boolean }) {
  const w = words[useLang()];
  return (
    <Column>
      <Head
        w={w}
        main={
          <>
            <IconButton icon="notifications" label={w.notify} pressed={on} />
            <Button size="timeline">{w.following}</Button>
          </>
        }
      />
    </Column>
  );
}

/** A confirmation over the profile, the page dimmed behind it. */
function Confirm({ kind }: { kind: "block" | "remove" }) {
  const lang = useLang();
  const w = words[lang];
  const handle = userOf(owner[lang].handle);
  return (
    <div>
      <Column>
        <Head w={w} />
      </Column>
      {/* The page dimmed and blurred behind it, as ConfirmDialog's layer draws it (a specimen holds still for axe). */}
      <div
        style={{
          position: "fixed",
          inset: 0,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          padding: rem(16),
          background: "var(--dialog-scrim)",
          backdropFilter: "blur(var(--dialog-blur))",
        }}
      >
        <ConfirmDialog
          layer={false}
          title={kind === "block" ? w.blockTitle(handle) : w.removeTitle(handle)}
          cancel={w.cancel}
          confirm={kind === "block" ? w.blockConfirm : w.removeConfirm}
          onCancel={() => undefined}
          onConfirm={() => undefined}
        >
          {kind === "block" ? w.blockText : w.removeText}
        </ConfirmDialog>
      </div>
    </div>
  );
}

/** The profile of an account whose server the reader muted: a line says so with its undo, the menu turns it off. */
function ServerMuted() {
  const lang = useLang();
  const w = words[lang];
  return (
    <Column>
      <ScreenStack>
        <Head
          w={w}
          lines={
            <CaptionLine icon="mute" action={<TextButton>{w.unmute}</TextButton>}>
              {w.serverMuted(domainOf(owner[lang].handle))}
            </CaptionLine>
          }
        />
        <OpenMenu label={w.more}>
          <ProfileItems w={w} serverMuted />
        </OpenMenu>
      </ScreenStack>
    </Column>
  );
}

/** A post's more menu by another, remote account: what reaches the post, then its author's and its server's actions. */
function PostMenu() {
  const lang = useLang();
  const w = words[lang];
  const handle = owner[lang].handle;
  return (
    <Column>
      <div style={{ display: "flex" }}>
        <MenuPanel label={w.more}>
          <MenuGroup>
            <MenuItem id="copy-link" icon="link">
              {w.copyLink}
            </MenuItem>
            <MenuItem id="open" icon="open">
              {w.openOriginal}
            </MenuItem>
            <MenuItem id="share" icon="share">
              {w.share}
            </MenuItem>
          </MenuGroup>
          <MenuGroup>
            <MenuItem id="bookmark" icon="bookmark">
              {w.bookmark}
            </MenuItem>
          </MenuGroup>
          <MenuGroup title={userOf(handle)}>
            <MenuItem id="mute" icon="mute">
              {w.mute}
            </MenuItem>
            <MenuItem id="block" icon="block">
              {w.block}
            </MenuItem>
          </MenuGroup>
          <MenuGroup title={domainOf(handle)}>
            <MenuItem id="mute-server" icon="mute">
              {w.muteServer}
            </MenuItem>
          </MenuGroup>
        </MenuPanel>
      </div>
    </Column>
  );
}

/** Where a post was, once the reader muted or blocked its author or muted its server from its menu: a line and its undo. */
function PostDone() {
  const lang = useLang();
  const w = words[lang];
  const handle = owner[lang].handle;
  return (
    <Column>
      <ScreenStack>
        <CaptionLine icon="mute" action={<TextButton>{w.undo}</TextButton>}>
          {w.muted(userOf(handle))}
        </CaptionLine>
        <CaptionLine icon="block" action={<TextButton>{w.unblock}</TextButton>}>
          {w.blocked(userOf(handle))}
        </CaptionLine>
        <CaptionLine icon="mute" action={<TextButton>{w.undo}</TextButton>}>
          {w.serverMutedNow(domainOf(handle))}
        </CaptionLine>
      </ScreenStack>
    </Column>
  );
}

// #3123: a moved account, and one that is not there.

/**
 * A moved account: its main action leads to the account it moved to, whose own profile offers follow; a line names
 * that account, its handle the link. No "Follows you": the followers moved with it.
 */
function Moved() {
  const w = words[useLang()];
  return (
    <Column>
      <Head
        w={w}
        followsYou={false}
        main={
          <LinkButton href="#" kind="primary" size="timeline">
            {w.goToNew}
          </LinkButton>
        }
        before={
          <CaptionLine icon="moved">
            {w.movedTo}{" "}
            <bdi dir="ltr">
              <TextLink href="#">{movedTo}</TextLink>
            </bdi>
          </CaptionLine>
        }
      />
    </Column>
  );
}

/** The profile of an account that is not there, whether deleted, deactivated or never made: back, then one line. */
function Gone() {
  const w = words[useLang()];
  return (
    <Column>
      <ScreenStack>
        <BackButton onPress={() => undefined}>{w.goneBack}</BackButton>
        <FeedNotice start>{w.gone("@sam@my.example")}</FeedNotice>
      </ScreenStack>
    </Column>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Profile relations", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const NoteNone: Story = { render: () => <Note written={false} /> };
export const NoteWritten: Story = { render: () => <Note written /> };
export const NoteEditing: Story = { render: () => <Note written editing /> };
export const NotifyOff: Story = { render: () => <Notify on={false} /> };
export const NotifyOn: Story = { render: () => <Notify on /> };
export const BlockConfirm: Story = { render: () => <Confirm kind="block" /> };
export const RemoveFollowerConfirm: Story = { render: () => <Confirm kind="remove" /> };
export const ServerMutedProfile: Story = { render: () => <ServerMuted /> };
export const PostMenuOfAnother: Story = { render: () => <PostMenu /> };
export const PostAfterMuteOrBlock: Story = { render: () => <PostDone /> };
export const MovedAccount: Story = { render: () => <Moved /> };
export const AccountNotThere: Story = { render: () => <Gone /> };
