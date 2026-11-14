// SPDX-License-Identifier: AGPL-3.0-only

// The keyboard shortcuts (ADR-0025 §4): the mechanism is the design system's, the actions the parts', the keymap the
// app's. The app places `ShortcutProvider` with its keymap (action ID → keys, keys.ts); a part declares each action it
// handles with `useAction(id, run, { scope, label })` and spreads what it returns (aria-keyshortcuts) on the control
// that does the same, and the action works and shows in the help. A screen action acts anywhere on the screen; a focus
// action acts while focus is inside the nearest `ActionScope` (a post's entry), the nearest such element first. No
// action acts while an input method composes, inside a text field (but for keys the keymap allows there), or under a
// layer over the screen (a dialog, a menu) for a screen action. `shortcuts.help` ("?") opens a dialog that lists the
// screen's actions, then the focus actions on the screen by the kind of thing they act on (a post, the timeline), the
// kind that holds focus now first, each with its keys as the reader's platform names them.

import {
  createContext,
  use,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react";
import { Dialog, Heading } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { IconButton } from "./icon-button";
import {
  isTextField,
  Shortcuts,
  spelled,
  type Keymap,
  type Listed,
  type ListedGroup,
  type Place,
  type Pressed,
  type Scope,
} from "./keys";
import { stringsIn, useReader } from "./locale";
import { ModalLayer, ModalPresence } from "./modal";

/** The action that opens the list of shortcuts; the keymap gives its key ("?"). */
export const helpAction = "shortcuts.help";

const StoreContext = createContext<Shortcuts<Element> | null>(null);
const ScopeContext = createContext<RefObject<HTMLElement | null> | null>(null);

// A layer over the screen: while focus is inside one, only the focus actions inside it act.
const layers = '[role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"]';

/** Where focus is: the element and its ancestors up to the layer that holds it, and what it is inside. */
function placeOf(target: EventTarget | null): Place<Element> {
  const el = target instanceof HTMLElement ? target : null;
  const layer = el?.closest(layers) ?? null;
  const path: Element[] = [];
  for (let at: Element | null = el; at !== null; at = at.parentElement) {
    path.push(at);
    if (at === layer) break;
  }
  return { path, inField: el !== null && isTextField(el), overlaid: layer !== null };
}

/**
 * Holds the keymap and the declared actions, runs the action of a key press, and shows the help. `keymap` is the
 * app's and is read once; a chord the browser keeps for itself throws. `mac` names the keys as on macOS (⌘): the app
 * passes what @my/platform says of the reader's keyboard.
 */
export function ShortcutProvider({ keymap, mac, children }: { keymap: Keymap; mac: boolean; children: ReactNode }) {
  const [store] = useState(() => new Shortcuts<Element>(keymap, mac));
  const [shown, setShown] = useState<ReturnType<Shortcuts<Element>["list"]> | null>(null);
  const { t } = stringsIn(useReader(undefined).lang);
  const label = t("shortcuts.help");
  useEffect(
    () =>
      store.declare({
        id: helpAction,
        label,
        scope: "screen",
        owner: () => null,
        run: () => {
          setShown(store.list(placeOf(document.activeElement).path));
        },
      }),
    [store, label],
  );
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (!e.defaultPrevented && store.press(e, placeOf(e.target))) e.preventDefault();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [store]);
  return (
    <StoreContext value={store}>
      {children}
      <ModalPresence>
        {shown !== null && (
          <ModalLayer
            key="help"
            onDismiss={() => {
              setShown(null);
            }}
            overlay={help().overlay()}
            panel={help().modal()}
          >
            <ShortcutList screen={shown.screen} groups={shown.groups} mac={store.mac} />
          </ModalLayer>
        )}
      </ModalPresence>
    </StoreContext>
  );
}

/** Marks the element focus actions inside it act on: focus must be inside `target` for them to act. */
export function ActionScope({ target, children }: { target: RefObject<HTMLElement | null>; children: ReactNode }) {
  return <ScopeContext value={target}>{children}</ScopeContext>;
}

/**
 * Declares an action this part handles: `run` runs on its keys in the keymap, within `scope`, and the help lists it
 * by `label` (from the part's catalog), told whether the key is held; when `run` returns `false` it did nothing, and
 * the key goes on to the page. A
 * focus action shows under its group, the namespace of its ID (`post` of `post.reply`) unless `group` names another,
 * headed by `groupLabel` from the part's catalog ("When a post is focused"). It returns the aria-keyshortcuts to
 * spread on the control that does the same. Without a provider (a part on its own) it declares nothing.
 */
