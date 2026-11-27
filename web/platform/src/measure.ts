// SPDX-License-Identifier: AGPL-3.0-only

// What the browser measures of how fast the page responds (ADR-0167 §1, §2): the largest contentful paint, the time
// from each interaction to its next paint (Event Timing, of which INP is the worst), and the long animation frames;
// and how a report of them leaves the page, with `navigator.sendBeacon` while it is hidden. A browser that lacks one of
// the entry types measures nothing of it, in the same form. Only the app imports this
// module (`@my/platform/measure`); it names what it hears by the route and sends it to the instance's own server.

type Target = Pick<EventTarget, "addEventListener" | "removeEventListener">;

/** What the observer of the performance timeline gives: an entry's start, its duration and its interaction. */
export type TimedEntry = { readonly startTime: number; readonly duration: number; readonly interactionId?: number };

/** The browser's PerformanceObserver, as far as this module uses it. */
export type PerformanceObserverLike = {
  observe(options: { type: string; buffered?: boolean; durationThreshold?: number }): void;
  takeRecords(): readonly TimedEntry[];
  disconnect(): void;
};

/** The constructor of the observers, and the entry types the browser knows. */
export type PerformanceObserverClass = {
  new (callback: (list: { getEntries(): readonly TimedEntry[] }) => void): PerformanceObserverLike;
  readonly supportedEntryTypes?: readonly string[];
};

/** What `watchVitals` hears, each in milliseconds. */
export type VitalsListener = {
  /** The largest contentful paint of the page's load, once, when the reader first acts or the page is settled. */
  largestPaint?: (ms: number) => void;
  /** An interaction, from the reader's input to the next paint; each event of one interaction is heard. */
  interaction?: (ms: number) => void;
  /** A long animation frame (50 ms and more). */
  longFrame?: (ms: number) => void;
};

/** Where the vitals are read: the browser's by default, a test's fakes otherwise. */
export type VitalsEnvironment = {
  readonly Observer?: PerformanceObserverClass | undefined;
  /** Where the reader's first input ends the largest contentful paint. */
  readonly target?: Target | undefined;
  /** Whether the page was hidden as it loaded, so that its largest paint says nothing of the reader's wait. */
  readonly hidden?: boolean | undefined;
};

/** The shortest interaction Event Timing reports (its minimum is 16 ms; INP's "good" is 200 ms). */
const interactionThreshold = 40;

/** The inputs that end the largest contentful paint, as the browser stops reporting it at the first. */
const firstInputs = ["keydown", "pointerdown"] as const;

const browserVitals = (): VitalsEnvironment => ({
  Observer: typeof PerformanceObserver === "undefined" ? undefined : PerformanceObserver,
  target: typeof window === "undefined" ? undefined : window,
  hidden: typeof document !== "undefined" && document.visibilityState === "hidden",
});

/**
 * Watches the vitals of the page until `stop`: `settle` hears what the observers hold and gives the largest
 * contentful paint now if it was not given yet; call it as the page is hidden, before the report is sent.
 */
export function watchVitals(
  listener: VitalsListener,
  { Observer, target, hidden = false }: VitalsEnvironment = browserVitals(),
): { settle: () => void; stop: () => void } {
  const observers: PerformanceObserverLike[] = [];
  const supported = (type: string) => Observer?.supportedEntryTypes?.includes(type) === true;
  const watch = (type: string, hear: (entry: TimedEntry) => void, options: { durationThreshold?: number } = {}) => {
    if (Observer === undefined || !supported(type)) return undefined;
    const observer = new Observer((list) => {
      for (const entry of list.getEntries()) hear(entry);
    });
    try {
      observer.observe({ type, buffered: true, ...options });
    } catch {
      return undefined;
    }
    observers.push(observer);
    return { observer, hear };
  };
  let largest: number | undefined;
  let paintDone = hidden || listener.largestPaint === undefined;
  const endPaint = () => {
    if (paintDone) return;
    paintDone = true;
    for (const type of firstInputs) target?.removeEventListener(type, endPaint, true);
    if (largest !== undefined) listener.largestPaint?.(largest);
  };
  const paints = paintDone
    ? undefined
    : watch("largest-contentful-paint", (entry) => {
        if (!paintDone) largest = entry.startTime;
      });
  if (paints !== undefined) for (const type of firstInputs) target?.addEventListener(type, endPaint, true);
  else paintDone = true;
  const interactions =
    listener.interaction === undefined
      ? undefined
      : watch(
          "event",
          (entry) => {
            if ((entry.interactionId ?? 0) > 0) listener.interaction?.(entry.duration);
          },
          { durationThreshold: interactionThreshold },
        );
  const frames =
    listener.longFrame === undefined
      ? undefined
      : watch("long-animation-frame", (entry) => {
          listener.longFrame?.(entry.duration);
        });
  const drain = () => {
    for (const watched of [paints, interactions, frames])
      if (watched !== undefined) for (const entry of watched.observer.takeRecords()) watched.hear(entry);
  };
  return {
    settle: () => {
      drain();
      endPaint();
    },
    stop: () => {
      paintDone = true;
      for (const type of firstInputs) target?.removeEventListener(type, endPaint, true);
      for (const observer of observers) observer.disconnect();
    },
  };
}

/**
 * Calls `listener` each time the page is hidden (another tab, the app in the background, the page going away), the
 * last moment a report surely leaves, until the function it returns is called.
 */
export function watchPageHidden(
  listener: () => void,
  target: Target | undefined = typeof document === "undefined" ? undefined : document,
  isHidden: () => boolean = () => typeof document !== "undefined" && document.visibilityState === "hidden",
): () => void {
  if (target === undefined) return () => undefined;
  const onChange = () => {
    if (isHidden()) listener();
  };
  target.addEventListener("visibilitychange", onChange);
  return () => {
    target.removeEventListener("visibilitychange", onChange);
  };
}

/**
 * Sends `body`, a JSON document, to `url` with a POST that outlives the page: `navigator.sendBeacon`, or a fetch
 * with `keepalive` where the browser refuses the beacon. It sends no access token and tells nothing of the answer.
 */
export function sendReport(url: string, body: string): void {
  try {
    if (typeof navigator !== "undefined" && typeof navigator.sendBeacon === "function") {
      if (navigator.sendBeacon(url, new Blob([body], { type: "application/json" }))) return;
    }
  } catch {
    // A beacon the browser refuses goes by fetch.
  }
  if (typeof fetch !== "function") return;
  fetch(url, {
    method: "POST",
    body,
    headers: { "Content-Type": "application/json" },
    keepalive: true,
    credentials: "omit",
  }).catch(() => undefined);
}
