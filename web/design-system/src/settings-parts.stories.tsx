// SPDX-License-Identifier: AGPL-3.0-only

// The parts of a settings screen (ADR-0110 §5), each in its states, in a column of the main column's width, in the
// reader's language (the toolbar's locale; ja, else en): the save state of a control that saves at once, in a
// setting's row and in a switch; the status summary, with something to do and with all well; a list with its one add
// action, and empty, where what stands in its place leads to adding; the submit bar of a form saved as a whole, at
// rest, sending, saved and refused, and with what saving does beside its button; rows that say what came of their own
// action, and facts apart under a name of their own; a choice from a long list and a number; the confirmation that
// asks the reader to authenticate again, by each proof, with no way to prove it, and in flight; a setting that cannot
// be changed now, with its reason; a number field given a number outside what it takes; what the reader copies and
// keeps (recovery codes shown once, before and after copying; a secret of a server's length, which wraps in its box);
// a text field among the rows of a section, its name in the rows' own look; rows of records marked by where they stand;
// the key of an authenticator app as a QR code and as text; rows that say what is known of each thing, one fact a
// line, under what came of a change in their section and over a sentence of the section with the links to where the
// reader acts on it; and the facts a section states, which are not pressed.

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { Button, TextButton } from "./button";
import { ReauthDialog } from "./dialog";
import { NumberField, Select } from "./field";
import { FormAlert, TextField } from "./form-page";
import { Stack } from "./layout";
import { StateMark } from "./records";
import { SaveState } from "./save-state";
import {
  CopyBlock,
  QrFrame,
  SettingRow,
  SettingsEmpty,
  SettingsFacts,
  SettingsList,
  SettingsListRow,
  SettingsNote,
  SettingsResult,
  SettingsSection,
  StatusSummary,
  SubmitBar,
} from "./settings";
import { Switch } from "./switch";

const en = {
  section: "Display",
  zone: "Time zone",
  cards: "Link previews",
  cardsAbout: "How large a link's card is in a post",
  large: "Large",
  small: "Small",
  arrive: "Show new posts as they arrive",
  arriveAbout: "Off, a button over the timeline counts them until you ask",
  alt: "Ask for a description of media",
  twoFactor: "Two-factor authentication",
  twoFactorOff: "Two-factor authentication is off",
  twoFactorOn: "Two-factor authentication is on",
  offAbout: "Anyone who learns your password can sign in. A code from your phone stops them.",
  onAbout: "Signing in asks for a code from your phone after the password.",
  turnOn: "Turn on",
  signIn: "Sign-in",
  passwordAbout: "Changed 3 months ago",
  twoFactorSince: "On since Nov 6",
  twoFactorCodes: "2 recovery codes left",
  passkeys: "Passkeys",
  passkeysAbout: "Sign in with your device's lock in place of the password.",
  add: "Add a passkey",
  remove: "Remove",
  keys: [
    ["MacBook Touch ID", "Added Mar 3 · used today"],
    ["A passkey with a name long enough to wrap on a phone", "Added Jun 12 · used 5d ago"],
  ],
  none: "No passkeys yet. Passkeys you add appear here.",
  languages: "Languages to show",
  languagesAbout: "Posts in other languages are left out of every timeline.",
  addLanguage: "Add a language",
  filters: "Filters",
  addFilter: "Add a filter",
  filterRows: [
    ["Spoilers", "3 words · home, notifications, public timelines"],
    ["Election", "12 words · everywhere · ends in 30 days"],
  ],
  save: "Save",
  refused: "The name is already used by another filter.",
  language: "Language",
  ends: "Ends after",
  endsAbout: "Empty, it stays until you delete it",
  range: "From 1 to 365 days",
  zoneOff: "This server sets one time zone for everyone.",
  endsOff: "Only a filter that warns can end.",
  arriveOff: "This server has turned live updates off.",
  days: "days",
  threshold: "Hide posts with more than",
  mentions: "mentions",
  confirmTitle: "Confirm it's you",
  confirmAbout: "To change your password, enter the one you use now.",
  password: "Password",
  code: "Code",
  codeAbout: "To see your recovery codes, enter the 6-digit code from your authenticator app.",
  passkeyAbout: "To delete your account, confirm with your passkey.",
  providerAbout: "To delete your account, sign in to GitHub again. You come back here afterwards.",
  close: "Close",
  cancel: "Cancel",
  confirm: "Confirm",
  usePasskey: "Use a passkey",
  useProvider: "Continue with GitHub",
  usePassword: "Use your password instead",
  usePasskeyInstead: "Use a passkey instead",
  useProviderInstead: "Continue with GitHub instead",
  actTitle: "Delete this account?",
  actEffect: "The account and its posts are deleted at once, and you are signed out. This cannot be undone.",
  actAbout: "Enter your current password to confirm it's you.",
  act: "Delete account",
  wrong: "That password is not right.",
  noWay: "This browser has no way to confirm it's you. Try a browser or device that can use one of your passkeys.",
  signedOut: "Chrome on Windows is signed out.",
  recovery: "Recovery codes",
  recoveryAbout:
    "Each code signs you in once when you cannot use your authenticator app. Keep them somewhere safe: they are not shown again.",
  copy: "Copy",
  copied: "Copied",
  setup: "Add to your authenticator app",
  setupAbout: "Scan the QR code with your authenticator app, or enter the key by hand.",
  qr: "QR code of the setup key",
  key: "Setup key",
  enter: "Enter the code",
  codeHint: "The six-digit code your authenticator app shows now",
  browsers: "Signed-in browsers",
  thisBrowser: "This browser",
  signedIn: "Signed in Nov 15",
  from: "Signed in from 203.0.113.24",
  lastUsed: "Last used Nov 18, 2 PM",
  signOut: "Sign out",
  advice:
    "If you do not recognize a sign-in, sign that browser out, change your password, and check which apps can use your account.",
  changePassword: "Change password",
  apps: "Check your apps",
  secret: "Client secret",
  secretAbout: "Copy it now and keep it somewhere safe: it is not shown again.",
  copySecret: "Copy secret",
  attention: "Permissions that need your attention",
  reads: "Permissions that only read",
  granted: [
    ["Read your account, posts, and timelines", ["Can read your direct messages"]],
    ["Post and change things for you", ["Can post, edit and delete posts as you", "Can change your profile"]],
  ] as [string, string[]][],
  read: ["Read your bookmarks", "Read your keyword filters", "Read the accounts and domains you mute"],
  overRange: "This server no longer allows that many days. Enter a smaller number and save again.",
  overZone: "This server no longer offers that time zone. Choose another and save again.",
  invites: "Your invites",
  invitesAbout: "A person signs up with an invite's link, or enters its code on the sign-up page.",
  invitesFull: "You have as many usable invites as allowed (2). Revoke one, or wait until one expires or is used up.",
  inviteRows: [
    ["k17wdrqpmmmm30em", true, ["Usable until Nov 25, 2 PM", "Used 0 of 1 time"]],
    ["k2fsrjhemmmm3vnp", true, ["Usable until Nov 23, 4 PM", "Used 1 of 5 times"]],
    ["k3np5b83mmmm4qws", false, ["Revoked", "Used 0 of 3 times"]],
    ["k4vlj2ysmmmm5m5v", false, ["Expired Nov 15, 4 PM", "Used 1 of 1 time"]],
  ] as [string, boolean, string[]][],
  deletes: "Saving deletes 273 of your posts.",
  files: "Files",
  download: "Download",
  fileRows: ["Follows", "Bookmarks", "Muted servers"],
  started: "The download of following_accounts.csv started.",
  failed: "The file could not be downloaded. Try again.",
  preview: "Preview",
  facts: ["7 new accounts to follow", "2 accounts you already follow"],
  removes: "What “Replace” removes",
  removed: "5 accounts to unfollow, which the file does not list",
};

