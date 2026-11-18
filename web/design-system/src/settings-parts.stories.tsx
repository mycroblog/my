// SPDX-License-Identifier: AGPL-3.0-only

// The parts of a settings screen (ADR-0110 §5), each in its states, in a column of the main column's width, in the
// reader's language (the toolbar's locale; ja, else en): the save state of a control that saves at once, in a
// setting's row and in a switch; the status summary, with something to do and with all well; a list with its one add
// action, and empty, where what stands in its place leads to adding; the submit bar of a form saved as a whole, at
// rest, sending, saved and refused; a choice from a long list and a number; and the confirmation that asks the reader
// to authenticate again, by each proof.

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { Button, TextButton } from "./button";
import { ReauthDialog } from "./dialog";
import { NumberField, Select } from "./field";
import { FormAlert, TextField } from "./form-page";
import { SaveState } from "./save-state";
import {
  SettingRow,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsSection,
  StatusSummary,
  SubmitBar,
} from "./settings";
import { Switch } from "./switch";

const en = {
  saving: "Saving…",
  saved: "Saved",
  failed: "Couldn't save.",
  retry: "Try again",
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
  on: "On",
  twoFactorOff: "Two-factor authentication is off",
  twoFactorOn: "Two-factor authentication is on",
  offAbout: "Anyone who learns your password can sign in. A code from your phone stops them.",
  onAbout: "Signing in asks for a code from your phone after the password.",
  turnOn: "Turn on",
  signIn: "Sign-in",
  passwordAbout: "Changed 3 months ago",
  twoFactorRow: "A code from your phone · 8 recovery codes left",
  passkeys: "Passkeys",
  passkeysAbout: "Sign in with your device's lock in place of the password.",
  add: "Add a passkey",
  remove: "Remove",
  keys: [
    ["MacBook Touch ID", "Added Mar 3 · used today"],
    ["A passkey with a name long enough to wrap on a phone", "Added Jun 12 · used 5d ago"],
  ],
  none: "No passkeys yet.",
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
  wrong: "That password is not right.",
};

const ja: typeof en = {
  saving: "保存しています…",
  saved: "保存しました",
  failed: "保存できませんでした。",
  retry: "もう一度試す",
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
  on: "オン",
  twoFactorOff: "二要素認証はオフです",
  twoFactorOn: "二要素認証はオンです",
  offAbout: "パスワードを知った人はだれでもログインできます。スマートフォンのコードがそれを防ぎます。",
  onAbout: "ログインのとき、パスワードのあとにスマートフォンのコードを入力します。",
  turnOn: "有効にする",
  signIn: "ログイン",
  passwordAbout: "3か月前に変更",
  twoFactorRow: "スマートフォンのコード · リカバリーコード残り8個",
  passkeys: "パスキー",
  passkeysAbout: "パスワードの代わりに、端末のロックでログインします。",
  add: "パスキーを追加",
  remove: "削除",
  keys: [
    ["MacBook Touch ID", "3月3日に追加 · 今日使用"],
    ["スマートフォンで折り返すほど長い名前を付けたパスキー", "6月12日に追加 · 5日前に使用"],
  ],
  none: "パスキーはまだありません。",
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
  wrong: "パスワードが正しくありません。",
};

const useWords = () => (new Intl.Locale(useLocale().locale).language === "ja" ? ja : en);

/** A column of the main column's width, its sections apart as a settings screen's. */
function Column({ children }: { children: ReactNode }) {
  return <div className="grid max-w-[37.5rem] gap-shell-edge p-shell-edge text-body">{children}</div>;
}

const zones = ["Tokyo (UTC+9)", "Lisbon (UTC+0)", "Los Angeles (UTC−8)"].map((label) => ({ value: label, label }));

function SaveStates() {
  const w = useWords();
  const retry = <TextButton>{w.retry}</TextButton>;
  const states = [
    null,
    <SaveState key="saving" state="saving">
      {w.saving}
    </SaveState>,
    <SaveState key="saved" state="saved">
      {w.saved}
    </SaveState>,
    <SaveState key="failed" state="failed" action={retry}>
      {w.failed}
    </SaveState>,
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
      {/* Once it is on, its row is its door, its state in words at the row's end; the summary is gone. */}
      <SettingsSection heading={w.signIn}>
        <SettingsList label={w.signIn}>
          <SettingsListRow
            icon="security"
            title={w.password}
            caption={w.passwordAbout}
            href="/settings/security/password"
          />
          <SettingsListRow
            icon="time"
            title={w.twoFactor}
            caption={w.twoFactorRow}
            state={w.on}
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
        <SubmitBar result={<SaveState state="saved">{w.saved}</SaveState>}>
          <Button kind="primary">{w.save}</Button>
        </SubmitBar>
      </Column>
      <Column>
        {field(3)}
        <SubmitBar result={<FormAlert>{w.refused}</FormAlert>}>
          <Button kind="primary">{w.save}</Button>
        </SubmitBar>
      </Column>
    </>
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
        <SettingRow id="field-language" label={w.language}>
          <Select labelledBy="field-language" options={languages} />
        </SettingRow>
        <SettingRow id="field-zone" label={w.zone}>
          <Select labelledBy="field-zone" options={grouped} />
        </SettingRow>
        <SettingRow id="field-ends" label={w.ends} about={w.endsAbout}>
          <NumberField labelledBy="field-ends" defaultValue="30" unit={w.days} />
        </SettingRow>
        <SettingRow id="field-threshold" label={w.threshold}>
          <NumberField labelledBy="field-threshold" defaultValue="5" unit={w.mentions} />
        </SettingRow>
      </SettingsSection>
      {/* On the page, they are on the neutral surface of a control. */}
      <SettingRow id="page-language" label={w.language}>
        <Select labelledBy="page-language" options={languages} />
      </SettingRow>
      <SettingRow id="page-ends" label={w.ends}>
        <NumberField labelledBy="page-ends" defaultValue="30" unit={w.days} />
      </SettingRow>
      <SettingRow id="page-off" label={w.zone}>
        <Select labelledBy="page-off" options={grouped} isDisabled />
      </SettingRow>
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
    </div>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Settings parts", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** A setting's row and a switch at rest, saving, saved and failed: the state in the place of the line under the name. */
export const SaveStatesOfAControl: Story = { render: () => <SaveStates /> };
/** The status summary: something to do, with the screen's primary action; all well; and the row that takes over. */
export const StatusSummaries: Story = { render: () => <Summaries /> };
/** A list with its add action, the list empty, and the one list of a screen, whose rows open. */
export const ListAndAdd: Story = { render: () => <Lists /> };
/** The submit bar at rest, sending, saved and refused: the result right before the button, every time. */
export const SubmitBarStates: Story = { render: () => <SubmitBars /> };
/** A choice from a list, plain and in groups, and a number with its unit, on a raised surface and on the page. */
export const ChoiceAndNumber: Story = { render: () => <Fields /> };
/** Authenticating again: by the password, a two-factor code, a passkey, an external provider, and refused. */
export const AuthenticateAgain: Story = { render: () => <Reauth /> };
