// SPDX-License-Identifier: AGPL-3.0-only
// Translating a post with the Translator API of the browser (FR-120, ADR-0148 §12): nothing here reaches this server or
// a provider. A post goes in as the fragments of ADR-0148 §8, made as internal/app/translation/fragment.go makes them:
// what must stay as it is (mentions, hashtags, links, :shortcode:, code) is left here with a marker in its place, and
// decoration becomes a pair of markers around its words. Models do not always keep the markers (Chrome's en to ar turns
// them into plain numbers), so a fragment that loses them is translated again piece by piece between the markers, which
// costs the sentence some context but keeps the post whole (ADR-0148, Open Questions). The specification warns that the output may hold markup and is
// no security boundary, and the client has no sanitizer, so the output never becomes HTML: it goes back into the
// structure of the sanitized body as text (PostContentNodes of @my/ui). The API is experimental and absent from the
// DOM's types: https://webmachinelearning.github.io/translation-api/
import { type PostContentElement, type PostContentNode, sanitizedHtmlNodes, serverSanitizedHtml } from "@my/ui";
import { queryOptions } from "@tanstack/react-query";
import { textTokens } from "../../../lib/textTokens";
import type { Status } from "./status";
import { isInLanguage, type TranslatedComment, type TranslatedPost } from "./translation";

/** The part of the Translator and Language Detector APIs used here. */
type Availability = "available" | "downloadable" | "downloading" | "unavailable";
type DownloadMonitor = {
  addEventListener: (type: "downloadprogress", listener: (event: { loaded: number }) => void) => void;
};
type WithMonitor = { monitor?: (monitor: DownloadMonitor) => void };
type LanguagePair = { sourceLanguage: string; targetLanguage: string };
type BuiltInTranslator = { translate: (input: string) => Promise<string>; destroy: () => void };
type BuiltInDetector = {
  detect: (input: string) => Promise<readonly { detectedLanguage: string; confidence: number }[]>;
  destroy: () => void;
};
type BuiltInApis = {
  Translator?: {
    availability: (pair: LanguagePair) => Promise<Availability>;
    create: (options: LanguagePair & WithMonitor) => Promise<BuiltInTranslator>;
  };
  LanguageDetector?: {
    availability: () => Promise<Availability>;
    create: (options: WithMonitor) => Promise<BuiltInDetector>;
  };
};

const builtIn = globalThis as BuiltInApis;

/** Why the browser gave no translation, for the sentence next to the button. */
export type BrowserTranslationFailure =
  "undetected" | "same_language" | "unsupported" | "not_allowed" | "model" | "failed";

export class BrowserTranslationError extends Error {
  readonly reason: BrowserTranslationFailure;

  constructor(reason: BrowserTranslationFailure, options?: ErrorOptions) {
    super(`browser translation: ${reason}`, options);
    this.reason = reason;
  }
}

/** Whether the browser has the Translator API, without asking it about any language. */
export function hasBrowserTranslator(): boolean {
  return builtIn.Translator !== undefined;
}

/** What the browser says about a pair; a tag that it refuses is one that it cannot translate. */
async function pairAvailability(sourceLanguage: string, targetLanguage: string): Promise<Availability> {
  try {
    return (await builtIn.Translator?.availability({ sourceLanguage, targetLanguage })) ?? "unavailable";
  } catch {
    return "unavailable";
  }
}

function isUnknownLanguage(language: string): boolean {
  return language.toLowerCase() === "und";
}

/**
 * Whether the browser may translate a post in `language` into `target`. The language of a post in `und` is only found
 * when the reader asks, since the detector's model may need the reader's gesture to download, so all that can be asked
 * before that is whether the browser has a detector.
 */
async function browserCanTranslate(language: string, target: string): Promise<boolean> {
  if (!isUnknownLanguage(language)) return (await pairAvailability(language, target)) !== "unavailable";
  try {
    return ((await builtIn.LanguageDetector?.availability()) ?? "unavailable") !== "unavailable";
  } catch {
    return false;
  }
}

/** The answer of `browserCanTranslate`, once per pair of languages for all the cards. */
export function browserTranslationQueryOptions(language: string, target: string) {
  return queryOptions({
    queryKey: ["browser-translation", language, target],
    queryFn: () => browserCanTranslate(language, target),
    staleTime: Infinity,
    retry: false,
  });
}

const markerPattern = /⟦\/?[1-9][0-9]*⟧/gu;

function marker(n: number): string {
  return `⟦${String(n)}⟧`;
}

function closingMarker(n: number): string {
  return `⟦/${String(n)}⟧`;
}

