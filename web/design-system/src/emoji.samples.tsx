// SPDX-License-Identifier: AGPL-3.0-only

// The emoji of the stories' pickers: a few of each Unicode group, in the order of emoji-test.txt, with an English name
// and keywords and the skin tones of those that have them, and a server's custom emoji, generated tiles, two of them in
// a category and one with aliases. The app generates the full set from Unicode and CLDR (ADR-0091 §1); these stand in
// for it. `pickerStrings` are placeholders the copy settles; `pickerSections` and `searchPicker` build what the picker
// takes.

import type { EmojiPickerLabels, EmojiSection, PickerEmoji } from "./emoji-picker";

type Unicode = { text: string; name: string; keywords?: string; toned?: boolean };

const tones = ["🏻", "🏼", "🏽", "🏾", "🏿"];

/** The emoji with each tone after its first code point (and before a ZWJ sequence's rest). */
const toned = (text: string) => {
  const [first = "", ...rest] = Array.from(text.replace("️", ""));
  return tones.map((t) => first + t + rest.join(""));
};

/** Every Unicode emoji of the samples, as written below, for the search. */
const unicodeList: Unicode[] = [];

const unicode = (list: readonly Unicode[]): PickerEmoji[] => {
  unicodeList.push(...list);
  return list.map((e) => ({ key: e.text, name: e.name, text: e.text, ...(e.toned ? { tones: toned(e.text) } : {}) }));
};

