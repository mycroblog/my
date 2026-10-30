// SPDX-License-Identifier: AGPL-3.0-only
// The list-and-add part of ADR-0110 §5 (web-client/21-experience-principles.md P1, P2, P4): a titled region whose main
// content is a list with its count, and the one place to add to it. Its heading line holds the `h2`, the count and the
// add button (the region's one primary action); pressing it opens the adding panel in its place, under the heading,
// and closing the panel gives focus back to the button. An empty list shows an empty state whose action is that same
// button, so that the button is in one place at a time. The result of adding and removing shows under the heading line,
// above the rows (`ActionResult`), where the list changes. A screen builds every list it adds to from this part, and
// writes no list region of its own.
import { type ReactNode, type Ref, useEffect, useId, useRef, useState } from "react";
import { ActionResult, type ActionResultValue } from "./ActionResult";
import { Badge } from "./Badge";
import { Box } from "./Box";
import { Button } from "./Button";
import { Cluster } from "./Cluster";
import { EmptyState } from "./EmptyState";
import { Heading } from "./Heading";
import { SectionHeader } from "./SectionHeader";
import { Stack } from "./Stack";
import { Hint } from "./Text";

/** How the region adds to its list. */
export type ListSectionAdd =
  | {
      /** The add button's words ("Add a passkey"), also the title (`h3`) of the panel it opens. */
      label: string;
      /**
       * The panel's content: a form (ending in a `FormActionBar` with its Cancel calling `close`) or a choice. `close`
       * closes the panel and gives focus back to the add button; call it once the item is added, too.
       */
      panel: (close: () => void) => ReactNode;
      /** Opens the panel on the first render: the page came back from a step outside the app with the panel's input. */
      defaultOpen?: boolean;
      unavailable?: never;
    }
  | {
      label?: never;
      panel?: never;
      defaultOpen?: never;
      /** Why nothing can be added now (the list is full, the browser cannot), in place of the add button. */
      unavailable: ReactNode;
    };

export type ListSectionProps<T> = {
  /** The title: the `h2` that names the region. */
  title: ReactNode;
  /** A note on the whole list under the heading line (`Hint`), which also describes the region. */
  description?: ReactNode;
  /**
   * What narrows the list (a search, the filters), under the heading line and the adding panel and above the result:
   * it belongs to the list, so it sits inside the region.
   */
  toolbar?: ReactNode;
  /** The items, in the order of the list. */
  items: readonly T[];
  /** A stable key of an item, also the row's `data-row-id` (for moving focus after a row goes away). */
  itemKey: (item: T) => string;
  /** The content of an item's row: its name and attributes first, then its actions. The part brings the `li`. */
  renderItem: (item: T) => ReactNode;
  /** What the count means, read instead of the drawn digits ("2 passkeys"). */
  countLabel: string;
  /** More items than `items` exist (the list loads in pages): the count is drawn as a lower bound ("40+"). */
  hasMore?: boolean;
  /** The most items the list may hold: the count is drawn over it ("3 / 50"); `countLabel` then says both. */
  limit?: number;
  /** The button that loads the next page, while `hasMore`. */
  more?: { label: string; onPress: () => void; isPending: boolean };
  /** Adding to the list. */
  add: ListSectionAdd;
  /** What the empty list shows: its heading and a sentence that leads to adding. */
  empty: { heading: ReactNode; description?: ReactNode };
  /** The result of the last add or removal, already translated; nothing before the first one. */
  result?: ActionResultValue | undefined;
  /** The `ul` of the rows, for a hook that moves focus between them. */
  listRef?: Ref<HTMLUListElement>;
};

/** A titled list with its count, the one button that adds to it, and the result of the last change above its rows. */
export function ListSection<T>({
  title,
  description,
  toolbar,
  items,
  itemKey,
  renderItem,
  countLabel,
  hasMore = false,
  limit,
  more,
  add,
  empty,
  result,
  listRef,
}: ListSectionProps<T>) {
  const headingId = useId();
  const descriptionId = useId();
  const panelHeadingId = useId();
  // `closed` before the panel first opens; `resumed` when it is open from the first render (focus stays where the page
  // puts it); `opened` once the add button opened it, which moves focus into it; `returned` once it closed, which gives
  // the add button focus back.
  const [panel, setPanel] = useState<"closed" | "resumed" | "opened" | "returned">(
    add.panel !== undefined && add.defaultOpen === true ? "resumed" : "closed",
  );
  const isOpen = panel === "resumed" || panel === "opened";
  const addButtonId = useId();
  const panelRef = useRef<HTMLElement>(null);
  // Focus is moved in the DOM, outside React, once the panel or the button has rendered: into the panel's first field
  // (else its heading) when it opens, back to the add button when it closes.
  useEffect(() => {
    if (panel === "returned") document.getElementById(addButtonId)?.focus();
    if (panel !== "opened") return;
    const element = panelRef.current;
    const first = element?.querySelector<HTMLElement>("input, textarea, select, button");
    (first ?? element?.querySelector<HTMLElement>("h3"))?.focus();
  }, [panel, addButtonId]);
  const close = () => {
    setPanel("returned");
  };
  const isEmpty = items.length === 0;
  const addButton =
    add.panel === undefined ? undefined : (
      <Button
        id={addButtonId}
        variant="primary"
        onPress={() => {
          setPanel("opened");
        }}
      >
        {add.label}
      </Button>
    );
  return (
    <Stack
      as="section"
      gap="4"
      aria-labelledby={headingId}
      aria-describedby={description === undefined ? undefined : descriptionId}
    >
      <Stack gap="2" container>
        <SectionHeader
          actions={
            // The button sits here while the list has items and the panel is closed; the empty state holds it otherwise.
            add.unavailable === undefined ? !isOpen && !isEmpty && addButton : <Hint>{add.unavailable}</Hint>
          }
        >
          <Cluster gap="2" align="center" shrink>
            <Heading level={2} id={headingId}>
              {title}
            </Heading>
            <Badge
              tone="neutral"
              count={items.length}
              atLeast={hasMore}
              max={999}
              {...(limit === undefined ? {} : { limit })}
              label={countLabel}
            />
          </Cluster>
        </SectionHeader>
        {description !== undefined && <Hint id={descriptionId}>{description}</Hint>}
      </Stack>
      {isOpen && add.panel !== undefined && (
        <Box as="section" ref={panelRef} aria-labelledby={panelHeadingId} border="lg" background="sunken" padding="4">
          <Stack gap="4">
            <Heading level={3} id={panelHeadingId} focusable>
              {add.label}
            </Heading>
            {add.panel(close)}
          </Stack>
        </Box>
      )}
      {toolbar}
      <div>
        <ActionResult result={result} />
      </div>
      {isEmpty ? (
        <EmptyState
          headingLevel={3}
          heading={empty.heading}
          {...(empty.description === undefined ? {} : { description: empty.description })}
          {...(isOpen || addButton === undefined ? {} : { action: addButton })}
        />
      ) : (
        <Stack gap="4">
          <Stack as="ul" ref={listRef} divided>
            {items.map((item) => {
              const key = itemKey(item);
              return (
                <Box as="li" key={key} data-row-id={key} container paddingY="3">
                  {renderItem(item)}
                </Box>
              );
            })}
          </Stack>
          {hasMore && more !== undefined && (
            <div>
              <Button onPress={more.onPress} isPending={more.isPending}>
                {more.label}
              </Button>
            </div>
          )}
        </Stack>
      )}
    </Stack>
  );
}
