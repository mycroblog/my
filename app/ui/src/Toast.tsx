// SPDX-License-Identifier: AGPL-3.0-only
// Toasts: short, visible notes about something that just happened (posted, copied, failed), stacked at
// the bottom edge. A toast never takes focus. It is announced through the Announcer's live regions
// (web-client/README.md Y7), so the visible list itself is not a live region and nothing is read twice.
// The list is a labelled region landmark, so keyboard and screen reader users can reach a toast's action.
//
// React Aria Components' Toast is still UNSTABLE_ (react-aria-components 1.21), so the kit keeps its own
// small queue. Auto-dismiss pauses while the pointer is over the toasts or focus is in them (WCAG 2.2.1),
// and a toast with an action stays until it is dismissed unless it sets a timeout.
import { createContext, type ReactNode, use, useEffect, useRef, useState } from "react";
import { Button } from "react-aria-components";
import { tv } from "tailwind-variants";
import { useAnnouncer } from "./Announcer";
import { celebrationClasses, useCelebration } from "./celebration";
import { AboutIcon, CheckIcon, CloseIcon, WarningIcon } from "./Icons";
import { Tooltip, TooltipTrigger } from "./Tooltip";

export type ToastTone = "neutral" | "success" | "danger";

export type ToastOptions = {
  /** The message, already translated. */
  message: string;
  /** `danger` is announced assertively; the others politely. */
  tone?: ToastTone;
  /** One follow-up action, such as "Undo" or "View". Pressing it also dismisses the toast. */
  action?: { label: string; onAction: () => void };
  /** Milliseconds before the toast closes by itself; `null` keeps it until dismissed. */
  timeout?: number | null;
  /**
   * The toast tells of a celebrated moment (web-client/21-experience-principles.md §21.5, such as the first post): its
   * icon grows in with the small overshoot of `ease-celebrate` as it appears, or, with reduced motion, takes the `soft`
   * fill for a moment. Only for the moments listed there; the message says the same in words.
   */
  celebrate?: boolean;
};

export type ToastApi = {
  /** Shows and announces a toast and returns its id. */
  show: (options: ToastOptions) => number;
  dismiss: (id: number) => void;
};

/** The toasts of the nearest ToastProvider; `useNotice` reads it too. */
export const ToastContext = createContext<ToastApi | null>(null);

type QueuedToast = ToastOptions & { id: number };

/** A timeout that can pause and resume, for auto-dismiss. It lives outside rendering, so it may read the clock. */
class PausableTimer {
  #handle: ReturnType<typeof setTimeout> | null = null;
  #remaining: number;
  #startedAt = 0;
  readonly #onDone: () => void;

  constructor(duration: number, onDone: () => void) {
    this.#remaining = duration;
    this.#onDone = onDone;
  }

