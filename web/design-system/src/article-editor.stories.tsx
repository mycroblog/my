// SPDX-License-Identifier: AGPL-3.0-only

// The article editor (#3091), a screen of its own, in grey, a story per state: the writing and the preview side by
// side from `desktop`, Write | Preview under it; the summary, the cover and the hashtags after the body under their
// heading; the settings in the row, as the composer's are. The words are placeholders, in the toolbar's locale.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { useLocale } from "react-aria-components";
import {
  ArticleDetails,
  ArticleField,
  ArticleFrame,
  ArticleLimits,
  ArticleMode,
  ArticlePane,
  ArticlePreview,
  ArticleTextArea,
  ArticleTitleInput,
  CoverRemove,
  CoverRow,
  TagInput,
} from "./article-editor";
import { Button } from "./button";
import { CharCount, ComposerText, ToolText } from "./composer";
import { PlainTextEditor } from "./editor";
import { CaptionLine } from "./entry";
import { FormAlert, FormText, GlyphLabel } from "./form-page";
import { IconButton } from "./icon-button";
import { images } from "./samples";

type Lang = "en" | "ja" | "ar";
const useLang = (): Lang => {
  const language = new Intl.Locale(useLocale().locale).language;
  return language === "ja" ? "ja" : language === "ar" ? "ar" : "en";
};

const n = (lang: Lang, v: number) => v.toLocaleString(lang);