const groups = {
  smileys: unicode([
    { text: "😀", name: "grinning face", keywords: "smile happy" },
    { text: "😃", name: "grinning face with big eyes", keywords: "smile happy" },
    { text: "😄", name: "grinning face with smiling eyes", keywords: "smile happy" },
    { text: "😁", name: "beaming face", keywords: "grin" },
    { text: "😆", name: "grinning squinting face", keywords: "laugh" },
    { text: "😅", name: "grinning face with sweat", keywords: "relief" },
    { text: "🤣", name: "rolling on the floor laughing", keywords: "laugh lol" },
    { text: "😂", name: "face with tears of joy", keywords: "laugh lol" },
    { text: "🙂", name: "slightly smiling face", keywords: "smile" },
    { text: "😉", name: "winking face", keywords: "wink" },
    { text: "😊", name: "smiling face with smiling eyes", keywords: "blush" },
    { text: "😇", name: "smiling face with halo", keywords: "angel" },
    { text: "🥰", name: "smiling face with hearts", keywords: "love" },
    { text: "😍", name: "smiling face with heart-eyes", keywords: "love" },
    { text: "🤩", name: "star-struck", keywords: "wow" },
    { text: "😘", name: "face blowing a kiss", keywords: "love" },
    { text: "😋", name: "face savoring food", keywords: "yum delicious" },
    { text: "🤔", name: "thinking face", keywords: "think hmm" },
    { text: "🫡", name: "saluting face", keywords: "ok yes" },
    { text: "🤗", name: "smiling face with open hands", keywords: "hug" },
    { text: "😴", name: "sleeping face", keywords: "sleep zzz" },
    { text: "😮", name: "face with open mouth", keywords: "wow surprise" },
    { text: "🥲", name: "smiling face with tear", keywords: "grateful" },
    { text: "😭", name: "loudly crying face", keywords: "cry sad" },
    { text: "😱", name: "face screaming in fear", keywords: "scream" },
    { text: "😤", name: "face with steam from nose", keywords: "triumph" },
    { text: "🥺", name: "pleading face", keywords: "please" },
    { text: "🙏", name: "folded hands", keywords: "thanks please", toned: true },
  ]),
  people: unicode([
    { text: "👋", name: "waving hand", keywords: "hello bye", toned: true },
    { text: "👍", name: "thumbs up", keywords: "like yes +1", toned: true },
    { text: "👎", name: "thumbs down", keywords: "no -1", toned: true },
    { text: "👏", name: "clapping hands", keywords: "applause", toned: true },
    { text: "🙌", name: "raising hands", keywords: "hooray", toned: true },
    { text: "🤝", name: "handshake", keywords: "agree" },
    { text: "✌️", name: "victory hand", keywords: "peace", toned: true },
    { text: "🤞", name: "crossed fingers", keywords: "luck", toned: true },
    { text: "👌", name: "OK hand", keywords: "ok", toned: true },
    { text: "💪", name: "flexed biceps", keywords: "strong", toned: true },
    { text: "👀", name: "eyes", keywords: "look" },
    { text: "🧠", name: "brain", keywords: "smart" },
    { text: "🙇", name: "person bowing", keywords: "sorry thanks", toned: true },
    { text: "🙆", name: "person gesturing OK", keywords: "ok", toned: true },
    { text: "🤷", name: "person shrugging", keywords: "shrug", toned: true },
  ]),
  nature: unicode([
    { text: "🐱", name: "cat face", keywords: "cat" },
    { text: "🐶", name: "dog face", keywords: "dog" },
    { text: "🦊", name: "fox", keywords: "fox" },
    { text: "🐻", name: "bear", keywords: "bear" },
    { text: "🐼", name: "panda", keywords: "panda" },
    { text: "🐧", name: "penguin", keywords: "bird" },
    { text: "🦉", name: "owl", keywords: "bird" },
    { text: "🐢", name: "turtle", keywords: "slow" },
    { text: "🌸", name: "cherry blossom", keywords: "flower sakura" },
    { text: "🌻", name: "sunflower", keywords: "flower" },
    { text: "🍁", name: "maple leaf", keywords: "autumn" },
    { text: "🌙", name: "crescent moon", keywords: "night" },
    { text: "⭐", name: "star", keywords: "star" },
    { text: "🔥", name: "fire", keywords: "hot lit" },
    { text: "🌈", name: "rainbow", keywords: "rainbow" },
  ]),
  food: unicode([
    { text: "🍎", name: "red apple", keywords: "fruit" },
    { text: "🍑", name: "peach", keywords: "fruit" },
    { text: "🍙", name: "rice ball", keywords: "onigiri" },
    { text: "🍜", name: "steaming bowl", keywords: "ramen noodle" },
    { text: "🍣", name: "sushi", keywords: "sushi" },
    { text: "🍰", name: "shortcake", keywords: "cake" },
    { text: "🍩", name: "doughnut", keywords: "donut" },
    { text: "☕", name: "hot beverage", keywords: "coffee tea" },
    { text: "🍵", name: "teacup without handle", keywords: "tea" },
    { text: "🍺", name: "beer mug", keywords: "beer" },
    { text: "🍻", name: "clinking beer mugs", keywords: "cheers" },
  ]),
  travel: unicode([
    { text: "🚃", name: "railway car", keywords: "train" },
    { text: "🚲", name: "bicycle", keywords: "bike" },
    { text: "✈️", name: "airplane", keywords: "travel" },
    { text: "🚀", name: "rocket", keywords: "launch ship" },
    { text: "🏠", name: "house", keywords: "home" },
    { text: "🗻", name: "mount fuji", keywords: "mountain" },
    { text: "🌃", name: "night with stars", keywords: "city" },
    { text: "🌧️", name: "cloud with rain", keywords: "rain" },
  ]),
  activities: unicode([
    { text: "🎉", name: "party popper", keywords: "party tada celebrate" },
    { text: "🎊", name: "confetti ball", keywords: "party" },
    { text: "🎂", name: "birthday cake", keywords: "birthday" },
    { text: "🎁", name: "wrapped gift", keywords: "present" },
    { text: "⚽", name: "soccer ball", keywords: "football" },
    { text: "🎮", name: "video game", keywords: "game" },
    { text: "🎨", name: "artist palette", keywords: "art" },
    { text: "🎵", name: "musical note", keywords: "music" },
  ]),
  objects: unicode([
    { text: "📚", name: "books", keywords: "read library" },
    { text: "📷", name: "camera", keywords: "photo" },
    { text: "💡", name: "light bulb", keywords: "idea" },
    { text: "📌", name: "pushpin", keywords: "pin" },
    { text: "🔧", name: "wrench", keywords: "fix tool" },
    { text: "💻", name: "laptop", keywords: "computer" },
    { text: "📝", name: "memo", keywords: "note write" },
    { text: "🔔", name: "bell", keywords: "notification" },
  ]),
  symbols: unicode([
    { text: "❤️", name: "red heart", keywords: "love" },
    { text: "🧡", name: "orange heart", keywords: "love" },
    { text: "💛", name: "yellow heart", keywords: "love" },
    { text: "💚", name: "green heart", keywords: "love" },
    { text: "💙", name: "blue heart", keywords: "love" },
    { text: "💜", name: "purple heart", keywords: "love" },
    { text: "💯", name: "hundred points", keywords: "100 perfect" },
    { text: "✅", name: "check mark button", keywords: "done yes" },
    { text: "❗", name: "red exclamation mark", keywords: "important" },
    { text: "❓", name: "red question mark", keywords: "question" },
    { text: "♻️", name: "recycling symbol", keywords: "recycle" },
  ]),
  flags: unicode([
    { text: "🏁", name: "chequered flag", keywords: "finish" },
    { text: "🏳️‍🌈", name: "rainbow flag", keywords: "pride" },
    { text: "🇯🇵", name: "flag: Japan", keywords: "japan" },
    { text: "🇺🇸", name: "flag: United States", keywords: "usa" },
    { text: "🇪🇺", name: "flag: European Union", keywords: "eu" },
  ]),
};

