// SPDX-License-Identifier: AGPL-3.0-only

// The settings in the app's shell (#3198, ADR-0110 §5), each whole and edge to edge so that it reads at the phone,
// tablet and PC widths of the toolbar's viewport, in the reader's language (the toolbar's locale; ja, else en):
//
// - /settings is the menu, in the main column at every width, with what is around the reader beside it as on every
//   screen; the same with the three entries later domains add.
// - An entry opens its screen in the main column. Under `wide` its head is the entry's name under the way back to the
//   menu; from `wide` the menu stands in the column beside the main one, the entry marked, and the screen starts at
//   its first section. Display and posting: every control saves at once. Security: the status summary, the one
//   place and the one door of what is to do, over lists, whose rows open the screens under the entry.
// - A screen under an entry (one keyword filter, a form saved as a whole) keeps its head at every width, the way
//   back leading to its entry, which the menu still marks.

import { color } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { Button } from "./button";
import { NumberField, Select } from "./field";
import { CheckField, TextField } from "./form-page";
import type { IconName } from "./icons";
import { BackButton } from "./list";
import { SaveState } from "./save-state";
import {
  SettingRow,
  SettingsGroup,
  SettingsItem,
  SettingsList,
  SettingsListRow,
  SettingsMenu,
  SettingsScreen,
  SettingsSection,
  StatusSummary,
  SubmitBar,
} from "./settings";
import {
  Frame,
  FrameAction,
  FrameActions,
  FrameAside,
  FrameDock,
  FrameFooter,
  FrameHeading,
  FrameMain,
  FrameMark,
  FrameSection,
  FrameSide,
  NavItem,
  NavList,
  NavMenu,
  SectionLine,
  SectionList,
} from "./shell";
import { Switch } from "./switch";

type At = "menu" | "display" | "security" | "filter";

// ---- The words ------------------------------------------------------------------------------------------------------

const en = {
  settings: "Settings",
  signOut: "Sign out",
  unread: "unread",
  groups: {
    general: "General",
    people: "People and filters",
    access: "Sign-in and access",
    account: "Account",
  },
  entries: {
    profile: ["Profile", "Name, bio, images, badges, how people find you"],
    display: ["Display and posting", "Language, time zone, new posts, timelines"],
    notifications: ["Notifications", "What reaches you, and where"],
    emoji: ["Custom emoji", "Emoji you added to this server"],
    reminders: ["Event reminders", "When to hear about the events you join"],
    relationships: ["Relationships", "Blocked and muted accounts, muted servers, followed hashtags"],
    filters: ["Keyword filters", "Words hidden from your timelines"],
    security: ["Security", "Password, passkeys, two-factor authentication, signed-in browsers"],
    apps: ["Apps", "Apps you let into your account, apps you made"],
    invites: ["Invites", "Invite people to this server"],
    account: ["Account and data", "Export and import, auto-delete, move, moderation history, delete"],
    supporter: ["Supporter membership", "Your plan and your payments"],
  },
  display: {
    display: "Display",
    language: "Language",
    timeZone: "Time zone",
    cards: "Link previews",
    cardsAbout: "How large a link's card is in a post",
    large: "Large",
    small: "Small",
    posts: "New posts",
    postsAbout: "What a new post starts with. You can change each as you write.",
    audience: "Audience",
    public: "Public",
    followers: "Followers",
    quote: "Who may quote",
    anyone: "Anyone",
    nobody: "Nobody",
    postLanguage: "Language",
    same: "Same as display",
    alt: "Ask for a description of media",
    altAbout: "Before a post goes out with a picture or a video that has none",
    timelines: "Timelines",
    languages: "Languages",
    languagesAbout: "Posts in other languages stay out of the timelines",
    all: "All languages",
    arrive: "Show new posts as they arrive",
    arriveAbout: "Off, a button over the timeline counts them until you ask",
  },
  security: {
    twoFactorOff: "Two-factor authentication is off",
    twoFactorAbout: "Anyone who learns your password can sign in. A code from your phone stops them.",
    turnOn: "Turn on",
    signIn: "Sign-in",
    password: "Password",
    passwordAbout: "Changed 3 months ago",
    passkeys: "Passkeys",
    passkeysAbout: "Sign in with your device's lock in place of the password.",
    add: "Add a passkey",
    remove: "Remove",
    keys: [
      ["MacBook Touch ID", "Added Mar 3 · used today"],
      ["Pixel 9", "Added Jun 12 · used 5d ago"],
    ],
    linked: "Linked accounts",
    linkedAbout: "Sign in through an account you have elsewhere.",
    notLinked: "Not linked",
    link: "Link",
    unlink: "Unlink",
    browsers: "Signed-in browsers",
    signOut: "Sign out",
    sessions: [
      ["Firefox on macOS", "Tokyo · now · this browser"],
      ["Safari on iPhone", "Tokyo · 2d ago"],
      ["Chrome on Windows", "Osaka · 3w ago"],
    ],
  },
  filter: {
    title: "Edit filter",
    section: "Filter",
    name: "Name",
    nameValue: "Spoilers",
    words: "Words",
    addWord: "Add a word",
    remove: "Remove",
    list: ["spoiler", "season finale", "#TheLongNight"],
    whole: "Whole word",
    where: "Where it applies",
    places: ["Home and lists", "Notifications", "Public timelines", "Conversations", "Profiles"],
    what: "What it does",
    matching: "Posts that match",
    warn: "Hidden behind a warning",
    hide: "Hidden completely",
    ends: "Ends after",
    endsAbout: "Empty, it stays until you delete it",
    days: "days",
    save: "Save",
  },
  around: {
    announcements: "Announcements",
    all: "All",
    allAnnouncements: "All announcements",
    events: "Upcoming events",
    lines: [
      ["Scheduled maintenance on Sunday, 03:00–04:00 UTC", "2h ago"],
      ["Custom emoji requests are open", "1d ago"],
      ["Fediverse meetup in Tokyo", "Sat, Nov 21, 19:00"],
      ["Moderators' office hours", "Tue, Nov 24, 12:00"],
    ],
  },
};