const words = {
  en: {
    newTitle: "New article",
    editTitle: "Edit article",
    close: "Close",
    drafts: "Drafts",
    mode: "Write or preview",
    write: "Write",
    preview: "Preview",
    previewCaption: "As it will be published",
    titleLabel: "Title",
    titlePlaceholder: "Title",
    title: "A month of running a small server",
    longTitle:
      "A month of running a small server for a neighbourhood of friends, what it cost, what broke, what we would do again, the three rules we wrote and changed, and why the timeline is slow on purpose, with notes for anyone who wants to start one",
    titleOver: "The title must be at most 200 characters.",
    details: "About the article",
    summary: "Summary",
    summaryHint: "If left empty, the start of the body is used.",
    summaryPlaceholder: "What the article is about, in a line or two",
    summaryText: "Notes from the first month, for anyone who is thinking about starting one.",
    summaryOver: "The summary must be at most 500 characters.",
    cover: "Cover image",
    addCover: "Add a cover image",
    coverAlt: "Snow on a ridge under a pale sky",
    description: "Description",
    descriptionHint: "What the image shows, for those who cannot see it",
    remove: "Remove the cover image",
    tags: "Hashtags",
    tagsPlaceholder: "Add a hashtag",
    tagsFull: "No more hashtags",
    tagList: ["selfhosting", "fediverse", "community"],
    tagFull: [
      "selfhosting",
      "fediverse",
      "community",
      "server",
      "notes",
      "meetup",
      "rules",
      "hardware",
      "backup",
      "winter",
    ],
    removeTag: (t: string) => `Remove #${t}`,
    body: "Body",
    bodyPlaceholder: "Write in Markdown…",
    source:
      "## The hardware\n\nAn old laptop and a backup drive did the job. The setup took **one evening**.\n\n![The rack in the hallway closet](upload:rack.jpg)\n\n## The rules\n\n1. Be kind.\n2. Write down why.\n3. Keep the rules short.\n\n> The timeline is slow, and that is the point.",
    emptyBody: "Nothing written yet. Write the body, and its preview shows here.",
    rendered: {
      h1: "The hardware",
      p1: "An old laptop and a backup drive did the job. The setup took ",
      p1End: ".",
      strong: "one evening",
      alt: "The rack in the hallway closet",
      h2: "The rules",
      items: ["Be kind.", "Write down why.", "Keep the rules short."],
      quote: "The timeline is slow, and that is the point.",
    },
    bytes: (used: number, max: number) =>
      `Length: ${(used / max).toLocaleString("en", { style: "percent" })} of the limit`,
    imageCount: (used: number, max: number) => `${n("en", used)} of ${n("en", max)} images`,
    attach: "Add an image to the body",
    visibility: "Visibility: Public",
    sensitive: "Mark as sensitive",
    schedule: "Schedule",
    language: "Language: English",
    languageName: "English",
    publish: "Publish",
    save: "Save",
    scheduleAction: "Schedule",
    scheduledAt: "Publishes tomorrow at 9:00 (Tokyo time)",
    change: "Change",
    editFixed: "Stays Public, as published",
    overAlert: "Part of the article is over its limit. Shorten it or take out images to publish.",
    notAllowed:
      "Your account cannot write new articles on this server. You can still edit the articles you wrote before.",
    left: (k: number) => (k < 0 ? `${String(-k)} characters over` : `${String(k)} characters left`),
  },
  ja: {
    newTitle: "新しい記事",
    editTitle: "記事を編集",
    close: "閉じる",
    drafts: "下書き",
    mode: "書く・プレビュー",
    write: "書く",
    preview: "プレビュー",
    previewCaption: "公開したときの表示",
    titleLabel: "タイトル",
    titlePlaceholder: "タイトル",
    title: "小さなサーバーを一か月動かしてみて",
    longTitle:
      "小さなサーバーを一か月動かしてみての覚え書き。機材はいちばん簡単だった。古いノートパソコンとバックアップ用のディスクで足りて、設定はひと晩で終わった。時間がかかったのは、どんな場所にしたいかを決めることのほう。ルールを三つ書いて、最初のオフ会で読み上げて、一週間後に二つ書き直した。タイムラインはゆっくり流れるけれど、それでいい。みんな発信するより返事をしていて、トレンドの画面がほしいと言った人はまだいない。",
    titleOver: "タイトルは 200 文字以内にしてください。",
    details: "記事について",
    summary: "要約",
    summaryHint: "空欄のときは、本文の書き出しが使われます。",
    summaryPlaceholder: "記事の内容を一、二行で",
    summaryText: "最初の一か月の覚え書き。これから立てようと思っている人のために。",
    summaryOver: "要約は 500 文字以内にしてください。",
    cover: "カバー画像",
    addCover: "カバー画像を追加",
    coverAlt: "淡い空の下、雪の積もった尾根",
    description: "説明",
    descriptionHint: "見えない人のために、画像に写っているもの",
    remove: "カバー画像を外す",
    tags: "ハッシュタグ",
    tagsPlaceholder: "ハッシュタグを追加",
    tagsFull: "これ以上は付けられません",
    tagList: ["自宅サーバー", "fediverse", "コミュニティ"],
    tagFull: [
      "自宅サーバー",
      "fediverse",
      "コミュニティ",
      "サーバー",
      "覚え書き",
      "オフ会",
      "ルール",
      "機材",
      "バックアップ",
      "冬",
    ],
    removeTag: (t: string) => `#${t} を外す`,
    body: "本文",
    bodyPlaceholder: "Markdown で書く…",
    source:
      "## 機材\n\n古いノートパソコンとバックアップ用のディスクで足りた。設定は**ひと晩**で終わった。\n\n![廊下の物入れのラック](upload:rack.jpg)\n\n## ルール\n\n1. 親切に。\n2. なぜかを書いておく。\n3. ルールは短く。\n\n> タイムラインはゆっくり流れるけれど、それでいい。",
    emptyBody: "まだ何も書かれていません。本文を書くと、ここにプレビューが表示されます。",
    rendered: {
      h1: "機材",
      p1: "古いノートパソコンとバックアップ用のディスクで足りた。設定は",
      p1End: "で終わった。",
      strong: "ひと晩",
      alt: "廊下の物入れのラック",
      h2: "ルール",
      items: ["親切に。", "なぜかを書いておく。", "ルールは短く。"],
      quote: "タイムラインはゆっくり流れるけれど、それでいい。",
    },
    bytes: (used: number, max: number) => `長さ: 上限の ${(used / max).toLocaleString("ja", { style: "percent" })}`,
    imageCount: (used: number, max: number) => `画像 ${n("ja", used)} / ${n("ja", max)}`,
    attach: "本文に画像を追加",
    visibility: "公開範囲: 公開",
    sensitive: "センシティブにする",
    schedule: "予約",
    language: "言語: 日本語",
    languageName: "日本語",
    publish: "公開",
    save: "保存",
    scheduleAction: "予約",
    scheduledAt: "明日 9:00（東京）に公開",
    change: "変更",
    editFixed: "公開範囲は公開のまま",
    overAlert: "記事の一部が上限を超えています。短くするか画像を減らすと公開できます。",
    notAllowed: "このサーバーでは、このアカウントで新しい記事を書けません。これまでに書いた記事は編集できます。",
    left: (k: number) => (k < 0 ? `${String(-k)} 文字超過` : `残り ${String(k)} 文字`),
  },
  ar: {
    newTitle: "مقالة جديدة",
    editTitle: "تعديل المقالة",
    close: "إغلاق",
    drafts: "المسودات",
    mode: "الكتابة أو المعاينة",
    write: "اكتب",
    preview: "معاينة",
    previewCaption: "كما ستُنشر",
    titleLabel: "العنوان",
    titlePlaceholder: "العنوان",
    title: "شهر من تشغيل خادم صغير",
    longTitle:
      "شهر من تشغيل خادم صغير لحيّ من الأصدقاء، كم كلّف، وما الذي تعطّل، وما الذي سنفعله مجددًا، والقواعد الثلاث التي كتبناها ثم غيّرناها، ولماذا الخط الزمني بطيء عن قصد، مع ملاحظات لكل من يريد أن يبدأ واحدًا",
    titleOver: "حتى 200 حرف.",
    details: "عن المقالة",
    summary: "الملخص",
    summaryHint: "إن تُرك فارغًا، يُصنع من بداية النص.",
    summaryPlaceholder: "سطر أو سطران يقودان إلى المقالة",
    summaryText: "ملاحظات من الشهر الأول، لمن يفكر في البدء.",
    summaryOver: "حتى 500 حرف.",
    cover: "الغلاف",
    addCover: "أضف صورة غلاف",
    coverAlt: "ثلج على قمة تحت سماء شاحبة",
    description: "الوصف",
    descriptionHint: "صِفها لمن لا يراها",
    remove: "إزالة الغلاف",
    tags: "الوسوم",
    tagsPlaceholder: "أضف وسمًا",
    tagsFull: "لا مزيد من الوسوم",
    tagList: ["استضافة_ذاتية", "fediverse", "مجتمع"],
    tagFull: ["استضافة_ذاتية", "fediverse", "مجتمع", "خادم", "ملاحظات", "لقاء", "قواعد", "عتاد", "نسخ", "شتاء"],
    removeTag: (t: string) => `إزالة #${t}`,
    body: "النص",
    bodyPlaceholder: "اكتب بـ Markdown…",
    source:
      "## العتاد\n\nحاسوب قديم وقرص احتياطي أدّيا المهمة. استغرق الإعداد **أمسية واحدة**.\n\n![الرف في خزانة الممر](upload:rack.jpg)\n\n## القواعد\n\n1. كن لطيفًا.\n2. اكتب السبب.\n3. اجعل القواعد قصيرة.\n\n> الخط الزمني بطيء، وهذا هو المقصود.",
    emptyBody: "لم يُكتب شيء بعد.",
    rendered: {
      h1: "العتاد",
      p1: "حاسوب قديم وقرص احتياطي أدّيا المهمة. استغرق الإعداد ",
      p1End: ".",
      strong: "أمسية واحدة",
      alt: "الرف في خزانة الممر",
      h2: "القواعد",
      items: ["كن لطيفًا.", "اكتب السبب.", "اجعل القواعد قصيرة."],
      quote: "الخط الزمني بطيء، وهذا هو المقصود.",
    },
    bytes: (used: number, max: number) => `${n("ar", used)} من ${n("ar", max)} بايت`,
    imageCount: (used: number, max: number) => `${n("ar", used)} من ${n("ar", max)} صور`,
    attach: "أضف صورة إلى النص",
    visibility: "الظهور: عام",
    sensitive: "وسم كحساس",
    schedule: "جدولة",
    language: "اللغة: العربية",
    languageName: "العربية",
    publish: "انشر",
    save: "احفظ",
    scheduleAction: "جدولة",
    scheduledAt: "يُنشر غدًا الساعة 9:00 (بتوقيت طوكيو)",
    change: "تغيير",
    editFixed: "يبقى عامًا كما نُشر",
    overAlert: "اختصر ما تجاوز حده لتنشر.",
    notAllowed: "على هذا الخادم، يكتب المقالات من تسمح لهم أدوارهم. ما زال بإمكانك تعديل مقالاتك السابقة.",
    left: (k: number) => (k < 0 ? `${String(-k)} حرفًا زائدًا` : `${String(k)} حرفًا متبقيًا`),
  },
};
type Words = (typeof words)["en"];