const ja: typeof en = {
  section: "表示",
  zone: "タイムゾーン",
  cards: "リンクのプレビュー",
  cardsAbout: "投稿の中のリンクのカードの大きさ",
  large: "大きく",
  small: "小さく",
  arrive: "新しい投稿をすぐに表示する",
  arriveAbout: "オフのときは、タイムラインの上のボタンが件数を知らせます",
  alt: "メディアの説明を確認する",
  twoFactor: "二要素認証",
  twoFactorOff: "二要素認証はオフです",
  twoFactorOn: "二要素認証はオンです",
  offAbout: "パスワードを知った人はだれでもログインできます。スマートフォンのコードがそれを防ぎます。",
  onAbout: "ログインのとき、パスワードのあとにスマートフォンのコードを入力します。",
  turnOn: "オンにする",
  signIn: "ログイン",
  passwordAbout: "3か月前に変更",
  twoFactorSince: "11月6日 からオン",
  twoFactorCodes: "リカバリーコードは残り 2 個",
  passkeys: "パスキー",
  passkeysAbout: "パスワードの代わりに、端末のロックでログインします。",
  add: "パスキーを追加",
  remove: "削除",
  keys: [
    ["MacBook Touch ID", "3月3日に追加 · 今日使用"],
    ["スマートフォンで折り返すほど長い名前を付けたパスキー", "6月12日に追加 · 5日前に使用"],
  ],
  none: "パスキーはまだありません。追加したパスキーがここに表示されます。",
  languages: "表示する言語",
  languagesAbout: "ほかの言語の投稿は、どのタイムラインにも表示しません。",
  addLanguage: "言語を追加",
  filters: "フィルター",
  addFilter: "フィルターを追加",
  filterRows: [
    ["ネタバレ", "3語 · ホーム、通知、公開タイムライン"],
    ["選挙", "12語 · すべての場所 · 30日後に終了"],
  ],
  save: "保存",
  refused: "その名前はほかのフィルターで使われています。",
  language: "言語",
  ends: "終了までの日数",
  endsAbout: "空のままなら、削除するまで続きます",
  range: "1〜365日",
  zoneOff: "このサーバーでは、全員が同じタイムゾーンを使います。",
  endsOff: "終了を設定できるのは、警告を表示するフィルターだけです。",
  arriveOff: "このサーバーではリアルタイムの更新がオフになっています。",
  days: "日",
  threshold: "これより多いメンションの投稿を隠す",
  mentions: "件",
  confirmTitle: "本人確認",
  confirmAbout: "パスワードを変更するには、現在のパスワードを入力してください。",
  password: "パスワード",
  code: "コード",
  codeAbout: "リカバリーコードを表示するには、認証アプリの6桁のコードを入力してください。",
  passkeyAbout: "アカウントを削除するには、パスキーで確認してください。",
  providerAbout: "アカウントを削除するには、GitHubにもう一度ログインしてください。そのあとここに戻ります。",
  close: "閉じる",
  cancel: "キャンセル",
  confirm: "確認",
  usePasskey: "パスキーを使う",
  useProvider: "GitHubで続ける",
  usePassword: "代わりにパスワードを使う",
  usePasskeyInstead: "代わりにパスキーを使う",
  useProviderInstead: "代わりにGitHubで続ける",
  actTitle: "このアカウントを削除しますか？",
  actEffect: "アカウントと投稿はすぐに削除され、ログアウトします。もとに戻せません。",
  actAbout: "本人確認のため、現在のパスワードを入力してください。",
  act: "アカウントを削除",
  wrong: "パスワードが正しくありません。",
  noWay: "このブラウザには本人確認の手段がありません。パスキーを使えるブラウザか端末で試してください。",
  signedOut: "Windows の Chrome をログアウトしました。",
  recovery: "リカバリーコード",
  recoveryAbout:
    "認証アプリを使えないとき、それぞれのコードで1回ずつログインできます。安全な場所に保管してください。このコードは二度と表示されません。",
  copy: "コピー",
  copied: "コピーしました",
  setup: "認証アプリに追加",
  setupAbout: "認証アプリでQRコードを読み取るか、キーを手で入力してください。",
  qr: "セットアップキーのQRコード",
  key: "セットアップキー",
  enter: "コードを入力",
  codeHint: "認証アプリにいま表示されている6桁のコード",
  browsers: "ログイン中のブラウザ",
  thisBrowser: "このブラウザ",
  signedIn: "ログイン日: 11月15日",
  from: "ログイン元: 203.0.113.24",
  lastUsed: "最終利用: 11月18日 14時",
  signOut: "ログアウト",
  advice:
    "心当たりのないログインがあるときは、そのブラウザをログアウトし、パスワードを変更して、アカウントを使えるアプリを確認してください。",
  changePassword: "パスワードを変更",
  apps: "アプリを確認",
  secret: "クライアントシークレット",
  secretAbout: "いまコピーして、安全な場所に保管してください。二度と表示されません。",
  copySecret: "シークレットをコピー",
  attention: "特に注意が必要な権限",
  reads: "読み取りだけの権限",
  granted: [
    ["アカウント・投稿・タイムラインの読み取り", ["ダイレクトメッセージを読めます"]],
    ["あなたに代わっての投稿と変更", ["あなたとして投稿・編集・削除ができます", "プロフィールを変更できます"]],
  ],
  read: ["ブックマークの読み取り", "キーワードフィルタの読み取り", "ミュートしているアカウントとドメインの読み取り"],
  overRange: "このサーバーでは、その日数を使えなくなりました。小さい数を入力して、もう一度保存してください。",
  overZone: "このサーバーでは、そのタイムゾーンを使えなくなりました。ほかを選んで、もう一度保存してください。",
  invites: "作成した招待",
  invitesAbout: "招待された人は、招待のリンクを開いて登録するか、登録画面で招待コードを入力します。",
  invitesFull:
    "使える招待の数の上限 (2 件) に達しています。どれかを取り消すか、期限切れか使い切りになるまで待ってください。",
  inviteRows: [
    ["k17wdrqpmmmm30em", true, ["11月25日 14時 まで使える", "1 回のうち 0 回使用"]],
    ["k2fsrjhemmmm3vnp", true, ["11月23日 16時 まで使える", "5 回のうち 1 回使用"]],
    ["k3np5b83mmmm4qws", false, ["取り消し済み", "3 回のうち 0 回使用"]],
    ["k4vlj2ysmmmm5m5v", false, ["11月15日 16時 に期限切れ", "1 回のうち 1 回使用"]],
  ],
  deletes: "保存すると 273 件の投稿が削除されます。",
  files: "ファイル",
  download: "ダウンロード",
  fileRows: ["フォロー", "ブックマーク", "ミュートしたサーバー"],
  started: "following_accounts.csv のダウンロードを開始しました。",
  failed: "ファイルをダウンロードできませんでした。もう一度試してください。",
  preview: "プレビュー",
  facts: ["7 件のアカウントを新しくフォローします", "2 件のアカウントはすでにフォローしています"],
  removes: "「置き換え」で削除されるもの",
  removed: "ファイルにない 5 件のアカウントのフォローを解除します",
};