const ja: typeof en = {
  settings: "設定",
  signOut: "ログアウト",
  unread: "件の未読",
  groups: {
    general: "全般",
    people: "人とフィルター",
    access: "ログインとアクセス",
    account: "アカウント",
  },
  entries: {
    profile: ["プロフィール", "名前、自己紹介、画像、バッジ、見つけてもらう方法"],
    display: ["表示と投稿", "言語、タイムゾーン、投稿の初期設定、タイムライン"],
    notifications: ["通知", "何を、どこで受け取るか"],
    emoji: ["カスタム絵文字", "このサーバーに追加した絵文字"],
    reminders: ["イベントのリマインダー", "参加するイベントをいつ知らせるか"],
    relationships: ["つながり", "ブロック、ミュート、ミュートしたサーバー、フォロー中のハッシュタグ"],
    filters: ["キーワードフィルター", "タイムラインに表示しない言葉"],
    security: ["セキュリティ", "パスワード、パスキー、二要素認証、ログイン中のブラウザ"],
    apps: ["アプリ", "連携を許可したアプリ、自分で登録したアプリ"],
    invites: ["招待", "このサーバーに人を招待する"],
    account: ["アカウントとデータ", "エクスポートとインポート、自動削除、引っ越し、モデレーションの履歴、削除"],
    supporter: ["サポーター", "プランと支払い"],
  },
  display: {
    display: "表示",
    language: "言語",
    timeZone: "タイムゾーン",
    cards: "リンクのプレビュー",
    cardsAbout: "投稿の中のリンクのカードの大きさ",
    large: "大きく",
    small: "小さく",
    posts: "新しい投稿",
    postsAbout: "新しい投稿の初期設定です。書くときに投稿ごとに変えられます。",
    audience: "公開範囲",
    public: "公開",
    followers: "フォロワー",
    quote: "引用できる人",
    anyone: "だれでも",
    nobody: "だれもできない",
    postLanguage: "言語",
    same: "表示と同じ",
    alt: "メディアの説明を確認する",
    altAbout: "説明のない画像や動画を付けた投稿を送る前に確認します",
    timelines: "タイムライン",
    languages: "言語",
    languagesAbout: "ほかの言語の投稿はタイムラインに表示しません",
    all: "すべての言語",
    arrive: "新しい投稿をすぐに表示する",
    arriveAbout: "オフのときは、タイムラインの上のボタンが件数を知らせます",
  },
  security: {
    twoFactorOff: "二要素認証はオフです",
    twoFactorAbout: "パスワードを知った人はだれでもログインできます。スマートフォンのコードがそれを防ぎます。",
    turnOn: "有効にする",
    signIn: "ログイン",
    password: "パスワード",
    passwordAbout: "3か月前に変更",
    passkeys: "パスキー",
    passkeysAbout: "パスワードの代わりに、端末のロックでログインします。",
    add: "パスキーを追加",
    remove: "削除",
    keys: [
      ["MacBook Touch ID", "3月3日に追加 · 今日使用"],
      ["Pixel 9", "6月12日に追加 · 5日前に使用"],
    ],
    linked: "連携アカウント",
    linkedAbout: "ほかのサービスのアカウントでログインします。",
    notLinked: "未連携",
    link: "連携",
    unlink: "解除",
    browsers: "ログイン中のブラウザ",
    signOut: "ログアウト",
    sessions: [
      ["macOS の Firefox", "東京 · 現在 · このブラウザ"],
      ["iPhone の Safari", "東京 · 2日前"],
      ["Windows の Chrome", "大阪 · 3週間前"],
    ],
  },
  filter: {
    title: "フィルターを編集",
    section: "フィルター",
    name: "名前",
    nameValue: "ネタバレ",
    words: "言葉",
    addWord: "言葉を追加",
    remove: "削除",
    list: ["ネタバレ", "最終回", "#TheLongNight"],
    whole: "単語全体",
    where: "適用する場所",
    places: ["ホームとリスト", "通知", "公開タイムライン", "会話", "プロフィール"],
    what: "動作",
    matching: "一致した投稿",
    warn: "警告の後ろに隠す",
    hide: "完全に隠す",
    ends: "終了までの日数",
    endsAbout: "空のままなら、削除するまで続きます",
    days: "日",
    save: "保存",
  },
  around: {
    announcements: "お知らせ",
    all: "すべて",
    allAnnouncements: "すべてのお知らせ",
    events: "今後のイベント",
    lines: [
      ["日曜日 03:00〜04:00 (UTC) に定期メンテナンスを行います", "2時間前"],
      ["カスタム絵文字のリクエストを受け付けています", "1日前"],
      ["東京でFediverseの集まり", "11月21日(土) 19:00"],
      ["モデレーターのオフィスアワー", "11月24日(火) 12:00"],
    ],
  },
};

