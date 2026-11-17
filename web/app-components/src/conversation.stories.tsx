// SPDX-License-Identifier: AGPL-3.0-only

// The direct messages (FR-212), in the reader's language (the toolbar's locale; ar-XB reads right-to-left): the list
// of conversations, newest first, unread and read, with one person, two, and many; the list with none; the list just
// after the reader hid one; a conversation of two, its posts oldest first, the reader's marked as theirs, and the reply
// in place at its end; and a conversation of several. The words are placeholders of the feature's.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId, type ReactNode } from "react";
import {
  ActionRow,
  Avatar,
  BackButton,
  Box,
  Button,
  Byline,
  CharCount,
  ComposerText,
  Entry,
  Feed,
  FeedItem,
  FeedNotice,
  IconButton,
  MenuButton,
  MenuItem,
  ScreenStack,
  Stamp,
  useLocale,
} from "@my/design-system";
import { PlainTextEditor } from "@my/design-system/editor";
import { ago, images } from "@my/design-system/samples";
import {
  ConversationAuthor,
  ConversationHead,
  ConversationLine,
  ConversationReply,
  ConversationText,
} from "./conversation";

const rem = (px: number) => `${px / rootPx}rem`;
const minute = 60_000;
const hour = 60 * minute;

type Lang = "en" | "ja" | "ar";
const useLang = (): Lang => {
  const language = new Intl.Locale(useLocale().locale).language;
  return language === "ja" ? "ja" : language === "ar" ? "ar" : "en";
};

type Person = { name: string; handle: string; avatar?: string | undefined };

const people: Record<Lang, Person[]> = {
  en: [
    { name: "Mika", handle: "@mika@my.example", avatar: images.dusk },
    { name: "Ren", handle: "@ren@my.example", avatar: images.snow },
    { name: "Sora", handle: "@sora@social.example", avatar: images.logo },
    { name: "Aoi Tanaka", handle: "@aoi@my.example" },
    { name: "Haruto", handle: "@haruto@photos.example", avatar: images.dusk },
    { name: "Lina", handle: "@lina@my.example" },
    { name: "Omar", handle: "@omar@social.example", avatar: images.snow },
    {
      name: "Saoirse with a display name long enough to run out of room",
      handle: "@saoirse_on_a_very_long_handle@photos.example",
    },
  ],
  ja: [
    { name: "みか", handle: "@mika@my.example", avatar: images.dusk },
    { name: "レン", handle: "@ren@my.example", avatar: images.snow },
    { name: "そら", handle: "@sora@social.example", avatar: images.logo },
    { name: "田中 葵", handle: "@aoi@my.example" },
    { name: "はると", handle: "@haruto@photos.example", avatar: images.dusk },
    { name: "Lina", handle: "@lina@my.example" },
    { name: "Omar", handle: "@omar@social.example", avatar: images.snow },
    { name: "早乙女 葵（とても長い表示名で行に収まらない）", handle: "@saoirse_on_a_very_long_handle@photos.example" },
  ],
  ar: [
    { name: "ميكا", handle: "@mika@my.example", avatar: images.dusk },
    { name: "رين", handle: "@ren@my.example", avatar: images.snow },
    { name: "سورا", handle: "@sora@social.example", avatar: images.logo },
    { name: "Aoi Tanaka", handle: "@aoi@my.example" },
    { name: "هاروتو", handle: "@haruto@photos.example", avatar: images.dusk },
    { name: "لينا", handle: "@lina@my.example" },
    { name: "عمر", handle: "@omar@social.example", avatar: images.snow },
    { name: "سيرشا باسم عرض طويل بما يكفي لينفد المكان", handle: "@saoirse_on_a_very_long_handle@photos.example" },
  ],
};

const reader: Record<Lang, Person> = {
  en: { name: "Yuki", handle: "@yuki@my.example" },
  ja: { name: "ゆき", handle: "@yuki@my.example" },
  ar: { name: "يوكي", handle: "@yuki@my.example" },
};