const useWords = () => (new Intl.Locale(useLocale().locale).language === "ja" ? ja : en);

/** A column of the main column's width, its sections apart as a settings screen's. */
function Column({ children }: { children: ReactNode }) {
  return <div className="grid max-w-[37.5rem] gap-shell-edge p-shell-edge text-body">{children}</div>;
}

const zones = ["Tokyo (UTC+9)", "Lisbon (UTC+0)", "Los Angeles (UTC−8)"].map((label) => ({ value: label, label }));

function SaveStates() {
  const w = useWords();
  // The words of each state are the design system's own.
  const states = [
    null,
    <SaveState key="saving" state="saving" />,
    <SaveState key="saved" state="saved" />,
    <SaveState key="failed" state="failed" onRetry={() => undefined} />,
  ];
  return (
    <Column>
      <SettingsSection heading={w.zone}>
        {states.map((save, i) => (
          <SettingRow key={i} id={`zone-${String(i)}`} label={w.zone} save={save}>
            <Select labelledBy={`zone-${String(i)}`} options={zones} />
          </SettingRow>
        ))}
      </SettingsSection>
      <SettingsSection heading={w.cards}>
        {states.map((save, i) => (
          <SettingRow key={i} id={`cards-${String(i)}`} label={w.cards} about={w.cardsAbout} save={save}>
            <Select
              labelledBy={`cards-${String(i)}`}
              options={[w.large, w.small].map((label) => ({ value: label, label }))}
            />
          </SettingRow>
        ))}
      </SettingsSection>
      <SettingsSection heading={w.arrive}>
        {states.map((save, i) => (
          <Switch key={i} label={w.arrive} description={w.arriveAbout} save={save} defaultSelected={i > 0} />
        ))}
      </SettingsSection>
      <SettingsSection heading={w.alt}>
        {states.map((save, i) => (
          <Switch key={i} label={w.alt} save={save} defaultSelected={i > 0} />
        ))}
      </SettingsSection>
    </Column>
  );
}

