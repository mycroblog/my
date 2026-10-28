// SPDX-License-Identifier: AGPL-3.0-only
// A timeline as a WAI-ARIA feed (web-client/README.md Y6, ADR-0027; https://www.w3.org/WAI/ARIA/apg/patterns/feed/):
// role="feed" labelled by a visible heading, one focusable <article> per item with aria-posinset and
// aria-setsize, aria-busy while items are loading, and the feed keys:
//
//   Page Down       focus the next article
//   Page Up         focus the previous article
//   Control + End   leave the feed: focus the first focusable element after it (the "load more" button)
//   Control + Home  leave the feed: focus the last focusable element before it
//
// A widget inside an article that uses these keys itself (a text field, a list box) keeps them: the feed
// ignores keys whose default the widget prevented and keys typed into text fields.
//
// Only the articles near the viewport are mounted (TanStack Virtual, ADR-0025), and the article that has
// focus stays mounted when it scrolls away, so focus is never lost to virtualization.
import { defaultRangeExtractor, type Range, useWindowVirtualizer, type VirtualItem } from "@tanstack/react-virtual";
import {
  type ComponentPropsWithRef,
  createContext,
  type Key,
  type ReactNode,
  type Ref,
  use,
  useEffect,
  useEffectEvent,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { mergeRefs, useKeyboard } from "react-aria";
import { tv } from "tailwind-variants";
import { Button } from "./Button";
import { Spinner } from "./Spinner";

const feed = tv({
  slots: {
    base: "flex flex-col",
    // Items inserted above are kept in place by the feed itself when `keepPosition` is set, so the browser must not also anchor.
    list: "relative w-full",
    top: "h-0",
    more: "flex justify-center p-4",
  },
});

// The space between two articles (`gap-3`, 0.75rem at the default spacing), kept by the virtualizer because the articles are
// positioned absolutely. Cards are raised, so they sit apart.
const cardGap = 12;

const article = tv({ base: "outline-none focus-visible:focus-ring" });

/** What the feed tells each article about its place in the feed. */
type FeedItem = {
  index: number;
  setSize: number;
  /** The article's offset from the top of the feed, in px. */
  offset: number;
  ref: (node: HTMLElement | null) => void;
};

const FeedItemContext = createContext<FeedItem | null>(null);

export type FeedArticleProps = ComponentPropsWithRef<"article">;

/**
 * One article of a Feed. Inside a Feed it is focusable (`tabindex="0"`) and carries its `aria-posinset`,
 * `aria-setsize` and position; outside one it is a plain `<article>`. Label it with `aria-labelledby`
 * (the author and time) and describe it with `aria-describedby` (the content), as PostCard does.
 */
export function FeedArticle({ ref, style, className, ...props }: FeedArticleProps) {
  const item = use(FeedItemContext);
  if (item === null) return <article {...props} ref={ref} style={style} className={className} />;
  return (
    <article
      {...props}
      ref={mergeRefs(ref, item.ref)}
      tabIndex={0}
      aria-posinset={item.index + 1}
      aria-setsize={item.setSize}
      data-index={item.index}
      style={{
        ...style,
        position: "absolute",
        insetBlockStart: 0,
        insetInline: 0,
        transform: `translateY(${String(item.offset)}px)`,
      }}
      className={article({ className })}
    />
  );
}

type VirtualFeedOptions = {
  count: number;
  estimateSize: number;
  gap: number;
  overscan: number;
  scrollMargin: number;
  getItemKey: (index: number) => Key;
  rangeExtractor: (range: Range) => number[];
};

type VirtualFeed = {
  virtualItems: VirtualItem[];
  totalSize: number;
  measureElement: (node: HTMLElement | null) => void;
  /** The item (its key, and its top edge relative to the viewport) that crosses the top of the viewport, or undefined. */
  readAnchor: () => FeedAnchor | undefined;
  /** The top edge of the item at an index relative to the viewport, or undefined when there is no such item. */
  viewportTopOf: (index: number) => number | undefined;
};

/** The item the reader is looking at, remembered to keep it in place when items are inserted above it. */
type FeedAnchor = { key: Key; index: number; viewportTop: number };

/**
 * The window virtualizer, read into plain values. The virtualizer is one mutable object for the life of
 * the component: its methods return new results after a scroll while the object stays the same, so a
 * memoized read of it would be stale. React Compiler would memoize exactly that (the compiler knows only
 * `useVirtualizer` as incompatible, not `useWindowVirtualizer`), so this hook alone opts out of the
 * compiler. Its callers see values that change when the result changes and are compiled as usual.
 */
function useVirtualFeed(options: VirtualFeedOptions): VirtualFeed {
  // eslint-disable-next-line restrict/syntax -- R4: the virtualizer is a mutable object React Compiler would read stale (see above).
  "use no memo";
  // eslint-disable-next-line react/incompatible-library -- R4: this hook opts out of the compiler for this library (see above); Oxlint's port of the rule reports inside "use no memo" too.
  const virtualizer = useWindowVirtualizer<HTMLElement>({
    count: options.count,
    estimateSize: () => options.estimateSize,
    gap: options.gap,
    overscan: options.overscan,
    scrollMargin: options.scrollMargin,
    getItemKey: options.getItemKey,
    rangeExtractor: options.rangeExtractor,
  });
  return {
    virtualItems: virtualizer.getVirtualItems(),
    totalSize: virtualizer.getTotalSize(),
    measureElement: virtualizer.measureElement,
    // Both read the virtualizer when called, in an effect or an event handler, never during render.
    readAnchor: () => {
      const scrollY = window.scrollY;
      const item = virtualizer.measurementsCache.find((measured) => measured.end > scrollY);
      return item === undefined ? undefined : { key: item.key, index: item.index, viewportTop: item.start - scrollY };
    },
    viewportTopOf: (index) => {
      const item = virtualizer.measurementsCache[index];
      return item === undefined ? undefined : item.start - window.scrollY;
    },
  };
}

/** Elements that can take focus with Tab, in the selectors browsers use; filtered further by `isTabbable`. */
const focusable =
  "a[href], area[href], button, input, select, textarea, iframe, summary, [contenteditable], [tabindex]";

function isTabbable(element: HTMLElement): boolean {
  return element.tabIndex >= 0 && !element.matches(":disabled") && element.closest("[hidden], [inert]") === null;
}

/** The focusable element nearest to the feed on one side of it, outside it. */
function tabbableOutside(feedElement: HTMLElement, side: "before" | "after"): HTMLElement | undefined {
  const candidates = [...feedElement.ownerDocument.querySelectorAll<HTMLElement>(focusable)].filter(
    (element) => !feedElement.contains(element) && isTabbable(element),
  );
  const position = side === "after" ? Node.DOCUMENT_POSITION_FOLLOWING : Node.DOCUMENT_POSITION_PRECEDING;
  const onSide = candidates.filter((element) => (feedElement.compareDocumentPosition(element) & position) !== 0);
  return side === "after" ? onSide[0] : onSide.at(-1);
}

/** Whether a key press belongs to a text field (or an editable region) rather than to the feed. */
function isTextEntry(target: EventTarget): boolean {
  return target instanceof HTMLElement && (target.isContentEditable || target.matches("input, textarea, select"));
}

export type FeedProps<T> = Omit<
  ComponentPropsWithRef<"div">,
  "children" | "role" | "aria-label" | "aria-labelledby" | "aria-busy"
> & {
  /** The loaded items, oldest last. */
  items: readonly T[];
  /** A stable key of an item (its ID). */
  getKey: (item: T) => string;
  /** Draws one item. It must render one `FeedArticle` (PostCard is one). */
  renderItem: (item: T, index: number) => ReactNode;
  /** The ID of the visible heading that names the feed. A feed with a visible label is labelled by it. */
  "aria-labelledby": string;
  /**
   * The number of items in the whole feed, for `aria-setsize`. Without it, the size is the number of loaded
   * items once there is nothing more to load, and -1 (unknown) while there is.
   */
  setSize?: number;
  /** Whether items are being loaded. The feed is `aria-busy` exactly while this is true. */
  isLoading: boolean;
  /** Whether there are older items to load. */
  hasMore: boolean;
  /**
   * Loads the next page. It is called when the end of the feed comes near the viewport and when the
   * "load more" button is pressed, never while `isLoading` or without `hasMore`.
   */
  onLoadMore: () => void;
  /** The text of the "load more" button ("Load more posts"). */
  loadMoreLabel: string;
  /** What the busy button announces while loading ("Loading posts"). */
  loadingLabel: string;
  /** The expected height of an article in px, used until it is measured. */
  estimateSize?: number;
  /** How many articles to keep mounted beyond each end of the viewport. */
  overscan?: number;
  /** A ref to the element with role="feed". */
  feedRef?: Ref<HTMLDivElement> | undefined;
  /**
   * Keeps the item the reader is looking at where it is on screen when items are inserted above it, unless the
   * feed's top is in view (then the new items show). The feed does this itself, since the browser's scroll
   * anchoring is not in every browser (Safari), and turns the browser's off for it (ADR-0077).
   */
  keepPosition?: boolean | undefined;
  /** Called with whether the feed's first item is in view, when that changes (the reader is at the top). */
  onTopChange?: ((atTop: boolean) => void) | undefined;
};

/**
 * A timeline that scrolls with the page. Items are not inserted above the reader on their own unless the caller sets
 * `keepPosition`; by default show a NewItemsButton above it instead (web-client/README.md Y7, ADR-0077). The keys are listed in the module comment above;
 * document them to users where the feed is used, since the feed role has no keyboard convention.
 */
export function Feed<T>({
  items,
  getKey,
  renderItem,
  "aria-labelledby": labelledBy,
  setSize,
  isLoading,
  hasMore,
  onLoadMore,
  loadMoreLabel,
  loadingLabel,
  estimateSize = 200,
  overscan = 4,
  feedRef,
  keepPosition = false,
  onTopChange,
  className,
  ...props
}: FeedProps<T>) {
  const slots = feed();
  const listRef = useRef<HTMLDivElement>(null);
  const moreRef = useRef<HTMLDivElement>(null);
  /** The article to focus as soon as it is mounted, after Page Up / Page Down to an article not mounted yet. */
  const pendingFocusRef = useRef<string | null>(null);
  // The key, not the index: items inserted above must not move focus to another article.
  const [focusedKey, setFocusedKey] = useState<string | null>(null);
  // The feed's distance from the top of the document, which the window virtualizer needs.
  const [scrollMargin, setScrollMargin] = useState(0);

  const focusedIndex = focusedKey === null ? -1 : items.findIndex((item) => getKey(item) === focusedKey);
  const size = setSize ?? (hasMore ? -1 : items.length);

  const { virtualItems, totalSize, measureElement, readAnchor, viewportTopOf } = useVirtualFeed({
    count: items.length,
    estimateSize,
    gap: cardGap,
    overscan,
    scrollMargin,
    getItemKey: (index) => {
      const item = items[index];
      return item === undefined ? index : getKey(item);
    },
    rangeExtractor: (range) => {
      const indexes = defaultRangeExtractor(range);
      if (focusedIndex < 0 || indexes.includes(focusedIndex)) return indexes;
      return [...indexes, focusedIndex].toSorted((a, b) => a - b);
    },
  });

  // Content above the feed (a header, a NewItemsButton) moves it; the document's size changes with it.
  useEffect(() => {
    const list = listRef.current;
    if (list === null || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(() => {
      setScrollMargin(list.getBoundingClientRect().top + window.scrollY);
    });
    observer.observe(list.ownerDocument.body);
    return () => {
      observer.disconnect();
    };
  }, []);

  // Whether the feed's top is in view, seen through a marker at its start (a ref, so that a change renders nothing).
  const topRef = useRef<HTMLDivElement>(null);
  const atTopRef = useRef(true);
  const topChanged = useEffectEvent((atTop: boolean) => {
    onTopChange?.(atTop);
  });
  useEffect(() => {
    const top = topRef.current;
    if (top === null || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver((entries) => {
      const last = entries.at(-1);
      if (last === undefined || last.isIntersecting === atTopRef.current) return;
      atTopRef.current = last.isIntersecting;
      topChanged(last.isIntersecting);
    });
    observer.observe(top);
    return () => {
      observer.disconnect();
    };
  }, []);

  // The item being read, as it was on screen before the last render; items inserted above it move it down in the
  // layout, and the scroll position is corrected before the browser paints (react.dev: useLayoutEffect).
  const anchorRef = useRef<FeedAnchor | undefined>(undefined);
  const rememberAnchor = () => {
    if (keepPosition) anchorRef.current = readAnchor();
  };
  useEffect(() => {
    if (!keepPosition) return;
    window.addEventListener("scroll", rememberAnchor, { passive: true });
    return () => {
      window.removeEventListener("scroll", rememberAnchor);
    };
  });
  useLayoutEffect(() => {
    const anchor = anchorRef.current;
    if (keepPosition && anchor !== undefined && !atTopRef.current) {
      const index = items.findIndex((item) => getKey(item) === anchor.key);
      // Only a move of the item's index means items were inserted above it; a resize or a scroll moves nothing to correct.
      const top = index < 0 || index === anchor.index ? undefined : viewportTopOf(index);
      if (top !== undefined && Math.abs(top - anchor.viewportTop) >= 1)
        window.scrollBy({ top: top - anchor.viewportTop, behavior: "instant" });
    }
    rememberAnchor();
  });

  // Load the next page when the "load more" button comes near the viewport. The observer is created again
  // when a page has loaded, so a short page that leaves the button in view loads the next one too.
  const loadMore = useEffectEvent(() => {
    onLoadMore();
  });
  useEffect(() => {
    const more = moreRef.current;
    if (!hasMore || isLoading || more === null || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadMore();
      },
      { rootMargin: "0px 0px 50% 0px" },
    );
    observer.observe(more);
    return () => {
      observer.disconnect();
    };
  }, [hasMore, isLoading, items.length]);

  const articleAt = (index: number) =>
    listRef.current?.querySelector<HTMLElement>(`:scope > [data-index="${String(index)}"]`) ?? null;

  const focusArticle = (index: number) => {
    const item = items[index];
    if (item === undefined) return;
    const key = getKey(item);
    setFocusedKey(key);
    const element = articleAt(index);
    if (element === null) {
      // Not mounted: the range now includes it (focusedKey), and its ref focuses it once it is.
      pendingFocusRef.current = key;
    } else {
      element.focus();
    }
  };

  /** The index of the feed article that contains a node, or -1. */
  const indexOf = (target: EventTarget | null) => {
    if (!(target instanceof Element)) return -1;
    const element = target.closest("[data-index]");
    if (element === null || element.parentElement !== listRef.current) return -1;
    return Number(element.getAttribute("data-index"));
  };

  const { keyboardProps } = useKeyboard({
    onKeyDown: (event) => {
      const list = listRef.current;
      const index = indexOf(event.target);
      if (
        list === null ||
        index < 0 ||
        event.isDefaultPrevented() ||
        isTextEntry(event.target) ||
        event.altKey ||
        event.shiftKey
      ) {
        event.continuePropagation();
        return;
      }
      if ((event.key === "PageDown" || event.key === "PageUp") && !event.ctrlKey && !event.metaKey) {
        const next = event.key === "PageDown" ? index + 1 : index - 1;
        if (next >= 0 && next < items.length) {
          event.preventDefault();
          focusArticle(next);
        }
        return;
      }
      if ((event.key === "End" || event.key === "Home") && event.ctrlKey && !event.metaKey) {
        const target = tabbableOutside(list, event.key === "End" ? "after" : "before");
        if (target !== undefined) {
          event.preventDefault();
          target.focus();
        }
        return;
      }
      event.continuePropagation();
    },
  });

  return (
    <div {...props} className={slots.base({ className })}>
      {(keepPosition || onTopChange !== undefined) && <div ref={topRef} aria-hidden="true" className={slots.top()} />}
      <div
        {...keyboardProps}
        ref={mergeRefs(listRef, feedRef)}
        role="feed"
        aria-labelledby={labelledBy}
        aria-busy={isLoading}
        className={slots.list({ className: keepPosition ? "[overflow-anchor:none]" : undefined })}
        style={{ blockSize: totalSize }}
        onFocus={(event) => {
          const index = indexOf(event.target);
          const item = items[index];
          if (item !== undefined) setFocusedKey(getKey(item));
        }}
        onBlur={(event) => {
          if (!(event.relatedTarget instanceof Node) || !event.currentTarget.contains(event.relatedTarget))
            setFocusedKey(null);
        }}
      >
        {virtualItems.map((virtualItem) => {
          const item = items[virtualItem.index];
          if (item === undefined) return null;
          const key = getKey(item);
          const value: FeedItem = {
            index: virtualItem.index,
            setSize: size,
            offset: virtualItem.start - scrollMargin,
            ref: (node) => {
              measureElement(node);
              if (node !== null && pendingFocusRef.current === key) {
                pendingFocusRef.current = null;
                node.focus();
              }
            },
          };
          return (
            <FeedItemContext key={key} value={value}>
              {renderItem(item, virtualItem.index)}
            </FeedItemContext>
          );
        })}
      </div>
      {hasMore && (
        <div ref={moreRef} className={slots.more()}>
          <Button isPending={isLoading} onPress={onLoadMore}>
            {isLoading && <Spinner size="sm" label={loadingLabel} className="text-current" />}
            {loadMoreLabel}
          </Button>
        </div>
      )}
    </div>
  );
}