const words = {
  en: {
    label: "Direct messages",
    unread: "Unread",
    you: "You",
    more: "More",
    hide: "Hide from the list",
    back: "Direct messages",
    direct: "Direct",
    reply: "Reply",
    favourite: "Favourite",
    react: "React",
    attach: "Add images",
    emoji: "Add emoji",
    send: "Send",
    text: "Message",
    placeholder: "Write a message",
    excerpt: ["", ": "],
    to: (names: string) => `To ${names}`,
    form: (names: string) => `Your message to ${names}`,
    sendTo: (names: string) => `Send to ${names}`,
    with: (names: string) => `Direct messages with ${names}`,
    empty: "No direct messages yet. To send one, mention someone in a new post and set who can see it to Direct.",
    hidden: (names: string) =>
      `You hid your direct messages with ${names} from the list. They come back when a new message arrives.`,
    others: ["", ", ", " and ", " others"],
    texts: [
      "Are you still coming on Saturday? I can bring the spare tripod if you need one.",
      "I uploaded the photos from the ridge to the shared album. The light on the third one is unreal, have a look when you get a minute and tell me which ones you want printed.",
      "Sounds good to me. Let's meet at the station at 9.",
      "Thanks for the recommendation! The book arrived today.",
      "Could you send me the link to the venue again?",
    ],
    thread: [
      "Hi! Are you still coming to the meetup on Saturday?",
      "Yes, wouldn't miss it. Is it still at the library?",
      "It moved to the community hall, two streets down. Same time.",
      "Great, see you there.",
    ],
    group: [
      "Who's bringing what on Saturday? I've got the tripods.",
      "Snacks and the extension cord.",
      "I'll print the map of the trail.",
      "Sounds good to me. Let's meet at the station at 9.",
    ],
  },
  ja: {
    label: "DM",
    unread: "未読",
    you: "あなた",
    more: "その他",
    hide: "一覧から非表示",
    back: "DM",
    direct: "ダイレクト",
    reply: "返信",
    favourite: "お気に入り",
    react: "リアクション",
    attach: "画像を添付",
    emoji: "絵文字を入れる",
    send: "送信する",
    text: "メッセージ",
    placeholder: "メッセージを書く",
    excerpt: ["", ": "],
    to: (names: string) => `宛先: ${names}`,
    form: (names: string) => `${names} へのメッセージ`,
    sendTo: (names: string) => `${names} に送信する`,
    with: (names: string) => `${names} との DM`,
    empty: "DM はまだありません。新しい投稿で相手をメンションし、見られる人を「ダイレクト」にすると送れます。",
    hidden: (names: string) => `${names} との DM を一覧から非表示にしました。新しいメッセージが届くと戻ります。`,
    others: ["", "、", " ほか ", " 人"],
    texts: [
      "土曜日まだ来られそう？ 予備の三脚、必要なら持っていくよ。",
      "尾根で撮った写真を共有アルバムに上げました。3 枚目の光がすごいので、時間のあるときに見て、プリントしたいのを教えてください。",
      "いいね。9 時に駅で待ち合わせよう。",
      "おすすめありがとう！ 本、今日届きました。",
      "会場のリンク、もう一度送ってもらえますか？",
    ],
    thread: [
      "こんにちは！ 土曜日のオフ会、まだ来られそう？",
      "うん、行くよ。まだ図書館でやるの？",
      "2 本先の公民館に変わったよ。時間は同じ。",
      "了解、また土曜日に。",
    ],
    group: [
      "土曜日、誰が何を持っていく？ 三脚はわたしが。",
      "おやつと延長コード。",
      "登山道の地図を印刷しておくね。",
      "いいね。9 時に駅で待ち合わせよう。",
    ],
  },
  ar: {
    label: "الرسائل المباشرة",
    unread: "غير مقروءة",
    you: "أنت",
    more: "المزيد",
    hide: "إخفاء من القائمة",
    back: "الرسائل المباشرة",
    direct: "مباشر",
    reply: "رد",
    favourite: "تفضيل",
    react: "تفاعل",
    attach: "إرفاق",
    emoji: "إضافة رمز تعبيري",
    send: "إرسال",
    text: "رسالة",
    placeholder: "اكتب رسالة",
    excerpt: ["", ": "],
    to: (names: string) => `إلى ${names}`,
    form: (names: string) => `رسالتك إلى ${names}`,
    sendTo: (names: string) => `إرسال إلى ${names}`,
    with: (names: string) => `الرسائل المباشرة مع ${names}`,
    empty: "لا توجد رسائل مباشرة بعد. لإرسال واحدة، اذكر أحدًا في منشور جديد واجعل من يمكنه رؤيته «مباشر».",
    hidden: (names: string) => `أخفيت رسائلك المباشرة مع ${names} من القائمة. تعود عند وصول رسالة جديدة.`,
    others: ["", "، ", " و", " آخرون"],
    texts: [
      "هل ما زلت قادمًا يوم السبت؟ يمكنني إحضار الحامل الاحتياطي إن احتجت إليه.",
      "رفعت صور الجبل إلى الألبوم المشترك. الضوء في الصورة الثالثة مذهل، ألقِ نظرة عندما يتسع وقتك وأخبرني بالصور التي تريد طباعتها.",
      "يبدو جيدًا. لنلتقِ في المحطة الساعة 9.",
      "شكرًا على التوصية! وصل الكتاب اليوم.",
      "هل يمكنك إرسال رابط المكان مرة أخرى؟",
    ],
    thread: [
      "مرحبًا! هل ما زلت قادمًا إلى اللقاء يوم السبت؟",
      "نعم، لن يفوتني. هل ما زال في المكتبة؟",
      "انتقل إلى قاعة الحي، على بعد شارعين. نفس الوقت.",
      "رائع، أراك هناك.",
    ],
    group: [
      "من سيحضر ماذا يوم السبت؟ معي الحوامل.",
      "الوجبات الخفيفة وسلك التمديد.",
      "سأطبع خريطة المسار.",
      "يبدو جيدًا. لنلتقِ في المحطة الساعة 9.",
    ],
  },
} as const;