function Summaries() {
  const w = useWords();
  return (
    <Column>
      {/* Something to do: the summary says it once and is the one door to it. */}
      <StatusSummary
        icon="caution"
        title={w.twoFactorOff}
        about={w.offAbout}
        action={<Button kind="primary">{w.turnOn}</Button>}
      />
      {/* All well, where a screen says so. */}
      <StatusSummary icon="accepted" title={w.twoFactorOn} about={w.onAbout} />
      {/* Once it is on, its row is its door, its state said once, in the muted line under its name; the summary is gone. */}
      <SettingsSection heading={w.signIn}>
        <SettingsList label={w.signIn}>
          <SettingsListRow
            icon="security"
            title={w.password}
            caption={w.passwordAbout}
            href="/settings/security/password"
          />
          <SettingsListRow
            icon="twoFactor"
            title={w.twoFactor}
            facts={[w.twoFactorSince, w.twoFactorCodes]}
            href="/settings/security/two-factor"
          />
        </SettingsList>
      </SettingsSection>
    </Column>
  );
}

function Lists() {
  const w = useWords();
  return (
    <Column>
      {/* A list among a screen's sections: its add action, neutral, at the end of the heading's line. */}
      <SettingsSection
        heading={w.passkeys}
        count={String(w.keys.length)}
        about={w.passkeysAbout}
        action={<Button size="timeline">{w.add}</Button>}
      >
        <SettingsList label={w.passkeys}>
          {w.keys.map(([title = "", caption]) => (
            <SettingsListRow
              key={title}
              icon="passkey"
              title={title}
              caption={caption}
              end={<Button size="timeline">{w.remove}</Button>}
            />
          ))}
        </SettingsList>
      </SettingsSection>
      {/* Empty: the sentence and the add action in the rows' place; the head leaves the action out. */}
      <SettingsSection heading={w.passkeys} about={w.passkeysAbout}>
        <SettingsEmpty action={<Button size="timeline">{w.add}</Button>}>{w.none}</SettingsEmpty>
      </SettingsSection>
      {/* The values of a setting, which its control adds to: close under its row, as its own, then the next setting. */}
      <SettingsSection heading={w.arrive}>
        <SettingRow
          id="values"
          label={w.languages}
          about={w.languagesAbout}
          save={null}
          values={
            <SettingsList label={w.languages}>
              {["English", "日本語"].map((title) => (
                <SettingsListRow key={title} title={title} end={<Button size="timeline">{w.remove}</Button>} />
              ))}
            </SettingsList>
          }
        >
          <Select options={[{ value: "", label: w.addLanguage }]} />
        </SettingRow>
        <Switch label={w.arrive} description={w.arriveAbout} save={null} />
      </SettingsSection>
      {/* The one list of a screen: adding is the screen's primary action; a row opens the screen under the entry. */}
      <SettingsSection
        heading={w.filters}
        count={String(w.filterRows.length)}
        action={
          <Button kind="primary" size="timeline">
            {w.addFilter}
          </Button>
        }
      >
        <SettingsList label={w.filters}>
          {w.filterRows.map(([title = "", caption], i) => (
            <SettingsListRow key={title} title={title} caption={caption} href={`/settings/filters/${String(i)}`} />
          ))}
        </SettingsList>
      </SettingsSection>
    </Column>
  );
}

