// SPDX-License-Identifier: AGPL-3.0-only

// One menu for every menu-like surface of the app (#3129), in the reader's language (the toolbar's locale; ja, else
// en): on a phone, a sheet from the bottom edge over the page dimmed and blurred as under every modal, headed by its
// name and close, its items scrolling under the head; from `tablet`, a popover under its trigger, at its end, with the
// same items, captions and groups and no head. A submenu takes its menu's place, its head a back control before its
// name, in a sheet and a popover alike. Every kind of item (a glyph and its label, a line under it, a toggle on, the
// value chosen, a submenu with what it holds now, one that cannot be done now, and the destructive ones last and
// apart), and each surface of the inventory: a post's more and boost menus, a profile's, Add to list, a conversation's,
// a draft's, the composer's drafts, New post's, the phone's Menu, the composer's audience with its submenus, an
// article's visibility, the post's language and a poll's duration. The sheet and its backdrop are drawn still, as they
// rest once open; the Open stories show the menu open. The words are the features' where they
// exist, else placeholders.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useState, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { IconButton } from "./icon-button";
import type { IconName } from "./icons";
import { MenuButton, MenuGroup, MenuHead, MenuItem, MenuPanel } from "./menu";

const rem = (px: number) => `${px / rootPx}rem`;

type Lang = "en" | "ja";
const useLang = (): Lang => (new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en");

/** From `tablet` (40rem) a menu is a popover; under it, a sheet. */
function useWide(): boolean {
  const query = "(min-width: 40rem)";
  const [wide, setWide] = useState(() => typeof window !== "undefined" && window.matchMedia(query).matches);
  useEffect(() => {
    const m = window.matchMedia(query);
    const on = () => {
      setWide(m.matches);
    };
    m.addEventListener("change", on);
    return () => {
      m.removeEventListener("change", on);
    };
  }, []);
  return wide;
}

const words = {
  en: {
    close: "Close",
    back: "Back",
    more: "More",
    copyLink: "Copy link",
    openOriginal: "Open original page",
    share: "Share",
    bookmark: "Bookmark",
    edit: "Edit",
    redraft: "Delete and redraft…",
    delete: "Delete…",
    mute: "Mute",
    block: "Block…",
    muteServer: "Mute server",
    boost: "Boost",
    unboost: "Undo boost",
    quote: "Quote",
    quoteWaits: "Waits for @sam's approval",
    addToList: "Add to list",
    addToListAbout: "You can add only accounts you follow",
    removeFollower: "Remove follower…",
    newList: "New list",
    lists: ["Photographers", "Friends", "Bread people", "Local news"],
    hide: "Hide from the list",
    open: "Open",
    schedule: "Change time",
    later: "Drafts",
    saveDraft: "Save as draft",
    drafts: "Drafts and scheduled posts",
    create: "New",
    newPost: "New post",
    newArticle: "New article",
    menu: "Menu",
    places: ["Bookmarks", "Lists", "Hashtags you follow", "Events", "Settings"],
    about: "About this server",
    audienceMenu: "Who can see and reply",
    audience: "Who can see it",
    visibilities: ["Public", "Quiet public", "Followers only", "Direct"],
    interactions: "Who can reply, boost and quote",
    interactBy: ["Anyone", "Followers", "Only you"],
    visibility: "Who can see it",
    language: "Language",
    languages: [
      "Japanese",
      "English",
      "Korean",
      "Chinese (Simplified)",
      "Chinese (Traditional)",
      "Spanish",
      "French",
      "German",
      "Portuguese (Brazil)",
      "Arabic",
      "Indonesian",
      "Thai",
    ],
    duration: "Duration",
    durations: ["5 minutes", "1 hour", "1 day", "3 days", "7 days"],
  },
  ja: {
    close: "閉じる",
    back: "戻る",
    more: "その他",
    copyLink: "リンクをコピー",
    openOriginal: "元のページを表示",
    share: "共有",
    bookmark: "ブックマーク",
    edit: "編集",
    redraft: "削除して下書きに戻す…",
    delete: "削除…",
    mute: "ミュート",
    block: "ブロック…",
    muteServer: "サーバーをミュート",
    boost: "ブースト",
    unboost: "ブーストを取り消す",
    quote: "引用",
    quoteWaits: "@sam さんの承認待ちになります",
    addToList: "リストに追加",
    addToListAbout: "フォロー中のアカウントだけ追加できます",
    removeFollower: "フォロワーから削除…",
    newList: "新しいリスト",
    lists: ["写真家", "友だち", "パン好き", "地元のニュース"],
    hide: "一覧から非表示",
    open: "開く",
    schedule: "時刻を変更",
    later: "下書き",
    saveDraft: "下書きに保存",
    drafts: "下書きと予約投稿",
    create: "新規作成",
    newPost: "新しい投稿",
    newArticle: "新しい記事",
    menu: "メニュー",
    places: ["ブックマーク", "リスト", "フォロー中のハッシュタグ", "イベント", "設定"],
    about: "このサーバーについて",
    audienceMenu: "見られる人と返信できる人",
    audience: "見られる人",
    visibilities: ["公開", "ひっそり公開", "フォロワーのみ", "ダイレクト"],
    interactions: "返信・ブースト・引用できる人",
    interactBy: ["誰でも", "フォロワー", "自分だけ"],
    visibility: "見られる人",
    language: "言語",
    languages: [
      "日本語",
      "英語",
      "韓国語",
      "中国語（簡体字）",
      "中国語（繁体字）",
      "スペイン語",
      "フランス語",
      "ドイツ語",
      "ポルトガル語（ブラジル）",
      "アラビア語",
      "インドネシア語",
      "タイ語",
    ],
    duration: "期間",
    durations: ["5分", "1時間", "1日", "3日", "7日"],
  },
} as const;

type W = (typeof words)[Lang];

/**
 * A menu as it shows open: from `tablet`, a popover under the trigger at the column's end, headed only in a submenu;
 * on a phone, a sheet from the bottom edge over the dimmed and blurred page, headed by its name and close. In a
 * submenu (`sub`, its name), the head's back control returns to the menu it opened from.
 */
function Surface({
  label,
  icon = "more",
  sub,
  children,
}: {
  label: string;
  icon?: IconName;
  sub?: string | undefined;
  children: ReactNode;
}) {
  const w = words[useLang()];
  const wide = useWide();
  const back = sub === undefined ? undefined : { label: w.back };
  const trigger = (
    <div style={{ display: "flex", justifyContent: "flex-end" }}>
      <IconButton icon={icon} label={label} />
    </div>
  );
  if (wide)
    return (
      <div style={{ maxWidth: rem(600), padding: rem(16) }}>
        {trigger}
        <div style={{ display: "flex", justifyContent: "flex-end", marginBlockStart: rem(4) }}>
          <MenuPanel label={sub ?? label} head={sub === undefined ? undefined : <MenuHead title={sub} back={back} />}>
            {children}
          </MenuPanel>
        </div>
      </div>
    );
  return (
    <div style={{ minHeight: "100dvh" }}>
      <div style={{ padding: rem(16) }}>{trigger}</div>
      <div className="fixed inset-0 bg-scrim backdrop-blur-[var(--dialog-blur)]" />
      <div className="fixed inset-x-0 bottom-0">
        <MenuPanel
          label={sub ?? label}
          sheet
          head={<MenuHead title={sub ?? label} back={back} close={{ label: w.close }} />}
        >
          {children}
        </MenuPanel>
      </div>
    </div>
  );
}

/** Every kind of item, in groups under captions, the destructive ones last and apart. */
function ItemKindsStory() {
  const w = words[useLang()];
  return (
    <Surface label={w.more}>
      <MenuGroup>
        <MenuItem id="link" icon="link">
          {w.copyLink}
        </MenuItem>
        <MenuItem id="bookmark" icon="bookmark" on>
          {w.bookmark}
        </MenuItem>
      </MenuGroup>
      <MenuGroup title={w.visibility}>
        <MenuItem id="public" icon="public" chosen>
          {w.visibilities[0]}
        </MenuItem>
        <MenuItem id="followers" icon="followers">
          {w.visibilities[2]}
        </MenuItem>
      </MenuGroup>
      <MenuGroup title="@sam">
        <MenuItem id="quote" icon="quote" about={w.quoteWaits}>
          {w.quote}
        </MenuItem>
        <MenuItem id="lists" icon="list" opens="2">
          {w.addToList}
        </MenuItem>
        <MenuItem id="lists-off" icon="list" disabled about={w.addToListAbout}>
          {w.addToList}
        </MenuItem>
      </MenuGroup>
      <MenuGroup>
        <MenuItem id="delete" icon="delete">
          {w.delete}
        </MenuItem>
      </MenuGroup>
    </Surface>
  );
}

const visibilityGlyphs = ["public", "unlisted", "followers", "direct"] as const;

/** The visibilities, the chosen one checked. */
const Visibilities = ({ w }: { w: W }) => (
  <MenuGroup>
    {w.visibilities.map((v, i) => (
      <MenuItem key={v} id={`v-${String(i)}`} icon={visibilityGlyphs[i] ?? "public"} chosen={i === 0}>
        {v}
      </MenuItem>
    ))}
  </MenuGroup>
);

/** The composer's audience, entered into its first submenu: who can see the post, under the head's back control. */
function SubmenuStory() {
  const w = words[useLang()];
  return (
    <Surface label={w.audienceMenu} icon="public" sub={w.audience}>
      <Visibilities w={w} />
    </Surface>
  );
}

/** Add to list, a submenu of a profile's menu: the reader's lists, each box filled while it holds the account. */
function AddToListStory() {
  const w = words[useLang()];
  return (
    <Surface label={w.more} sub={w.addToList}>
      <MenuGroup>
        {w.lists.map((name, i) => (
          <MenuItem key={name} id={`l-${String(i)}`} icon="included" on={i % 2 === 0}>
            {name}
          </MenuItem>
        ))}
      </MenuGroup>
      <MenuGroup>
        <MenuItem id="new" icon="add">
          {w.newList}
        </MenuItem>
      </MenuGroup>
    </Surface>
  );
}

/** A surface of the inventory, as the one menu shows it. */
function Inventory({ kind }: { kind: string }) {
  const w = words[useLang()];
  switch (kind) {
    case "post":
      return (
        <Surface label={w.more}>
          <MenuGroup>
            <MenuItem id="link" icon="link">
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
          <MenuGroup title="@sam">
            <MenuItem id="mute" icon="mute">
              {w.mute}
            </MenuItem>
            <MenuItem id="block" icon="block">
              {w.block}
            </MenuItem>
          </MenuGroup>
          <MenuGroup title="social.example">
            <MenuItem id="mute-server" icon="mute">
              {w.muteServer}
            </MenuItem>
          </MenuGroup>
        </Surface>
      );
    case "own-post":
      return (
        <Surface label={w.more}>
          <MenuGroup>
            <MenuItem id="link" icon="link">
              {w.copyLink}
            </MenuItem>
            <MenuItem id="share" icon="share">
              {w.share}
            </MenuItem>
          </MenuGroup>
          <MenuGroup>
            <MenuItem id="edit" icon="edit">
              {w.edit}
            </MenuItem>
          </MenuGroup>
          <MenuGroup>
            <MenuItem id="redraft" icon="redraft">
              {w.redraft}
            </MenuItem>
            <MenuItem id="delete" icon="delete">
              {w.delete}
            </MenuItem>
          </MenuGroup>
        </Surface>
      );
    case "boost":
      return (
        <Surface label={w.boost} icon="boost">
          <MenuGroup>
            <MenuItem id="boost" icon="boost" on>
              {w.unboost}
            </MenuItem>
            <MenuItem id="quote" icon="quote" about={w.quoteWaits}>
              {w.quote}
            </MenuItem>
          </MenuGroup>
        </Surface>
      );
    case "profile":
      return (
        <Surface label={w.more}>
          <MenuGroup>
            <MenuItem id="open" icon="open">
              {w.openOriginal}
            </MenuItem>
          </MenuGroup>
          <MenuGroup title="@sam">
            <MenuItem id="lists" icon="list" opens="2">
              {w.addToList}
            </MenuItem>
            <MenuItem id="mute" icon="mute">
              {w.mute}
            </MenuItem>
            <MenuItem id="block" icon="block">
              {w.block}
            </MenuItem>
            <MenuItem id="remove" icon="removeFollower">
              {w.removeFollower}
            </MenuItem>
          </MenuGroup>
          <MenuGroup title="social.example">
            <MenuItem id="mute-server" icon="mute">
              {w.muteServer}
            </MenuItem>
          </MenuGroup>
        </Surface>
      );
    case "conversation":
      return (
        <Surface label={w.more}>
          <MenuGroup>
            <MenuItem id="hide" icon="hidden">
              {w.hide}
            </MenuItem>
          </MenuGroup>
        </Surface>
      );
    case "draft":
      return (
        <Surface label={w.more}>
          <MenuGroup>
            <MenuItem id="open" icon="edit">
              {w.open}
            </MenuItem>
            <MenuItem id="schedule" icon="schedule">
              {w.schedule}
            </MenuItem>
          </MenuGroup>
          <MenuGroup>
            <MenuItem id="delete" icon="delete">
              {w.delete}
            </MenuItem>
          </MenuGroup>
        </Surface>
      );
    case "later":
      return (
        <Surface label={w.later} icon="drafts">
          <MenuGroup>
            <MenuItem id="save" icon="saveDraft">
              {w.saveDraft}
            </MenuItem>
            <MenuItem id="drafts" icon="drafts">
              {w.drafts}
            </MenuItem>
          </MenuGroup>
        </Surface>
      );
    case "new-post":
      // Named by what it holds, not by its first item.
      return (
        <Surface label={w.create} icon="compose">
          <MenuGroup>
            <MenuItem id="post" icon="compose">
              {w.newPost}
            </MenuItem>
            <MenuItem id="article" icon="article">
              {w.newArticle}
            </MenuItem>
          </MenuGroup>
        </Surface>
      );
    case "nav":
      return (
        <Surface label={w.menu} icon="menu">
          <MenuGroup>
            {w.places.map((p, i) => (
              <MenuItem
                key={p}
                id={`p-${String(i)}`}
                icon={(["bookmark", "list", "hashtag", "events", "settings"] as const)[i] ?? "list"}
              >
                {p}
              </MenuItem>
            ))}
          </MenuGroup>
          <MenuGroup>
            <MenuItem id="about" icon="about">
              {w.about}
            </MenuItem>
          </MenuGroup>
        </Surface>
      );
    case "audience":
      // One name for both submenus: who can see the post, and who can reply, boost and quote it.
      return (
        <Surface label={w.audienceMenu} icon="public">
          <MenuGroup>
            <MenuItem id="visibility" icon="public" opens={w.visibilities[0]}>
              {w.audience}
            </MenuItem>
            <MenuItem id="interactions" icon="quote" opens={w.interactBy[0]}>
              {w.interactions}
            </MenuItem>
          </MenuGroup>
        </Surface>
      );
    case "article-visibility":
      return (
        <Surface label={w.visibility} icon="public">
          <Visibilities w={w} />
        </Surface>
      );
    case "language":
      return (
        <Surface label={w.language} icon="translate">
          <MenuGroup>
            {w.languages.map((l, i) => (
              <MenuItem key={l} id={`lang-${String(i)}`} icon="translate" chosen={i === 0}>
                {l}
              </MenuItem>
            ))}
          </MenuGroup>
        </Surface>
      );
    default:
      return (
        <Surface label={w.duration} icon="time">
          <MenuGroup>
            {w.durations.map((d, i) => (
              <MenuItem key={d} id={`d-${String(i)}`} icon="time" chosen={i === 2}>
                {d}
              </MenuItem>
            ))}
          </MenuGroup>
        </Surface>
      );
  }
}

/**
 * The one menu itself, `MenuButton` on `MenuSurface`, open as the reader opens it: a sheet on a phone (the stories'
 * `MenuSheet`, by the viewport, as the app's), a popover from `tablet`. `kind` picks a menu of actions with one that
 * cannot be done now, the values of one setting with the chosen one checked, or a menu entered into its submenu.
 */
function Live({ kind }: { kind: "actions" | "values" | "submenu" }) {
  const w = words[useLang()];
  const [open, setOpen] = useState(true);
  const [chosen, setChosen] = useState("v-0");
  const [panel, setPanel] = useState<"top" | "lists">(kind === "submenu" ? "lists" : "top");
  const [on, setOn] = useState<readonly string[]>(["l-0", "l-2"]);
  let items: ReactNode;
  if (kind === "values")
    items = (
      <MenuGroup chosen={chosen} onChoose={setChosen}>
        {w.visibilities.map((v, i) => (
          <MenuItem key={v} id={`v-${String(i)}`} icon={visibilityGlyphs[i] ?? "public"}>
            {v}
          </MenuItem>
        ))}
      </MenuGroup>
    );
  else if (panel === "lists")
    items = (
      <>
        <MenuGroup
          checked={on}
          stay
          onCheck={(id, checked) => {
            setOn(checked ? [...on, id] : on.filter((x) => x !== id));
          }}
        >
          {w.lists.map((name, i) => (
            <MenuItem key={name} id={`l-${String(i)}`} icon="included">
              {name}
            </MenuItem>
          ))}
        </MenuGroup>
        {/* New list, a group of its own after the lists, as Add to list on a profile has it. */}
        <MenuGroup>
          <MenuItem id="new" icon="add">
            {w.newList}
          </MenuItem>
        </MenuGroup>
      </>
    );
  else
    items = (
      <>
        <MenuGroup>
          <MenuItem id="link" icon="link">
            {w.copyLink}
          </MenuItem>
          <MenuItem id="bookmark" icon="bookmark" on>
            {w.bookmark}
          </MenuItem>
        </MenuGroup>
        <MenuGroup title="@sam">
          <MenuItem
            id="lists"
            icon="list"
            opens="2"
            onAction={() => {
              setPanel("lists");
            }}
          >
            {w.addToList}
          </MenuItem>
          <MenuItem id="lists-off" icon="list" disabled about={w.addToListAbout}>
            {w.addToList}
          </MenuItem>
        </MenuGroup>
        <MenuGroup>
          <MenuItem id="delete" icon="delete">
            {w.delete}
          </MenuItem>
        </MenuGroup>
      </>
    );
  return (
    <div style={{ display: "flex", justifyContent: "flex-end", padding: rem(16) }}>
      <MenuButton
        icon={kind === "values" ? "public" : "more"}
        label={kind === "values" ? w.visibility : w.more}
        sub={
          panel === "lists"
            ? {
                title: w.addToList,
                onBack: () => {
                  setPanel("top");
                },
              }
            : undefined
        }
        open={open}
        onOpenChange={(next) => {
          setOpen(next);
          if (!next && kind !== "submenu") setPanel("top");
        }}
      >
        {items}
      </MenuButton>
    </div>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Menu everywhere", tags: ["themes"], parameters: { layout: "fullscreen" } } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** A menu of actions, opened: one cannot be done now, and says why. */
export const OpenActions: Story = { render: () => <Live kind="actions" /> };
/** The values of one setting, opened: the chosen one checked; a choice closes it. */
export const OpenValues: Story = { render: () => <Live kind="values" /> };
/** A submenu, opened in its menu's place: back and its name head it; its toggles keep it open. */
export const OpenSubmenu: Story = { render: () => <Live kind="submenu" /> };

export const ItemKinds: Story = { render: () => <ItemKindsStory /> };
export const Submenu: Story = { render: () => <SubmenuStory /> };
export const AddToList: Story = { render: () => <AddToListStory /> };
export const PostOfAnother: Story = { render: () => <Inventory kind="post" /> };
export const OwnPost: Story = { render: () => <Inventory kind="own-post" /> };
export const BoostMenu: Story = { render: () => <Inventory kind="boost" /> };
export const ProfileMenu: Story = { render: () => <Inventory kind="profile" /> };
export const ConversationMenu: Story = { render: () => <Inventory kind="conversation" /> };
export const DraftMenu: Story = { render: () => <Inventory kind="draft" /> };
export const ComposerDrafts: Story = { render: () => <Inventory kind="later" /> };
export const NewPostMenu: Story = { render: () => <Inventory kind="new-post" /> };
export const NavMenuSheet: Story = { render: () => <Inventory kind="nav" /> };
export const ComposerAudience: Story = { render: () => <Inventory kind="audience" /> };
export const ArticleVisibility: Story = { render: () => <Inventory kind="article-visibility" /> };
export const PostLanguage: Story = { render: () => <Inventory kind="language" /> };
export const PollDuration: Story = { render: () => <Inventory kind="duration" /> };
