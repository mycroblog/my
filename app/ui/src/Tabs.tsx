// SPDX-License-Identifier: AGPL-3.0-only
// Tabs: React Aria's Tabs + TabList + Tab + TabPanel (WAI-ARIA APG, tabs pattern with automatic activation).
// Left / Right follow the reading direction (in RTL, Left moves to the next tab), Home / End jump to the ends.
//
// The selection is controllable, so a route can keep the tab in the URL (web-client/README.md S3): pass the key
// from the validated search params as `selectedKey`, and navigate in `onSelectionChange`. Each Tab may
// instead have an `href`, so that the tabs are links and the router's own navigation selects them.
//
// Tabs that do not fit scroll sideways inside the list (by finger, trackpad or Shift + wheel), which has no
// scrollbar, as in React Aria's own TabList styles. The inline edge behind which tabs are hidden fades out,
// so that a tab cut by the edge reads as "more this way". The fade is static (no transition), so it shows
// with reduced motion too. The tab that gets focus or the selection scrolls fully into view, clear of the fade.
import { use, useEffect, useRef, useState, type ReactNode, type Ref } from "react";
import { mergeRefs } from "react-aria";
import {
  Tab as AriaTab,
  TabList as AriaTabList,
  TabPanel as AriaTabPanel,
  Tabs as AriaTabs,
  TabListStateContext,
  type TabListProps as AriaTabListProps,
  type TabPanelProps as AriaTabPanelProps,
  type TabProps as AriaTabProps,
  type Key,
  type TabsProps as AriaTabsProps,
} from "react-aria-components";
import { tv } from "tailwind-variants";
import type { Space } from "./layout";

const tabs = tv({
  slots: {
    root: "flex flex-col gap-4",
    // overflow-y-clip keeps the list from scrolling vertically by a pixel, as overflow-x-auto alone would.
    // scroll-px-8 is the width of the fade: a tab scrolled into view stays clear of it (scrollTabIntoView).
    list: [
      "flex scroll-px-8 gap-1 overflow-x-auto overflow-y-clip border-b border-border scrollbar-none",
      // The edge with hidden tabs fades out over its last 2rem (the scroll-px above). It is a mask rather than a
      // gradient overlay, so it works on any surface, and the two edges' mask layers combine.
      "data-overflow-start:ltr:mask-l-from-[calc(100%-2rem)] data-overflow-start:rtl:mask-r-from-[calc(100%-2rem)]",
      "data-overflow-end:ltr:mask-r-from-[calc(100%-2rem)] data-overflow-end:rtl:mask-l-from-[calc(100%-2rem)]",
    ],
    tab: [
      // relative: the visually hidden text of a badge in the tab (absolutely positioned) stays inside the scrolling
      // list instead of widening the page.
      "relative flex shrink-0 cursor-default items-center gap-1.5 border-b-3 border-transparent px-4 py-2 text-sm font-bold",
      "text-fg-muted outline-none select-none pointer-coarse:min-h-11",
      "data-hovered:text-fg data-selected:border-accent data-selected:text-accent data-disabled:opacity-60",
      // The list scrolls sideways on narrow screens and would clip an outer ring, so the tab's ring is inset.
      "data-focus-visible:rounded-t-md data-focus-visible:outline-2 data-focus-visible:-outline-offset-2 data-focus-visible:outline-focus",
      "motion-safe:transition-colors",
    ],
    // The icon before a tab's text takes the tab's color, at the size of the text.
    icon: "shrink-0 [&>svg]:size-4",
    panel: "outline-none data-focus-visible:rounded-md data-focus-visible:focus-ring",
  },
  variants: {
    // The panel's content one above another, as a `Stack` with the same `gap` would lay it out.
    gap: {
      "0": { panel: "flex flex-col gap-0" },
      "0.5": { panel: "flex flex-col gap-0.5" },
      "1": { panel: "flex flex-col gap-1" },
      "2": { panel: "flex flex-col gap-2" },
      "3": { panel: "flex flex-col gap-3" },
      "4": { panel: "flex flex-col gap-4" },
      "6": { panel: "flex flex-col gap-6" },
      "8": { panel: "flex flex-col gap-8" },
      "10": { panel: "flex flex-col gap-10" },
    },
    // Room between the tab list and the panel, added to the `gap-4` of `Tabs`.
    offset: { "3": { panel: "pt-3" }, "4": { panel: "pt-4" } },
  },
});

export type TabsProps = Omit<AriaTabsProps, "className" | "style" | "children" | "orientation"> & {
  children: ReactNode;
  /** The selected tab's id, for a selection kept outside (the URL). */
  selectedKey?: Key | null;
  onSelectionChange?: (key: Key) => void;
  /** Classes merged onto the root with tailwind-merge. */
  className?: string;
  ref?: Ref<HTMLDivElement>;
};

/** A set of tabs: a TabList followed by one TabPanel per Tab, matched by `id`. */
export function Tabs({ className, ...props }: TabsProps) {
  return <AriaTabs {...props} className={tabs().root({ class: className })} />;
}

export type TabListProps<T extends object> = Omit<AriaTabListProps<T>, "className" | "style"> & {
  /** What the tabs switch between, for assistive technology (or use aria-labelledby). */
  "aria-label"?: string;
  className?: string;
  ref?: Ref<HTMLDivElement>;
};