function SubmitBars() {
  const w = useWords();
  const field = (i: number) => (
    <SettingsSection heading={w.filters}>
      <TextField id={`submit-name-${String(i)}`} name="name" label={w.filters} defaultValue={w.filterRows[0]?.[0]} />
    </SettingsSection>
  );
  return (
    <>
      <Column>
        {field(0)}
        <SubmitBar>
          <Button kind="primary">{w.save}</Button>
        </SubmitBar>
      </Column>
      <Column>
        {field(1)}
        <SubmitBar>
          <Button kind="primary" busy>
            {w.save}
          </Button>
        </SubmitBar>
      </Column>
      <Column>
        {field(2)}
        <SubmitBar result={<SaveState state="saved" />}>
          <Button kind="primary">{w.save}</Button>
        </SubmitBar>
      </Column>
      <Column>
        {field(3)}
        <SubmitBar result={<FormAlert>{w.refused}</FormAlert>}>
          <Button kind="primary">{w.save}</Button>
        </SubmitBar>
      </Column>
      {/* What saving does, where it cannot be undone: beside the button, which it describes; and with a result. */}
      <Column>
        {field(4)}
        <SubmitBar about={{ id: "submit-about-4", text: w.deletes }}>
          <Button kind="primary" aria-describedby="submit-about-4">
            {w.save}
          </Button>
        </SubmitBar>
      </Column>
      <Column>
        {field(5)}
        <SubmitBar result={<SaveState state="saved" />} about={{ id: "submit-about-5", text: w.deletes }}>
          <Button kind="primary" aria-describedby="submit-about-5">
            {w.save}
          </Button>
        </SubmitBar>
      </Column>
      <Column>
        {field(6)}
        <SubmitBar result={<FormAlert>{w.refused}</FormAlert>} about={{ id: "submit-about-6", text: w.deletes }}>
          <Button kind="primary" aria-describedby="submit-about-6">
            {w.save}
          </Button>
        </SubmitBar>
      </Column>
    </>
  );
}

/**
 * What came of an action that leaves its row as it was (a download): in the row, under its name, the button staying;
 * a row that has said nothing keeps the place. And facts told from the ones before them, under their own name.
 */
function RowsThatSay() {
  const w = useWords();
  const said = [{ text: w.started, n: 1 }, null, { text: w.failed, n: 2 }];
  return (
    <Column>
      <SettingsSection heading={w.files}>
        <SettingsList label={w.files}>
          {w.fileRows.map((title, i) => (
            <SettingsListRow
              key={title}
              title={title}
              said={said[i] ?? null}
              end={<Button size="timeline">{w.download}</Button>}
            />
          ))}
        </SettingsList>
      </SettingsSection>
      <SettingsSection heading={w.preview}>
        {/* Further apart than the facts of one list are. */}
        <Stack gap="form-page-form-gap">
          <SettingsFacts label={w.preview} items={w.facts.map((text) => ({ text }))} />
          <SettingsFacts label={w.removes} shown items={[{ text: w.removed }]} />
        </Stack>
      </SettingsSection>
    </Column>
  );
}

const languages = [
  "English",
  "Español",
  "Français",
  "Deutsch",
  "Português (Brasil)",
  "日本語",
  "简体中文",
  "한국어",
].map((label) => ({ value: label, label }));
const grouped = [
  {
    label: "Asia",
    options: ["Tokyo (UTC+9)", "Seoul (UTC+9)", "Kolkata (UTC+5:30)"].map((l) => ({ value: l, label: l })),
  },
  { label: "Europe", options: ["Lisbon (UTC+0)", "Berlin (UTC+1)"].map((l) => ({ value: l, label: l })) },
  { label: "America", options: ["New York (UTC−5)", "Los Angeles (UTC−8)"].map((l) => ({ value: l, label: l })) },
];

function Fields() {
  const w = useWords();
  return (
    <Column>
      {/* On a raised surface, the fields take the page's. */}
      <SettingsSection heading={w.section}>
        <SettingRow id="field-language" label={w.language} form>
          <Select labelledBy="field-language" options={languages} />
        </SettingRow>
        <SettingRow id="field-zone" label={w.zone} form>
          <Select labelledBy="field-zone" options={grouped} />
        </SettingRow>
        <SettingRow id="field-ends" label={w.ends} about={w.endsAbout} form>
          <NumberField labelledBy="field-ends" defaultValue={30} min={1} unit={w.days} />
        </SettingRow>
        <SettingRow id="field-threshold" label={w.threshold} form>
          <NumberField labelledBy="field-threshold" defaultValue={5} min={0} unit={w.mentions} />
        </SettingRow>
      </SettingsSection>
      {/* On the page, they are on the neutral surface of a control. */}
      <SettingRow id="page-language" label={w.language} form>
        <Select labelledBy="page-language" options={languages} />
      </SettingRow>
      <SettingRow id="page-ends" label={w.ends} form>
        <NumberField labelledBy="page-ends" defaultValue={30} min={1} unit={w.days} />
      </SettingRow>
      <SettingRow id="page-off" label={w.zone} form>
        <Select labelledBy="page-off" options={grouped} isDisabled />
      </SettingRow>
    </Column>
  );
}

