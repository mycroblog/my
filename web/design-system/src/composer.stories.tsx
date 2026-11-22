// SPDX-License-Identifier: AGPL-3.0-only

// The composer's text in its editor, in the reader's language (the toolbar's locale; ar-XB reads right-to-left), with
// the suggestions for the word at the caret, under its line across the text's width: accounts for `@` (followed
// first), hashtags for `#` (by their use), emoji for `:` (custom, then Unicode), the first active; while they are
// asked for, and when none came; and the notice that follows a write, in the flow, with its action. The marks of what
// the server will read are the app's nodes, in its composer stories. The words are placeholders.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { Button } from "./button";
import { CharCount, ComposerFrame, ComposerText, UndoNotice } from "./composer";
import { PlainTextEditor } from "./editor";
import { sampleCustomEmoji } from "./emoji.samples";
import { IconButton } from "./icon-button";
import { images } from "./samples";
import { SuggestionList, type Suggestion } from "./suggestions";

type Lang = "en" | "ja" | "ar";
const useLang = (): Lang => {
  const language = new Intl.Locale(useLocale().locale).language;
  return language === "ja" ? "ja" : language === "ar" ? "ar" : "en";
};

const words = {
  en: {
    title: "New post",
    close: "Close",
    post: "Post",
    label: "Text",
    placeholder: "What's on your mind?",
    cwLabel: "Content warning",
    cw: "Trail photos :sparkle: lots of them",
    marked:
      "Thanks @sam and @layla@social.example for the walk! Photos at https://photos.example/ridge #山 #hiking :sparkle:",
    accounts: "Lunch tomorrow with @sa",
    hashtags: "First frost on the trail #ph",
    emoji: "Finally finished the quilt :sp",
    list: "Suggestions",
    loading: "Loading…",
    empty: "No suggestions",
    failed: "The notification could not be dismissed.",
    retry: "Try again",
    restricted:
      "A moderation action keeps your account from following, favouriting, boosting, or replying for now. To see why or to appeal, open “Account status” under “Account and data” in the settings.",
    status: "Account status",
    uses: (n: number) => `${new Intl.NumberFormat("en").format(n)} posts`,
  },
  ja: {
    title: "新しい投稿",
    close: "閉じる",
    post: "投稿",
    label: "本文",
    placeholder: "いまどうしてる？",
    cwLabel: "注意書き",
    cw: "登山の写真 :sparkle: たくさん",
    marked:
      "@sam さん @layla@social.example さん、今日はありがとう！写真は https://photos.example/ridge に #山 #写真 :sparkle:",
    accounts: "明日のお昼は @sa",
    hashtags: "今朝の尾根 #山",
    emoji: "キルトがやっと完成 :sp",
    list: "候補",
    loading: "読み込み中…",
    empty: "候補はありません",
    failed: "通知を消せませんでした。",
    retry: "もう一度試す",
    restricted:
      "モデレーションの処分により、今はこのアカウントでフォロー・お気に入り・ブースト・返信ができません。理由の確認と異議申し立ては、設定の「アカウントとデータ」にある「アカウントの状態」からできます。",
    status: "アカウントの状態",
    uses: (n: number) => `${new Intl.NumberFormat("ja").format(n)} 件`,
  },
  ar: {
    title: "منشور جديد",
    close: "إغلاق",
    post: "انشر",
    label: "النص",
    placeholder: "ماذا يدور في ذهنك؟",
    cwLabel: "تحذير المحتوى",
    cw: "صور المسار :sparkle: كثيرة",
    marked:
      "شكرًا @sam و @layla@social.example على النزهة! الصور في https://photos.example/ridge #جبال #hiking :sparkle:",
    accounts: "غداء الغد مع @sa",
    hashtags: "أول صقيع على المسار #ph",
    emoji: "أنهيت اللحاف أخيرًا :sp",
    list: "اقتراحات",
    loading: "جارٍ التحميل…",
    empty: "لا توجد اقتراحات",
    failed: "تعذّر تجاهل الإشعار.",
    retry: "حاول مرة أخرى",
    restricted:
      "يمنع إجراء إشرافي حسابك من المتابعة والتفضيل والترقية والرد حاليًا. لمعرفة السبب أو للطعن، افتح «حالة الحساب» في الإعدادات.",
    status: "حالة الحساب",
    uses: (n: number) => `${new Intl.NumberFormat("ar").format(n)} منشور`,
  },
} as const;

const people = {
  en: ["Sam Rivera", "Sasha Kim", "Samira Haddad", "Saoirse O'Neill"],
  ja: ["サム・リベラ", "佐々木 さくら", "Samira Haddad", "早乙女 葵"],
  ar: ["سام ريفيرا", "Sasha Kim", "سميرة حداد", "Saoirse O'Neill"],
};

/** The accounts for `@sa`: the followed first (sam, sasha), then by their followers. */
const accounts = (lang: Lang): Suggestion[] =>
  [
    { handle: "@sam", avatar: images.dusk },
    { handle: "@sasha@mastodon.example", avatar: images.snow },
    { handle: "@samira@social.example", avatar: undefined },
    { handle: "@saoirse_on_a_very_long_handle@photos.example", avatar: images.logo },
  ].map(({ handle, avatar }, i) => {
    const name = people[lang][i] ?? "";
    return { kind: "account", name, initialOf: name, handle, avatar };
  });

const hashtags = (lang: Lang): Suggestion[] =>
  (lang === "ja"
    ? [
        ["山", 1204],
        ["山登り", 342],
        ["山歩き", 87],
        ["山小屋", 9],
      ]
    : [
        ["photography", 1204],
        ["photo", 860],
        ["photosOfTheDay", 87],
        ["phonography", 9],
      ]
  ).map(([name, n]) => ({ kind: "hashtag", name: String(name), uses: words[lang].uses(Number(n)) }));