/**
 * Whether every marker of `fragment` comes back in `translation` exactly once and no other marker appears, with its pairs
 * in order and nested (MarkersIntact in internal/app/translation/translator.go). Items may move, as word order differs
 * between languages.
 */
export function markersIntact(fragment: string, translation: string): boolean {
  const unseen = new Map<string, boolean>();
  for (const [found] of fragment.matchAll(markerPattern)) {
    if (unseen.has(found)) return false;
    unseen.set(found, true);
  }
  const open: string[] = [];
  for (const [found] of translation.matchAll(markerPattern)) {
    if (unseen.get(found) !== true) return false;
    unseen.set(found, false);
    if (found.startsWith("⟦/")) {
      if (open.pop() !== found) return false;
      continue;
    }
    const closing = `⟦/${found.slice(1)}`;
    if (unseen.has(closing)) open.push(closing);
  }
  return open.length === 0 && ![...unseen.values()].some(Boolean);
}

type MarkerMeaning =
  { kind: "item"; node: PostContentNode } | { kind: "open"; element: PostContentElement } | { kind: "close" };

/** One fragment: its text with markers, and what each marker stands for. */
type MaskedText = { fragment: string; meanings: ReadonlyMap<string, MarkerMeaning>; translation: string };

/** Whether a fragment has words to translate: whitespace, punctuation, numbers, and markers alone stay as they are. */
function sendable({ fragment }: MaskedText): boolean {
  return /\p{L}/u.test(fragment.replaceAll(markerPattern, ""));
}

const blockTags: readonly string[] = ["p", "pre", "blockquote", "ul", "ol", "li"];

/** Whether a node has text to translate, or is one item that stays as it is (hasWords in fragment.go). */
function hasWords(node: PostContentNode): boolean {
  if (typeof node === "string") return node.trim() !== "";
  if (["a", "code", "ruby", "br", ...blockTags].includes(node.tag)) return false;
  return node.children.some(hasWords);
}

function isBlock(node: PostContentNode): node is PostContentElement {
  return typeof node !== "string" && blockTags.includes(node.tag);
}

/** Builds the fragment of a run of inline nodes. The markers number from 1 in each fragment. */
function maskInline(nodes: readonly PostContentNode[]): MaskedText {
  const meanings = new Map<string, MarkerMeaning>();
  let out = "";
  let pending = "";
  let last = 0;

  const item = (node: PostContentNode) => {
    last += 1;
    meanings.set(marker(last), { kind: "item", node });
    out += marker(last);
  };
  /** Text that has the form of a marker could otherwise pass for one of the markers made here. */
  const plain = (text: string) => {
    let from = 0;
    for (const match of text.matchAll(markerPattern)) {
      out += text.slice(from, match.index);
      item(match[0]);
      from = match.index + match[0].length;
    }
    out += text.slice(from);
  };
  /** The pending text, in which the handles, URLs, and shortcodes that no element marks stay too. */
  const flush = () => {
    const text = pending;
    pending = "";
    const core = text.trim();
    const start = text.indexOf(core);
    out += text.slice(0, start);
    let from = 0;
    for (const token of textTokens(core)) {
      plain(core.slice(from, token.start));
      item(core.slice(token.start, token.end));
      from = token.end;
    }
    plain(core.slice(from));
    out += text.slice(start + core.length);
  };
  const inline = (node: PostContentNode) => {
    if (typeof node === "string") {
      pending += node;
      return;
    }
    flush();
    if (!hasWords(node)) {
      item(node);
      return;
    }
    last += 1;
    const n = last;
    meanings.set(marker(n), { kind: "open", element: node });
    meanings.set(closingMarker(n), { kind: "close" });
    out += marker(n);
    node.children.forEach(inline);
    flush();
    out += closingMarker(n);
  };
  nodes.forEach(inline);
  flush();
  return { fragment: out, meanings, translation: out };
}

/**
 * The nodes of a fragment's translation. Each piece between the markers is text and stays text, whatever it holds; the
 * markers were checked by `markersIntact`, so that the pairs nest.
 */
function restore({ translation, meanings }: MaskedText): PostContentNode[] {
  const root: PostContentNode[] = [];
  const stack = [root];
  const add = (node: PostContentNode) => stack.at(-1)?.push(node);
  let from = 0;
  const onMarker = (found: string) => {
    const meaning = meanings.get(found);
    if (meaning?.kind === "item") {
      add(meaning.node);
    } else if (meaning?.kind === "open") {
      const children: PostContentNode[] = [];
      add({ ...meaning.element, children });
      stack.push(children);
    } else if (meaning?.kind === "close") {
      stack.pop();
    }
  };
  for (const match of translation.matchAll(markerPattern)) {
    if (match.index > from) add(translation.slice(from, match.index));
    onMarker(match[0]);
    from = match.index + match[0].length;
  }
  if (from < translation.length) add(translation.slice(from));
  return root;
}