/** What cannot be changed now: the name muted, the reason in the place of the line about it, the control disabled. */
function Disabled() {
  const w = useWords();
  return (
    <Column>
      <SettingsSection heading={w.section}>
        <SettingRow id="off-zone" label={w.zone} isDisabled reason={w.zoneOff} save={null}>
          <Select options={grouped} />
        </SettingRow>
        <SettingRow id="off-ends" label={w.ends} about={w.endsAbout} isDisabled reason={w.endsOff} save={null}>
          <NumberField defaultValue={30} min={1} unit={w.days} />
        </SettingRow>
        <Switch label={w.arrive} description={w.arriveAbout} isDisabled reason={w.arriveOff} save={null} />
      </SettingsSection>
    </Column>
  );
}

/** Fields of a form saved as a whole whose sent values the server refused: the words under each, the field marked. */
function Refused() {
  const w = useWords();
  return (
    <Column>
      <SettingsSection heading={w.section}>
        <SettingRow form id="refused-number" label={w.ends} about={w.range} error={w.overRange}>
          <NumberField name="ends" defaultValue={30} min={1} max={365} unit={w.days} />
        </SettingRow>
        <SettingRow form id="refused-select" label={w.zone} error={w.overZone}>
          <Select name="zone" options={zones} />
        </SettingRow>
      </SettingsSection>
    </Column>
  );
}

/**
 * A number field given a number outside what it takes, over the most and under the least, shows the nearest it
 * takes; and a field with none.
 */
/** A text field among a section's rows, named as they are, not as a field of a form of fields. */
function TextAmongRows() {
  const w = useWords();
  return (
    <Column>
      <SettingsSection heading={w.section}>
        <SettingRow id="rows-days" label={w.ends} form>
          <NumberField defaultValue={30} min={1} max={365} unit={w.days} />
        </SettingRow>
        <TextField id="rows-text" name="text" label={w.filters} row defaultValue={w.filterRows[0]?.[0]} />
      </SettingsSection>
    </Column>
  );
}

function NumberRange() {
  const w = useWords();
  return (
    <Column>
      <SettingsSection heading={w.section}>
        <SettingRow id="range-over" label={w.ends} about={w.range} save={null}>
          <NumberField defaultValue={400} min={1} max={365} unit={w.days} />
        </SettingRow>
        <SettingRow id="range-under" label={w.ends} about={w.range} save={null}>
          <NumberField defaultValue={0} min={1} max={365} unit={w.days} />
        </SettingRow>
        <SettingRow id="range-none" label={w.ends} about={w.endsAbout} save={null}>
          <NumberField min={1} max={365} unit={w.days} />
        </SettingRow>
      </SettingsSection>
    </Column>
  );
}

function Reauth() {
  const w = useWords();
  const pair = {
    closeLabel: w.close,
    cancel: w.cancel,
    onCancel: () => undefined,
    onConfirm: () => undefined,
    layer: false,
  };
  return (
    <div className="grid justify-items-start gap-shell-section p-shell-edge">
      <ReauthDialog {...pair} title={w.confirmTitle} about={w.confirmAbout} confirm={w.confirm}>
        <TextField id="reauth-password" name="password" type="password" label={w.password} />
      </ReauthDialog>
      <ReauthDialog
        {...pair}
        title={w.confirmTitle}
        about={w.codeAbout}
        confirm={w.confirm}
        other={<TextButton>{w.usePassword}</TextButton>}
      >
        <TextField id="reauth-code" name="code" label={w.code} inputMode="numeric" />
      </ReauthDialog>
      <ReauthDialog
        {...pair}
        title={w.confirmTitle}
        about={w.passkeyAbout}
        confirm={w.usePasskey}
        other={<TextButton>{w.usePassword}</TextButton>}
      />
      <ReauthDialog {...pair} title={w.confirmTitle} about={w.providerAbout} confirm={w.useProvider} />
      <ReauthDialog
        {...pair}
        title={w.confirmTitle}
        about={w.confirmAbout}
        confirm={w.confirm}
        error={<FormAlert>{w.wrong}</FormAlert>}
      >
        <TextField id="reauth-wrong" name="password" type="password" label={w.password} />
      </ReauthDialog>
      {/* An act done at once: what the press does over how to prove it, the button named by the act, and the other
          ways each a row of its own. */}
      <ReauthDialog
        {...pair}
        title={w.actTitle}
        effect={w.actEffect}
        about={w.actAbout}
        confirm={w.act}
        other={[
          <TextButton key="passkey">{w.usePasskeyInstead}</TextButton>,
          <TextButton key="provider">{w.useProviderInstead}</TextButton>,
        ]}
      >
        <TextField id="reauth-act" name="password" type="password" label={w.password} />
      </ReauthDialog>
      {/* No way to prove it here: it says so, with nothing to confirm; Cancel and close are its ways out. */}
      <ReauthDialog {...pair} title={w.confirmTitle} about={w.confirmAbout} error={<FormAlert>{w.noWay}</FormAlert>} />
    </div>
  );
}