  resume() {
    if (this.#handle !== null) return;
    this.#startedAt = Date.now();
    this.#handle = setTimeout(this.#onDone, this.#remaining);
  }

  pause() {
    if (this.#handle === null) return;
    clearTimeout(this.#handle);
    this.#handle = null;
    this.#remaining = Math.max(0, this.#remaining - (Date.now() - this.#startedAt));
  }
}

const toast = tv({
  slots: {
    region: [
      "pointer-events-none fixed inset-x-0 bottom-0 z-50 flex flex-col items-center gap-2 p-4",
      "pb-[max(--spacing(4),env(safe-area-inset-bottom))]",
    ],
    item: [
      "pointer-events-auto flex w-full max-w-md items-center gap-3 rounded-xl bg-toast",
      "py-2 ps-4 pe-2 text-sm text-toast-fg shadow-lg dark:border dark:border-border",
    ],
    // The tone shows by this icon and not by a colored edge (WCAG 1.4.1).
    icon: "shrink-0 rounded-full text-toast-muted [&>svg]:size-5",
    message: "flex-1 py-1",
    action: [
      "shrink-0 rounded-md px-2 py-1.5 font-extrabold text-toast-action outline-none data-focus-visible:focus-ring",
      "pointer-coarse:min-h-11",
    ],
    dismiss: [
      "grid size-8 shrink-0 place-items-center rounded-md text-toast-muted outline-none data-focus-visible:focus-ring",
      "data-hovered:bg-toast-fg/10 data-hovered:text-toast-fg pointer-coarse:size-11 [&>svg]:size-4",
    ],
  },
  variants: {
    clearsTabBar: {
      true: {
        region:
          "bottom-[calc(4rem+1px+env(safe-area-inset-bottom))] pb-4 short:bottom-[calc(3rem+1px+env(safe-area-inset-bottom))] md:bottom-0 md:pb-[max(--spacing(4),env(safe-area-inset-bottom))] with-compose-fab:bottom-[calc(8rem+env(safe-area-inset-bottom))] with-compose-fab:short:bottom-[calc(6.5rem+env(safe-area-inset-bottom))] with-compose-fab:md:bottom-0",
      },
    },
    tone: {
      neutral: {},
      success: { icon: "text-toast-success" },
      danger: { icon: "text-toast-danger" },
    },
    celebrate: { true: { icon: celebrationClasses } },
  },
});

export type ToastProviderProps = {
  children: ReactNode;
  /** The accessible name of the toast region, such as "Notifications". */
  regionLabel: string;
  /** The accessible name of each toast's × button, such as "Dismiss". */
  dismissLabel: string;
  /** The timeout of a toast without an action, in ms. */
  defaultTimeout?: number;
  /**
   * Below md the toasts stand above the app's tab bar along the bottom edge (4rem high, 3rem on short screens, with a 1px
   * border above and the safe-area inset below), which they would otherwise cover. On a page with a floating new-post
   * button (`[data-compose-fab]` in the document) they stand above it too (web-client/16-look-states-and-wording.md G4). From md the navigation is a
   * side rail and the toasts go back to the bottom edge.
   */
  clearsTabBar?: boolean;
};

/**
 * Renders the toasts and gives `useToast()` to its children. Mount it once, inside the AnnouncerProvider.
 */
export function ToastProvider({
  children,
  regionLabel,
  dismissLabel,
  defaultTimeout = 5000,
  clearsTabBar = false,
}: ToastProviderProps) {
  const announce = useAnnouncer();
  const [toasts, setToasts] = useState<readonly QueuedToast[]>([]);
  const nextIdRef = useRef(0);
  const timersRef = useRef(new Map<number, PausableTimer>());
  const regionRef = useRef<HTMLElement>(null);
  /** Pointer over the region, focus in it: either pauses every timer. */
  const holdRef = useRef({ hovered: false, focused: false });
  /** Where focus came from when it entered the region, to return it when the last toast closes. */
  const returnFocusRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    const timers = timersRef.current;
    return () => {
      for (const timer of timers.values()) timer.pause();
      timers.clear();
    };
  }, []);

  const isHeld = () => holdRef.current.hovered || holdRef.current.focused;

  const pause = () => {
    for (const timer of timersRef.current.values()) timer.pause();
  };

  const resume = () => {
    if (isHeld()) return;
    for (const timer of timersRef.current.values()) timer.resume();
  };

  function dismiss(id: number) {
    timersRef.current.get(id)?.pause();
    timersRef.current.delete(id);

    // Removing the focused toast would drop focus to <body>; move it to a neighbour or back where it came from.
    const items = [...(regionRef.current?.querySelectorAll<HTMLElement>("[data-toast-id]") ?? [])];
    const index = items.findIndex((item) => item.dataset["toastId"] === String(id));
    const current = items[index];
    if (current?.contains(document.activeElement) === true) {
      const neighbour = items[index + 1] ?? items[index - 1];
      if (neighbour !== undefined) neighbour.querySelector<HTMLElement>("button")?.focus();
      else if (returnFocusRef.current?.isConnected === true) returnFocusRef.current.focus();
    }
    if (items.length <= 1) {
      // The region unmounts with its last toast, so it fires no leave or blur events.
      holdRef.current = { hovered: false, focused: false };
      returnFocusRef.current = null;
    }
    setToasts((list) => list.filter((t) => t.id !== id));
  }

  const show = (options: ToastOptions) => {
    const id = nextIdRef.current++;
    setToasts((list) => [...list, { ...options, id }]);
    announce(options.message, options.tone === "danger" ? "assertive" : "polite");
    const timeout =
      options.timeout === undefined ? (options.action === undefined ? defaultTimeout : null) : options.timeout;
    if (timeout !== null) {
      const timer = new PausableTimer(timeout, () => {
        dismiss(id);
      });
      timersRef.current.set(id, timer);
      if (!isHeld()) timer.resume();
    }
    return id;
  };

  const styles = toast({ clearsTabBar });
  return (
    <ToastContext value={{ show, dismiss }}>
      {children}
      {toasts.length > 0 && (
        <section
          ref={regionRef}
          aria-label={regionLabel}
          // Stays reachable while a modal hides the rest of the page (React Aria's ariaHideOutside).
          data-react-aria-top-layer="true"
          className={styles.region()}
          onPointerEnter={() => {
            holdRef.current.hovered = true;
            pause();
          }}
          onPointerLeave={() => {
            holdRef.current.hovered = false;
            resume();
          }}
          onFocus={(event) => {
            if (
              !holdRef.current.focused &&
              event.relatedTarget instanceof HTMLElement &&
              !event.currentTarget.contains(event.relatedTarget)
            ) {
              returnFocusRef.current = event.relatedTarget;
            }
            holdRef.current.focused = true;
            pause();
          }}
          onBlur={(event) => {
            if (event.relatedTarget instanceof Node && event.currentTarget.contains(event.relatedTarget)) return;
            holdRef.current.focused = false;
            resume();
          }}
        >
          <ol className="contents">
            {toasts.map((t) => (
              <li key={t.id} data-toast-id={t.id} className={styles.item({ tone: t.tone ?? "neutral" })}>
                <ToastIcon tone={t.tone ?? "neutral"} celebrate={t.celebrate === true} />
                <p className={styles.message()}>{t.message}</p>
                {t.action !== undefined && (
                  <Button
                    className={styles.action()}
                    onPress={() => {
                      t.action?.onAction();
                      dismiss(t.id);
                    }}
                  >
                    {t.action.label}
                  </Button>
                )}
                <TooltipTrigger>
                  <Button
                    aria-label={dismissLabel}
                    className={styles.dismiss()}
                    onPress={() => {
                      dismiss(t.id);
                    }}
                  >
                    <CloseIcon />
                  </Button>
                  <Tooltip>{dismissLabel}</Tooltip>
                </TooltipTrigger>
              </li>
            ))}
          </ol>
        </section>
      )}
    </ToastContext>
  );
}

/** The icon of a toast's tone; a celebrating toast's icon carries the moment from when it appears. */
function ToastIcon({ tone, celebrate }: { tone: ToastTone; celebrate: boolean }) {
  const celebration = useCelebration(celebrate);
  return (
    <span className={toast().icon({ tone, celebrate })} {...celebration.celebrationProps}>
      {tone === "success" ? <CheckIcon /> : tone === "danger" ? <WarningIcon filled /> : <AboutIcon />}
    </span>
  );
}

/** Returns `{ show, dismiss }`. Call them from event handlers and mutation callbacks. */
export function useToast(): ToastApi {
  const api = use(ToastContext);
  if (api === null) throw new Error("useToast needs a <ToastProvider> above it");
  return api;
}