const svg = (body: string) =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="128" height="128">${body}</svg>`)}`;

/** A server's custom emoji: its shortcode, its aliases and its category (none for the first ones). */
const custom: { shortcode: string; aliases?: string[]; category?: string; body: string }[] = [
  {
    shortcode: "sparkle",
    aliases: ["kirakira", "shine"],
    body: `<path d="M16 2 L19 13 L30 16 L19 19 L16 30 L13 19 L2 16 L13 13Z" fill="#ffc94a"/><path d="M26 3 L27 6 L30 7 L27 8 L26 11 L25 8 L22 7 L25 6Z" fill="#ffe08a"/>`,
  },
  {
    shortcode: "mycro",
    aliases: ["logo"],
    body: `<rect width="32" height="32" rx="7" fill="#ff6b6b"/><path d="M8 22 V12 Q12 9 16 13 V22 M16 13 Q20 9 24 12 V22" stroke="#111" stroke-width="2.6" fill="none" stroke-linecap="round"/>`,
  },
  {
    shortcode: "party",
    body: `<path d="M5 28 L13 8 L24 19Z" fill="#7a5cff"/><path d="M8 21 L18 13" stroke="#ffd34d" stroke-width="2.4"/><circle cx="24" cy="7" r="2.4" fill="#ff6b6b"/><circle cx="27" cy="14" r="1.8" fill="#4cc38a"/><circle cx="19" cy="4" r="1.6" fill="#ffd34d"/>`,
  },
  {
    shortcode: "blobcat",
    aliases: ["neko"],
    category: "Blobs",
    body: `<path d="M5 12 L8 3 L13 9 L19 9 L24 3 L27 12 Q30 28 16 28 Q2 28 5 12Z" fill="#f2c14e"/><circle cx="12" cy="16" r="2" fill="#3a2a10"/><circle cx="20" cy="16" r="2" fill="#3a2a10"/><path d="M14 21 Q16 23 18 21" stroke="#3a2a10" stroke-width="1.6" fill="none"/>`,
  },
  {
    shortcode: "blobwave",
    category: "Blobs",
    body: `<circle cx="16" cy="16" r="14" fill="#ffcf5a"/><path d="M9 18 Q16 25 23 18" stroke="#6b4a1f" stroke-width="2.4" fill="none" stroke-linecap="round"/><circle cx="11" cy="12" r="2" fill="#6b4a1f"/><circle cx="21" cy="12" r="2" fill="#6b4a1f"/>`,
  },
  {
    shortcode: "blobheart",
    category: "Blobs",
    body: `<circle cx="16" cy="17" r="13" fill="#ffcf5a"/><path d="M10 13 q2 -3 4 0 q2 -3 4 0 q0 3 -4 6 q-4 -3 -4 -6Z" fill="#e5484d" transform="translate(0 1)"/><path d="M11 22 Q16 26 21 22" stroke="#6b4a1f" stroke-width="2" fill="none" stroke-linecap="round"/>`,
  },
  {
    shortcode: "ok_sign",
    aliases: ["ok", "yes"],
    category: "Signs",
    body: `<rect x="2" y="6" width="28" height="20" rx="5" fill="#2f9e6a"/><text x="16" y="21" text-anchor="middle" font-family="sans-serif" font-weight="700" font-size="12" fill="#fff">OK</text>`,
  },
  {
    shortcode: "wip",
    category: "Signs",
    body: `<rect x="2" y="6" width="28" height="20" rx="5" fill="#f2a516"/><text x="16" y="21" text-anchor="middle" font-family="sans-serif" font-weight="700" font-size="11" fill="#111">WIP</text>`,
  },
];

const customEmoji = (e: (typeof custom)[number]): PickerEmoji => {
  const src = svg(e.body);
  return { key: `:${e.shortcode}:`, name: `:${e.shortcode}:`, image: { url: src, staticUrl: src } };
};