type Words = typeof en;
const useWords = (): Words => (new Intl.Locale(useLocale().locale).language === "ja" ? ja : en);

// ---- The menu -------------------------------------------------------------------------------------------------------

type EntryId = keyof Words["entries"];
type Entry = { id: EntryId; icon: IconName; count?: number; later?: true };

const groups: { id: keyof Words["groups"]; entries: Entry[] }[] = [
  {
    id: "general",
    entries: [
      { id: "profile", icon: "profile" },
      { id: "display", icon: "display" },
      { id: "notifications", icon: "notifications" },
      { id: "emoji", icon: "emojiCustom", later: true },
      { id: "reminders", icon: "eventReminder", later: true },
    ],
  },
  {
    id: "people",
    entries: [
      { id: "relationships", icon: "members" },
      { id: "filters", icon: "filter" },
    ],
  },
  {
    id: "access",
    entries: [
      { id: "security", icon: "security" },
      { id: "apps", icon: "apps" },
      { id: "invites", icon: "invite" },
    ],
  },
  {
    id: "account",
    entries: [
      { id: "account", icon: "accountData", count: 1 },
      { id: "supporter", icon: "funding", later: true },
    ],
  },
];

/** The menu: its groups, then signing out, apart. `grown`, with the entries later domains add. */
function Menu({ compact = false, current, grown = false }: { compact?: boolean; current?: EntryId; grown?: boolean }) {
  const w = useWords();
  return (
    <SettingsMenu
      label={w.settings}
      compact={compact}
      end={<SettingsItem icon="signOut" label={w.signOut} count={undefined} href="/auth/sign_out" away />}
    >
      {groups.map((g) => (
        <SettingsGroup key={g.id} heading={w.groups[g.id]}>
          {g.entries
            .filter((e) => grown || !e.later)
            .map((e) => (
              <SettingsItem
                key={e.id}
                icon={e.icon}
                label={w.entries[e.id][0] ?? ""}
                hint={w.entries[e.id][1]}
                count={e.count}
                countLabel={e.count === undefined ? undefined : `${String(e.count)} ${w.unread}`}
                href={`/settings/${e.id}`}
                current={e.id === current}
              />
            ))}
        </SettingsGroup>
      ))}
    </SettingsMenu>
  );
}

// ---- The screens ----------------------------------------------------------------------------------------------------