type W = (typeof words)[Lang];

const languages: Record<Lang, string> = { en: "en", ja: "ja", ar: "ar" };

/**
 * The names of `list` as the locale lists them, each isolated, and as plain text: up to three by name, more as the
 * first two and how many others.
 */
function nameList(lang: Lang, list: Person[]): { node: ReactNode; text: string } {
  const w = words[lang];
  const names = list.map((p) => p.name);
  if (names.length > 3) {
    const [start, sep, and, end] = w.others;
    const others = new Intl.NumberFormat(languages[lang]).format(names.length - 2);
    return {
      node: (
        <>
          {start}
          <bdi>{names[0]}</bdi>
          {sep}
          <bdi>{names[1]}</bdi>
          {and}
          {others}
          {end}
        </>
      ),
      text: `${start}${names[0] ?? ""}${sep}${names[1] ?? ""}${and}${others}${end}`,
    };
  }
  const format = new Intl.ListFormat(languages[lang], { type: "conjunction" });
  return {
    node: format
      .formatToParts(names)
      .map((part, i) => (part.type === "element" ? <bdi key={i}>{part.value}</bdi> : part.value)),
    text: format.format(names),
  };
}

const face = (p: Person) => <Avatar name={p.name} src={p.avatar} size="row" decorative />;

/** A column of the timeline's width, with its side margin. */
const Column = ({ children }: { children: ReactNode }) => (
  <div style={{ maxWidth: rem(600), paddingInline: rem(16), paddingBlock: rem(16) }}>{children}</div>
);

function More({ w }: { w: W }) {
  return (
    <MenuButton icon="more" label={w.more}>
      <MenuItem id="hide" icon="hidden">
        {w.hide}
      </MenuItem>
    </MenuButton>
  );
}

/** A conversation of the samples: its other people (by index), who wrote the latest post (-1: the reader), its text. */
type Sample = { id: string; with: number[]; by: number; text: number; at: number; unread: boolean };

const samples: Sample[] = [
  { id: "c1", with: [0], by: 0, text: 0, at: 5 * minute, unread: true },
  { id: "c2", with: [1, 2], by: 1, text: 1, at: hour, unread: true },
  { id: "c3", with: [3, 4, 5, 6, 2], by: -1, text: 2, at: 3 * hour, unread: false },
  { id: "c4", with: [7], by: 7, text: 3, at: 26 * hour, unread: false },
  { id: "c5", with: [1], by: -1, text: 4, at: 9 * 24 * hour, unread: false },
];

const peopleOf = (lang: Lang, indexes: number[]) =>
  indexes.map((i) => people[lang][i]).filter((p): p is Person => p !== undefined);

/** A conversation of the list. */
function Conversation({ sample, lang }: { sample: Sample; lang: Lang }) {
  const w = words[lang];
  const id = useId();
  const others = peopleOf(lang, sample.with);
  const author = sample.by === -1 ? undefined : people[lang][sample.by];
  const shown = author !== undefined && others.length === 1 ? undefined : (author?.name ?? w.you);
  const text = <ConversationText lang={languages[lang]}>{w.texts[sample.text] ?? ""}</ConversationText>;
  const [before, between] = w.excerpt;
  return (
    <Entry labelledBy={id}>
      <ConversationLine
        faces={others.map(face)}
        names={nameList(lang, others).node}
        namesId={id}
        date={ago(sample.at)}
        unread={sample.unread ? w.unread : undefined}
        more={<More w={w} />}
      >
        {shown === undefined ? (
          text
        ) : (
          <>
            {before}
            <ConversationAuthor>{shown}</ConversationAuthor>
            {between}
            {text}
          </>
        )}
      </ConversationLine>
    </Entry>
  );
}