/** The proof in flight: the confirmation spins, and neither close nor Cancel answers. */
function ReauthInFlight() {
  const w = useWords();
  return (
    <div className="grid justify-items-start gap-shell-section p-shell-edge">
      <ReauthDialog
        title={w.confirmTitle}
        about={w.confirmAbout}
        closeLabel={w.close}
        cancel={w.cancel}
        confirm={w.confirm}
        onCancel={() => undefined}
        onConfirm={() => undefined}
        busy
        layer={false}
      >
        <TextField id="reauth-busy" name="password" type="password" label={w.password} />
      </ReauthDialog>
    </div>
  );
}

const codes = [
  "q6b4k-rs",
  "q8f2m-s1",
  "qa1c7-sn",
  "qc7x3-t9",
  "qe5h9-tv",
  "qg3n5-uh",
  "qi1t1-v3",
  "qjz8x-vp",
  "qlx4t-wb",
  "qnv0p-wx",
];

// A finder pattern in place of a whole code: a square of 15 modules, the quiet zone of four included.
const qrPath = "M4 4h7v7H4zM5 5v5h5V5zM6 6h3v3H6z";

/** What the reader copies and keeps: recovery codes, shown once, before copying and after. */
function Kept() {
  const w = useWords();
  return (
    <Column>
      {[undefined, w.copied].map((result) => (
        <SettingsSection key={result ?? ""} heading={w.recovery} about={w.recoveryAbout}>
          <CopyBlock
            label={w.recovery}
            values={codes}
            action={<Button size="timeline">{w.copy}</Button>}
            result={result}
          />
        </SettingsSection>
      ))}
    </Column>
  );
}

/** The key of an authenticator app, as a QR code and as text to copy, then the field of the code the app makes. */
function Authenticator() {
  const w = useWords();
  return (
    <Column>
      <SettingsSection heading={w.setup} about={w.setupAbout}>
        <QrFrame label={w.qr} size={15} path={qrPath} />
        <CopyBlock
          label={w.key}
          values={["JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"]}
          action={<Button size="timeline">{w.copy}</Button>}
        />
      </SettingsSection>
      <SettingsSection heading={w.enter}>
        <TextField id="authenticator-code" name="code" label={w.code} hint={w.codeHint} inputMode="numeric" />
      </SettingsSection>
      <SubmitBar>
        <Button kind="primary">{w.turnOn}</Button>
      </SubmitBar>
    </Column>
  );
}

/**
 * Rows that say what is known of each thing, one fact a line, and after the list a sentence of the section with the
 * links to where the reader acts on it.
 */