/** A custom emoji of the samples by its shortcode, as a post's reaction shows it. */
export const sampleCustomEmoji = Object.fromEntries(custom.map((e) => [e.shortcode, svg(e.body)]));

/** The reader's frequent ones; none for a reader who has reacted with none yet, whose default row this is. */
const frequent = ["👍", "❤️", "😂", "🎉", "🙏", "😮", "🥲", "👀", ":sparkle:", ":blobcat:"];

type Lang = "en" | "ja";

const sectionNames: Record<Lang, Record<keyof typeof groups | "frequent" | "custom", string>> = {
  en: {
    frequent: "Frequently used",
    custom: "Custom emoji",
    smileys: "Smileys & emotion",
    people: "People & body",
    nature: "Animals & nature",
    food: "Food & drink",
    travel: "Travel & places",
    activities: "Activities",
    objects: "Objects",
    symbols: "Symbols",
    flags: "Flags",
  },
  ja: {
    frequent: "よく使う絵文字",
    custom: "カスタム絵文字",
    smileys: "顔と感情",
    people: "人と体",
    nature: "動物と自然",
    food: "食べ物と飲み物",
    travel: "旅行と場所",
    activities: "アクティビティ",
    objects: "物",
    symbols: "記号",
    flags: "旗",
  },
};

const icons = {
  smileys: "emojiSmileys",
  people: "emojiPeople",
  nature: "emojiNature",
  food: "emojiFood",
  travel: "emojiTravel",
  activities: "emojiActivities",
  objects: "emojiObjects",
  symbols: "emojiSymbols",
  flags: "emojiFlags",
} as const;

// The custom emoji first, as a search lists them (ADR-0091 §3).
const all = [...custom.map(customEmoji), ...Object.values(groups).flat()];
const byKey = new Map(all.map((e) => [e.key, e]));

/** The words a query's start may match: a custom emoji's shortcode and aliases, a Unicode emoji's name and keywords. */
const words = new Map<string, string[]>([
  ...custom.map((e): [string, string[]] => [`:${e.shortcode}:`, [e.shortcode, ...(e.aliases ?? [])]]),
  ...unicodeList.map((e): [string, string[]] => [
    e.text,
    `${e.name} ${e.keywords ?? ""}`.toLowerCase().split(/[\s:]+/),
  ]),
]);

/** The sections of the samples, in `lang`: frequent, the server's custom emoji (unless `noCustom`), the nine groups. */
export function pickerSections(lang: Lang, { noCustom = false }: { noCustom?: boolean } = {}): EmojiSection[] {
  const n = sectionNames[lang];
  const categories = [...new Set(custom.flatMap((e) => (e.category === undefined ? [] : [e.category])))];
  return [
    {
      id: "frequent",
      label: n.frequent,
      icon: "time",
      emojis: frequent.flatMap((k) => (noCustom && k.startsWith(":") ? [] : (byKey.get(k) ?? []))),
    },
    ...(noCustom
      ? []
      : [
          {
            id: "custom",
            label: n.custom,
            icon: "emojiCustom" as const,
            emojis: custom.filter((e) => e.category === undefined).map(customEmoji),
            groups: categories.map((c) => ({
              label: c,
              emojis: custom.filter((e) => e.category === c).map(customEmoji),
            })),
          },
        ]),
    ...(Object.keys(groups) as (keyof typeof groups)[]).map((g) => ({
      id: g,
      label: n[g],
      icon: icons[g],
      emojis: groups[g],
    })),
  ];
}

/** The samples that match `query`: a custom emoji's shortcode or alias, or a Unicode emoji's name or keyword. */
export function searchPicker(query: string): PickerEmoji[] {
  const q = query.toLowerCase().replace(/^:|:$/g, "");
  if (q === "") return [];
  return all.filter((e) => (words.get(e.key) ?? []).some((w) => w.startsWith(q)));
}

/** The skin tone's words in `lang`, as the app gives them: its catalog's, and the tones' names from CLDR. */
export const pickerStrings: Record<Lang, EmojiPickerLabels> = {
  en: {
    tone: "Skin tone",
    tones: ["Default", "Light", "Medium-light", "Medium", "Medium-dark", "Dark"],
  },
  ja: {
    tone: "肌の色",
    tones: ["標準", "明るい", "やや明るい", "中間", "やや暗い", "暗い"],
  },
};
