// SPDX-License-Identifier: AGPL-3.0-only

// The translator built into the browser (the Translator API, ADR-0148 §12), where the browser has one: whether it
// translates between two languages, and a translator between them, which downloads its model first when it has to
// and says how much has come. It sends nothing anywhere: the browser translates on the device.

/** Whether the browser translates between two languages: not at all, after downloading its model, or now. */
export type TranslatorAvailability = "unavailable" | "downloadable" | "downloading" | "available";

/** A translator between two languages: text in, text out. Its output is text, never markup to parse. */
export type BrowserTranslator = {
  readonly translate: (text: string) => Promise<string>;
  readonly destroy: () => void;
};

type Pair = { sourceLanguage: string; targetLanguage: string };

/** The part of the Translator API this reads (https://webmachinelearning.github.io/translation-api/). */
type TranslatorGlobal = {
  availability: (pair: Pair) => Promise<TranslatorAvailability>;
  create: (
    options: Pair & {
      monitor?: (m: EventTarget) => void;
      signal?: AbortSignal;
    },
  ) => Promise<BrowserTranslator>;
};

function translatorGlobal(): TranslatorGlobal | undefined {
  const found: unknown = Reflect.get(globalThis, "Translator");
  return typeof found === "function" || (typeof found === "object" && found !== null)
    ? (found as TranslatorGlobal)
    : undefined;
}

/** Whether the browser has a translator at all. */
export function hasBrowserTranslator(): boolean {
  return translatorGlobal() !== undefined;
}

/**
 * Whether the browser translates from `source` into `target` (BCP 47 tags): `unavailable` where it has no translator,
 * or it fails to say.
 */
export async function browserTranslatorAvailability(source: string, target: string): Promise<TranslatorAvailability> {
  const translator = translatorGlobal();
  if (translator === undefined) return "unavailable";
  try {
    return await translator.availability({ sourceLanguage: source, targetLanguage: target });
  } catch {
    return "unavailable";
  }
}

/**
 * A translator from `source` into `target`. When the browser downloads its model first, `onProgress` hears the share
 * that came (0 to 1). It needs the reader's press just before when it downloads (the browser refuses otherwise, with a
 * NotAllowedError); it rejects where the browser has no translator.
 */
export async function openBrowserTranslator(
  source: string,
  target: string,
  onProgress?: (share: number) => void,
  signal?: AbortSignal,
): Promise<BrowserTranslator> {
  const translator = translatorGlobal();
  if (translator === undefined) throw new DOMException("no translator in this browser", "NotSupportedError");
  const created = await translator.create({
    sourceLanguage: source,
    targetLanguage: target,
    monitor: (m) => {
      m.addEventListener("downloadprogress", (e) => {
        const loaded: unknown = Reflect.get(e, "loaded");
        if (typeof loaded === "number") onProgress?.(loaded);
      });
    },
    ...(signal === undefined ? {} : { signal }),
  });
  return {
    translate: (text) => created.translate(text),
    destroy: () => {
      created.destroy();
    },
  };
}

/**
 * The browser's name, for the note of what it translated ("Google Chrome"), from the brands it states; undefined where
 * it states none but the engine's.
 */
export function browserName(): string | undefined {
  const data: unknown = typeof navigator === "undefined" ? undefined : Reflect.get(navigator, "userAgentData");
  const brands: unknown = typeof data === "object" && data !== null ? Reflect.get(data, "brands") : undefined;
  if (!Array.isArray(brands)) return undefined;
  const names = (brands as unknown[])
    .map((b): unknown => (typeof b === "object" && b !== null ? Reflect.get(b, "brand") : undefined))
    .filter((n): n is string => typeof n === "string" && !/not.?a.?brand/i.test(n));
  return names.find((n) => n !== "Chromium");
}

/** Whether `error` is the browser refusing to download a model without the reader's press just before. */
export const needsActivation = (error: unknown): boolean =>
  error instanceof DOMException && error.name === "NotAllowedError";