function Notes() {
  const w = useWords();
  return (
    <Column>
      <SettingsSection heading={w.browsers}>
        {/* What came of a change whose row went with its button: a status at the section's top. */}
        <SettingsResult>
          <SaveState state="saved">{w.signedOut}</SaveState>
        </SettingsResult>
        <SettingsList label={w.browsers}>
          <SettingsListRow
            icon="public"
            title="Firefox on macOS"
            facts={[w.thisBrowser, w.signedIn, w.from, w.lastUsed]}
          />
          <SettingsListRow
            icon="public"
            title="Safari on iPhone"
            facts={[w.signedIn, w.from, w.lastUsed]}
            end={<Button size="timeline">{w.signOut}</Button>}
          />
        </SettingsList>
        <SettingsNote
          links={[
            { href: "/settings/security/password", label: w.changePassword },
            { href: "/settings/apps", label: w.apps },
          ]}
        >
          {w.advice}
        </SettingsNote>
      </SettingsSection>
    </Column>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
/**
 * A secret shown once, as long as a server makes one: whole in its box, on as many lines as it takes, under the words
 * that say it is not shown again and over the button that copies it.
 */
function Secret() {
  const w = useWords();
  return (
    <Column>
      <SettingsSection heading={w.secret} about={w.secretAbout}>
        <CopyBlock
          label={w.secret}
          values={["my_cs_Jq3vX8nB2mK7pL0wR5tY9uE4iO6aS1dF3gH8jZ2xC7v"]}
          action={<Button size="timeline">{w.copySecret}</Button>}
        />
      </SettingsSection>
    </Column>
  );
}

/**
 * What a section states and nothing in it is pressed: facts one under another at one distance, with a muted line or
 * two about them or none, the same under each heading.
 */
function Facts() {
  const w = useWords();
  return (
    <Column>
      <SettingsSection heading={w.attention} count={String(w.granted.length)}>
        <SettingsFacts label={w.attention} items={w.granted.map(([text, about]) => ({ text, about }))} />
      </SettingsSection>
      <SettingsSection heading={w.reads} count={String(w.read.length)}>
        <SettingsFacts label={w.reads} items={w.read.map((text) => ({ text }))} />
      </SettingsSection>
    </Column>
  );
}

/**
 * A list of things that work and things that do not: the title of one that does not is muted, so that the ones that
 * work stand out; and the sentence in the place of the add action the reader cannot take, apart from the description.
 */
function Working() {
  const w = useWords();
  return (
    <Column>
      <SettingsSection heading={w.invites} count={String(w.inviteRows.length)} about={w.invitesAbout}>
        <SettingsNote instead>{w.invitesFull}</SettingsNote>
        <SettingsList label={w.invites}>
          {w.inviteRows.map(([title, works, facts], i) => (
            <SettingsListRow
              key={title}
              icon="invite"
              title={title}
              inactive={!works}
              facts={facts}
              href={`/settings/invites/${String(i)}`}
            />
          ))}
        </SettingsList>
      </SettingsSection>
    </Column>
  );
}

/** Rows of records (a reader's requests), each marked by where it stands, as every list of records marks it. */
function MarkedRows() {
  const ja = new Intl.Locale(useLocale().locale).language === "ja";
  const rows: [string, "pending" | "resolved" | "failed", string, string][] = ja
    ? [
        [":sparkle_cat:", "pending", "審査待ち", "11月18日に申請"],
        [":tea_time:", "resolved", "承認済み", "11月2日に申請"],
        [":blobheart:", "failed", "断った", "10月28日に申請"],
      ]
    : [
        [":sparkle_cat:", "pending", "Waiting for review", "Asked Nov 18"],
        [":tea_time:", "resolved", "Approved", "Asked Nov 2"],
        [":blobheart:", "failed", "Declined", "Asked Oct 28"],
      ];
  return (
    <Column>
      <SettingsSection heading={ja ? "申請" : "Requests"}>
        <SettingsList label={ja ? "申請" : "Requests"}>
          {rows.map(([title, state, words, asked]) => (
            <SettingsListRow
              key={title}
              title={title}
              facts={[asked]}
              mark={<StateMark state={state}>{words}</StateMark>}
            />
          ))}
        </SettingsList>
      </SettingsSection>
    </Column>
  );
}

const meta = { title: "Settings parts", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** A setting's row and a switch at rest, saving, saved and failed: the state in the place of the line under the name. */
export const SaveStatesOfAControl: Story = { render: () => <SaveStates /> };
/** The status summary: something to do, with the screen's primary action; all well; and the row that takes over. */
export const StatusSummaries: Story = { render: () => <Summaries /> };
/** A list with its add action, the list empty, and the one list of a screen, whose rows open. */
export const ListAndAdd: Story = { render: () => <Lists /> };
/**
 * The submit bar at rest, sending, saved and refused: the result right before the button, every time; and with what
 * saving does beside the button, alone, saved and refused.
 */
export const SubmitBarStates: Story = { render: () => <SubmitBars /> };
/** Rows that say what came of their own action and keep their button, and facts apart under a name of their own. */
export const RowsThatSayAndFactsApart: Story = { render: () => <RowsThatSay /> };
/** A choice from a list, plain and in groups, and a number with its unit, on a raised surface and on the page. */
export const ChoiceAndNumber: Story = { render: () => <Fields /> };
/** Authenticating again: by the password, a two-factor code, a passkey, an external provider, and refused. */
export const AuthenticateAgain: Story = { render: () => <Reauth /> };
/** Authenticating again while the proof is in flight. */
export const AuthenticatingAgain: Story = { render: () => <ReauthInFlight /> };
/** A setting that cannot be changed now, with its reason: a choice, a number, a switch. */
export const DisabledSettings: Story = { render: () => <Disabled /> };
/** Recovery codes shown once, to copy and keep: before copying, and once copied. */
export const CodesToKeep: Story = { render: () => <Kept /> };
/** Setting an authenticator app up: its key as a QR code and as text, then the code the app makes. */
export const AuthenticatorSetup: Story = { render: () => <Authenticator /> };
/** Rows of facts, one a line, and a sentence of the section with the links to where the reader acts on it. */
export const NoteWithLinks: Story = { render: () => <Notes /> };
/** A secret of a server's length shown once: it wraps inside its box, whole, at any width. */
export const SecretToKeep: Story = { render: () => <Secret /> };
/** Facts a section states, with and without a line about them: one distance between them, and under each heading. */
export const StatedFacts: Story = { render: () => <Facts /> };
/** Rows that work and rows that do not, muted, and the sentence that says why the add action is not there. */
export const WorkingAndNot: Story = { render: () => <Working /> };
/** A number and a choice of a form whose sent values the server refused: what is wrong right under each. */
export const RefusedValues: Story = { render: () => <Refused /> };
/** A number field given a number over its most and under its least, and none. */
export const NumberOutOfRange: Story = { render: () => <NumberRange /> };
/** A text field among the rows of a section: its name in the rows' look. */
export const TextAmongSettingRows: Story = { render: () => <TextAmongRows /> };
/** Rows of records marked by where they stand, with the mark every list of records uses. */
export const RowsMarkedByState: Story = { render: () => <MarkedRows /> };