const emoji = (lang: Lang): Suggestion[] => [
  { kind: "emoji", name: ":sparkle:", image: sampleCustomEmoji["sparkle"] },
  { kind: "emoji", name: ":party:", image: sampleCustomEmoji["party"] },
  { kind: "emoji", name: lang === "ja" ? "キラキラ" : "sparkles", text: "✨" },
  { kind: "emoji", name: lang === "ja" ? "キラキラ ハート" : "sparkling heart", text: "💖" },
  { kind: "emoji", name: lang === "ja" ? "スペード" : "spade suit", text: "♠️" },
];

type Kind = "accounts" | "hashtags" | "emoji";
const kinds: Record<Kind, (lang: Lang) => Suggestion[]> = { accounts, hashtags, emoji };

/** The composer's frame around what a story writes, as a dialog shows it, with its row. */
function Frame({ children }: { children: ReactNode }) {
  const w = words[useLang()];
  return (
    <div style={{ maxInlineSize: "37.5rem", marginInline: "auto" }}>
      <ComposerFrame
        title={w.title}
        closeLabel={w.close}
        onClose={() => undefined}
        onDismiss={() => undefined}
        layer={false}
        tools={<IconButton icon="attach" label="Attach" />}
        count={<CharCount left={420} near={20} label="420" />}
        action={
          <Button kind="primary" size="timeline">
            {w.post}
          </Button>
        }
      >
        {children}
      </ComposerFrame>
    </div>
  );
}

/** The text, with the suggestions of `kind` for the word at its end, or the line in their place. */
function Suggesting({ kind, status }: { kind: Kind; status?: "loading" | "empty" | undefined }) {
  const lang = useLang();
  const w = words[lang];
  const value = w[kind];
  const items = status === undefined ? kinds[kind](lang) : [];
  // The arrow keys move the active option, as in the composer.
  const [active, setActive] = useState(0);
  const id = `suggest-${kind}`;
  return (
    <Frame>
      <ComposerText>
        <PlainTextEditor
          namespace="story"
          look="composer"
          value={value}
          onChange={() => undefined}
          label={w.label}
          placeholder={w.placeholder}
          roomy
          completes
          suggest={{
            at: value.length,
            list: (
              <SuggestionList
                id={id}
                label={w.list}
                items={items}
                active={active}
                onChoose={() => undefined}
                status={status === undefined ? undefined : { kind: status, text: w[status] }}
              />
            ),
            listId: items.length > 0 ? id : undefined,
            count: items.length,
            active,
            announce: status === undefined ? String(items.length) : w[status],
            onActive: setActive,
            onAccept: () => undefined,
            onClose: () => undefined,
          }}
        />
      </ComposerText>
    </Frame>
  );
}

const against = {
  ltr: "Walked along the river this morning, and the light on the water was worth the early start.",
  rtl: "مشيت على طول النهر هذا الصباح، وكان الضوء على الماء يستحق الاستيقاظ المبكر.",
};

/**
 * The text in the direction `text` runs, whatever the page's, with the emoji button at the end of its first line in
 * the page's direction.
 */
function Ended({ text }: { text: keyof typeof against }) {
  const w = words[useLang()];
  return (
    <Frame>
      <ComposerText end={<IconButton icon="emojiSmileys" label="Add emoji" />}>
        <PlainTextEditor
          namespace="story"
          look="composer"
          value={against[text]}
          onChange={() => undefined}
          label={w.label}
          placeholder={w.placeholder}
          roomy
          ended
        />
      </ComposerText>
    </Frame>
  );
}

/** A notice in the flow, with its action: a write that failed, and Try again. */
function Noticed() {
  const w = words[useLang()];
  return (
    <div style={{ padding: "1rem" }}>
      <UndoNotice inline action={w.retry} onAction={() => undefined}>
        {w.failed}
      </UndoNotice>
    </div>
  );
}

/** A notice in the flow whose action is a link: a refusal of a restricted account, and Account status. */
function NoticedLink() {
  const w = words[useLang()];
  return (
    <div style={{ padding: "1rem" }}>
      <UndoNotice inline action={w.status} href="/settings/account/status">
        {w.restricted}
      </UndoNotice>
    </div>
  );
}

// Tagged `themes`: the check of every story shows each in dark too.
const meta = {
  title: "Composer",
  tags: ["themes"],
  parameters: { layout: "fullscreen" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** `@sa`: the accounts, the followed first, the first active. */
export const Accounts: Story = { render: () => <Suggesting kind="accounts" /> };
/** `#ph` (`#山` in ja): the hashtags by their use. */
export const Hashtags: Story = { render: () => <Suggesting kind="hashtags" /> };
/** `:sp`: the custom emoji, then the Unicode ones. */
export const Emoji: Story = { render: () => <Suggesting kind="emoji" /> };
/** A right-to-left page (the toolbar's ar-XB) and a text in English: the button at the page's end. */
export const EndedLeftToRightText: Story = { render: () => <Ended text="ltr" /> };
/** A left-to-right page and a text in Arabic: the button at the page's end. */
export const EndedRightToLeftText: Story = { render: () => <Ended text="rtl" /> };
/** Asked for and not here after a moment. */
export const Loading: Story = { render: () => <Suggesting kind="accounts" status="loading" /> };
/** None came. */
export const Empty: Story = { render: () => <Suggesting kind="hashtags" status="empty" /> };
/** The notice at the bottom of the page after a write that failed: what failed, and its action. */
export const NoticeWithAction: Story = { render: () => <Noticed /> };
/** The notice of a refusal the reader can appeal: the server's sentence, and a link to Account status. */
export const NoticeWithLink: Story = { render: () => <NoticedLink /> };