const languages = [
  "English",
  "Español",
  "Français",
  "Deutsch",
  "Português (Brasil)",
  "日本語",
  "简体中文",
  "繁體中文",
  "한국어",
  "العربية",
].map((label) => ({ value: label, label }));

const zones = [
  {
    label: "Asia",
    options: ["Tokyo (UTC+9)", "Seoul (UTC+9)", "Shanghai (UTC+8)", "Kolkata (UTC+5:30)"].map((label) => ({
      value: label,
      label,
    })),
  },
  {
    label: "Europe",
    options: ["Lisbon (UTC+0)", "Berlin (UTC+1)", "Helsinki (UTC+2)"].map((label) => ({ value: label, label })),
  },
  {
    label: "America",
    options: ["São Paulo (UTC−3)", "New York (UTC−5)", "Los Angeles (UTC−8)"].map((label) => ({
      value: label,
      label,
    })),
  },
];

const pick = (...labels: string[]) => labels.map((label) => ({ value: label, label }));

function Display({ back }: { back: ReactNode }) {
  const w = useWords();
  const d = w.display;
  return (
    <SettingsScreen icon="display" title={w.entries.display[0] ?? ""} back={back}>
      <SettingsSection heading={d.display}>
        <SettingRow id="display-language" label={d.language} save={null}>
          <Select labelledBy="display-language" options={languages} defaultValue="English" />
        </SettingRow>
        <SettingRow id="display-zone" label={d.timeZone} save={<SaveState state="saved" />}>
          <Select labelledBy="display-zone" options={zones} defaultValue="Tokyo (UTC+9)" />
        </SettingRow>
        <SettingRow id="display-cards" label={d.cards} about={d.cardsAbout} save={null}>
          <Select labelledBy="display-cards" options={pick(d.large, d.small)} />
        </SettingRow>
      </SettingsSection>
      <SettingsSection heading={d.posts} about={d.postsAbout}>
        <SettingRow id="post-audience" label={d.audience} save={null}>
          <Select labelledBy="post-audience" options={pick(d.public, d.followers)} />
        </SettingRow>
        <SettingRow id="post-quote" label={d.quote} save={null}>
          <Select labelledBy="post-quote" options={pick(d.anyone, d.followers, d.nobody)} />
        </SettingRow>
        <SettingRow id="post-language" label={d.postLanguage} save={null}>
          <Select labelledBy="post-language" options={[...pick(d.same), ...languages]} />
        </SettingRow>
        <Switch label={d.alt} description={d.altAbout} defaultSelected save={null} />
      </SettingsSection>
      <SettingsSection heading={d.timelines}>
        <SettingRow id="timeline-languages" label={d.languages} about={d.languagesAbout} save={null}>
          <Select labelledBy="timeline-languages" options={[...pick(d.all), ...languages]} />
        </SettingRow>
        <Switch label={d.arrive} description={d.arriveAbout} save={null} />
      </SettingsSection>
    </SettingsScreen>
  );
}

function Security({ back }: { back: ReactNode }) {
  const w = useWords();
  const s = w.security;
  return (
    <SettingsScreen icon="security" title={w.entries.security[0] ?? ""} back={back}>
      {/* While it is off, the summary is the one place that says so and the one door to it: no row repeats it. */}
      <StatusSummary
        icon="caution"
        title={s.twoFactorOff}
        about={s.twoFactorAbout}
        action={<Button kind="primary">{s.turnOn}</Button>}
      />
      <SettingsSection heading={s.signIn}>
        <SettingsList label={s.signIn}>
          <SettingsListRow
            icon="security"
            title={s.password}
            caption={s.passwordAbout}
            href="/settings/security/password"
          />
        </SettingsList>
      </SettingsSection>
      <SettingsSection
        heading={s.passkeys}
        count={String(s.keys.length)}
        about={s.passkeysAbout}
        action={<Button size="timeline">{s.add}</Button>}
      >
        <SettingsList label={s.passkeys}>
          {s.keys.map(([title = "", caption]) => (
            <SettingsListRow
              key={title}
              icon="passkey"
              title={title}
              caption={caption}
              end={<Button size="timeline">{s.remove}</Button>}
            />
          ))}
        </SettingsList>
      </SettingsSection>
      <SettingsSection heading={s.linked} about={s.linkedAbout}>
        <SettingsList label={s.linked}>
          <SettingsListRow
            icon="github"
            title="GitHub"
            caption="yuki-snow"
            end={<Button size="timeline">{s.unlink}</Button>}
          />
          <SettingsListRow
            icon="discord"
            title="Discord"
            caption={s.notLinked}
            end={<Button size="timeline">{s.link}</Button>}
          />
        </SettingsList>
      </SettingsSection>
      <SettingsSection heading={s.browsers} count={String(s.sessions.length)}>
        <SettingsList label={s.browsers}>
          {s.sessions.map(([title = "", caption], i) => (
            <SettingsListRow
              key={title}
              icon="public"
              title={title}
              caption={caption}
              end={i === 0 ? undefined : <Button size="timeline">{s.signOut}</Button>}
            />
          ))}
        </SettingsList>
      </SettingsSection>
    </SettingsScreen>
  );
}