export function useAction(
  id: string,
  run: (press: Pressed) => unknown,
  {
    scope,
    label,
    group,
    groupLabel,
  }: { scope: Scope; label: string; group?: string | undefined; groupLabel?: string | undefined },
): { "aria-keyshortcuts"?: string } {
  const store = use(StoreContext);
  const target = use(ScopeContext);
  if (store !== null && scope === "focus" && target === null)
    throw new Error(`${id} acts on the focused element, but no ActionScope holds it`);
  const latest = useRef(run);
  useLayoutEffect(() => {
    latest.current = run;
  });
  useEffect(() => {
    if (store === null) return undefined;
    return store.declare({
      id,
      label,
      scope,
      owner: () => (scope === "focus" ? (target?.current ?? null) : null),
      run: (press) => latest.current(press),
      group,
      groupLabel,
    });
  }, [store, target, id, label, scope, group, groupLabel]);
  const keys = store?.aria(id);
  return keys === undefined ? {} : { "aria-keyshortcuts": keys };
}

const help = tv({
  slots: {
    overlay: "fixed inset-0 z-50 box-border flex items-center justify-center p-dialog-margin",
    modal: [
      "relative box-border max-h-full w-full max-w-[var(--dialog-max-width)] overflow-auto rounded-overlay bg-surface",
      "p-dialog font-sans text-body text-text shadow-overlay outline-hidden",
    ],
    dialog: "grid gap-dialog-gap",
    header: "flex items-center justify-between gap-dialog-gap",
    title: "m-0 text-title",
    group: "grid gap-dialog-row",
    caption: "m-0 flex flex-wrap items-center gap-kbd-gap text-caption font-strong text-muted-text",
    current: "rounded-control bg-surface-raised px-kbd-inline font-label text-text",
    list: "m-0 grid gap-dialog-row",
    row: "flex items-center justify-between gap-dialog-gap",
    label: "min-w-0",
    keys: "m-0 flex flex-none gap-kbd-gap",
    key: [
      "box-border inline-flex h-kbd min-w-[var(--kbd-min-width)] items-center justify-center rounded-control",
      "bg-surface-raised px-kbd-inline font-sans text-caption text-text",
    ],
  },
});

/** A group of the help: its heading, marked when focus is in it now, and each action's label and keys. */
function Group({
  title,
  current,
  actions,
  mac,
}: {
  title: string;
  /** The words that mark the group focus is in now; none for another group. */
  current?: string | undefined;
  actions: readonly Listed[];
  mac: boolean;
}) {
  const s = help();
  const id = useId();
  return (
    <section aria-labelledby={id} className={s.group()} data-current={current === undefined ? undefined : ""}>
      <h3 id={id} className={s.caption()}>
        {title}
        {current !== undefined && <span className={s.current()}>{current}</span>}
      </h3>
      <dl className={s.list()}>
        {actions.map((a) => (
          <div key={a.id} className={s.row()} data-action={a.id}>
            <dt className={s.label()}>{a.label}</dt>
            <dd className={s.keys()}>
              {a.keys.map((k) => (
                <kbd key={k} className={s.key()}>
                  {spelled(k, mac)}
                </kbd>
              ))}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

/**
 * The help: a dialog titled with what it lists, the screen's actions, then a section per group of focus actions (the
 * group focus is in now first, marked), and a button that closes it, as do Escape and a press outside.
 */
export function ShortcutList({
  screen,
  groups,
  mac,
}: {
  screen: readonly Listed[];
  groups: readonly ListedGroup[];
  mac: boolean;
}) {
  const s = help();
  const { lang, direction } = useReader(undefined);
  const { t } = stringsIn(lang);
  return (
    <Dialog className="outline-hidden">
      <div className={s.dialog()} lang={lang} dir={direction}>
        <div className={s.header()}>
          <Heading slot="title" className={s.title()}>
            {t("shortcuts.title")}
          </Heading>
          <IconButton icon="close" label={t("shortcuts.close")} slot="close" />
        </div>
        {screen.length > 0 && <Group title={t("shortcuts.screen")} actions={screen} mac={mac} />}
        {groups.map((g) => (
          <Group
            key={g.id}
            title={g.label}
            current={g.current ? t("shortcuts.current") : undefined}
            actions={g.actions}
            mac={mac}
          />
        ))}
      </div>
    </Dialog>
  );
}