type State = "empty" | "writing" | "preview" | "over" | "scheduled" | "edit" | "not-allowed";

/** What the state holds, written. */
const holds = (state: State, w: Words) => {
  const written = state !== "empty";
  const over = state === "over";
  return {
    title: !written ? "" : over ? w.longTitle : w.title,
    summary: !written ? "" : over ? `${w.longTitle} `.repeat(3).trim() : w.summaryText,
    cover: written,
    tags: !written ? [] : over ? w.tagFull : w.tagList,
    body: written ? w.source : "",
    bytes: !written ? 0 : over ? 104_211 : 1_486,
    images: !written ? 0 : over ? 21 : 1,
  };
};

/** The body as the server renders it (the preview's HTML), for the sample's source. */
function Rendered({ w }: { w: Words }) {
  const r = w.rendered;
  return (
    <>
      <h2>{r.h1}</h2>
      <p>
        {r.p1}
        <strong>{r.strong}</strong>
        {r.p1End}
      </p>
      <p>
        <img src={images.snow} alt={r.alt} width={512} height={512} />
      </p>
      <h2>{r.h2}</h2>
      <ol>
        {r.items.map((item) => (
          <li key={item}>{item}</li>
        ))}
      </ol>
      <blockquote>
        <p>{r.quote}</p>
      </blockquote>
    </>
  );
}