/** A screen under an entry: one keyword filter, a form saved as a whole, its submit bar in reach. */
function Filter({ back }: { back: ReactNode }) {
  const w = useWords();
  const f = w.filter;
  return (
    <SettingsScreen
      sub
      title={f.title}
      back={back}
      foot={
        <SubmitBar>
          <Button kind="primary">{f.save}</Button>
        </SubmitBar>
      }
    >
      <SettingsSection heading={f.section}>
        <TextField id="filter-name" name="name" label={f.name} defaultValue={f.nameValue} />
      </SettingsSection>
      <SettingsSection
        heading={f.words}
        count={String(f.list.length)}
        action={<Button size="timeline">{f.addWord}</Button>}
      >
        <SettingsList label={f.words}>
          {f.list.map((word, i) => (
            <SettingsListRow
              key={word}
              title={word}
              caption={i === 0 ? f.whole : undefined}
              end={<Button size="timeline">{f.remove}</Button>}
            />
          ))}
        </SettingsList>
      </SettingsSection>
      <SettingsSection heading={f.where}>
        {f.places.map((place, i) => (
          <CheckField
            key={place}
            id={`filter-place-${String(i)}`}
            name="context"
            value={String(i)}
            label={place}
            defaultChecked={i < 3}
          />
        ))}
      </SettingsSection>
      <SettingsSection heading={f.what}>
        <SettingRow id="filter-action" label={f.matching} form>
          <Select labelledBy="filter-action" name="action" options={pick(f.warn, f.hide)} />
        </SettingRow>
        <SettingRow id="filter-ends" label={f.ends} about={f.endsAbout} form>
          <NumberField labelledBy="filter-ends" name="days" defaultValue={30} min={1} unit={f.days} />
        </SettingRow>
      </SettingsSection>
    </SettingsScreen>
  );
}

// ---- The app's shell around them ------------------------------------------------------------------------------------

// A stand-in for the brand's wordmark, in the text's color of a scheme.
const wordmark = (fill: string) =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 44 24" width="44" height="24"><text x="2" y="19" font-family="system-ui, sans-serif" font-size="21" font-style="italic" font-weight="800" fill="${fill}">my</text></svg>`)}`;

type Place = { href: string; icon: IconName; label: string; count?: number };

const tabs: Place[] = [
  { href: "/", icon: "home", label: "Home" },
  { href: "/local", icon: "local", label: "Local" },
  { href: "/public", icon: "federated", label: "Federated" },
  { href: "/notifications", icon: "notifications", label: "Notifications", count: 3 },
  { href: "/search", icon: "search", label: "Search" },
];
const direct: Place = { href: "/direct", icon: "directMessages", label: "Direct messages", count: 2 };
const rest: Place[] = [
  { href: "/@yuki", icon: "profile", label: "Profile" },
  { href: "/bookmarks", icon: "bookmark", label: "Bookmarks" },
  { href: "/settings", icon: "settings", label: "Settings" },
];
const side = [...tabs, ...rest].flatMap((p) => (p.href === "/notifications" ? [p, direct] : [p]));
const server: Place[] = [
  { href: "/announcements", icon: "announcements", label: "Announcements" },
  { href: "/voice", icon: "room", label: "Voice rooms" },
  { href: "/events", icon: "events", label: "Events" },
];

const places = (items: Place[]) => (
  <NavList>
    {items.map((p) => (
      <NavItem
        key={p.href}
        href={p.href}
        icon={p.icon}
        label={p.label}
        current={p.href === "/settings"}
        count={p.count}
        countLabel={p.count === undefined ? undefined : `${String(p.count)} unread`}
      />
    ))}
  </NavList>
);