/** The text of a field of plain text after its translation: its items are all text. */
function restoredText(text: MaskedText): string {
  return restore(text)
    .filter((node) => typeof node === "string")
    .join("");
}

/** A part of a masked body: markup that stays, with its fragments inside it, or a fragment. */
type MaskedPart =
  string | { run: MaskedText } | (Omit<PostContentElement, "children"> & { children: readonly MaskedPart[] });

function maskBlocks(nodes: readonly PostContentNode[], texts: MaskedText[]): MaskedPart[] {
  const parts: MaskedPart[] = [];
  let run: PostContentNode[] = [];
  const endRun = () => {
    if (run.every((node) => typeof node === "string" && node.trim() === "")) {
      parts.push(...run);
    } else {
      const text = maskInline(run);
      texts.push(text);
      parts.push({ run: text });
    }
    run = [];
  };
  for (const node of nodes) {
    if (!isBlock(node)) {
      run.push(node);
      continue;
    }
    endRun();
    if (node.tag === "pre") parts.push(node);
    else parts.push({ ...node, children: maskBlocks(node.children, texts) });
  }
  endRun();
  return parts;
}

function restoreParts(parts: readonly MaskedPart[]): PostContentNode[] {
  return parts.flatMap((part): PostContentNode[] => {
    if (typeof part === "string") return [part];
    if ("run" in part) return restore(part.run);
    return [{ ...part, children: restoreParts(part.children) }];
  });
}

/** The monitor of a model that is not there yet; a model that is there has no progress to tell of. */
function monitorFor(availability: Availability, onDownload: (loaded: number) => void): WithMonitor {
  if (availability === "available") return {};
  return {
    monitor: (monitor) => {
      monitor.addEventListener("downloadprogress", ({ loaded }) => {
        onDownload(loaded);
      });
    },
  };
}

/**
 * The failure of making a translator or a detector. Without a gesture a download is refused with a NotAllowedError (the
 * gesture of a press lapses after a few seconds, so another download may outlast it), and so is a browser that disabled
 * the API. The rest is a model that did not download or start.
 */
function createFailure(cause: unknown): BrowserTranslationError {
  const notAllowed = cause instanceof DOMException && cause.name === "NotAllowedError";
  return new BrowserTranslationError(notAllowed ? "not_allowed" : "model", { cause });
}

/** The least confidence for trusting a detection; Chrome's documentation asks for such a cut, and gives no number. */
const leastConfidence = 0.5;

/** The language of `text`, by the browser's detector. */
async function detectLanguage(text: string, onDownload: (loaded: number) => void): Promise<string> {
  const api = builtIn.LanguageDetector;
  if (api === undefined) throw new BrowserTranslationError("undetected");
  let detector: BuiltInDetector;
  try {
    detector = await api.create(monitorFor(await api.availability(), onDownload));
  } catch (cause) {
    throw createFailure(cause);
  }
  try {
    const [best] = await detector.detect(text);
    if (best === undefined || isUnknownLanguage(best.detectedLanguage) || best.confidence < leastConfidence) {
      throw new BrowserTranslationError("undetected");
    }
    return best.detectedLanguage;
  } catch (cause) {
    if (cause instanceof BrowserTranslationError) throw cause;
    throw new BrowserTranslationError("undetected", { cause });
  } finally {
    detector.destroy();
  }
}

/** Whether a translation has its markers as they should be, and no broken piece of one (Chrome's de to ja adds a `⟧`). */
function markersWhole(fragment: string, translation: string): boolean {
  return markersIntact(fragment, translation) && !/[⟦⟧]/u.test(translation.replaceAll(markerPattern, ""));
}

/** Translates the words between the markers of `fragment` one piece at a time, each marker staying where it is. */
async function translateBetweenMarkers(fragment: string, translator: BuiltInTranslator): Promise<string> {
  let out = "";
  let from = 0;
  const piece = async (text: string) => {
    if (!/\p{L}/u.test(text)) return text;
    const core = text.trim();
    const start = text.indexOf(core);
    return text.slice(0, start) + (await translator.translate(core)) + text.slice(start + core.length);
  };
  for (const match of fragment.matchAll(markerPattern)) {
    out += (await piece(fragment.slice(from, match.index))) + match[0];
    from = match.index + match[0].length;
  }
  return out + (await piece(fragment.slice(from)));
}

