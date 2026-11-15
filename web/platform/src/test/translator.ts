// SPDX-License-Identifier: AGPL-3.0-only
// A browser with the Translator API for the tests (https://webmachinelearning.github.io/translation-api/): Bun has
// none, so this stands in for the global Translator until `restore`. Each pair of languages is available as the test
// sets it; a translator puts the target language before each text ("[en] …"), and its model downloads in the steps
// the test gives, each a `downloadprogress` event on the monitor, before `create` resolves.

type Availability = "unavailable" | "downloadable" | "downloading" | "available";
type Pair = { sourceLanguage: string; targetLanguage: string };
type CreateOptions = Pair & { monitor?: (m: EventTarget) => void; signal?: AbortSignal };

export interface FakeTranslator {
  /** What `availability` answers for `source>target`; a pair not named is unavailable. */
  pairs: Record<string, Availability>;
  /** The shares of the model that come, in order, when `create` downloads it. */
  download: number[];
  /** What `create` rejects with, when it does. */
  refuse: Error | undefined;
  /** The pairs `create` was called with, as `source>target`. */
  created: string[];
  /** The texts each translator was given. */
  translated: string[];
  restore(): void;
}

/** Makes the browser of the test one with a translator, and returns what it was asked. */
export function installTranslator(pairs: Record<string, Availability> = {}): FakeTranslator {
  const fake: FakeTranslator = {
    pairs,
    download: [],
    refuse: undefined,
    created: [],
    translated: [],
    restore: () => {
      Reflect.deleteProperty(globalThis, "Translator");
    },
  };
  const Translator = {
    availability: ({ sourceLanguage, targetLanguage }: Pair) =>
      Promise.resolve(fake.pairs[`${sourceLanguage}>${targetLanguage}`] ?? "unavailable"),
    create: ({ sourceLanguage, targetLanguage, monitor }: CreateOptions) => {
      fake.created.push(`${sourceLanguage}>${targetLanguage}`);
      if (fake.refuse !== undefined) return Promise.reject(fake.refuse);
      const target = new EventTarget();
      monitor?.(target);
      for (const loaded of fake.download)
        target.dispatchEvent(Object.assign(new Event("downloadprogress"), { loaded }));
      return Promise.resolve({
        translate: (text: string) => {
          fake.translated.push(text);
          return Promise.resolve(`[${targetLanguage}] ${text}`);
        },
        destroy: () => undefined,
      });
    },
  };
  Object.defineProperty(globalThis, "Translator", { configurable: true, value: Translator });
  return fake;
}
