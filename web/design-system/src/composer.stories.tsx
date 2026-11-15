// SPDX-License-Identifier: AGPL-3.0-only

// The composer's text as it is typed, in the reader's language (the toolbar's locale; ar-XB reads right-to-left): what
// the server will read (mentions, hashtags, URLs, custom emoji) marked behind it, also where a line breaks inside one,
// and the part over the limit; the content warning's custom emoji; and the suggestions for the word at the caret,
// under its line across the text's width: accounts for `@` (followed first), hashtags for `#` (by their use), emoji
// for `:` (custom, then Unicode), the first active; while they are asked for, and when none came. The words are
// placeholders.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { Button } from "./button";
import { CharCount, ComposerFrame, ComposerText, WarningField, type TextMark } from "./composer";
import { sampleCustomEmoji } from "./emoji.samples";
import { IconButton } from "./icon-button";
import { images } from "./samples";
import { SuggestionList, type Suggestion } from "./suggestions";

type Lang = "en" | "ja" | "ar";
const useLang = (): Lang => {
  const language = new Intl.Locale(useLocale().locale).language;
  return language === "ja" ? "ja" : language === "ar" ? "ar" : "en";
};

/** The marks of a sample text: a sketch of the server's rules, which the app's own parser follows. */
function marksOf(text: string): TextMark[] {
  const marks: TextMark[] = [];
  const find = (pattern: RegExp, kind: TextMark["kind"], keep: (s: string) => boolean = () => true) => {
    for (const m of text.matchAll(pattern))
      if (keep(m[0])) marks.push({ start: m.index, end: m.index + m[0].length, kind });
  };
  find(/https?:\/\/[^\s]+/gu, "url");
  find(/(?<![\p{L}\p{N}_@/])@[A-Za-z0-9_]+(?:@[A-Za-z0-9.-]*[A-Za-z0-9])?/gu, "mention");
  find(/(?<![\p{L}\p{N}_#/])#[\p{L}\p{N}_]+/gu, "hashtag");
  find(/(?<![A-Za-z0-9:]):[A-Za-z0-9_]{2,}:/gu, "emoji", (s) => s.slice(1, -1) in sampleCustomEmoji);
  return marks;
}

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
    uses: (n: number) => `${n.toLocaleString("en")} posts`,
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
    uses: (n: number) => `${n.toLocaleString("ja")} 件`,
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
    uses: (n: number) => `${n.toLocaleString("ar")} منشور`,
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
      <ComposerText
        value={value}
        onChange={() => undefined}
        label={w.label}
        placeholder={w.placeholder}
        over={0}
        marks={marksOf(value)}
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
    </Frame>
  );
}

/** The marked text, with as much over the limit as `over`, under the content warning when `cw`. */
function Marked({ over = 0, cw = false, narrow = false }: { over?: number; cw?: boolean; narrow?: boolean }) {
  const w = words[useLang()];
  const field = (
    <ComposerText
      value={w.marked}
      onChange={() => undefined}
      label={w.label}
      placeholder={w.placeholder}
      over={over}
      marks={marksOf(w.marked)}
    />
  );
  return (
    <Frame>
      {cw && (
        <WarningField
          value={w.cw}
          onChange={() => undefined}
          label={w.cwLabel}
          placeholder={w.cwLabel}
          marks={marksOf(w.cw).filter((m) => m.kind === "emoji")}
        />
      )}
      {narrow ? <div style={{ maxInlineSize: "15rem" }}>{field}</div> : field}
    </Frame>
  );
}

// Tagged `themes`: the story tests run each in dark too.
const meta = {
  title: "Composer",
  tags: ["themes"],
  parameters: { layout: "fullscreen" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Mentions, hashtags, a URL and a custom emoji marked behind the text as it is typed. */
export const Marks: Story = { render: () => <Marked /> };
/** The same in a narrow text, where the field breaks a line inside a URL or a hashtag: the mark follows it. */
export const MarksNarrow: Story = { render: () => <Marked narrow /> };
/** Past the limit: the part over it in a deeper neutral, underlined, over any mark in it. */
export const MarksOverLimit: Story = { render: () => <Marked over={24} /> };
/** The content warning marks its custom emoji only. */
export const MarksContentWarning: Story = { render: () => <Marked cw /> };
export const MarksRightToLeft: Story = { render: () => <Marked cw />, globals: { locale: "ar-XB" } };
/** `@sa`: the accounts, the followed first, the first active. */
export const Accounts: Story = { render: () => <Suggesting kind="accounts" /> };
/** `#ph` (`#山` in ja): the hashtags by their use. */
export const Hashtags: Story = { render: () => <Suggesting kind="hashtags" /> };
/** `:sp`: the custom emoji, then the Unicode ones. */
export const Emoji: Story = { render: () => <Suggesting kind="emoji" /> };
export const AccountsRightToLeft: Story = {
  render: () => <Suggesting kind="accounts" />,
  globals: { locale: "ar-XB" },
};
/** Asked for and not here after a moment. */
export const Loading: Story = { render: () => <Suggesting kind="accounts" status="loading" /> };
/** None came. */
export const Empty: Story = { render: () => <Suggesting kind="hashtags" status="empty" /> };