/** The translation of a fragment with its markers whole; ADR-0148 §8: one that still lacks them is a failure, and is not shown. */
async function translateFragment(fragment: string, translator: BuiltInTranslator): Promise<string> {
  const translation = await translator.translate(fragment);
  if (markersWhole(fragment, translation)) return translation;
  const pieces = await translateBetweenMarkers(fragment, translator);
  if (markersWhole(fragment, pieces)) return pieces;
  throw new BrowserTranslationError("failed");
}

/** Translates `texts` in order, since the browser handles one at a time, and keeps each translation in its fragment. */
async function translateAll(
  texts: readonly MaskedText[],
  pair: LanguagePair,
  onDownload: (loaded: number) => void,
): Promise<void> {
  const availability = await pairAvailability(pair.sourceLanguage, pair.targetLanguage);
  if (availability === "unavailable") throw new BrowserTranslationError("unsupported");
  let translator: BuiltInTranslator | undefined;
  try {
    translator = await builtIn.Translator?.create({ ...pair, ...monitorFor(availability, onDownload) });
  } catch (cause) {
    throw createFailure(cause);
  }
  if (translator === undefined) throw new BrowserTranslationError("unsupported");
  try {
    for (const text of texts) {
      text.translation = await translateFragment(text.fragment, translator);
    }
  } catch (cause) {
    if (cause instanceof BrowserTranslationError) throw cause;
    throw new BrowserTranslationError("failed", { cause });
  } finally {
    translator.destroy();
  }
}

/** The brands that a browser with the Translator API gives; the others (and the fake ones it adds) name nothing known. */
const browserBrands: ReadonlySet<string> = new Set(["Google Chrome", "Microsoft Edge", "Opera"]);

/** The name of this browser for the attribution, or null when it gives none that is known. */
function browserName(): string | null {
  const { userAgentData } = navigator as { userAgentData?: { brands: readonly { brand: string }[] } };
  return userAgentData?.brands.find(({ brand }) => browserBrands.has(brand))?.brand ?? null;
}

/**
 * Translates `post` in the browser. A post with no words to translate comes back as the author's own text, as the
 * server answers one. The result goes only to the caller, which keeps it in memory (./translation.ts).
 */
export async function translateInBrowser(
  post: Status,
  target: string,
  onDownload: (loaded: number) => void,
): Promise<TranslatedPost> {
  const bodyTexts: MaskedText[] = [];
  const html = serverSanitizedHtml(post.content);
  const body = maskBlocks(sanitizedHtmlNodes(html), bodyTexts);
  const warning = maskInline([post.spoiler_text]);
  const options = (post.poll?.options ?? []).map(({ title }) => maskInline([title]));
  const described = post.media_attachments.flatMap(({ id, description }) =>
    description === undefined || description === "" ? [] : [{ id, description, text: maskInline([description]) }],
  );
  const texts = [...bodyTexts, warning, ...options, ...described.map(({ text }) => text)].filter(sendable);
  if (texts.length === 0) {
    return {
      language: post.language,
      body: { kind: "html", html },
      contentWarning: post.spoiler_text,
      altTexts: Object.fromEntries(described.map(({ id, description }) => [id, description])),
      pollOptions: post.poll?.options.map(({ title }) => title) ?? [],
      by: { kind: "author" },
    };
  }

  let source = post.language;
  if (isUnknownLanguage(source)) {
    source = await detectLanguage(
      texts.map(({ fragment }) => fragment.replaceAll(markerPattern, "")).join("\n"),
      onDownload,
    );
    if (isInLanguage(source, target)) throw new BrowserTranslationError("same_language");
  }
  await translateAll(texts, { sourceLanguage: source, targetLanguage: target }, onDownload);
  return {
    language: target,
    body: { kind: "nodes", nodes: restoreParts(body) },
    contentWarning: restoredText(warning),
    altTexts: Object.fromEntries(described.map(({ id, text }) => [id, restoredText(text)])),
    pollOptions: options.map(restoredText),
    by: { kind: "browser", name: browserName() },
  };
}

/**
 * Translates `text`, plain text in a language nobody named, such as the comment of a report (ADR-0148 §13), in the
 * browser: the detector finds its language first. Text with no words to translate comes back as it is.
 */
export async function translateTextInBrowser(
  text: string,
  target: string,
  onDownload: (loaded: number) => void,
): Promise<TranslatedComment> {
  const masked = maskInline([text]);
  if (!sendable(masked)) return { language: "und", text, by: { kind: "author" } };
  const source = await detectLanguage(masked.fragment.replaceAll(markerPattern, ""), onDownload);
  if (isInLanguage(source, target)) throw new BrowserTranslationError("same_language");
  await translateAll([masked], { sourceLanguage: source, targetLanguage: target }, onDownload);
  return { language: target, text: restoredText(masked), by: { kind: "browser", name: browserName() } };
}