/** The list of conversations; `hidden`, just after the reader hid the second. */
function List({ state = "normal" }: { state?: "normal" | "empty" | "hidden" }) {
  const lang = useLang();
  const w = words[lang];
  const shown = state === "empty" ? [] : state === "hidden" ? samples.filter((s) => s.id !== "c2") : samples;
  const hidden = nameList(lang, peopleOf(lang, samples[1]?.with ?? [])).text;
  return (
    <Column>
      <ScreenStack>
        {state === "hidden" && <FeedNotice start>{w.hidden(hidden)}</FeedNotice>}
        <Feed
          label={w.label}
          count={shown.length}
          more={false}
          busy={false}
          end={shown.length === 0 ? <FeedNotice>{w.empty}</FeedNotice> : undefined}
        >
          {shown.map((s, i) => (
            <FeedItem key={s.id} id={s.id} position={i + 1}>
              <Conversation sample={s} lang={lang} />
            </FeedItem>
          ))}
        </Feed>
      </ScreenStack>
    </Column>
  );
}

/**
 * A conversation with `with` (indexes of the people), its posts by turns from `texts`: the people's in turn, and every
 * second one the reader's, marked as theirs; then the reply in place.
 */
function Thread({ with: indexes, texts }: { with: number[]; texts: "thread" | "group" }) {
  const lang = useLang();
  const w = words[lang];
  const others = peopleOf(lang, indexes);
  const names = nameList(lang, others);
  const me = reader[lang];
  const list = w[texts];
  const posts = list.map((text, i) => ({
    id: `p${String(i)}`,
    by: i % 2 === 1 ? undefined : others[(i / 2) % others.length],
    text,
    date: ago((list.length - i) * 40 * minute),
  }));
  return (
    <Column>
      <ScreenStack>
        <ConversationHead
          back={<BackButton onPress={() => undefined}>{w.back}</BackButton>}
          faces={others.map(face)}
          more={<More w={w} />}
        >
          {names.node}
        </ConversationHead>
        <Feed label={w.with(names.text)} count={posts.length} more={false} busy={false} tight>
          {posts.map((p, i) => {
            const by = p.by ?? me;
            return (
              <FeedItem key={p.id} id={p.id} position={i + 1}>
                <Entry labelledBy={`${p.id}-name`}>
                  <Byline
                    avatar={<Avatar name={by.name} src={by.avatar} size="post" decorative />}
                    name={by.name}
                    nameId={`${p.id}-name`}
                    handle={by.handle}
                    self={p.by === undefined ? w.you : undefined}
                    end={<Stamp glyph={{ icon: "direct", label: w.direct }} date={p.date} />}
                  />
                  <Box as="p" dir="auto">
                    {p.text}
                  </Box>
                  <ActionRow end={<IconButton icon="more" label={w.more} />}>
                    <IconButton icon="reply" label={w.reply} />
                    <IconButton icon="favourite" label={w.favourite} />
                    <IconButton icon="react" label={w.react} />
                  </ActionRow>
                </Entry>
              </FeedItem>
            );
          })}
        </Feed>
        <ConversationReply
          label={w.form(names.text)}
          to={w.to(names.text)}
          visibility={w.direct}
          tools={
            <>
              <IconButton icon="attach" label={w.attach} />
              <IconButton icon="emojiSmileys" label={w.emoji} />
            </>
          }
          count={<CharCount left={500} near={20} label="500" />}
          action={
            <Button kind="primary" size="timeline" aria-label={w.sendTo(names.text)}>
              {w.send}
            </Button>
          }
        >
          <ComposerText roomy={false}>
            <PlainTextEditor
              namespace="story"
              look="composer"
              value=""
              onChange={() => undefined}
              label={w.text}
              placeholder={w.placeholder}
            />
          </ComposerText>
        </ConversationReply>
      </ScreenStack>
    </Column>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Direct messages", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Conversations: Story = { render: () => <List /> };
export const Empty: Story = { render: () => <List state="empty" /> };
export const Hidden: Story = { render: () => <List state="hidden" /> };
export const ConversationOpen: Story = { render: () => <Thread with={[0]} texts="thread" /> };
export const ConversationGroup: Story = { render: () => <Thread with={[3, 4, 5, 6, 2]} texts="group" /> };