function Article({ state }: { state: State }) {
  const lang = useLang();
  const w = words[lang];
  const h = holds(state, w);
  const [mode, setMode] = useState<"write" | "preview">(state === "preview" ? "preview" : "write");
  const editing = state === "edit";
  const titleLeft = 200 - Array.from(h.title).length;
  const summaryLeft = 500 - Array.from(h.summary).length;

  if (state === "not-allowed")
    return (
      <ArticleFrame title={w.newTitle} closeLabel={w.close} onClose={() => undefined}>
        <FormText>{w.notAllowed}</FormText>
      </ArticleFrame>
    );

  const title = (
    <ArticleTitleInput
      id="article-title"
      label={w.titleLabel}
      placeholder={w.titlePlaceholder}
      value={h.title}
      count={titleLeft <= 20 ? <CharCount left={titleLeft} near={20} label={w.left(titleLeft)} /> : undefined}
      error={titleLeft < 0 ? w.titleOver : undefined}
    />
  );
  const summary = (
    <ArticleField
      id="article-summary"
      label={w.summary}
      hint={w.summaryHint}
      end={summaryLeft <= 50 ? <CharCount left={summaryLeft} near={50} label={w.left(summaryLeft)} /> : undefined}
      error={summaryLeft < 0 ? w.summaryOver : undefined}
    >
      <ArticleTextArea
        label={w.summary}
        placeholder={w.summaryPlaceholder}
        value={h.summary}
        invalid={summaryLeft < 0}
      />
    </ArticleField>
  );
  const cover = (
    <ArticleField id="article-cover" label={w.cover} end={h.cover ? <CoverRemove label={w.remove} /> : undefined}>
      {h.cover ? (
        <CoverRow
          id="article-cover-description"
          src={images.snow}
          description={w.coverAlt}
          descriptionLabel={w.description}
          placeholder={w.descriptionHint}
        />
      ) : (
        <span>
          <Button kind="secondary" size="timeline">
            <GlyphLabel icon="attach">{w.addCover}</GlyphLabel>
          </Button>
        </span>
      )}
    </ArticleField>
  );
  const full = h.tags.length >= 10;
  const tags = (
    <ArticleField
      id="article-tags"
      label={w.tags}
      end={h.tags.length >= 8 ? <CharCount left={10 - h.tags.length} near={2} label="" /> : undefined}
    >
      <TagInput
        label={w.tags}
        items={h.tags}
        removeLabel={w.removeTag}
        placeholder={full ? w.tagsFull : w.tagsPlaceholder}
      />
    </ArticleField>
  );
  const body = (
    <ComposerText>
      <PlainTextEditor
        namespace="article"
        look="composer"
        value={h.body}
        onChange={() => undefined}
        label={w.body}
        placeholder={w.bodyPlaceholder}
        roomy
      />
    </ComposerText>
  );
  const writing = (
    <>
      {title}
      {body}
      <ArticleDetails id="article-details" heading={w.details}>
        {summary}
        {cover}
        {tags}
      </ArticleDetails>
    </>
  );
  const shown = (
    <ArticlePreview
      title={h.title}
      summary={h.summary === "" ? undefined : h.summary}
      cover={h.cover ? { src: images.snow, alt: w.coverAlt } : undefined}
      empty={w.emptyBody}
      lang={lang}
    >
      {h.body === "" ? undefined : <Rendered w={w} />}
    </ArticlePreview>
  );
  const modes = [
    { id: "write" as const, name: w.write },
    { id: "preview" as const, name: w.preview },
  ];
  const limitState = (used: number, max: number) => (used > max ? "over" : used >= max * 0.9 ? "near" : "ok");
  return (
    <ArticleFrame
      title={editing ? w.editTitle : w.newTitle}
      closeLabel={w.close}
      onClose={() => undefined}
      headEnd={editing ? undefined : <IconButton icon="drafts" label={w.drafts} />}
      preview={{ caption: w.previewCaption, children: shown }}
      limits={
        <ArticleLimits
          items={[
            {
              id: "bytes",
              text: w.bytes(h.bytes, 100_000),
              state: limitState(h.bytes, 100_000),
            },
            {
              id: "images",
              text: w.imageCount(h.images, 20),
              state: limitState(h.images, 20),
            },
          ]}
        />
      }
      note={
        state === "scheduled" ? (
          <CaptionLine icon="schedule" end={<span>{w.change}</span>}>
            {w.scheduledAt}
          </CaptionLine>
        ) : editing ? (
          <CaptionLine icon="public">{w.editFixed}</CaptionLine>
        ) : undefined
      }
      alert={state === "over" ? <FormAlert>{w.overAlert}</FormAlert> : undefined}
      tools={
        <>
          <IconButton icon="attach" label={w.attach} />
          {!editing && <IconButton icon="public" label={w.visibility} />}
          <IconButton icon="hidden" label={w.sensitive} />
          {!editing && <IconButton icon="schedule" label={w.schedule} pressed={state === "scheduled"} />}
          <ToolText label={w.language} expanded={false} onPress={() => undefined} short={lang.toUpperCase()}>
            {w.languageName}
          </ToolText>
        </>
      }
      action={
        <Button kind="primary" size="timeline">
          {editing ? w.save : state === "scheduled" ? w.scheduleAction : w.publish}
        </Button>
      }
    >
      <ArticleMode label={w.mode} items={modes} current={mode} onChange={setMode} />
      <ArticlePane shown={mode === "write" ? "always" : "desktop"}>{writing}</ArticlePane>
      <ArticlePane shown={mode === "preview" ? "narrow" : "never"}>{shown}</ArticlePane>
    </ArticleFrame>
  );
}