export function TabList<T extends object>({ className, ref, ...props }: TabListProps<T>) {
  const listRef = useRef<HTMLDivElement>(null);
  const overflow = useInlineOverflow(listRef);
  // Null while React Aria builds the collection, which renders the list once without its state.
  const state = use(TabListStateContext);
  const selectedKey = state?.selectedKey;
  const focusedKey = state?.selectionManager.focusedKey;

  // The list's scroll position is outside React: bring the tab that is now focused or selected into view,
  // which includes the selected tab of the first render (a tab from the URL). It is done again in the next
  // frame, after the tabs of the first render exist and after a router's scroll restoration of the
  // navigation that a URL-bound selection makes, which puts back the list's scroll position from before it.
  useEffect(() => {
    const list = listRef.current;
    if (list === null) return;
    const reveal = () => {
      const active = list.ownerDocument.activeElement;
      const tab =
        active instanceof HTMLElement && active.getAttribute("role") === "tab" && list.contains(active)
          ? active
          : list.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]');
      if (tab !== null) scrollTabIntoView(list, tab);
    };
    reveal();
    const frame = requestAnimationFrame(reveal);
    return () => {
      cancelAnimationFrame(frame);
    };
  }, [selectedKey, focusedKey]);

  return (
    <AriaTabList
      {...props}
      ref={mergeRefs(listRef, ref)}
      data-overflow-start={overflow.start || undefined}
      data-overflow-end={overflow.end || undefined}
      className={tabs().list({ class: className })}
    />
  );
}

type InlineOverflow = { start: boolean; end: boolean };

/**
 * Whether the element hides content before its inline start and after its inline end. In RTL, scrollLeft
 * is 0 at the start and negative towards the end (CSSOM View), so its magnitude is the logical offset.
 */
function measureInlineOverflow(element: HTMLElement): InlineOverflow {
  const offset = Math.abs(element.scrollLeft);
  const max = element.scrollWidth - element.clientWidth;
  // A pixel of slack for fractional widths and scroll positions.
  return { start: offset > 1, end: max - offset > 1 };
}

function useInlineOverflow(ref: { readonly current: HTMLElement | null }): InlineOverflow {
  const [overflow, setOverflow] = useState<InlineOverflow>({ start: false, end: false });
  useEffect(() => {
    const element = ref.current;
    if (element === null || typeof ResizeObserver === "undefined") return;
    const update = () => {
      const next = measureInlineOverflow(element);
      setOverflow((current) => (current.start === next.start && current.end === next.end ? current : next));
    };
    // The list resizes with its container, and its tabs when a label or the font changes. A ResizeObserver
    // reports every element once when it starts observing it, which takes the first measurement.
    const observer = new ResizeObserver(update);
    observer.observe(element);
    for (const child of element.children) observer.observe(child);
    element.addEventListener("scroll", update, { passive: true });
    return () => {
      observer.disconnect();
      element.removeEventListener("scroll", update);
    };
  }, [ref]);
  return overflow;
}

/**
 * Scrolls the list, and only the list (never the page), so that the tab is inside it and clear of the
 * fades, whose width is the list's scroll-padding. Rectangles and the change of scrollLeft are both
 * physical, so the arithmetic is the same in LTR and RTL; a tab wider than the room aligns its inline start.
 */
function scrollTabIntoView(list: HTMLElement, tab: HTMLElement) {
  const style = getComputedStyle(list);
  const listRect = list.getBoundingClientRect();
  const tabRect = tab.getBoundingClientRect();
  const left = listRect.left + (Number.parseFloat(style.scrollPaddingLeft) || 0);
  const right = listRect.right - (Number.parseFloat(style.scrollPaddingRight) || 0);
  const rtl = style.direction === "rtl";
  if (tabRect.width > right - left) {
    list.scrollLeft += rtl ? tabRect.right - right : tabRect.left - left;
  } else if (tabRect.left < left) {
    list.scrollLeft += tabRect.left - left;
  } else if (tabRect.right > right) {
    list.scrollLeft += tabRect.right - right;
  }
}

export type TabProps = Omit<AriaTabProps, "className" | "style" | "children"> & {
  /** Matches the `id` of its TabPanel and is the key of `selectedKey`. */
  id: Key;
  children: ReactNode;
  /**
   * An icon from @my/ui before the text, which shows the tab's kind at a glance (web-client/21-experience-principles.md
   * P3). It is decorative: the text names the tab.
   */
  icon?: ReactNode;
  className?: string;
  ref?: Ref<HTMLDivElement>;
};

export function Tab({ icon, children, className, ...props }: TabProps) {
  const slots = tabs();
  return (
    <AriaTab {...props} className={slots.tab({ class: className })}>
      {icon !== undefined && (
        <span aria-hidden="true" className={slots.icon()}>
          {icon}
        </span>
      )}
      {children}
    </AriaTab>
  );
}

export type TabPanelProps = Omit<AriaTabPanelProps, "className" | "style" | "children"> & {
  id: Key;
  children: ReactNode;
  /** Lays the panel's content out one above another with this gap, in a spacing step (as a `Stack`). */
  gap?: Space;
  /** More room between the tab list and the panel than the `gap-4` of `Tabs`: `pt-3` or `pt-4` on the panel. */
  offset?: "3" | "4";
  className?: string;
  ref?: Ref<HTMLDivElement>;
};

export function TabPanel({ gap, offset, className, ...props }: TabPanelProps) {
  return <AriaTabPanel {...props} className={tabs({ gap, offset }).panel({ class: className })} />;
}