const footer = (
  <FrameFooter
    label="This server"
    links={[
      { href: "/about", label: "About", icon: "about" },
      { href: "/terms", label: "Terms", icon: "file" },
      { href: "/privacy", label: "Privacy", icon: "file" },
    ]}
  />
);

/** What is around the reader, as on every screen: beside the menu at /settings. */
function Around() {
  const a = useWords().around;
  const line = (i: number, unread = false) => (
    <SectionLine
      href={`/line/${String(i)}`}
      title={a.lines[i]?.[0] ?? ""}
      caption={a.lines[i]?.[1]}
      unread={unread ? "Unread" : undefined}
    />
  );
  return (
    <FrameAside label="Server activity">
      <FrameSection
        id="around-announcements"
        icon="announcements"
        href="/announcements"
        heading={a.announcements}
        all={{ text: a.all, name: a.allAnnouncements }}
      >
        <SectionList>
          {line(0, true)}
          {line(1, true)}
        </SectionList>
      </FrameSection>
      <FrameSection id="around-events" icon="events" href="/events" heading={a.events}>
        <SectionList>
          {line(2)}
          {line(3)}
        </SectionList>
      </FrameSection>
      {footer}
    </FrameAside>
  );
}

/** The entry a screen belongs to, which the menu beside it marks, and the screen itself. */
const screens = {
  display: { entry: "display", Screen: Display },
  security: { entry: "security", Screen: Security },
  filter: { entry: "filters", Screen: Filter },
} as const;

/** The settings in the shell: the menu at /settings, or a screen with the menu beside it from `wide`. */
function Settings({ at, grown = false }: { at: At; grown?: boolean }) {
  const w = useWords();
  const open = at === "menu" ? undefined : screens[at];
  // The way back: to the menu from an entry's screen, to its entry from a screen under one.
  const back = (
    <BackButton onPress={() => undefined}>{at === "filter" ? (w.entries.filters[0] ?? "") : w.settings}</BackButton>
  );
  return (
    <Frame>
      <FrameSide label="Main">
        <FrameMark href="/" label="my.example" light={wordmark(color.text.light)} dark={wordmark(color.text.dark)} />
        <FrameActions>
          <FrameAction icon="compose" label="New post" onPress={() => undefined} />
          <FrameAction icon="article" label="New article" secondary onPress={() => undefined} />
        </FrameActions>
        {places(side)}
        <NavList>
          <NavMenu label="Menu" footer={footer}>
            {places(server)}
          </NavMenu>
        </NavList>
      </FrameSide>
      <FrameMain>
        <FrameHeading>
          {at === "menu" ? w.settings : at === "filter" ? w.filter.title : (w.entries[screens[at].entry][0] ?? "")}
        </FrameHeading>
        {open === undefined ? <Menu grown={grown} /> : <open.Screen back={back} />}
      </FrameMain>
      {open === undefined ? (
        <Around />
      ) : (
        <FrameAside label={w.settings}>
          <Menu compact current={open.entry} />
          {footer}
        </FrameAside>
      )}
      <FrameDock
        label="Main"
        tabs={
          <NavList>
            {tabs.map((p) => (
              <NavItem
                key={p.href}
                href={p.href}
                icon={p.icon}
                label={p.label}
                count={p.count}
                countLabel={p.count === undefined ? undefined : `${String(p.count)} unread`}
              />
            ))}
            <NavMenu label="Menu" count={direct.count} markLabel="2 unread in direct messages" footer={footer}>
              {places([direct, ...rest])}
              {places(server)}
            </NavMenu>
          </NavList>
        }
      />
    </Frame>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = {
  title: "Settings",
  tags: ["themes"],
  parameters: { layout: "fullscreen" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** /settings: the menu, in the main column at every width. */
export const AtMenu: Story = { render: () => <Settings at="menu" /> };
/** The menu with the entries later domains add: custom emoji, event reminders, the supporter membership. */
export const AtMenuGrown: Story = { render: () => <Settings at="menu" grown /> };
/** /settings/display: every control saves at once. */
export const AtDisplay: Story = { render: () => <Settings at="display" /> };
/** /settings/security: the status summary over lists, whose rows open the screens under the entry. */
export const AtSecurity: Story = { render: () => <Settings at="security" /> };
/** /settings/filters/3: a screen under an entry, a form saved as a whole. */
export const AtFilter: Story = { render: () => <Settings at="filter" /> };