// Tagged `themes`: the story tests run each in dark too.
const meta = {
  title: "Article editor",
  tags: ["themes"],
  parameters: { layout: "fullscreen" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Nothing written: the title's and the body's placeholders, the limits at nothing used. */
export const Empty: Story = { render: () => <Article state="empty" /> };
/** Written: a title, a summary, a cover, three hashtags, and a body with headings, an image, a list and a quote. */
export const Writing: Story = { render: () => <Article state="writing" /> };
/** Preview chosen: the article as the server renders it. */
export const Preview: Story = { render: () => <Article state="preview" /> };
/** Over every limit: the title, the summary, the bytes and the images over, the hashtags full. */
export const OverLimits: Story = { render: () => <Article state="over" /> };
/** A time set: one line over the row says when it is published, and Schedule takes Publish's place. */
export const Scheduled: Story = { render: () => <Article state="scheduled" /> };
/** Editing a published article: Edit article and Save; the visibility stays, said over the row. */
export const Edit: Story = { render: () => <Article state="edit" /> };
/** The server keeps articles to a role the reader does not have: no form, the one sentence why. */
export const NotAllowed: Story = {
  render: () => <Article state="not-allowed" />,
};
export const RightToLeft: Story = {
  render: () => <Article state="writing" />,
  globals: { locale: "ar-XB" },
};
export const MinimumWidth: Story = {
  render: () => <Article state="writing" />,
  globals: { viewport: { value: "minimum" } },
};
